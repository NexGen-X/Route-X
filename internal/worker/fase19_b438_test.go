package worker

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB438BackfillProjectFactoryMemanggilCallback adalah pengunci
// B-438 sisi factory: ResolveCodeAssistProject menyimpan project
// hasil backfill malas ke sesi — konteks layan yang tidak masuk
// kunci response cache — tetapi jalur ini tidak punya jalur
// siraman apa pun, sehingga jawaban ter-cache untuk sesi tanpa
// project tetap tersaji. Kini callback SesudahBackfillProject
// (yang dipasang cmd sebagai siraman cache respons) terpanggil
// tepat sekali sesudah penyimpanan berhasil, dan project-nya
// tersimpan di sesi.
func TestB438BackfillProjectFactoryMemanggilCallback(t *testing.T) {
	env := siapkanRefreshG8(t, false)
	ctx := context.Background()

	caSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-b438-worker"}`))
	}))
	defer caSrv.Close()

	klien := oauth.NewGoogleOAuthClient(nil).WithCodeAssistEndpoint(caSrv.URL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := NewPenyegarOAuth(env.pool, env.oauthRepo, env.egress, klien, logger)

	var panggilan atomic.Int32
	p.SetSesudahBackfillProject(func(context.Context) { panggilan.Add(1) })

	proj, err := p.ResolveCodeAssistProject(ctx, env.meta.ProviderID, env.meta.CredentialID,
		security.Secret("GANTI_token_akses_b438_worker"))
	if err != nil {
		t.Fatalf("ResolveCodeAssistProject: %v", err)
	}
	if proj != "proj-b438-worker" {
		t.Fatalf("project = %q, mau proj-b438-worker", proj)
	}
	if n := panggilan.Load(); n != 1 {
		t.Errorf("callback sesudah backfill terpanggil %d kali, mau tepat 1 — jalur factory tidak menyiram cache respons (B-438)", n)
	}

	meta, err := env.oauthRepo.GetSession(ctx, env.meta.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if meta.CodeAssistProject == nil || *meta.CodeAssistProject != "proj-b438-worker" {
		t.Errorf("project tersimpan di sesi = %v, mau proj-b438-worker", meta.CodeAssistProject)
	}

	// Panggilan kedua memakai project yang sudah tersimpan
	// (tanpa backfill baru): callback TIDAK boleh terpanggil lagi.
	proj, err = p.ResolveCodeAssistProject(ctx, env.meta.ProviderID, env.meta.CredentialID,
		security.Secret("GANTI_token_akses_b438_worker"))
	if err != nil {
		t.Fatalf("ResolveCodeAssistProject kedua: %v", err)
	}
	if proj != "proj-b438-worker" {
		t.Fatalf("project kedua = %q, mau proj-b438-worker", proj)
	}
	if n := panggilan.Load(); n != 1 {
		t.Errorf("callback terpanggil %d kali sesudah resolusi tanpa backfill, mau tetap 1", n)
	}
}
