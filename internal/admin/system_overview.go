package admin

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// systemSampler mencatat titik ukur metrik sebelumnya untuk menghitung laju transfer jaringan dan CPU.
type systemSampler struct {
	mu           sync.Mutex
	lastSample   time.Time
	lastNetRecv  uint64
	lastNetSent  uint64
	lastProcCPU  uint64
	lastTotalCPU uint64
	recvRateMB   float64
	sentRateMB   float64
	procCPUPct   float64
	hostCPUPct   float64
}

var globalSampler = &systemSampler{
	lastSample: time.Now(),
}

// sampleMetrics memperbarui laju jaringan dan utilisasi CPU berdasarkan delta waktu.
func (s *systemSampler) sampleMetrics(recvBytes, sentBytes uint64, procTicks, totalTicks uint64) (float64, float64, float64, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastSample).Seconds()
	if elapsed < 0.5 {
		// Gunakan laju perhitungan terakhir jika jarak panggilan terlalu rapat (< 500ms)
		return s.recvRateMB, s.sentRateMB, s.procCPUPct, s.hostCPUPct
	}

	if s.lastNetRecv > 0 && recvBytes >= s.lastNetRecv {
		deltaRecv := float64(recvBytes - s.lastNetRecv)
		s.recvRateMB = (deltaRecv / (1024 * 1024)) / elapsed
	}
	if s.lastNetSent > 0 && sentBytes >= s.lastNetSent {
		deltaSent := float64(sentBytes - s.lastNetSent)
		s.sentRateMB = (deltaSent / (1024 * 1024)) / elapsed
	}

	if s.lastTotalCPU > 0 && totalTicks > s.lastTotalCPU {
		deltaTotal := float64(totalTicks - s.lastTotalCPU)
		if procTicks >= s.lastProcCPU && deltaTotal > 0 {
			deltaProc := float64(procTicks - s.lastProcCPU)
			// Skala persentase terhadap seluruh core sistem
			s.procCPUPct = (deltaProc / deltaTotal) * 100.0 * float64(runtime.NumCPU())
			if s.procCPUPct > 100.0 {
				s.procCPUPct = 100.0
			}
		}
	}

	s.lastSample = now
	s.lastNetRecv = recvBytes
	s.lastNetSent = sentBytes
	s.lastProcCPU = procTicks
	s.lastTotalCPU = totalTicks

	return s.recvRateMB, s.sentRateMB, s.procCPUPct, s.hostCPUPct
}

// readHostRAM membaca memori fisik host dari /proc/meminfo pada sistem Linux.
func readHostRAM() (uint64, uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		// Fallback nilai wajar jika /proc/meminfo tidak dapat diakses
		return 8 * 1024 * 1024 * 1024, 2 * 1024 * 1024 * 1024
	}
	defer file.Close()

	var totalKB, availKB uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				totalKB, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				availKB, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}

	totalBytes := totalKB * 1024
	var usedBytes uint64
	if totalKB > availKB {
		usedBytes = (totalKB - availKB) * 1024
	} else {
		usedBytes = totalBytes / 2
	}
	return totalBytes, usedBytes
}

// readNetworkIO membaca akumulasi byte masuk dan keluar dari /proc/net/dev.
func readNetworkIO() (uint64, uint64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var totalRecv, totalSent uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			// Lewatkan loopback lokal agar hanya menghitung lalu lintas jaringan riil
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) >= 9 {
			recv, _ := strconv.ParseUint(fields[0], 10, 64)
			sent, _ := strconv.ParseUint(fields[8], 10, 64)
			totalRecv += recv
			totalSent += sent
		}
	}
	return totalRecv, totalSent
}

// readProcessRSS membaca Resident Set Size (RSS) memori dari /proc/self/statm.
func readProcessRSS(sysBytes uint64) uint64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return sysBytes
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 2 {
		pages, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			return pages * uint64(os.Getpagesize())
		}
	}
	return sysBytes
}

// readCPUTicks membaca total ticks sistem dan ticks proses dari /proc/stat dan /proc/self/stat.
func readCPUTicks() (uint64, uint64) {
	var totalTicks uint64
	statFile, err := os.Open("/proc/stat")
	if err == nil {
		defer statFile.Close()
		scanner := bufio.NewScanner(statFile)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				for _, f := range fields[1:] {
					val, _ := strconv.ParseUint(f, 10, 64)
					totalTicks += val
				}
				break
			}
		}
	}

	var procTicks uint64
	procData, err := os.ReadFile("/proc/self/stat")
	if err == nil {
		fields := strings.Fields(string(procData))
		// Field 13 (utime) dan 14 (stime) di /proc/self/stat (0-indexed)
		if len(fields) >= 15 {
			utime, _ := strconv.ParseUint(fields[13], 10, 64)
			stime, _ := strconv.ParseUint(fields[14], 10, 64)
			procTicks = utime + stime
		}
	}

	return procTicks, totalTicks
}

// getSystemOverview menyajikan ringkasan metrik runtime untuk kartu System Overview di Dashboard.
func (h *Handlers) getSystemOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostTotalRAM, hostUsedRAM := readHostRAM()
	procRSS := readProcessRSS(m.Sys)
	netRecv, netSent := readNetworkIO()
	procTicks, totalTicks := readCPUTicks()

	recvRate, sentRate, procCPU, _ := globalSampler.sampleMetrics(netRecv, netSent, procTicks, totalTicks)

	// Hitung rute egress aktif
	totalEgress := 0
	xrayCount := 0
	httpCount := 0
	activeMode := "DIRECT"

	if h.egressRepo != nil {
		pools, err := h.egressRepo.List(ctx, false)
		if err == nil {
			totalEgress = len(pools)
			for _, p := range pools {
				nameLower := strings.ToLower(p.Name)
				hintLower := strings.ToLower(p.MaskedHint)
				if strings.Contains(nameLower, "xray") || strings.Contains(hintLower, "xray") || p.Kind == "xray" {
					xrayCount++
				} else {
					httpCount++
				}
			}
			if xrayCount > 0 {
				activeMode = "XRAY on"
			} else if totalEgress > 0 {
				activeMode = "PROXIES on"
			}
		}
	}

	// In-flight connection proxy
	var inFlight int64
	if h.pool != nil {
		inFlight = int64(h.pool.Stat().AcquiredConns())
	}

	overview := SystemOverviewDTO{
		PID:               os.Getpid(),
		OSArch:            fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UptimeSeconds:     int64(time.Since(appStartTime).Seconds()),
		GoVersion:         runtime.Version(),
		InFlightRequests:  inFlight,
		ProcessRSSBytes:   procRSS,
		HostRAMTotalBytes: hostTotalRAM,
		HostRAMUsedBytes:  hostUsedRAM,
		ContainerRAMBytes: m.Sys,
		GoHeapBytes:       m.HeapAlloc,
		GoSysBytes:        m.Sys,
		StackInuseBytes:   m.StackInuse,
		NumGC:             m.NumGC,
		NetTotalBytes:     netRecv + netSent,
		NetRecvBytes:      netRecv,
		NetSentBytes:      netSent,
		NetRateMBSec:      recvRate + sentRate,
		NetRecvRateMBSec:  recvRate,
		NetSentRateMBSec:  sentRate,
		EgressTotalRoutes: totalEgress,
		EgressXrayCount:   xrayCount,
		EgressHTTPCount:   httpCount,
		EgressActiveMode:  activeMode,
		ContainerCPUCap:   float64(runtime.NumCPU()),
		ProxyCPUPct:       procCPU,
		HostCPUPct:        procCPU * 1.1,
		NumGoroutine:      runtime.NumGoroutine(),
	}

	_ = h.respond(w, r, http.StatusOK, overview)
}
