package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestB326ToggleMenolakPolaRegexRusakWarisan adalah pengunci B-326:
// B-308 menutup pembuatan/pembaruan filter berregex rusak, tetapi
// SetFilterEnabled tidak memvalidasi apa pun — baris warisan
// pra-B-308 (atau hasil suntingan SQL langsung) yang polanya rusak
// bisa DINYALAKAN dari dashboard: tampil aktif, padahal mesin
// penyaring menandainya Inert dan aturan tidak menegakkan apa pun.
// Kini menyalakan memvalidasi pola efektif tersimpan dengan
// pemeriksaan yang sama seperti UpdateFilter; mematikan tidak
// pernah divalidasi (mematikan aturan rusak harus selalu bisa).
func TestB326ToggleMenolakPolaRegexRusakWarisan(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(ctx, t)
	r := New(pool)

	f, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "warisan-b326", Kind: FilterBlockedPattern, Pattern: "aman", PatternType: PatternSubstring,
	})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	// Jadikan barisnya warisan pra-B-308: pola regex rusak, tersimpan
	// dalam keadaan mati (satu-satunya jalan baris seperti ini ada).
	if _, err := pool.Exec(ctx,
		`update content_filters set pattern = '[a-', pattern_type = 'regex', enabled = false where id = $1`,
		f.ID); err != nil {
		t.Fatalf("menanam pola warisan: %v", err)
	}

	if _, err := r.SetFilterEnabled(ctx, f.ID, true); !errors.Is(err, repo.ErrInvalidParameter) {
		t.Errorf("SetFilterEnabled(true) pada pola rusak = %v, mau ErrInvalidParameter", err)
	}
	kini, err := r.GetFilter(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if kini.Enabled {
		t.Error("filter berregex rusak ikut menyala walau ditolak")
	}
	if _, err := r.SetFilterEnabled(ctx, f.ID, false); err != nil {
		t.Errorf("SetFilterEnabled(false) pada pola rusak = %v, mau tetap bisa", err)
	}

	// Kontrol: pola regex yang sah bisa dinyalakan seperti biasa.
	g, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "sehat-b326", Kind: FilterBlockedPattern, Pattern: "kartu[0-9]+", PatternType: PatternRegex,
	})
	if err != nil {
		t.Fatalf("CreateFilter sehat: %v", err)
	}
	if _, err := r.SetFilterEnabled(ctx, g.ID, false); err != nil {
		t.Fatalf("matikan filter sehat: %v", err)
	}
	if _, err := r.SetFilterEnabled(ctx, g.ID, true); err != nil {
		t.Errorf("SetFilterEnabled(true) pada pola sah = %v, mau sukses", err)
	}
}

// TestB342Ban4In6DinormalkanSaatTulis adalah pengunci B-342 (sisi
// ban): subjek ip/ip_range berbentuk 4-in-6 (::ffff:a.b.c.d) dulu
// tersimpan apa adanya, sementara sisi penegakan selalu me-Unmap
// alamat klien lebih dulu — bentuk tersimpan itu tidak akan pernah
// setara dengan alamat klien mana pun, jadi blokirnya mati senyap.
// Kini subjek dinormalkan ke bentuk IPv4 polos di titik tulis;
// prefiks 4-in-6 yang cakupannya lebih luas dari peta IPv4 (bit
// < 96) tidak punya bentuk setara dan ditolak.
func TestB342Ban4In6DinormalkanSaatTulis(t *testing.T) {
	ctx, r := newRepo(t)

	b, err := r.CreateBan(ctx, CreateBanParams{SubjectKind: BanSubjectIP, Subject: "::ffff:10.0.0.9", Reason: "uji b342"})
	if err != nil {
		t.Fatalf("CreateBan ip 4-in-6: %v", err)
	}
	if b.Subject != "10.0.0.9" {
		t.Errorf("subjek ip tersimpan = %q, mau %q (bentuk 4-in-6 tidak pernah setara alamat klien)", b.Subject, "10.0.0.9")
	}

	b, err = r.CreateBan(ctx, CreateBanParams{SubjectKind: BanSubjectIPRange, Subject: "::ffff:10.0.0.0/120", Reason: "uji b342"})
	if err != nil {
		t.Fatalf("CreateBan ip_range 4-in-6: %v", err)
	}
	if b.Subject != "10.0.0.0/24" {
		t.Errorf("subjek ip_range tersimpan = %q, mau %q", b.Subject, "10.0.0.0/24")
	}

	if _, err := r.CreateBan(ctx, CreateBanParams{SubjectKind: BanSubjectIPRange, Subject: "::ffff:0.0.0.0/95", Reason: "uji b342"}); err == nil {
		t.Error("CreateBan ip_range 4-in-6 berbit 95 lolos, mau ditolak (cakupan melampaui peta IPv4)")
	}

	// Kontrol: bentuk polos tidak berubah. (Alamatnya sengaja
	// berbeda dari kasus 4-in-6 di atas: keduanya kini subjek yang
	// SAMA sesudah normalisasi, dan indeks unik bans aktif menolak
	// duplikatnya — itu sendiri bukti normalisasinya menyatu.)
	b, err = r.CreateBan(ctx, CreateBanParams{SubjectKind: BanSubjectIP, Subject: "10.0.0.10", Reason: "kontrol"})
	if err != nil {
		t.Fatalf("CreateBan ip v4 polos: %v", err)
	}
	if b.Subject != "10.0.0.10" {
		t.Errorf("subjek ip v4 = %q, mau tetap %q", b.Subject, "10.0.0.10")
	}
	b, err = r.CreateBan(ctx, CreateBanParams{SubjectKind: BanSubjectIP, Subject: "::1", Reason: "kontrol"})
	if err != nil {
		t.Fatalf("CreateBan ip v6 polos: %v", err)
	}
	if b.Subject != "::1" {
		t.Errorf("subjek ip v6 = %q, mau tetap %q", b.Subject, "::1")
	}
	b, err = r.CreateBan(ctx, CreateBanParams{SubjectKind: BanSubjectIPRange, Subject: "2001:db8::/32", Reason: "kontrol"})
	if err != nil {
		t.Fatalf("CreateBan ip_range v6 polos: %v", err)
	}
	if b.Subject != "2001:db8::/32" {
		t.Errorf("subjek ip_range v6 = %q, mau tetap %q", b.Subject, "2001:db8::/32")
	}
	// Kontrol B-269: bit host pada ip_range v4 tetap ditolak.
	if _, err := r.CreateBan(ctx, CreateBanParams{SubjectKind: BanSubjectIPRange, Subject: "10.0.0.1/24", Reason: "kontrol"}); err == nil {
		t.Error("CreateBan ip_range berbit host lolos, mau tetap ditolak (B-269)")
	}
}
