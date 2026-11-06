package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// pelaporAuthPalsu menghitung panggilan MarkAuthFailure (B-210).
type pelaporAuthPalsu struct {
	jumlah     atomic.Int32
	terakhirID atomic.Value
}

func (p *pelaporAuthPalsu) MarkAuthFailure(_ context.Context, id string) (int, error) {
	p.jumlah.Add(1)
	p.terakhirID.Store(id)
	return int(p.jumlah.Load()), nil
}

// penyegarKredPalsu menghitung panggilan RefreshKredensial (B-210).
type penyegarKredPalsu struct {
	jumlah       atomic.Int32
	providerID   atomic.Value
	credentialID atomic.Value
}

func (p *penyegarKredPalsu) RefreshKredensial(_ context.Context, providerID, credentialID string) error {
	p.jumlah.Add(1)
	p.providerID.Store(providerID)
	p.credentialID.Store(credentialID)
	return nil
}

// serverTolakAuth menjawab semua permintaan dengan 401 berbentuk error
// OpenAI berkode invalid_api_key — penolakan auth sungguhan dari upstream.
func serverTolakAuth(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// serverGagalServer menjawab 500 — kegagalan upstream yang BUKAN auth.
func serverGagalServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"ledakan internal","type":"server_error","code":"internal"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func permintaanChatUji() *providers.ChatRequest {
	return &providers.ChatRequest{
		Model:    "m",
		Messages: []providers.Message{{Role: "user", Content: "halo"}},
	}
}

// TestFactoryPenolakanAuthMemicuCatatDanRefresh membuktikan B-210 di jalur
// pabrik: penolakan auth dari upstream kini mencatatkan kegagalan auth pada
// kredensial (MarkAuthFailure sebelumnya yatim tanpa pemanggil produksi)
// dan memicu refresh paksa sesi OAuth kredensial itu — sekali per jendela
// jeda, bukan per permintaan. Di kode lama kedua hook ini tidak ada sama
// sekali, jadi test ini tidak dapat dikompilasi di sana; bukti dua arahnya
// adalah perilaku nol-akibat yang didokumentasikan temuan.
func TestFactoryPenolakanAuthMemicuCatatDanRefresh(t *testing.T) {
	ctx := context.Background()
	srv := serverTolakAuth(t)
	pelapor := &pelaporAuthPalsu{}
	penyegar := &penyegarKredPalsu{}
	f := pabrikUji(t, kredensialUji(), nil,
		WithAuthFailureReporter(pelapor), WithOAuthCredentialRefresher(penyegar))

	adapter, err := f.Provider(ctx, kandidatUji(providers.KindOpenAI, srv.URL))
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}
	if _, err := adapter.ChatCompletion(ctx, permintaanChatUji()); err == nil {
		t.Fatal("ChatCompletion ke upstream 401 tidak menghasilkan error")
	}
	if got := pelapor.jumlah.Load(); got != 1 {
		t.Errorf("MarkAuthFailure terpanggil %d kali, ingin 1", got)
	}
	if got := pelapor.terakhirID.Load(); got != "cred-1" {
		t.Errorf("MarkAuthFailure untuk kredensial %v, ingin cred-1", got)
	}
	if got := penyegar.jumlah.Load(); got != 1 {
		t.Errorf("RefreshKredensial terpanggil %d kali, ingin 1", got)
	}
	if got := penyegar.credentialID.Load(); got != "cred-1" {
		t.Errorf("RefreshKredensial untuk kredensial %v, ingin cred-1", got)
	}
	kandidat := kandidatUji(providers.KindOpenAI, srv.URL)
	if got := penyegar.providerID.Load(); got != kandidat.ProviderID {
		t.Errorf("RefreshKredensial untuk provider %v, ingin %s", got, kandidat.ProviderID)
	}

	// Penolakan kedua dalam jendela jeda yang sama: TIDAK ada pencatatan
	// dan refresh kedua — upstream yang rusak tidak boleh mengubah setiap
	// permintaan menjadi penulisan DB + panggilan refresh.
	adapter2, err := f.Provider(ctx, kandidat)
	if err != nil {
		t.Fatalf("Provider kedua: %v", err)
	}
	if _, err := adapter2.ChatCompletion(ctx, permintaanChatUji()); err == nil {
		t.Fatal("ChatCompletion kedua ke upstream 401 tidak menghasilkan error")
	}
	if got := pelapor.jumlah.Load(); got != 1 {
		t.Errorf("MarkAuthFailure terpanggil %d kali sesudah penolakan kedua, ingin tetap 1 (gerbang jeda)", got)
	}
	if got := penyegar.jumlah.Load(); got != 1 {
		t.Errorf("RefreshKredensial terpanggil %d kali sesudah penolakan kedua, ingin tetap 1 (gerbang jeda)", got)
	}

	// Jalur stream: penolakan saat membuka arus juga memicu lingkaran yang
	// sama — di sini lewat pabrik terpisah agar gerbangnya bersih.
	pelapor2 := &pelaporAuthPalsu{}
	penyegar2 := &penyegarKredPalsu{}
	f2 := pabrikUji(t, kredensialUji(), nil,
		WithAuthFailureReporter(pelapor2), WithOAuthCredentialRefresher(penyegar2))
	adapterS, err := f2.Provider(ctx, kandidatUji(providers.KindOpenAI, srv.URL))
	if err != nil {
		t.Fatalf("Provider stream: %v", err)
	}
	if _, err := adapterS.ChatCompletionStream(ctx, permintaanChatUji()); err == nil {
		t.Fatal("ChatCompletionStream ke upstream 401 tidak menghasilkan error")
	}
	if pelapor2.jumlah.Load() != 1 || penyegar2.jumlah.Load() != 1 {
		t.Errorf("jalur stream: catat=%d refresh=%d, ingin 1/1", pelapor2.jumlah.Load(), penyegar2.jumlah.Load())
	}
}

// TestFactoryNonAuthTidakMemicuLingkaran membuktikan sisi negatif B-210:
// keberhasilan dan kegagalan non-auth tidak mencatatkan apa pun dan tidak
// memicu refresh.
func TestFactoryNonAuthTidakMemicuLingkaran(t *testing.T) {
	ctx := context.Background()
	pelapor := &pelaporAuthPalsu{}
	penyegar := &penyegarKredPalsu{}
	f := pabrikUji(t, kredensialUji(), nil,
		WithAuthFailureReporter(pelapor), WithOAuthCredentialRefresher(penyegar))

	baik, err := f.Provider(ctx, kandidatUji(providers.KindOpenAI, serverUji(t).URL))
	if err != nil {
		t.Fatalf("Provider baik: %v", err)
	}
	if _, err := baik.ChatCompletion(ctx, permintaanChatUji()); err != nil {
		t.Fatalf("ChatCompletion baik: %v", err)
	}

	runtuh, err := f.Provider(ctx, kandidatUji(providers.KindOpenAI, serverGagalServer(t).URL))
	if err != nil {
		t.Fatalf("Provider runtuh: %v", err)
	}
	if _, err := runtuh.ChatCompletion(ctx, permintaanChatUji()); err == nil {
		t.Fatal("ChatCompletion ke upstream 500 tidak menghasilkan error")
	}

	if pelapor.jumlah.Load() != 0 || penyegar.jumlah.Load() != 0 {
		t.Errorf("catat=%d refresh=%d untuk keberhasilan + kegagalan non-auth, ingin 0/0",
			pelapor.jumlah.Load(), penyegar.jumlah.Load())
	}
}

// TestFactoryTanpaHookPenolakanAuthBiasa membuktikan memasang hook bersifat
// opt-in penuh: tanpa kedua opsi, penolakan auth mengalir sebagai error
// failover biasa tanpa akibat samping apa pun.
func TestFactoryTanpaHookPenolakanAuthBiasa(t *testing.T) {
	ctx := context.Background()
	f := pabrikUji(t, kredensialUji(), nil)
	adapter, err := f.Provider(ctx, kandidatUji(providers.KindOpenAI, serverTolakAuth(t).URL))
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}
	_, err = adapter.ChatCompletion(ctx, permintaanChatUji())
	if err == nil {
		t.Fatal("ChatCompletion ke upstream 401 tidak menghasilkan error")
	}
	if pe := providers.AsError(err); pe == nil || pe.Kind != providers.ErrKindAuth {
		t.Errorf("error = %v, ingin *providers.Error berkelas auth", err)
	}
}
