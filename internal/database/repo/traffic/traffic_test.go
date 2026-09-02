package traffic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// testEnv menyiapkan schema sementara berisi skema lengkap.
func testEnv(t *testing.T) (context.Context, *pgxpool.Pool, *Repo) {
	t.Helper()
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL / DATABASE_URL tidak diset")
	}

	ctx := context.Background()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("acak: %v", err)
	}
	schema := "test_traffic_" + hex.EncodeToString(buf)

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("tidak bisa terhubung ke Postgres: %v", err)
	}
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
		admin.Close()
		t.Fatalf("membuat schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "drop schema "+schema+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
		admin.Close()
	})

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	cfg := &config.Config{
		DatabaseURL: security.Secret(dsn + sep + "search_path=" + schema),
		DBMaxConns:  4,
		DBMinConns:  1,
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	db, err := database.Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("menghubungkan database uji: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatalf("menjalankan migrasi: %v", err)
	}
	return ctx, db.Pool, New(db.Pool)
}

func ptr[T any](v T) *T { return &v }

// sampleRecord adalah baris log yang lengkap terisi.
func sampleRecord() Record {
	ip := netip.MustParseAddr("203.0.113.7")
	return Record{
		RequestID:      "req_" + strings.Repeat("a", 8),
		APIKeyName:     ptr("kunci produksi"),
		Method:         "POST",
		Endpoint:       "/v1/chat/completions",
		RequestedModel: "gpt-5",
		ModelName:      ptr("gpt-5"),
		ProviderName:   ptr("openai"),
		UpstreamModel:  ptr("gpt-5-2026-08-01"),
		StatusCode:     200,
		Stream:         true,
		InputTokens:    1200, OutputTokens: 340, CachedInputTokens: 800,
		ReasoningTokens: 120, TotalTokens: 1540,
		CostUSD:           1234567, // 0,01234567 USD
		LatencyMS:         842,
		TTFTMS:            ptr(210),
		UpstreamLatencyMS: ptr(790),
		RetryCount:        1,
		FailoverCount:     0,
		RoutingStrategy:   ptr("priority"),
		RoutingDecision:   json.RawMessage(`{"candidates":["openai","anthropic"],"chosen":"openai"}`),
		ClientIP:          &ip,
		UserAgent:         ptr("openai-python/1.2.3"),
	}
}

func TestInsertAndGet(t *testing.T) {
	ctx, _, r := testEnv(t)

	rec := sampleRecord()
	id, createdAt, err := r.Insert(ctx, rec)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if id == "" || createdAt.IsZero() {
		t.Fatalf("Insert mengembalikan id=%q createdAt=%v", id, createdAt)
	}

	got, err := r.Get(ctx, id, createdAt)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"RequestID", got.RequestID, rec.RequestID},
		{"Endpoint", got.Endpoint, rec.Endpoint},
		{"RequestedModel", got.RequestedModel, rec.RequestedModel},
		{"ProviderName", got.ProviderName, *rec.ProviderName},
		{"UpstreamModel", got.UpstreamModel, *rec.UpstreamModel},
		{"StatusCode", got.StatusCode, rec.StatusCode},
		{"Stream", got.Stream, rec.Stream},
		{"InputTokens", got.InputTokens, rec.InputTokens},
		{"OutputTokens", got.OutputTokens, rec.OutputTokens},
		{"TotalTokens", got.TotalTokens, rec.TotalTokens},
		{"CachedInputTokens", got.CachedInputTokens, rec.CachedInputTokens},
		{"ReasoningTokens", got.ReasoningTokens, rec.ReasoningTokens},
		{"LatencyMS", got.LatencyMS, rec.LatencyMS},
		{"RetryCount", got.RetryCount, rec.RetryCount},
		{"RoutingStrategy", got.RoutingStrategy, *rec.RoutingStrategy},
		{"ClientIP", got.ClientIP, "203.0.113.7"},
		{"UserAgent", got.UserAgent, *rec.UserAgent},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, mau %v", c.name, c.got, c.want)
		}
	}

	// Biaya harus kembali persis, tanpa pembulatan di jalur mana pun.
	if got.CostUSD != rec.CostUSD {
		t.Errorf("CostUSD = %d, mau %d — ada pembulatan di jalur biaya", got.CostUSD, rec.CostUSD)
	}
	if got.TTFTMS == nil || *got.TTFTMS != *rec.TTFTMS {
		t.Errorf("TTFTMS = %v, mau %d", got.TTFTMS, *rec.TTFTMS)
	}

	var decision map[string]any
	if err := json.Unmarshal(got.RoutingDecision, &decision); err != nil {
		t.Errorf("RoutingDecision bukan JSON: %v", err)
	} else if decision["chosen"] != "openai" {
		t.Errorf("RoutingDecision.chosen = %v", decision["chosen"])
	}
}

// Nilai biaya ekstrem harus tetap eksak: inilah alasan biaya dibawa sebagai bilangan
// bulat satuan 10^-8 dan diskalakan di SQL, bukan lewat float64.
func TestCostPrecision(t *testing.T) {
	ctx, _, r := testEnv(t)

	costs := []int64{0, 1, 99999999, 100000000, 123456789, 999999999999}
	for _, cost := range costs {
		t.Run(fmt.Sprintf("%d", cost), func(t *testing.T) {
			rec := sampleRecord()
			rec.CostUSD = cost

			id, createdAt, err := r.Insert(ctx, rec)
			if err != nil {
				t.Fatalf("Insert: %v", err)
			}
			got, err := r.Get(ctx, id, createdAt)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.CostUSD != cost {
				t.Errorf("CostUSD = %d, mau %d", got.CostUSD, cost)
			}
		})
	}
}

func TestEventsAndPayload(t *testing.T) {
	ctx, _, r := testEnv(t)

	id, createdAt, err := r.Insert(ctx, sampleRecord())
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	events := []Event{
		{Seq: 1, Kind: EventAttemptStarted, ProviderName: ptr("openai")},
		{Seq: 2, Kind: EventAttemptFailed, ProviderName: ptr("openai"),
			StatusCode: ptr(429), ErrorKind: ptr("rate_limit"), ErrorMessage: ptr("terlalu banyak permintaan")},
		{Seq: 3, Kind: EventFailover, ProviderName: ptr("anthropic")},
		{Seq: 4, Kind: EventAttemptSucceeded, ProviderName: ptr("anthropic"),
			StatusCode: ptr(200), LatencyMS: ptr(640)},
	}
	if err := r.AppendEvents(ctx, id, createdAt, events); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}

	got, err := r.Events(ctx, id, createdAt)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(got) != len(events) {
		t.Fatalf("jumlah event = %d, mau %d", len(got), len(events))
	}
	for i, e := range got {
		if e.Seq != events[i].Seq || e.Kind != events[i].Kind {
			t.Errorf("event %d = (%d, %s), mau (%d, %s)", i, e.Seq, e.Kind, events[i].Seq, events[i].Kind)
		}
	}

	// Daftar event kosong tidak boleh menyentuh database maupun menghasilkan error.
	if err := r.AppendEvents(ctx, id, createdAt, nil); err != nil {
		t.Errorf("AppendEvents kosong: %v", err)
	}

	payload := Payload{
		RequestHeaders:  json.RawMessage(`{"content-type":"application/json"}`),
		RequestBody:     json.RawMessage(`{"model":"gpt-5","messages":[]}`),
		ResponseHeaders: json.RawMessage(`{"content-type":"text/event-stream"}`),
		ResponseBody:    json.RawMessage(`{"choices":[]}`),
		SizeBytes:       128,
	}
	if err := r.SavePayload(ctx, id, createdAt, payload); err != nil {
		t.Fatalf("SavePayload: %v", err)
	}

	gotPayload, err := r.Payload(ctx, id, createdAt)
	if err != nil {
		t.Fatalf("Payload: %v", err)
	}
	if gotPayload.SizeBytes != payload.SizeBytes {
		t.Errorf("SizeBytes = %d, mau %d", gotPayload.SizeBytes, payload.SizeBytes)
	}

	// Penyimpanan ulang harus menimpa, bukan gagal karena kunci ganda.
	payload.Truncated = true
	payload.SizeBytes = 256
	if err := r.SavePayload(ctx, id, createdAt, payload); err != nil {
		t.Fatalf("SavePayload kedua: %v", err)
	}
	gotPayload, _ = r.Payload(ctx, id, createdAt)
	if !gotPayload.Truncated || gotPayload.SizeBytes != 256 {
		t.Errorf("penyimpanan ulang tidak menimpa: %+v", gotPayload)
	}
}

// Ini properti terpenting paket ini: penanda paginasi harus punya urutan total.
// Tiga puluh baris dengan latensi IDENTIK adalah kasus yang membuat penanda
// berbasis satu kolom melewatkan atau menggandakan baris.
func TestKeysetPaginationWithTiedSortValues(t *testing.T) {
	ctx, _, r := testEnv(t)

	const total = 30
	for i := 0; i < total; i++ {
		rec := sampleRecord()
		rec.RequestID = fmt.Sprintf("req_%03d", i)
		rec.LatencyMS = 500    // sengaja sama untuk semua baris
		rec.TotalTokens = 1000 // sama juga
		rec.CostUSD = 1000000  // dan biaya pun sama
		if _, _, err := r.Insert(ctx, rec); err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
	}

	for _, sortKey := range []string{SortTime, SortLatency, SortTokens, SortCost} {
		for _, asc := range []bool{false, true} {
			name := fmt.Sprintf("%s/asc=%v", sortKey, asc)
			t.Run(name, func(t *testing.T) {
				seen := map[string]int{}
				cursor := ""
				for pages := 0; pages < 20; pages++ {
					res, err := r.List(ctx,
						Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour),
							Sort: sortKey, Ascending: asc},
						repo.Page{Limit: 7, Cursor: cursor})
					if err != nil {
						t.Fatalf("List: %v", err)
					}
					for _, row := range res.Rows {
						seen[row.ID]++
					}
					if res.NextCursor == "" {
						break
					}
					cursor = res.NextCursor
				}

				if len(seen) != total {
					t.Errorf("terlihat %d baris unik, mau %d", len(seen), total)
				}
				for id, n := range seen {
					if n != 1 {
						t.Errorf("baris %s muncul %d kali", id, n)
					}
				}
			})
		}
	}
}

func TestListFilters(t *testing.T) {
	ctx, pool, r := testEnv(t)

	provider := newProvider(t, ctx, pool, "openai")
	model := newModel(t, ctx, pool, "gpt-5")

	insert := func(status int, providerID, modelID *string, reqID, providerName string) {
		rec := sampleRecord()
		rec.RequestID = reqID
		rec.StatusCode = status
		rec.ProviderID = providerID
		rec.ModelID = modelID
		rec.ProviderName = &providerName
		if _, _, err := r.Insert(ctx, rec); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	insert(200, &provider, &model, "req_ok_1", "openai")
	insert(200, &provider, &model, "req_ok_2", "openai")
	insert(429, &provider, &model, "req_limit", "openai")
	insert(500, nil, nil, "req_err", "anthropic")

	window := Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour)}

	tests := []struct {
		name   string
		mutate func(*Filter)
		want   int
	}{
		{"tanpa filter", func(*Filter) {}, 4},
		{"per provider", func(f *Filter) { f.ProviderID = provider }, 3},
		{"per model", func(f *Filter) { f.ModelID = model }, 3},
		{"kode status persis", func(f *Filter) { f.Status = "429" }, 1},
		{"kelas status 2xx", func(f *Filter) { f.Status = "2xx" }, 2},
		{"kelas status 5xx", func(f *Filter) { f.Status = "5xx" }, 1},
		{"hanya error", func(f *Filter) { f.OnlyErrors = true }, 2},
		{"cari request id", func(f *Filter) { f.Search = "req_limit" }, 1},
		{"cari nama provider", func(f *Filter) { f.Search = "anthropic" }, 1},
		{"cari model", func(f *Filter) { f.Search = "gpt-5" }, 4},
		{"rentang waktu di luar jangkauan", func(f *Filter) {
			f.From = time.Now().Add(-48 * time.Hour)
			f.To = time.Now().Add(-24 * time.Hour)
		}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := window
			tc.mutate(&f)
			res, err := r.List(ctx, f, repo.Page{Limit: 100})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(res.Rows) != tc.want {
				ids := make([]string, 0, len(res.Rows))
				for _, row := range res.Rows {
					ids = append(ids, row.RequestID)
				}
				t.Errorf("dapat %d baris (%v), mau %d", len(res.Rows), ids, tc.want)
			}
		})
	}
}

func TestListRejectsBadInput(t *testing.T) {
	ctx, _, r := testEnv(t)

	if _, err := r.List(ctx, Filter{Sort: "drop table requests"}, repo.Page{}); err == nil {
		t.Error("kolom pengurutan asing seharusnya ditolak")
	}
	if _, err := r.List(ctx, Filter{Status: "bukan-status"}, repo.Page{}); err == nil {
		t.Error("status tidak sah seharusnya ditolak")
	}
	if _, err := r.List(ctx, Filter{}, repo.Page{Cursor: "bukan-base64!!"}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("kursor rusak: err = %v, mau ErrInvalidCursor", err)
	}
}

// Mengganti kolom pengurutan membuat kursor lama tidak bermakna; menolaknya lebih baik
// daripada menghasilkan halaman yang melewatkan baris.
func TestCursorRejectedWhenSortChanges(t *testing.T) {
	ctx, _, r := testEnv(t)
	for i := 0; i < 5; i++ {
		if _, _, err := r.Insert(ctx, sampleRecord()); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	window := Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour)}
	res, err := r.List(ctx, window, repo.Page{Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.NextCursor == "" {
		t.Fatal("tidak ada kursor untuk diuji")
	}

	changed := window
	changed.Sort = SortCost
	if _, err := r.List(ctx, changed, repo.Page{Limit: 2, Cursor: res.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("err = %v, mau ErrInvalidCursor", err)
	}
}

func TestGetNotFound(t *testing.T) {
	ctx, _, r := testEnv(t)

	_, err := r.Get(ctx, "11111111-1111-1111-1111-111111111111", time.Now())
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("err = %v, mau ErrNotFound", err)
	}
}

// List selalu membawa batas waktu, jadi perencana harus bisa memangkas partisi.
func TestListPrunesPartitions(t *testing.T) {
	ctx, pool, r := testEnv(t)
	if _, _, err := r.Insert(ctx, sampleRecord()); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Rencana diambil untuk query yang bentuknya sama dengan yang dipakai List.
	rows, err := pool.Query(ctx, `
		explain select id from requests
		where created_at >= $1 and created_at < $2
		order by created_at desc, id desc limit 10`,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}

	// Partisi yang tidak relevan harus dibuang, entah lewat pemangkasan saat
	// perencanaan (partisi tidak muncul) atau saat eksekusi ("Subplans Removed").
	text := plan.String()
	if strings.Contains(text, "requests_default") && !strings.Contains(text, "Subplans Removed") {
		t.Errorf("partisi tidak dipangkas:\n%s", text)
	}
}

// --- helper ---------------------------------------------------------------

func newProvider(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		insert into providers (name, display_name, kind, base_url)
		values ($1, $1, 'openai_compatible', 'https://upstream.test') returning id::text`, name).Scan(&id)
	if err != nil {
		t.Fatalf("membuat provider: %v", err)
	}
	return id
}

func newModel(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		insert into models (model_id, display_name) values ($1, $1) returning id::text`, name).Scan(&id)
	if err != nil {
		t.Fatalf("membuat model: %v", err)
	}
	return id
}
