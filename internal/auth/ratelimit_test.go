package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/security"
)

// newLimiter menyalakan Redis tiruan lalu menyambungkannya lewat cache.Connect, sehingga
// jalur yang diuji sama dengan jalur produksi.
func newLimiter(t *testing.T, opts ...LimiterOption) (*LoginLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)

	rdb, err := cache.Connect(context.Background(), &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	return NewLoginLimiter(rdb, slog.New(slog.DiscardHandler), opts...), mr
}

// Pembatas nil dan pembatas tanpa Redis harus meloloskan semuanya, supaya deployment tanpa
// Redis memakai jalur kode yang sama.
func TestLimiterWithoutRedisAllows(t *testing.T) {
	var nilLimiter *LoginLimiter
	if ok, _ := nilLimiter.Allow(context.Background(), "203.0.113.9", "a@example.test"); !ok {
		t.Error("pembatas nil menolak percobaan")
	}
	nilLimiter.ResetEmail(context.Background(), "a@example.test")

	empty := NewLoginLimiter(nil, nil)
	for i := 0; i < 100; i++ {
		if ok, _ := empty.Allow(context.Background(), "203.0.113.9", "a@example.test"); !ok {
			t.Fatalf("percobaan %d ditolak padahal Redis tidak dikonfigurasi", i)
		}
	}
}

func TestLimiterPerIP(t *testing.T) {
	limiter, _ := newLimiter(t, WithLoginRateLimits(3, 0), WithLoginRateWindow(time.Minute))
	ctx := context.Background()

	// Email berbeda setiap kali: yang diuji hanya sumbu IP.
	for i := 0; i < 3; i++ {
		ok, retryAfter := limiter.Allow(ctx, "203.0.113.9", "orang-"+string(rune('a'+i))+"@example.test")
		if !ok {
			t.Fatalf("percobaan %d ditolak, mau lolos (retry %v)", i+1, retryAfter)
		}
	}

	ok, retryAfter := limiter.Allow(ctx, "203.0.113.9", "orang-x@example.test")
	if ok {
		t.Fatal("percobaan keempat dari IP yang sama seharusnya ditolak")
	}
	if retryAfter <= 0 || retryAfter > time.Minute {
		t.Errorf("retryAfter = %v, mau di antara 0 dan lebar jendela", retryAfter)
	}

	// IP lain tidak terpengaruh.
	if ok, _ := limiter.Allow(ctx, "198.51.100.7", "orang-x@example.test"); !ok {
		t.Error("IP lain ikut terkena batas")
	}
	// IP kosong (mis. koneksi lewat unix socket) tidak dihitung.
	if ok, _ := limiter.Allow(ctx, "", "orang-x@example.test"); !ok {
		t.Error("percobaan tanpa IP ikut terkena batas")
	}
}

func TestLimiterPerEmail(t *testing.T) {
	limiter, _ := newLimiter(t, WithLoginRateLimits(0, 2), WithLoginRateWindow(time.Minute))
	ctx := context.Background()

	// IP berbeda setiap kali: yang diuji hanya sumbu email. Inilah pola serangan yang
	// tidak tertangkap penguncian per akun bila penyerangnya berpindah-pindah alamat.
	for i, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		if ok, _ := limiter.Allow(ctx, ip, "target@example.test"); !ok {
			t.Fatalf("percobaan %d ditolak, mau lolos", i+1)
		}
	}
	if ok, _ := limiter.Allow(ctx, "203.0.113.3", "target@example.test"); ok {
		t.Fatal("percobaan ketiga untuk email yang sama seharusnya ditolak")
	}

	// Kapitalisasi tidak boleh menjadi jalan pintas melewati batas.
	if ok, _ := limiter.Allow(ctx, "203.0.113.4", "TARGET@Example.TEST"); ok {
		t.Error("email dengan kapitalisasi berbeda dihitung sebagai email lain")
	}

	// Email lain tidak terpengaruh.
	if ok, _ := limiter.Allow(ctx, "203.0.113.5", "lain@example.test"); !ok {
		t.Error("email lain ikut terkena batas")
	}

	// Login berhasil melepas penghitung email.
	limiter.ResetEmail(ctx, "target@example.test")
	if ok, _ := limiter.Allow(ctx, "203.0.113.6", "target@example.test"); !ok {
		t.Error("penghitung email tidak dilepas setelah ResetEmail")
	}
}

// Penghitung IP yang sudah melampaui batas tidak boleh ikut menaikkan penghitung email:
// kalau ikut, penyerang yang sudah diblokir masih bisa menghabiskan kuota email korban dan
// menghalangi pemiliknya masuk dari tempat lain.
func TestLimiterDoesNotBurnEmailQuotaWhenIPBlocked(t *testing.T) {
	limiter, mr := newLimiter(t, WithLoginRateLimits(1, 5), WithLoginRateWindow(time.Minute))
	ctx := context.Background()

	if ok, _ := limiter.Allow(ctx, "203.0.113.9", "korban@example.test"); !ok {
		t.Fatal("percobaan pertama ditolak")
	}
	emailKeys := func() int {
		n := 0
		for _, key := range mr.Keys() {
			if strings.Contains(key, "login_email") {
				n++
			}
		}
		return n
	}
	before := emailKeys()

	for i := 0; i < 10; i++ {
		if ok, _ := limiter.Allow(ctx, "203.0.113.9", "korban@example.test"); ok {
			t.Fatalf("percobaan %d dari IP yang sudah diblokir diloloskan", i+2)
		}
	}
	if got := emailKeys(); got != before {
		t.Errorf("jumlah kunci email = %d, mau tetap %d", got, before)
	}

	// Pemilik akun yang sah masih punya kuota dari alamat lain.
	if ok, _ := limiter.Allow(ctx, "198.51.100.7", "korban@example.test"); !ok {
		t.Error("kuota email korban ikut terhabiskan oleh IP yang diblokir")
	}
}

// Email tidak boleh tersimpan mentah di Redis: instance-nya bisa dipakai bersama, isinya
// tidak terenkripsi, dan daftar alamat email admin adalah data yang tidak perlu ada di sana.
func TestLimiterDoesNotStoreRawEmail(t *testing.T) {
	limiter, mr := newLimiter(t, WithLoginRateWindow(time.Minute))
	const email = "rahasia.admin@example.test"

	if ok, _ := limiter.Allow(context.Background(), "203.0.113.9", email); !ok {
		t.Fatal("percobaan pertama ditolak")
	}

	keys := mr.Keys()
	if len(keys) == 0 {
		t.Fatal("tidak ada kunci yang ditulis")
	}
	for _, key := range keys {
		if strings.Contains(strings.ToLower(key), "rahasia.admin") ||
			strings.Contains(strings.ToLower(key), "example.test") {
			t.Errorf("kunci %q memuat email mentah", key)
		}
		if !strings.HasPrefix(key, cache.KeyPrefix+":") {
			t.Errorf("kunci %q tidak bernamespace %q", key, cache.KeyPrefix)
		}
	}

	// Penghitung wajib punya masa berlaku. Tanpa itu, satu proses yang mati di saat yang
	// salah akan meninggalkan alamat atau akun terblokir selamanya.
	for _, key := range keys {
		if ttl := mr.TTL(key); ttl <= 0 {
			t.Errorf("kunci %q tidak punya masa berlaku (ttl %v)", key, ttl)
		}
	}
}

// Redis yang mati harus membuat percobaan diloloskan, bukan ditolak: kalau gagal tertutup,
// matinya Redis berarti tidak ada seorang pun bisa masuk untuk memperbaikinya.
func TestLimiterFailsOpen(t *testing.T) {
	limiter, mr := newLimiter(t, WithLoginRateLimits(1, 1), WithLoginRateWindow(time.Minute))
	ctx := context.Background()

	if ok, _ := limiter.Allow(ctx, "203.0.113.9", "a@example.test"); !ok {
		t.Fatal("percobaan pertama ditolak")
	}
	if ok, _ := limiter.Allow(ctx, "203.0.113.9", "a@example.test"); ok {
		t.Fatal("percobaan kedua seharusnya ditolak sebelum Redis dimatikan")
	}

	mr.Close()
	if ok, retryAfter := limiter.Allow(ctx, "203.0.113.9", "a@example.test"); !ok {
		t.Errorf("percobaan ditolak setelah Redis mati (retry %v); pembatas harus gagal terbuka", retryAfter)
	}
	// ResetEmail juga tidak boleh panik saat Redis mati.
	limiter.ResetEmail(ctx, "a@example.test")
}

// Jendela penghitungan harus berpindah bersama waktu, sehingga penghitung lama hilang
// sendiri lewat masa berlakunya tanpa pekerjaan latar.
func TestLimiterWindowRollsOver(t *testing.T) {
	limiter, _ := newLimiter(t, WithLoginRateLimits(0, 1), WithLoginRateWindow(time.Minute))

	now := time.Now()
	first := limiter.bucket(now)
	if second := limiter.bucket(now.Add(90 * time.Second)); first == second {
		t.Errorf("label jendela tidak berubah setelah satu jendela penuh: %q", first)
	}
	if same := limiter.bucket(now.Add(time.Millisecond)); same != first {
		t.Errorf("label jendela berubah di dalam jendela yang sama: %q vs %q", first, same)
	}
}

// --- Integrasi dengan Login (butuh database) ---------------------------------

// Login yang melampaui batas laju harus dibedakan dari kredensial yang salah: alasannya
// ReasonRateLimited dengan lama tunggu, supaya lapisan HTTP bisa menjawab 429 alih-alih
// menyuruh pengguna mencoba password lain.
func TestLoginRateLimited(t *testing.T) {
	limiter, _ := newLimiter(t, WithLoginRateLimits(0, 2), WithLoginRateWindow(time.Hour))
	e := newEnv(t, WithLoginLimiter(limiter))
	e.makeUser(t, "batas@example.test", seed.RoleViewer)

	for i := 0; i < 2; i++ {
		_, err := e.svc.Login(e.ctx, "batas@example.test", security.Secret("Salah-Terus-2026"), "203.0.113.9", "")
		var failure *LoginError
		if !errors.As(err, &failure) || failure.Reason != ReasonBadPassword {
			t.Fatalf("percobaan %d: err = %v", i+1, err)
		}
	}

	_, err := e.svc.Login(e.ctx, "batas@example.test", security.Secret(testPassword), "203.0.113.9", "")
	var failure *LoginError
	if !errors.As(err, &failure) {
		t.Fatalf("err = %v, mau *LoginError", err)
	}
	if failure.Reason != ReasonRateLimited {
		t.Fatalf("Reason = %q, mau %q", failure.Reason, ReasonRateLimited)
	}
	if failure.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, mau positif", failure.RetryAfter)
	}
	// Pesan yang keluar tetap seragam dengan kegagalan kredensial; yang berbeda hanya
	// status HTTP yang dipilih lapisan handler.
	if err.Error() != MessageLoginFailed {
		t.Errorf("pesan = %q, mau %q", err.Error(), MessageLoginFailed)
	}
	// Percobaan yang ditolak batas laju tetap tercatat di audit.
	if got := e.countAudit(t, "auth.login_failed"); got != 3 {
		t.Errorf("catatan audit login gagal = %d, mau 3", got)
	}

	// Login berhasil melepas penghitung email, sehingga salah ketik hari ini tidak
	// menghalangi pemilik akun besok.
	limiter.ResetEmail(e.ctx, "batas@example.test")
	if _, err := e.svc.Login(e.ctx, "batas@example.test", security.Secret(testPassword), "203.0.113.9", ""); err != nil {
		t.Fatalf("Login setelah penghitung dilepas: %v", err)
	}
	if _, err := e.svc.Login(e.ctx, "batas@example.test", security.Secret(testPassword), "203.0.113.9", ""); err != nil {
		t.Errorf("penghitung email tidak dilepas setelah login berhasil: %v", err)
	}
}

// Perkakas pembatas harus bertahan terhadap masukan yang tidak sempurna: jendela nol,
// email kosong, dan hasil skrip yang bentuknya tidak dikenali.
func TestLimiterInternals(t *testing.T) {
	if got := emailBucketKey("   "); got != "" {
		t.Errorf("emailBucketKey(spasi) = %q, mau kosong", got)
	}
	if emailBucketKey("a@example.test") == emailBucketKey("b@example.test") {
		t.Error("dua email berbeda menghasilkan kunci yang sama")
	}
	if emailBucketKey("A@Example.Test") != emailBucketKey("a@example.test") {
		t.Error("kapitalisasi menghasilkan kunci berbeda")
	}

	// Jendela nol tidak boleh membagi dengan nol.
	zero := &LoginLimiter{}
	if got := zero.bucket(time.Unix(120, 0)); got != "120" {
		t.Errorf("bucket dengan jendela nol = %q, mau \"120\"", got)
	}

	// Hasil skrip yang tidak dikenali diperlakukan sebagai nol, sejalan dengan sifat
	// gagal-terbuka pembatas ini.
	cases := []any{nil, "bukan tabel", []any{}, []any{int64(3)}, []any{"a", "b"}}
	for _, res := range cases {
		hits, ttl := parseCounterResult(res)
		if hits != 0 || ttl != 0 {
			t.Errorf("parseCounterResult(%v) = %d, %v; mau 0, 0", res, hits, ttl)
		}
	}
	hits, ttl := parseCounterResult([]any{int64(4), int64(1500)})
	if hits != 4 || ttl != 1500*time.Millisecond {
		t.Errorf("parseCounterResult = %d, %v", hits, ttl)
	}
	// PTTL negatif (-1 tanpa masa berlaku, -2 kunci hilang) tidak boleh menjadi durasi.
	if _, ttl := parseCounterResult([]any{int64(4), int64(-2)}); ttl != 0 {
		t.Errorf("ttl dari PTTL negatif = %v, mau 0", ttl)
	}
}
