package usage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// sumberLatensi adalah LatencyStore palsu.
type sumberLatensi struct {
	latensi  map[string]time.Duration
	potongan []traffic.Slice
	err      error
	// window, percentile, dan minSamples menangkap argumen pemanggilan terakhir.
	window     time.Duration
	percentile float64
	minSamples int
}

func (s *sumberLatensi) LatencyByProviderModel(
	_ context.Context, window time.Duration, percentile float64, minSamples int,
) (map[string]time.Duration, error) {
	s.window, s.percentile, s.minSamples = window, percentile, minSamples
	if s.err != nil {
		return nil, s.err
	}
	return s.latensi, nil
}

func (s *sumberLatensi) Breakdown(
	_ context.Context, _ traffic.Query, _ traffic.Dimension, _ int,
) ([]traffic.Slice, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.potongan, nil
}

func TestIndexBelumTerukurSebelumPenyegaranPertama(t *testing.T) {
	ix := NewIndex(&sumberLatensi{}, nil, testLogger())
	if _, ok := ix.LatencyP95("pm-1"); ok {
		t.Fatal("latensi dilaporkan terukur sebelum penyegaran; kandidat yang belum terukur harus diurutkan paling belakang, bukan dianggap tercepat")
	}
	if !ix.RefreshedAt().IsZero() {
		t.Error("RefreshedAt terisi tanpa penyegaran")
	}
}

func TestIndexMenyegarkanLatensiDanKetersediaan(t *testing.T) {
	src := &sumberLatensi{
		latensi: map[string]time.Duration{"pm-1": 88 * time.Millisecond},
		potongan: []traffic.Slice{
			{ID: "prov-1", Name: "openai", Totals: traffic.Totals{Requests: 100, Success: 97}},
			{ID: "", Name: "", Totals: traffic.Totals{Requests: 5, Success: 0}},
		},
	}
	metrics := observability.NewMetrics()
	ix := NewIndex(src, metrics, testLogger(), WithIndexWindow(10*time.Minute), WithIndexMinSamples(3))

	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if src.window != 10*time.Minute || src.minSamples != 3 || src.percentile != 0.95 {
		t.Errorf("argumen pengukuran = %v/%v/%v", src.window, src.percentile, src.minSamples)
	}
	if d, ok := ix.LatencyP95("pm-1"); !ok || d != 88*time.Millisecond {
		t.Errorf("LatencyP95(pm-1) = %v, %v", d, ok)
	}
	if _, ok := ix.LatencyP95("pm-2"); ok {
		t.Error("pemetaan tanpa pengukuran dilaporkan terukur")
	}
	if v, ok := ix.Availability("prov-1"); !ok || v != 0.97 {
		t.Errorf("Availability(prov-1) = %v, %v; mau 0.97", v, ok)
	}
	// Potongan tanpa provider adalah permintaan yang ditolak sebelum provider terpilih; ia
	// bukan ketersediaan provider mana pun.
	if _, ok := ix.Availability(""); ok {
		t.Error("potongan tanpa provider ikut menjadi ketersediaan")
	}
	// Label gauge memakai NAMA provider, bukan id: setiap metrik gateway lain memakai nama,
	// dan gauge ber-UUID memaksa pembaca dashboard menyambungkan dua kosakata sendiri.
	if body := scrape(t, metrics); !strings.Contains(body, `routex_provider_availability_ratio{provider="openai"} 0.97`) {
		t.Errorf("gauge ketersediaan tidak memakai nama provider; keluaran:\n%s", body)
	}
}

// Gauge yang tertinggal melaporkan angka terakhirnya selamanya, sehingga provider yang
// berhenti menerima lalu lintas terlihat tetap sehat — atau tetap rusak — dan alert di atasnya
// salah dalam kedua arah.
func TestIndexMenghapusGaugeProviderYangKeluarDariJendela(t *testing.T) {
	src := &sumberLatensi{potongan: []traffic.Slice{
		{ID: "prov-1", Name: "openai", Totals: traffic.Totals{Requests: 10, Success: 10}},
		{ID: "prov-2", Name: "anthropic", Totals: traffic.Totals{Requests: 10, Success: 5}},
	}}
	metrics := observability.NewMetrics()
	ix := NewIndex(src, metrics, testLogger())

	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh pertama: %v", err)
	}
	src.potongan = src.potongan[:1]
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh kedua: %v", err)
	}

	body := scrape(t, metrics)
	if !strings.Contains(body, `provider="openai"`) {
		t.Error("provider yang masih aktif hilang dari gauge")
	}
	if strings.Contains(body, `provider="anthropic"`) {
		t.Error("gauge provider yang keluar dari jendela tidak dihapus")
	}
	if _, ok := ix.Availability("prov-2"); ok {
		t.Error("ketersediaan provider yang keluar dari jendela masih dilaporkan")
	}
}

func TestIndexMempertahankanCuplikanSaatPenyegaranGagal(t *testing.T) {
	src := &sumberLatensi{latensi: map[string]time.Duration{"pm-1": 50 * time.Millisecond}}
	ix := NewIndex(src, nil, testLogger())
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	src.err = errors.New("statement timeout")
	if err := ix.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh tidak melaporkan kegagalan")
	}
	// Membuang cuplikan saat database sedang tidak bisa dibaca berarti gangguan sementara
	// mengubah cara seluruh lalu lintas dirutekan.
	if d, ok := ix.LatencyP95("pm-1"); !ok || d != 50*time.Millisecond {
		t.Fatalf("cuplikan hilang setelah penyegaran gagal: %v, %v", d, ok)
	}
}

func TestIndexTanpaSumberAmanDipakai(t *testing.T) {
	ix := NewIndex(nil, nil, testLogger())
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh tanpa sumber: %v", err)
	}
	if _, ok := ix.LatencyP95("pm-1"); ok {
		t.Error("Index tanpa sumber melaporkan pengukuran")
	}
	// Run harus kembali seketika, bukan menahan goroutine sampai ctx selesai.
	ctx, batal := context.WithCancel(context.Background())
	defer batal()
	selesai := make(chan struct{})
	go func() { ix.Run(ctx); close(selesai) }()
	select {
	case <-selesai:
	case <-time.After(2 * time.Second):
		t.Fatal("Run tanpa sumber tidak kembali")
	}
}

func TestIndexRunMenyegarkanSebelumTickPertama(t *testing.T) {
	src := &sumberLatensi{latensi: map[string]time.Duration{"pm-1": 12 * time.Millisecond}}
	ix := NewIndex(src, nil, testLogger(), WithIndexInterval(time.Hour))

	ctx, batal := context.WithCancel(context.Background())
	selesai := make(chan struct{})
	go func() { ix.Run(ctx); close(selesai) }()

	// Tanpa penyegaran awal, lowest_latency berperilaku seperti priority selama satu interval
	// penuh setelah start — tepat saat operator baru menyalakan gateway dan memperhatikannya.
	tenggat := time.After(3 * time.Second)
	for {
		if _, ok := ix.LatencyP95("pm-1"); ok {
			break
		}
		select {
		case <-tenggat:
			batal()
			t.Fatal("Run tidak menyegarkan sebelum tick pertama")
		case <-time.After(10 * time.Millisecond):
		}
	}
	batal()
	<-selesai
}
