package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Test regresi sisa grup 8 audit3: kesehatan egress pool yang dicatat
// worker kini dibaca pemilihan rute — pool tercatat tak sehat disisihkan
// selama ada alternatif, dan fail-open bila semuanya tak sehat.

// kesehatanEgressPalsu adalah EgressHealthSource untuk test.
type kesehatanEgressPalsu struct {
	mu    sync.Mutex
	pools map[string]*upstream.EgressPool
	err   error
}

func (s *kesehatanEgressPalsu) Get(_ context.Context, id string) (*upstream.EgressPool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	pool, ada := s.pools[id]
	if !ada {
		return nil, errors.New("pool tidak ditemukan")
	}
	return pool, nil
}

// poolDenganKesehatan menyusun cuplikan satu egress pool.
func poolDenganKesehatan(id, status string, pada time.Time) *upstream.EgressPool {
	return &upstream.EgressPool{
		ID:               id,
		Name:             "pool-" + id,
		LastHealthStatus: ptrString(status),
		LastHealthAt:     &pada,
	}
}

// kandidatEgress membuat kandidat yang memakai egress pool tertentu.
func kandidatEgress(nama, poolID string) *upstream.RouteCandidate {
	return kandidat(nama, func(c *upstream.RouteCandidate) {
		c.EgressPoolID = ptrString(poolID)
	})
}

// executorEgressUji membuat Executor dengan sumber kesehatan egress palsu.
func executorEgressUji(src EgressHealthSource) *Executor {
	return NewExecutor(nil, nil,
		WithJitterFunc(func(n int64) int64 { return n }),
		WithSleepFunc(func(ctx context.Context, _ time.Duration) error { return ctx.Err() }),
		WithEgressHealth(src),
	)
}

// opPencatat mengembalikan operasi yang mencatat kandidat yang dicoba dan
// selalu berhasil pada kandidat pertama yang disentuh.
func opPencatat(dipanggil *[]string) func(context.Context, *upstream.RouteCandidate) (string, error) {
	return func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
		*dipanggil = append(*dipanggil, c.ProviderName)
		return "ok-" + c.ProviderName, nil
	}
}

func TestExecuteMenyisihkanKandidatEgressTakSehat(t *testing.T) {
	baru := time.Now()
	src := &kesehatanEgressPalsu{pools: map[string]*upstream.EgressPool{
		"pool-mati": poolDenganKesehatan("pool-mati", "unhealthy", baru),
	}}
	e := executorEgressUji(src)

	var dipanggil []string
	out := Execute(context.Background(), e,
		rencana("gpt-5", 1, 0, kandidatEgress("sakit", "pool-mati"), kandidat("sehat")),
		opPencatat(&dipanggil))

	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses lewat kandidat sehat", out.Err)
	}
	if len(dipanggil) != 1 || dipanggil[0] != "sehat" {
		t.Fatalf("kandidat yang dicoba = %v, mau hanya [sehat] — pool tak sehat harus disisihkan", dipanggil)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "sehat" {
		t.Fatalf("Candidate = %v, mau sehat", out.Candidate)
	}
}

func TestExecuteFailOpenBilaSemuaEgressTakSehat(t *testing.T) {
	baru := time.Now()
	src := &kesehatanEgressPalsu{pools: map[string]*upstream.EgressPool{
		"pool-a": poolDenganKesehatan("pool-a", "unhealthy", baru),
		"pool-b": poolDenganKesehatan("pool-b", "unhealthy", baru),
	}}
	e := executorEgressUji(src)

	var dipanggil []string
	out := Execute(context.Background(), e,
		rencana("gpt-5", 1, 0, kandidatEgress("satu", "pool-a"), kandidatEgress("dua", "pool-b")),
		opPencatat(&dipanggil))

	// Fail-open: menyisihkan semuanya berarti pemadaman penuh berdasar
	// sinyal probe tidak langsung — kandidat pertama tetap dicoba.
	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses fail-open", out.Err)
	}
	if len(dipanggil) != 1 || dipanggil[0] != "satu" {
		t.Fatalf("kandidat yang dicoba = %v, mau [satu] (fail-open)", dipanggil)
	}
}

func TestExecuteKesehatanEgressBasiTidakMenyisihkan(t *testing.T) {
	// Cuplikan berumur 10 menit melewati ambang basi 5 menit: pen-probe
	// kemungkinan yang mati, bukan pool-nya — kabar lama tidak menyisihkan.
	basi := time.Now().Add(-10 * time.Minute)
	src := &kesehatanEgressPalsu{pools: map[string]*upstream.EgressPool{
		"pool-lama": poolDenganKesehatan("pool-lama", "unhealthy", basi),
	}}
	e := executorEgressUji(src)

	var dipanggil []string
	out := Execute(context.Background(), e,
		rencana("gpt-5", 1, 0, kandidatEgress("utama", "pool-lama"), kandidat("cadangan")),
		opPencatat(&dipanggil))

	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses", out.Err)
	}
	if len(dipanggil) != 1 || dipanggil[0] != "utama" {
		t.Fatalf("kandidat yang dicoba = %v, mau [utama] — kesehatan basi tidak boleh menyisihkan", dipanggil)
	}
}

func TestExecuteKesehatanEgressGagalBacaTidakMenyisihkan(t *testing.T) {
	src := &kesehatanEgressPalsu{err: errors.New("database tidak terjangkau")}
	e := executorEgressUji(src)

	var dipanggil []string
	out := Execute(context.Background(), e,
		rencana("gpt-5", 1, 0, kandidatEgress("utama", "pool-x"), kandidat("cadangan")),
		opPencatat(&dipanggil))

	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses", out.Err)
	}
	if len(dipanggil) != 1 || dipanggil[0] != "utama" {
		t.Fatalf("kandidat yang dicoba = %v, mau [utama] — kegagalan membaca kesehatan tidak boleh menyisihkan", dipanggil)
	}
}

func TestExecuteKandidatEgressSehatTetapDipakai(t *testing.T) {
	src := &kesehatanEgressPalsu{pools: map[string]*upstream.EgressPool{
		"pool-sehat": poolDenganKesehatan("pool-sehat", "healthy", time.Now()),
	}}
	e := executorEgressUji(src)

	var dipanggil []string
	out := Execute(context.Background(), e,
		rencana("gpt-5", 1, 0, kandidatEgress("utama", "pool-sehat"), kandidat("cadangan")),
		opPencatat(&dipanggil))

	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses", out.Err)
	}
	if len(dipanggil) != 1 || dipanggil[0] != "utama" {
		t.Fatalf("kandidat yang dicoba = %v, mau [utama]", dipanggil)
	}
}
