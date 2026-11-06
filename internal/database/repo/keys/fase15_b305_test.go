package keys

import (
	"net/netip"
	"testing"
)

// TestAuthenticateBanWarisanTeksSampahB305 mengunci B-305: baris ban
// warisan yang subjeknya BUKAN alamat sama sekali — di luar jangkauan
// lapis-2 B-269 yang hanya menangani bit host — sebelumnya tetap
// melempar 22P02 dari cast ::inet di Authenticate, menjatuhkan
// autentikasi SEMUA key. Baris racun ditanam langsung via SQL, persis
// pola uji B-269. Sesudah perbaikan (cast toleran routex_inet_aman,
// migrasi 0026): autentikasi normal berhasil, baris racun tidak cocok
// dengan IP mana pun, dan ban yang sah — termasuk baris ber-bit-host
// B-269 — tetap menegakkan jaringannya.
func TestAuthenticateBanWarisanTeksSampahB305(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	created := mustCreate(t, ctx, r, CreateParams{
		Name: "kunci uji", OwnerUserID: owner, Live: true,
	})
	raw := created.Raw.Reveal()

	exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('ip', 'bukan-alamat-ip', 'racun warisan B-305')`)
	exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('ip_range', '999.1.1.1/24', 'racun warisan B-305')`)
	exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('ip_range', 'teks-sampah', 'racun warisan B-305')`)

	// Autentikasi normal harus BERHASIL walau tiga baris racun aktif
	// (pra-fix: error 22P02 dari cast, bukan Rejection dan bukan sukses).
	got, err := r.Authenticate(ctx, raw, netip.MustParseAddr("198.51.100.9"))
	if err != nil {
		t.Fatalf("Authenticate dengan baris ban teks-sampah aktif: %v", err)
	}
	if got.ID != created.Key.ID {
		t.Errorf("ID = %q, mau %q", got.ID, created.Key.ID)
	}

	// Ban ip yang sah tetap menegakkan alamatnya persis.
	exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('ip', '203.0.113.9', 'ban sah')`)
	_, err = r.Authenticate(ctx, raw, netip.MustParseAddr("203.0.113.9"))
	assertRejection(t, err, ReasonBanned)

	// Baris ber-bit-host (semantik B-269) tetap menegakkan jaringan
	// termasked-nya walau baris racun ikut aktif.
	exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('ip_range', '203.0.113.1/24', 'warisan pra-B-269')`)
	_, err = r.Authenticate(ctx, raw, netip.MustParseAddr("203.0.113.20"))
	assertRejection(t, err, ReasonBanned)

	// Dan IP lain di luar semua ban tetap lolos.
	if _, err := r.Authenticate(ctx, raw, netip.MustParseAddr("198.51.100.9")); err != nil {
		t.Fatalf("Authenticate IP bersih sesudah semua baris ditanam: %v", err)
	}
}
