package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
)

func (h *Handlers) accessRoutes(r chi.Router) {
	// 1. Client API Keys
	r.Route("/api-keys", func(kr chi.Router) {
		kr.With(auth.RequirePermission(seed.PermAPIKeysRead)).Get("/", h.listAPIKeys)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/", h.createAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysRead)).Get("/{id}", h.getAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Put("/{id}", h.updateAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Delete("/{id}", h.deleteAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/{id}/rotate", h.rotateAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/{id}/toggle", h.toggleAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysRead)).Get("/{id}/allowed", h.getAPIKeyAllowed)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/{id}/allowed", h.setAllowedAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Put("/{id}/allowed", h.setAllowedAPIKey)
	})

	// 2. Admin Users
	r.Route("/users", func(ur chi.Router) {
		ur.With(auth.RequirePermission(seed.PermUsersRead)).Get("/", h.listUsers)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/", h.createUser)
		ur.With(auth.RequirePermission(seed.PermUsersRead)).Get("/{id}", h.getUser)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Put("/{id}", h.updateUser)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Delete("/{id}", h.deleteUser)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/{id}/force-reset-password", h.forceResetPassword)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/{id}/revoke-sessions", h.revokeUserSessions)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/{id}/unlock", h.unlockUser)
	})

	// 3. Sessions
	r.Route("/sessions", func(sr chi.Router) {
		sr.With(auth.RequirePermission(seed.PermUsersRead)).Get("/", h.listSessions)
		sr.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/revoke-all", h.revokeAllSessions)
		sr.With(auth.RequirePermission(seed.PermUsersWrite)).Delete("/{id}", h.revokeSession)
	})
}

// errAdminAktifTerakhir menandai penolakan mutasi yang akan menghilangkan
// admin aktif terakhir. Dipisah sebagai sentinel supaya handler bisa
// membedakannya dari kegagalan database dan menjawab 400 berkode tetap.
var errAdminAktifTerakhir = errors.New("pengguna adalah admin aktif terakhir")

// kunciPenjagaAdmin adalah kunci advisory Postgres (di luar rentang kunci
// aplikasi lain; "RTXADMIN" dalam ASCII) yang menserialkan SEMUA mutasi yang
// menjaga invariansi "selalu ada minimal satu admin aktif".
//
// Invariansi itu sebelumnya ditegakkan pola periksa-lalu-tulis tanpa kunci:
// baca daftar admin aktif, hitung, lalu ubah status/hapus. Dua penonaktifan
// pengguna BERBEDA yang berjalan bersamaan masing-masing melihat hitungan 2
// dan sama-sama lolos — hasilnya nol admin aktif dan lockout total (audit3
// V-AU-10). Dengan pg_advisory_xact_lock di awal transaksi, pemeriksaan dan
// mutasinya menjadi satu rangkaian yang tidak bisa disela mutasi penjaga lain;
// kunci terlepas otomatis saat transaksi selesai.
const kunciPenjagaAdmin int64 = 0x52545841444d494e

// kunciPenjagaAdminTx mengambil kunci advisory penjaga admin di dalam
// transaksi q. Wajib dipanggil sebelum pemeriksaan jumlah admin aktif.
func kunciPenjagaAdminTx(ctx context.Context, q repo.Querier) error {
	_, err := q.Exec(ctx, `select pg_advisory_xact_lock($1)`, kunciPenjagaAdmin)
	return err
}

// -----------------------------------------------------------------------------
// API Keys Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := repo.DefaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	filter := keys.ListFilter{
		OwnerUserID: r.URL.Query().Get("user_id"),
		Status:      r.URL.Query().Get("status"),
		Search:      r.URL.Query().Get("search"),
	}

	page := repo.Page{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.keyRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	keyIDs := make([]string, 0, len(items))
	for _, k := range items {
		keyIDs = append(keyIDs, k.ID)
	}
	modelMap, providerMap, _ := h.keyRepo.GetAllowedMap(ctx, keyIDs)

	keysDTO := make([]KeyDTO, 0, len(items))
	for _, k := range items {
		var mIDs, pIDs []string
		if modelMap != nil {
			mIDs = modelMap[k.ID]
		}
		if providerMap != nil {
			pIDs = providerMap[k.ID]
		}
		keysDTO = append(keysDTO, toKeyDTO(k, mIDs, pIDs))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[KeyDTO]{
		Items:      keysDTO,
		NextCursor: next,
	})
}

func (h *Handlers) getAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	k, err := h.keyRepo.GetByID(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	_ = h.respond(w, r, http.StatusOK, h.keyDetailResponse(ctx, k))
}

// keyDetailResponse membangun respons detail kunci dari keadaan TERSIMPAN
// penuh — baris kunci PLUS daftar putih model/provider yang benar-benar
// melekat padanya di database.
//
// Jalur ini satu-satunya yang boleh dipakai respons baca maupun respons
// MUTASI (B-112): update, toggle, dan rotate sebelumnya membangun DTO
// dengan cakupan nil sehingga allowed_models/allowed_providers tampil []
// padahal cakupan di database utuh — klien yang memercayai respons mutasi
// bisa menyimpulkan cakupan terhapus lalu "memperbaikinya" dengan menimpa
// cakupan yang benar. Kegagalan membaca cakupan ditelan seperti jalur baca
// lama (DTO tetap terkirim); cakupan yang terbaca selalu yang tersimpan.
func (h *Handlers) keyDetailResponse(ctx context.Context, k *keys.Key) KeyDetailResponse {
	mIDs, pIDs, _ := h.keyRepo.GetAllowed(ctx, k.ID)
	return KeyDetailResponse{
		Key:    toKeyDTO(k, mIDs, pIDs),
		Masked: k.Masked(),
	}
}

type createAPIKeyReq struct {
	Name                string   `json:"name"`
	Label               string   `json:"label"`
	OwnerUserID         string   `json:"owner_user_id"`
	Live                bool     `json:"live"`
	Scopes              []string `json:"scopes"`
	RateLimitRPS        *int     `json:"rate_limit_rps"`
	RateLimitRPM        *int     `json:"rate_limit_rpm"`
	RateLimitTPM        *int     `json:"rate_limit_tpm"`
	RPMLimit            *int     `json:"rpm_limit"`
	TPMLimit            *int     `json:"tpm_limit"`
	DailyRequestLimit   *int64   `json:"daily_request_limit"`
	MonthlyRequestLimit *int64   `json:"monthly_request_limit"`
	DailyTokenLimit     *int64   `json:"daily_token_limit"`
	MonthlyTokenLimit   *int64   `json:"monthly_token_limit"`
	IPAllowlist         []string `json:"ip_allowlist"`
	ExpiresAt           *string  `json:"expires_at"`
	ModelIDs            []string `json:"model_ids"`
	ProviderIDs         []string `json:"provider_ids"`
	AllowedModels       []string `json:"allowed_models"`
	AllowedProviders    []string `json:"allowed_providers"`
}

// uraikanAllowlistIP mengurai entri ip_allowlist mentah menjadi
// prefiks kanonis yang disimpan. SATU pengurai untuk jalur buat dan
// ubah (B-355(ii)): kedua blok pengurai dulu salinan persis satu
// sama lain, sehingga perbaikan perilaku harus dilakukan dua kali
// dan bisa menyimpang — B-342 membuktikannya, normalisasi 4-in-6
// tidak boleh hanya hidup di satu jalur.
//
// Perilaku per entri:
//   - entri kosong (sesudah trim) dilewati;
//   - prefiks CIDR: bentuk 4-in-6 (::ffff:a.b.c.d/n) dinormalkan ke
//     IPv4 polos lebih dulu (B-342: penegakan membandingkan alamat
//     klien yang sudah di-Unmap, jadi bentuk 4-in-6 tersimpan tidak
//     akan pernah cocok); prefiks 4-in-6 berbit < 96 mencakup ruang
//     di luar peta IPv4-dalam-IPv6 dan ditolak; awalan berbit host
//     ditolak dengan menyebut entrinya (B-307);
//   - alamat tunggal: di-Unmap lalu menjadi prefiks host-nya.
//
// Semantik nil dipertahankan pemanggil: fungsi ini mengembalikan
// nil bila tak satu pun entri menghasilkan prefiks; jalur ubah
// membungkusnya ke slice kosong non-nil sendiri (buka-dari-mana-
// saja), jalur buat membiarkan nil (biarkan).
func uraikanAllowlistIP(entri []string) ([]netip.Prefix, error) {
	var ips []netip.Prefix
	for _, s := range entri {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			if p.Addr().Is4In6() {
				if p.Bits() < 96 {
					return nil, fmt.Errorf("entri ip_allowlist %q mencakup ruang di luar peta IPv4-dalam-IPv6 dan tidak punya bentuk IPv4 yang setara", s)
				}
				p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
			}
			// B-307: awalan berbit host (mis. 10.0.0.1/24) ditolak
			// di sini dengan menyebut entrinya — sebelumnya ia
			// lolos validasi handler lalu ditolak kolom cidr[] DB
			// sebagai 400 invalid_reference generik.
			if p != p.Masked() {
				return nil, fmt.Errorf("entri ip_allowlist %q memiliki bit host; gunakan bentuk jaringannya %q", s, p.Masked().String())
			}
			ips = append(ips, p)
		} else if addr, err := netip.ParseAddr(s); err == nil {
			addr = addr.Unmap()
			ips = append(ips, netip.PrefixFrom(addr, addr.BitLen()))
		} else {
			return nil, fmt.Errorf("entri ip_allowlist tidak sah: %q", s)
		}
	}
	return ips, nil
}

func (h *Handlers) createAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createAPIKeyReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// Normalisasi alias field
	if req.Name == "" && req.Label != "" {
		req.Name = req.Label
	}
	if req.RateLimitRPM == nil && req.RPMLimit != nil {
		req.RateLimitRPM = req.RPMLimit
	}
	if req.RateLimitTPM == nil && req.TPMLimit != nil {
		req.RateLimitTPM = req.TPMLimit
	}
	if len(req.ModelIDs) == 0 && len(req.AllowedModels) > 0 {
		req.ModelIDs = req.AllowedModels
	}
	if len(req.ProviderIDs) == 0 && len(req.AllowedProviders) > 0 {
		req.ProviderIDs = req.AllowedProviders
	}

	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_expires_at", fmt.Sprintf("format expires_at tidak sah (harus RFC3339): %v", err))
			return
		}
		exp = &t
	}

	var ips []netip.Prefix
	if len(req.IPAllowlist) > 0 {
		diurai, err := uraikanAllowlistIP(req.IPAllowlist)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_ip_allowlist", err.Error())
			return
		}
		ips = diurai
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	ownerID := req.OwnerUserID
	if ownerID == "" && actorID != nil {
		ownerID = *actorID
	}
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{"inference"}
	}

	// Validasi bentuk disamakan dengan updateAPIKey (B-200): jalur buat
	// sebelumnya menerima nama kosong (sesudah normalisasi alias), scope
	// karangan, batas <= 0, expires_at lampau, dan pemilik yang tidak
	// ada/nonaktif — semua baru meledak di constraint DB sebagai 500
	// atau tertolak di jalur ubah. Di sini semuanya 400 yang menyebut
	// field-nya, sebelum transaksi apa pun dibuka.
	if strings.TrimSpace(req.Name) == "" {
		httpx.BadRequest(w, r, "invalid_name", "name wajib diisi")
		return
	}
	for _, sc := range scopes {
		if !keys.ValidScope(sc) {
			httpx.BadRequest(w, r, "invalid_scopes",
				fmt.Sprintf("scope %q tidak dikenal; scope yang sah: %s", sc, strings.Join(keys.KnownScopes, ", ")))
			return
		}
	}
	for _, batas := range []struct {
		nama  string
		ada   bool
		nilai int64
	}{
		{"rate_limit_rps", req.RateLimitRPS != nil, int64(nilaiAtau(req.RateLimitRPS, 0))},
		{"rate_limit_rpm", req.RateLimitRPM != nil, int64(nilaiAtau(req.RateLimitRPM, 0))},
		{"rate_limit_tpm", req.RateLimitTPM != nil, int64(nilaiAtau(req.RateLimitTPM, 0))},
		{"daily_request_limit", req.DailyRequestLimit != nil, nilaiAtau(req.DailyRequestLimit, 0)},
		{"monthly_request_limit", req.MonthlyRequestLimit != nil, nilaiAtau(req.MonthlyRequestLimit, 0)},
		{"daily_token_limit", req.DailyTokenLimit != nil, nilaiAtau(req.DailyTokenLimit, 0)},
		{"monthly_token_limit", req.MonthlyTokenLimit != nil, nilaiAtau(req.MonthlyTokenLimit, 0)},
	} {
		if batas.ada && batas.nilai <= 0 {
			httpx.BadRequest(w, r, "invalid_limit", "batas "+batas.nama+" harus bilangan positif")
			return
		}
	}
	if exp != nil && exp.Before(time.Now()) {
		httpx.BadRequest(w, r, "expires_in_past", "expires_at tidak boleh di masa lalu")
		return
	}
	if req.OwnerUserID != "" {
		pemilik, err := h.usersRepo.GetByID(ctx, ownerID)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				httpx.BadRequest(w, r, "invalid_owner_user_id", "owner_user_id tidak ditemukan")
				return
			}
			mapRepoError(w, r, err, "pengguna pemilik")
			return
		}
		if pemilik.Status != identity.UserStatusActive {
			httpx.BadRequest(w, r, "invalid_owner_user_id", "owner_user_id menunjuk pengguna nonaktif")
			return
		}
	}
	// Daftar putih model/provider dipecahkan di sini (B-201) supaya slug
	// yang tidak dikenal menjadi 400 yang menyebut nilainya, bukan 500
	// dari ledakan cast uuid[] di dalam transaksi repo.
	if err := h.validasiDaftarPutihAPIKey(ctx, req.ModelIDs, req.ProviderIDs); err != nil {
		tulisErrDaftarPutih(w, r, err)
		return
	}

	// Bungkus pembuatan key beserta seluruh pembatasannya dalam SATU transaksi atomik.
	// Jika pembatasan model/provider gagal, key tidak boleh tercipta bebas ke publik.
	var created *keys.Created
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txKeyRepo := h.keyRepo.WithQuerier(q)
		var err error
		created, err = txKeyRepo.Create(ctx, keys.CreateParams{
			Name:                req.Name,
			OwnerUserID:         ownerID,
			Live:                req.Live,
			Scopes:              scopes,
			RateLimitRPS:        req.RateLimitRPS,
			RateLimitRPM:        req.RateLimitRPM,
			RateLimitTPM:        req.RateLimitTPM,
			DailyRequestLimit:   req.DailyRequestLimit,
			MonthlyRequestLimit: req.MonthlyRequestLimit,
			DailyTokenLimit:     req.DailyTokenLimit,
			MonthlyTokenLimit:   req.MonthlyTokenLimit,
			IPAllowlist:         ips,
			ExpiresAt:           exp,
			CreatedBy:           actorID,
			AllowedModelIDs:     req.ModelIDs,
			AllowedProviderIDs:  req.ProviderIDs,
		})
		return err
	})
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	h.writeAudit(ctx, r, "create", "api_key", created.Key.ID, map[string]any{"name": created.Key.Name, "prefix": created.Key.Prefix})
	// Produser event api_key.created (audit3 V-WH-08). Payload hanya memuat
	// pengenal publik — nilai key mentah tidak pernah masuk antrean webhook.
	h.pancarkanEvent(ctx, "api_key.created", map[string]any{
		"api_key_id":    created.Key.ID,
		"name":          created.Key.Name,
		"prefix":        created.Key.Prefix,
		"owner_user_id": ownerID,
	})
	_ = h.respond(w, r, http.StatusCreated, KeyCreatedResponse{
		Key:    toKeyDTO(created.Key, req.ModelIDs, req.ProviderIDs),
		Masked: created.Key.Masked(),
		RawKey: created.Raw.Reveal(),
	})
}

// errDaftarPutih adalah penolakan validasi daftar putih API key yang
// membawa kode respons dan nilai entri yang menyinggung, supaya handler
// menjawab 400 yang menyebut nilainya (pola B-196/B-179).
type errDaftarPutih struct {
	kode  string
	nilai string
}

func (e *errDaftarPutih) Error() string { return e.kode + ": " + e.nilai }

// tulisErrDaftarPutih menerjemahkan error validasi daftar putih menjadi
// respons handler: errDaftarPutih menjadi 400 berkode, sisanya error
// repo biasa.
func tulisErrDaftarPutih(w http.ResponseWriter, r *http.Request, err error) {
	var edp *errDaftarPutih
	if errors.As(err, &edp) {
		bidang := "model_ids"
		if edp.kode == "invalid_provider_ids" {
			bidang = "provider_ids"
		}
		httpx.BadRequest(w, r, edp.kode,
			fmt.Sprintf("%s memuat entri yang tidak dikenal: %s", bidang, edp.nilai))
		return
	}
	mapRepoError(w, r, err, "api key")
}

// validasiDaftarPutihAPIKey memastikan setiap entri daftar putih
// model/provider yang BUKAN UUID dapat dipecahkan menjadi baris katalog
// nyata (slug model lewat GetByModelID, nama provider lewat GetByName —
// persis semantik resolusi repo) sebelum daftar itu mencapai repo
// (B-201). Entri berbentuk UUID diserahkan ke pemeriksaan repo, yang
// menolak referensi yatim sebagai ErrInvalidReference.
func (h *Handlers) validasiDaftarPutihAPIKey(ctx context.Context, modelIDs, providerIDs []string) error {
	for _, mentah := range modelIDs {
		id := strings.TrimSpace(mentah)
		if id == "" || bentukUUID(id) {
			continue
		}
		if _, err := h.modelRepo.GetByModelID(ctx, id); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return &errDaftarPutih{kode: "invalid_model_ids", nilai: id}
			}
			return err
		}
	}
	for _, mentah := range providerIDs {
		nama := strings.TrimSpace(mentah)
		if nama == "" || bentukUUID(nama) {
			continue
		}
		if _, err := h.providerRepo.GetByName(ctx, nama); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return &errDaftarPutih{kode: "invalid_provider_ids", nilai: nama}
			}
			return err
		}
	}
	return nil
}

// nilaiAtau mengambil isi pointer atau nilai bawaan bila nil.
func nilaiAtau[T any](p *T, bawaan T) T {
	if p == nil {
		return bawaan
	}
	return *p
}

func (h *Handlers) updateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name                *string  `json:"name"`
		Label               *string  `json:"label"`
		RateLimitRPS        *int     `json:"rate_limit_rps"`
		RateLimitRPM        *int     `json:"rate_limit_rpm"`
		RateLimitTPM        *int     `json:"rate_limit_tpm"`
		RPMLimit            *int     `json:"rpm_limit"`
		TPMLimit            *int     `json:"tpm_limit"`
		DailyRequestLimit   *int64   `json:"daily_request_limit"`
		MonthlyRequestLimit *int64   `json:"monthly_request_limit"`
		DailyTokenLimit     *int64   `json:"daily_token_limit"`
		MonthlyTokenLimit   *int64   `json:"monthly_token_limit"`
		IPAllowlist         []string `json:"ip_allowlist"`
		ExpiresAt           *string  `json:"expires_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.Name == nil && req.Label != nil {
		req.Name = req.Label
	}
	if req.RateLimitRPM == nil && req.RPMLimit != nil {
		req.RateLimitRPM = req.RPMLimit
	}
	if req.RateLimitTPM == nil && req.TPMLimit != nil {
		req.RateLimitTPM = req.TPMLimit
	}

	// B-369: pembedaan "tidak disebut" vs "disebut kosong" hidup di
	// kenon-nilan slice ini, persis kontrak repo (nil = biarkan, kosong
	// non-nil = buka dari mana saja). Versi lama menginisialisasi ips
	// sebagai slice KOSONG non-nil tanpa syarat, jadi SETIAP pembaruan
	// yang tidak menyebut ip_allowlist — ganti nama saja, misalnya —
	// ikut mengosongkan allowlist yang tersimpan: kunci terbuka dari
	// IP mana pun secara senyap. Kini ips hanya non-nil bila body
	// menyebut field-nya; uraikanAllowlistIP mengembalikan nil bila
	// tak satu pun entri menghasilkan prefiks, jadi hasil urai untuk
	// body [] (atau entri yang semuanya kosong) dibungkus kembali ke
	// slice kosong non-nil di sini.
	var ips []netip.Prefix
	if req.IPAllowlist != nil {
		diurai, err := uraikanAllowlistIP(req.IPAllowlist)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_ip_allowlist", err.Error())
			return
		}
		ips = diurai
		if ips == nil {
			ips = []netip.Prefix{}
		}
	}

	// expires_at "": niat membuka (tanpa kedaluwarsa) BELUM didukung kontrak —
	// repo sudah punya keys.Clear, tetapi pointer *string tidak bisa membedakan
	// "tidak disebut" (nil) dari "kosongkan" (""). Daripada diam-diam
	// mengabaikan ("biarkan"), tolak dengan pesan yang menunjuk jalan keluar.
	if req.ExpiresAt != nil && *req.ExpiresAt == "" {
		httpx.BadRequest(w, r, "expires_clear_unsupported", "pengosongan expires_at belum didukung lewat endpoint ini; hubungi operator database")
		return
	}
	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_expires_at", fmt.Sprintf("format expires_at tidak sah (harus RFC3339): %v", err))
			return
		}
		if t.Before(time.Now()) {
			httpx.BadRequest(w, r, "expires_in_past", "expires_at tidak boleh di masa lalu")
			return
		}
		exp = &t
	}

	// Batas <= 0 DITOLAK di sini dengan 400: DB menolaknya lewat constraint
	// (hasilnya 500 yang membingungkan), dan tidak satu pun batas <= 0 yang
	// bermakna. Tanpa-batas dinyatakan dengan NULL, dan kontrak pembuka NULL
	// belum ada — lihat catatan di atas ExpiresAt.
	for _, batas := range []struct {
		nama  string
		ada   bool
		nilai int64
	}{
		{"rate_limit_rps", req.RateLimitRPS != nil, int64(nilaiAtau(req.RateLimitRPS, 0))},
		{"rate_limit_rpm", req.RateLimitRPM != nil, int64(nilaiAtau(req.RateLimitRPM, 0))},
		{"rate_limit_tpm", req.RateLimitTPM != nil, int64(nilaiAtau(req.RateLimitTPM, 0))},
		{"daily_request_limit", req.DailyRequestLimit != nil, nilaiAtau(req.DailyRequestLimit, 0)},
		{"monthly_request_limit", req.MonthlyRequestLimit != nil, nilaiAtau(req.MonthlyRequestLimit, 0)},
		{"daily_token_limit", req.DailyTokenLimit != nil, nilaiAtau(req.DailyTokenLimit, 0)},
		{"monthly_token_limit", req.MonthlyTokenLimit != nil, nilaiAtau(req.MonthlyTokenLimit, 0)},
	} {
		if batas.ada && batas.nilai <= 0 {
			httpx.BadRequest(w, r, "invalid_limit", "batas "+batas.nama+" harus bilangan positif")
			return
		}
	}
	var params keys.UpdateParams
	params.Name = req.Name
	params.IPAllowlist = ips
	if req.RateLimitRPS != nil {
		params.RateLimitRPS = keys.Set(*req.RateLimitRPS)
	}
	if req.RateLimitRPM != nil {
		params.RateLimitRPM = keys.Set(*req.RateLimitRPM)
	}
	if req.RateLimitTPM != nil {
		params.RateLimitTPM = keys.Set(*req.RateLimitTPM)
	}
	if req.DailyRequestLimit != nil {
		params.DailyRequestLimit = keys.Set(*req.DailyRequestLimit)
	}
	if req.MonthlyRequestLimit != nil {
		params.MonthlyRequestLimit = keys.Set(*req.MonthlyRequestLimit)
	}
	if req.DailyTokenLimit != nil {
		params.DailyTokenLimit = keys.Set(*req.DailyTokenLimit)
	}
	if req.MonthlyTokenLimit != nil {
		params.MonthlyTokenLimit = keys.Set(*req.MonthlyTokenLimit)
	}
	if exp != nil {
		params.ExpiresAt = keys.Set(*exp)
	}

	k, err := h.keyRepo.Update(ctx, id, params)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	h.writeAudit(ctx, r, "update", "api_key", id, nil)
	_ = h.respond(w, r, http.StatusOK, h.keyDetailResponse(ctx, k))
}

// bersihkanCounterRateLimitKey menghapus seluruh counter rate limit Redis
// milik satu API key (pola routex:rl:apikey:<id>:*).
//
// Tanpa ini, menghapus/merotasi/menonaktifkan kunci meninggalkan counter
// yatim sampai TTL jendelanya habis (audit3 V-LN-16) — paling lama jendela
// bulanan — dan kunci BARU hasil rotasi yang kebetulan mewarisi id yang sama
// di masa depan akan mewarisi pula hitungan kunci lama. Best-effort:
// kegagalan Redis hanya dicatat, tidak menggagalkan operasi kunci yang
// sudah berhasil di database.
func (h *Handlers) bersihkanCounterRateLimitKey(ctx context.Context, id string) {
	if h.redis == nil || id == "" {
		return
	}
	pola := cache.Key(cache.NamespaceRateLimit, "apikey", id) + ":*"
	var cursor uint64
	for {
		kunci, berikut, err := h.redis.Scan(ctx, cursor, pola, 100).Result()
		if err != nil {
			h.logger.WarnContext(ctx, "gagal memindai counter rate limit kunci",
				"api_key_id", id, "error", err)
			return
		}
		if len(kunci) > 0 {
			if err := h.redis.Del(ctx, kunci...).Err(); err != nil {
				h.logger.WarnContext(ctx, "gagal menghapus counter rate limit kunci",
					"api_key_id", id, "error", err)
				return
			}
		}
		cursor = berikut
		if cursor == 0 {
			return
		}
	}
}

// rotateAPIKey menangani POST /api-keys/{id}/rotate.
//
// Semantik rotasi yang BERLAKU dan disengaja (putusan B-113, dikunci test
// TestRotateAPIKeySemantikBuatBaruB113): rotasi MENERBITKAN kunci baru
// ber-ID baru yang mewarisi seluruh setelan kunci lama, lalu MENCABUT
// kunci lama (status revoked, final). ID kunci lama TIDAK berpindah ke
// kunci baru, dan riwayat request tetap terpisah per ID — itulah gunanya
// rantai rotated_from/Lineage di repo keys, pasangan event
// api_key.revoked + api_key.created, dan field new_key_id pada audit
// aksi ini. "Rotate" di sini berarti mengganti KREDENSIAL (rahasia) yang
// dipakai klien, bukan menulis ulang rahasia pada baris yang sama;
// operator yang ingin mematikan sementara tanpa mengganti kredensial
// memakai toggle, dan yang ingin membuang baris memakai DELETE.
func (h *Handlers) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	created, err := h.keyRepo.RotateWithPool(ctx, h.pool, id, actorID)
	if err != nil {
		mapRepoError(w, r, err, "rotasi api key")
		return
	}

	h.bersihkanCounterRateLimitKey(ctx, id)
	h.writeAudit(ctx, r, "rotate", "api_key", id, map[string]any{"new_key_id": created.Key.ID})
	// Rotasi adalah pencabutan + pembuatan dalam satu tindakan: kedua event
	// diproduksi supaya pelanggan tidak melewatkan salah satunya.
	h.pancarkanEvent(ctx, "api_key.revoked", map[string]any{
		"api_key_id": id,
		"reason":     "rotasi",
	})
	h.pancarkanEvent(ctx, "api_key.created", map[string]any{
		"api_key_id": created.Key.ID,
		"name":       created.Key.Name,
		"prefix":     created.Key.Prefix,
	})
	// DTO dibangun dari cakupan tersimpan kunci BARU (yang disalin rotasi
	// dari kunci lama), bukan nil — lihat keyDetailResponse (B-112).
	mIDs, pIDs, _ := h.keyRepo.GetAllowed(ctx, created.Key.ID)
	_ = h.respond(w, r, http.StatusOK, KeyCreatedResponse{
		Key:    toKeyDTO(created.Key, mIDs, pIDs),
		Masked: created.Key.Masked(),
		RawKey: created.Raw.Reveal(),
	})
}

// deleteAPIKey menangani DELETE /api-keys/{id}: penghapusan permanen (B-080).
//
// Sejak siklus hidup kunci disederhanakan, inilah satu-satunya cara membuang
// key selamanya — endpoint pencabutan tersendiri sudah dicabut. Matikan
// sementara lewat toggle, ganti rahasia lewat rotate, buang lewat sini.
// Riwayat request tidak ikut terhapus: baris requests menyimpan snapshot
// nama key dari saat request dicatat (lihat keys.Repo.Delete).
func (h *Handlers) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.keyRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	h.bersihkanCounterRateLimitKey(ctx, id)
	h.writeAudit(ctx, r, "delete", "api_key", id, nil)
	// Pelanggan webhook tetap diberi tahu bahwa key ini mati permanen, lewat
	// event yang sama seperti matinya key oleh rotasi; reason membedakannya.
	h.pancarkanEvent(ctx, "api_key.revoked", map[string]any{
		"api_key_id": id,
		"reason":     "dihapus permanen",
	})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Body selaras dengan semua toggle lain di sistem (B-080): {enabled: bool}.
	// Bentuk lama {status: "active"|"disabled"} hanya dipakai toggle ini dan
	// kini ditolak decodeJSON sebagai field tak dikenal. Field enabled yang
	// ABSEN juga ditolak 400 (B-088, decodeToggleEnabled) — tidak pernah
	// ditafsirkan sebagai false.
	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	status := keys.StatusDisabled
	if enabled {
		status = keys.StatusActive
	}

	k, err := h.keyRepo.SetStatus(ctx, id, status)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	if !enabled {
		h.bersihkanCounterRateLimitKey(ctx, id)
	}
	h.writeAudit(ctx, r, "toggle", "api_key", id, map[string]any{"enabled": enabled})
	_ = h.respond(w, r, http.StatusOK, h.keyDetailResponse(ctx, k))
}

func (h *Handlers) getAPIKeyAllowed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	mIDs, pIDs, err := h.keyRepo.GetAllowed(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "izin model/provider api key")
		return
	}

	_ = h.respond(w, r, http.StatusOK, KeyAllowedResponse{
		ModelIDs:    mIDs,
		ProviderIDs: pIDs,
	})
}

func (h *Handlers) setAllowedAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		ModelIDs         []string `json:"model_ids"`
		ProviderIDs      []string `json:"provider_ids"`
		AllowedModels    []string `json:"allowed_models"`
		AllowedProviders []string `json:"allowed_providers"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if len(req.ModelIDs) == 0 && len(req.AllowedModels) > 0 {
		req.ModelIDs = req.AllowedModels
	}
	if len(req.ProviderIDs) == 0 && len(req.AllowedProviders) > 0 {
		req.ProviderIDs = req.AllowedProviders
	}

	// B-201: slug/nama yang tidak dikenal ditolak 400 di sini (menyebut
	// nilainya), bukan 500 dari cast uuid[] di repo.
	if err := h.validasiDaftarPutihAPIKey(ctx, req.ModelIDs, req.ProviderIDs); err != nil {
		tulisErrDaftarPutih(w, r, err)
		return
	}

	if err := h.keyRepo.SetAllowed(ctx, id, req.ModelIDs, req.ProviderIDs); err != nil {
		mapRepoError(w, r, err, "izin model/provider api key")
		return
	}

	h.writeAudit(ctx, r, "set_allowed", "api_key", id, map[string]any{"models": req.ModelIDs, "providers": req.ProviderIDs})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "updated"})
}

// -----------------------------------------------------------------------------
// Admin Users Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	userPage, err := h.usersRepo.List(ctx, identity.UserFilter{}, page)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	usersDTO := make([]UserDTO, 0, len(userPage.Users))
	for _, u := range userPage.Users {
		usersDTO = append(usersDTO, toUserDTO(&u))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[UserDTO]{
		Items:      usersDTO,
		NextCursor: userPage.NextCursor,
	})
}

func (h *Handlers) getUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	u, err := h.usersRepo.GetByID(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	_ = h.respond(w, r, http.StatusOK, UserDetailResponse{
		User:        toUserDTO(&u),
		Roles:       u.Roles(),
		Permissions: u.Permissions(),
	})
}

// Nilai field "role" yang sah pada pembuatan/pembaruan pengguna (B-102).
// Taksonominya dua tingkat, mengikuti model yang hidup di kode: "admin"
// (peran Admin, izin "*") dan "user" (pengguna biasa tanpa izin admin).
const (
	peranAdmin = "admin"
	peranUser  = "user"
)

// uraiPeran menerjemahkan nilai field "role" menjadi status admin. Nilai
// kosong BUKAN peran: pemanggil memutuskan maknanya (pada pembuatan =
// bawaan non-admin; pada pembaruan = peran tidak diubah). Nilai lain di
// luar dua peran yang sah ditolak (sah=false) supaya salah ketik tidak
// diam-diam menjadi keputusan kewenangan.
func uraiPeran(role string) (isAdmin bool, sah bool) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case peranAdmin:
		return true, true
	case peranUser:
		return false, true
	default:
		return false, false
	}
}

type createUserReq struct {
	Email              string `json:"email"`
	Name               string `json:"name"`
	Password           string `json:"password"`
	MustChangePassword bool   `json:"must_change_password"`
	// Role adalah peran yang diminta: "admin" atau "user". Kosong berarti
	// bawaan AMAN non-admin (B-102) — admin hanya lahir dari permintaan
	// eksplisit, tidak pernah dari ketiadaan field.
	Role string `json:"role"`
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createUserReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	isAdmin := false
	if req.Role != "" {
		var sah bool
		isAdmin, sah = uraiPeran(req.Role)
		if !sah {
			httpx.BadRequest(w, r, "invalid_role", fmt.Sprintf("role %q tidak sah; nilai yang diterima: %q atau %q", req.Role, peranAdmin, peranUser))
			return
		}
	}

	u, err := h.usersRepo.Create(ctx, identity.NewUser{
		Email:              req.Email,
		DisplayName:        req.Name,
		Password:           security.Secret(req.Password),
		MustChangePassword: req.MustChangePassword,
		IsAdmin:            isAdmin,
	})
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	h.writeAudit(ctx, r, "create", "user", u.ID, map[string]any{"email": u.Email, "is_admin": u.IsAdmin})
	_ = h.respond(w, r, http.StatusCreated, toUserDTO(&u))
}

func (h *Handlers) updateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name   string  `json:"name"`
		Status string  `json:"status"`
		Role   *string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// Validasi seluruh body DI MUKA, sebelum mutasi pertama (B-194).
	// Urutan lama bermutasi langkah demi langkah — nama ter-commit lebih
	// dulu, validasi peran menyusul — sehingga body campuran sah+tak sah
	// (mis. {name, role:"bogus"}) meninggalkan perubahan parsial yang
	// terjadi TANPA catatan audit. Kini semua yang bisa divalidasi dari
	// body dan keadaan awal target divalidasi di sini; penjaga yang
	// bergantung keadaan berubah (admin aktif terakhir) tetap di dalam
	// transaksinya masing-masing di bawah.
	var peranBaru *bool
	if req.Role != nil {
		isAdmin, sah := uraiPeran(*req.Role)
		if !sah {
			httpx.BadRequest(w, r, "invalid_role", fmt.Sprintf("role %q tidak sah; nilai yang diterima: %q atau %q", *req.Role, peranAdmin, peranUser))
			return
		}
		peranBaru = &isAdmin
	}
	if req.Status != "" && !identity.UserStatus(req.Status).Valid() {
		httpx.BadRequest(w, r, "invalid_status", fmt.Sprintf("status %q tidak sah; nilai yang diterima: %q, %q, atau %q", req.Status, identity.UserStatusActive, identity.UserStatusDisabled, identity.UserStatusLocked))
		return
	}

	// Target dibaca sekali di muka: 404 untuk pengguna yang tidak ada
	// kini mendahului semua mutasi (bukan muncul sesudah sebagian
	// ter-commit), dan keadaannya adalah dasar pemeriksaan diri.
	target, err := h.usersRepo.GetByID(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}
	diriSendiri := false
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil && p.User.ID == id {
		diriSendiri = true
	}
	// Menurunkan peran akun SENDIRI DITOLAK, sejajar penonaktifan
	// akun sendiri: sesi curian tidak boleh dipakai mengunci
	// admin keluar dari kewenangannya sendiri tanpa jejak khusus.
	if diriSendiri && peranBaru != nil && target.IsAdmin && !*peranBaru {
		httpx.BadRequest(w, r, "self_demote_forbidden", "peran akun sendiri tidak bisa diturunkan; minta admin lain yang melakukannya")
		return
	}
	// Mengubah status akun SENDIRI menjadi non-aktif DITOLAK: salah klik
	// atau sesi curian langsung mengunci admin keluar tanpa jejak khusus.
	if diriSendiri && req.Status != "" && identity.UserStatus(req.Status) != identity.UserStatusActive {
		httpx.BadRequest(w, r, "self_disable_forbidden", "akun sendiri tidak bisa dinonaktifkan; minta admin lain yang melakukannya")
		return
	}

	if req.Name != "" {
		_, err := h.usersRepo.UpdateProfile(ctx, id, identity.ProfileUpdate{
			DisplayName: &req.Name,
		})
		if err != nil {
			mapRepoError(w, r, err, "pengguna")
			return
		}
	}

	// Perubahan peran (B-102): "admin" menaikkan, "user" menurunkan. Field
	// absen (nil) berarti peran tidak disentuh — berbeda dari pembuatan,
	// ketiadaan field di sini tidak boleh menafsirkan apa pun. Nilai peran
	// dan penolakan turun-peran-sendiri sudah divalidasi di muka (B-194);
	// target di sini adalah bacaan awal yang sama.
	if peranBaru != nil {
		isAdmin := *peranBaru
		if target.IsAdmin != isAdmin {
			if isAdmin {
				if _, err := h.usersRepo.SetAdmin(ctx, id, true); err != nil {
					mapRepoError(w, r, err, "peran pengguna")
					return
				}
			} else {
				// Menurunkan ADMIN AKTIF TERAKHIR DITOLAK (lockout total),
				// dengan pola transaksi berkunci advisory yang sama seperti
				// penonaktifan/penghapusan di bawah.
				err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
					if err := kunciPenjagaAdminTx(ctx, q); err != nil {
						return err
					}
					users := identity.NewUsers(q)
					targetTx, err := users.GetByID(ctx, id)
					if err != nil {
						return err
					}
					if !targetTx.IsAdmin {
						return nil
					}
					if targetTx.Status == identity.UserStatusActive {
						jumlah, err := users.CountActiveAdmins(ctx)
						if err != nil {
							return err
						}
						if jumlah <= 1 {
							return errAdminAktifTerakhir
						}
					}
					_, err = users.SetAdmin(ctx, id, false)
					return err
				})
				if errors.Is(err, errAdminAktifTerakhir) {
					httpx.BadRequest(w, r, "last_admin_forbidden", "pengguna ini admin aktif terakhir; buat admin lain dulu sebelum menurunkan perannya")
					return
				}
				if err != nil {
					mapRepoError(w, r, err, "peran pengguna")
					return
				}
			}
		}
	}

	if req.Status != "" {
		if identity.UserStatus(req.Status) != identity.UserStatusActive {
			// (Penolakan non-aktif akun sendiri sudah divalidasi di muka, B-194.)
			// Menonaktifkan admin aktif TERAKHIR DITOLAK: mencegah lockout sistem.
			// Pemeriksaan dan mutasinya berjalan dalam SATU transaksi berkunci
			// advisory (lihat kunciPenjagaAdmin): pola periksa-lalu-tulis tanpa
			// kunci bisa dilewati dua penonaktifan bersamaan (audit3 V-AU-10).
			// Pemeriksaan tetap FAIL-CLOSED (audit F5): kegagalan query di
			// dalam transaksi membatalkan mutasi.
			//
			// Penolakan hanya berlaku bila TARGET saat ini benar-benar aktif:
			// menonaktifkan akun yang sudah nonaktif tidak mengurangi jumlah
			// admin aktif dan tidak perlu ditolak.
			err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
				if err := kunciPenjagaAdminTx(ctx, q); err != nil {
					return err
				}
				users := identity.NewUsers(q)
				target, err := users.GetByID(ctx, id)
				if err != nil {
					return err
				}
				// Yang dijaga adalah jumlah ADMIN aktif (B-102): menonaktifkan
				// pengguna biasa tidak mengurangi admin sama sekali, dan
				// pengguna biasa yang aktif tidak ikut menghitung sebagai
				// "admin lain" yang menjaga invariansi.
				if target.IsAdmin && target.Status == identity.UserStatusActive {
					jumlah, err := users.CountActiveAdmins(ctx)
					if err != nil {
						return err
					}
					if jumlah <= 1 {
						return errAdminAktifTerakhir
					}
				}
				_, err = users.SetStatus(ctx, id, identity.UserStatus(req.Status))
				return err
			})
			if errors.Is(err, errAdminAktifTerakhir) {
				httpx.BadRequest(w, r, "last_admin_forbidden", "pengguna ini admin aktif terakhir; buat admin lain dulu sebelum menonaktifkannya")
				return
			}
			if err != nil {
				mapRepoError(w, r, err, "status pengguna")
				return
			}
		} else {
			_, err := h.usersRepo.SetStatus(ctx, id, identity.UserStatus(req.Status))
			if err != nil {
				mapRepoError(w, r, err, "status pengguna")
				return
			}
		}
	}

	// Audit ditulis SEBELUM baca-ulang (B-194): catatan perubahan tidak
	// boleh bergantung pada keberhasilan membaca kembali baris yang baru
	// diubah — kegagalan baca-ulang tetap menjawab 500, tetapi perubahan
	// yang nyata itu wajib sudah teraudit.
	h.writeAudit(ctx, r, "update", "user", id, nil)

	// Gagal baca-ulang setelah update berarti 500, BUKAN 200 dengan user
	// kosong: menjawab zero-value membuat operator mengira pengguna terhapus
	// datanya.
	u, err := h.usersRepo.GetByID(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}
	_ = h.respond(w, r, http.StatusOK, toUserDTO(&u))
}

func (h *Handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Menghapus akun sendiri DITOLAK: lewat sesi curian ini berarti lockout
	// seketika, dan lewat klik yang salah berarti kehilangan akses admin tanpa
	// ada yang bisa mengembalikannya kecuali akses DB langsung.
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil && p.User.ID == id {
		httpx.BadRequest(w, r, "self_delete_forbidden", "akun sendiri tidak bisa dihapus; minta admin lain yang menghapusnya")
		return
	}
	// Menghapus admin aktif TERAKHIR DITOLAK untuk mencegah lockout sistem.
	// Pemeriksaan dan penghapusannya satu transaksi berkunci advisory, sama
	// seperti penonaktifan di updateUser — pola periksa-lalu-hapus tanpa kunci
	// dapat dilewati penghapusan/penonaktifan bersamaan (audit3 V-AU-10).
	// FAIL-CLOSED seperti sebelumnya (audit F5): error di dalam transaksi =
	// mutasi batal.
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		if err := kunciPenjagaAdminTx(ctx, q); err != nil {
			return err
		}
		users := identity.NewUsers(q)
		target, err := users.GetByID(ctx, id)
		if err != nil {
			return err
		}
		// Sama seperti penonaktifan: yang dijaga adalah ADMIN aktif
		// terakhir (B-102), bukan pengguna aktif terakhir.
		if target.IsAdmin && target.Status == identity.UserStatusActive {
			jumlah, err := users.CountActiveAdmins(ctx)
			if err != nil {
				return err
			}
			if jumlah <= 1 {
				return errAdminAktifTerakhir
			}
		}
		return users.Delete(ctx, id)
	})
	if errors.Is(err, errAdminAktifTerakhir) {
		httpx.BadRequest(w, r, "last_admin_forbidden", "pengguna ini admin aktif terakhir; buat admin lain dulu sebelum menghapusnya")
		return
	}
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	h.writeAudit(ctx, r, "delete", "user", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) forceResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		TempPassword string `json:"temp_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if err := h.usersRepo.ChangePassword(ctx, id, security.Secret(req.TempPassword), true); err != nil {
		mapRepoError(w, r, err, "reset password")
		return
	}

	// Pencabutan sesi adalah bagian INVARIANSI reset paksa: password sudah
	// diganti tetapi sesi bajakan tetap hidup bila ini gagal diam-diam.
	// Kegagalannya dilaporkan eksplisit (500 + jumlah sesi yang BERHASIL
	// dicabut) supaya operator tahu sesi mana yang masih hidup.
	revoked, err := h.sessionsRepo.RevokeAllOfUser(ctx, id)
	if err != nil {
		h.logger.ErrorContext(ctx, "reset password berhasil tetapi pencabutan sesi gagal",
			"sesi_tercabut", revoked, "user_id", id, "error", err)
		// B-271: perubahan password SUDAH ter-commit pada titik ini — fakta
		// kredensial berubah itu wajib tetap teraudit walau pencabutan sesi
		// gagal. Sebelumnya jalur ini menjawab 500 tanpa satu pun baris
		// audit, sehingga perubahan kredensial yang benar-benar terjadi
		// tidak terlihat di jejak audit. Metadata menandai hasil sebagian
		// secara eksplisit.
		h.writeAudit(ctx, r, "force_reset_password", "user", id, map[string]any{
			"sesi_tercabut":         revoked,
			"pencabutan_sesi_gagal": true,
		})
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.ErrTypeAPI,
			"revoke_failed", "password sudah diganti tetapi pencabutan sesi gagal; cabut manual lewat revoke-sessions")
		return
	}

	h.writeAudit(ctx, r, "force_reset_password", "user", id, map[string]any{"sesi_tercabut": revoked})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "password_reset"})
}

// unlockUser menangani POST /api/admin/access/users/{id}/unlock: membuka
// kunci login pengguna secara manual (audit3 B-076). Sebelumnya method repo
// Users.Unlock lengkap dan teruji tetapi tidak terjangkau permukaan mana
// pun — akun terkunci hanya pulih sesudah locked_until kedaluwarsa.
//
// Perilaku untuk pengguna yang memang TIDAK terkunci: idempoten 200 dengan
// keadaan akhirnya (Unlock menolkan penghitung gagal dan mengosongkan
// locked_until; status hanya berubah dari locked). Ini sejajar
// revoke-sessions pada pengguna tanpa sesi yang menjawab 200 count=0.
// Yang ditolak hanya pengguna yang tidak ada (404 dari repo).
func (h *Handlers) unlockUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	u, err := h.usersRepo.Unlock(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	h.writeAudit(ctx, r, "unlock", "user", id, nil)
	_ = h.respond(w, r, http.StatusOK, toUserDTO(&u))
}

func (h *Handlers) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	count, err := h.sessionsRepo.RevokeAllOfUser(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "sesi")
		return
	}

	h.writeAudit(ctx, r, "revoke_sessions", "user", id, map[string]any{"count": count})
	_ = h.respond(w, r, http.StatusOK, StatusCountResponse{Status: "revoked", Count: int64(count)})
}

// -----------------------------------------------------------------------------
// Sessions Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
			userID = p.User.ID
		}
	}

	sessions, err := h.sessionsRepo.ListActive(ctx, userID)
	if err != nil {
		mapRepoError(w, r, err, "sesi")
		return
	}

	sessionDTOs := make([]SessionDTO, 0, len(sessions))
	for _, s := range sessions {
		sessionDTOs = append(sessionDTOs, toSessionDTO(s))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[SessionDTO]{
		Items: sessionDTOs,
	})
}

func (h *Handlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.sessionsRepo.Revoke(ctx, id); err != nil {
		mapRepoError(w, r, err, "sesi")
		return
	}

	h.writeAudit(ctx, r, "revoke", "session", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "revoked"})
}

// revokeAllSessions menangani POST /api/admin/access/sessions/revoke-all:
// mencabut SELURUH sesi aktif semua pengguna sekaligus.
//
// Ini saklar respons insiden yang selama ini tidak ada (audit3 V-AU-06):
// daftar sesi hanya bisa dicabut satu per satu atau per pengguna, dan
// runbook lama keliru menyarankan rotasi SESSION_SECRET — padahal keabsahan
// sesi adalah baris database dan secret itu hanya menandatangani token CSRF.
// Sesi operator yang memanggil ikut tercabut (respons ini tetap terkirim;
// request berikutnya wajib masuk ulang) — memang itulah tujuannya.
func (h *Handlers) revokeAllSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	count, err := h.sessionsRepo.RevokeAll(ctx)
	if err != nil {
		mapRepoError(w, r, err, "sesi")
		return
	}

	h.writeAudit(ctx, r, "revoke_all_sessions", "session", "", map[string]any{"count": count})
	_ = h.respond(w, r, http.StatusOK, StatusCountResponse{Status: "revoked", Count: int64(count)})
}
