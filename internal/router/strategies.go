package router

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Selector menyusun urutan kandidat menurut strategi.
//
// Aman dipakai bersamaan oleh banyak goroutine: satu-satunya keadaan yang berubah adalah
// penghitung round robin, dan itu atomik.
type Selector struct {
	latency LatencySource
	cost    CostSource

	// rr menyimpan penghitung round robin per kunci rotasi.
	//
	// Di memori, bukan di Redis, dan itu pilihan yang sadar. Round robin lintas instance
	// menuntut satu round-trip Redis di jalur permintaan TERPANAS, sementara yang
	// diperoleh cuma pemerataan yang lebih rapi di atas kertas: dengan N instance yang
	// masing-masing berotasi sendiri, beban tetap terbagi rata ke semua provider — yang
	// hilang hanya jaminan bahwa dua permintaan berurutan lintas instance tidak mengenai
	// provider yang sama. Itu bukan sifat yang dijanjikan siapa pun, dan bukan sifat yang
	// pantas dibayar dengan latensi setiap permintaan.
	rr sync.Map // map[string]*atomic.Uint64

	// acak dipisah supaya test bisa menjadikan strategi weighted deterministik.
	acak func() float64
}

// SelectorOption menyetel Selector.
type SelectorOption func(*Selector)

// WithLatencySource memasang sumber latensi untuk StrategyLowestLatency.
func WithLatencySource(src LatencySource) SelectorOption {
	return func(s *Selector) { s.latency = src }
}

// WithCostSource memasang sumber harga untuk StrategyLowestCost.
func WithCostSource(src CostSource) SelectorOption {
	return func(s *Selector) { s.cost = src }
}

// WithRandFunc mengganti sumber acak strategi weighted.
//
// Ada untuk test: weighted yang tidak bisa dibuat deterministik hanya bisa diuji secara
// statistik, dan uji statistik pada satu proses test selalu punya pilihan buruk antara
// lambat dan rapuh.
func WithRandFunc(f func() float64) SelectorOption {
	return func(s *Selector) {
		if f != nil {
			s.acak = f
		}
	}
}

// NewSelector membuat Selector.
//
// Sumber latensi dan harga boleh nil: strateginya tetap bisa dipanggil, dan karena tidak
// ada nilai yang diketahui, seluruh kandidat dianggap "tidak terukur" lalu diurutkan
// seperti StrategyPriority. Itu lebih baik daripada menolak permintaan hanya karena
// pengumpulan metrik Fase 9 belum terpasang.
func NewSelector(opts ...SelectorOption) *Selector {
	s := &Selector{acak: defaultRand}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Order menyusun urutan kandidat untuk satu permintaan.
//
// rule boleh nil, yang berarti tidak ada aturan yang cocok: strateginya StrategyPriority
// dan seluruh kandidat ikut. Hasilnya slice BARU — cands tidak diubah, karena pemanggil
// kemungkinan memakainya lagi untuk mencatat kandidat yang tersedia.
//
// Slice kosong berarti tidak ada kandidat yang bisa melayani permintaan ini. Itu keadaan
// yang sah dan berbeda dari error: pemanggil menjawabnya dengan pesan yang menjelaskan
// syarat mana yang tidak terpenuhi, bukan dengan 500.
func (s *Selector) Order(req Request, rule *Rule, cands []*upstream.RouteCandidate) []*upstream.RouteCandidate {
	if len(cands) == 0 {
		return nil
	}

	strategy := StrategyPriority
	if rule != nil && rule.Strategy.Valid() {
		strategy = rule.Strategy
	}

	// Dua penyaringan berjalan lebih dulu, dan keduanya soal KEMAMPUAN, bukan preferensi:
	// kandidat yang tersaring memang tidak bisa melayani permintaan ini, jadi tidak ada
	// strategi yang boleh mengembalikannya.
	out := s.saringKandidat(req, rule, cands)
	if len(out) == 0 {
		return nil
	}

	switch strategy {
	case StrategyRoundRobin:
		urutPrioritas(out)
		s.putar(rotasiKey(req, rule), out)
	case StrategyWeighted:
		s.urutBobot(rule, out)
	case StrategyLowestLatency:
		s.urutLatensi(out)
	case StrategyLowestCost:
		s.urutBiaya(req, out)
	case StrategyCapability:
		urutKemampuan(out)
	default:
		urutPrioritas(out)
	}
	return out
}

// saringKandidat membuang kandidat yang tidak bisa melayani permintaan ini.
//
// Ini bukan bagian dari strategi. Streaming yang tidak didukung atau jendela konteks yang
// terlalu kecil membuat permintaan GAGAL, bukan melambat, jadi menaruh kandidat seperti itu
// di belakang barisan failover cuma menunda kegagalan sambil membakar kuota.
func (s *Selector) saringKandidat(req Request, rule *Rule, cands []*upstream.RouteCandidate) []*upstream.RouteCandidate {
	// Aturan yang menyebut kandidat membatasi pilihan HANYA pada yang disebutnya.
	var izin map[string]bool
	if rule != nil && len(rule.Providers) > 0 {
		izin = make(map[string]bool, len(rule.Providers))
		for _, rp := range rule.Providers {
			izin[rp.ProviderID] = true
		}
	}

	butuhTools := slices.Contains(req.Capabilities, upstream.CapTools)

	out := make([]*upstream.RouteCandidate, 0, len(cands))
	for _, c := range cands {
		switch {
		case izin != nil && !izin[c.ProviderID]:
		case req.Streaming && !c.SupportsStreaming:
		case butuhTools && !c.SupportsTools:
		case !muatKonteks(c, req.Tokens):
		default:
			out = append(out, c)
		}
	}
	return out
}

// muatKonteks melaporkan apakah jendela konteks kandidat cukup untuk permintaan ini.
//
// Jendela yang TIDAK dideklarasikan (nil) dianggap cukup, bukan tidak cukup: banyak
// provider compatible tidak melaporkannya, dan menyaringnya di sini akan membuat provider
// yang sebenarnya sanggup tidak pernah terpakai. Yang disaring hanya kandidat yang
// menyatakan sendiri bahwa jendelanya lebih kecil daripada masukan yang akan dikirim.
func muatKonteks(c *upstream.RouteCandidate, est TokenEstimate) bool {
	if c.MaxContextWindow == nil || est.InputTokens <= 0 {
		return true
	}
	return est.InputTokens <= *c.MaxContextWindow
}

// urutPrioritas mengurutkan dengan urutan yang paling bisa diramalkan operator.
//
// Kuncinya berlapis sampai habis: prioritas efektif naik, lalu bobot turun, lalu nama.
// Lapis terakhir bukan hiasan — tanpa pembanding yang benar-benar unik, dua kandidat
// sederajat bisa bertukar tempat antar pemanggilan, dan urutan failover yang berubah
// sendiri adalah kelas bug yang hampir tidak mungkin dilacak dari log.
func urutPrioritas(cands []*upstream.RouteCandidate) {
	slices.SortStableFunc(cands, func(a, b *upstream.RouteCandidate) int {
		if v := cmp.Compare(a.Priority, b.Priority); v != 0 {
			return v
		}
		if v := cmp.Compare(b.Weight, a.Weight); v != 0 {
			return v
		}
		return cmp.Compare(a.ProviderName, b.ProviderName)
	})
}

// urutKemampuan mengurutkan dari kandidat yang paling sanggup.
//
// Syarat kemampuan yang bersifat mutlak (streaming, tools, jendela konteks) sudah
// disaring saringKandidat, jadi yang tersisa di sini semuanya SANGGUP melayani. Yang
// diurutkan adalah kelapangannya: jendela konteks terbesar lebih dulu, karena permintaan
// yang tumbuh di tengah percakapan lebih mungkin selamat di sana.
//
// Jendela yang tidak dideklarasikan diletakkan di belakang yang dideklarasikan. Ia tidak
// boleh menang: nil berarti "tidak diketahui", dan menganggapnya tak terbatas akan
// membuat provider yang paling sedikit melaporkan datanya justru selalu terpilih.
func urutKemampuan(cands []*upstream.RouteCandidate) {
	urutPrioritas(cands)
	slices.SortStableFunc(cands, func(a, b *upstream.RouteCandidate) int {
		ja, jb := a.MaxContextWindow, b.MaxContextWindow
		switch {
		case ja == nil && jb == nil:
			return 0
		case ja == nil:
			return 1
		case jb == nil:
			return -1
		default:
			return cmp.Compare(*jb, *ja)
		}
	})
}

// putar memutar slice sehingga titik awalnya bergeser satu setiap permintaan.
//
// Yang diputar adalah SELURUH urutan, bukan hanya elemen pertama, supaya urutan failover
// ikut bergeser. Kalau hanya kepalanya yang diputar, setiap permintaan yang gagal di
// kandidat pertama akan jatuh ke kandidat yang sama — dan provider itu menerima seluruh
// beban failover sistem.
func (s *Selector) putar(key string, cands []*upstream.RouteCandidate) {
	if len(cands) < 2 {
		return
	}
	v, _ := s.rr.LoadOrStore(key, new(atomic.Uint64))
	ctr := v.(*atomic.Uint64)
	// Add mengembalikan nilai SETELAH penambahan, jadi permintaan pertama mulai dari
	// indeks 1. Titik awalnya tidak penting; yang penting ia bergerak.
	n := int(ctr.Add(1) % uint64(len(cands)))
	putarSlice(cands, n)
}

// putarSlice memutar slice ke kiri sebanyak n posisi, di tempat.
//
// Tiga kali Reverse, bukan menyalin ke slice baru: fungsi ini berjalan sekali per
// permintaan inference, dan slice-nya milik pemanggil yang memang mengharapkannya terurut.
func putarSlice[E any](s []E, n int) {
	if len(s) < 2 {
		return
	}
	n %= len(s)
	if n == 0 {
		return
	}
	slices.Reverse(s[:n])
	slices.Reverse(s[n:])
	slices.Reverse(s)
}

// rotasiKey menentukan apa yang berotasi terpisah.
//
// Per (aturan, model), bukan global: rotasi global membuat lalu lintas satu model
// menggeser titik awal model lain, sehingga model yang jarang dipakai selalu mendapat
// kandidat yang seolah-olah acak. Pemisahan per aturan diperlukan karena dua aturan bisa
// menunjuk himpunan provider yang berbeda untuk model yang sama.
func rotasiKey(req Request, rule *Rule) string {
	if rule == nil {
		return req.ModelID
	}
	return rule.ID + "\x00" + req.ModelID
}

// urutBobot mengundi urutan sesuai bobot.
//
// Yang diundi adalah SELURUH permutasi, bukan pemenang tunggal, dan itu penting untuk
// failover: urutan yang tersisa setelah kandidat pertama gagal juga harus menghormati
// bobot. Memilih satu pemenang lalu mengurutkan sisanya dengan cara lain membuat bobot
// hanya berlaku pada percobaan pertama.
//
// Caranya kunci Efraimidis–Spirakis: setiap kandidat mendapat kunci -ln(u)/w dengan u acak
// seragam, lalu diurutkan naik. Hasilnya sampel acak TANPA pengembalian yang peluangnya
// tepat sebanding bobot pada setiap posisi — satu lintasan, tanpa pengundian berulang yang
// harus membuang kandidat terpilih dan menghitung ulang total bobot.
func (s *Selector) urutBobot(rule *Rule, cands []*upstream.RouteCandidate) {
	// Bobot khusus aturan menimpa bobot provider; itu memang guna kolomnya.
	timpa := map[string]int{}
	if rule != nil {
		for _, rp := range rule.Providers {
			if rp.Weight != nil {
				timpa[rp.ProviderID] = *rp.Weight
			}
		}
	}

	kunci := make(map[string]float64, len(cands))
	for _, c := range cands {
		w := c.Weight
		if v, ok := timpa[c.ProviderID]; ok {
			w = v
		}
		// Bobot tidak positif seharusnya sudah ditolak constraint di migrasi 0008. Kalau
		// tetap lolos, ia dijadikan 1, bukan 0: bobot nol membuat kandidat tak pernah
		// terpilih DAN tak pernah muncul di urutan failover, yaitu provider yang hilang
		// diam-diam. Konfigurasi yang keliru lebih baik berperilaku seperti bobot terkecil.
		if w <= 0 {
			w = 1
		}
		u := s.acak()
		// u == 0 membuat -ln(u) tak terhingga; digeser ke nilai terkecil yang masih
		// bermakna supaya kandidat itu sekadar kalah, bukan menghasilkan NaN.
		if u <= 0 {
			u = math.SmallestNonzeroFloat64
		}
		kunci[c.ProviderModelID] = -math.Log(u) / float64(w)
	}

	slices.SortStableFunc(cands, func(a, b *upstream.RouteCandidate) int {
		if v := cmp.Compare(kunci[a.ProviderModelID], kunci[b.ProviderModelID]); v != 0 {
			return v
		}
		return cmp.Compare(a.ProviderName, b.ProviderName)
	})
}

// defaultRand adalah sumber acak bawaan strategi weighted.
func defaultRand() float64 { return rand.Float64() }

// urutLatensi mengurutkan dari yang paling cepat menurut latensi terukur.
//
// Kandidat yang belum punya pengukuran ditaruh di belakang SEMUA yang sudah terukur, lalu
// diurutkan di antara mereka sendiri dengan aturan prioritas. Ini bukan kehati-hatian
// berlebihan: nol yang berarti "belum diukur" adalah nilai tercepat yang bisa ada, jadi
// menganggapnya angka sungguhan akan mengirim seluruh lalu lintas ke provider yang paling
// baru ditambahkan — tepat provider yang paling belum terbukti.
func (s *Selector) urutLatensi(cands []*upstream.RouteCandidate) {
	ukur := make(map[string]time.Duration, len(cands))
	for _, c := range cands {
		if d, ok := s.latensi(c); ok {
			ukur[c.ProviderModelID] = d
		}
	}
	urutTerukur(cands, func(c *upstream.RouteCandidate) (int64, bool) {
		d, ok := ukur[c.ProviderModelID]
		return int64(d), ok
	})
}

// latensi mengambil latensi satu kandidat dari sumber, dengan cadangan cuplikan kesehatan.
//
// Cadangannya ada supaya strategi ini sudah berguna sebelum Fase 9 memasang histogram:
// last_latency_ms dari health check adalah pengukuran nyata, hanya kasar. Yang tidak
// dilakukan adalah mengarang angka ketika keduanya tidak ada.
func (s *Selector) latensi(c *upstream.RouteCandidate) (time.Duration, bool) {
	if s.latency != nil {
		if d, ok := s.latency.LatencyP95(c.ProviderModelID); ok {
			return d, true
		}
	}
	if c.LastLatencyMS != nil && *c.LastLatencyMS >= 0 {
		return time.Duration(*c.LastLatencyMS) * time.Millisecond, true
	}
	return 0, false
}

// urutBiaya mengurutkan dari yang paling murah menurut tabel harga.
//
// Aturan "yang tidak diketahui tidak boleh menang" paling penting justru di sini: harga
// yang belum diisi bernilai nol, dan nol selalu termurah. Tanpa pemisahan ini, strategi
// lowest_cost akan mengarahkan seluruh lalu lintas ke pemetaan model yang harganya lupa
// dimasukkan — dan laporan biayanya akan tetap terlihat wajar, karena memang tidak ada
// harga yang dicatat.
func (s *Selector) urutBiaya(req Request, cands []*upstream.RouteCandidate) {
	biaya := make(map[string]upstream.USD, len(cands))
	if s.cost != nil {
		for _, c := range cands {
			if v, ok := s.cost.Cost(c.ProviderModelID, req.Tokens); ok {
				biaya[c.ProviderModelID] = v
			}
		}
	}
	urutTerukur(cands, func(c *upstream.RouteCandidate) (int64, bool) {
		v, ok := biaya[c.ProviderModelID]
		return v.Units(), ok
	})
}

// urutTerukur mengurutkan naik berdasarkan satu nilai terukur, dengan yang tak terukur
// di belakang.
//
// Dipakai bersama oleh strategi latensi dan biaya karena aturannya identik, dan aturan
// itulah yang mudah salah bila ditulis dua kali.
func urutTerukur(cands []*upstream.RouteCandidate, nilai func(*upstream.RouteCandidate) (int64, bool)) {
	// Prioritas dipasang lebih dulu supaya menjadi pemutus seri yang pasti — termasuk di
	// antara seluruh kandidat yang sama-sama tidak terukur.
	urutPrioritas(cands)
	slices.SortStableFunc(cands, func(a, b *upstream.RouteCandidate) int {
		va, oka := nilai(a)
		vb, okb := nilai(b)
		switch {
		case !oka && !okb:
			return 0
		case !oka:
			return 1
		case !okb:
			return -1
		default:
			return cmp.Compare(va, vb)
		}
	})
}
