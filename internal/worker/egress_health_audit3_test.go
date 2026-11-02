package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
)

// sumberEgressPalsu adalah EgressHealthSource tiruan yang merekam pencatatan
// kesehatan per pool.
type sumberEgressPalsu struct {
	mu        sync.Mutex
	pools     []*upstream.EgressPool
	urls      map[string]security.Secret
	errs      map[string]error
	tercatat  map[string]string
	latensi   map[string]*int
	listAktif *bool
}

func (s *sumberEgressPalsu) List(_ context.Context, enabledOnly bool) ([]*upstream.EgressPool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listAktif = &enabledOnly
	return s.pools, nil
}

func (s *sumberEgressPalsu) ProxyURL(_ context.Context, id string) (security.Secret, error) {
	if err, ada := s.errs[id]; ada {
		return security.Secret(""), err
	}
	return s.urls[id], nil
}

func (s *sumberEgressPalsu) RecordHealth(_ context.Context, id string, status string, latencyMS *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tercatat == nil {
		s.tercatat = map[string]string{}
		s.latensi = map[string]*int{}
	}
	s.tercatat[id] = status
	s.latensi[id] = latencyMS
	return nil
}

// TestPeriksaEgressMencatatSemuaPool membuktikan audit3 V-EO-05 tertutup di
// lapisan worker: putaran pemeriksaan berkala kini mengunjungi SETIAP pool
// aktif dan mencatat kesehatannya — dulu RecordHealth hanya terpanggil dari
// tombol Tes manual sehingga status membeku berhari-hari.
func TestPeriksaEgressMencatatSemuaPool(t *testing.T) {
	// Server proxy pura-pura: menerima CONNECT lalu memutus — cukup untuk
	// membuktikan probe benar-benar mencoba menembus proxy (hasilnya
	// unhealthy karena TLS tidak pernah terbangun ke target).
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			hj, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()

	sumber := &sumberEgressPalsu{
		pools: []*upstream.EgressPool{
			{ID: "pool-probe"},
			{ID: "pool-rusak"},
			{ID: "pool-url-aneh"},
		},
		urls: map[string]security.Secret{
			"pool-probe":    security.Secret(proxy.URL),
			"pool-url-aneh": security.Secret("://bukan-url"),
		},
		errs: map[string]error{
			"pool-rusak": errors.New("tidak bisa didekripsi"),
		},
	}

	job := &HealthCheckerJob{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		egress: sumber,
		egressPolicy: security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: []netip.Prefix{
			netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"),
		}},
	}
	job.periksaEgress(context.Background())

	if sumber.listAktif == nil || !*sumber.listAktif {
		t.Error("List tidak dipanggil dengan enabledOnly=true")
	}
	for _, id := range []string{"pool-probe", "pool-rusak", "pool-url-aneh"} {
		status, ada := sumber.tercatat[id]
		if !ada {
			t.Errorf("pool %s tidak tercatat kesehatannya", id)
			continue
		}
		if status != "unhealthy" {
			t.Errorf("pool %s tercatat %q, ingin unhealthy pada lingkungan uji ini", id, status)
		}
	}
}
