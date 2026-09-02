package identity

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Nama aksi ikut tersimpan di database dan dipakai menyaring, jadi tidak boleh ada dua
// konstanta yang bernilai sama — kesalahan seperti itu membuat dua kejadian berbeda
// tidak bisa lagi dipisahkan setelah tercatat.
func TestAuditActionConstantsAreDistinct(t *testing.T) {
	actions := map[string]string{
		"ActionLogin":          ActionLogin,
		"ActionLogout":         ActionLogout,
		"ActionLoginFailed":    ActionLoginFailed,
		"ActionProviderCreate": ActionProviderCreate,
		"ActionProviderUpdate": ActionProviderUpdate,
		"ActionProviderDelete": ActionProviderDelete,
		"ActionAPIKeyCreate":   ActionAPIKeyCreate,
		"ActionAPIKeyRotate":   ActionAPIKeyRotate,
		"ActionAPIKeyRevoke":   ActionAPIKeyRevoke,
		"ActionRoutingUpdate":  ActionRoutingUpdate,
		"ActionSettingUpdate":  ActionSettingUpdate,
	}

	seen := make(map[string]string, len(actions))
	for name, value := range actions {
		if value == "" {
			t.Errorf("%s kosong", name)
		}
		if !strings.Contains(value, ".") {
			t.Errorf("%s = %q, ingin berbentuk sumber_daya.aksi", name, value)
		}
		if prev, dup := seen[value]; dup {
			t.Errorf("%s dan %s bernilai sama: %q", prev, name, value)
		}
		seen[value] = name
	}

	resources := map[string]string{
		"ResourceUser":     ResourceUser,
		"ResourceRole":     ResourceRole,
		"ResourceSession":  ResourceSession,
		"ResourceProvider": ResourceProvider,
		"ResourceAPIKey":   ResourceAPIKey,
		"ResourceRouting":  ResourceRouting,
		"ResourceSetting":  ResourceSetting,
	}
	seen = make(map[string]string, len(resources))
	for name, value := range resources {
		if prev, dup := seen[value]; dup {
			t.Errorf("%s dan %s bernilai sama: %q", prev, name, value)
		}
		seen[value] = name
	}
}

// Jalur bahagia penulisan dan pembacaan audit log.
func TestAuditIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	audit := NewAudit(db.Pool)
	actorUser := makeUser(ctx, t, users, "pelaku@routex.test")

	event := Event{
		Actor:        ActorOf(actorUser, "Admin"),
		Action:       ActionProviderCreate,
		ResourceType: ResourceProvider,
		ResourceID:   "openai-utama",
		IP:           "198.51.100.4",
		UserAgent:    "Mozilla/5.0 (uji)",
		RequestID:    "req-abc-123",
		Metadata:     map[string]any{"base_url": "https://api.openai.com", "enabled": true},
	}
	if err := audit.Write(ctx, event); err != nil {
		t.Fatalf("Write: %v", err)
	}

	page, err := audit.List(ctx, AuditFilter{}, repo.Page{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("jumlah catatan = %d, ingin 1", len(page.Entries))
	}
	got := page.Entries[0]

	t.Run("catatan memuat seluruh masukan", func(t *testing.T) {
		if got.ID == 0 || got.OccurredAt.IsZero() {
			t.Errorf("id/occurred_at tidak terisi: %+v", got)
		}
		if got.ActorUserID != actorUser.ID || got.ActorEmail != actorUser.Email || got.ActorRole != "Admin" {
			t.Errorf("cuplikan pelaku salah: %+v", got)
		}
		if got.Action != ActionProviderCreate || got.ResourceType != ResourceProvider || got.ResourceID != "openai-utama" {
			t.Errorf("aksi/sumber daya salah: %+v", got)
		}
		if got.IP != "198.51.100.4" || got.UserAgent != "Mozilla/5.0 (uji)" || got.RequestID != "req-abc-123" {
			t.Errorf("jejak request salah: %+v", got)
		}
		if !strings.Contains(string(got.Metadata), `"base_url"`) {
			t.Errorf("Metadata = %s, ingin memuat base_url", got.Metadata)
		}
	})

	t.Run("metadata kosong menjadi objek JSON kosong", func(t *testing.T) {
		if err := audit.Write(ctx, Event{
			Action:       ActionSettingUpdate,
			ResourceType: ResourceSetting,
		}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		page, err := audit.List(ctx, AuditFilter{Action: ActionSettingUpdate}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(page.Entries) != 1 {
			t.Fatalf("jumlah catatan = %d, ingin 1", len(page.Entries))
		}
		if string(page.Entries[0].Metadata) != "{}" {
			t.Errorf("Metadata = %s, ingin {}", page.Entries[0].Metadata)
		}
		if page.Entries[0].ActorUserID != "" || page.Entries[0].ActorEmail != "" {
			t.Errorf("pelaku terisi padahal aksinya oleh sistem: %+v", page.Entries[0])
		}
	})

	t.Run("catatan tanpa aksi atau jenis sumber daya ditolak", func(t *testing.T) {
		cases := map[string]Event{
			"tanpa aksi":              {ResourceType: ResourceProvider},
			"aksi hanya spasi":        {Action: "   ", ResourceType: ResourceProvider},
			"tanpa jenis sumber daya": {Action: ActionLogin},
			"pelaku bukan UUID":       {Actor: Actor{UserID: "bukan-uuid"}, Action: ActionLogin, ResourceType: ResourceSession},
		}
		for name, e := range cases {
			t.Run(name, func(t *testing.T) {
				if err := audit.Write(ctx, e); !errors.Is(err, ErrInvalidInput) {
					t.Errorf("Write = %v, ingin ErrInvalidInput", err)
				}
			})
		}
	})

	t.Run("pelaku yang tidak ada dilaporkan", func(t *testing.T) {
		err := audit.Write(ctx, Event{
			Actor:        Actor{UserID: "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c", Email: "hantu@routex.test"},
			Action:       ActionLogin,
			ResourceType: ResourceSession,
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("Write = %v, ingin ErrInvalidReference", err)
		}
	})

	// Inilah alasan cuplikan email dan peran ada di tabel ini.
	t.Run("cuplikan pelaku tetap ada setelah akunnya dihapus", func(t *testing.T) {
		if err := users.Delete(ctx, actorUser.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		page, err := audit.List(ctx, AuditFilter{Action: ActionProviderCreate}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(page.Entries) != 1 {
			t.Fatalf("catatan audit ikut terhapus bersama akunnya")
		}
		got := page.Entries[0]
		if got.ActorUserID != "" {
			t.Errorf("ActorUserID = %q, ingin kosong karena akunnya sudah dihapus", got.ActorUserID)
		}
		if got.ActorEmail != actorUser.Email || got.ActorRole != "Admin" {
			t.Errorf("cuplikan pelaku hilang: email=%q role=%q", got.ActorEmail, got.ActorRole)
		}
	})
}

// Penyaringan dan paginasi audit log.
func TestAuditListFiltersIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	audit := NewAudit(db.Pool)

	alice := makeUser(ctx, t, users, "alice@routex.test")
	bob := makeUser(ctx, t, users, "bob@routex.test")

	events := []Event{
		{Actor: ActorOf(alice, "Admin"), Action: ActionLogin, ResourceType: ResourceSession},
		{Actor: ActorOf(alice, "Admin"), Action: ActionProviderCreate, ResourceType: ResourceProvider, ResourceID: "openai"},
		{Actor: ActorOf(alice, "Admin"), Action: ActionProviderUpdate, ResourceType: ResourceProvider, ResourceID: "openai"},
		{Actor: ActorOf(bob, "Operator"), Action: ActionProviderUpdate, ResourceType: ResourceProvider, ResourceID: "anthropic"},
		{Actor: ActorOf(bob, "Operator"), Action: ActionLoginFailed, ResourceType: ResourceSession},
	}
	for i, e := range events {
		if err := audit.Write(ctx, e); err != nil {
			t.Fatalf("Write ke-%d: %v", i, err)
		}
	}

	cases := []struct {
		name   string
		filter AuditFilter
		want   int
	}{
		{"tanpa filter", AuditFilter{}, 5},
		{"per pelaku", AuditFilter{ActorUserID: alice.ID}, 3},
		{"per aksi", AuditFilter{Action: ActionProviderUpdate}, 2},
		{"per jenis sumber daya", AuditFilter{ResourceType: ResourceSession}, 2},
		{"per sumber daya tertentu", AuditFilter{ResourceType: ResourceProvider, ResourceID: "openai"}, 2},
		{"gabungan pelaku dan aksi", AuditFilter{ActorUserID: bob.ID, Action: ActionProviderUpdate}, 1},
		{"tidak ada yang cocok", AuditFilter{Action: "tidak.pernah"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := audit.List(ctx, tc.filter, repo.Page{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(page.Entries) != tc.want {
				t.Errorf("jumlah catatan = %d, ingin %d", len(page.Entries), tc.want)
			}
		})
	}

	t.Run("pelaku bukan UUID ditolak", func(t *testing.T) {
		if _, err := audit.List(ctx, AuditFilter{ActorUserID: "bukan-uuid"}, repo.Page{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("List = %v, ingin ErrInvalidInput", err)
		}
	})

	t.Run("rentang waktu", func(t *testing.T) {
		// Dua catatan dimundurkan supaya rentang waktu bisa diuji tanpa menunggu.
		if _, err := db.Pool.Exec(ctx, `update audit_logs set occurred_at = now() - interval '2 hours'
			where action = $1`, ActionLogin); err != nil {
			t.Fatalf("memundurkan occurred_at: %v", err)
		}

		now := time.Now()
		recent, err := audit.List(ctx, AuditFilter{From: now.Add(-time.Hour)}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(recent.Entries) != 4 {
			t.Errorf("catatan sejak satu jam lalu = %d, ingin 4", len(recent.Entries))
		}

		old, err := audit.List(ctx, AuditFilter{To: now.Add(-time.Hour)}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(old.Entries) != 1 {
			t.Errorf("catatan sebelum satu jam lalu = %d, ingin 1", len(old.Entries))
		}

		none, err := audit.List(ctx, AuditFilter{From: now.Add(time.Hour), To: now.Add(2 * time.Hour)}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(none.Entries) != 0 {
			t.Errorf("catatan di masa depan = %d, ingin 0", len(none.Entries))
		}
	})
}

// Paginasi keyset audit log. Di tabel ini penanda waktu beberapa baris bisa sama persis
// karena catatan ditulis dalam satu tarikan, jadi id ikut menjadi pemecah seri —
// tanpa itu, urutan antar halaman tidak stabil dan baris bisa terlewat.
func TestAuditListKeysetPaginationIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	audit := NewAudit(db.Pool)
	actor := makeUser(ctx, t, users, "banyak@routex.test")

	const (
		total    = 9
		pageSize = 4
	)
	for i := range total {
		if err := audit.Write(ctx, Event{
			Actor:        ActorOf(actor, "Admin"),
			Action:       ActionSettingUpdate,
			ResourceType: ResourceSetting,
			ResourceID:   fmt.Sprintf("kunci.%d", i),
		}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	seen := make(map[int64]struct{}, total)
	var (
		ids    []int64
		cursor string
	)
	for pages := 0; ; pages++ {
		if pages > total {
			t.Fatal("paginasi tidak berhenti")
		}
		page, err := audit.List(ctx, AuditFilter{}, repo.Page{Limit: pageSize, Cursor: cursor})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, e := range page.Entries {
			if _, dup := seen[e.ID]; dup {
				t.Errorf("catatan %d terbaca dua kali", e.ID)
			}
			seen[e.ID] = struct{}{}
			ids = append(ids, e.ID)
		}
		if page.NextCursor == "" {
			break
		}
		// Catatan baru menyusup di antara pengambilan halaman: tidak boleh menggeser
		// halaman-halaman berikutnya.
		if err := audit.Write(ctx, Event{
			Actor:        ActorOf(actor, "Admin"),
			Action:       ActionLogin,
			ResourceType: ResourceSession,
		}); err != nil {
			t.Fatalf("Write penyusup: %v", err)
		}
		cursor = page.NextCursor
	}

	if len(ids) != total {
		t.Fatalf("total catatan terbaca = %d, ingin %d", len(ids), total)
	}
	// Terbaru lebih dulu berarti id menurun, karena kolomnya bigserial.
	for i := 1; i < len(ids); i++ {
		if ids[i] >= ids[i-1] {
			t.Errorf("urutan id tidak menurun di posisi %d: %v", i, ids)
			break
		}
	}

	t.Run("cursor rusak dilaporkan", func(t *testing.T) {
		for _, bad := range []string{"bukan-cursor", base64Raw("123|bukan-bilangan")} {
			if _, err := audit.List(ctx, AuditFilter{}, repo.Page{Cursor: bad}); !errors.Is(err, ErrInvalidCursor) {
				t.Errorf("List dengan cursor %q = %v, ingin ErrInvalidCursor", bad, err)
			}
		}
	})
}
