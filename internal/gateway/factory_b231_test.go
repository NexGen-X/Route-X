package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// penyelesaiProjectBerubah adalah CodeAssistProjectResolver tiruan yang
// jawabannya berubah antar-panggilan, meniru keadaan nyata B-231: resolusi
// pertama terjadi sebelum project tersedia (sesi belum ter-backfill /
// loadCodeAssist belum menawarkan project) dan menjawab "", resolusi
// berikutnya menjawab project yang sebenarnya.
type penyelesaiProjectBerubah struct {
	mu      sync.Mutex
	jawaban []string
	panggil int
}

func (p *penyelesaiProjectBerubah) ResolveCodeAssistProject(_ context.Context, _, _ string, _ security.Secret) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.panggil
	p.panggil++
	if i >= len(p.jawaban) {
		return p.jawaban[len(p.jawaban)-1], nil
	}
	return p.jawaban[i], nil
}

// TestFactoryB231ProjectIkutKunciCache membuktikan regresi B-231 tidak
// kembali: adapter yang dibangun saat project masih "" tidak boleh
// terkunci permanen. Begitu resolver menjawab "proj-x", panggilan
// berikutnya WAJIB menghasilkan adapter baru yang benar-benar membawa
// proj-x — dibuktikan dari body fetchAvailableModels yang dikirim
// adapter ke upstream, bukan dari klaim internal.
func TestFactoryB231ProjectIkutKunciCache(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{}}`))
	}))
	defer srv.Close()

	resolver := &penyelesaiProjectBerubah{jawaban: []string{"", "proj-x"}}
	f := pabrikUji(t, kredensialUji(), nil, WithCodeAssistProjectResolver(resolver))
	target := ProviderTarget{
		ID:        "8f1f0b6e-0000-4000-8000-0000000000aa",
		Name:      "antigravity-uji",
		Kind:      providers.KindGoogle,
		BaseURL:   srv.URL,
		TimeoutMS: 3000,
	}
	ctx := context.Background()

	p1, err := f.ProviderForTarget(ctx, target)
	if err != nil {
		t.Fatalf("ProviderForTarget pertama: %v", err)
	}
	if _, err := p1.Models(ctx); err != nil {
		t.Fatalf("Models pada adapter pertama: %v", err)
	}

	p2, err := f.ProviderForTarget(ctx, target)
	if err != nil {
		t.Fatalf("ProviderForTarget kedua: %v", err)
	}
	if p2 == p1 {
		t.Fatalf("adapter kedua adalah instance yang sama dengan adapter tanpa project: " +
			"project tidak ikut kunci cache (regresi B-231)")
	}
	if _, err := p2.Models(ctx); err != nil {
		t.Fatalf("Models pada adapter kedua: %v", err)
	}

	// Project yang stabil harus kembali ke cache hit: adapter ketiga
	// adalah instance yang sama dengan adapter kedua.
	p3, err := f.ProviderForTarget(ctx, target)
	if err != nil {
		t.Fatalf("ProviderForTarget ketiga: %v", err)
	}
	if p3 != p2 {
		t.Fatalf("adapter ketiga berbeda dari adapter kedua padahal project tidak berubah: cache tidak stabil")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("upstream menerima %d panggilan Models, mau 2: %v", len(bodies), bodies)
	}
	if strings.Contains(bodies[0], "proj-x") {
		t.Fatalf("body adapter pertama memuat proj-x padahal resolusi masih kosong: %s", bodies[0])
	}
	if !strings.Contains(bodies[1], `"project":"proj-x"`) {
		t.Fatalf("body adapter kedua tidak membawa project terselesaikan: %s", bodies[1])
	}
}

// TestFactoryB231KunciBerbedaUntukProjectBerbeda mengunci lapis kunci
// secara langsung: dua project berbeda tidak boleh berbagi kunci, dan
// project yang sama harus menghasilkan kunci yang sama (deterministik).
func TestFactoryB231KunciBerbedaUntukProjectBerbeda(t *testing.T) {
	target := ProviderTarget{
		ID:        "8f1f0b6e-0000-4000-8000-0000000000aa",
		Name:      "antigravity-uji",
		Kind:      providers.KindGoogle,
		BaseURL:   "https://cloudcode-pa.googleapis.com",
		TimeoutMS: 3000,
	}
	tanpa := kunciAdapterTarget(target, nil, "", "")
	dengan := kunciAdapterTarget(target, nil, "", "proj-x")
	denganLagi := kunciAdapterTarget(target, nil, "", "proj-x")
	proyekLain := kunciAdapterTarget(target, nil, "", "proj-y")
	if tanpa == dengan {
		t.Fatalf("kunci adapter tanpa project sama dengan kunci ber-project: %s", tanpa)
	}
	if dengan != denganLagi {
		t.Fatalf("kunci tidak deterministik untuk project yang sama: %s vs %s", dengan, denganLagi)
	}
	if dengan == proyekLain {
		t.Fatalf("kunci sama untuk dua project berbeda: %s", dengan)
	}
}
