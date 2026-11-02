package worker

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
)

// EgressHealthSource memasok daftar egress pool beserta pencatatan
// kesehatannya. Dipenuhi *upstream.EgressRepo.
type EgressHealthSource interface {
	List(ctx context.Context, enabledOnly bool) ([]*upstream.EgressPool, error)
	ProxyURL(ctx context.Context, id string) (security.Secret, error)
	RecordHealth(ctx context.Context, id string, status string, latencyMS *int) error
}

// batasProbeEgress membatasi satu kali probe kesehatan egress.
const batasProbeEgress = 7 * time.Second

// probeEgressPool memeriksa satu egress pool lewat jalur yang BENAR-BENAR
// dipakai lalu lintasnya, bukan satu URL hardcode untuk semua jenis pool
// (audit3 V-EO-05):
//
//   - forward proxy: request ke endpoint trace Cloudflare LEWAT proxy —
//     yang diuji adalah proxy-nya beserta jalan keluarnya;
//   - relay Deno: request ke host relay pool itu sendiri dengan header
//     x-target-host ke Cloudflare — yang diuji adalah relay milik pool;
//   - gateway Cloudflare: request ke URL gateway MILIK pool (akun + gateway
//     di path) — respons HTTP apa pun membuktikan gateway itu ada dan edge
//     menjangkaunya; versi lama mem-probe domain trace umum yang tidak
//     menguji gateway milik pool sama sekali.
//
// Hasilnya hanya sehat/tidak beserta latensi; rincian exit IP dsb. tetap
// wilayah probe manual admin yang lebih kaya.
func probeEgressPool(ctx context.Context, rawProxy string, policy security.SSRFPolicy) (bool, time.Duration) {
	jenis, pu := security.KlasifikasiRelay(rawProxy)
	if pu == nil {
		return false, 0
	}

	dialer := &net.Dialer{Timeout: batasProbeEgress}
	probeCtx, cancel := context.WithTimeout(ctx, batasProbeEgress)
	defer cancel()

	var target string
	transport := &http.Transport{
		DialContext:       security.GuardedDialContext(policy, dialer),
		DisableKeepAlives: true,
	}
	header := http.Header{}

	switch jenis {
	case security.RelayCloudflare:
		// URL gateway milik pool itu sendiri, apa adanya.
		target = strings.TrimRight(rawProxy, "/")
	case security.RelayDeno:
		target = pu.Scheme + "://" + pu.Host + "/"
		header.Set("x-target-host", "cloudflare.com")
		if pu.User != nil {
			if token, ok := pu.User.Password(); ok && token != "" {
				header.Set("x-relay-token", token)
			}
		}
	default:
		transport.Proxy = http.ProxyURL(pu)
		target = "https://cloudflare.com/cdn-cgi/trace"
	}

	client := &http.Client{Transport: transport, Timeout: batasProbeEgress}
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, target, nil)
	if err != nil {
		return false, 0
	}
	req.Header = header
	req.Header.Set("User-Agent", "Route-X-Probe/1.0")

	mulai := time.Now()
	resp, err := client.Do(req)
	latensi := time.Since(mulai)
	if err != nil {
		return false, latensi
	}
	defer resp.Body.Close()

	if jenis == security.RelayCloudflare {
		// Gateway yang ada menjawab sesuatu (401/404 termasuk): yang
		// membuktikan kesehatan jalurnya adalah edge menjangkau gateway
		// milik pool, bukan status aplikasinya.
		return true, latensi
	}
	return resp.StatusCode == http.StatusOK, latensi
}

// periksaEgress menjalankan satu putaran probe untuk seluruh pool aktif dan
// mencatatnya. Kegagalan satu pool tidak menghentikan pool lain.
func (h *HealthCheckerJob) periksaEgress(ctx context.Context) {
	pools, err := h.egress.List(ctx, true)
	if err != nil {
		h.logger.WarnContext(ctx, "gagal mengambil daftar egress pool untuk health check", "error", err)
		return
	}
	for _, pool := range pools {
		raw, err := h.egress.ProxyURL(ctx, pool.ID)
		if err != nil {
			h.logger.WarnContext(ctx, "gagal membuka URL egress pool untuk health check",
				"egress_pool_id", pool.ID, "error", err)
			if rerr := h.egress.RecordHealth(ctx, pool.ID, "unhealthy", nil); rerr != nil {
				h.logger.WarnContext(ctx, "gagal mencatat kesehatan egress pool",
					"egress_pool_id", pool.ID, "error", rerr)
			}
			continue
		}
		sehat, latensi := probeEgressPool(ctx, raw.Reveal(), h.egressPolicy)
		status := "unhealthy"
		var latMS *int
		if sehat {
			status = "healthy"
			v := int(latensi.Milliseconds())
			latMS = &v
		}
		if err := h.egress.RecordHealth(ctx, pool.ID, status, latMS); err != nil {
			h.logger.WarnContext(ctx, "gagal mencatat kesehatan egress pool",
				"egress_pool_id", pool.ID, "error", err)
		}
	}
}
