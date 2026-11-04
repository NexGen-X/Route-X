package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// TestAuditLogsFilterAktorDanLimit membuktikan B-139: handler
// GET /api/admin/system/audit-logs kini membaca actor_user_id (repo
// AuditFilter.ActorUserID sudah lama mendukungnya) dan menghormati
// parameter limit alih-alih selalu memakai DefaultPageLimit.
func TestAuditLogsFilterAktorDanLimit(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login(env.adminUser.Email, testPassword)

	// Tiga catatan dari aktor viewer (admin menghasilkan catatannya sendiri
	// lewat login di atas) agar filter aktor dan limit bisa dibedakan.
	audit := identity.NewAudit(env.pool)
	for i := 0; i < 3; i++ {
		err := audit.Write(context.Background(), identity.Event{
			Actor:        identity.ActorOf(env.viewerUser, "User"),
			Action:       identity.ActionProviderUpdate,
			ResourceType: identity.ResourceProvider,
			ResourceID:   "openai",
		})
		if err != nil {
			t.Fatalf("tulis audit viewer: %v", err)
		}
	}

	ambil := func(path string) (int, ListEnvelope[AuditEntryDTO]) {
		t.Helper()
		res, body := client.do(http.MethodGet, path, nil, false)
		var envlp ListEnvelope[AuditEntryDTO]
		if res.StatusCode == http.StatusOK {
			if err := json.Unmarshal(body, &envlp); err != nil {
				t.Fatalf("urai envelope %s: %v", path, err)
			}
		}
		return res.StatusCode, envlp
	}

	// Filter aktor: hanya catatan viewer yang kembali.
	status, hasil := ambil("/api/admin/system/audit-logs?window=24h&actor_user_id=" + env.viewerUser.ID)
	if status != http.StatusOK {
		t.Fatalf("filter aktor: status %d, mau 200", status)
	}
	if len(hasil.Items) != 3 {
		t.Fatalf("filter aktor: %d catatan, mau 3 milik viewer", len(hasil.Items))
	}
	for _, it := range hasil.Items {
		if it.ActorUserID != env.viewerUser.ID {
			t.Errorf("catatan aktor %q lolos filter aktor viewer", it.ActorUserID)
		}
	}

	// Limit dihormati: limit=1 atas filter yang sama memberi 1 + kursor.
	status, hasil = ambil("/api/admin/system/audit-logs?window=24h&actor_user_id=" + env.viewerUser.ID + "&limit=1")
	if status != http.StatusOK {
		t.Fatalf("limit=1: status %d, mau 200", status)
	}
	if len(hasil.Items) != 1 {
		t.Errorf("limit=1: %d catatan, mau 1", len(hasil.Items))
	}
	if hasil.NextCursor == "" {
		t.Error("limit=1: next_cursor kosong, mau ada (masih ada sisa catatan)")
	}

	// Limit di atas plafon dijepit repo (MaxPageLimit), bukan error: semua
	// catatan viewer tetap kembali dalam satu halaman.
	status, hasil = ambil("/api/admin/system/audit-logs?window=24h&actor_user_id=" + env.viewerUser.ID + "&limit=100000")
	if status != http.StatusOK {
		t.Fatalf("limit besar: status %d, mau 200", status)
	}
	if len(hasil.Items) != 3 {
		t.Errorf("limit besar: %d catatan, mau 3", len(hasil.Items))
	}

	// Aktor bukan UUID ditolak repo sebagai input tidak sah → 400.
	status, _ = ambil("/api/admin/system/audit-logs?window=24h&actor_user_id=bukan-uuid")
	if status != http.StatusBadRequest {
		t.Errorf("actor_user_id bukan UUID: status %d, mau 400", status)
	}
}
