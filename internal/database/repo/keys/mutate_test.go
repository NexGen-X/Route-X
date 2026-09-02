package keys

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// UUID berbentuk sah yang dijamin tidak ada di tabel mana pun.
const missingID = "00000000-0000-0000-0000-000000000000"

// fullParams adalah key dengan SETIAP setelan terisi, supaya rotasi dan pembaruan diuji
// terhadap baris yang tidak punya satu pun kolom kosong yang bisa menyembunyikan
// kesalahan penyalinan.
func fullParams(owner, admin string, expires time.Time) CreateParams {
	return CreateParams{
		Name:                "kunci pembayaran",
		OwnerUserID:         owner,
		Live:                true,
		Scopes:              []string{ScopeInference, ScopeUsageRead},
		RateLimitRPS:        ptr(7),
		RateLimitRPM:        ptr(70),
		RateLimitTPM:        ptr(7000),
		DailyRequestLimit:   ptr(int64(1000)),
		MonthlyRequestLimit: ptr(int64(20000)),
		DailyTokenLimit:     ptr(int64(500000)),
		MonthlyTokenLimit:   ptr(int64(9000000)),
		IPAllowlist: []netip.Prefix{
			netip.MustParsePrefix("203.0.113.0/24"),
			netip.MustParsePrefix("10.0.0.0/8"),
		},
		ExpiresAt: &expires,
		CreatedBy: &admin,
	}
}

func TestRotateInheritsEverySetting(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	admin := newUser(t, ctx, pool)
	modelA := newModel(t, ctx, pool, "model-a")
	modelB := newModel(t, ctx, pool, "model-b")
	provider := newProvider(t, ctx, pool, "prov-a")

	p := fullParams(owner, admin, time.Now().Add(720*time.Hour))
	p.AllowedModelIDs = []string{modelA, modelB}
	p.AllowedProviderIDs = []string{provider}
	old := mustCreate(t, ctx, r, p)

	// metadata bukan bagian dari Key, jadi diisi lewat SQL untuk memastikan kolom yang
	// tidak terlihat dari Go pun ikut tersalin.
	exec(t, ctx, pool, `update api_keys set metadata = '{"tim":"pembayaran"}'::jsonb where id = $1`, old.Key.ID)

	rotated, err := RotateKey(ctx, pool, testPepper, old.Key.ID, &admin)
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}

	// --- key pengganti mewarisi setiap setelan ---
	if rotated.Key.ID == old.Key.ID {
		t.Fatal("rotasi mengembalikan key yang sama, bukan pengganti")
	}
	if rotated.Raw.Reveal() == old.Raw.Reveal() {
		t.Fatal("nilai key pengganti sama dengan key lama")
	}
	assertSameSettings(t, rotated.Key, old.Key)

	if got := queryText(t, ctx, pool, `select rotated_from::text from api_keys where id = $1`, rotated.Key.ID); got != old.Key.ID {
		t.Errorf("rotated_from = %q, mau %q", got, old.Key.ID)
	}
	if got := queryText(t, ctx, pool, `select created_by::text from api_keys where id = $1`, rotated.Key.ID); got != admin {
		t.Errorf("created_by pengganti = %q, mau pelaku rotasi %q", got, admin)
	}
	if got := queryText(t, ctx, pool, `select metadata::text from api_keys where id = $1`, rotated.Key.ID); got != `{"tim": "pembayaran"}` {
		t.Errorf("metadata pengganti = %q, mau ikut tersalin", got)
	}

	// Daftar putih model dan provider harus sama isinya, bukan sekadar sama jumlahnya.
	for _, tc := range []struct{ name, sql string }{
		{"model", `select model_id::text from api_key_allowed_models where api_key_id = $1 order by 1`},
		{"provider", `select provider_id::text from api_key_allowed_providers where api_key_id = $1 order by 1`},
	} {
		want := queryTexts(t, ctx, pool, tc.sql, old.Key.ID)
		got := queryTexts(t, ctx, pool, tc.sql, rotated.Key.ID)
		if !slices.Equal(got, want) {
			t.Errorf("daftar putih %s pengganti = %v, mau %v", tc.name, got, want)
		}
		if len(want) == 0 {
			t.Fatalf("uji daftar putih %s tidak bermakna: key lama tidak punya pembatasan", tc.name)
		}
	}

	// --- pengganti lahir bersih ---
	if rotated.Key.Status != StatusActive {
		t.Errorf("status pengganti = %q, mau %q", rotated.Key.Status, StatusActive)
	}
	if rotated.Key.RevokedAt != nil || rotated.Key.LastUsedAt != nil {
		t.Errorf("pengganti mewarisi riwayat: revoked_at %v, last_used_at %v",
			rotated.Key.RevokedAt, rotated.Key.LastUsedAt)
	}

	// --- key lama tercabut, dengan jejak yang lengkap ---
	oldNow, err := r.GetByID(ctx, old.Key.ID)
	if err != nil {
		t.Fatalf("GetByID key lama: %v", err)
	}
	if oldNow.Status != StatusRevoked || oldNow.RevokedAt == nil {
		t.Errorf("key lama = status %q revoked_at %v, mau tercabut dengan waktu", oldNow.Status, oldNow.RevokedAt)
	}
	if got := queryText(t, ctx, pool, `select revoked_by::text from api_keys where id = $1`, old.Key.ID); got != admin {
		t.Errorf("revoked_by key lama = %q, mau %q", got, admin)
	}
	if got := queryText(t, ctx, pool, `select revoke_reason from api_keys where id = $1`, old.Key.ID); got != RevokeReasonRotated {
		t.Errorf("revoke_reason key lama = %q, mau %q", got, RevokeReasonRotated)
	}

	// --- penegakan benar-benar berpindah ke key baru ---
	inside := netip.MustParseAddr("203.0.113.9")
	_, err = r.Authenticate(ctx, old.Raw.Reveal(), inside)
	assertRejection(t, err, ReasonRevoked)

	got, err := r.Authenticate(ctx, rotated.Raw.Reveal(), inside)
	if err != nil {
		t.Fatalf("Authenticate key pengganti: %v", err)
	}
	if got.ID != rotated.Key.ID {
		t.Errorf("Authenticate mengembalikan key %q, mau %q", got.ID, rotated.Key.ID)
	}
	// ip_allowlist yang diwarisi harus benar-benar ditegakkan, bukan hanya tersimpan.
	_, err = r.Authenticate(ctx, rotated.Raw.Reveal(), netip.MustParseAddr("192.0.2.1"))
	assertRejection(t, err, ReasonIPNotAllowed)
}

func TestRotateGuards(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})

	// Repo di atas pool, bukan transaksi: rotasi harus menolak sebelum menulis apa pun.
	_, err := r.Rotate(ctx, created.Key.ID, nil)
	if !errors.Is(err, ErrNeedsTx) {
		t.Errorf("Rotate di luar transaksi: err = %v, mau ErrNeedsTx", err)
	}
	key, err := r.GetByID(ctx, created.Key.ID)
	if err != nil || key.Status != StatusActive {
		t.Errorf("key = %v (%v), mau tetap aktif setelah rotasi ditolak", key, err)
	}
	if n := queryInt(t, ctx, pool, `select count(*) from api_keys`); n != 1 {
		t.Errorf("ada %d key, mau tetap 1", n)
	}

	// Key yang sudah dicabut tidak bisa diputar: kalau bisa, satu key mati bisa
	// mengeluarkan kredensial hidup berulang kali.
	if _, err := r.Revoke(ctx, created.Key.ID, nil, "uji"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	_, err = RotateKey(ctx, pool, testPepper, created.Key.ID, nil)
	if !errors.Is(err, repo.ErrConflict) {
		t.Errorf("rotasi key tercabut: err = %v, mau ErrConflict", err)
	}
	if n := queryInt(t, ctx, pool, `select count(*) from api_keys`); n != 1 {
		t.Errorf("ada %d key setelah rotasi ditolak, mau tetap 1", n)
	}

	// Key yang tidak ada dan pengenal yang salah bentuk keduanya berarti tidak ada.
	for _, id := range []string{missingID, "bukan-uuid"} {
		if _, err := RotateKey(ctx, pool, testPepper, id, nil); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("rotasi id %q: err = %v, mau ErrNotFound", id, err)
		}
	}
}

// Rotasi menulis tiga hal: pencabutan key lama, baris key pengganti, dan salinan daftar
// putihnya. Kegagalan di tengah tidak boleh meninggalkan satu pun di antaranya.
func TestRotateIsAtomic(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	model := newModel(t, ctx, pool, "model-a")

	assertNothingHappened := func(t *testing.T, created *Created) {
		t.Helper()
		key, err := r.GetByID(ctx, created.Key.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if key.Status != StatusActive || key.RevokedAt != nil {
			t.Errorf("key lama = status %q revoked_at %v, mau tetap aktif dan belum tercabut",
				key.Status, key.RevokedAt)
		}
		if n := queryInt(t, ctx, pool, `select count(*) from api_keys where rotated_from = $1`, created.Key.ID); n != 0 {
			t.Errorf("ada %d key pengganti tertinggal setelah rotasi gagal", n)
		}
		// Key lama masih bisa dipakai klien yang sedang jalan.
		if _, err := r.Authenticate(ctx, created.Raw.Reveal(), netip.MustParseAddr("203.0.113.9")); err != nil {
			t.Errorf("key lama seharusnya masih bisa dipakai: %v", err)
		}
	}

	t.Run("gagal saat membuat pengganti", func(t *testing.T) {
		created := mustCreate(t, ctx, r, CreateParams{Name: "k1", OwnerUserID: owner, Live: true})

		// created_by yang tidak ada baru gagal pada penyisipan key pengganti, yaitu
		// SETELAH pencabutan key lama sudah tertulis di dalam transaksi. Jadi keadaan
		// akhir hanya bisa bersih kalau pencabutan itu benar-benar dibatalkan.
		_, err := RotateKey(ctx, pool, testPepper, created.Key.ID, ptr(missingID))
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Fatalf("err = %v, mau ErrInvalidReference", err)
		}
		assertNothingHappened(t, created)
	})

	t.Run("gagal saat menyalin daftar putih", func(t *testing.T) {
		created := mustCreate(t, ctx, r, CreateParams{
			Name: "k2", OwnerUserID: owner, Live: true, AllowedModelIDs: []string{model},
		})

		// Kegagalan dipaksa pada pernyataan TERAKHIR rotasi, ketika baris key pengganti
		// sudah tertulis. Tidak ada masukan yang bisa memaksanya dari luar — seluruh nilai
		// yang disalin berasal dari baris yang sudah ada dan dijamin foreign key — jadi
		// trigger inilah satu-satunya cara menguji titik kegagalan itu.
		exec(t, ctx, pool, `create function gagal_menyalin() returns trigger language plpgsql as $fn$
			begin raise exception 'kegagalan buatan untuk menguji atomisitas rotasi'; end $fn$`)
		exec(t, ctx, pool, `create trigger gagal_menyalin before insert on api_key_allowed_models
			for each row execute function gagal_menyalin()`)
		t.Cleanup(func() {
			exec(t, ctx, pool, `drop trigger gagal_menyalin on api_key_allowed_models`)
			exec(t, ctx, pool, `drop function gagal_menyalin()`)
		})

		if _, err := RotateKey(ctx, pool, testPepper, created.Key.ID, nil); err == nil {
			t.Fatal("rotasi seharusnya gagal")
		}
		assertNothingHappened(t, created)
		if got := queryTexts(t, ctx, pool,
			`select model_id::text from api_key_allowed_models where api_key_id = $1`, created.Key.ID); len(got) != 1 {
			t.Errorf("daftar putih key lama = %v, mau tetap satu baris", got)
		}
	})
}

func TestRevokeIsIdempotent(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	admin := newUser(t, ctx, pool)
	other := newUser(t, ctx, pool)
	created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})

	first, err := r.Revoke(ctx, created.Key.ID, &admin, "dilaporkan bocor di repositori publik")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if first.Status != StatusRevoked || first.RevokedAt == nil {
		t.Fatalf("hasil Revoke = status %q revoked_at %v", first.Status, first.RevokedAt)
	}

	// Pencabutan kedua oleh pelaku lain dengan alasan lain tidak boleh menimpa jejak
	// pencabutan pertama, dan tidak boleh menjadi error.
	second, err := r.Revoke(ctx, created.Key.ID, &other, "alasan susulan")
	if err != nil {
		t.Fatalf("Revoke kedua: %v", err)
	}
	if !second.RevokedAt.Equal(*first.RevokedAt) {
		t.Errorf("revoked_at berubah: %v → %v", first.RevokedAt, second.RevokedAt)
	}
	if got := queryText(t, ctx, pool, `select revoked_by::text from api_keys where id = $1`, created.Key.ID); got != admin {
		t.Errorf("revoked_by = %q, mau tetap pelaku pertama %q", got, admin)
	}
	if got := queryText(t, ctx, pool, `select revoke_reason from api_keys where id = $1`, created.Key.ID); got != "dilaporkan bocor di repositori publik" {
		t.Errorf("revoke_reason = %q, mau tetap alasan pertama", got)
	}

	// Key yang tercabut ditolak jalur autentikasi.
	_, err = r.Authenticate(ctx, created.Raw.Reveal(), netip.MustParseAddr("203.0.113.9"))
	assertRejection(t, err, ReasonRevoked)

	// Alasan dan pelaku memang boleh tidak diisi; keduanya harus tersimpan sebagai NULL,
	// bukan teks kosong.
	blank := mustCreate(t, ctx, r, CreateParams{Name: "k2", OwnerUserID: owner, Live: true})
	if _, err := r.Revoke(ctx, blank.Key.ID, nil, "   "); err != nil {
		t.Fatalf("Revoke tanpa alasan: %v", err)
	}
	if n := queryInt(t, ctx, pool, `select count(*) from api_keys
		where id = $1 and revoke_reason is null and revoked_by is null and revoked_at is not null`, blank.Key.ID); n != 1 {
		t.Error("alasan kosong seharusnya tersimpan sebagai NULL dengan waktu pencabutan tetap terisi")
	}

	if _, err := r.Revoke(ctx, missingID, nil, "uji"); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Revoke key tidak ada: err = %v, mau ErrNotFound", err)
	}
}

func TestSetStatusTransitions(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
	ip := netip.MustParseAddr("203.0.113.9")

	// active → disabled, dan penegakannya langsung terasa.
	key, err := r.SetStatus(ctx, created.Key.ID, StatusDisabled)
	if err != nil {
		t.Fatalf("SetStatus disabled: %v", err)
	}
	if key.Status != StatusDisabled {
		t.Errorf("status = %q, mau %q", key.Status, StatusDisabled)
	}
	_, err = r.Authenticate(ctx, created.Raw.Reveal(), ip)
	assertRejection(t, err, ReasonDisabled)

	// disabled → active kembali.
	key, err = r.SetStatus(ctx, created.Key.ID, StatusActive)
	if err != nil {
		t.Fatalf("SetStatus active: %v", err)
	}
	if key.Status != StatusActive {
		t.Errorf("status = %q, mau %q", key.Status, StatusActive)
	}
	if _, err := r.Authenticate(ctx, created.Raw.Reveal(), ip); err != nil {
		t.Errorf("key yang dihidupkan kembali seharusnya bisa dipakai: %v", err)
	}

	// Mencabut lewat SetStatus ditolak: pencabutan tanpa waktu, pelaku, dan alasan tidak
	// bisa dijelaskan kepada siapa pun nanti.
	if _, err := r.SetStatus(ctx, created.Key.ID, StatusRevoked); !errors.Is(err, repo.ErrConstraint) {
		t.Errorf("SetStatus revoked: err = %v, mau ErrConstraint", err)
	}
	if _, err := r.SetStatus(ctx, created.Key.ID, "deleted"); !errors.Is(err, repo.ErrConstraint) {
		t.Errorf("SetStatus status asing: err = %v, mau ErrConstraint", err)
	}
	if n := queryInt(t, ctx, pool, `select count(*) from api_keys
		where id = $1 and status = 'active' and revoked_at is null`, created.Key.ID); n != 1 {
		t.Error("key berubah padahal SetStatus ditolak")
	}

	// Pencabutan bersifat final: tidak ada status yang bisa membangkitkannya kembali.
	if _, err := r.Revoke(ctx, created.Key.ID, nil, "uji"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	for _, status := range []string{StatusActive, StatusDisabled} {
		if _, err := r.SetStatus(ctx, created.Key.ID, status); !errors.Is(err, repo.ErrConflict) {
			t.Errorf("SetStatus %q pada key tercabut: err = %v, mau ErrConflict", status, err)
		}
	}
	after, err := r.GetByID(ctx, created.Key.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.Status != StatusRevoked || after.RevokedAt == nil {
		t.Errorf("key = status %q revoked_at %v, mau tetap tercabut", after.Status, after.RevokedAt)
	}

	if _, err := r.SetStatus(ctx, missingID, StatusActive); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("SetStatus key tidak ada: err = %v, mau ErrNotFound", err)
	}
}

func TestUpdateFields(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	admin := newUser(t, ctx, pool)

	t.Run("setiap field berubah", func(t *testing.T) {
		created := mustCreate(t, ctx, r, fullParams(owner, admin, time.Now().Add(24*time.Hour)))
		expires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
		allowlist := []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}

		got, err := r.Update(ctx, created.Key.ID, UpdateParams{
			Name:                ptr("nama baru"),
			Scopes:              []string{ScopeInference, ScopeAdminRead},
			RateLimitRPS:        Set(1),
			RateLimitRPM:        Set(2),
			RateLimitTPM:        Set(3),
			DailyRequestLimit:   Set(int64(4)),
			MonthlyRequestLimit: Set(int64(5)),
			DailyTokenLimit:     Set(int64(6)),
			MonthlyTokenLimit:   Set(int64(7)),
			IPAllowlist:         allowlist,
			ExpiresAt:           Set(expires),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}

		if got.Name != "nama baru" {
			t.Errorf("Name = %q", got.Name)
		}
		if !slices.Equal(got.Scopes, []string{ScopeInference, ScopeAdminRead}) {
			t.Errorf("Scopes = %v", got.Scopes)
		}
		for _, f := range []struct {
			name string
			got  *int
			want int
		}{
			{"RateLimitRPS", got.RateLimitRPS, 1},
			{"RateLimitRPM", got.RateLimitRPM, 2},
			{"RateLimitTPM", got.RateLimitTPM, 3},
		} {
			if f.got == nil || *f.got != f.want {
				t.Errorf("%s = %s, mau %d", f.name, show(f.got), f.want)
			}
		}
		for _, f := range []struct {
			name string
			got  *int64
			want int64
		}{
			{"DailyRequestLimit", got.DailyRequestLimit, 4},
			{"MonthlyRequestLimit", got.MonthlyRequestLimit, 5},
			{"DailyTokenLimit", got.DailyTokenLimit, 6},
			{"MonthlyTokenLimit", got.MonthlyTokenLimit, 7},
		} {
			if f.got == nil || *f.got != f.want {
				t.Errorf("%s = %s, mau %d", f.name, show(f.got), f.want)
			}
		}
		if !slices.Equal(got.IPAllowlist, allowlist) {
			t.Errorf("IPAllowlist = %v, mau %v", got.IPAllowlist, allowlist)
		}
		if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expires) {
			t.Errorf("ExpiresAt = %v, mau %v", got.ExpiresAt, expires)
		}
		// Pembaruan tidak boleh menyentuh apa pun yang tidak diminta.
		if got.OwnerUserID != created.Key.OwnerUserID || got.Prefix != created.Key.Prefix || got.Status != StatusActive {
			t.Errorf("kolom di luar permintaan berubah: %+v", got)
		}
	})

	// Inti dari Opt: "jangan diubah" dan "kosongkan" adalah dua permintaan berbeda pada
	// kolom yang sama, dan pointer biasa tidak bisa membedakannya.
	t.Run("tidak diubah berbeda dari dikosongkan", func(t *testing.T) {
		created := mustCreate(t, ctx, r, fullParams(owner, admin, time.Now().Add(24*time.Hour)))

		// (1) Opt kosong: batas dan expires_at harus utuh setelah mengubah nama saja.
		got, err := r.Update(ctx, created.Key.ID, UpdateParams{Name: ptr("hanya nama")})
		if err != nil {
			t.Fatalf("Update nama: %v", err)
		}
		assertSameSettingsExceptName(t, got, created.Key)

		// (2) Clear pada satu batas: batas itu menjadi NULL, yang lain tidak ikut hilang.
		got, err = r.Update(ctx, created.Key.ID, UpdateParams{
			RateLimitRPS:    Clear[int](),
			DailyTokenLimit: Clear[int64](),
		})
		if err != nil {
			t.Fatalf("Update clear: %v", err)
		}
		if got.RateLimitRPS != nil {
			t.Errorf("RateLimitRPS = %s, mau nil (tanpa batas)", show(got.RateLimitRPS))
		}
		if got.DailyTokenLimit != nil {
			t.Errorf("DailyTokenLimit = %s, mau nil", show(got.DailyTokenLimit))
		}
		if !eqPtr(got.RateLimitRPM, created.Key.RateLimitRPM) || !eqPtr(got.MonthlyTokenLimit, created.Key.MonthlyTokenLimit) {
			t.Errorf("batas lain ikut berubah: rpm %s, monthly_token %s",
				show(got.RateLimitRPM), show(got.MonthlyTokenLimit))
		}
		if !eqTime(got.ExpiresAt, created.Key.ExpiresAt) {
			t.Errorf("ExpiresAt = %v, mau tidak tersentuh", got.ExpiresAt)
		}

		// (3) Batas yang sudah NULL tetap NULL bila tidak diminta berubah, dan bisa diisi
		// kembali dengan Set.
		got, err = r.Update(ctx, created.Key.ID, UpdateParams{Name: ptr("lagi")})
		if err != nil {
			t.Fatalf("Update nama kedua: %v", err)
		}
		if got.RateLimitRPS != nil {
			t.Errorf("RateLimitRPS = %s, mau tetap nil", show(got.RateLimitRPS))
		}
		got, err = r.Update(ctx, created.Key.ID, UpdateParams{RateLimitRPS: Set(99)})
		if err != nil {
			t.Fatalf("Update set: %v", err)
		}
		if got.RateLimitRPS == nil || *got.RateLimitRPS != 99 {
			t.Errorf("RateLimitRPS = %s, mau 99", show(got.RateLimitRPS))
		}

		// (4) expires_at dikosongkan berarti key tidak pernah kedaluwarsa.
		got, err = r.Update(ctx, created.Key.ID, UpdateParams{ExpiresAt: Clear[time.Time]()})
		if err != nil {
			t.Fatalf("Update clear expires: %v", err)
		}
		if got.ExpiresAt != nil {
			t.Errorf("ExpiresAt = %v, mau nil", got.ExpiresAt)
		}

		// (5) Slice: nil berarti biarkan, slice kosong berarti kosongkan. ip_allowlist yang
		// dikosongkan harus benar-benar membuka key dari alamat mana saja.
		outside := netip.MustParseAddr("192.0.2.1")
		_, err = r.Authenticate(ctx, created.Raw.Reveal(), outside)
		assertRejection(t, err, ReasonIPNotAllowed)

		got, err = r.Update(ctx, created.Key.ID, UpdateParams{IPAllowlist: []netip.Prefix{}, Scopes: []string{}})
		if err != nil {
			t.Fatalf("Update kosongkan slice: %v", err)
		}
		if len(got.IPAllowlist) != 0 || len(got.Scopes) != 0 {
			t.Errorf("IPAllowlist = %v, Scopes = %v; mau keduanya kosong", got.IPAllowlist, got.Scopes)
		}
		if _, err := r.Authenticate(ctx, created.Raw.Reveal(), outside); err != nil {
			t.Errorf("daftar putih kosong seharusnya menerima alamat mana saja: %v", err)
		}
	})

	t.Run("tanpa perubahan ditolak", func(t *testing.T) {
		created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
		if _, err := r.Update(ctx, created.Key.ID, UpdateParams{}); !errors.Is(err, ErrNoChanges) {
			t.Errorf("err = %v, mau ErrNoChanges", err)
		}
	})

	t.Run("nilai yang melanggar aturan tabel ditolak", func(t *testing.T) {
		created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
		for _, tc := range []struct {
			name string
			p    UpdateParams
		}{
			{"batas nol", UpdateParams{RateLimitRPS: Set(0)}},
			{"batas negatif", UpdateParams{DailyTokenLimit: Set(int64(-1))}},
			{"nama kosong", UpdateParams{Name: ptr("   ")}},
			{"cakupan asing", UpdateParams{Scopes: []string{"root:everything"}}},
		} {
			if _, err := r.Update(ctx, created.Key.ID, tc.p); !errors.Is(err, repo.ErrConstraint) {
				t.Errorf("%s: err = %v, mau ErrConstraint", tc.name, err)
			}
		}
	})

	t.Run("key tidak ada", func(t *testing.T) {
		if _, err := r.Update(ctx, missingID, UpdateParams{Name: ptr("x")}); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("err = %v, mau ErrNotFound", err)
		}
	})
}

func TestDeleteRemovesKeyPermanently(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	model := newModel(t, ctx, pool, "model-a")

	old := mustCreate(t, ctx, r, CreateParams{
		Name: "k", OwnerUserID: owner, Live: true, AllowedModelIDs: []string{model},
	})
	successor, err := RotateKey(ctx, pool, testPepper, old.Key.ID, nil)
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}

	if err := r.Delete(ctx, old.Key.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByID(ctx, old.Key.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetByID setelah Delete: err = %v, mau ErrNotFound", err)
	}
	if n := queryInt(t, ctx, pool, `select count(*) from api_key_allowed_models where api_key_id = $1`, old.Key.ID); n != 0 {
		t.Errorf("%d baris daftar putih tertinggal, mau 0 (on delete cascade)", n)
	}
	// Penerusnya tetap ada dan tetap bisa dipakai, tetapi jejak rotasinya terputus —
	// akibat on delete set null yang didokumentasikan pada Delete.
	if got := queryText(t, ctx, pool, `select rotated_from::text from api_keys where id = $1`, successor.Key.ID); got != "" {
		t.Errorf("rotated_from penerus = %q, mau NULL setelah pendahulunya dihapus", got)
	}
	if _, err := r.Authenticate(ctx, successor.Raw.Reveal(), netip.MustParseAddr("203.0.113.9")); err != nil {
		t.Errorf("penerus seharusnya tidak terpengaruh: %v", err)
	}
	if n := queryInt(t, ctx, pool, `select count(*) from api_key_allowed_models where api_key_id = $1`, successor.Key.ID); n != 1 {
		t.Errorf("daftar putih penerus = %d baris, mau 1", n)
	}

	// Menghapus dua kali dan menghapus pengenal yang salah bentuk sama-sama ErrNotFound.
	for _, id := range []string{old.Key.ID, missingID, "bukan-uuid"} {
		if err := r.Delete(ctx, id); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Delete %q: err = %v, mau ErrNotFound", id, err)
		}
	}
}

func TestExpireDue(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	create := func(name string, expires *time.Time) *Created {
		return mustCreate(t, ctx, r, CreateParams{
			Name: name, OwnerUserID: owner, Live: true, ExpiresAt: expires,
		})
	}
	due := create("kedaluwarsa", &past)
	upcoming := create("belum kedaluwarsa", &future)
	perpetual := create("tanpa batas waktu", nil)
	revoked := create("kedaluwarsa dan tercabut", &past)
	if _, err := r.Revoke(ctx, revoked.Key.ID, nil, "uji"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	n, err := r.ExpireDue(ctx)
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if n != 1 {
		t.Errorf("ExpireDue = %d, mau 1", n)
	}

	for _, tc := range []struct {
		name string
		id   string
		want string
	}{
		{"key kedaluwarsa", due.Key.ID, StatusDisabled},
		{"key belum kedaluwarsa", upcoming.Key.ID, StatusActive},
		{"key tanpa batas waktu", perpetual.Key.ID, StatusActive},
		{"key yang sudah tercabut", revoked.Key.ID, StatusRevoked},
	} {
		key, err := r.GetByID(ctx, tc.id)
		if err != nil {
			t.Fatalf("GetByID %s: %v", tc.name, err)
		}
		if key.Status != tc.want {
			t.Errorf("%s: status = %q, mau %q", tc.name, key.Status, tc.want)
		}
	}

	// Aman diulang: tidak ada lagi baris yang cocok dengan pagarnya.
	if n, err := r.ExpireDue(ctx); err != nil || n != 0 {
		t.Errorf("ExpireDue kedua = %d, %v; mau 0, nil", n, err)
	}
}

func TestLineage(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	rotate := func(t *testing.T, id string) *Created {
		t.Helper()
		created, err := RotateKey(ctx, pool, testPepper, id, nil)
		if err != nil {
			t.Fatalf("RotateKey: %v", err)
		}
		return created
	}

	first := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
	second := rotate(t, first.Key.ID)
	third := rotate(t, second.Key.ID)
	chain := []string{first.Key.ID, second.Key.ID, third.Key.ID}

	// Rantai yang sama harus keluar dari titik mana pun: pemanggil bisa memegang key
	// pertama, terakhir, atau yang di tengah.
	t.Run("rantai penuh dari titik mana pun", func(t *testing.T) {
		for i, id := range chain {
			got, err := r.Lineage(ctx, id)
			if err != nil {
				t.Fatalf("Lineage dari mata rantai ke-%d: %v", i+1, err)
			}
			if !slices.Equal(keyIDs(got), chain) {
				t.Errorf("Lineage dari mata rantai ke-%d = %v, mau %v", i+1, keyIDs(got), chain)
			}
		}
	})

	t.Run("key tanpa rotasi hanya berisi dirinya", func(t *testing.T) {
		lone := mustCreate(t, ctx, r, CreateParams{Name: "sendiri", OwnerUserID: owner, Live: true})
		got, err := r.Lineage(ctx, lone.Key.ID)
		if err != nil {
			t.Fatalf("Lineage: %v", err)
		}
		if !slices.Equal(keyIDs(got), []string{lone.Key.ID}) {
			t.Errorf("Lineage = %v, mau hanya key itu sendiri", keyIDs(got))
		}
	})

	t.Run("key tidak ada", func(t *testing.T) {
		for _, id := range []string{missingID, "bukan-uuid"} {
			if _, err := r.Lineage(ctx, id); !errors.Is(err, repo.ErrNotFound) {
				t.Errorf("Lineage %q: err = %v, mau ErrNotFound", id, err)
			}
		}
	})

	// Siklus hanya bisa masuk lewat SQL langsung, dan justru karena itu harus ditahan:
	// tanpa batas kedalaman, satu baris rusak menahan satu koneksi pool selamanya.
	t.Run("batas kedalaman menahan siklus", func(t *testing.T) {
		exec(t, ctx, pool, `update api_keys set rotated_from = $1 where id = $2`, third.Key.ID, first.Key.ID)

		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		got, err := r.Lineage(bounded, first.Key.ID)
		if err != nil {
			t.Fatalf("Lineage pada data bersiklus: %v", err)
		}
		if !slices.Equal(keyIDs(got), chain) {
			t.Errorf("Lineage = %v, mau ketiga key tanpa pengulangan: %v", keyIDs(got), chain)
		}
		if len(got) > 2*MaxLineageDepth+1 {
			t.Errorf("Lineage mengembalikan %d baris, di atas batas kedalaman", len(got))
		}
	})
}

// Setiap kegagalan mutasi harus sampai ke pemanggil sebagai sentinel repo yang bisa
// dipetakan ke kode status HTTP, dan pesannya tidak boleh memuat hash maupun nilai key.
func TestMutationErrorsTranslatedAndDoNotLeakKey(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})

	id := created.Key.ID
	raw := created.Raw.Reveal()
	hash := security.HashAPIKey(raw, testPepper)
	body := strings.TrimPrefix(raw, security.KeyPrefixLive)

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{"Revoke key tidak ada", func() error { _, err := r.Revoke(ctx, missingID, nil, "uji"); return err }, repo.ErrNotFound},
		{"Revoke pengenal salah bentuk", func() error { _, err := r.Revoke(ctx, "bukan-uuid", nil, ""); return err }, repo.ErrNotFound},
		{"Revoke pelaku bukan UUID", func() error { _, err := r.Revoke(ctx, id, ptr("bukan-uuid"), ""); return err }, repo.ErrInvalidReference},
		{"Revoke pelaku tidak ada", func() error { _, err := r.Revoke(ctx, id, ptr(missingID), ""); return err }, repo.ErrInvalidReference},
		{"SetStatus ke revoked", func() error { _, err := r.SetStatus(ctx, id, StatusRevoked); return err }, repo.ErrConstraint},
		{"SetStatus status asing", func() error { _, err := r.SetStatus(ctx, id, "hidup"); return err }, repo.ErrConstraint},
		{"SetStatus key tidak ada", func() error { _, err := r.SetStatus(ctx, missingID, StatusActive); return err }, repo.ErrNotFound},
		{"Update batas nol", func() error { _, err := r.Update(ctx, id, UpdateParams{RateLimitRPM: Set(0)}); return err }, repo.ErrConstraint},
		{"Update tanpa perubahan", func() error { _, err := r.Update(ctx, id, UpdateParams{}); return err }, ErrNoChanges},
		{"Update key tidak ada", func() error { _, err := r.Update(ctx, missingID, UpdateParams{Name: ptr("x")}); return err }, repo.ErrNotFound},
		{"Delete key tidak ada", func() error { return r.Delete(ctx, missingID) }, repo.ErrNotFound},
		{"Lineage key tidak ada", func() error { _, err := r.Lineage(ctx, missingID); return err }, repo.ErrNotFound},
		{"Rotate di luar transaksi", func() error { _, err := r.Rotate(ctx, id, nil); return err }, ErrNeedsTx},
		{"Rotate pembuat tidak ada", func() error {
			_, err := RotateKey(ctx, pool, testPepper, id, ptr(missingID))
			return err
		}, repo.ErrInvalidReference},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("seharusnya gagal")
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, mau membungkus %v", err, tc.want)
			}
			for _, secret := range []struct{ name, value string }{
				{"hash key", hash},
				{"badan key mentah", body},
			} {
				if strings.Contains(err.Error(), secret.value) {
					t.Errorf("pesan error memuat %s: %v", secret.name, err)
				}
			}
		})
	}

	// Key tetap utuh: tidak satu pun kegagalan di atas meninggalkan perubahan.
	key, err := r.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if key.Status != StatusActive || key.RevokedAt != nil || key.RateLimitRPM != nil {
		t.Errorf("key berubah setelah rangkaian kegagalan: %+v", key)
	}
}

// Pemeriksaan yang harus selesai sebelum ada perjalanan ke database. Repo-nya dibuat tanpa
// Querier, jadi pemeriksaan yang bocor ke query akan panik alih-alih diam-diam lolos —
// dan test ini ikut jalan pada `go test -short`, tanpa PostgreSQL.
func TestMutationGuardsBeforeDatabase(t *testing.T) {
	r, err := New(nil, testPepper)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	const id = "11111111-1111-1111-1111-111111111111"

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{"Rotate di luar transaksi", func() error { _, err := r.Rotate(ctx, id, nil); return err }, ErrNeedsTx},
		{"Update tanpa perubahan", func() error { _, err := r.Update(ctx, id, UpdateParams{}); return err }, ErrNoChanges},
		{"SetStatus ke revoked", func() error { _, err := r.SetStatus(ctx, id, StatusRevoked); return err }, repo.ErrConstraint},
		{"SetStatus status asing", func() error { _, err := r.SetStatus(ctx, id, ""); return err }, repo.ErrConstraint},
		{"Revoke pengenal salah bentuk", func() error { _, err := r.Revoke(ctx, "bukan-uuid", nil, ""); return err }, repo.ErrNotFound},
		{"Revoke pelaku salah bentuk", func() error { _, err := r.Revoke(ctx, id, ptr("x"), ""); return err }, repo.ErrInvalidReference},
		{"SetStatus pengenal salah bentuk", func() error { _, err := r.SetStatus(ctx, "bukan-uuid", StatusActive); return err }, repo.ErrNotFound},
		{"Update pengenal salah bentuk", func() error {
			_, err := r.Update(ctx, "bukan-uuid", UpdateParams{Name: ptr("x")})
			return err
		}, repo.ErrNotFound},
		{"Delete pengenal salah bentuk", func() error { return r.Delete(ctx, "bukan-uuid") }, repo.ErrNotFound},
		{"Lineage pengenal salah bentuk", func() error { _, err := r.Lineage(ctx, "bukan-uuid"); return err }, repo.ErrNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, mau membungkus %v", err, tc.want)
			}
		})
	}
}

func TestIDShapeCheck(t *testing.T) {
	valid := []string{
		"11111111-1111-1111-1111-111111111111",
		"A1B2C3D4-E5F6-4789-ABCD-0123456789EF",
	}
	invalid := []string{
		"", "bukan-uuid", "11111111111111111111111111111111",
		"11111111-1111-1111-1111-11111111111",   // terlalu pendek
		"11111111-1111-1111-1111-1111111111111", // terlalu panjang
		"11111111_1111-1111-1111-111111111111",  // pemisah salah
		"11111111-1111-1111-1111-11111111111g",  // bukan hex
	}
	for _, id := range valid {
		if !idOK(id) {
			t.Errorf("idOK(%q) = false, mau true", id)
		}
	}
	for _, id := range invalid {
		if idOK(id) {
			t.Errorf("idOK(%q) = true, mau false", id)
		}
	}
}

// --- helper ---------------------------------------------------------------------------

// assertSameSettings memeriksa seluruh setelan yang wajib diwarisi key hasil rotasi.
func assertSameSettings(t *testing.T, got, want *Key) {
	t.Helper()
	if got.Name != want.Name {
		t.Errorf("Name = %q, mau %q", got.Name, want.Name)
	}
	if got.OwnerUserID != want.OwnerUserID {
		t.Errorf("OwnerUserID = %q, mau %q", got.OwnerUserID, want.OwnerUserID)
	}
	// Prefix menentukan lingkungan (live/test); rotasi tidak boleh memindahkannya.
	if got.Prefix != want.Prefix {
		t.Errorf("Prefix = %q, mau %q", got.Prefix, want.Prefix)
	}
	if !slices.Equal(got.Scopes, want.Scopes) {
		t.Errorf("Scopes = %v, mau %v", got.Scopes, want.Scopes)
	}
	for _, f := range []struct {
		name      string
		got, want *int
	}{
		{"RateLimitRPS", got.RateLimitRPS, want.RateLimitRPS},
		{"RateLimitRPM", got.RateLimitRPM, want.RateLimitRPM},
		{"RateLimitTPM", got.RateLimitTPM, want.RateLimitTPM},
	} {
		if !eqPtr(f.got, f.want) {
			t.Errorf("%s = %s, mau %s", f.name, show(f.got), show(f.want))
		}
	}
	for _, f := range []struct {
		name      string
		got, want *int64
	}{
		{"DailyRequestLimit", got.DailyRequestLimit, want.DailyRequestLimit},
		{"MonthlyRequestLimit", got.MonthlyRequestLimit, want.MonthlyRequestLimit},
		{"DailyTokenLimit", got.DailyTokenLimit, want.DailyTokenLimit},
		{"MonthlyTokenLimit", got.MonthlyTokenLimit, want.MonthlyTokenLimit},
	} {
		if !eqPtr(f.got, f.want) {
			t.Errorf("%s = %s, mau %s", f.name, show(f.got), show(f.want))
		}
	}
	if !slices.Equal(got.IPAllowlist, want.IPAllowlist) {
		t.Errorf("IPAllowlist = %v, mau %v", got.IPAllowlist, want.IPAllowlist)
	}
	if !eqTime(got.ExpiresAt, want.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, mau %v", got.ExpiresAt, want.ExpiresAt)
	}
}

// assertSameSettingsExceptName dipakai menguji "tidak diubah": hanya nama yang boleh
// berbeda, semua setelan lain harus persis seperti sebelumnya.
func assertSameSettingsExceptName(t *testing.T, got, want *Key) {
	t.Helper()
	shadow := *want
	shadow.Name = got.Name
	assertSameSettings(t, got, &shadow)
}

func keyIDs(keys []*Key) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.ID)
	}
	return out
}

func eqPtr[T comparable](a, b *T) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

// eqTime membandingkan lewat Equal, bukan ==: waktu dari PostgreSQL bisa membawa lokasi
// yang berbeda untuk saat yang sama.
func eqTime(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equal(*b)
	}
}

func show[T any](p *T) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprint(*p)
}

// queryText membaca satu kolom teks; NULL menjadi "".
func queryText(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var v *string
	if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		t.Fatalf("query: %v", err)
	}
	if v == nil {
		return ""
	}
	return *v
}

// queryTexts membaca satu kolom teks dari banyak baris.
func queryTexts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) []string {
	t.Helper()
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query: %v", err)
	}
	return out
}

func queryInt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	return n
}
