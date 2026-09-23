package router

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

func TestRuleMatchesKondisiKosongCocokSemua(t *testing.T) {
	// Aturan tanpa kondisi apa pun adalah aturan bawaan. Ini semantik yang dijanjikan
	// komentar migrasi 0008, dan Matches adalah satu-satunya tempat ia ditegakkan.
	r := &Rule{Name: "bawaan", Strategy: StrategyPriority}
	for _, req := range []Request{
		{},
		{ModelID: "m1", APIKeyID: "k1"},
		{ModelID: "m2", Capabilities: []string{upstream.CapVision, upstream.CapTools}},
	} {
		if !r.Matches(req) {
			t.Errorf("aturan tanpa kondisi tidak cocok untuk %+v", req)
		}
	}
}

func TestRuleMatchesModelDanAPIKey(t *testing.T) {
	r := &Rule{MatchModelID: "m1", MatchAPIKeyID: "k1"}

	if !r.Matches(Request{ModelID: "m1", APIKeyID: "k1"}) {
		t.Error("permintaan yang memenuhi kedua kondisi tidak cocok")
	}
	// Kondisi digabung dengan AND: satu yang tidak cocok sudah cukup menggagalkan.
	if r.Matches(Request{ModelID: "m1", APIKeyID: "lain"}) {
		t.Error("api key berbeda ikut cocok")
	}
	if r.Matches(Request{ModelID: "lain", APIKeyID: "k1"}) {
		t.Error("model berbeda ikut cocok")
	}
	if r.Matches(Request{ModelID: "m1"}) {
		t.Error("permintaan tanpa api key cocok pada aturan yang mensyaratkannya")
	}
}

// Arah pembandingan kemampuan mudah terbalik, dan terbaliknya tidak kelihatan sampai
// produksi. Aturan "khusus lalu lintas vision" HARUS cocok untuk permintaan bergambar yang
// juga memakai tools: yang diperiksa adalah apakah syarat aturan termuat di kemampuan yang
// diminta, bukan sebaliknya.
func TestRuleMatchesKemampuanAdalahHimpunanBagian(t *testing.T) {
	r := &Rule{MatchCapabilities: []string{upstream.CapVision}}

	if !r.Matches(Request{Capabilities: []string{upstream.CapVision}}) {
		t.Error("permintaan dengan kemampuan yang tepat sama tidak cocok")
	}
	if !r.Matches(Request{Capabilities: []string{upstream.CapVision, upstream.CapTools}}) {
		t.Error("permintaan dengan kemampuan LEBIH tidak cocok — arah pembandingannya terbalik")
	}
	if r.Matches(Request{Capabilities: []string{upstream.CapTools}}) {
		t.Error("permintaan tanpa kemampuan yang disyaratkan ikut cocok")
	}
	if r.Matches(Request{}) {
		t.Error("permintaan tanpa kemampuan apa pun ikut cocok")
	}

	// Aturan yang mensyaratkan DUA kemampuan menuntut keduanya.
	dua := &Rule{MatchCapabilities: []string{upstream.CapVision, upstream.CapTools}}
	if dua.Matches(Request{Capabilities: []string{upstream.CapVision}}) {
		t.Error("aturan dua syarat cocok padahal hanya satu yang diminta")
	}
	if !dua.Matches(Request{Capabilities: []string{upstream.CapTools, upstream.CapVision}}) {
		t.Error("urutan kemampuan memengaruhi kecocokan")
	}
}

func TestRuleMatchesPadaNil(t *testing.T) {
	var r *Rule
	if r.Matches(Request{}) {
		t.Error("aturan nil melaporkan cocok")
	}
}

func TestFirstMatchMengambilYangPertama(t *testing.T) {
	// Urutan slice-lah yang menentukan pemenang; FirstMatch sengaja TIDAK mengurutkan
	// ulang, supaya urutan tetap menjadi tanggung jawab query di repository.
	rules := []*Rule{
		{Name: "khusus-model", Priority: 10, MatchModelID: "m1"},
		{Name: "khusus-key", Priority: 20, MatchAPIKeyID: "k1"},
		{Name: "bawaan", Priority: 100},
	}

	got := FirstMatch(rules, Request{ModelID: "m1", APIKeyID: "k1"})
	if got == nil || got.Name != "khusus-model" {
		t.Errorf("got = %v, mau khusus-model", got)
	}
	got = FirstMatch(rules, Request{ModelID: "lain", APIKeyID: "k1"})
	if got == nil || got.Name != "khusus-key" {
		t.Errorf("got = %v, mau khusus-key", got)
	}
	got = FirstMatch(rules, Request{ModelID: "lain", APIKeyID: "lain"})
	if got == nil || got.Name != "bawaan" {
		t.Errorf("got = %v, mau bawaan", got)
	}
}

// Tidak ada aturan yang cocok adalah keadaan yang sah: pemanggil memakai strategi bawaan.
func TestFirstMatchTanpaYangCocok(t *testing.T) {
	rules := []*Rule{{Name: "khusus", MatchModelID: "m1"}}
	if got := FirstMatch(rules, Request{ModelID: "lain"}); got != nil {
		t.Errorf("got = %v, mau nil", got)
	}
	if got := FirstMatch(nil, Request{}); got != nil {
		t.Errorf("got = %v, mau nil untuk daftar kosong", got)
	}
}

func TestRuleFromRow(t *testing.T) {
	model, key := "m1", "k1"
	row := &upstream.RoutingRule{
		ID: "r1", Name: "aturan uji", Priority: 25,
		MatchModelID: &model, MatchAPIKeyID: &key,
		MatchCapabilities: []string{upstream.CapVision},
		Strategy:          string(StrategyWeighted),
		MaxAttempts:       4, BackoffMS: 500,
		FailureThreshold: 7, OpenDurationMS: 45_000, HalfOpenProbes: 2,
		Providers: []upstream.RuleProvider{
			{ProviderID: "p-kedua", Position: 2, Weight: ptr(3)},
			{ProviderID: "p-pertama", Position: 1},
		},
	}

	got, err := RuleFromRow(row)
	if err != nil {
		t.Fatalf("RuleFromRow: %v", err)
	}

	// Konversi milidetik ke Duration dilakukan SEKALI di sini. Kalau dibiarkan sebagai int,
	// setiap pemakai harus mengalikan sendiri — dan yang lupa tidak mendapat error, hanya
	// tenggat sejuta kali lebih panjang.
	if got.BackoffBase != 500*time.Millisecond {
		t.Errorf("BackoffBase = %v, mau 500ms", got.BackoffBase)
	}
	if got.OpenDuration != 45*time.Second {
		t.Errorf("OpenDuration = %v, mau 45s", got.OpenDuration)
	}
	if got.MatchModelID != "m1" || got.MatchAPIKeyID != "k1" {
		t.Errorf("kondisi = %q/%q, mau m1/k1", got.MatchModelID, got.MatchAPIKeyID)
	}
	if got.Strategy != StrategyWeighted {
		t.Errorf("Strategy = %q", got.Strategy)
	}
	// Kandidat diurutkan position naik meski barisnya datang terbalik.
	if len(got.Providers) != 2 || got.Providers[0].ProviderID != "p-pertama" {
		t.Errorf("Providers = %+v, mau terurut position", got.Providers)
	}
}

func TestRuleFromRowKondisiKosong(t *testing.T) {
	got, err := RuleFromRow(&upstream.RoutingRule{
		ID: "r1", Name: "bawaan", Strategy: string(StrategyPriority), MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("RuleFromRow: %v", err)
	}
	// Pointer nil menjadi string kosong, yang berarti "tidak membatasi" bagi Matches.
	if got.MatchModelID != "" || got.MatchAPIKeyID != "" {
		t.Errorf("kondisi = %q/%q, mau kosong", got.MatchModelID, got.MatchAPIKeyID)
	}
	if !got.Matches(Request{ModelID: "apa pun", APIKeyID: "apa pun"}) {
		t.Error("aturan tanpa kondisi tidak cocok untuk sembarang permintaan")
	}
}

// Strategi tak dikenal ditolak, bukan diperlakukan sebagai priority. Nilai seperti itu
// hanya bisa masuk kalau check constraint dan kode ini tidak lagi sepakat, dan diam pada
// keadaan itu berarti seluruh lalu lintas yang cocok aturan tersebut dirutekan dengan cara
// yang bukan diminta operator — tanpa satu pun tanda di log.
func TestRuleFromRowMenolakStrategiTakDikenal(t *testing.T) {
	_, err := RuleFromRow(&upstream.RoutingRule{
		ID: "r1", Name: "rusak", Strategy: "entah_apa",
	})
	if err == nil {
		t.Fatal("strategi tak dikenal diterima")
	}
	for _, petunjuk := range []string{"rusak", "entah_apa"} {
		if !strings.Contains(err.Error(), petunjuk) {
			t.Errorf("pesan %q tidak menyebut %q — operator tidak bisa menemukan aturannya", err, petunjuk)
		}
	}
}

func TestRuleFromRowMenolakNil(t *testing.T) {
	if _, err := RuleFromRow(nil); err == nil {
		t.Error("baris nil diterima")
	}
}

// Seluruh nilai strategi yang sah menurut check constraint migrasi 0008 harus dikenali
// Strategy.Valid. Ketidaksepakatan di sini berarti operator bisa menyimpan aturan yang
// lolos database lalu ditolak aplikasi.
func TestStrategyValidSepakatDenganMigrasi(t *testing.T) {
	dariMigrasi := []string{"priority", "round_robin", "weighted", "lowest_latency", "lowest_cost", "capability"}
	for _, s := range dariMigrasi {
		if !Strategy(s).Valid() {
			t.Errorf("strategi %q sah di migrasi 0008 tetapi ditolak Strategy.Valid", s)
		}
	}
	if Strategy("").Valid() || Strategy("entah").Valid() {
		t.Error("nilai tak dikenal dilaporkan sah")
	}
}

func TestRuleString(t *testing.T) {
	var nil_ *Rule
	if got := nil_.String(); !strings.Contains(got, "bawaan") {
		t.Errorf("String() pada nil = %q, mau menyebut aturan bawaan", got)
	}
	r := &Rule{Name: "aturan uji", Priority: 5, Strategy: StrategyWeighted,
		Providers: []RuleProvider{{ProviderID: "p1", Position: 1}}}
	got := r.String()
	for _, petunjuk := range []string{"aturan uji", "weighted", "1 kandidat"} {
		if !strings.Contains(got, petunjuk) {
			t.Errorf("String() = %q, tidak menyebut %q", got, petunjuk)
		}
	}
}

// Pipeline NULL berarti Model Only: aturan berperilaku persis seperti sebelum fitur
// combo ada. Ini adalah janji migrasi 0014 — kolom baru nullable dan tidak satu pun
// baris lama diubah — dan Combo() adalah satu-satunya tempat janji itu ditegakkan.
func TestRuleFromRowPipelineNullAdalahJalurLama(t *testing.T) {
	got, err := RuleFromRow(&upstream.RoutingRule{
		ID: "r1", Name: "model saja", Strategy: string(StrategyPriority), MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("RuleFromRow: %v", err)
	}
	if got.Pipeline != nil {
		t.Errorf("Pipeline = %+v, ingin nil untuk baris tanpa pipeline", got.Pipeline)
	}
	if got.Combo() {
		t.Error("Combo() true padahal pipeline NULL — aturan ini bukan combo")
	}
	if got.VirtualAlias != "" {
		t.Errorf("VirtualAlias = %q, ingin kosong", got.VirtualAlias)
	}
}

// Pipeline valid terurai utuh: strategi, anggaran total, dan urutan model dipertahankan.
func TestRuleFromRowPipelineValidTerurai(t *testing.T) {
	alias := "murah-cerdas"
	row := &upstream.RoutingRule{
		ID: "r2", Name: "combo hemat", Strategy: string(StrategyPriority), MaxAttempts: 3,
		Pipeline:     []byte(`{"strategy":"round_robin","attempts":3,"models":["gpt-4o-mini","gemini-2.5-flash","claude-3-haiku"]}`),
		VirtualAlias: &alias,
	}

	got, err := RuleFromRow(row)
	if err != nil {
		t.Fatalf("RuleFromRow: %v", err)
	}
	if !got.Combo() {
		t.Fatal("Combo() false padahal pipeline valid dengan 3 model")
	}
	switch {
	case got.Pipeline == nil:
		t.Fatal("Pipeline nil meski baris membawa pipeline valid")
	case got.Pipeline.Strategy != StrategyRoundRobin:
		t.Errorf("Strategi pipeline = %q, ingin round_robin", got.Pipeline.Strategy)
	case got.Pipeline.Attempts != 3:
		t.Errorf("Attempts = %d, ingin 3", got.Pipeline.Attempts)
	case len(got.Pipeline.Models) != 3:
		t.Fatalf("Models = %v, ingin 3 model", got.Pipeline.Models)
	case got.Pipeline.Models[0] != "gpt-4o-mini" || got.Pipeline.Models[2] != "claude-3-haiku":
		// Urutan adalah urutan fallback, jadi terbaliknya merusak janji resep.
		t.Errorf("urutan Models = %v, ingin terurut seperti di jsonb", got.Pipeline.Models)
	case got.VirtualAlias != "murah-cerdas":
		t.Errorf("VirtualAlias = %q, ingin %q", got.VirtualAlias, alias)
	}
}

// Strategi weighted dan capability tidak disuguhkan UI pipeline, namun dikenal enum
// Strategy — memvalidasi hanya 4 di level Go berarti menolak keadaan yang sah menurut
// enum. Penerimaan kedua nilai ini di sini adalah sengaja, bukan kelalaian.
func TestRuleFromRowPipelineMenerimaStrategiEnumPenuh(t *testing.T) {
	for _, s := range []Strategy{StrategyWeighted, StrategyCapability} {
		row := &upstream.RoutingRule{
			ID: "rp", Name: "combo enum", Strategy: string(StrategyPriority),
			Pipeline: []byte(fmt.Sprintf(`{"strategy":%q,"attempts":2,"models":["a","b"]}`, s)),
		}
		got, err := RuleFromRow(row)
		if err != nil {
			t.Fatalf("RuleFromRow strategi %s: %v", s, err)
		}
		if got.Pipeline == nil || got.Pipeline.Strategy != s {
			t.Errorf("strategi pipeline %q tidak diterima: %+v", s, got.Pipeline)
		}
	}
}

// Pipeline rusak tidak mematikan aturan: jsonb yang tidak bisa diurai hanya membuat
// aturan berjalan tanpa cascade (jalur lama), dan keadaannya dicatat. Menggagalkan
// seluruh aturan karena satu field opsional akan diam-diam mengalihkan lalu lintasnya
// ke aturan berikutnya tanpa operator paham sebabnya.
func TestRuleFromRowPipelineRusakJatuhKeJalurLama(t *testing.T) {
	for _, raw := range []string{
		`{ini bukan json`,
		`{"strategy":123,"attempts":3,"models":[]}`, // tipe field salah
		`[]`, // pipeline array, bukan objek
	} {
		t.Run(raw, func(t *testing.T) {
			got, err := RuleFromRow(&upstream.RoutingRule{
				ID: "r3", Name: "combo rusak", Strategy: string(StrategyPriority),
				Pipeline: []byte(raw),
			})
			if err != nil {
				t.Fatalf("pipeline rusak menggagalkan RuleFromRow: %v", err)
			}
			if got.Pipeline != nil {
				t.Errorf("Pipeline = %+v, ingin nil agar jatuh ke jalur lama", got.Pipeline)
			}
			if got.Combo() {
				t.Error("Combo() true pada pipeline yang rusak")
			}
			// Sisa aturan tetap utuh dan dapat dipakai mesin.
			if got.Name != "combo rusak" || got.Strategy != StrategyPriority {
				t.Errorf("aturan terbaca sebagai %+v", got)
			}
		})
	}
}

// Pengurai pipeline sengaja LEBIH LONGGAR daripada constraint database: ia tidak
// menolak models kosong maupun attempts nol. Ambang models minimal 1 dijaga di
// migrasi 0014, bukan di sini — menolaknya di dua tempat hanya membuat pesan error
// bergantung pada jalur mana yang kebetulan dipakai. Combo() sudah cukup untuk
// memastikan resep seperti itu tidak dipakai sebagai combo.
func TestRuleFromRowPipelineTidakLengkapBukanCombo(t *testing.T) {
	got, err := RuleFromRow(&upstream.RoutingRule{
		ID: "r4", Name: "combo setengah", Strategy: string(StrategyPriority),
		Pipeline: []byte(`{"strategy":"round_robin"}`),
	})
	if err != nil {
		t.Fatalf("RuleFromRow: %v", err)
	}
	if got.Pipeline == nil || got.Pipeline.Strategy != StrategyRoundRobin {
		t.Fatalf("Pipeline = %+v, ingin terparse tanpa models", got.Pipeline)
	}
	if got.Combo() {
		t.Error("Combo() true untuk pipeline tanpa models")
	}
}

// Ambang Combo() adalah lebih dari satu model: resep dengan satu model hanya
// menggantikan daftar kandidat biasa, tanpa cascade antar model.
func TestRuleComboAmbangSatuModel(t *testing.T) {
	satu := &Rule{Pipeline: &ComboPipeline{Strategy: StrategyPriority, Attempts: 2, Models: []string{"satu"}}}
	if satu.Combo() {
		t.Error("Combo() true untuk satu model")
	}
	dua := &Rule{Pipeline: &ComboPipeline{Strategy: StrategyPriority, Attempts: 2, Models: []string{"satu", "dua"}}}
	if !dua.Combo() {
		t.Error("Combo() false untuk dua model")
	}
	if (&Rule{}).Combo() {
		t.Error("Combo() true tanpa pipeline")
	}
}

// --- Alias: kolom virtual_alias + kompatibilitas tag description ---------------

// Alias() mengutamakan kolom virtual_alias, dan jatuh ke tag description bila kolomnya
// kosong. Urutan ini menjaga aturan produksi lama (tag) tetap resolvable sampai
// dikonversi — membalik urutannya akan membuat tag mengalahkan kolom, sehingga
// mengedit aturan tanpa menghapus tag lama diam-diam membatalkan alias barunya.
func TestAliasUtamakanKolomLaluTag(t *testing.T) {
	kolom := "alias-baru"
	denganKolom := &Rule{VirtualAlias: kolom, Description: "aturan [combo:alias=alias-lama]"}
	if got := denganKolom.Alias(); got != kolom {
		t.Errorf("Alias() = %q, ingin %q (kolom lebih diutamakan)", got, kolom)
	}
	hanyaTag := &Rule{Description: "aturan [combo:alias=raute-x]"}
	if got := hanyaTag.Alias(); got != "raute-x" {
		t.Errorf("Alias() = %q, ingin raute-x dari tag description", got)
	}
	if got := (&Rule{Description: "tanpa tag"}).Alias(); got != "" {
		t.Errorf("Alias() = %q, ingin kosong", got)
	}
	if got := (*Rule)(nil).Alias(); got != "" {
		t.Errorf("Alias() pada nil = %q, ingin kosong", got)
	}
}

// Permintaan ke virtual_alias harus memilih aturan itu, meski ada aturan bawaan yang
// lebih umum. Inilah inti "virtual endpoint": klien memanggil satu nama, dan keputusan
// routing-nya diambil dari aturan pemilik alias.
func TestFirstMatchMengambilSesuaiVirtualAlias(t *testing.T) {
	rules := []*Rule{
		{Name: "bawaan", Priority: 100},
		{Name: "combo-hemat", Priority: 20, VirtualAlias: "murah-cerdas",
			Pipeline: &ComboPipeline{Strategy: StrategyRoundRobin, Attempts: 3, Models: []string{"a", "b"}}},
	}
	got := FirstMatch(rules, Request{ModelName: "murah-cerdas"})
	if got == nil || got.Name != "combo-hemat" {
		t.Fatalf("got = %v, mau combo-hemat", got)
	}
}

// Aturan produksi "raute-x" memakai tag [combo:alias=...] di description dengan kolom
// virtual_alias masih NULL. Resolve lewat tag WAJIB tetap berjalan sampai seluruh
// aturan dikonversi ke kolom — ini bukan kompatibilitas opsional, ini lalu lintas nyata.
func TestFirstMatchKompatibilitasTagAliasLama(t *testing.T) {
	rules := []*Rule{
		{Name: "bawaan", Priority: 100},
		{Name: "raute-x", Priority: 20,
			Description: "combo produksi [combo:alias=raute-x]"},
	}
	got := FirstMatch(rules, Request{ModelName: "raute-x"})
	if got == nil || got.Name != "raute-x" {
		t.Fatalf("got = %v, mau raute-x lewat tag description", got)
	}
}

// Matches wajib memperlakukan alias kolom dan alias tag sama: permintaan ke alias
// diperlakukan sebagai pencocokan eksplisit, sehingga kondisi model lain tidak
// menyebabkannya ditolak.
func TestMatchesMenerimaAliasKolomSebagaiTargetEksplisit(t *testing.T) {
	denganKolom := &Rule{VirtualAlias: "murah-cerdas", MatchModelID: "m-lain"}
	if !denganKolom.Matches(Request{ModelName: "murah-cerdas", ModelID: "apa pun"}) {
		t.Error("permintaan ke alias kolom ditolak oleh kondisi model aturan")
	}
	denganTag := &Rule{Description: "[combo:alias=murah-cerdas]", MatchModelID: "m-lain"}
	if !denganTag.Matches(Request{ModelName: "murah-cerdas", ModelID: "apa pun"}) {
		t.Error("permintaan ke alias tag ditolak oleh kondisi model aturan")
	}
}

// --- ComboPipelineDTO: kontrak API -------------------------------------------

func TestParseComboPipelineDTOSah(t *testing.T) {
	p, err := ParseComboPipelineDTO([]byte(`{"strategy":"round_robin","attempts":3,"models":["a","b","c"]}`))
	if err != nil {
		t.Fatalf("ParseComboPipelineDTO: %v", err)
	}
	if p.Strategy != "round_robin" || p.Attempts != 3 || len(p.Models) != 3 {
		t.Fatalf("hasil parse = %+v", p)
	}
}

// Validasi DTO mengikuti constraint pipeline (migrasi 0014) persis: penolakan di sini
// memberi pesan 400 yang menyebut fieldnya, bukan error constraint generik dari database
// setelah aturan dibuat.
func TestParseComboPipelineDTOMenolakYangTidakSah(t *testing.T) {
	for _, tc := range []struct {
		nama string
		raw  string
	}{
		{"bukan objek", `["a","b"]`},
		{"strategi asing", `{"strategy":"weighted","attempts":2,"models":["a","b"]}`},
		{"strategi kosong", `{"strategy":"","attempts":2,"models":["a","b"]}`},
		{"attempts nol", `{"strategy":"priority","attempts":0,"models":["a","b"]}`},
		{"attempts lebih dari 20", `{"strategy":"priority","attempts":21,"models":["a","b"]}`},
		{"models kosong", `{"strategy":"priority","attempts":3,"models":[]}`},
		{"models lebih dari 8", `{"strategy":"priority","attempts":3,"models":["1","2","3","4","5","6","7","8","9"]}`},
		{"model ganda", `{"strategy":"priority","attempts":3,"models":["a","a"]}`},
		{"model kosong", `{"strategy":"priority","attempts":3,"models":["a",""]}`},
		{"json rusak", `{tidak sah`},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			if _, err := ParseComboPipelineDTO([]byte(tc.raw)); err == nil {
				t.Errorf("pipeline %q diterima, seharusnya ditolak", tc.raw)
			}
		})
	}
}

// weighted dan capability dikenal enum Strategy tetapi tidak disuguhkan UI pipeline,
// sehingga DTO menolaknya — constraint database pun hanya menerima empat nilai UI.
func TestComboPipelineDTOMenolakStrategiNonUI(t *testing.T) {
	for _, s := range []string{"weighted", "capability"} {
		if err := (&ComboPipelineDTO{Strategy: s, Attempts: 2, Models: []string{"a", "b"}}).Valid(); err == nil {
			t.Errorf("strategi %q diterima DTO, seharusnya hanya 4 nilai UI", s)
		}
	}
	for _, s := range []string{"priority", "round_robin", "lowest_latency", "lowest_cost"} {
		if err := (&ComboPipelineDTO{Strategy: s, Attempts: 2, Models: []string{"a", "b"}}).Valid(); err != nil {
			t.Errorf("strategi %q ditolak DTO: %v", s, err)
		}
	}
}
