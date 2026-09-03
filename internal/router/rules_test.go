package router

import (
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
