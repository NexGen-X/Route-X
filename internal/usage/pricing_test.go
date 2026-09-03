package usage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// sumberHarga adalah PriceSource palsu yang bisa dibuat gagal.
type sumberHarga struct {
	rows []*upstream.Price
	err  error
	baca atomic.Int64
}

func (s *sumberHarga) CurrentAll(context.Context) ([]*upstream.Price, error) {
	s.baca.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func usd(t *testing.T, s string) upstream.USD {
	t.Helper()
	v, err := upstream.ParseUSD(s)
	if err != nil {
		t.Fatalf("ParseUSD(%q): %v", s, err)
	}
	return v
}

// Konversi ini adalah titik paling mudah salah di seluruh jalur biaya, dan salahnya tidak
// menghasilkan satu pun error — hanya tagihan yang keliru. Angka pembandingnya dihitung
// tangan di komentar supaya kesalahan di kode tidak bisa "membenarkan" ekspektasinya sendiri.
func TestTokensForMemisahkanBagianYangBersarang(t *testing.T) {
	// Konvensi OpenAI: prompt_tokens SUDAH memuat cached, completion_tokens SUDAH memuat
	// reasoning.
	got := TokensFor(providers.Usage{
		InputTokens:       1000,
		CachedInputTokens: 400,
		OutputTokens:      500,
		ReasoningTokens:   200,
		TotalTokens:       1500,
	})
	want := upstream.TokenUsage{Input: 600, CachedInput: 400, Output: 300, Reasoning: 200}
	if got != want {
		t.Fatalf("TokensFor = %+v, mau %+v", got, want)
	}
	// Jumlahnya harus tetap sama dengan yang dilaporkan provider: pemisahan tidak boleh
	// menambah atau menghilangkan token.
	if total := got.Total(); total != 1500 {
		t.Errorf("total token setelah dipisah = %d, mau 1500", total)
	}
}

func TestTokensForMenjepitAngkaTidakKonsisten(t *testing.T) {
	// Provider melaporkan bagian yang lebih besar daripada keseluruhannya. Jumlah token
	// harus tetap sama, dan tidak boleh ada nilai negatif yang bisa menghasilkan biaya
	// negatif.
	got := TokensFor(providers.Usage{
		InputTokens: 100, CachedInputTokens: 900,
		OutputTokens: 50, ReasoningTokens: 500,
	})
	want := upstream.TokenUsage{Input: 0, CachedInput: 100, Output: 0, Reasoning: 50}
	if got != want {
		t.Fatalf("TokensFor = %+v, mau %+v", got, want)
	}
}

func TestTokensForMenolakAngkaNegatif(t *testing.T) {
	got := TokensFor(providers.Usage{InputTokens: -5, OutputTokens: -7, TotalTokens: -1})
	if got != (upstream.TokenUsage{}) {
		t.Fatalf("TokensFor angka negatif = %+v, mau nol semua", got)
	}
}

// Biaya harus dihitung dari bagian yang SUDAH dipisah. Kalau angka bersarang dipakai apa
// adanya, token cache dan token penalaran ditagih dua kali — dan test ini menyebut kedua
// angkanya supaya selisihnya terlihat kalau pemisahannya hilang.
func TestBiayaMemakaiBagianYangSudahDipisah(t *testing.T) {
	cached := usd(t, "0.30")
	harga := &upstream.Price{
		ProviderModelID: "pm-1",
		Input:           usd(t, "3.00"),
		Output:          usd(t, "15.00"),
		CachedInput:     &cached,
		// Reasoning nil: ditagih dengan harga output, seperti seluruh penyedia yang tidak
		// menagihnya terpisah.
	}

	ev := Event{
		InputTokens: 1000, CachedInputTokens: 400,
		OutputTokens: 500, ReasoningTokens: 200, TotalTokens: 1500,
	}
	rincian, err := harga.Cost(ev.tokenUsage())
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}

	// 600 x 3,00/Mtok = 0,0018 ; 400 x 0,30 = 0,00012 ; 300 x 15,00 = 0,0045 ;
	// 200 x 15,00 = 0,003  →  0,00942 USD = 942.000 satuan 10^-8.
	const mau = upstream.USD(942_000)
	if rincian.Total != mau {
		t.Fatalf("biaya = %s (%d satuan), mau %s (%d satuan)",
			rincian.Total, rincian.Total, mau, mau)
	}

	// Angka yang akan muncul kalau pemisahannya dilewatkan, ditulis di sini supaya jelas
	// bahwa kesalahannya bukan pembulatan: 1.362.000 satuan, 44% lebih mahal.
	salah, err := harga.Cost(upstream.TokenUsage{Input: 1000, CachedInput: 400, Output: 500, Reasoning: 200})
	if err != nil {
		t.Fatalf("Cost pembanding: %v", err)
	}
	if salah.Total == rincian.Total {
		t.Fatal("angka bersarang dan angka terpisah menghasilkan biaya yang sama; test ini tidak lagi menguji apa pun")
	}
}

func TestPricerPriceDanCost(t *testing.T) {
	src := &sumberHarga{rows: []*upstream.Price{{
		ProviderModelID: "pm-1",
		Input:           usd(t, "2.00"),
		Output:          usd(t, "10.00"),
	}}}
	p := NewPricer(src, testLogger())

	if _, ok := p.Cost("pm-1", router.TokenEstimate{InputTokens: 1000}); ok {
		t.Error("Cost berhasil sebelum cuplikan pernah dimuat; jalur permintaan tidak boleh menunggu pembacaan database")
	}
	if err := p.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	if _, ok := p.Price(context.Background(), "pm-2"); ok {
		t.Error("pemetaan tanpa harga dilaporkan punya harga")
	}
	got, ok := p.Price(context.Background(), "pm-1")
	if !ok || got.Input != usd(t, "2.00") {
		t.Fatalf("Price(pm-1) = %v, %v", got, ok)
	}

	// 1.000 token input x 2,00/Mtok = 0,002 USD = 200.000 satuan.
	biaya, ok := p.Cost("pm-1", router.TokenEstimate{InputTokens: 1000})
	if !ok || biaya != upstream.USD(200_000) {
		t.Fatalf("Cost = %s (%v), mau 0.00200000", biaya, ok)
	}

	// Perkiraan kosong tetap harus punya jawaban: satu juta input + satu juta output.
	// 2,00 + 10,00 = 12,00 USD = 1.200.000.000 satuan.
	biaya, ok = p.Cost("pm-1", router.TokenEstimate{})
	if !ok || biaya != upstream.USD(1_200_000_000) {
		t.Fatalf("Cost tanpa perkiraan = %s (%v), mau 12.00000000", biaya, ok)
	}
}

func TestPricerMempertahankanCuplikanSaatPembacaanGagal(t *testing.T) {
	src := &sumberHarga{rows: []*upstream.Price{{ProviderModelID: "pm-1", Input: usd(t, "1.00")}}}
	jam := time.Now()
	p := NewPricer(src, testLogger(),
		WithPriceTTL(time.Second),
		WithPriceClock(func() time.Time { return jam }))

	if err := p.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	src.err = errors.New("database sedang tidak bisa dibaca")
	jam = jam.Add(2 * time.Second)

	if _, ok := p.Price(context.Background(), "pm-1"); !ok {
		t.Fatal("harga hilang setelah pembacaan gagal; cuplikan lama harus dipertahankan")
	}

	// Waktu segar TIDAK diperbarui saat gagal, jadi percobaan berikutnya tidak menunggu satu
	// TTL penuh.
	sebelum := src.baca.Load()
	if _, ok := p.Price(context.Background(), "pm-1"); !ok {
		t.Fatal("harga hilang pada pembacaan kedua")
	}
	if src.baca.Load() <= sebelum {
		t.Error("pembacaan berikutnya tidak dicoba lagi setelah kegagalan")
	}
}

func TestPricerTanpaSumberTidakMengetahuiApaPun(t *testing.T) {
	p := NewPricer(nil, testLogger())
	if err := p.Warm(context.Background()); err != nil {
		t.Fatalf("Warm tanpa sumber: %v", err)
	}
	if _, ok := p.Price(context.Background(), "pm-1"); ok {
		t.Error("Pricer tanpa sumber melaporkan punya harga")
	}
	if _, ok := p.Cost("pm-1", router.TokenEstimate{InputTokens: 10}); ok {
		t.Error("Cost tanpa sumber melaporkan biaya; lowest_cost harus memperlakukannya belum diketahui")
	}
}

func TestPricerInvalidateMemaksaPembacaanUlang(t *testing.T) {
	src := &sumberHarga{rows: []*upstream.Price{{ProviderModelID: "pm-1", Input: usd(t, "1.00")}}}
	p := NewPricer(src, testLogger(), WithPriceTTL(time.Hour))
	if err := p.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	sebelum := src.baca.Load()

	src.rows = []*upstream.Price{{ProviderModelID: "pm-1", Input: usd(t, "9.00")}}
	if got, _ := p.Price(context.Background(), "pm-1"); got.Input != usd(t, "1.00") {
		t.Fatal("harga berubah sebelum TTL habis maupun Invalidate dipanggil")
	}

	p.Invalidate()
	got, ok := p.Price(context.Background(), "pm-1")
	if !ok || got.Input != usd(t, "9.00") {
		t.Fatalf("harga setelah Invalidate = %v, mau 9.00", got)
	}
	if src.baca.Load() <= sebelum {
		t.Error("Invalidate tidak memicu pembacaan ulang")
	}
}
