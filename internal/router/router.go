// Package router memilih provider mana yang melayani satu permintaan, dan dalam urutan
// apa provider berikutnya dicoba bila yang pertama gagal.
//
// # Strategi adalah URUTAN, bukan pilihan
//
// Ini keputusan bentuk yang paling menentukan di paket ini. Setiap strategi
// mengembalikan SELURUH kandidat dalam urutan, bukan satu pemenang. Alasannya failover:
// gateway ini menjanjikan permintaan berpindah otomatis ke provider berikutnya, dan
// "berikutnya" hanya punya makna kalau strateginya menghasilkan urutan.
//
// Kalau strategi hanya memilih satu, failover terpaksa menebak urutan sisanya — dan
// tebakan itu selalu salah untuk sebagian strategi. Pada lowest_cost, provider kedua
// yang benar adalah yang termurah KEDUA; menyusun sisanya berdasarkan prioritas akan
// mengirim permintaan ke provider mahal padahal ada yang lebih murah. Pada weighted,
// memilih satu lalu mengurutkan sisanya secara sembarang membuat bobot hanya berlaku
// pada percobaan pertama.
//
// # Nilai yang tidak diketahui tidak boleh menang
//
// Dua strategi bergantung pada angka yang datang dari luar: lowest_latency dari latensi
// terukur, lowest_cost dari tabel harga. Keduanya bisa tidak punya nilai untuk kandidat
// tertentu — provider baru yang belum pernah diukur, atau pemetaan model yang harganya
// belum diisi.
//
// Kandidat seperti itu ditaruh di BELAKANG, bukan di depan. Tanpa aturan ini, biaya nol
// yang sebenarnya berarti "belum diketahui" akan selalu menjadi yang termurah, sehingga
// strategi lowest_cost secara sistematis mengarahkan seluruh lalu lintas ke provider yang
// datanya paling tidak lengkap. Kegagalan seperti itu tidak memunculkan error apa pun —
// ia hanya membuat tagihan salah.
package router

import (
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Strategy adalah cara menyusun urutan kandidat.
//
// Nilainya harus sama dengan constraint routing_rules_strategy_valid di migrasi 0008.
// Menambah strategi baru berarti mengubah constraint itu lewat migrasi baru.
type Strategy string

const (
	// StrategyPriority mengikuti prioritas efektif provider; urutan yang paling bisa
	// diramalkan operator, dan karena itu bawaannya.
	StrategyPriority Strategy = "priority"
	// StrategyRoundRobin memutar titik awal setiap permintaan.
	StrategyRoundRobin Strategy = "round_robin"
	// StrategyWeighted mengundi urutan sesuai bobot.
	StrategyWeighted Strategy = "weighted"
	// StrategyLowestLatency mengurut dari yang paling cepat menurut latensi terukur.
	StrategyLowestLatency Strategy = "lowest_latency"
	// StrategyLowestCost mengurut dari yang paling murah menurut tabel harga.
	StrategyLowestCost Strategy = "lowest_cost"
	// StrategyCapability menyaring kandidat yang benar-benar mendukung apa yang diminta
	// request, lalu mengurutkan sisanya seperti StrategyPriority.
	StrategyCapability Strategy = "capability"
)

// Valid melaporkan apakah nilai ini strategi yang dikenal.
func (s Strategy) Valid() bool {
	switch s {
	case StrategyPriority, StrategyRoundRobin, StrategyWeighted,
		StrategyLowestLatency, StrategyLowestCost, StrategyCapability:
		return true
	}
	return false
}

// Request adalah ciri satu permintaan yang menentukan rutenya.
//
// Isinya sengaja hanya yang dipakai untuk MEMILIH, bukan seluruh permintaan: paket ini
// tidak boleh bisa membaca isi pesan pengguna. Batas itu bukan kerapian belaka — mesin
// routing masuk ke log dan metrik, dan apa pun yang bisa dijangkaunya bisa ikut tercatat.
type Request struct {
	// ModelID adalah models.id hasil resolusi alias, bukan nama yang dikirim klien.
	ModelID string
	// ModelName adalah nama yang diminta klien; hanya untuk log dan pesan error.
	ModelName string

	// APIKeyID kosong berarti permintaan tanpa API key (jalur admin).
	APIKeyID string
	// OwnerUserID menentukan provider BYOK siapa yang ikut menjadi kandidat.
	OwnerUserID string

	// Capabilities adalah kemampuan yang benar-benar DIBUTUHKAN permintaan ini —
	// diturunkan dari isinya (ada gambar berarti vision, ada tools berarti tools), bukan
	// dari kemampuan yang dipunyai modelnya.
	Capabilities []string

	// Streaming menuntut kandidat yang mendukung streaming.
	Streaming bool

	// Tokens adalah perkiraan pemakaian token, dipakai StrategyLowestCost. Nol berarti
	// tidak diketahui, dan lowest_cost akan memakai perbandingan harga satuan saja.
	Tokens TokenEstimate
}

// TokenEstimate adalah perkiraan pemakaian token satu permintaan.
//
// Diperlukan karena "termurah" tidak bisa dijawab dari harga satuan saja: provider dengan
// input murah dan output mahal menang untuk ringkasan dan kalah untuk pembangkitan
// panjang. Perkiraan yang kasar tetap jauh lebih benar daripada mengabaikan bedanya.
type TokenEstimate struct {
	InputTokens  int
	OutputTokens int
}

// IsZero melaporkan apakah perkiraan ini kosong.
func (t TokenEstimate) IsZero() bool { return t.InputTokens == 0 && t.OutputTokens == 0 }

// LatencySource memasok latensi terukur untuk StrategyLowestLatency.
//
// Yang diminta persentil, bukan rata-rata: rata-rata latensi menyembunyikan provider yang
// biasanya cepat tetapi kadang menggantung, dan justru ekor itulah yang dirasakan
// pengguna. Fase 9 memasang implementasi yang membacanya dari histogram usage; sebelum itu
// ada FromHealthSnapshot yang memakai latensi health check terakhir.
type LatencySource interface {
	// LatencyP95 mengembalikan p95 untuk satu pemetaan (provider, model).
	//
	// ok false berarti belum ada pengukuran. Kandidat seperti itu diurutkan paling
	// belakang, bukan dianggap tercepat — lihat catatan paket.
	LatencyP95(providerModelID string) (d time.Duration, ok bool)
}

// CostSource memasok biaya perkiraan untuk StrategyLowestCost.
type CostSource interface {
	// Cost mengembalikan biaya perkiraan menjalankan est pada satu pemetaan model.
	//
	// ok false berarti harganya belum diisi. Kandidat seperti itu diurutkan paling
	// belakang — biaya nol yang berarti "belum diketahui" tidak boleh menang sebagai
	// yang termurah.
	Cost(providerModelID string, est TokenEstimate) (c upstream.USD, ok bool)
}
