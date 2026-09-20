package admin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
)

const (
	// SettingKeyDomainConfig adalah kunci penyimpanan setelan domain di tabel database settings.
	SettingKeyDomainConfig = "system:domain:config"
	// SettingKeyPublicURL adalah kunci penyimpanan URL kanonikal publik sistem.
	SettingKeyPublicURL = "system:public_url"
)

var (
	domainRegex       = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	serverIPCache     string
	serverIPCacheTime time.Time
	serverIPMu        sync.Mutex
	// tlsCheckLimiter membatasi panggilan CaddyTLSCheck: Caddy memanggilnya
	// sekali per TLS handshake domain baru, jadi 60/menit longgar untuk
	// operasional normal tetapi menahan scraper lokal yang lepas kendali.
	// Ember lokal (bukan Redis): endpoint ini justru dipakai sebelum
	// infrastruktur lain terbukti sehat.
	tlsCheckLimiter = newDetikEmber(60, time.Minute)
)

// detikEmber adalah pembatas laju jendela-tetap per proses: sederhana karena
// hanya dipakai satu endpoint internal. Mutex melindungi hitungan; jendela
// dihitung dari jam dinding, bukan timer, supaya tidak ada goroutine yang
// berjalan selamanya.
type detikEmber struct {
	mu         sync.Mutex
	batas      int
	jendela    time.Duration
	terpakai   int
	awalJendel time.Time
}

func newDetikEmber(batas int, jendela time.Duration) *detikEmber {
	return &detikEmber{batas: batas, jendela: jendela}
}

func (e *detikEmber) izinkan() bool {
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	if now.Sub(e.awalJendel) >= e.jendela {
		e.awalJendel = now
		e.terpakai = 0
	}
	if e.terpakai >= e.batas {
		return false
	}
	e.terpakai++
	return true
}

// StoredDomainConfig menyimpan format konfigurasi domain di database.
type StoredDomainConfig struct {
	Domain      string    `json:"domain"`
	Mode        string    `json:"mode"`
	Status      string    `json:"status"`
	LastChecked time.Time `json:"last_checked"`
	Message     string    `json:"message"`
}

// detectServerIP mendeteksi IP publik server atau fallback ke interface lokal non-loopback.
func detectServerIP() string {
	serverIPMu.Lock()
	defer serverIPMu.Unlock()

	if serverIPCache != "" && time.Since(serverIPCacheTime) < 15*time.Minute {
		return serverIPCache
	}

	// 1. Coba deteksi via ipify dengan batas waktu ketat 2 detik.
	// Transport memakai GuardedDialContext agar alamat hasil resolusi diperiksa
	// terhadap kebijakan SSRF, dan pengalihan ditolak: respons ipify tidak lain
	// adalah teks IP polos tanpa redirect.
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext:     security.GuardedDialContext(security.DefaultSSRFPolicy(), dialer),
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get("https://api.ipify.org")
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
				serverIPCache = ip
				serverIPCacheTime = time.Now()
				return ip
			}
		}
	}

	// 2. Fallback: telusuri interface host
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					serverIPCache = ipnet.IP.String()
					serverIPCacheTime = time.Now()
					return serverIPCache
				}
			}
		}
	}

	return "127.0.0.1"
}

// getDomainConfigFromDB membaca konfigurasi domain tersimpan.
func (h *Handlers) getDomainConfigFromDB(ctx context.Context) (*StoredDomainConfig, error) {
	if h.settingsRepo == nil {
		return nil, nil
	}
	s, err := h.settingsRepo.Get(ctx, SettingKeyDomainConfig)
	if err != nil {
		return nil, err
	}
	var cfg StoredDomainConfig
	if err := json.Unmarshal(s.Value, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// getDomainStatus mengembalikan informasi status domain dan konfigurasi HTTPS saat ini.
func (h *Handlers) getDomainStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverIP := detectServerIP()

	cfg, err := h.getDomainConfigFromDB(ctx)
	if err != nil || cfg == nil || cfg.Domain == "" {
		res := DomainConfigDTO{
			Domain:     "",
			Mode:       "letsencrypt",
			Status:     "unconfigured",
			PublicURL:  fmt.Sprintf("http://%s:8080", serverIP),
			BaseURL:    fmt.Sprintf("http://%s:8080/v1", serverIP),
			ServerIP:   serverIP,
			DNSMatched: false,
			Message:    "Belum ada domain kustom yang dikonfigurasi. Menggunakan akses langsung IP / localhost.",
		}
		_ = h.respond(w, r, http.StatusOK, res)
		return
	}

	// Lakukan pemeriksaan DNS live
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	ips, lookupErr := net.DefaultResolver.LookupHost(lookupCtx, cfg.Domain)
	var (
		dnsMatched bool
		status     string
		message    string
	)

	if lookupErr != nil {
		dnsMatched = false
		status = "error"
		message = fmt.Sprintf("Gagal me-resolve DNS domain %s: %v", cfg.Domain, lookupErr)
	} else {
		if cfg.Mode == "cloudflare" {
			// Mode Cloudflare Proxy: IP adalah Cloudflare Edge CDN, DNS resolve menandakan aktif
			dnsMatched = len(ips) > 0
			status = "active"
			message = "Terkoneksi via Cloudflare Proxy (SSL Edge Aktif)"
		} else {
			// Mode Let's Encrypt: periksa apakah IP server cocok dengan salah satu IP hasil resolve
			for _, ip := range ips {
				if ip == serverIP {
					dnsMatched = true
					break
				}
			}
			if dnsMatched {
				status = "active"
				message = "DNS A-Record valid dan sertifikat otomatis HTTPS aktif"
			} else {
				status = "warning"
				message = fmt.Sprintf("DNS domain mengarah ke %v, sedangkan IP server Route-X adalah %s", ips, serverIP)
			}
		}
	}

	now := time.Now().UTC()
	res := DomainConfigDTO{
		Domain:      cfg.Domain,
		Mode:        cfg.Mode,
		Status:      status,
		PublicURL:   fmt.Sprintf("https://%s", cfg.Domain),
		BaseURL:     fmt.Sprintf("https://%s/v1", cfg.Domain),
		ServerIP:    serverIP,
		ResolvedIPs: ips,
		DNSMatched:  dnsMatched,
		LastChecked: &now,
		Message:     message,
	}

	_ = h.respond(w, r, http.StatusOK, res)
}

// updateDomainConfig memvalidasi DNS, mendaftarkan domain, dan memperbarui konfigurasi sistem.
func (h *Handlers) updateDomainConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverIP := detectServerIP()

	var req UpdateDomainRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_payload", "Payload JSON tidak valid: "+err.Error())
		return
	}

	// Normalisasi nama domain
	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}
	if idx := strings.Index(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	if domain == "" || !domainRegex.MatchString(domain) {
		httpx.BadRequest(w, r, "invalid_domain", "Format nama domain tidak sah (contoh yang benar: ai.domainanda.com)")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "cloudflare" && mode != "letsencrypt" {
		mode = "letsencrypt"
	}

	// Pre-flight DNS Verification
	lookupCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(lookupCtx, domain)
	if err != nil {
		httpx.BadRequest(w, r, "dns_lookup_failed", fmt.Sprintf("Domain %q tidak dapat di-resolve melalui DNS publik (%v). Pastikan DNS Record A sudah dibuat di penyedia domain / Cloudflare Anda.", domain, err))
		return
	}

	dnsMatched := false
	if mode == "cloudflare" {
		// Pada mode Cloudflare, domain diarahkan ke CDN Cloudflare
		dnsMatched = len(ips) > 0
	} else {
		for _, ip := range ips {
			if ip == serverIP {
				dnsMatched = true
				break
			}
		}
		if !dnsMatched {
			httpx.BadRequest(w, r, "dns_mismatch", fmt.Sprintf("Domain %q saat ini mengarah ke IP %v, bukan ke IP server Route-X (%s). Harap perbarui DNS A Record Anda terlebih dahulu.", domain, ips, serverIP))
			return
		}
	}

	now := time.Now().UTC()
	stored := StoredDomainConfig{
		Domain:      domain,
		Mode:        mode,
		Status:      "active",
		LastChecked: now,
		Message:     "Domain dan HTTPS otomatis berhasil diaktifkan.",
	}

	valBytes, err := json.Marshal(stored)
	if err != nil {
		httpx.InternalError(w, r)
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	if h.settingsRepo != nil {
		// Simpan konfigurasi domain
		_, err = h.settingsRepo.Put(ctx, identity.SettingWrite{
			Key:         SettingKeyDomainConfig,
			Value:       valBytes,
			Description: "Konfigurasi domain kustom dan HTTPS otomatis Route-X",
			UpdatedBy:   actorID,
		})
		if err != nil {
			mapRepoError(w, r, err, "konfigurasi domain")
			return
		}

		// Simpan URL publik kanonikal
		pubURLBytes, _ := json.Marshal("https://" + domain)
		_, _ = h.settingsRepo.Put(ctx, identity.SettingWrite{
			Key:         SettingKeyPublicURL,
			Value:       pubURLBytes,
			Description: "URL publik kanonikal Route-X",
			UpdatedBy:   actorID,
		})
	}

	h.writeAudit(ctx, r, "configure", "system_domain", domain, map[string]any{
		"mode": mode,
		"ips":  ips,
	})

	res := DomainConfigDTO{
		Domain:      domain,
		Mode:        mode,
		Status:      "active",
		PublicURL:   fmt.Sprintf("https://%s", domain),
		BaseURL:     fmt.Sprintf("https://%s/v1", domain),
		ServerIP:    serverIP,
		ResolvedIPs: ips,
		DNSMatched:  dnsMatched,
		LastChecked: &now,
		Message:     "Domain dan HTTPS otomatis berhasil diaktifkan. Anda kini dapat mengakses dashboard dan API via HTTPS.",
	}

	_ = h.respond(w, r, http.StatusOK, res)
}

// deleteDomainConfig menghapus konfigurasi domain dan mengembalikan gateway ke akses IP langsung.
func (h *Handlers) deleteDomainConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.settingsRepo != nil {
		_ = h.settingsRepo.Delete(ctx, SettingKeyDomainConfig)
		_ = h.settingsRepo.Delete(ctx, SettingKeyPublicURL)
	}

	h.writeAudit(ctx, r, "delete", "system_domain", "", nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "ok"})
}

// CaddyTLSCheck adalah endpoint internal tanpa autentikasi yang digunakan Caddy On-Demand TLS
// untuk memverifikasi apakah suatu domain berhak diterbitkan sertifikat SSL-nya.
//
// Tanpa autentikasi karena Caddy memanggilnya sebagai HTTP biasa, jadi
// pertahanannya berlapis di sini, bukan di sesi:
//
//  1. Hanya peer loopback yang dilayani — Caddy berjalan di host yang sama.
//     Permintaan dari jaringan luar DITOLAK sebelum membaca parameter apa pun,
//     supaya endpoint ini tidak menjadi oracle enumerasi domain publik.
//  2. Rate-limit per menit per peer (ember lokal, tanpa Redis) supaya scraper
//     lokal yang lepas kendali tidak membanjiri database.
//  3. Jawaban selalu 200-untuk-diizinkan / 403-untuk-ditolak TANPA membedakan
//     "domain tidak dikenal" dari "domain dikenal tapi tidak cocok": keduanya
//     403 yang sama, supaya responsnya tidak bisa dipakai menebak konfigurasi.
func (h *Handlers) CaddyTLSCheck(w http.ResponseWriter, r *http.Request) {
	if ip, ok := httpx.PeerIPFrom(r); !ok || !ip.IsLoopback() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !tlsCheckLimiter.izinkan() {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if domain == "" {
		http.Error(w, "missing domain parameter", http.StatusBadRequest)
		return
	}

	// Selalu izinkan localhost / 127.0.0.1
	if domain == "localhost" || domain == "127.0.0.1" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	// Cek apakah domain cocok dengan konfigurasi domain yang tersimpan di database
	ctx := r.Context()
	cfg, err := h.getDomainConfigFromDB(ctx)
	if err == nil && cfg != nil && cfg.Domain != "" {
		if strings.EqualFold(domain, cfg.Domain) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
	}

	// Tolak domain yang tidak diizinkan untuk mencegah eksploitasi penerbitan sertifikat SSL
	http.Error(w, "domain not authorized", http.StatusForbidden)
}

// resolveGatewayURL mengembalikan URL kanonikal /v1 dengan preferensi domain kustom jika ada.
func (h *Handlers) resolveGatewayURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	reqGWURL := fmt.Sprintf("%s://%s/v1", scheme, r.Host)

	// Jika domain publik terdaftar di database dan request belum memakai HTTPS
	if scheme == "http" && h.settingsRepo != nil {
		if s, err := h.settingsRepo.Get(r.Context(), SettingKeyPublicURL); err == nil && len(s.Value) > 0 {
			var pubURL string
			if err := json.Unmarshal(s.Value, &pubURL); err == nil && pubURL != "" {
				return strings.TrimRight(pubURL, "/") + "/v1"
			}
		}
	}
	return reqGWURL
}
