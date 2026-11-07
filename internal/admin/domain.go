package admin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
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
//
// AUDIT FIX: mutex tidak lagi ditahan selama probe HTTP keluar. Sebelumnya seluruh
// caller seri di belakang round-trip jaringan 2 detik (blackhole ipify = antrean
// handler domain terblokir). Kini: cek cache cepat di bawah lock, probe jaringan
// tanpa lock, lalu tulis kembali hasilnya di bawah lock (single-flight de facto:
// caller bersamaan memprobe paralel tapi tidak saling menunggu antrean).
func detectServerIP() string {
	serverIPMu.Lock()
	if serverIPCache != "" && time.Since(serverIPCacheTime) < 15*time.Minute {
		cached := serverIPCache
		serverIPMu.Unlock()
		return cached
	}
	serverIPMu.Unlock()

	ip := probeServerIP()

	serverIPMu.Lock()
	if ip != "" {
		serverIPCache = ip
		serverIPCacheTime = time.Now()
	} else if serverIPCache != "" {
		// Probe gagal: pertahankan nilai cache lama daripada mengosongkannya.
		ip = serverIPCache
	}
	serverIPMu.Unlock()
	if ip != "" {
		return ip
	}
	return "127.0.0.1"
}

// probeServerIP menjalankan deteksi IP: ipify dulu, lalu fallback interface host.
func probeServerIP() string {
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
		// Respons ipify hanyalah teks IP (audit F3): batasi pembacaan —
		// sebelumnya io.ReadAll tanpa batas atas respons eksternal.
		body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
		if err == nil {
			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
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
					return ipnet.IP.String()
				}
			}
		}
	}

	return ""
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

// domainDariPublicURL mengekstrak nama domain dari PUBLIC_URL konfigurasi
// env (h.publicURL), atau "" bila tidak ada / hostnya bukan domain.
//
// PUBLIC_URL adalah sumber kebenaran yang ditulis installer native dari
// variabel DOMAIN (routex.env + Caddyfile melayani host itu), jadi domain
// produksi bisa sepenuhnya bekerja TANPA pernah disimpan lewat panel —
// status domain wajib mencerminkannya (B-116 butir 8).
func (h *Handlers) domainDariPublicURL() string {
	if h.publicURL == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(h.publicURL))
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil || !domainRegex.MatchString(host) {
		return ""
	}
	return host
}

// getDomainStatus mengembalikan informasi status domain dan konfigurasi HTTPS saat ini.
func (h *Handlers) getDomainStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverIP := detectServerIP()

	cfg, err := h.getDomainConfigFromDB(ctx)
	if err != nil || cfg == nil || cfg.Domain == "" {
		// Panel belum menyimpan konfigurasi domain, tetapi domain produksi
		// bisa datang dari installer (PUBLIC_URL env ← DOMAIN, dilayani
		// Caddyfile). Melaporkannya "unconfigured" menyesatkan operator
		// instalasi standar (B-116 butir 8): laporkan domain itu sebagai
		// aktif dari sumber installer, dengan pesan yang menyebut sumber
		// DAN batasnya (panel belum memverifikasi DNS untuk domain ini).
		if dom := h.domainDariPublicURL(); dom != "" {
			res := DomainConfigDTO{
				Domain:     dom,
				Mode:       "installer",
				Status:     "active",
				PublicURL:  fmt.Sprintf("https://%s", dom),
				BaseURL:    fmt.Sprintf("https://%s/v1", dom),
				ServerIP:   serverIP,
				DNSMatched: false,
				Message:    "Domain berasal dari konfigurasi installer (PUBLIC_URL/DOMAIN, dilayani Caddy) dan sedang melayani produksi. Panel belum menyimpan konfigurasi domain sendiri, jadi DNS untuk domain ini belum diverifikasi panel — simpan lewat formulir ini bila ingin panel mengelolanya.",
			}
			_ = h.respond(w, r, http.StatusOK, res)
			return
		}
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

	// KEPUTUSAN TERCATAT (B-418): mode yang absen atau tidak
	// dikenal jatuh ke "letsencrypt", menimpa mode tersimpan
	// (mis. "cloudflare"). Ini diterima dengan sadar karena
	// endpoint ini adalah POST konfigurasi PENUH, bukan update
	// parsial: dashboard selalu mengirim mode, dan bentuk
	// permintaannya adalah "jadikan ini konfigurasi domain",
	// bukan "ubah field yang disebut". Membedakan mode absen di
	// sini menuntut DTO penunjuk dan kontrak parsial yang tidak
	// dimiliki permukaan konfigurasi-penuh ini di field mana pun.
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
		// B-500: dua Put ini adalah SATU keadaan, sama seperti
		// pasangan hapusnya yang ditransaksikan B-472 — versi lama
		// menulisnya berurutan tanpa transaksi, jadi kegagalan pada
		// Put kedua meninggalkan konfigurasi domain BARU ter-commit
		// tanpa public_url yang menyertainya, sementara operator
		// menerima 500: keadaan setengah yang persis ditutup B-472
		// di jalur hapus, bertahan di jalur update. Kini keduanya
		// dalam SATU transaksi: gagal di mana pun → rollback,
		// keadaan lama utuh, error jujur.
		// Simpan URL publik kanonikal. Error-nya diteruskan (audit F5):
		// versi lama menelannya sehingga domain bisa aktif sementara
		// public_url tetap yang lama — padahal nilai itu dipakai
		// resolveGatewayURL dan pemeriksaan origin CSRF.
		pubURLBytes, _ := json.Marshal("https://" + domain)
		err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
			txSettings := identity.NewSettings(q)
			if _, err := txSettings.Put(ctx, identity.SettingWrite{
				Key:         SettingKeyDomainConfig,
				Value:       valBytes,
				Description: "Konfigurasi domain kustom dan HTTPS otomatis Route-X",
				UpdatedBy:   actorID,
			}); err != nil {
				return err
			}
			if _, err := txSettings.Put(ctx, identity.SettingWrite{
				Key:         SettingKeyPublicURL,
				Value:       pubURLBytes,
				Description: "URL publik kanonikal Route-X",
				UpdatedBy:   actorID,
			}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			mapRepoError(w, r, err, "konfigurasi domain")
			return
		}
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
		// B-472: dua baris setelan ini adalah SATU keadaan — dulu
		// dihapus berurutan tanpa transaksi, jadi keadaan warisan
		// parsial (konfigurasi ada, url publik tidak) menghasilkan
		// 404 "url publik tidak ditemukan" PADAHAL baris
		// konfigurasinya sudah terhapus: jawaban gagal untuk
		// penghapusan yang setengah berhasil. Kini keberadaan
		// diperiksa dan keduanya dihapus di dalam SATU transaksi:
		// konfigurasi tidak ada → 404 tanpa satu pun baris
		// tersentuh; konfigurasi ada → keduanya hilang (url publik
		// yang memang tidak ada dilewati, bukan dijadikan kegagalan);
		// kegagalan nyata di tengah → rollback, keadaan utuh,
		// error jujur.
		err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
			txSettings := identity.NewSettings(q)
			if _, err := txSettings.Get(ctx, SettingKeyDomainConfig); err != nil {
				return err
			}
			if err := txSettings.Delete(ctx, SettingKeyDomainConfig); err != nil {
				return err
			}
			if _, err := txSettings.Get(ctx, SettingKeyPublicURL); err == nil {
				if err := txSettings.Delete(ctx, SettingKeyPublicURL); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			// Error penghapusan diteruskan (audit F5): operator tidak boleh
			// menerima "ok" untuk pencabutan domain yang sebenarnya gagal.
			mapRepoError(w, r, err, "konfigurasi domain")
			return
		}
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

// resolveGatewayURL mengembalikan URL kanonikal /v1 yang ditulis ke berkas
// konfigurasi perkakas CLI di host (ANTHROPIC_BASE_URL, ~/.routex/cli-env.sh).
//
// Sumbernya berjenjang menurut kepercayaan, dari yang paling tepercaya:
//
//  1. Setting domain kustom di database (SettingKeyPublicURL) — ditulis
//     operator lewat alur konfigurasi domain. Selalu dipakai.
//  2. PUBLIC_URL dari env (h.publicURL) — konfigurasi operator saat deploy.
//  3. Turunan dari request, dengan gerbang ketat: skema https dari
//     X-Forwarded-Proto dan host dari X-Forwarded-Host HANYA dipercaya bila
//     koneksi datang dari proxy terdaftar (TRUSTED_PROXIES); selain itu
//     skema dari r.TLS dan host dari r.Host apa adanya.
//
// Versi lama membaca Host dan X-Forwarded-Proto mentah tanpa gerbang apa pun
// (audit3 V-AU-19, terbukti live): satu request admin membawa
// "Host: evil.example" + "X-Forwarded-Proto: https" membuat URL yang
// teracuni itu ditulis ke berkas konfigurasi CLI di host, dan setting DB
// hanya menolong untuk skema http sehingga tidak menutupinya. Nilai dari
// konfigurasi operator sekarang selalu menang atas turunan request.
func (h *Handlers) resolveGatewayURL(r *http.Request) string {
	// 1. Setting domain kustom operator di database.
	if h.settingsRepo != nil {
		if s, err := h.settingsRepo.Get(r.Context(), SettingKeyPublicURL); err == nil && len(s.Value) > 0 {
			var pubURL string
			if err := json.Unmarshal(s.Value, &pubURL); err == nil && pubURL != "" {
				return strings.TrimRight(pubURL, "/") + "/v1"
			}
		}
	}

	// 2. PUBLIC_URL dari konfigurasi env.
	if h.publicURL != "" {
		return strings.TrimRight(h.publicURL, "/") + "/v1"
	}

	// 3. Turunan request dengan gerbang proxy tepercaya.
	trusted := httpx.IsTrustedProxy(r, h.trustedProxies)
	scheme := "http"
	if r.TLS != nil || (trusted && r.Header.Get("X-Forwarded-Proto") == "https") {
		scheme = "https"
	}
	host := r.Host
	if trusted {
		if fh := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); fh != "" {
			host = fh
		}
	}
	return fmt.Sprintf("%s://%s/v1", scheme, host)
}
