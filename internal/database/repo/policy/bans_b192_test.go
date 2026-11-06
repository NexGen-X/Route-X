package policy

import (
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestValidasiSubjekBanB192 mengunci validasi bentuk subjek per jenis
// (B-192, diperluas B-269): baris ip/ip_range bersubjek tak sah tidak
// boleh pernah lahir, karena cast atas subject di query Authenticate
// akan melempar error dan menggagalkan autentikasi key lain yang sah.
// B-269 menambahkan bentuk ip_range ber-bit-host (`10.0.0.1/24`) —
// ParsePrefix menerimanya tetapi cast ::cidr PostgreSQL menolaknya.
func TestValidasiSubjekBanB192(t *testing.T) {
	uuid := "8f1f0b6e-0000-4000-8000-000000000001"
	sah := []struct{ kind, subject string }{
		{BanSubjectIP, "203.0.113.7"},
		{BanSubjectIP, "2001:db8::1"},
		{BanSubjectIPRange, "203.0.113.0/24"},
		{BanSubjectIPRange, "2001:db8::/32"},
		{BanSubjectIPRange, "10.0.0.1/32"},     // prefiks satu host: p == p.Masked()
		{BanSubjectIPRange, "2001:db8::1/128"}, // prefiks satu host v6
		{BanSubjectAPIKey, uuid},
		{BanSubjectUser, uuid},
	}
	for _, c := range sah {
		if err := validasiSubjekBan(c.kind, c.subject); err != nil {
			t.Errorf("validasiSubjekBan(%q, %q) = %v, mau sah", c.kind, c.subject, err)
		}
	}

	takSah := []struct{ kind, subject string }{
		{BanSubjectIP, "bukan-alamat-ip"},
		{BanSubjectIP, "203.0.113.0/24"},   // notasi CIDR untuk jenis ip
		{BanSubjectIP, "fe80::1%eth0"},     // zona tak dikenal inet polos
		{BanSubjectIP, "999.1.1.1"},        // oktet di luar rentang
		{BanSubjectIPRange, "203.0.113.7"}, // tanpa prefiks
		{BanSubjectIPRange, "203.0.113.0/33"},
		{BanSubjectIPRange, "bukan-cidr"},
		{BanSubjectIPRange, "10.0.0.1/24"},    // bit host terisi (B-269): cast ::cidr menolaknya
		{BanSubjectIPRange, "203.0.113.7/24"}, // bit host terisi
		{BanSubjectIPRange, "2001:db8::1/32"}, // bit host terisi v6
		{BanSubjectAPIKey, "bukan-uuid"},
		{BanSubjectAPIKey, "203.0.113.7"},
		{BanSubjectUser, "nama-pengguna"},
		{"jenis_ngawur", "203.0.113.7"},
		{BanSubjectIP, ""},
		{BanSubjectIP, "   "},
	}
	for _, c := range takSah {
		err := validasiSubjekBan(c.kind, c.subject)
		if err == nil {
			t.Errorf("validasiSubjekBan(%q, %q) = nil, mau ditolak", c.kind, c.subject)
			continue
		}
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("validasiSubjekBan(%q, %q) = %v, mau membungkus ErrInvalidReference", c.kind, c.subject, err)
		}
	}
}

// TestCreateBanMenolakSubjekTakSahB192 membuktikan penjagaannya hidup di
// titik tulis repo: CreateBan menolak subjek tak sah dengan
// ErrInvalidReference (yang dipetakan handler admin menjadi 400), dan
// subjek sah tetap bisa dibuat persis seperti sebelumnya.
func TestCreateBanMenolakSubjekTakSahB192(t *testing.T) {
	ctx, r := newRepo(t)

	for _, c := range []CreateBanParams{
		{SubjectKind: BanSubjectIP, Subject: "bukan-alamat-ip", Reason: "uji"},
		{SubjectKind: BanSubjectIPRange, Subject: "10.0.0.0/33", Reason: "uji"},
		{SubjectKind: BanSubjectIPRange, Subject: "10.0.0.1/24", Reason: "uji"}, // bit host (B-269)
		{SubjectKind: BanSubjectAPIKey, Subject: "bukan-uuid", Reason: "uji"},
		{SubjectKind: BanSubjectUser, Subject: "", Reason: "uji"},
	} {
		if _, err := r.CreateBan(ctx, c); !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("CreateBan(%s/%q) = %v, mau ErrInvalidReference", c.SubjectKind, c.Subject, err)
		}
	}

	if _, err := r.CreateBan(ctx, CreateBanParams{
		SubjectKind: BanSubjectIPRange, Subject: "198.51.100.0/24", Reason: "uji sah",
	}); err != nil {
		t.Errorf("CreateBan subjek sah ditolak: %v", err)
	}
}
