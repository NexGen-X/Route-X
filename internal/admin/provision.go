package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/provision"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/worker"
)

type provisionCloudflareReq struct {
	AccountID   string  `json:"account_id"`
	APIToken    string  `json:"api_token"`
	GatewayID   string  `json:"gateway_id"`
	Name        string  `json:"name"`
	CollectLogs bool    `json:"collect_logs"`
	EnableCache bool    `json:"enable_cache"`
	Region      *string `json:"region"`
}

type provisionDenoReq struct {
	AccessToken string  `json:"access_token"`
	ProjectName string  `json:"project_name"`
	Name        string  `json:"name"`
	Region      *string `json:"region"`
}

type provisionResultDTO struct {
	Pool         EgressPoolDTO `json:"pool"`
	ProviderType string        `json:"provider_type"`
	TargetURL    string        `json:"target_url"`
	AlreadyExist bool          `json:"already_exist,omitempty"`
}

// Jahitan test (B-364): konstruktor klien provisioning sebagai
// variabel paket supaya test dapat mengarahkan panggilan pihak
// ketiga ke server tiruan; produksi selalu memakai konstruktor
// asli dari paket provision.
var (
	buatKlienCloudflare = provision.NewCloudflareClient
	buatKlienDeno       = provision.NewDenoClient
)

// hostDariMaskedHint mengekstrak host dari masked hint URL pool yang
// tersimpan (mis. "http://****@proyek.deno.dev:3128" menjadi
// "proyek.deno.dev"): skema dibuang, userinfo tersamar dibuang sampai
// "@" terakhir, lalu sisa dipotong pada pemisah port/path pertama.
// Dipakai mencocokkan pool yang sudah ada dengan proyek Deno yang akan
// di-provisioning (B-203) tanpa pernah membuka URL aslinya.
func hostDariMaskedHint(hint string) string {
	s := hint
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "/:"); i >= 0 {
		s = s[:i]
	}
	return s
}

func (provisionResultDTO) adalahDTO() {}

func (h *Handlers) provisionCloudflareAIGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req provisionCloudflareReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	accountID := strings.TrimSpace(req.AccountID)
	if accountID == "" {
		httpx.BadRequest(w, r, "missing_account_id", "cloudflare account_id wajib diisi")
		return
	}
	apiToken := strings.TrimSpace(req.APIToken)
	if apiToken == "" {
		httpx.BadRequest(w, r, "missing_api_token", "cloudflare api_token wajib diisi")
		return
	}

	gatewayID := strings.TrimSpace(req.GatewayID)
	if gatewayID == "" {
		gatewayID = "routex-ai-gateway"
	}

	client := buatKlienCloudflare("", 20*time.Second)
	res, err := client.ProvisionAIGateway(ctx, provision.CloudflareProvisionParams{
		AccountID:   accountID,
		APIToken:    apiToken,
		GatewayID:   gatewayID,
		CollectLogs: req.CollectLogs,
		EnableCache: req.EnableCache,
	})
	if err != nil {
		// Kegagalan API pihak ketiga: 502 + pesan umum, teks asli ke log
		// (audit3 V-EM-03).
		h.gagalHulu(w, r, "provisioning cloudflare gagal", "cloudflare_provision_failed",
			"Gagal mendaftarkan AI Gateway di Cloudflare; periksa kredensial dan koneksi lalu coba lagi", err)
		return
	}

	poolName := strings.TrimSpace(req.Name)
	if poolName == "" {
		poolName = fmt.Sprintf("☁️ Cloudflare AI Gateway (%s)", res.GatewayID)
	}

	region := "cloudflare-edge"
	if req.Region != nil && strings.TrimSpace(*req.Region) != "" {
		region = strings.TrimSpace(*req.Region)
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	enabled := true
	weight := 1
	ep, err := h.egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name:      poolName,
		Kind:      "https",
		ProxyURL:  security.Secret(res.GatewayURL),
		Enabled:   &enabled,
		Weight:    &weight,
		Region:    &region,
		CreatedBy: actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	h.writeAudit(ctx, r, "provision", "egress_pool_cloudflare", ep.ID, map[string]any{
		"name":        ep.Name,
		"account_id":  res.AccountID,
		"gateway_id":  res.GatewayID,
		"gateway_url": res.GatewayURL,
	})

	// B-364: pool baru yang langsung aktif mengubah himpunan
	// kandidat egress — siram cache respons seperti createEgressPool
	// biasa (B-339 menutup permukaan non-provisioning-nya; dua jalur
	// provisioning ini terlewat dari sapuan itu).
	h.kosongkanCacheRespons(ctx)

	_ = h.respond(w, r, http.StatusCreated, provisionResultDTO{
		Pool:         toEgressPoolDTO(ep),
		ProviderType: "cloudflare_ai_gateway",
		TargetURL:    res.GatewayURL,
		AlreadyExist: res.AlreadyExist,
	})
}

func (h *Handlers) provisionDenoRelay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req provisionDenoReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" {
		httpx.BadRequest(w, r, "missing_access_token", "deno access_token wajib diisi")
		return
	}

	projectName := strings.TrimSpace(req.ProjectName)
	if projectName == "" {
		// Deterministik per pemasangan (B-203): nama acak berbasis waktu
		// membuat retry mencetak proyek yatim yang tidak pernah ketemu
		// lagi, dan tidak ada jalan mendeteksi "proyek ini sudah
		// di-provisioning" — pendeteksian di bawah bergantung padanya.
		projectName = provision.NamaProyekRelayBawaan
	}

	// Satu provisioning Deno pada satu waktu: dua permintaan bersamaan
	// bisa sama-sama lolos pemeriksaan "sudah ada" di bawah lalu
	// balapan men-deploy dan membuat pool.
	unlock, terkunci, err := worker.TryAdvisoryLock(ctx, h.pool, worker.LockProvisionDeno)
	if err != nil {
		h.gagalInternal(w, r, "gagal mengambil kunci provisioning deno", err)
		return
	}
	if !terkunci {
		httpx.WriteError(w, r, http.StatusConflict, httpx.ErrTypeAPI, "provision_in_progress",
			"Provisioning Deno lain sedang berjalan; coba lagi sebentar")
		return
	}
	defer unlock()

	poolName := strings.TrimSpace(req.Name)
	if poolName == "" {
		poolName = fmt.Sprintf("🦕 Deno Relay (%s)", projectName)
	}

	// Tolak LEBIH DULU bila proyek ini sudah punya pool (B-203):
	// ProvisionRelay selalu mencetak token relay baru dan men-deploy-nya,
	// jadi provisioning ulang atas proyek yang sama diam-diam memutus
	// pool lama (token tersimpannya tidak berlaku lagi), dan pool kedua
	// bahkan tidak bisa dibuat karena nama unik — kegagalannya terjadi
	// SESUDAH efek samping deploy. Pemeriksaan di sini berjalan sebelum
	// panggilan Deno apa pun. Batas jujur: pool dengan domain kustom di
	// luar *.deno.dev tidak tertangkap pemeriksaan host; pemeriksaan
	// nama pool menutup jalur retry nama-bawaan yang umum.
	pools, err := h.egressRepo.List(ctx, false)
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}
	hostProyek := projectName + ".deno.dev"
	for _, p := range pools {
		if p.Name == poolName || hostDariMaskedHint(p.MaskedHint) == hostProyek {
			httpx.WriteError(w, r, http.StatusConflict, httpx.ErrTypeAPI, "deno_project_already_provisioned",
				fmt.Sprintf("proyek Deno %q sudah punya egress pool %q; hapus pool itu dulu bila ingin provisioning ulang — provisioning ulang merotasi token relay dan akan memutus pool yang ada", projectName, p.Name))
			return
		}
	}

	client := buatKlienDeno("", 30*time.Second)
	res, err := client.ProvisionRelay(ctx, provision.DenoProvisionParams{
		AccessToken: accessToken,
		ProjectName: projectName,
	})
	if err != nil {
		h.gagalHulu(w, r, "provisioning deno gagal", "deno_provision_failed",
			"Gagal mendeploy script relay ke Deno; periksa kredensial dan koneksi lalu coba lagi", err)
		return
	}

	region := "deno-edge"
	if req.Region != nil && strings.TrimSpace(*req.Region) != "" {
		region = strings.TrimSpace(*req.Region)
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	// Token relay ditanam sebagai password userinfo pada URL pool yang
	// tersimpan: factory membacanya dari sana dan mengirimnya sebagai header
	// x-relay-token (lihat gateway.buatTarget). Menyimpannya di kolom
	// terenkripsi yang sama dengan URL berarti token tidak butuh skema atau
	// kolom baru, dan masked hint pool menyamarkannya seperti kredensial
	// proxy biasa.
	poolURL := res.RelayURL
	if res.RelayToken != "" {
		if u, perr := url.Parse(res.RelayURL); perr == nil && u != nil {
			u.User = url.UserPassword("relay", res.RelayToken)
			poolURL = u.String()
		}
	}

	enabled := true
	weight := 1
	ep, err := h.egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name:      poolName,
		Kind:      "https",
		ProxyURL:  security.Secret(poolURL),
		Enabled:   &enabled,
		Weight:    &weight,
		Region:    &region,
		CreatedBy: actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	h.writeAudit(ctx, r, "provision", "egress_pool_deno", ep.ID, map[string]any{
		"name":         ep.Name,
		"project_name": res.ProjectName,
		"relay_url":    res.RelayURL,
	})

	// B-364: sama seperti jalur Cloudflare di atas — pool baru yang
	// langsung aktif menyiram cache respons.
	h.kosongkanCacheRespons(ctx)

	_ = h.respond(w, r, http.StatusCreated, provisionResultDTO{
		Pool:         toEgressPoolDTO(ep),
		ProviderType: "deno_deploy_relay",
		TargetURL:    res.RelayURL,
	})
}
