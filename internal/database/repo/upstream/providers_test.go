package upstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Jalur bahagia CRUD provider, termasuk pembaruan sebagian dan penghapusan.
func TestProviderCRUDIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	providers := NewProviderRepo(pool)
	admin := seedUser(ctx, t, pool)

	created, err := providers.Create(ctx, CreateProviderParams{
		Name:         "openai-utama",
		DisplayName:  "OpenAI Utama",
		Kind:         KindOpenAI,
		BaseURL:      "https://api.openai.com/v1",
		Priority:     ptr(10),
		Weight:       ptr(5),
		RateLimitRPM: ptr(3000),
		Metadata:     []byte(`{"catatan":"akun produksi"}`),
		CreatedBy:    &admin,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	switch {
	case !idOK(created.ID):
		t.Errorf("ID = %q, bukan UUID", created.ID)
	case created.Priority != 10 || created.Weight != 5:
		t.Errorf("priority/weight = %d/%d, ingin 10/5", created.Priority, created.Weight)
	case created.TimeoutMS != DefaultTimeoutMS || created.MaxRetries != DefaultMaxRetries:
		t.Errorf("nilai bawaan tidak terpasang: timeout=%d retries=%d", created.TimeoutMS, created.MaxRetries)
	case !created.Enabled:
		t.Error("provider baru seharusnya aktif")
	case created.RateLimitRPM == nil || *created.RateLimitRPM != 3000:
		t.Errorf("rate_limit_rpm = %v, ingin 3000", created.RateLimitRPM)
	case created.RateLimitTPM != nil:
		t.Errorf("rate_limit_tpm = %v, ingin nil", created.RateLimitTPM)
	case string(created.Metadata) != `{"catatan": "akun produksi"}`:
		t.Errorf("metadata = %s", created.Metadata)
	case created.CreatedBy == nil || *created.CreatedBy != admin:
		t.Errorf("created_by = %v, ingin %s", created.CreatedBy, admin)
	}

	t.Run("get dan get by name mengembalikan baris yang sama", func(t *testing.T) {
		byID, err := providers.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		byName, err := providers.GetByName(ctx, created.Name)
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		if byID.ID != byName.ID {
			t.Errorf("Get dan GetByName mengembalikan baris berbeda: %s vs %s", byID.ID, byName.ID)
		}
	})

	t.Run("nama ganda menjadi ErrConflict", func(t *testing.T) {
		_, err := providers.Create(ctx, CreateProviderParams{
			Name: created.Name, DisplayName: "Kembar", Kind: KindOpenAI,
			BaseURL: "https://api.openai.com/v1",
		})
		if !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("Create nama ganda = %v, ingin ErrConflict", err)
		}
		if !strings.Contains(err.Error(), "providers_name_key") {
			t.Errorf("pesan tidak menyebut constraint yang dilanggar: %v", err)
		}
	})

	t.Run("referensi egress pool yang tidak ada menjadi ErrInvalidReference", func(t *testing.T) {
		ghostID, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		_, err = providers.Create(ctx, CreateProviderParams{
			Name: "provider-hantu", DisplayName: "Hantu", Kind: KindCustom,
			BaseURL: "https://hantu.contoh.test", EgressPoolID: &ghostID,
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Fatalf("Create dengan egress pool hantu = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("pengenal salah bentuk tidak menjadi kegagalan internal", func(t *testing.T) {
		if _, err := providers.Get(ctx, "bukan-uuid"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get(%q) = %v, ingin ErrNotFound", "bukan-uuid", err)
		}
		_, err := providers.Create(ctx, CreateProviderParams{
			Name: "x", DisplayName: "X", Kind: KindCustom,
			BaseURL: "https://x.test", CreatedBy: ptr("123"),
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("Create dengan created_by salah bentuk = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("update sebagian", func(t *testing.T) {
		updated, err := providers.Update(ctx, created.ID, UpdateProviderParams{
			DisplayName:  ptr("OpenAI Produksi"),
			Priority:     ptr(1),
			RateLimitRPM: Clear[int](),
			RateLimitTPM: Set(120000),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		switch {
		case updated.DisplayName != "OpenAI Produksi":
			t.Errorf("display_name = %q", updated.DisplayName)
		case updated.Priority != 1:
			t.Errorf("priority = %d, ingin 1", updated.Priority)
		case updated.RateLimitRPM != nil:
			t.Errorf("rate_limit_rpm = %v, ingin dikosongkan", updated.RateLimitRPM)
		case updated.RateLimitTPM == nil || *updated.RateLimitTPM != 120000:
			t.Errorf("rate_limit_tpm = %v, ingin 120000", updated.RateLimitTPM)
		case updated.Weight != 5:
			t.Errorf("weight = %d, seharusnya tidak ikut berubah", updated.Weight)
		case !updated.UpdatedAt.After(created.UpdatedAt):
			t.Error("updated_at tidak dimajukan trigger")
		}
	})

	t.Run("update tanpa perubahan ditolak", func(t *testing.T) {
		if _, err := providers.Update(ctx, created.ID, UpdateProviderParams{}); !errors.Is(err, ErrNoChanges) {
			t.Errorf("Update kosong = %v, ingin ErrNoChanges", err)
		}
	})

	t.Run("set enabled", func(t *testing.T) {
		off, err := providers.SetEnabled(ctx, created.ID, false)
		if err != nil {
			t.Fatalf("SetEnabled: %v", err)
		}
		if off.Enabled {
			t.Error("provider masih aktif setelah dimatikan")
		}
	})

	t.Run("delete lalu tidak ditemukan", func(t *testing.T) {
		if err := providers.Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if err := providers.Delete(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Delete kedua = %v, ingin ErrNotFound", err)
		}
		if _, err := providers.Get(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get setelah dihapus = %v, ingin ErrNotFound", err)
		}
	})
}

// Constraint providers_byok_has_owner adalah pemisah antara kredensial milik pengguna dan
// lalu lintas pengguna lain, jadi kedua arah pelanggarannya diuji.
func TestProviderBYOKConstraintIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	providers := NewProviderRepo(pool)
	owner := seedUser(ctx, t, pool)

	base := CreateProviderParams{
		DisplayName: "BYOK Anthropic",
		Kind:        KindAnthropic,
		BaseURL:     "https://api.anthropic.com",
	}

	t.Run("BYOK tanpa pemilik ditolak", func(t *testing.T) {
		p := base
		p.Name = "byok-tanpa-pemilik"
		p.IsBYOK = true

		_, err := providers.Create(ctx, p)
		if !errors.Is(err, repo.ErrConstraint) {
			t.Fatalf("Create = %v, ingin ErrConstraint", err)
		}
		if !strings.Contains(err.Error(), "providers_byok_has_owner") {
			t.Errorf("pesan tidak menyebut constraint: %v", err)
		}
	})

	t.Run("non-BYOK dengan pemilik ditolak", func(t *testing.T) {
		p := base
		p.Name = "bersama-dengan-pemilik"
		p.OwnerUserID = &owner

		_, err := providers.Create(ctx, p)
		if !errors.Is(err, repo.ErrConstraint) {
			t.Fatalf("Create = %v, ingin ErrConstraint", err)
		}
		if !strings.Contains(err.Error(), "providers_byok_has_owner") {
			t.Errorf("pesan tidak menyebut constraint: %v", err)
		}
	})

	t.Run("BYOK dengan pemilik diterima", func(t *testing.T) {
		p := base
		p.Name = "byok-milik-pengguna"
		p.IsBYOK = true
		p.OwnerUserID = &owner

		created, err := providers.Create(ctx, p)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !created.IsBYOK || created.OwnerUserID == nil || *created.OwnerUserID != owner {
			t.Errorf("provider BYOK = %+v", created)
		}
	})
}

// Cuplikan kesehatan dan riwayatnya harus selalu berjalan bersama, termasuk aturan
// penghitung kegagalan berurutan.
func TestProviderHealthIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	providers := NewProviderRepo(pool)
	provider := seedProvider(ctx, t, pool, "provider-kesehatan")

	steps := []struct {
		report        HealthReport
		wantFailures  int
		wantStatus    string
		wantLatencyMS int
	}{
		{HealthReport{Status: HealthHealthy, LatencyMS: ptr(120)}, 0, HealthHealthy, 120},
		{HealthReport{Status: HealthUnhealthy, StatusCode: ptr(503), ErrorKind: "http_5xx",
			ErrorMessage: "upstream membalas 503"}, 1, HealthUnhealthy, 0},
		{HealthReport{Status: HealthUnhealthy, ErrorKind: "timeout"}, 2, HealthUnhealthy, 0},
		// Degraded tidak menaikkan dan tidak mereset: provider yang lambat masih melayani.
		{HealthReport{Status: HealthDegraded, LatencyMS: ptr(4000)}, 2, HealthDegraded, 4000},
		{HealthReport{Status: HealthHealthy, LatencyMS: ptr(95)}, 0, HealthHealthy, 95},
	}

	for i, step := range steps {
		if err := providers.RecordHealth(ctx, provider.ID, step.report); err != nil {
			t.Fatalf("RecordHealth langkah %d: %v", i, err)
		}
		got, err := providers.Get(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Get langkah %d: %v", i, err)
		}
		if got.ConsecutiveFailures != step.wantFailures {
			t.Errorf("langkah %d: consecutive_failures = %d, ingin %d",
				i, got.ConsecutiveFailures, step.wantFailures)
		}
		if got.LastHealthStatus == nil || *got.LastHealthStatus != step.wantStatus {
			t.Errorf("langkah %d: last_health_status = %v, ingin %q", i, got.LastHealthStatus, step.wantStatus)
		}
		if got.LastHealthAt == nil {
			t.Errorf("langkah %d: last_health_at kosong", i)
		}
		if step.wantLatencyMS > 0 && (got.LastLatencyMS == nil || *got.LastLatencyMS != step.wantLatencyMS) {
			t.Errorf("langkah %d: last_latency_ms = %v, ingin %d", i, got.LastLatencyMS, step.wantLatencyMS)
		}
	}

	t.Run("riwayat tercatat lengkap dan terbaru dulu", func(t *testing.T) {
		history, next, err := providers.ListHealthChecks(ctx, provider.ID, repo.Page{})
		if err != nil {
			t.Fatalf("ListHealthChecks: %v", err)
		}
		if len(history) != len(steps) {
			t.Fatalf("jumlah riwayat = %d, ingin %d", len(history), len(steps))
		}
		if next != "" {
			t.Errorf("kursor = %q, ingin kosong karena semuanya sudah terbaca", next)
		}
		if history[0].Status != HealthHealthy {
			t.Errorf("baris pertama = %q, ingin pemeriksaan terakhir (%q)", history[0].Status, HealthHealthy)
		}
		// Pesan yang sudah disaring dan kategori kegagalan ikut tersimpan.
		var found bool
		for _, h := range history {
			if h.ErrorKind != nil && *h.ErrorKind == "http_5xx" {
				found = true
				if h.StatusCode == nil || *h.StatusCode != 503 {
					t.Errorf("status_code = %v, ingin 503", h.StatusCode)
				}
				if h.ErrorMessage == nil || *h.ErrorMessage != "upstream membalas 503" {
					t.Errorf("error_message = %v", h.ErrorMessage)
				}
			}
		}
		if !found {
			t.Error("riwayat tidak memuat pemeriksaan berkategori http_5xx")
		}
	})

	t.Run("paginasi riwayat tidak mengulang baris", func(t *testing.T) {
		seen := map[int64]bool{}
		cursor := ""
		for range steps {
			page, next, err := providers.ListHealthChecks(ctx, provider.ID, repo.Page{Limit: 2, Cursor: cursor})
			if err != nil {
				t.Fatalf("ListHealthChecks: %v", err)
			}
			for _, h := range page {
				if seen[h.ID] {
					t.Fatalf("baris %d muncul dua kali", h.ID)
				}
				seen[h.ID] = true
			}
			if next == "" {
				break
			}
			cursor = next
		}
		if len(seen) != len(steps) {
			t.Errorf("baris terbaca = %d, ingin %d", len(seen), len(steps))
		}
	})

	t.Run("provider yang tidak ada tidak meninggalkan riwayat yatim", func(t *testing.T) {
		ghostID, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		if err := providers.RecordHealth(ctx, ghostID, HealthReport{Status: HealthHealthy}); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("RecordHealth provider hantu = %v, ingin ErrNotFound", err)
		}

		var n int
		if err := pool.QueryRow(ctx, `select count(*) from health_checks`).Scan(&n); err != nil {
			t.Fatalf("menghitung health_checks: %v", err)
		}
		if n != len(steps) {
			t.Errorf("jumlah baris health_checks = %d, ingin %d — ada riwayat yang tertulis tanpa provider", n, len(steps))
		}
	})
}

// Kandidat rute: urutan, penyaringan, dan aturan BYOK.
func TestRouteCandidatesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	providers := NewProviderRepo(pool)
	models := NewModelRepo(pool)

	owner := seedUser(ctx, t, pool)
	otherUser := seedUser(ctx, t, pool)
	model := seedModel(ctx, t, pool, "gpt-5", CapText, CapTools)

	// Nama provider dipilih supaya urutan abjadnya jelas: pemutus seri terakhir adalah nama.
	create := func(name string, priority, weight int, byokOwner *string) *Provider {
		t.Helper()
		p := CreateProviderParams{
			Name: name, DisplayName: name, Kind: KindOpenAICompatible,
			BaseURL:  "https://" + name + ".contoh.test/v1",
			Priority: ptr(priority), Weight: ptr(weight),
		}
		if byokOwner != nil {
			p.IsBYOK = true
			p.OwnerUserID = byokOwner
		}
		created, err := providers.Create(ctx, p)
		if err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		return created
	}

	// aaa: prioritas provider tinggi (50) tapi ditimpa pemetaan menjadi 1.
	aaa := create("aaa-ditimpa-pemetaan", 50, 1, nil)
	bbb := create("bbb-berat-besar", 10, 9, nil)
	ccc := create("ccc-berat-kecil", 10, 1, nil)
	ddd := create("ddd-berat-kecil", 10, 1, nil)
	eee := create("eee-provider-mati", 5, 1, nil)
	fff := create("fff-tidak-sehat", 6, 1, nil)
	ggg := create("ggg-pemetaan-mati", 2, 1, nil)
	hhh := create("hhh-byok-saya", 20, 1, &owner)
	iii := create("iii-byok-orang-lain", 20, 1, &otherUser)

	seedAttachment(ctx, t, pool, model.ID, aaa.ID, ptr(1))
	for _, p := range []*Provider{bbb, ccc, ddd, eee, fff, hhh, iii} {
		seedAttachment(ctx, t, pool, model.ID, p.ID, nil)
	}
	// Pemetaan yang dimatikan, providernya sendiri tetap aktif.
	disabledMapping := seedAttachment(ctx, t, pool, model.ID, ggg.ID, nil)
	if _, err := models.UpdateProviderModel(ctx, disabledMapping.ID, UpdateProviderModelParams{Enabled: ptr(false)}); err != nil {
		t.Fatalf("mematikan pemetaan: %v", err)
	}
	if _, err := providers.SetEnabled(ctx, eee.ID, false); err != nil {
		t.Fatalf("mematikan provider: %v", err)
	}
	if err := providers.RecordHealth(ctx, fff.ID, HealthReport{Status: HealthUnhealthy}); err != nil {
		t.Fatalf("menandai provider tidak sehat: %v", err)
	}
	// Degraded tetap boleh menjadi kandidat: masih melayani, hanya lambat.
	if err := providers.RecordHealth(ctx, ccc.ID, HealthReport{Status: HealthDegraded, LatencyMS: ptr(3000)}); err != nil {
		t.Fatalf("menandai provider degraded: %v", err)
	}

	namesOf := func(candidates []*RouteCandidate) []string {
		out := make([]string, len(candidates))
		for i, c := range candidates {
			out[i] = c.ProviderName
		}
		return out
	}
	equal := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	t.Run("urutan prioritas efektif lalu bobot lalu nama", func(t *testing.T) {
		got, err := providers.RouteCandidates(ctx, RouteQuery{ModelID: model.ID})
		if err != nil {
			t.Fatalf("RouteCandidates: %v", err)
		}
		want := []string{aaa.Name, bbb.Name, ccc.Name, ddd.Name}
		if !equal(namesOf(got), want) {
			t.Fatalf("urutan kandidat = %v, ingin %v", namesOf(got), want)
		}
		if got[0].Priority != 1 {
			t.Errorf("prioritas efektif kandidat pertama = %d, ingin 1 (penimpaan pemetaan)", got[0].Priority)
		}
		if got[1].Weight != 9 {
			t.Errorf("bobot efektif = %d, ingin 9 (warisan dari provider)", got[1].Weight)
		}
		if got[0].UpstreamModelName != model.ModelID {
			t.Errorf("upstream_model_name = %q, ingin %q", got[0].UpstreamModelName, model.ModelID)
		}
		if !got[0].SupportsStreaming || !got[0].SupportsTools {
			t.Error("dukungan streaming dan tools seharusnya aktif secara bawaan")
		}
	})

	t.Run("provider tidak sehat ikut hanya bila diminta", func(t *testing.T) {
		got, err := providers.RouteCandidates(ctx, RouteQuery{ModelID: model.ID, IncludeUnhealthy: true})
		if err != nil {
			t.Fatalf("RouteCandidates: %v", err)
		}
		want := []string{aaa.Name, fff.Name, bbb.Name, ccc.Name, ddd.Name}
		if !equal(namesOf(got), want) {
			t.Errorf("urutan kandidat = %v, ingin %v", namesOf(got), want)
		}
	})

	t.Run("BYOK hanya milik pemintanya", func(t *testing.T) {
		got, err := providers.RouteCandidates(ctx, RouteQuery{ModelID: model.ID, OwnerUserID: owner})
		if err != nil {
			t.Fatalf("RouteCandidates: %v", err)
		}
		want := []string{aaa.Name, bbb.Name, ccc.Name, ddd.Name, hhh.Name}
		if !equal(namesOf(got), want) {
			t.Fatalf("urutan kandidat = %v, ingin %v", namesOf(got), want)
		}
		for _, c := range got {
			if c.IsBYOK && (c.OwnerUserID == nil || *c.OwnerUserID != owner) {
				t.Errorf("kandidat BYOK %s bukan milik peminta", c.ProviderName)
			}
		}
		if strings.Contains(strings.Join(namesOf(got), ","), iii.Name) {
			t.Error("BYOK milik pengguna lain ikut menjadi kandidat")
		}
	})

	t.Run("model tanpa pemetaan tidak menghasilkan kandidat", func(t *testing.T) {
		orphan := seedModel(ctx, t, pool, "model-tanpa-provider", CapText)
		got, err := providers.RouteCandidates(ctx, RouteQuery{ModelID: orphan.ID})
		if err != nil {
			t.Fatalf("RouteCandidates: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("kandidat = %v, ingin kosong", namesOf(got))
		}
	})
}

// Query terpanas di sistem wajib dilayani indeks provider_models_routing_idx, bukan
// sequential scan. Dibuktikan dengan membaca rencana eksekusi PostgreSQL.
func TestRouteCandidatesUsesIndexIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	seedBulkRouting(ctx, t, pool, 600, 200, 4)

	var modelID string
	if err := pool.QueryRow(ctx, `select id::text from models where model_id = 'bulk-m-7'`).Scan(&modelID); err != nil {
		t.Fatalf("mengambil model contoh: %v", err)
	}

	plan := explain(ctx, t, pool, routeCandidatesSQL, modelID, false, nil)
	t.Logf("rencana kandidat rute:\n%s", plan)

	if !strings.Contains(plan, "provider_models_routing_idx") {
		t.Errorf("rencana tidak memakai provider_models_routing_idx:\n%s", plan)
	}
	if strings.Contains(plan, "Seq Scan on provider_models") {
		t.Errorf("rencana membaca seluruh provider_models:\n%s", plan)
	}

	// Tabel providers pada skala test masih dibaca utuh, dan itu memang pilihan termurah:
	// beberapa ratus baris provider hanya sebesar satu-dua halaman, jadi menyusun tabel
	// hash lebih murah daripada beberapa pencarian indeks. Yang perlu dipastikan adalah
	// tidak ada hambatan bentuk query di sisi itu — dengan sequential scan dimatikan,
	// rencananya harus tetap bisa dijalankan lewat indeks sepenuhnya.
	t.Run("sisi providers pun bisa dilayani indeks", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("membuka transaksi: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err := tx.Exec(ctx, "set local enable_seqscan = off"); err != nil {
			t.Fatalf("mematikan sequential scan: %v", err)
		}
		plan := explain(ctx, t, tx, routeCandidatesSQL, modelID, false, nil)
		t.Logf("rencana tanpa sequential scan:\n%s", plan)

		if strings.Contains(plan, "Seq Scan") {
			t.Errorf("masih ada sequential scan padahal indeks tersedia:\n%s", plan)
		}
	})
}

// Penyaringan dan paginasi keyset daftar provider.
func TestProviderListIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	providers := NewProviderRepo(pool)
	owner := seedUser(ctx, t, pool)

	for i := range 7 {
		p := CreateProviderParams{
			Name:        fmt.Sprintf("daftar-%d", i),
			DisplayName: fmt.Sprintf("Daftar %d", i),
			Kind:        KindOpenAICompatible,
			BaseURL:     "https://daftar.contoh.test/v1",
		}
		if i%2 == 0 {
			p.Kind = KindAnthropic
		}
		if i == 6 {
			p.IsBYOK = true
			p.OwnerUserID = &owner
			p.Enabled = ptr(false)
		}
		if _, err := providers.Create(ctx, p); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	t.Run("filter jenis, status, dan BYOK", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			filter ProviderFilter
			want   int
		}{
			{"semua", ProviderFilter{}, 7},
			{"jenis anthropic", ProviderFilter{Kind: KindAnthropic}, 4},
			{"aktif", ProviderFilter{Enabled: ptr(true)}, 6},
			{"tidak aktif", ProviderFilter{Enabled: ptr(false)}, 1},
			{"byok", ProviderFilter{IsBYOK: ptr(true)}, 1},
			{"bersama", ProviderFilter{IsBYOK: ptr(false)}, 6},
			{"milik pengguna", ProviderFilter{OwnerUserID: owner}, 1},
			{"pencarian nama", ProviderFilter{Search: "daftar-3"}, 1},
			{"pencarian tanpa hasil", ProviderFilter{Search: "tidak-ada"}, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, _, err := providers.List(ctx, tc.filter, repo.Page{})
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(got) != tc.want {
					t.Errorf("jumlah = %d, ingin %d", len(got), tc.want)
				}
			})
		}
	})

	t.Run("paginasi keyset membaca semua baris tepat sekali", func(t *testing.T) {
		seen := map[string]bool{}
		cursor := ""
		for range 10 {
			page, next, err := providers.List(ctx, ProviderFilter{}, repo.Page{Limit: 3, Cursor: cursor})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(page) > 3 {
				t.Fatalf("halaman berisi %d baris, melebihi limit", len(page))
			}
			for _, p := range page {
				if seen[p.ID] {
					t.Fatalf("provider %s muncul dua kali", p.Name)
				}
				seen[p.ID] = true
			}
			if next == "" {
				break
			}
			cursor = next
		}
		if len(seen) != 7 {
			t.Errorf("provider terbaca = %d, ingin 7", len(seen))
		}
	})

	t.Run("kursor rusak ditolak sebagai pelanggaran aturan", func(t *testing.T) {
		_, _, err := providers.List(ctx, ProviderFilter{}, repo.Page{Cursor: "bukan-kursor"})
		if !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("List dengan kursor rusak = %v, ingin ErrConstraint", err)
		}
	})
}
