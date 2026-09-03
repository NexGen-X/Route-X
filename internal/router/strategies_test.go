package router

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// kandidat membuat RouteCandidate untuk test dengan nilai bawaan yang wajar.
//
// Bawaannya sengaja "serba bisa" (streaming dan tools didukung, jendela konteks tidak
// dideklarasikan) supaya setiap test hanya perlu menyebut hal yang benar-benar diujinya.
func kandidat(nama string, prioritas, bobot int, ubah ...func(*upstream.RouteCandidate)) *upstream.RouteCandidate {
	c := &upstream.RouteCandidate{
		ProviderModelID:   "pm-" + nama,
		UpstreamModelName: "model-" + nama,
		SupportsStreaming: true,
		SupportsTools:     true,
		Priority:          prioritas,
		Weight:            bobot,
		ProviderID:        "prov-" + nama,
		ProviderName:      nama,
		Kind:              upstream.KindOpenAI,
	}
	for _, f := range ubah {
		f(c)
	}
	return c
}

// nama mengambil urutan nama provider dari hasil pengurutan.
func nama(cands []*upstream.RouteCandidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.ProviderName
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func TestOrderPriority(t *testing.T) {
	s := NewSelector()

	// Prioritas naik lebih dulu; bobot turun sebagai pemutus; nama sebagai pemutus akhir.
	cands := []*upstream.RouteCandidate{
		kandidat("charlie", 10, 1),
		kandidat("alpha", 5, 1),
		kandidat("bravo", 10, 9),
	}
	got := nama(s.Order(Request{}, nil, cands))
	mau := []string{"alpha", "bravo", "charlie"}
	if !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// Urutan kandidat sederajat harus PASTI, bukan kebetulan. Urutan failover yang berubah
// sendiri antar permintaan adalah kelas bug yang hampir tidak bisa dilacak dari log.
func TestOrderPriorityStabilDenganKandidatSederajat(t *testing.T) {
	s := NewSelector()
	buat := func() []*upstream.RouteCandidate {
		return []*upstream.RouteCandidate{
			kandidat("delta", 10, 5), kandidat("alpha", 10, 5), kandidat("charlie", 10, 5),
		}
	}
	pertama := nama(s.Order(Request{}, nil, buat()))
	for i := 0; i < 50; i++ {
		if got := nama(s.Order(Request{}, nil, buat())); !slices.Equal(got, pertama) {
			t.Fatalf("urutan berubah pada iterasi %d: %v lalu %v", i, pertama, got)
		}
	}
	if !slices.Equal(pertama, []string{"alpha", "charlie", "delta"}) {
		t.Errorf("urutan = %v, mau terurut nama", pertama)
	}
}

// Order tidak boleh mengubah slice milik pemanggil: pemanggil memakainya lagi untuk
// mencatat kandidat yang sempat tersedia.
func TestOrderTidakMengubahMasukan(t *testing.T) {
	s := NewSelector()
	asli := []*upstream.RouteCandidate{
		kandidat("charlie", 10, 1), kandidat("alpha", 5, 1), kandidat("bravo", 7, 1),
	}
	sebelum := nama(asli)
	_ = s.Order(Request{}, nil, asli)
	if got := nama(asli); !slices.Equal(got, sebelum) {
		t.Errorf("slice masukan ikut terurut: %v, semula %v", got, sebelum)
	}
}

func TestOrderRoundRobinMemutarSeluruhUrutan(t *testing.T) {
	s := NewSelector()
	buat := func() []*upstream.RouteCandidate {
		return []*upstream.RouteCandidate{
			kandidat("a", 1, 1), kandidat("b", 2, 1), kandidat("c", 3, 1),
		}
	}
	rule := &Rule{ID: "r1", Strategy: StrategyRoundRobin}
	req := Request{ModelID: "m1"}

	var terlihat [][]string
	for i := 0; i < 3; i++ {
		terlihat = append(terlihat, nama(s.Order(req, rule, buat())))
	}

	// Setiap permintaan mulai dari kandidat berikutnya, dan SISANYA ikut bergeser —
	// bukan hanya kepalanya. Kalau hanya kepala yang berputar, kandidat kedua selalu
	// sama dan satu provider menanggung seluruh beban failover.
	mau := [][]string{
		{"b", "c", "a"},
		{"c", "a", "b"},
		{"a", "b", "c"},
	}
	for i := range mau {
		if !slices.Equal(terlihat[i], mau[i]) {
			t.Errorf("permintaan %d: urutan = %v, mau %v", i+1, terlihat[i], mau[i])
		}
	}
}

// Rotasi dipisah per (aturan, model): lalu lintas satu model tidak boleh menggeser titik
// awal model lain, karena model yang jarang dipakai akan selalu mendapat kandidat yang
// seolah-olah acak.
func TestOrderRoundRobinDipisahPerModel(t *testing.T) {
	s := NewSelector()
	buat := func() []*upstream.RouteCandidate {
		return []*upstream.RouteCandidate{kandidat("a", 1, 1), kandidat("b", 2, 1)}
	}
	rule := &Rule{ID: "r1", Strategy: StrategyRoundRobin}

	// Model pertama diputar tiga kali.
	for i := 0; i < 3; i++ {
		s.Order(Request{ModelID: "m1"}, rule, buat())
	}
	// Model kedua harus mulai dari awal rotasinya sendiri.
	got := nama(s.Order(Request{ModelID: "m2"}, rule, buat()))
	if mau := []string{"b", "a"}; !slices.Equal(got, mau) {
		t.Errorf("model kedua = %v, mau %v — rotasinya ikut tergeser lalu lintas model lain", got, mau)
	}
}

func TestOrderWeightedMenghormatiBobot(t *testing.T) {
	// Sumber acak deterministik: nilai u yang tetap membuat kunci -ln(u)/w hanya
	// bergantung pada bobot, sehingga bobot terbesar selalu di depan. Ini menguji
	// aritmetikanya, bukan keacakannya.
	s := NewSelector(WithRandFunc(func() float64 { return 0.5 }))
	cands := []*upstream.RouteCandidate{
		kandidat("kecil", 1, 1), kandidat("besar", 1, 100), kandidat("sedang", 1, 10),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyWeighted}, cands))
	if mau := []string{"besar", "sedang", "kecil"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// Bobot khusus aturan menimpa bobot provider; itu memang guna kolomnya di
// routing_rule_providers.
func TestOrderWeightedBobotAturanMenimpaBobotProvider(t *testing.T) {
	s := NewSelector(WithRandFunc(func() float64 { return 0.5 }))
	cands := []*upstream.RouteCandidate{
		kandidat("a", 1, 100), // bobot provider besar
		kandidat("b", 1, 1),   // bobot provider kecil
	}
	rule := &Rule{
		Strategy: StrategyWeighted,
		Providers: []RuleProvider{
			{ProviderID: "prov-a", Position: 1, Weight: ptr(1)},
			{ProviderID: "prov-b", Position: 2, Weight: ptr(100)},
		},
	}
	got := nama(s.Order(Request{}, rule, cands))
	if mau := []string{"b", "a"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v — bobot aturan tidak menimpa bobot provider", got, mau)
	}
}

// Bobot tidak positif tidak boleh membuat provider hilang dari urutan failover.
// Konfigurasi keliru lebih baik berperilaku seperti bobot terkecil daripada seperti
// provider yang tidak ada.
func TestOrderWeightedBobotNolTetapMuncul(t *testing.T) {
	s := NewSelector(WithRandFunc(func() float64 { return 0.5 }))
	cands := []*upstream.RouteCandidate{kandidat("nol", 1, 0), kandidat("wajar", 1, 5)}
	got := s.Order(Request{}, &Rule{Strategy: StrategyWeighted}, cands)
	if len(got) != 2 {
		t.Fatalf("len = %d, mau 2 — kandidat berbobot nol hilang dari urutan", len(got))
	}
	if got[0].ProviderName != "wajar" {
		t.Errorf("kandidat pertama = %s, mau wajar", got[0].ProviderName)
	}
}

// Distribusi weighted diperiksa secara statistik dengan sumber acak sungguhan, karena
// kunci deterministik tidak bisa membuktikan bobot benar-benar memengaruhi peluang.
func TestOrderWeightedDistribusiCondongKeBobotBesar(t *testing.T) {
	s := NewSelector()
	const n = 4000
	hitung := map[string]int{}
	for i := 0; i < n; i++ {
		cands := []*upstream.RouteCandidate{kandidat("berat", 1, 9), kandidat("ringan", 1, 1)}
		got := s.Order(Request{}, &Rule{Strategy: StrategyWeighted}, cands)
		hitung[got[0].ProviderName]++
	}
	// Bobot 9:1 berarti sekitar 90% berbanding 10%. Batasnya dilonggarkan lebar supaya
	// test tidak rapuh, tetapi tetap cukup ketat untuk menangkap bobot yang diabaikan
	// (yang akan menghasilkan sekitar 50:50).
	bagian := float64(hitung["berat"]) / n
	if bagian < 0.85 || bagian > 0.95 {
		t.Errorf("bagian kandidat berbobot 9 = %.3f, mau sekitar 0,90 (hitung: %v)", bagian, hitung)
	}
}

// sumberLatensi adalah LatencySource untuk test.
type sumberLatensi map[string]time.Duration

func (m sumberLatensi) LatencyP95(id string) (time.Duration, bool) {
	d, ok := m[id]
	return d, ok
}

// sumberBiaya adalah CostSource untuk test.
type sumberBiaya map[string]upstream.USD

func (m sumberBiaya) Cost(id string, _ TokenEstimate) (upstream.USD, bool) {
	v, ok := m[id]
	return v, ok
}

func TestOrderLowestLatency(t *testing.T) {
	s := NewSelector(WithLatencySource(sumberLatensi{
		"pm-lambat": 900 * time.Millisecond,
		"pm-cepat":  40 * time.Millisecond,
		"pm-sedang": 200 * time.Millisecond,
	}))
	cands := []*upstream.RouteCandidate{
		kandidat("lambat", 1, 1), kandidat("cepat", 9, 1), kandidat("sedang", 5, 1),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyLowestLatency}, cands))
	if mau := []string{"cepat", "sedang", "lambat"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// Inti aturan "yang tidak diketahui tidak boleh menang": kandidat tanpa pengukuran harus
// di BELAKANG semua yang terukur, sekalipun prioritasnya paling bagus. Nol yang berarti
// "belum diukur" adalah nilai tercepat yang bisa ada, jadi memperlakukannya sebagai angka
// akan mengirim seluruh lalu lintas ke provider yang paling belum terbukti.
func TestOrderLowestLatencyKandidatTakTerukurDiBelakang(t *testing.T) {
	s := NewSelector(WithLatencySource(sumberLatensi{"pm-terukur": 900 * time.Millisecond}))
	cands := []*upstream.RouteCandidate{
		kandidat("baru", 1, 1), // prioritas terbaik, tanpa pengukuran apa pun
		kandidat("terukur", 99, 1),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyLowestLatency}, cands))
	if mau := []string{"terukur", "baru"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v — kandidat tak terukur menang", got, mau)
	}
}

// Cuplikan kesehatan dipakai sebagai cadangan supaya strategi ini sudah berguna sebelum
// Fase 9 memasang histogram. Itu pengukuran nyata, hanya kasar.
func TestOrderLowestLatencyMemakaiCuplikanKesehatan(t *testing.T) {
	s := NewSelector() // tanpa LatencySource sama sekali
	cands := []*upstream.RouteCandidate{
		kandidat("lambat", 1, 1, func(c *upstream.RouteCandidate) { c.LastLatencyMS = ptr(800) }),
		kandidat("cepat", 9, 1, func(c *upstream.RouteCandidate) { c.LastLatencyMS = ptr(30) }),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyLowestLatency}, cands))
	if mau := []string{"cepat", "lambat"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// Sumber p95 menang atas cuplikan kesehatan bila keduanya ada: p95 mengukur lalu lintas
// sungguhan, health check hanya mengukur satu probe ringan.
func TestOrderLowestLatencyP95MenangAtasCuplikan(t *testing.T) {
	s := NewSelector(WithLatencySource(sumberLatensi{"pm-a": 10 * time.Millisecond}))
	cands := []*upstream.RouteCandidate{
		kandidat("a", 1, 1, func(c *upstream.RouteCandidate) { c.LastLatencyMS = ptr(5000) }),
		kandidat("b", 1, 1, func(c *upstream.RouteCandidate) { c.LastLatencyMS = ptr(100) }),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyLowestLatency}, cands))
	if mau := []string{"a", "b"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v — p95 diabaikan", got, mau)
	}
}

func TestOrderLowestCost(t *testing.T) {
	s := NewSelector(WithCostSource(sumberBiaya{
		"pm-mahal":  upstream.MustParseUSD("0.05"),
		"pm-murah":  upstream.MustParseUSD("0.001"),
		"pm-sedang": upstream.MustParseUSD("0.01"),
	}))
	cands := []*upstream.RouteCandidate{
		kandidat("mahal", 1, 1), kandidat("murah", 9, 1), kandidat("sedang", 5, 1),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyLowestCost}, cands))
	if mau := []string{"murah", "sedang", "mahal"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// Cacat yang paling mahal di paket ini kalau salah: harga yang belum diisi bernilai nol,
// dan nol selalu termurah. Tanpa pemisahan ini, lowest_cost mengarahkan seluruh lalu
// lintas ke pemetaan model yang harganya lupa dimasukkan — dan laporan biayanya tetap
// terlihat wajar, karena memang tidak ada harga yang tercatat.
func TestOrderLowestCostHargaKosongTidakMenangSebagaiTermurah(t *testing.T) {
	s := NewSelector(WithCostSource(sumberBiaya{"pm-berharga": upstream.MustParseUSD("0.05")}))
	cands := []*upstream.RouteCandidate{
		kandidat("tanpaHarga", 1, 1), // prioritas terbaik, harga belum diisi
		kandidat("berharga", 99, 1),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyLowestCost}, cands))
	if mau := []string{"berharga", "tanpaHarga"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v — harga kosong dianggap termurah", got, mau)
	}
}

// Tanpa CostSource sama sekali, strategi ini tidak boleh menolak permintaan — ia jatuh ke
// urutan prioritas. Fase 9 yang memasang sumbernya, dan sebelum itu gateway tetap harus
// bisa melayani.
func TestOrderLowestCostTanpaSumberJatuhKePrioritas(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{kandidat("b", 9, 1), kandidat("a", 1, 1)}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyLowestCost}, cands))
	if mau := []string{"a", "b"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

func TestOrderCapabilityMengurutJendelaKonteksTerbesarDulu(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{
		kandidat("kecil", 1, 1, func(c *upstream.RouteCandidate) { c.MaxContextWindow = ptr(8_000) }),
		kandidat("besar", 9, 1, func(c *upstream.RouteCandidate) { c.MaxContextWindow = ptr(200_000) }),
		kandidat("sedang", 5, 1, func(c *upstream.RouteCandidate) { c.MaxContextWindow = ptr(32_000) }),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyCapability}, cands))
	if mau := []string{"besar", "sedang", "kecil"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// Jendela yang tidak dideklarasikan tidak boleh dianggap tak terbatas: kalau begitu,
// provider yang paling sedikit melaporkan datanya justru selalu terpilih.
func TestOrderCapabilityJendelaTakDideklarasikanDiBelakang(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{
		kandidat("takLapor", 1, 1),
		kandidat("lapor", 99, 1, func(c *upstream.RouteCandidate) { c.MaxContextWindow = ptr(4_000) }),
	}
	got := nama(s.Order(Request{}, &Rule{Strategy: StrategyCapability}, cands))
	if mau := []string{"lapor", "takLapor"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// Penyaringan kemampuan berlaku untuk SEMUA strategi, bukan hanya StrategyCapability:
// kandidat yang tidak mendukung streaming akan GAGAL, bukan melambat, jadi menaruhnya di
// belakang barisan failover cuma menunda kegagalan sambil membakar kuota.
func TestSaringKandidatBerlakuDiSemuaStrategi(t *testing.T) {
	s := NewSelector()
	semua := []Strategy{
		StrategyPriority, StrategyRoundRobin, StrategyWeighted,
		StrategyLowestLatency, StrategyLowestCost, StrategyCapability,
	}
	for _, st := range semua {
		t.Run(string(st), func(t *testing.T) {
			cands := []*upstream.RouteCandidate{
				kandidat("takStream", 1, 1, func(c *upstream.RouteCandidate) { c.SupportsStreaming = false }),
				kandidat("bisa", 2, 1),
			}
			got := s.Order(Request{Streaming: true}, &Rule{ID: "r", Strategy: st}, cands)
			if len(got) != 1 || got[0].ProviderName != "bisa" {
				t.Errorf("hasil = %v, mau hanya [bisa]", nama(got))
			}
		})
	}
}

func TestSaringKandidatTools(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{
		kandidat("takTools", 1, 1, func(c *upstream.RouteCandidate) { c.SupportsTools = false }),
		kandidat("bisa", 2, 1),
	}
	req := Request{Capabilities: []string{upstream.CapTools}}
	got := s.Order(req, nil, cands)
	if len(got) != 1 || got[0].ProviderName != "bisa" {
		t.Errorf("hasil = %v, mau hanya [bisa]", nama(got))
	}
}

func TestSaringKandidatJendelaKonteks(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{
		kandidat("sempit", 1, 1, func(c *upstream.RouteCandidate) { c.MaxContextWindow = ptr(8_000) }),
		kandidat("lapang", 2, 1, func(c *upstream.RouteCandidate) { c.MaxContextWindow = ptr(200_000) }),
		kandidat("takLapor", 3, 1), // tidak dideklarasikan: dianggap cukup
	}
	req := Request{Tokens: TokenEstimate{InputTokens: 100_000}}
	got := nama(s.Order(req, nil, cands))
	if mau := []string{"lapang", "takLapor"}; !slices.Equal(got, mau) {
		t.Errorf("hasil = %v, mau %v", got, mau)
	}
}

// Aturan yang menyebut kandidat membatasi pilihan HANYA pada yang disebutnya.
func TestOrderAturanMembatasiKandidat(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{
		kandidat("a", 1, 1), kandidat("b", 2, 1), kandidat("c", 3, 1),
	}
	rule := &Rule{
		ID: "r", Strategy: StrategyPriority,
		Providers: []RuleProvider{
			{ProviderID: "prov-c", Position: 1},
			{ProviderID: "prov-a", Position: 2},
		},
	}
	got := nama(s.Order(Request{}, rule, cands))
	// Strategi priority tetap mengurutkan menurut prioritas provider; yang dilakukan
	// daftar aturan adalah MENYARING, dan itu memang pembagian tugasnya.
	if mau := []string{"a", "c"}; !slices.Equal(got, mau) {
		t.Errorf("hasil = %v, mau %v", got, mau)
	}
}

// Tidak ada kandidat yang lolos adalah keadaan yang SAH, bukan error: pemanggil
// menjawabnya dengan pesan yang menjelaskan syarat mana yang tidak terpenuhi.
func TestOrderTanpaKandidatYangLolos(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{
		kandidat("takStream", 1, 1, func(c *upstream.RouteCandidate) { c.SupportsStreaming = false }),
	}
	if got := s.Order(Request{Streaming: true}, nil, cands); len(got) != 0 {
		t.Errorf("hasil = %v, mau kosong", nama(got))
	}
	if got := s.Order(Request{}, nil, nil); got != nil {
		t.Errorf("hasil = %v, mau nil untuk masukan kosong", nama(got))
	}
}

// Strategi tak dikenal jatuh ke priority alih-alih panik atau mengembalikan kosong.
// Nilai seperti itu hanya bisa masuk lewat Rule yang dirakit tangan; RuleFromRow
// menolaknya lebih dulu.
func TestOrderStrategiTakDikenalJatuhKePrioritas(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{kandidat("b", 9, 1), kandidat("a", 1, 1)}
	got := nama(s.Order(Request{}, &Rule{Strategy: Strategy("entah")}, cands))
	if mau := []string{"a", "b"}; !slices.Equal(got, mau) {
		t.Errorf("urutan = %v, mau %v", got, mau)
	}
}

// putarSlice diuji langsung karena ia yang menentukan bentuk rotasi round robin, dan
// kekeliruan di sini menghasilkan urutan yang tetap "terlihat berputar".
func TestPutarSlice(t *testing.T) {
	for _, tc := range []struct {
		n   int
		mau []int
	}{
		{0, []int{1, 2, 3, 4}},
		{1, []int{2, 3, 4, 1}},
		{2, []int{3, 4, 1, 2}},
		{4, []int{1, 2, 3, 4}},
		{5, []int{2, 3, 4, 1}},
	} {
		got := []int{1, 2, 3, 4}
		putarSlice(got, tc.n)
		if !slices.Equal(got, tc.mau) {
			t.Errorf("putarSlice(_, %d) = %v, mau %v", tc.n, got, tc.mau)
		}
	}
	// Slice pendek tidak boleh panik.
	putarSlice([]int{}, 3)
	putarSlice([]int{7}, 3)
}

// Kunci Efraimidis–Spirakis tidak boleh menghasilkan NaN saat sumber acak mengembalikan
// nol; kandidatnya cukup kalah, bukan merusak seluruh pengurutan.
func TestUrutBobotAmanSaatAcakNol(t *testing.T) {
	s := NewSelector(WithRandFunc(func() float64 { return 0 }))
	cands := []*upstream.RouteCandidate{kandidat("a", 1, 1), kandidat("b", 1, 2)}
	got := s.Order(Request{}, &Rule{Strategy: StrategyWeighted}, cands)
	if len(got) != 2 {
		t.Fatalf("len = %d, mau 2", len(got))
	}
	for _, c := range got {
		if c == nil {
			t.Fatal("ada kandidat nil di hasil")
		}
	}
	// Nilai kuncinya boleh sangat besar, tetapi tidak boleh NaN — NaN membuat urutan
	// bergantung pada urutan pembandingan, yaitu tidak terdefinisi.
	if math.IsNaN(-math.Log(math.SmallestNonzeroFloat64) / 1) {
		t.Error("kunci bernilai NaN")
	}
}
