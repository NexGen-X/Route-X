package identity

import (
	"testing"
	"time"
)

// --- B-042 / V-AU-11: kedaluwarsa geser dengan batas mutlak ------------------

// Touch dengan TTL positif menggeser kedaluwarsa sesi yang sedang dipakai,
// bukan hanya memperbarui jejak last_seen_at seperti sebelumnya.
func TestTouchMenggeserKedaluwarsa(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	user := makeUser(ctx, t, users, "geser@routex.test")

	created, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Persempit sisa umur menjadi ±10 menit, seperti sesi yang hampir habis.
	if _, err := db.Pool.Exec(ctx, `update sessions set expires_at = now() + interval '10 minutes' where id = $1`, created.ID); err != nil {
		t.Fatalf("menyetel expires_at: %v", err)
	}

	if err := sessions.Touch(ctx, created.ID, 12*time.Hour); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, err := sessions.Lookup(ctx, created.Token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if sisa := time.Until(got.ExpiresAt); sisa < 11*time.Hour {
		t.Errorf("sisa umur sesudah Touch = %v, mau sekitar 12 jam (geser)", sisa)
	}
}

// Penggeseran tidak pernah melewati created_at + MaxSessionLifetime: sesi
// yang dibuat hampir 7 hari lalu hanya boleh diperpanjang sampai batas itu,
// bukan 12 jam penuh dari sekarang.
func TestTouchDibatasiUmurMutlak(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	user := makeUser(ctx, t, users, "mutlak@routex.test")

	created, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Mundurkan pembuatan ke 6 hari 23 jam lalu; kedaluwarsa disisakan 30
	// menit dari sekarang supaya sesinya masih berlaku untuk disentuh.
	if _, err := db.Pool.Exec(ctx, `update sessions set
		created_at = now() - interval '6 days 23 hours',
		expires_at = now() + interval '30 minutes'
		where id = $1`, created.ID); err != nil {
		t.Fatalf("memundurkan sesi: %v", err)
	}

	if err := sessions.Touch(ctx, created.ID, 12*time.Hour); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, err := sessions.Lookup(ctx, created.Token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	// Batas mutlaknya = pembuatan + 7 hari = sekitar 1 jam dari sekarang.
	if sisa := time.Until(got.ExpiresAt); sisa > 2*time.Hour {
		t.Errorf("sisa umur = %v, mau mendekati 1 jam (batas mutlak 7 hari), bukan 12 jam", sisa)
	}
	if sisa := time.Until(got.ExpiresAt); sisa < 30*time.Minute {
		t.Errorf("sisa umur = %v, penggeseran tidak boleh MEMENDEKKAN sesi", sisa)
	}
}

// --- B-042 / V-AU-05 & V-AU-06: RevokeOldestBeyond dan RevokeAll -------------

func TestRevokeOldestBeyondDanRevokeAll(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	user := makeUser(ctx, t, users, "batas@routex.test")
	other := makeUser(ctx, t, users, "lain@routex.test")

	var ids []string
	for i := 0; i < 5; i++ {
		c, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		ids = append(ids, c.ID)
		// Bedakan created_at secara eksplisit supaya urutan "terbaru" pasti,
		// tidak bergantung pada resolusi jam di dalam satu milidetik.
		if _, err := db.Pool.Exec(ctx, `update sessions set created_at = now() + make_interval(secs => $2) where id = $1`,
			c.ID, float64(i)); err != nil {
			t.Fatalf("menyetel created_at: %v", err)
		}
	}
	if _, err := sessions.Create(ctx, NewSession{UserID: other.ID, TTL: time.Hour}); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	evicted, err := sessions.RevokeOldestBeyond(ctx, user.ID, 2)
	if err != nil {
		t.Fatalf("RevokeOldestBeyond: %v", err)
	}
	if evicted != 3 {
		t.Errorf("terusir = %d, mau 3", evicted)
	}
	active, err := sessions.ListActive(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("aktif = %d, mau 2", len(active))
	}
	// Yang bertahan adalah dua yang terbaru (ids[3], ids[4]).
	survivors := map[string]bool{active[0].ID: true, active[1].ID: true}
	for _, want := range ids[3:] {
		if !survivors[want] {
			t.Errorf("sesi %s seharusnya bertahan", want)
		}
	}
	// Pengguna lain tidak tersentuh oleh batas per pengguna ini.
	if otherActive, err := sessions.ListActive(ctx, other.ID); err != nil || len(otherActive) != 1 {
		t.Errorf("sesi pengguna lain = %d (err %v), mau 1", len(otherActive), err)
	}

	// RevokeAll mencabut lintas pengguna: 2 sisa user + 1 other = 3.
	total, err := sessions.RevokeAll(ctx)
	if err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}
	if total != 3 {
		t.Errorf("RevokeAll mencabut %d, mau 3", total)
	}
	if again, err := sessions.RevokeAll(ctx); err != nil || again != 0 {
		t.Errorf("RevokeAll kedua = %d (err %v), mau 0", again, err)
	}
}
