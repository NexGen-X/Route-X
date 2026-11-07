package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/router"
)

func (h *Handlers) gatewayRoutes(r chi.Router) {
	// 1. Routing Rules
	r.Route("/routing-rules", func(rr chi.Router) {
		rr.With(auth.RequirePermission(seed.PermRoutingRead)).Get("/", h.listRoutingRules)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Post("/", h.createRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingRead)).Get("/{id}", h.getRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Put("/{id}", h.updateRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Delete("/{id}", h.deleteRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Post("/{id}/toggle", h.toggleRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Post("/{id}/providers", h.setRuleProviders)
	})

	// 2. Rate Limits
	r.Route("/rate-limits", func(rl chi.Router) {
		rl.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listRateLimits)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Post("/", h.createRateLimit)
		rl.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/{id}", h.getRateLimit)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Put("/{id}", h.updateRateLimit)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Delete("/{id}", h.deleteRateLimit)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Post("/{id}/toggle", h.toggleRateLimit)
	})

	// 3. Budgets
	r.Route("/budgets", func(br chi.Router) {
		br.With(auth.RequirePermission(seed.PermUsageRead)).Get("/", h.listBudgets)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Post("/", h.createBudget)
		br.With(auth.RequirePermission(seed.PermUsageRead)).Get("/{id}", h.getBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Put("/{id}", h.updateBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Delete("/{id}", h.deleteBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Post("/{id}/toggle", h.toggleBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Post("/{id}/reset", h.resetBudget)
	})

	// 4. Bans
	r.Route("/bans", func(bnr chi.Router) {
		bnr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listBans)
		bnr.With(auth.RequirePermission(seed.PermBansWrite)).Post("/", h.createBan)
		bnr.With(auth.RequirePermission(seed.PermBansWrite)).Post("/{id}/lift", h.liftBan)
	})

	// 5. Content Filters
	r.Route("/content-filters", func(cfr chi.Router) {
		cfr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listContentFilters)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Post("/", h.createContentFilter)
		cfr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/{id}", h.getContentFilter)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Put("/{id}", h.updateContentFilter)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Delete("/{id}", h.deleteContentFilter)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Post("/{id}/toggle", h.toggleContentFilter)
	})

	// 6. Circuit Breakers
	r.Route("/circuit-breakers", func(cbr chi.Router) {
		cbr.With(auth.RequirePermission(seed.PermHealthRead)).Get("/", h.listCircuitBreakers)
		cbr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/reset", h.resetCircuitBreaker)
	})
}

// -----------------------------------------------------------------------------
// Routing Rules Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listRoutingRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := repo.DefaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	filter := upstream.RoutingRuleFilter{
		Search: r.URL.Query().Get("search"),
	}
	if s := r.URL.Query().Get("enabled"); s != "" {
		en := s == "true"
		filter.Enabled = &en
	}

	page := repo.Page{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.routingRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	dtos := make([]RoutingRuleDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toRoutingRuleDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[RoutingRuleDTO]{
		Items:      dtos,
		NextCursor: next,
	})
}

func (h *Handlers) getRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	rule, err := h.routingRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
}

type createRoutingRuleReq struct {
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Strategy          string          `json:"strategy"`
	Priority          int             `json:"priority"`
	MatchModelID      *string         `json:"match_model_id"`
	MatchAPIKeyID     *string         `json:"match_api_key_id"`
	MatchCapabilities []string        `json:"match_capabilities"`
	MaxAttempts       *int            `json:"max_attempts"`
	BackoffMS         *int            `json:"backoff_ms"`
	FailureThreshold  *int            `json:"failure_threshold"`
	OpenDurationMS    *int            `json:"open_duration_ms"`
	HalfOpenProbes    *int            `json:"half_open_probes"`
	Enabled           *bool           `json:"enabled"`
	ProviderIDs       []string        `json:"provider_ids"`
	Weights           map[string]int  `json:"weights"`
	Pipeline          json.RawMessage `json:"pipeline"`
	VirtualAlias      *string         `json:"virtual_alias"`
}

func cleanUUID(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	return &trimmed
}

// galatProviderTanpaModel menandai penolakan audit3 V-FF-11: provider yang
// diikat ke aturan tidak memetakan model target aturan itu, sehingga
// kandidatnya tidak akan pernah bisa melayani model tersebut. Dulu ketiga
// jalur tulis aturan (buat, ubah, pasang provider) menerima ikatan mati
// ini tanpa bunyi; kini ditolak 400 dengan nama providernya disebut.
type galatProviderTanpaModel struct {
	Nama []string
}

func (e *galatProviderTanpaModel) Error() string {
	return fmt.Sprintf(
		"provider berikut tidak memetakan model target aturan ini: %s — petakan dulu modelnya ke provider tersebut, atau lepaskan provider itu dari aturan",
		strings.Join(e.Nama, ", "),
	)
}

// tulisGalatProviderModel menulis respons 400 bila err adalah penolakan
// V-FF-11 dan mengembalikan true; false berarti err lain, tangani seperti
// biasa.
func tulisGalatProviderModel(w http.ResponseWriter, r *http.Request, err error) bool {
	var g *galatProviderTanpaModel
	if errors.As(err, &g) {
		httpx.BadRequest(w, r, "provider_tidak_memetakan_model", g.Error())
		return true
	}
	return false
}

// periksaIkatanProvider memeriksa daftar provider terhadap model target
// aturan dan mengembalikan galatProviderTanpaModel bila ada yang tidak
// memetakan model itu.
func periksaIkatanProvider(ctx context.Context, repoRouting *upstream.RoutingRepo, modelID string, providerIDs []string) error {
	tanpa, err := repoRouting.ProviderTanpaPemetaanModel(ctx, modelID, providerIDs)
	if err != nil {
		return err
	}
	if len(tanpa) > 0 {
		return &galatProviderTanpaModel{Nama: tanpa}
	}
	return nil
}

// galatValidasiAturan adalah penolakan validasi jalur tulis aturan routing
// dengan kode galat yang stabil untuk klien. Dipakai keluarga pemeriksaan
// B-179/B-180/B-183/B-184: konfigurasi yang lolos simpan tetapi tidak akan
// pernah berlaku sebagaimana tampilannya harus ditolak di sini, bukan
// ditemukan operator dari perilaku produksi.
type galatValidasiAturan struct {
	Kode  string
	Pesan string
}

func (e *galatValidasiAturan) Error() string { return e.Pesan }

// tulisGalatValidasiAturan menulis respons 400 bila err adalah penolakan
// galatValidasiAturan dan mengembalikan true; false berarti err lain,
// tangani seperti biasa.
func tulisGalatValidasiAturan(w http.ResponseWriter, r *http.Request, err error) bool {
	var g *galatValidasiAturan
	if errors.As(err, &g) {
		httpx.BadRequest(w, r, g.Kode, g.Error())
		return true
	}
	return false
}

// periksaAnggotaPipeline memvalidasi anggota resep combo terhadap katalog
// model (B-179, B-180). Resolusi memakai ModelRepo.Resolve yang sama dengan
// jalur runtime kandidatCombo, supaya "dikenal" di sini berarti persis
// "dikenal" saat resep dijalankan:
//
//   - Anggota yang tidak bisa diresolusi ditolak (B-179): runtime hanya
//     melewatinya dengan log Warn, sehingga resep menyusut diam-diam dan
//     gabungan kosong jatuh ke kandidat model jangkar — endpoint combo
//     yang tampak sah tetapi tidak pernah menjalankan resepnya.
//   - Sesudah resolusi harus tersisa minimal dua model BERBEDA (B-180):
//     dua nama/alias/UUID yang menunjuk model kanonik yang sama
//     menyusutkan resep menjadi satu model, dan Rule.Combo() menuntut
//     lebih dari satu anggota — resep seperti itu tidak pernah masuk
//     jalur combo sama sekali.
func (h *Handlers) periksaAnggotaPipeline(ctx context.Context, pipeline []byte) error {
	if len(pipeline) == 0 || h.modelRepo == nil {
		return nil
	}
	dto, err := router.ParseComboPipelineDTO(pipeline)
	if err != nil {
		return err
	}
	terlihat := make(map[string]string, len(dto.Models))
	for _, nama := range dto.Models {
		m, err := h.modelRepo.Resolve(ctx, nama)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return &galatValidasiAturan{
					Kode:  "pipeline_model_tidak_dikenal",
					Pesan: fmt.Sprintf("model %q dalam pipeline tidak dikenal: sinkronkan atau petakan dulu modelnya ke katalog, atau keluarkan dari resep — anggota yang tidak dikenal hanya dilewati saat runtime dan resep menyusut diam-diam", nama),
				}
			}
			return err
		}
		if m == nil {
			return &galatValidasiAturan{
				Kode:  "pipeline_model_tidak_dikenal",
				Pesan: fmt.Sprintf("model %q dalam pipeline tidak dikenal: sinkronkan atau petakan dulu modelnya ke katalog, atau keluarkan dari resep — anggota yang tidak dikenal hanya dilewati saat runtime dan resep menyusut diam-diam", nama),
			}
		}
		if namaLain, ganda := terlihat[m.ID]; ganda {
			return &galatValidasiAturan{
				Kode:  "pipeline_model_berulang",
				Pesan: fmt.Sprintf("model %q dan %q dalam pipeline menunjuk model yang sama (%s) sesudah resolusi: resep combo butuh model yang benar-benar berbeda", namaLain, nama, m.ModelID),
			}
		}
		terlihat[m.ID] = nama
	}
	if len(terlihat) < 2 {
		return &galatValidasiAturan{
			Kode:  "pipeline_model_berulang",
			Pesan: fmt.Sprintf("pipeline combo butuh minimal 2 model berbeda sesudah resolusi, dapat %d: resep satu model tidak pernah masuk jalur combo", len(terlihat)),
		}
	}
	return nil
}

// tolakKapabilitasMustahil menolak kombinasi match_capabilities yang tidak
// akan pernah cocok dengan permukaan mana pun (B-183). Permukaan chat
// (chat/completions, messages, responses) meminta subset dari
// {text, vision, tools, reasoning}; permukaan embeddings meminta tepat
// {embeddings}. Aturan yang mensyaratkan embeddings BERSAMA kapabilitas
// lain menuntut satu permintaan meminta keduanya sekaligus — tidak pernah
// terjadi — sehingga aturan itu mati sejak disimpan.
func tolakKapabilitasMustahil(caps []string) *galatValidasiAturan {
	punya := make(map[string]bool, len(caps))
	for _, c := range caps {
		punya[c] = true
	}
	if punya["embeddings"] && len(punya) > 1 {
		return &galatValidasiAturan{
			Kode:  "kapabilitas_tidak_mungkin",
			Pesan: "match_capabilities tidak boleh menggabungkan embeddings dengan kapabilitas lain: permukaan embeddings hanya meminta {embeddings} dan permukaan chat tidak pernah meminta embeddings, jadi aturan ini tidak akan pernah cocok",
		}
	}
	return nil
}

// tolakIkatanProviderCombo menolak ikatan provider pada aturan ber-pipeline
// (B-184). Semantik yang dipilih: ikatan provider (Rule.Providers) hanya
// berlaku di jalur non-combo untuk model target; mesin SENGAJA tidak
// menerapkannya pada resep combo (lihat komentar Engine.OrderCandidates —
// pembatasan model target tidak bermakna untuk kandidat model lain dalam
// resep). Menyimpan ikatan yang tidak akan pernah diterapkan menyesatkan
// operator, jadi jalur tulis menolaknya; operator combo membatasi lewat
// anggota resep, bukan ikatan provider.
func tolakIkatanProviderCombo(pipeline []byte, providerIDs []string) *galatValidasiAturan {
	if len(pipeline) > 0 && len(providerIDs) > 0 {
		return &galatValidasiAturan{
			Kode:  "provider_tidak_berlaku_untuk_combo",
			Pesan: "aturan dengan pipeline combo tidak memakai ikatan provider: mesin mengabaikan ikatan provider pada resep combo, jadi ikatan tidak disimpan agar tidak menyesatkan — kosongkan daftar provider atau hapus pipeline",
		}
	}
	return nil
}

// literalNullJSON adalah representasi JSON null mentah, dipakai membandingkan field
// pipeline mentah supaya "null eksplisit" bisa dibedakan dari "tidak disebut".
var literalNullJSON = []byte("null")

// permintaanPipeline mengubah field pipeline dari body permintaan menjadi jsonb siap
// simpan beserta mode perubahannya.
//
// Pipeline adalah satu-satunya field di permintaan ini yang harus membedakan TIGA
// keadaan: tidak disebut (biarkan resep yang sekarang), null eksplisit (hapus resep,
// kembali ke jalur lama), dan objek (pasang resep). Pointer biasa hanya bisa
// membedakan dua keadaan, sehingga update tidak bisa menghapus resep combo — dan itulah
// jebakan yang membuat alias lama tak pernah bisa dilepas dari aturannya.
//
// Mengembalikan (diatur, data): diatur=false berarti biarkan; diatur=true dengan data nil
// berarti hapus; diatur=true dengan data terisi berarti pasang.
func permintaanPipeline(raw json.RawMessage) (diatur bool, data []byte, err error) {
	if len(raw) == 0 {
		return false, nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), literalNullJSON) {
		return true, nil, nil
	}
	p, err := router.ParseComboPipelineDTO(raw)
	if err != nil {
		return false, nil, err
	}
	// Disimpan ulang dalam bentuk kanonik: field asing yang tidak dikenal constraint
	// maupun mesin dibuang, supaya jsonb di database hanya berisi apa yang dipakai.
	out, mErr := json.Marshal(p)
	if mErr != nil {
		return false, nil, mErr
	}
	return true, out, nil
}

// aliasPermintaan menormalkan field virtual_alias dari body permintaan.
//
// Mengembalikan (diatur, nilai): diatur=false berarti field tidak disebut, biarkan; nilai
// kosong berarti hapus alias (whitespace dianggap kosong karena alias adalah pengenal
// publik, bukan teks bebas).
func aliasPermintaan(v *string) (diatur bool, nilai string) {
	if v == nil {
		return false, ""
	}
	return true, strings.TrimSpace(*v)
}

// pastikanAliasBebas memastikan alias belum diduduki aturan lain.
//
// Pemeriksaan ini memberi pesan 409 yang menjelaskan aturan mana yang memakainya, sebelum
// partial unique index routing_rules_virtual_alias_idx menolaknya sebagai pelanggaran
// constraint generik. Unique index tetap menjadi pengaman terakhir untuk dua permintaan
// yang berlomba — pemeriksaan ini tidak memakai kunci baris, dan itupun sudah cukup
// karena operator admin adalah tunggal.
func (h *Handlers) pastikanAliasBebas(ctx context.Context, alias, idSekarang string) error {
	if alias == "" {
		return nil
	}
	pemilik, err := h.routingRepo.FindByAlias(ctx, alias)
	if errors.Is(err, repo.ErrNotFound) {
		return nil
	}
	if err != nil {
		// Bukan kegagalan keunikan: jangan blokir penyimpanan hanya karena pemeriksaan
		// tambahan ini tidak bisa dibaca. Unique index menangkalu ganda yang sebenarnya.
		return nil
	}
	if pemilik.ID == idSekarang {
		return nil
	}
	return fmt.Errorf("alias virtual %q sudah dipakai aturan %q", alias, pemilik.Name)
}

// tolakAliasYatim menolak alias yang disertai tanpa resep pipeline combo.
//
// Tanpa pipeline, aturan tidak punya apapun untuk di-failover-kan ke model lain, dan
// aliasnya hanya menjebak klien yang memanggilnya: permintaan akan cocok dengan aturan
// yang kondisi pencocokannya tidak pernah diminta siapa pun. Dulu alias yatim seperti ini
// tertinggal diam-diam karena pendaftarannya adalah panggilan API terpisah yang
// kegagalannya hanya dicatat sebagai peringatan.
func tolakAliasYatim(alias string, pipeline []byte) error {
	if alias == "" || len(pipeline) > 0 {
		return nil
	}
	return fmt.Errorf("virtual_alias %q wajib disertai pipeline combo; kirim pipeline atau kosongkan virtual_alias", alias)
}

// tolakAliasTanpaModel menolak alias yang disertai pipeline tetapi
// tanpa match_model_id (B-165).
//
// Resolusi nama di permukaan chat (engine.FindRuleTargetModelID)
// hanya mengembalikan target bila aturan memiliki MatchModelID —
// alias + pipeline saja tidak pernah cukup, terbukti live: combo
// tanpa model target menjawab 404 model_not_found untuk setiap
// panggilan, dan 404 resolusi itu bahkan tidak tercatat di tabel
// requests. Menolaknya di jalur tulis menyelaraskan validasi simpan
// dengan kenyataan resolusi: operator menerima 400 yang menyebut
// syaratnya, bukan endpoint virtual mati yang tampak sah.
func tolakAliasTanpaModel(alias, matchModelID string) error {
	if alias == "" || strings.TrimSpace(matchModelID) != "" {
		return nil
	}
	return fmt.Errorf("virtual_alias %q wajib disertai match_model_id; alias combo harus menunjuk model target (jangkar resolusi) atau kosongkan virtual_alias", alias)
}

func (h *Handlers) createRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createRoutingRuleReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// Pipeline diurai dan divalidasi SEBELUM transaksi. Objek yang cacat ditolak di
	// sini, bukan oleh constraint database sesudah aturan dibuat — penolakan terlambat
	// itu membiarkan operator menyimpan aturan tanpa sengaja dan kehilangan seluruh
	// isian formnya.
	_, pipeline, err := permintaanPipeline(req.Pipeline)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_pipeline", err.Error())
		return
	}
	_, alias := aliasPermintaan(req.VirtualAlias)
	if err := tolakAliasYatim(alias, pipeline); err != nil {
		httpx.BadRequest(w, r, "alias_tanpa_pipeline", err.Error())
		return
	}
	// Nilai efektif yang benar-benar akan disimpan adalah hasil
	// cleanUUID — string non-UUID tersimpan sebagai NULL, jadi
	// pemeriksaan B-165 berjalan atas nilai efektif itu.
	matchModel := ""
	if v := cleanUUID(req.MatchModelID); v != nil {
		matchModel = *v
	}
	if err := tolakAliasTanpaModel(alias, matchModel); err != nil {
		httpx.BadRequest(w, r, "alias_tanpa_model", err.Error())
		return
	}
	// B-183: kombinasi kapabilitas yang mustahil cocok ditolak sebelum
	// aturan lahir, bukan ditemukan operator dari aturan yang tak pernah
	// terpicu.
	if g := tolakKapabilitasMustahil(req.MatchCapabilities); g != nil {
		httpx.BadRequest(w, r, g.Kode, g.Error())
		return
	}
	// B-179/B-180: anggota resep divalidasi terhadap katalog sekarang,
	// dengan resolver yang sama seperti runtime.
	if err := h.periksaAnggotaPipeline(ctx, pipeline); err != nil {
		if tulisGalatValidasiAturan(w, r, err) {
			return
		}
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	// B-184: ikatan provider pada aturan ber-pipeline tidak pernah
	// diterapkan mesin; menolaknya menjaga validasi simpan jujur.
	if g := tolakIkatanProviderCombo(pipeline, req.ProviderIDs); g != nil {
		httpx.BadRequest(w, r, g.Kode, g.Error())
		return
	}
	if err := h.pastikanAliasBebas(ctx, alias, ""); err != nil {
		httpx.WriteError(w, r, http.StatusConflict, httpx.ErrTypeInvalidRequest, "alias_conflict", err.Error())
		return
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}
	var prio *int
	if req.Priority > 0 {
		prio = &req.Priority
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	var rule *upstream.RoutingRule
	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRouting := upstream.NewRoutingRepo(q)
		var err error
		rule, err = txRouting.Create(ctx, upstream.CreateRoutingRuleParams{
			Name:              req.Name,
			Description:       desc,
			Priority:          prio,
			MatchModelID:      cleanUUID(req.MatchModelID),
			MatchAPIKeyID:     cleanUUID(req.MatchAPIKeyID),
			MatchCapabilities: req.MatchCapabilities,
			Strategy:          req.Strategy,
			MaxAttempts:       req.MaxAttempts,
			BackoffMS:         req.BackoffMS,
			FailureThreshold:  req.FailureThreshold,
			OpenDurationMS:    req.OpenDurationMS,
			HalfOpenProbes:    req.HalfOpenProbes,
			Enabled:           req.Enabled,
			CreatedBy:         actorID,
			Pipeline:          pipeline,
			VirtualAlias:      &alias,
		})
		if err != nil {
			return err
		}

		if len(req.ProviderIDs) > 0 {
			// V-FF-11: ikatan provider wajib memetakan model target aturan.
			if modelID := cleanUUID(req.MatchModelID); modelID != nil {
				if err := periksaIkatanProvider(ctx, txRouting, *modelID, req.ProviderIDs); err != nil {
					return err
				}
			}
			if err := txRouting.SetProviders(ctx, rule.ID, req.ProviderIDs, req.Weights); err != nil {
				return err
			}
			rule, err = txRouting.Get(ctx, rule.ID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if tulisGalatProviderModel(w, r, err) {
			return
		}
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	if rule == nil {
		httpx.InternalError(w, r)
		return
	}

	h.writeAudit(ctx, r, "create", "routing_rule", rule.ID, map[string]any{
		"name":          rule.Name,
		"strategy":      rule.Strategy,
		"virtual_alias": alias,
	})
	h.invalidateRules(ctx)
	_ = h.respond(w, r, http.StatusCreated, toRoutingRuleDTO(rule))
}

func (h *Handlers) updateRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req createRoutingRuleReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	pipeDiatur, pipeline, err := permintaanPipeline(req.Pipeline)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_pipeline", err.Error())
		return
	}
	aliasDiatur, aliasBaru := aliasPermintaan(req.VirtualAlias)

	// Aturan yang diubah bisa sudah punya alias dan resep combo sebelumnya, jadi
	// permintaan sebagian ini harus digabung dengan keadaan sekarang. Tanpa ini, "biarkan
	// alias" tak bisa dibedakan dari "hapus alias", dan aturan combo yang hanya diubah
	// provider-nya akan kehilangan aliasnya diam-diam.
	existing, err := h.routingRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	pipeAkhir := existing.Pipeline
	if pipeDiatur {
		pipeAkhir = pipeline
	}
	aliasAkhir := ""
	if existing.VirtualAlias != nil {
		aliasAkhir = *existing.VirtualAlias
	}
	if aliasDiatur {
		aliasAkhir = aliasBaru
	}
	if err := tolakAliasYatim(aliasAkhir, pipeAkhir); err != nil {
		httpx.BadRequest(w, r, "alias_tanpa_pipeline", err.Error())
		return
	}
	if err := h.pastikanAliasBebas(ctx, aliasAkhir, id); err != nil {
		httpx.WriteError(w, r, http.StatusConflict, httpx.ErrTypeInvalidRequest, "alias_conflict", err.Error())
		return
	}

	// Model target akhir sesudah penggabungan dengan keadaan sekarang —
	// dasar pemeriksaan ikatan provider V-FF-11 di dalam transaksi.
	modelAkhir := ""
	if existing.MatchModelID != nil {
		modelAkhir = *existing.MatchModelID
	}
	if req.MatchModelID != nil {
		modelAkhir = strings.TrimSpace(*req.MatchModelID)
	}
	// B-165 di jalur ubah berjalan atas keadaan AKHIR gabungan:
	// menghapus match_model_id dari aturan ber-alias (atau memberi
	// alias ke aturan bermodel kosong) sama mematikannya dengan
	// membuat combo tanpa model sejak awal.
	if err := tolakAliasTanpaModel(aliasAkhir, modelAkhir); err != nil {
		httpx.BadRequest(w, r, "alias_tanpa_model", err.Error())
		return
	}
	// B-183 di jalur ubah: MatchCapabilities selalu diganti utuh oleh
	// permintaan, jadi nilai permintaan adalah keadaan akhir.
	if g := tolakKapabilitasMustahil(req.MatchCapabilities); g != nil {
		httpx.BadRequest(w, r, g.Kode, g.Error())
		return
	}
	// B-179/B-180 di jalur ubah berjalan atas resep AKHIR gabungan, hanya
	// bila permintaan ini memang mengubah resep: aturan warisan dengan
	// anggota yang sudah tidak dikenal tetap boleh diubah field lainnya
	// (sinyal runtime B-179 menandai degradasinya), tetapi resep baru
	// tidak boleh disimpan dalam keadaan menyusut diam-diam.
	if pipeDiatur {
		if err := h.periksaAnggotaPipeline(ctx, pipeAkhir); err != nil {
			if tulisGalatValidasiAturan(w, r, err) {
				return
			}
			mapRepoError(w, r, err, "aturan routing")
			return
		}
	}
	// B-184 di jalur ubah: yang ditolak adalah permintaan yang MENCIPTAKAN
	// keadaan ikatan-provider + pipeline — mengikat provider ke aturan
	// ber-pipeline, atau memasang pipeline ke aturan yang terikat
	// provider. Keadaan warisan yang sudah ada tidak dikunci dari
	// perubahan field lain.
	if req.ProviderIDs != nil {
		if g := tolakIkatanProviderCombo(pipeAkhir, req.ProviderIDs); g != nil {
			httpx.BadRequest(w, r, g.Kode, g.Error())
			return
		}
	} else if pipeDiatur {
		providerAda := make([]string, 0, len(existing.Providers))
		for _, rp := range existing.Providers {
			providerAda = append(providerAda, rp.ProviderID)
		}
		if g := tolakIkatanProviderCombo(pipeAkhir, providerAda); g != nil {
			httpx.BadRequest(w, r, g.Kode, g.Error())
			return
		}
	}

	var p upstream.UpdateRoutingRuleParams
	if req.Name != "" {
		p.Name = &req.Name
	}
	// KEPUTUSAN TERCATAT (B-418): description tidak bisa
	// dikosongkan dan priority tidak bisa di-nol-kan lewat update
	// ini — keduanya dijaga oleh != "" / > 0 di bawah, melebur
	// "absen" dan "kosong" menjadi "biarkan". Batas ekspresif
	// yang sadar, sejajar updateContentFilter: harga pola
	// anti-B-369, dan tidak ada kebutuhan operasional nyata untuk
	// description kosong atau priority 0 pada aturan yang sudah
	// ada (aturan baru membuatnya lewat create).
	if req.Description != "" {
		p.Description = upstream.Set(req.Description)
	}
	if req.Priority > 0 {
		p.Priority = &req.Priority
	}
	if req.Strategy != "" {
		p.Strategy = &req.Strategy
	}
	if req.MatchModelID != nil {
		if strings.TrimSpace(*req.MatchModelID) == "" {
			p.MatchModelID = upstream.Clear[string]()
		} else {
			trimmed := strings.TrimSpace(*req.MatchModelID)
			p.MatchModelID = upstream.Set(trimmed)
		}
	}
	if req.MatchAPIKeyID != nil {
		if strings.TrimSpace(*req.MatchAPIKeyID) == "" {
			p.MatchAPIKeyID = upstream.Clear[string]()
		} else {
			trimmed := strings.TrimSpace(*req.MatchAPIKeyID)
			p.MatchAPIKeyID = upstream.Set(trimmed)
		}
	}
	p.MatchCapabilities = req.MatchCapabilities
	p.MaxAttempts = req.MaxAttempts
	p.BackoffMS = req.BackoffMS
	p.FailureThreshold = req.FailureThreshold
	p.OpenDurationMS = req.OpenDurationMS
	p.HalfOpenProbes = req.HalfOpenProbes
	p.Enabled = req.Enabled
	if pipeDiatur {
		if len(pipeline) == 0 {
			p.Pipeline = upstream.Clear[[]byte]()
		} else {
			p.Pipeline = upstream.Set(pipeline)
		}
	}
	if aliasDiatur {
		if aliasBaru == "" {
			p.VirtualAlias = upstream.Clear[string]()
		} else {
			p.VirtualAlias = upstream.Set(aliasBaru)
		}
	}

	// B-501: no-op total diputuskan SEBELUM transaksi dari keadaan
	// tersimpan. B-468 membandingkan nilai hanya untuk menekan
	// audit/invalidasi SESUDAH repo Update menulis baris — jadi
	// updated_at tetap bergerak pada PUT nilai-sama. Kini, bila
	// setiap field yang disebut nilai-sama DAN ikatan provider
	// absen/identik DAN permintaan memang menyebut sesuatu (params
	// tak kosong atau provider_ids disebut), Update dan
	// SetProviders tidak dipanggil sama sekali: baris tidak
	// ditulis, updated_at tidak bergerak. Permintaan yang tidak
	// menyebut apa pun TIDAK dilewatkan — ia tetap jatuh ke
	// ErrNoChanges repo dan 400 no_changes seperti kontrak
	// B-401/B-434.
	nilaiSama := perubahanRoutingNilaiSama(existing, p)
	providerIdentikAwal := req.ProviderIDs != nil &&
		ikatanProviderIdentik(existing.Providers, req.ProviderIDs, req.Weights)
	lewatiTulis := nilaiSama && (req.ProviderIDs == nil || providerIdentikAwal) &&
		(paramsRoutingMenyebut(p) || req.ProviderIDs != nil)

	var rule *upstream.RoutingRule
	// B-434: dua penanda no-op dijumlahkan sesudah transaksi —
	// pembaruan field yang ditelan sebagai ErrNoChanges DAN ikatan
	// provider yang identik berarti permintaan ini tidak mengubah
	// apa pun, jadi audit "update" dan invalidasi cache aturan
	// tidak ditulis (respons tetap 200 dengan keadaan tersimpan).
	updateDitelan := false
	providerIdentik := false
	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRouting := upstream.NewRoutingRepo(q)
		var err error
		if lewatiTulis {
			// B-501: no-op total — baca keadaan tersimpan tanpa
			// satu pun tulisan baris.
			updateDitelan = true
			rule, err = txRouting.Get(ctx, id)
			if err != nil {
				return err
			}
		} else {
			rule, err = txRouting.Update(ctx, id, p)
		}
		if err != nil && !lewatiTulis {
			// B-401: body yang HANYA menyebut provider_ids tidak
			// membawa perubahan field, jadi Update menolaknya
			// dengan ErrNoChanges — dan penolakan itu dulu
			// menggagalkan seluruh transaksi, termasuk
			// SetProviders yang sah di bawah (400 palsu untuk
			// permintaan ikatan yang sah). Telan sentinel itu
			// HANYA bila ikatan provider memang disebut; tanpa
			// provider_ids, no_changes tetap kesalahan pemanggil
			// yang diteruskan ke mapRepoError.
			if !(req.ProviderIDs != nil && errors.Is(err, upstream.ErrNoChanges)) {
				return err
			}
			updateDitelan = true
		}

		if req.ProviderIDs != nil {
			// V-FF-11: ikatan provider wajib memetakan model target akhir.
			if modelAkhir != "" && len(req.ProviderIDs) > 0 {
				if err := periksaIkatanProvider(ctx, txRouting, modelAkhir, req.ProviderIDs); err != nil {
					return err
				}
			}
			// B-434: ikatan yang identik (urutan + bobot — urutan
			// adalah posisi fallback) tidak ditulis ulang.
			providerIdentik = ikatanProviderIdentik(existing.Providers, req.ProviderIDs, req.Weights)
			if !providerIdentik {
				if err := txRouting.SetProviders(ctx, id, req.ProviderIDs, req.Weights); err != nil {
					return err
				}
			}
			rule, err = txRouting.Get(ctx, id)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if tulisGalatProviderModel(w, r, err) {
			return
		}
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	if rule == nil {
		httpx.InternalError(w, r)
		return
	}

	// B-468: penekanan audit B-434 hanya menelan no-op bila Update
	// DITELAN (params kosong) DAN ikatan identik — tetapi repo
	// Update memutuskan ErrNoChanges dari KEHADIRAN parameter,
	// bukan nilainya, jadi PUT yang menyebut field dengan nilai
	// yang PERSIS sama dengan yang tersimpan (nama sama, strategi
	// sama) tetap menulis + diaudit sebagai perubahan. Kini
	// perbandingannya atas nilai: audit & invalidasi hanya terjadi
	// bila ada field yang nilainya benar-benar berubah, atau
	// ikatan provider disebut dan berubah. Ikatan yang tidak
	// disebut berarti ikatan tidak berubah (vakum = identik).
	tanpaPerubahan := (req.ProviderIDs == nil || providerIdentik) &&
		(updateDitelan || perubahanRoutingNilaiSama(existing, p))
	if !tanpaPerubahan {
		h.writeAudit(ctx, r, "update", "routing_rule", id, map[string]any{
			"name":          rule.Name,
			"virtual_alias": aliasAkhir,
		})
		h.invalidateRules(ctx)
	}
	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
}

// paramsRoutingMenyebut melaporkan apakah params menyebut SATU PUN
// field (kehadiran, bukan nilai) — pasangan kehadiran dari
// perubahanRoutingNilaiSama untuk keputusan B-501: params kosong
// total bukan no-op yang dilewatkan, melainkan ErrNoChanges yang
// diteruskan sebagai 400 (kontrak B-401/B-434).
func paramsRoutingMenyebut(p upstream.UpdateRoutingRuleParams) bool {
	return p.Name != nil || p.Description.Set || p.Priority != nil ||
		p.MatchModelID.Set || p.MatchAPIKeyID.Set || p.MatchCapabilities != nil ||
		p.Strategy != nil || p.MaxAttempts != nil || p.BackoffMS != nil ||
		p.FailureThreshold != nil || p.OpenDurationMS != nil || p.HalfOpenProbes != nil ||
		p.Enabled != nil || p.Pipeline.Set || p.VirtualAlias.Set
}

// perubahanRoutingNilaiSama melaporkan apakah SETIAP field yang
// disebut params sama nilainya dengan keadaan tersimpan (B-468).
// Field yang tidak disebut selalu "sama" (semantik pembaruan
// parsial). Pipeline dibandingkan sebagai byte mentah: resep yang
// sama secara semantik tetapi tertulis beda urutan kunci terbaca
// "berubah" — arah yang aman (audit berlebih sesekali, tidak
// pernah menelan perubahan nyata).
func perubahanRoutingNilaiSama(tersimpan *upstream.RoutingRule, p upstream.UpdateRoutingRuleParams) bool {
	if tersimpan == nil {
		return false
	}
	if !samaBilaDisebut(p.Name, &tersimpan.Name) ||
		!samaBilaDisebut(p.Priority, &tersimpan.Priority) ||
		!samaBilaDisebut(p.Strategy, &tersimpan.Strategy) ||
		!samaBilaDisebut(p.MaxAttempts, &tersimpan.MaxAttempts) ||
		!samaBilaDisebut(p.BackoffMS, &tersimpan.BackoffMS) ||
		!samaBilaDisebut(p.FailureThreshold, &tersimpan.FailureThreshold) ||
		!samaBilaDisebut(p.OpenDurationMS, &tersimpan.OpenDurationMS) ||
		!samaBilaDisebut(p.HalfOpenProbes, &tersimpan.HalfOpenProbes) ||
		!samaBilaDisebut(p.Enabled, &tersimpan.Enabled) {
		return false
	}
	// Field Opt: tidak Set = biarkan; Set dengan Value nil = Clear
	// (sama hanya bila tersimpan memang sudah kosong); Set dengan
	// nilai = nilai itu harus sama dengan yang tersimpan. optSama
	// menampung ketiganya lewat ptrSama.
	if !optSama(p.Description.Set, p.Description.Value, tersimpan.Description) ||
		!optSamaUUID(p.MatchModelID.Set, p.MatchModelID.Value, tersimpan.MatchModelID) ||
		!optSamaUUID(p.MatchAPIKeyID.Set, p.MatchAPIKeyID.Value, tersimpan.MatchAPIKeyID) ||
		!optSama(p.VirtualAlias.Set, p.VirtualAlias.Value, tersimpan.VirtualAlias) {
		return false
	}
	// B-502: match_capabilities ber-semantik himpunan di runtime,
	// jadi perbandingannya pun himpunan — urutan berbeda dengan isi
	// sama BUKAN perubahan.
	if p.MatchCapabilities != nil && !himpunanStringSama(tersimpan.MatchCapabilities, p.MatchCapabilities) {
		return false
	}
	if p.Pipeline.Set {
		var nilaiBaru []byte
		if p.Pipeline.Value != nil {
			nilaiBaru = *p.Pipeline.Value
		}
		if !bytes.Equal(tersimpan.Pipeline, nilaiBaru) {
			return false
		}
	}
	return true
}

func (h *Handlers) deleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.routingRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	h.writeAudit(ctx, r, "delete", "routing_rule", id, nil)
	h.invalidateRules(ctx)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	// B-469: toggle ke keadaan yang SUDAH berlaku adalah no-op —
	// jawab keadaan tersimpan tanpa audit "toggle" dan tanpa
	// invalidasi. Muatan gagal (aturan tidak ada) dibiarkan jatuh
	// ke SetEnabled yang menjawab errornya seperti biasa.
	if ada, err := h.routingRepo.Get(ctx, id); err == nil && ada != nil && ada.Enabled == enabled {
		_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(ada))
		return
	}

	rule, err := h.routingRepo.SetEnabled(ctx, id, enabled)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	h.writeAudit(ctx, r, "toggle", "routing_rule", id, map[string]any{"enabled": enabled})
	h.invalidateRules(ctx)
	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
}

// ptrSama membandingkan dua penunjuk nilai: dua-duanya nil, atau
// dua-duanya menunjuk nilai yang sama (B-469).
func ptrSama[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// samaBilaDisebut adalah perbandingan nilai-sama untuk field
// pembaruan parsial (B-469): field yang TIDAK disebut (penunjuk
// nil) selalu "sama"; field yang disebut harus sama nilainya
// dengan yang tersimpan.
func samaBilaDisebut[T comparable](diminta, tersimpan *T) bool {
	return diminta == nil || ptrSama(diminta, tersimpan)
}

// ptrSamaUUID adalah ptrSama untuk field UUID tunggal (B-499):
// kolom uuid Postgres selalu menyimpan ejaan kanonik huruf kecil,
// jadi ejaan kapital dari UUID yang sama adalah NILAI yang sama —
// perbandingan teks mentah membacanya "berubah" dan memicu audit,
// tulisan, bahkan siraman cache untuk no-op representasi. B-467
// menutup kaki himpunan/ikatan; varian ini menutup kaki field
// tunggalnya.
func ptrSamaUUID(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return strings.EqualFold(*a, *b)
}

// optSamaUUID adalah optSama untuk field Opt berisi UUID (B-499):
// tidak Set selalu "sama"; Clear (Value nil) sama hanya bila
// tersimpan juga kosong; Set-nilai dibandingkan tak peka huruf.
func optSamaUUID(disebut bool, nilai, tersimpan *string) bool {
	return !disebut || ptrSamaUUID(nilai, tersimpan)
}

// samaBilaDisebutUUID adalah samaBilaDisebut untuk field UUID
// tunggal berpola "string kosong berarti kosongkan" (B-499): field
// tak disebut selalu "sama"; "" (Clear) sama hanya bila tersimpan
// memang kosong; UUID disebut dibandingkan tak peka huruf.
func samaBilaDisebutUUID(diminta, tersimpan *string) bool {
	if diminta == nil {
		return true
	}
	if *diminta == "" {
		return tersimpan == nil || *tersimpan == ""
	}
	return tersimpan != nil && strings.EqualFold(*diminta, *tersimpan)
}

// himpunanStringSama membandingkan dua daftar string sebagai
// HIMPUNAN (B-502): urutan dan duplikat tidak bermakna. Semantik
// runtime daftar seperti match_capabilities memang himpunan
// (aturan cocok bila seluruh syarat termuat di permintaan), jadi
// perbandingan terurut membaca no-op representasi sebagai
// perubahan.
func himpunanStringSama(a, b []string) bool {
	sa := make(map[string]struct{}, len(a))
	for _, s := range a {
		sa[s] = struct{}{}
	}
	sb := make(map[string]struct{}, len(b))
	for _, s := range b {
		sb[s] = struct{}{}
	}
	if len(sa) != len(sb) {
		return false
	}
	for s := range sa {
		if _, ok := sb[s]; !ok {
			return false
		}
	}
	return true
}

// optSama adalah samaBilaDisebut untuk field berbalut Opt[repo]:
// Opt yang tidak Set selalu "sama"; yang Set dibandingkan nilainya.
func optSama[T comparable](disebut bool, nilai, tersimpan *T) bool {
	return !disebut || ptrSama(nilai, tersimpan)
}

// jsonSamaSemantik membandingkan dua dokumen JSON secara semantik
// (B-469): urutan kunci objek & format angka yang setara tidak
// dianggap perbedaan. Dokumen yang tidak terurai tidak pernah
// "sama" kecuali byte-nya identik — arah aman.
//
// B-495: penguraian memakai json.Decoder UseNumber dan angka
// dibandingkan sebagai nilai rasional EKSAK (math/big.Rat), bukan
// float64. Versi float64 membaca 9007199254740993 dan ...992 sebagai
// nilai yang sama (keduanya runtuh ke float64 yang sama), sehingga
// perubahan nyata pada integer besar di setting/metadata dinyatakan
// "tidak berubah" dan tidak pernah tertulis — kelas B-478 yang
// masuk kembali lewat helper ini. Format yang setara secara nilai
// (1.50 vs 1.5, 1e2 vs 100) tetap "sama" karena Rat menormalkan
// keduanya ke nilai rasional yang sama.
func jsonSamaSemantik(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	va, okA := uraiJSONEksak(a)
	vb, okB := uraiJSONEksak(b)
	return okA && okB && nilaiJSONSama(va, vb)
}

// uraiJSONEksak mengurai SATU dokumen JSON dengan angka sebagai
// json.Number (UseNumber); data susulan sesudah dokumen pertama
// membuat penguraian gagal, sejajar json.Unmarshal.
func uraiJSONEksak(doc []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if dec.More() {
		return nil, false
	}
	return v, true
}

// nilaiJSONSama membandingkan dua nilai hasil uraiJSONEksak secara
// rekursif: objek per kunci (urutan tak bermakna), larik per urutan,
// skalar per nilai, angka per nilai rasional eksak.
func nilaiJSONSama(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case json.Number:
		bv, ok := b.(json.Number)
		return ok && angkaJSONSama(av, bv)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !nilaiJSONSama(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, va := range av {
			vb, ada := bv[k]
			if !ada || !nilaiJSONSama(va, vb) {
				return false
			}
		}
		return true
	}
	return false
}

// angkaJSONSama membandingkan dua literal angka JSON menurut NILAI
// rasionalnya yang eksak: 9007199254740993 != 9007199254740992,
// tetapi 1.50 == 1.5 dan 1e2 == 100.
func angkaJSONSama(a, b json.Number) bool {
	if a == b {
		return true
	}
	ra, okA := new(big.Rat).SetString(a.String())
	rb, okB := new(big.Rat).SetString(b.String())
	return okA && okB && ra.Cmp(rb) == 0
}

// ikatanProviderIdentik melaporkan apakah daftar provider yang
// diminta PERSIS sama dengan ikatan yang tersimpan (B-434):
// urutan sama (urutan adalah posisi fallback yang disimpan
// SetProviders) dan bobot sama per provider (absen di peta bobot
// permintaan berarti bobot NULL, sama seperti absen di sisi
// tersimpan).
//
// B-467: perbandingannya TIDAK peka huruf. UUID yang tersimpan
// selalu huruf kecil (keluaran Postgres), tetapi klien boleh
// mengirim ejaan huruf besar untuk UUID yang sama — versi lama
// membandingkannya sebagai teks mentah, jadi ikatan yang secara
// semantik identik terbaca "berubah": audit update naik dan,
// lebih buruk, SetProviders yang menyusul mencocokkan bobot
// dengan kunci teks mentah itu juga (lihat SetProviders).
func ikatanProviderIdentik(tersimpan []upstream.RuleProvider, diminta []string, bobot map[string]int) bool {
	if len(tersimpan) != len(diminta) {
		return false
	}
	bobotNorm := make(map[string]int, len(bobot))
	for k, v := range bobot {
		bobotNorm[strings.ToLower(k)] = v
	}
	for i, rp := range tersimpan {
		if !strings.EqualFold(rp.ProviderID, diminta[i]) {
			return false
		}
		b, ada := bobotNorm[strings.ToLower(diminta[i])]
		switch {
		case ada && rp.Weight == nil:
			return false
		case !ada && rp.Weight != nil:
			return false
		case ada && rp.Weight != nil && *rp.Weight != b:
			return false
		}
	}
	return true
}

func (h *Handlers) setRuleProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		ProviderIDs []string       `json:"provider_ids"`
		Weights     map[string]int `json:"weights"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// V-FF-11: aturan yang punya model target hanya boleh diikat ke
	// provider yang memetakan model itu.
	aturan, err := h.routingRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	// B-184: ikatan provider tidak berlaku pada aturan ber-pipeline —
	// mesin mengabaikannya di jalur combo. Tolak di jalur pasang provider
	// juga, supaya drawer "Edit Penyedia & Bobot" tidak menyimpan ikatan
	// mati. (Melepaskan ikatan dengan daftar kosong tetap dibolehkan.)
	if g := tolakIkatanProviderCombo(aturan.Pipeline, req.ProviderIDs); g != nil {
		httpx.BadRequest(w, r, g.Kode, g.Error())
		return
	}
	if aturan.MatchModelID != nil && len(req.ProviderIDs) > 0 {
		if err := periksaIkatanProvider(ctx, h.routingRepo, *aturan.MatchModelID, req.ProviderIDs); err != nil {
			if tulisGalatProviderModel(w, r, err) {
				return
			}
			mapRepoError(w, r, err, "provider aturan routing")
			return
		}
	}

	// B-434: ikatan yang IDENTIK dengan yang tersimpan (urutan +
	// bobot — urutan daftar adalah posisi fallback, jadi
	// perbandingannya terurut, bukan himpunan) adalah no-op:
	// tanpa DELETE+INSERT, tanpa audit "set_providers", tanpa
	// invalidasi cache aturan; respons tetap 200 dengan keadaan
	// tersimpan.
	if ikatanProviderIdentik(aturan.Providers, req.ProviderIDs, req.Weights) {
		_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(aturan))
		return
	}

	// SetRuleProviders (pembungkus bertransaksi), BUKAN metode repo di atas
	// pool: SetProviders menolak berjalan di luar transaksi (ErrNeedsTx),
	// sehingga pemanggilan langsung versi lama membuat endpoint ini selalu
	// menjawab 500 untuk ikatan yang sah sekalipun — drawer "Edit Penyedia
	// & Bobot" tidak pernah berhasil menyimpan. Terbukti oleh
	// TestIkatanProviderWajibMemetakanModel (audit3 grup 13).
	if err := upstream.SetRuleProviders(ctx, h.pool, id, req.ProviderIDs, req.Weights); err != nil {
		mapRepoError(w, r, err, "provider aturan routing")
		return
	}

	// Baca-ulang yang gagal = 500 (audit F5): versi lama membuang error
	// lalu menjawab 200 dengan DTO kosong, seolah aturan baru saja kosong.
	rule, err := h.routingRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	h.writeAudit(ctx, r, "set_providers", "routing_rule", id, map[string]any{"providers": req.ProviderIDs})
	h.invalidateRules(ctx)
	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
}

// -----------------------------------------------------------------------------
// Rate Limits Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listRateLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.policyRepo.ListRateLimits(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	dtos := make([]RateLimitDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toRateLimitDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[RateLimitDTO]{
		Items:      dtos,
		NextCursor: next,
	})
}

func (h *Handlers) getRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	l, err := h.policyRepo.GetRateLimit(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toRateLimitDTO(l))
}

type createRateLimitReq struct {
	Scope               string `json:"scope"`
	ScopeID             string `json:"scope_id"`
	RequestsPerSecond   *int   `json:"requests_per_second"`
	RequestsPerMinute   *int   `json:"requests_per_minute"`
	TokensPerMinute     *int   `json:"tokens_per_minute"`
	DailyRequestLimit   *int64 `json:"daily_request_limit"`
	MonthlyRequestLimit *int64 `json:"monthly_request_limit"`
	DailyTokenLimit     *int64 `json:"daily_token_limit"`
	MonthlyTokenLimit   *int64 `json:"monthly_token_limit"`
}

func (h *Handlers) createRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createRateLimitReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// Semua cakupan diselesaikan di batas tulis — lihat
	// validasiScopeKebijakan (B-196; cakupan model lewat
	// normalisasiScopeModel, audit3 V-AB-03).
	scopeID, ok := h.selesaikanScopeID(w, r, req.Scope, req.ScopeID)
	if !ok {
		return
	}
	req.ScopeID = scopeID

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	l, err := h.policyRepo.CreateRateLimit(ctx, policy.CreateRateLimitParams{
		Scope:               req.Scope,
		ScopeID:             req.ScopeID,
		RequestsPerSecond:   req.RequestsPerSecond,
		RequestsPerMinute:   req.RequestsPerMinute,
		TokensPerMinute:     req.TokensPerMinute,
		DailyRequestLimit:   req.DailyRequestLimit,
		MonthlyRequestLimit: req.MonthlyRequestLimit,
		DailyTokenLimit:     req.DailyTokenLimit,
		MonthlyTokenLimit:   req.MonthlyTokenLimit,
		CreatedBy:           actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "create", "rate_limit", l.ID, map[string]any{"scope": l.Scope, "scope_id": l.ScopeID})
	h.invalidateRateLimits()
	_ = h.respond(w, r, http.StatusCreated, toRateLimitDTO(l))
}

func (h *Handlers) updateRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		RequestsPerSecond   *int   `json:"requests_per_second"`
		RequestsPerMinute   *int   `json:"requests_per_minute"`
		TokensPerMinute     *int   `json:"tokens_per_minute"`
		DailyRequestLimit   *int64 `json:"daily_request_limit"`
		MonthlyRequestLimit *int64 `json:"monthly_request_limit"`
		DailyTokenLimit     *int64 `json:"daily_token_limit"`
		MonthlyTokenLimit   *int64 `json:"monthly_token_limit"`
		Enabled             *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// B-469: field yang disebut dengan nilai yang PERSIS sama
	// dengan yang tersimpan bukan perubahan — tolak no_changes
	// sejajar params kosong (yang sudah ditolak repo), supaya
	// audit "update" & invalidasi tidak naik untuk no-op.
	if req.RequestsPerSecond != nil || req.RequestsPerMinute != nil || req.TokensPerMinute != nil ||
		req.DailyRequestLimit != nil || req.MonthlyRequestLimit != nil ||
		req.DailyTokenLimit != nil || req.MonthlyTokenLimit != nil || req.Enabled != nil {
		ada, err := h.policyRepo.GetRateLimit(ctx, id)
		if err != nil {
			mapRepoError(w, r, err, "batas laju")
			return
		}
		if samaBilaDisebut(req.RequestsPerSecond, ada.RequestsPerSecond) &&
			samaBilaDisebut(req.RequestsPerMinute, ada.RequestsPerMinute) &&
			samaBilaDisebut(req.TokensPerMinute, ada.TokensPerMinute) &&
			samaBilaDisebut(req.DailyRequestLimit, ada.DailyRequestLimit) &&
			samaBilaDisebut(req.MonthlyRequestLimit, ada.MonthlyRequestLimit) &&
			samaBilaDisebut(req.DailyTokenLimit, ada.DailyTokenLimit) &&
			samaBilaDisebut(req.MonthlyTokenLimit, ada.MonthlyTokenLimit) &&
			samaBilaDisebut(req.Enabled, &ada.Enabled) {
			mapRepoError(w, r, policy.ErrNoChanges, "batas laju")
			return
		}
	}

	l, err := h.policyRepo.UpdateRateLimit(ctx, id, policy.UpdateRateLimitParams{
		RequestsPerSecond:   req.RequestsPerSecond,
		RequestsPerMinute:   req.RequestsPerMinute,
		TokensPerMinute:     req.TokensPerMinute,
		DailyRequestLimit:   req.DailyRequestLimit,
		MonthlyRequestLimit: req.MonthlyRequestLimit,
		DailyTokenLimit:     req.DailyTokenLimit,
		MonthlyTokenLimit:   req.MonthlyTokenLimit,
		Enabled:             req.Enabled,
	})
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "update", "rate_limit", id, nil)
	h.invalidateRateLimits()
	_ = h.respond(w, r, http.StatusOK, toRateLimitDTO(l))
}

func (h *Handlers) deleteRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteRateLimit(ctx, id); err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "delete", "rate_limit", id, nil)
	h.invalidateRateLimits()
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	// B-469: toggle nilai-sama adalah no-op (lihat toggleRoutingRule).
	if ada, err := h.policyRepo.GetRateLimit(ctx, id); err == nil && ada != nil && ada.Enabled == enabled {
		_ = h.respond(w, r, http.StatusOK, toRateLimitDTO(ada))
		return
	}

	l, err := h.policyRepo.SetRateLimitEnabled(ctx, id, enabled)
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "toggle", "rate_limit", id, map[string]any{"enabled": enabled})
	h.invalidateRateLimits()
	_ = h.respond(w, r, http.StatusOK, toRateLimitDTO(l))
}

// -----------------------------------------------------------------------------
// Budgets Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listBudgets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.policyRepo.ListBudgets(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	dtos := make([]BudgetDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toBudgetDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[BudgetDTO]{
		Items:      dtos,
		NextCursor: next,
	})
}

func (h *Handlers) getBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	b, err := h.policyRepo.GetBudget(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
}

// normalisasiScopeModel mengubah scope_id cakupan model menjadi UUID model kanonik.
//
// Jalur request (target gateway, mesin batas laju, AddSpend, CalculateSpend) SELALU
// memakai UUID model, sedangkan yang wajar diketik/dipilih operator adalah nama atau
// alias model. Tanpa normalisasi di batas tulis ini, baris anggaran/batas laju scope
// model menyimpan nama dan tidak pernah bertemu target UUID — aturan itu lahir mati
// (audit3 V-AB-02, V-AB-03). Resolve menerima nama kanonik, alias, maupun UUID itu
// sendiri, jadi nilai yang sudah benar lolos apa adanya.
func (h *Handlers) normalisasiScopeModel(ctx context.Context, scope, scopeID string) (string, error) {
	if scope != policy.ScopeModel || scopeID == "" || h.modelRepo == nil {
		return scopeID, nil
	}
	m, err := h.modelRepo.Resolve(ctx, scopeID)
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

// errScopeTakSah menandai scope_id yang bentuknya sendiri tidak sah untuk
// cakupannya (UUID acak untuk api_key, teks bukan alamat untuk ip) —
// berbeda dari repo.ErrNotFound yang berarti bentuknya sah tetapi
// entitasnya tidak ada.
var errScopeTakSah = errors.New("scope_id tidak sah untuk cakupannya")

// validasiScopeKebijakan menyelesaikan scope_id untuk SEMUA cakupan
// kebijakan (B-196), simetris dengan cara penegak mencocokkannya di
// jalur request.
//
// Sebelumnya hanya cakupan model yang dinormalisasi; api_key/user/
// provider/ip lolos mentah dari body. Akibatnya aturan yang scope_id-nya
// bukan UUID entitas sungguhan (atau IP tak kanonik) lahir mati — penegak
// mencocokkan persis terhadap UUID/IP kanonik sehingga tidak pernah
// bertemu — dan pelanggarannya baru muncul sebagai 400 generik dari
// constraint database, atau tidak muncul sama sekali. Kini setiap
// cakupan diselesaikan di batas tulis yang sama:
//
//   - global: scope_id dipaksa kosong (constraint presence).
//   - model: resolusi nama/alias → UUID lewat normalisasiScopeModel.
//   - api_key/user/provider: entitasnya wajib ada (ErrNotFound → 400).
//     Bentuk UUID diperiksa lebih dulu untuk api_key karena repo
//     kuncinya meneruskan string apa pun ke pgx.
//   - ip: diuraikan dan disimpan dalam bentuk kanonik netip, persis
//     bentuk yang dibandingkan penegak.
//   - cakupan tak dikenal: diteruskan apa adanya — constraint
//     *_scope_valid menolaknya dan mapRepoError menyebut field-nya.
func (h *Handlers) validasiScopeKebijakan(ctx context.Context, scope, scopeID string) (string, error) {
	switch scope {
	case policy.ScopeGlobal:
		return "", nil
	case policy.ScopeModel:
		return h.normalisasiScopeModel(ctx, scope, scopeID)
	case policy.ScopeAPIKey:
		if !bentukUUID(scopeID) {
			return "", errScopeTakSah
		}
		if h.keyRepo != nil {
			if _, err := h.keyRepo.GetByID(ctx, scopeID); err != nil {
				return "", err
			}
		}
		return scopeID, nil
	case policy.ScopeUser:
		if h.usersRepo != nil {
			if _, err := h.usersRepo.GetByID(ctx, scopeID); err != nil {
				return "", err
			}
		}
		return scopeID, nil
	case policy.ScopeProvider:
		if h.providerRepo != nil {
			if _, err := h.providerRepo.Get(ctx, scopeID); err != nil {
				return "", err
			}
		}
		return scopeID, nil
	case policy.ScopeIP:
		addr, err := netip.ParseAddr(scopeID)
		if err != nil {
			return "", errScopeTakSah
		}
		return addr.String(), nil
	default:
		return scopeID, nil
	}
}

// selesaikanScopeID menjalankan validasiScopeKebijakan dan menulis
// respons 400 yang menyebut field-nya bila gagal; ok=false berarti
// respons sudah ditulis dan pemanggil wajib berhenti.
func (h *Handlers) selesaikanScopeID(w http.ResponseWriter, r *http.Request, scope, scopeID string) (string, bool) {
	hasil, err := h.validasiScopeKebijakan(r.Context(), scope, scopeID)
	if err == nil {
		return hasil, true
	}
	switch {
	case errors.Is(err, repo.ErrNotFound):
		httpx.BadRequest(w, r, "invalid_scope_id",
			"scope_id tidak menunjuk "+subjekScope(scope)+" yang dikenal: "+scopeID)
	case errors.Is(err, errScopeTakSah):
		httpx.BadRequest(w, r, "invalid_scope_id",
			"scope_id tidak sah untuk cakupan "+scope+": "+scopeID)
	default:
		mapRepoError(w, r, err, "kebijakan")
	}
	return "", false
}

// subjekScope adalah nama manusiawi entitas yang ditunjuk scope_id,
// untuk pesan penolakan di atas.
func subjekScope(scope string) string {
	switch scope {
	case policy.ScopeModel:
		return "model"
	case policy.ScopeAPIKey:
		return "kunci API"
	case policy.ScopeUser:
		return "pengguna"
	case policy.ScopeProvider:
		return "provider"
	default:
		return "entitas"
	}
}

// bentukUUID memeriksa bentuk tekstual UUID (8-4-4-4-12 heksadesimal).
// Pemeriksaan bentuk, bukan keberadaan: dipakai sebelum menyerahkan
// string ke repo yang tidak menjaganya sendiri, agar UUID acak
// membalas 400 yang jelas alih-alih 500 dari pgx.
func bentukUUID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

type createBudgetReq struct {
	Name              string `json:"name"`
	Scope             string `json:"scope"`
	ScopeID           string `json:"scope_id"`
	Period            string `json:"period"`
	LimitUSD          string `json:"limit_usd"`
	MaxSpendUSD       string `json:"max_spend_usd"`
	ActionOnExceed    string `json:"action_on_exceed"`
	Action            string `json:"action"`
	AlertThresholdPct int    `json:"alert_threshold_pct"`
	AlertThreshold    int    `json:"alert_threshold"`
}

func (h *Handlers) createBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createBudgetReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.LimitUSD == "" && req.MaxSpendUSD != "" {
		req.LimitUSD = req.MaxSpendUSD
	}
	if req.ActionOnExceed == "" && req.Action != "" {
		req.ActionOnExceed = req.Action
	}
	if req.AlertThresholdPct == 0 && req.AlertThreshold != 0 {
		req.AlertThresholdPct = req.AlertThreshold
	}

	// Semua cakupan diselesaikan di batas tulis — lihat
	// validasiScopeKebijakan (B-196). Cakupan global termasuk di
	// dalamnya: scope_id-nya dipaksa kosong sesuai constraint
	// presence database, seperti yang dulu dilakukan manual di sini.
	scopeID, ok := h.selesaikanScopeID(w, r, req.Scope, req.ScopeID)
	if !ok {
		return
	}
	req.ScopeID = scopeID

	limitUSD, err := upstream.ParseUSD(req.LimitUSD)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_amount", "limit_usd tidak sah: "+err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	b, err := h.policyRepo.CreateBudget(ctx, policy.CreateBudgetParams{
		Name:              req.Name,
		Scope:             req.Scope,
		ScopeID:           req.ScopeID,
		Period:            req.Period,
		LimitUSD:          limitUSD,
		ActionOnExceed:    req.ActionOnExceed,
		AlertThresholdPct: req.AlertThresholdPct,
		CreatedBy:         actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "create", "budget", b.ID, map[string]any{"name": b.Name, "limit_usd": limitUSD.String()})
	h.invalidateBudgets()
	_ = h.respond(w, r, http.StatusCreated, toBudgetDTO(b))
}

func (h *Handlers) updateBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name              *string `json:"name"`
		LimitUSD          *string `json:"limit_usd"`
		MaxSpendUSD       *string `json:"max_spend_usd"`
		ActionOnExceed    *string `json:"action_on_exceed"`
		Action            *string `json:"action"`
		AlertThresholdPct *int    `json:"alert_threshold_pct"`
		AlertThreshold    *int    `json:"alert_threshold"`
		Enabled           *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.LimitUSD == nil && req.MaxSpendUSD != nil {
		req.LimitUSD = req.MaxSpendUSD
	}
	if req.ActionOnExceed == nil && req.Action != nil {
		req.ActionOnExceed = req.Action
	}
	if req.AlertThresholdPct == nil && req.AlertThreshold != nil {
		req.AlertThresholdPct = req.AlertThreshold
	}

	var lim *upstream.USD
	if req.LimitUSD != nil {
		parsed, err := upstream.ParseUSD(*req.LimitUSD)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_amount", "limit_usd tidak sah: "+err.Error())
			return
		}
		lim = &parsed
	}

	// B-469: nilai-sama bukan perubahan (lihat updateRateLimit).
	if req.Name != nil || lim != nil || req.ActionOnExceed != nil ||
		req.AlertThresholdPct != nil || req.Enabled != nil {
		ada, err := h.policyRepo.GetBudget(ctx, id)
		if err != nil {
			mapRepoError(w, r, err, "anggaran")
			return
		}
		if samaBilaDisebut(req.Name, &ada.Name) &&
			samaBilaDisebut(lim, &ada.LimitUSD) &&
			samaBilaDisebut(req.ActionOnExceed, &ada.ActionOnExceed) &&
			samaBilaDisebut(req.AlertThresholdPct, &ada.AlertThresholdPct) &&
			samaBilaDisebut(req.Enabled, &ada.Enabled) {
			mapRepoError(w, r, policy.ErrNoChanges, "anggaran")
			return
		}
	}

	b, err := h.policyRepo.UpdateBudget(ctx, id, policy.UpdateBudgetParams{
		Name:              req.Name,
		LimitUSD:          lim,
		ActionOnExceed:    req.ActionOnExceed,
		AlertThresholdPct: req.AlertThresholdPct,
		Enabled:           req.Enabled,
	})
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "update", "budget", id, map[string]any{"name": b.Name})
	h.invalidateBudgets()
	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
}

func (h *Handlers) deleteBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteBudget(ctx, id); err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "delete", "budget", id, nil)
	h.invalidateBudgets()
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	// B-469: toggle nilai-sama adalah no-op (lihat toggleRoutingRule).
	if ada, err := h.policyRepo.GetBudget(ctx, id); err == nil && ada != nil && ada.Enabled == enabled {
		_ = h.respond(w, r, http.StatusOK, toBudgetDTO(ada))
		return
	}

	b, err := h.policyRepo.SetBudgetEnabled(ctx, id, enabled)
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "toggle", "budget", id, map[string]any{"enabled": enabled})
	h.invalidateBudgets()
	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
}

func (h *Handlers) resetBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Periode baru WAJIB punya batas akhir (audit3 V-AB-01): reset manual yang
	// menulis period_end NULL membuat anggaran kebal terhadap BudgetResetterJob
	// selamanya — sama seperti bug pembuatannya. Batasnya dihitung dari periode
	// anggaran itu sendiri; hanya periode 'total' yang sah tanpa batas.
	ada, err := h.policyRepo.GetBudget(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "reset anggaran")
		return
	}

	now := time.Now().UTC()
	var end *time.Time
	if e, ok := policy.AkhirPeriodeDi(ada.Period, now, h.budgetTimezone); ok {
		end = &e
	}
	b, err := h.policyRepo.ResetPeriod(ctx, id, now, end)
	if err != nil {
		mapRepoError(w, r, err, "reset anggaran")
		return
	}

	h.writeAudit(ctx, r, "reset", "budget", id, nil)
	h.invalidateBudgets()
	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
}

// -----------------------------------------------------------------------------
// Bans Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listBans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit: repo.DefaultPageLimit,
	}

	items, err := h.policyRepo.ActiveBans(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "pemblokiran")
		return
	}

	dtos := make([]BanDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toBanDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[BanDTO]{
		Items: dtos,
	})
}

type createBanReq struct {
	SubjectKind string  `json:"subject_kind"`
	Subject     string  `json:"subject"`
	Reason      string  `json:"reason"`
	ExpiresAt   *string `json:"expires_at"`
}

func (h *Handlers) createBan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createBanReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_expires_at", fmt.Sprintf("format expires_at tidak sah (harus RFC3339): %v", err))
			return
		}
		// Ban yang lahir sudah kedaluwarsa tidak pernah berlaku sedetik
		// pun — tolak seperti updateAPIKey menolak expires_at lampau
		// (B-192). Repo sengaja tetap mengizinkannya untuk kebutuhan
		// pencatatan/penyaringan riwayat; permukaan admin tidak.
		if t.Before(time.Now()) {
			httpx.BadRequest(w, r, "expires_in_past", "expires_at tidak boleh di masa lalu")
			return
		}
		exp = &t
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	ban, err := h.policyRepo.CreateBan(ctx, policy.CreateBanParams{
		SubjectKind: req.SubjectKind,
		Subject:     req.Subject,
		Reason:      req.Reason,
		ExpiresAt:   exp,
		CreatedBy:   actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "pemblokiran")
		return
	}

	h.writeAudit(ctx, r, "create", "ban", ban.ID, map[string]any{"kind": ban.SubjectKind, "subject": ban.Subject})
	// Produser event ban.created (audit3 V-WH-08): fitur pemblokiran memang
	// ada (policy.CreateBan), hanya produser webhook-nya yang belum terpasang.
	h.pancarkanEvent(ctx, "ban.created", map[string]any{
		"ban_id":       ban.ID,
		"subject_kind": ban.SubjectKind,
		"subject":      ban.Subject,
	})
	_ = h.respond(w, r, http.StatusCreated, toBanDTO(ban))
}

func (h *Handlers) liftBan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	ban, err := h.policyRepo.Lift(ctx, id, actorID)
	if err != nil {
		mapRepoError(w, r, err, "pemblokiran")
		return
	}

	h.writeAudit(ctx, r, "lift", "ban", id, nil)
	_ = h.respond(w, r, http.StatusOK, toBanDTO(ban))
}

// -----------------------------------------------------------------------------
// Content Filters Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listContentFilters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.policyRepo.ListFilters(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	dtos := make([]ContentFilterDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toContentFilterDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[ContentFilterDTO]{
		Items:      dtos,
		NextCursor: next,
	})
}

func (h *Handlers) getContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	f, err := h.policyRepo.GetFilter(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(f))
}

type createFilterReq struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Kind            string `json:"kind"`
	Priority        int    `json:"priority"`
	AppliesTo       string `json:"applies_to"`
	Action          string `json:"action"`
	Pattern         string `json:"pattern"`
	PatternType     string `json:"pattern_type"`
	CaseSensitive   bool   `json:"case_sensitive"`
	MaxEvalMS       int    `json:"max_eval_ms"`
	ModelID         string `json:"model_id"`
	ProviderID      string `json:"provider_id"`
	MaxRequestBytes *int64 `json:"max_request_bytes"`
	Enabled         *bool  `json:"enabled"`
}

func (h *Handlers) createContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createFilterReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	f, err := h.policyRepo.CreateFilter(ctx, policy.CreateFilterParams{
		Name:            req.Name,
		Description:     req.Description,
		Kind:            req.Kind,
		Priority:        req.Priority,
		AppliesTo:       req.AppliesTo,
		Action:          req.Action,
		Pattern:         req.Pattern,
		PatternType:     req.PatternType,
		CaseSensitive:   req.CaseSensitive,
		MaxEvalMS:       req.MaxEvalMS,
		ModelID:         req.ModelID,
		ProviderID:      req.ProviderID,
		MaxRequestBytes: req.MaxRequestBytes,
		CreatedBy:       actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "create", "content_filter", f.ID, map[string]any{"name": f.Name, "kind": f.Kind})
	h.invalidateContentFilters()
	_ = h.respond(w, r, http.StatusCreated, toContentFilterDTO(f))
}

func (h *Handlers) updateContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// model_id/provider_id dibaca sebagai penunjuk lewat struct pembungkus
	// (audit3 V-DT-25): pada DTO biasa, "" tak bisa dibedakan dari "field tidak
	// dikirim", sehingga mengosongkan cakupan filter mustahil lewat API — repo
	// sudah memetakan "" menjadi NULL, handler wajib meneruskannya apa adanya.
	// Field pembungkus yang lebih dangkal menaungi field DTO untuk dua kunci
	// JSON itu; sisa field tetap dari createFilterReq.
	//
	// case_sensitive ikut dibaca sebagai penunjuk dengan alasan yang sama
	// (B-370): pada bool polos, "tidak dikirim" terurai menjadi false dan
	// handler lama memasangnya tanpa syarat, jadi pembaruan nama-saja
	// menimpa kepekaan huruf filter secara senyap.
	var req struct {
		createFilterReq
		ModelID       *string `json:"model_id"`
		ProviderID    *string `json:"provider_id"`
		CaseSensitive *bool   `json:"case_sensitive"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// KEPUTUSAN TERCATAT (B-418): beberapa field di sini tidak
	// bisa dikosongkan/di-nol-kan lewat update — description dan
	// pattern dijaga != "", priority dan max_eval_ms dijaga > 0.
	// Ini batas ekspresif yang SADAR, bukan kelalaian: DTO memakai
	// nilai polos (bukan penunjuk/Set-Clear) karena untuk field
	// ini "absen" dan "kosong" sengaja dilebur menjadi "biarkan",
	// harga dari pola anti-B-369 (update parsial tidak pernah
	// menghapus yang tidak disebut). Mengosongkan description
	// atau pattern bukan kebutuhan operasional yang nyata
	// (pattern kosong = filter tanpa pola, keadaan yang memang
	// tidak boleh ada), dan jalur menghapus filter tersedia.
	var p policy.UpdateFilterParams
	if req.Name != "" {
		p.Name = &req.Name
	}
	if req.Description != "" {
		p.Description = &req.Description
	}
	if req.Kind != "" {
		p.Kind = &req.Kind
	}
	if req.Priority > 0 {
		p.Priority = &req.Priority
	}
	if req.AppliesTo != "" {
		p.AppliesTo = &req.AppliesTo
	}
	if req.Action != "" {
		p.Action = &req.Action
	}
	if req.Pattern != "" {
		p.Pattern = &req.Pattern
	}
	if req.PatternType != "" {
		p.PatternType = &req.PatternType
	}
	if req.CaseSensitive != nil {
		p.CaseSensitive = req.CaseSensitive
	}
	if req.MaxEvalMS > 0 {
		p.MaxEvalMS = &req.MaxEvalMS
	}
	if req.ModelID != nil {
		p.ModelID = req.ModelID
	}
	if req.ProviderID != nil {
		p.ProviderID = req.ProviderID
	}
	if req.MaxRequestBytes != nil {
		p.MaxRequestBytes = req.MaxRequestBytes
	}
	p.Enabled = req.Enabled

	// B-469: nilai-sama bukan perubahan. Berbeda dari permukaan
	// PUT lain, body endpoint ini berbentuk FORM PENUH (DTO buat
	// yang sama, peleburan B-418) — dashboard selalu mengirim
	// seluruh form saat menyimpan, jadi no-op di sini dijawab
	// 200 diam bergaya endpoint set (preseden B-434), BUKAN 400:
	// tanpa tulisan, tanpa audit, tanpa invalidasi matcher.
	// Params hasil peleburan hanya membawa field yang disebut,
	// jadi yang dibandingkan hanya field yang terbawa params.
	if p.Name != nil || p.Description != nil || p.Kind != nil || p.Priority != nil ||
		p.AppliesTo != nil || p.Action != nil || p.Pattern != nil || p.PatternType != nil ||
		p.CaseSensitive != nil || p.MaxEvalMS != nil || p.ModelID != nil || p.ProviderID != nil ||
		p.MaxRequestBytes != nil || p.Enabled != nil {
		ada, err := h.policyRepo.GetFilter(ctx, id)
		if err != nil {
			mapRepoError(w, r, err, "filter konten")
			return
		}
		if samaBilaDisebut(p.Name, &ada.Name) &&
			samaBilaDisebut(p.Description, &ada.Description) &&
			samaBilaDisebut(p.Kind, &ada.Kind) &&
			samaBilaDisebut(p.Priority, &ada.Priority) &&
			samaBilaDisebut(p.AppliesTo, &ada.AppliesTo) &&
			samaBilaDisebut(p.Action, &ada.Action) &&
			samaBilaDisebut(p.Pattern, &ada.Pattern) &&
			samaBilaDisebut(p.PatternType, &ada.PatternType) &&
			samaBilaDisebut(p.CaseSensitive, &ada.CaseSensitive) &&
			samaBilaDisebut(p.MaxEvalMS, &ada.MaxEvalMS) &&
			samaBilaDisebutUUID(p.ModelID, &ada.ModelID) &&
			samaBilaDisebutUUID(p.ProviderID, &ada.ProviderID) &&
			samaBilaDisebut(p.MaxRequestBytes, ada.MaxRequestBytes) &&
			samaBilaDisebut(p.Enabled, &ada.Enabled) {
			_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(ada))
			return
		}
	}

	f, err := h.policyRepo.UpdateFilter(ctx, id, p)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "update", "content_filter", id, map[string]any{"name": f.Name})
	h.invalidateContentFilters()
	_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(f))
}

func (h *Handlers) deleteContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteFilter(ctx, id); err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "delete", "content_filter", id, nil)
	h.invalidateContentFilters()
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	// B-469: toggle nilai-sama adalah no-op (lihat toggleRoutingRule).
	if ada, err := h.policyRepo.GetFilter(ctx, id); err == nil && ada != nil && ada.Enabled == enabled {
		_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(ada))
		return
	}

	f, err := h.policyRepo.SetFilterEnabled(ctx, id, enabled)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "toggle", "content_filter", id, map[string]any{"enabled": enabled})
	h.invalidateContentFilters()
	_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(f))
}

// -----------------------------------------------------------------------------
// Circuit Breakers Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items := make([]CircuitBreakerDTO, 0)
	if h.redis != nil {
		// Pindai kunci di bawah namespace sirkuit breaker resmi routex:cb:*
		pattern := cache.Key(cache.NamespaceCircuitBreaker) + ":*"
		iter := h.redis.Scan(ctx, 0, pattern, 500).Iterator()
		for iter.Next(ctx) {
			k := iter.Val()
			// Segmen kunci: routex:cb:<provider_id>[:<model>]
			parts := strings.Split(k, ":")
			if len(parts) < 3 {
				continue
			}
			providerID, _ := url.PathUnescape(parts[2])
			model := ""
			if len(parts) >= 4 {
				model, _ = url.PathUnescape(parts[3])
			}

			// Ambil status keputusan state sesungguhnya dari Breaker.State().
			// Bila tidak bisa ditentukan, laporkan "unknown" (audit F5):
			// versi lama membiarkan nilai bawaan "closed" sehingga breaker
			// yang sebenarnya terbuka tampil sehat saat Redis bermasalah.
			//
			// Kunci bersegmen lima adalah varian konfigurasi per-aturan
			// (audit3 B-024): state-nya hanya bisa dijawab dari kuncinya
			// sendiri, karena (provider, model) saja menunjuk breaker
			// konfigurasi global — melaporkan state breaker lain di baris
			// ini akan menampilkan breaker terbuka sebagai tertutup.
			stateStr := "unknown"
			if h.breaker != nil {
				var st gateway.State
				var err error
				if len(parts) >= 5 {
					st, err = h.breaker.StateUntukKunci(ctx, k)
				} else {
					st, err = h.breaker.State(ctx, providerID, model)
				}
				if err == nil {
					stateStr = string(st)
				} else {
					h.logger.WarnContext(ctx, "gagal membaca status circuit breaker", "key", k, "error", err)
				}
			}

			// Ambil metrik kegagalan dan jadwal probe dari Redis hash
			hashData, hashErr := h.redis.HGetAll(ctx, k).Result()
			if hashErr != nil {
				h.logger.WarnContext(ctx, "gagal membaca metrik circuit breaker", "key", k, "error", hashErr)
			}
			var failureCount int64
			for fKey, fVal := range hashData {
				if strings.HasPrefix(fKey, "f") {
					if cnt, err := strconv.ParseInt(fVal, 10, 64); err == nil {
						failureCount += cnt
					}
				}
			}
			// Breaker yang TERBUKA tidak lagi mempunyai ember f*: pencatat
			// menghapus hash saat membuka dan menyisakan open_until saja,
			// jadi jumlah di atas selalu 0 tepat pada breaker yang sedang
			// mengisolasi provider (B-110). Pencatat kini juga menulis
			// open_failures pada transisi membuka — angka itulah jumlah
			// kegagalan pemicunya, dan itulah yang dilaporkan di sini.
			if failureCount == 0 {
				if v, ok := hashData["open_failures"]; ok {
					if cnt, err := strconv.ParseInt(v, 10, 64); err == nil {
						failureCount = cnt
					}
				}
			}

			var nextProbeAt string
			if openUntilStr, ok := hashData["open_until"]; ok {
				if openUntilMs, err := strconv.ParseInt(openUntilStr, 10, 64); err == nil && openUntilMs > 0 {
					nextProbeAt = time.UnixMilli(openUntilMs).Format(time.RFC3339)
				}
			}

			items = append(items, CircuitBreakerDTO{
				Key:          k,
				ProviderID:   providerID,
				Model:        model,
				State:        stateStr,
				FailureCount: failureCount,
				NextProbeAt:  nextProbeAt,
			})
		}
		// Iterator yang gagal di tengah = daftar parsial yang menyesatkan;
		// laporkan sebagai error (audit F5).
		if err := iter.Err(); err != nil {
			h.logger.ErrorContext(ctx, "gagal memindai kunci circuit breaker", "error", err)
			httpx.InternalError(w, r)
			return
		}
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[CircuitBreakerDTO]{
		Items: items,
	})
}

type resetBreakerReq struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

func (h *Handlers) resetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req resetBreakerReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if strings.TrimSpace(req.ProviderID) == "" {
		httpx.BadRequest(w, r, "invalid_request", "provider_id wajib diisi")
		return
	}

	// Kunci Redis SELALU dirakit di sisi server menggunakan helper cache.CircuitBreakerKey.
	// Klien dilarang menyuplai kunci Redis mentah untuk mencegah penghapusan namespace lain
	// (misal routex:rl:apikey:* untuk menghapus rate limit).
	serverKey := cache.CircuitBreakerKey(req.ProviderID, req.Model)

	if h.breaker != nil {
		if err := h.breaker.Reset(ctx, req.ProviderID, req.Model); err != nil {
			httpx.InternalError(w, r)
			return
		}
	} else if h.redis != nil {
		// Error Del diteruskan (audit F5): reset yang gagal tidak boleh
		// dicatat audit sebagai berhasil.
		if err := h.redis.Del(ctx, serverKey).Err(); err != nil {
			h.logger.ErrorContext(ctx, "gagal mereset circuit breaker via redis", "key", serverKey, "error", err)
			httpx.InternalError(w, r)
			return
		}
	} else {
		// Tidak ada backend breaker sama sekali: jangan pura-pura mereset.
		tidakTersedia(w, r, "breaker_unavailable", "Circuit breaker tidak tersedia pada instalasi ini")
		return
	}

	h.writeAudit(ctx, r, "reset", "circuit_breaker", serverKey, map[string]any{
		"provider_id": req.ProviderID,
		"model":       req.Model,
	})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "reset"})
}
