package gateway

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// --- Perkakas uji ------------------------------------------------------------

// Dua provider dan dua model, supaya isolasi antar pasangan bisa diperiksa.
const (
	breakerProviderA = "5f1c1e64-2a53-4a2f-9c4f-11c9a1b7d001"
	breakerProviderB = "5f1c1e64-2a53-4a2f-9c4f-11c9a1b7d002"
	breakerModelA    = "gpt-4o-mini"
	breakerModelB    = "openai/gpt-4o"
)

// breakerEnv adalah satu breaker beserta Redis tiruan di belakangnya.
type breakerEnv struct {
	breaker *Breaker
	mr      *miniredis.Miniredis
	rdb     *cache.Redis
	now     time.Time
}

// cfgUji adalah konfigurasi yang dipakai hampir semua test di berkas ini.
//
// OpenDuration satu detik karena itu nilai terendah yang diizinkan constraint
// routing_rules_open_duration di migrasi 0008 — dan karena jamnya dikuasai test, satu
// detik di sini tidak berarti menunggu satu detik.
//
// MinSamples 4 dengan ambang 0,5 berarti breaker baru boleh membuka setelah empat sampel,
// dan itu yang membuat test "sampel minimum" di bawah bisa membedakan breaker yang bekerja
// dari breaker yang membuka pada kegagalan pertama.
func cfgUji() BreakerConfig {
	return BreakerConfig{
		FailureThreshold: 0.5,
		MinSamples:       4,
		Window:           10 * time.Second,
		OpenDuration:     time.Second,
		HalfOpenProbes:   1,
	}
}

// newBreakerEnv menyalakan Redis tiruan lalu menyambungkannya lewat cache.Connect,
// sehingga jalur yang diuji sama dengan jalur produksi — termasuk EVALSHA dan opsi pool.
//
// Jamnya dibekukan. Seluruh perpindahan state breaker ditentukan selisih waktu, jadi tanpa
// jam yang dikuasai test satu-satunya cara menguji open -> half_open adalah benar-benar
// menunggu OpenDuration.
func newBreakerEnv(t *testing.T, cfg BreakerConfig) *breakerEnv {
	t.Helper()
	bungkamLogDefault(t)

	mr := miniredis.RunT(t)
	rdb, err := cache.Connect(context.Background(), &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	env := &breakerEnv{
		breaker: NewBreaker(rdb, cfg, slog.New(slog.DiscardHandler)),
		mr:      mr,
		rdb:     rdb,
		now:     time.Date(2026, 9, 3, 10, 30, 30, 0, time.UTC),
	}
	env.freeze(env.now)
	return env
}

// bungkamLogDefault membungkam logger default proses selama satu test.
//
// Log internal go-redis dialihkan cache.Connect ke slog, dan pesan dari goroutine
// pemelihara pool jatuh ke slog.Default() karena context-nya tidak membawa logger.
// Beberapa test di sini sengaja mematikan Redis, yang memicu puluhan pesan semacam itu dan
// menenggelamkan keluaran test.
func bungkamLogDefault(t *testing.T) {
	t.Helper()
	sebelum := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	t.Cleanup(func() { slog.SetDefault(sebelum) })
}

// freeze memaku jam breaker pada satu titik waktu.
func (e *breakerEnv) freeze(at time.Time) {
	e.now = at
	e.breaker.now = func() time.Time { return at }
}

// advance memindahkan jam breaker ke depan tanpa menunggu waktu nyata.
func (e *breakerEnv) advance(d time.Duration) { e.freeze(e.now.Add(d)) }

func (e *breakerEnv) allow(t *testing.T, provider, model string) (bool, State) {
	t.Helper()
	izin, state, err := e.breaker.Allow(context.Background(), provider, model)
	if err != nil {
		t.Fatalf("Allow(%s, %s): %v", provider, model, err)
	}
	return izin, state
}

func (e *breakerEnv) record(t *testing.T, provider, model string, sukses bool) {
	t.Helper()
	if err := e.breaker.Record(context.Background(), provider, model, sukses); err != nil {
		t.Fatalf("Record(%s, %s, sukses=%v): %v", provider, model, sukses, err)
	}
}

// recordN mencatat n hasil yang sama berturut-turut.
func (e *breakerEnv) recordN(t *testing.T, provider, model string, sukses bool, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		e.record(t, provider, model, sukses)
	}
}

func (e *breakerEnv) state(t *testing.T, provider, model string) State {
	t.Helper()
	state, err := e.breaker.State(context.Background(), provider, model)
	if err != nil {
		t.Fatalf("State(%s, %s): %v", provider, model, err)
	}
	return state
}

// assertKeputusan memeriksa hasil Allow beserta state yang dilaporkannya.
func (e *breakerEnv) assertKeputusan(t *testing.T, langkah, provider, model string, mauIzin bool, mauState State) {
	t.Helper()
	izin, state := e.allow(t, provider, model)
	if izin != mauIzin || state != mauState {
		t.Fatalf("%s: Allow = (%v, %s), mau (%v, %s)", langkah, izin, state, mauIzin, mauState)
	}
}

// bukaBreaker membuat breaker terbuka lewat jalur normal: cukup kegagalan di dalam window
// untuk melewati ambang rasio sekaligus sampel minimum.
func (e *breakerEnv) bukaBreaker(t *testing.T, provider, model string) {
	t.Helper()
	e.recordN(t, provider, model, false, e.breaker.cfg.MinSamples)
	if state := e.state(t, provider, model); state != StateOpen {
		t.Fatalf("breaker tidak terbuka setelah %d kegagalan: state = %s", e.breaker.cfg.MinSamples, state)
	}
}

// --- Perpindahan state -------------------------------------------------------

// TestBreakerClosedKeOpenKeHalfOpenKeClosed menelusuri seluruh siklus pemulihan: breaker
// membuka karena rasio kegagalan, menunggu OpenDuration, melepas satu probe, lalu menutup
// kembali karena probe itu berhasil.
func TestBreakerClosedKeOpenKeHalfOpenKeClosed(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())

	e.assertKeputusan(t, "awal", breakerProviderA, breakerModelA, true, StateClosed)

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.assertKeputusan(t, "setelah terbuka", breakerProviderA, breakerModelA, false, StateOpen)

	// Sebelum tenggat lewat, breaker masih menolak. Jamnya digeser melewati memoTTL supaya
	// yang diuji keputusan Redis, bukan keputusan yang masih tersimpan di memori.
	e.advance(cfgUji().OpenDuration - 300*time.Millisecond)
	e.assertKeputusan(t, "sebelum tenggat", breakerProviderA, breakerModelA, false, StateOpen)

	e.advance(300 * time.Millisecond)
	e.assertKeputusan(t, "probe pertama", breakerProviderA, breakerModelA, true, StateHalfOpen)

	// Kuota probe adalah 1, jadi permintaan berikutnya harus ditolak walau state-nya sama.
	// Ini juga yang membuktikan izin probe TIDAK di-cache di memori: kalau ia di-cache,
	// pemanggil kedua akan mendapat izin yang sama.
	e.assertKeputusan(t, "probe kedua", breakerProviderA, breakerModelA, false, StateHalfOpen)

	e.record(t, breakerProviderA, breakerModelA, true)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
		t.Fatalf("probe berhasil tidak menutup breaker: state = %s", state)
	}
	e.assertKeputusan(t, "setelah menutup", breakerProviderA, breakerModelA, true, StateClosed)

	// Jendela ikut direset saat menutup. Kalau tidak, rasio yang tadi memicu breaker masih
	// ada di sana dan kegagalan pertama sesudah pemulihan akan membukanya lagi seketika.
	e.record(t, breakerProviderA, breakerModelA, false)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
		t.Fatalf("jendela tidak direset saat menutup: satu kegagalan langsung membuka lagi (state = %s)", state)
	}
}

// TestBreakerHalfOpenGagalMembukaLagiDenganTimerBaru memeriksa sisi sebaliknya: probe yang
// gagal harus membuka breaker lagi dengan tenggat yang dihitung dari sekarang, bukan
// meneruskan tenggat lama yang sudah lewat.
func TestBreakerHalfOpenGagalMembukaLagiDenganTimerBaru(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.advance(cfgUji().OpenDuration)
	e.assertKeputusan(t, "probe", breakerProviderA, breakerModelA, true, StateHalfOpen)

	e.record(t, breakerProviderA, breakerModelA, false)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateOpen {
		t.Fatalf("probe gagal tidak membuka breaker lagi: state = %s", state)
	}

	// Tenggat baru: hampir satu OpenDuration setelah probe gagal, breaker masih menolak.
	// Tanpa timer baru, state di titik ini sudah half_open lagi.
	e.advance(cfgUji().OpenDuration - 300*time.Millisecond)
	e.assertKeputusan(t, "tenggat baru belum lewat", breakerProviderA, breakerModelA, false, StateOpen)

	e.advance(300 * time.Millisecond)
	e.assertKeputusan(t, "probe berikutnya", breakerProviderA, breakerModelA, true, StateHalfOpen)
}

// TestBreakerSampelMinimumMencegahPembukaanDini adalah alasan MinSamples ada: tanpa
// penyebut yang cukup besar, satu kegagalan atas jendela kosong berarti rasio 1/1 dan satu
// timeout tunggal mematikan sebuah provider selama OpenDuration.
func TestBreakerSampelMinimumMencegahPembukaanDini(t *testing.T) {
	cfg := cfgUji()
	e := newBreakerEnv(t, cfg)

	// Semuanya gagal — rasionya 100% sejak sampel pertama — tetapi jumlahnya masih di bawah
	// sampel minimum.
	for i := 1; i < cfg.MinSamples; i++ {
		e.record(t, breakerProviderA, breakerModelA, false)
		if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
			t.Fatalf("breaker membuka pada kegagalan ke-%d, sampel minimum %d: state = %s",
				i, cfg.MinSamples, state)
		}
		e.assertKeputusan(t, "belum cukup sampel", breakerProviderA, breakerModelA, true, StateClosed)
	}

	// Sampel yang menyamai minimum melengkapi kedua syarat sekaligus.
	e.record(t, breakerProviderA, breakerModelA, false)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateOpen {
		t.Fatalf("breaker tidak membuka pada sampel ke-%d: state = %s", cfg.MinSamples, state)
	}
}

// TestBreakerRasioBukanHitunganMentah memeriksa bahwa yang menentukan adalah perbandingan,
// bukan jumlah kegagalan: lalu lintas yang sebagian besar berhasil tidak boleh membuka
// breaker walau jumlah kegagalannya jauh melebihi sampel minimum.
func TestBreakerRasioBukanHitunganMentah(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())

	// 6 gagal dari 30 sampel = 20%, di bawah ambang 50% — padahal 6 kegagalan sendiri sudah
	// lebih banyak daripada MinSamples, dan breaker berbasis hitungan mentah akan membuka
	// di sini.
	for i := 0; i < 6; i++ {
		e.record(t, breakerProviderA, breakerModelA, false)
		e.recordN(t, breakerProviderA, breakerModelA, true, 4)
	}
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
		t.Fatalf("breaker membuka pada rasio 20%%: state = %s", state)
	}

	// Kegagalan yang cukup untuk melewati setengah jendela membukanya.
	e.recordN(t, breakerProviderA, breakerModelA, false, 20)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateOpen {
		t.Fatalf("breaker tidak membuka setelah rasio melewati ambang: state = %s", state)
	}
}

// TestBreakerWindowMembuangSampelLama memeriksa jendela geser: sampel yang keluar dari
// window tidak boleh ikut dihitung, dan field embernya tidak boleh menumpuk di Redis.
func TestBreakerWindowMembuangSampelLama(t *testing.T) {
	cfg := cfgUji()
	e := newBreakerEnv(t, cfg)
	ctx := context.Background()
	key := cache.CircuitBreakerKey(breakerProviderA, breakerModelA)

	// Tiga kegagalan, satu kurang dari sampel minimum: belum membuka.
	e.recordN(t, breakerProviderA, breakerModelA, false, cfg.MinSamples-1)

	e.advance(cfg.Window + time.Second)

	// Dua kegagalan lagi. Kalau yang lama masih dihitung, totalnya lima — melewati sampel
	// minimum dengan rasio 100% — dan breaker membuka di sini.
	e.recordN(t, breakerProviderA, breakerModelA, false, 2)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
		t.Fatalf("sampel di luar window masih dihitung: state = %s", state)
	}

	// Ember yang kedaluwarsa dibuang, bukan hanya diabaikan: hash yang terus bertambah
	// field adalah kebocoran yang tumbuh selama kunci masih dipakai.
	n, err := e.rdb.Client().HLen(ctx, key).Result()
	if err != nil {
		t.Fatalf("HLEN: %v", err)
	}
	if n != 2 {
		t.Fatalf("hash breaker punya %d field, mau 2 (satu ember: total dan gagal) — ember lama tidak dibuang", n)
	}

	// Penghitungnya tetap bekerja setelah pembersihan.
	e.recordN(t, breakerProviderA, breakerModelA, false, cfg.MinSamples-2)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateOpen {
		t.Fatalf("breaker tidak membuka setelah sampel cukup pada window baru: state = %s", state)
	}
}

// TestBreakerTerpisahPerProviderDanModel menjaga janji cache.CircuitBreakerKey: satu model
// yang bermasalah tidak boleh mematikan model lain di provider yang sama, apalagi provider
// lain.
func TestBreakerTerpisahPerProviderDanModel(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())

	e.bukaBreaker(t, breakerProviderA, breakerModelA)

	e.assertKeputusan(t, "pasangan yang rusak", breakerProviderA, breakerModelA, false, StateOpen)
	e.assertKeputusan(t, "model lain, provider sama", breakerProviderA, breakerModelB, true, StateClosed)
	e.assertKeputusan(t, "provider lain, model sama", breakerProviderB, breakerModelA, true, StateClosed)
	e.assertKeputusan(t, "provider dan model lain", breakerProviderB, breakerModelB, true, StateClosed)
}

// TestBreakerRecordSaatOpenTidakMemperpanjangTenggat menguji hasil yang datang terlambat:
// permintaan yang lolos sebelum breaker membuka tetap akan melaporkan hasilnya, dan laporan
// itu tidak boleh menggeser tenggat open. Kalau ia menggeser, aliran permintaan yang belum
// selesai bisa menahan breaker di open jauh lebih lama daripada yang diminta operator.
func TestBreakerRecordSaatOpenTidakMemperpanjangTenggat(t *testing.T) {
	cfg := cfgUji()
	e := newBreakerEnv(t, cfg)

	e.bukaBreaker(t, breakerProviderA, breakerModelA)

	e.advance(cfg.OpenDuration / 2)
	e.recordN(t, breakerProviderA, breakerModelA, false, 5)

	e.advance(cfg.OpenDuration / 2)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateHalfOpen {
		t.Fatalf("hasil yang datang saat open menggeser tenggatnya: state = %s, mau %s", state, StateHalfOpen)
	}
}

// TestBreakerStateTidakMengubahApaPun menjaga sifat yang dijanjikan State: ia dipanggil
// dashboard, mungkin sekali per beberapa detik untuk setiap pasangan, dan pemanggilan itu
// tidak boleh menghabiskan kuota probe — kalau habis, membuka halaman status berarti
// menahan pemulihan.
func TestBreakerStateTidakMengubahApaPun(t *testing.T) {
	cfg := cfgUji()
	e := newBreakerEnv(t, cfg)

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.advance(cfg.OpenDuration)

	for i := 0; i < 5; i++ {
		if state := e.state(t, breakerProviderA, breakerModelA); state != StateHalfOpen {
			t.Fatalf("pembacaan ke-%d melaporkan %s, mau %s", i+1, state, StateHalfOpen)
		}
	}

	// Kuota probe masih utuh: Allow pertama sesudah lima pembacaan tetap mendapat izin.
	e.assertKeputusan(t, "probe setelah dibaca berkali-kali", breakerProviderA, breakerModelA, true, StateHalfOpen)
}

// TestBreakerReset memeriksa tombol operator: breaker yang terbuka harus menutup seketika,
// dan jendelanya ikut bersih sehingga kegagalan pertama sesudahnya tidak membukanya lagi.
func TestBreakerReset(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.assertKeputusan(t, "sebelum reset", breakerProviderA, breakerModelA, false, StateOpen)

	if err := e.breaker.Reset(ctx, breakerProviderA, breakerModelA); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Tanpa pembuangan cache di memori, instance yang menekan tombolnya sendiri masih
	// menolak lalu lintas sampai memo kedaluwarsa — persis yang tidak diharapkan operator
	// setelah tombolnya melaporkan berhasil.
	e.assertKeputusan(t, "setelah reset", breakerProviderA, breakerModelA, true, StateClosed)

	e.record(t, breakerProviderA, breakerModelA, false)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
		t.Fatalf("jendela tidak bersih setelah reset: satu kegagalan membuka lagi (state = %s)", state)
	}

	// Reset atas breaker yang tidak pernah ada bukan kesalahan: dashboard tidak perlu tahu
	// lebih dulu apakah kuncinya sudah terbentuk.
	if err := e.breaker.Reset(ctx, breakerProviderB, breakerModelB); err != nil {
		t.Fatalf("Reset atas breaker yang belum ada: %v", err)
	}
}

// TestBreakerKunciSelaluPunyaTTL menjaga agar pasangan (provider, model) yang tidak dipakai
// lagi tidak menyisakan kunci di Redis selamanya, dan agar keadaan apa pun yang membuat
// sebuah breaker berhenti diperbarui berakhir pulih, bukan menolak lalu lintas tanpa batas.
func TestBreakerKunciSelaluPunyaTTL(t *testing.T) {
	cfg := cfgUji()
	e := newBreakerEnv(t, cfg)
	key := cache.CircuitBreakerKey(breakerProviderA, breakerModelA)

	e.record(t, breakerProviderA, breakerModelA, false)
	ttl := e.mr.TTL(key)
	if ttl <= 0 {
		t.Fatalf("kunci jendela tanpa TTL: %s", ttl)
	}
	if mau := cfg.Window + 2*cfg.OpenDuration; ttl > mau {
		t.Fatalf("TTL %s melebihi jumlah tenggat yang bermakna (%s)", ttl, mau)
	}

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	if ttl := e.mr.TTL(key); ttl <= cfg.OpenDuration {
		t.Fatalf("TTL kunci open (%s) tidak melebihi OpenDuration (%s): breaker bisa menutup diam-diam karena kuncinya hilang",
			ttl, cfg.OpenDuration)
	}
}

// --- Gagal terbuka -----------------------------------------------------------

// TestBreakerGagalTerbukaSaatRedisMati adalah jaminan paling penting di berkas ini. Breaker
// yang gagal TERTUTUP mengubah matinya Redis menjadi matinya seluruh gateway: setiap
// permintaan ke setiap provider ditolak sekaligus, termasuk yang upstream-nya sehat.
func TestBreakerGagalTerbukaSaatRedisMati(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()

	e.mr.Close()

	izin, state, err := e.breaker.Allow(ctx, breakerProviderA, breakerModelA)
	if !izin {
		t.Fatal("Allow menolak permintaan padahal redis mati")
	}
	if err == nil {
		t.Fatal("Allow tidak melaporkan error, jadi pemanggil tidak punya cara mengetahui penegakan breaker sedang hilang")
	}
	// State yang dilaporkan menggambarkan keputusan yang diambil, bukan hasil pengamatan:
	// yang dijalankan adalah perilaku closed.
	if state != StateClosed {
		t.Fatalf("state saat gagal terbuka = %s, mau %s", state, StateClosed)
	}

	// Record dan State ikut melaporkan error, tetapi tidak boleh panik atau menggantung.
	if err := e.breaker.Record(ctx, breakerProviderA, breakerModelA, false); err == nil {
		t.Fatal("Record tidak melaporkan error padahal redis mati")
	}
	if _, err := e.breaker.State(ctx, breakerProviderA, breakerModelA); err == nil {
		t.Fatal("State tidak melaporkan error padahal redis mati")
	}
	// Reset justru TIDAK gagal terbuka: ia menjawab tindakan operator, dan operator harus
	// tahu tombolnya tidak bekerja.
	if err := e.breaker.Reset(ctx, breakerProviderA, breakerModelA); err == nil {
		t.Fatal("Reset melaporkan berhasil padahal redis mati")
	}
}

// TestBreakerGagalTerbukaWalauSedangOpen menutup sudut yang lebih mudah terlewat: breaker
// yang sudah terbuka lalu kehilangan Redis. Keputusan terakhir yang diketahui adalah
// "tolak", dan justru di situlah gagal-tertutup paling menggoda — tetapi tanpa Redis tidak
// ada lagi yang bisa memindahkan state ke half_open, sehingga menolak berarti menolak
// selamanya.
func TestBreakerGagalTerbukaWalauSedangOpen(t *testing.T) {
	cfg := cfgUji()
	// OpenDuration panjang supaya yang diuji adalah hilangnya Redis, bukan tenggat yang
	// kebetulan lewat.
	cfg.OpenDuration = 30 * time.Second
	e := newBreakerEnv(t, cfg)
	ctx := context.Background()

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.assertKeputusan(t, "sebelum redis mati", breakerProviderA, breakerModelA, false, StateOpen)

	// Jam digeser melewati memoTTL supaya keputusan berikutnya benar-benar mencari Redis.
	e.advance(2 * memoTTL)
	e.mr.Close()

	izin, _, err := e.breaker.Allow(ctx, breakerProviderA, breakerModelA)
	if !izin {
		t.Fatal("breaker yang terbuka tetap menolak setelah redis mati; tidak ada lagi yang bisa memindahkannya ke half_open")
	}
	if err == nil {
		t.Fatal("Allow tidak melaporkan error padahal redis mati")
	}
}

// TestBreakerJalurCepatMemakaiCacheMemori membuktikan jalur cepat itu ada dan dipakai:
// dengan Redis dimatikan setelah keputusan diambil, jawaban "tolak" hanya mungkin datang
// dari cache di memori — jalur Redis akan gagal terbuka dan meloloskan.
func TestBreakerJalurCepatMemakaiCacheMemori(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.assertKeputusan(t, "mengisi cache", breakerProviderA, breakerModelA, false, StateOpen)

	e.mr.Close()

	izin, state, err := e.breaker.Allow(ctx, breakerProviderA, breakerModelA)
	if err != nil {
		t.Fatalf("Allow menyentuh redis padahal keputusannya masih ter-cache: %v", err)
	}
	if izin || state != StateOpen {
		t.Fatalf("Allow = (%v, %s) dari cache, mau (false, %s)", izin, state, StateOpen)
	}

	// Dan cache itu memang berumur pendek: setelah memoTTL lewat, keputusan dicari lagi ke
	// Redis — yang sekarang mati, jadi ia gagal terbuka.
	e.advance(memoTTL)
	izin, _, err = e.breaker.Allow(ctx, breakerProviderA, breakerModelA)
	if err == nil {
		t.Fatal("cache di memori tidak kedaluwarsa setelah memoTTL")
	}
	if !izin {
		t.Fatal("keputusan setelah cache kedaluwarsa tidak gagal terbuka")
	}
}

// --- Pemakaian bersamaan -----------------------------------------------------

// TestBreakerKuotaProbeAtomikSaatBersamaan menguji satu-satunya tempat di breaker ini yang
// benar-benar menuntut atomisitas: pemberian izin probe.
//
// Justru saat breaker beralih ke half_open, lalu lintas yang tertahan berdatangan bersamaan
// dari semua instance. Kalau izinnya diputuskan dengan GET lalu HSET, semua pemanggil akan
// membaca kuota yang sama sebelum ada yang menuliskannya, dan seluruh gelombang itu mengalir
// ke upstream yang justru sedang diuji dengan satu permintaan.
func TestBreakerKuotaProbeAtomikSaatBersamaan(t *testing.T) {
	cfg := cfgUji()
	e := newBreakerEnv(t, cfg)
	ctx := context.Background()

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.advance(cfg.OpenDuration)

	const goroutines = 60
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		diizini int
	)
	mulai := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-mulai
			izin, _, err := e.breaker.Allow(ctx, breakerProviderA, breakerModelA)
			if err != nil {
				t.Errorf("Allow: %v", err)
				return
			}
			if izin {
				mu.Lock()
				diizini++
				mu.Unlock()
			}
		}()
	}
	close(mulai)
	wg.Wait()

	if diizini != cfg.HalfOpenProbes {
		t.Fatalf("%d permintaan lolos sebagai probe, mau tepat %d", diizini, cfg.HalfOpenProbes)
	}
}

// TestBreakerAmanDipakaiBanyakGoroutine menjalankan jalur lengkap dari banyak goroutine
// sekaligus, untuk dijalankan dengan -race: cache di memori dibaca dan ditulis dari setiap
// permintaan, dan penjagaannya harus benar.
func TestBreakerAmanDipakaiBanyakGoroutine(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()

	pasangan := [][2]string{
		{breakerProviderA, breakerModelA},
		{breakerProviderA, breakerModelB},
		{breakerProviderB, breakerModelA},
	}

	const (
		goroutines = 40
		putaran    = 8
	)
	var wg sync.WaitGroup
	mulai := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-mulai
			p := pasangan[i%len(pasangan)]
			for j := 0; j < putaran; j++ {
				izin, _, err := e.breaker.Allow(ctx, p[0], p[1])
				if err != nil {
					t.Errorf("Allow: %v", err)
					return
				}
				if !izin {
					continue
				}
				// Sebagian gagal, sebagian berhasil: campuran itu yang membuat state
				// berpindah bolak-balik selama test berjalan.
				if err := e.breaker.Record(ctx, p[0], p[1], (i+j)%3 != 0); err != nil {
					t.Errorf("Record: %v", err)
					return
				}
			}
		}(i)
	}
	close(mulai)
	wg.Wait()

	for _, p := range pasangan {
		switch state := e.state(t, p[0], p[1]); state {
		case StateClosed, StateOpen, StateHalfOpen:
		default:
			t.Fatalf("state %s/%s tidak dikenal: %q", p[0], p[1], state)
		}
	}
}

// --- Konfigurasi -------------------------------------------------------------

func TestBreakerConfigNormalized(t *testing.T) {
	bawaan := DefaultBreakerConfig()

	tests := []struct {
		name string
		in   BreakerConfig
		mau  BreakerConfig
	}{
		{"kosong seluruhnya jadi bawaan", BreakerConfig{}, bawaan},
		{
			// Rasio di atas 1 tidak bisa dipenuhi sampel apa pun, jadi membiarkannya berarti
			// breaker yang tidak pernah membuka — mati tanpa satu pun tanda.
			name: "rasio di atas satu dijepit",
			in:   BreakerConfig{FailureThreshold: 4},
			mau:  BreakerConfig{FailureThreshold: 1, MinSamples: bawaan.MinSamples, Window: bawaan.Window, OpenDuration: bawaan.OpenDuration, HalfOpenProbes: bawaan.HalfOpenProbes},
		},
		{
			// NaN membuat seluruh perbandingan bernilai false: kegagalan yang paling sunyi.
			name: "rasio NaN jatuh ke bawaan",
			in:   BreakerConfig{FailureThreshold: math.NaN()},
			mau:  bawaan,
		},
		{
			name: "negatif jatuh ke bawaan",
			in:   BreakerConfig{FailureThreshold: -1, MinSamples: -3, Window: -time.Second, OpenDuration: -time.Minute, HalfOpenProbes: -2},
			mau:  bawaan,
		},
		{
			// Rentangnya mengikuti constraint routing_rules di migrasi 0008.
			name: "di luar rentang routing_rules dijepit",
			in:   BreakerConfig{FailureThreshold: 0.9, MinSamples: 2, Window: 5 * time.Hour, OpenDuration: 5 * time.Hour, HalfOpenProbes: 5000},
			mau:  BreakerConfig{FailureThreshold: 0.9, MinSamples: 2, Window: maxWindow, OpenDuration: maxOpenDuration, HalfOpenProbes: maxHalfOpenProbes},
		},
		{
			name: "di bawah rentang dinaikkan",
			in:   BreakerConfig{FailureThreshold: 0.1, MinSamples: 1, Window: time.Millisecond, OpenDuration: time.Millisecond, HalfOpenProbes: 1},
			mau:  BreakerConfig{FailureThreshold: 0.1, MinSamples: 1, Window: minWindow, OpenDuration: minOpenDuration, HalfOpenProbes: 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.normalized(); got != tc.mau {
				t.Fatalf("normalized() = %+v, mau %+v", got, tc.mau)
			}
		})
	}
}

// TestBreakerConfigBawaanMengikutiMigrasi menahan bawaan di sini tetap sama dengan bawaan
// kolom routing_rules di migrasi 0008. Kalau salah satunya berubah tanpa yang lain, aturan
// routing yang membiarkan kolomnya apa adanya akan berperilaku berbeda dari yang tertulis di
// skema — dan tidak ada yang gagal untuk memberi tahu.
func TestBreakerConfigBawaanMengikutiMigrasi(t *testing.T) {
	cfg := DefaultBreakerConfig()
	if cfg.OpenDuration != 30*time.Second {
		t.Errorf("OpenDuration bawaan = %s, mau 30s (routing_rules.open_duration_ms = 30000)", cfg.OpenDuration)
	}
	if cfg.HalfOpenProbes != 1 {
		t.Errorf("HalfOpenProbes bawaan = %d, mau 1 (routing_rules.half_open_probes = 1)", cfg.HalfOpenProbes)
	}
	if cfg.MinSamples != 5 {
		t.Errorf("MinSamples bawaan = %d, mau 5 (routing_rules.failure_threshold = 5)", cfg.MinSamples)
	}
}

// TestBreakerBucketWidth menjaga pembagian window menjadi ember tetap punya lantai: lebar
// nol akan membuat pembagian di dalam skrip Lua menghasilkan indeks ember tak berhingga.
func TestBreakerBucketWidth(t *testing.T) {
	if got := (BreakerConfig{Window: 10 * time.Second}).normalized().bucketWidth(); got != time.Second {
		t.Errorf("bucketWidth untuk window 10s = %s, mau 1s", got)
	}
	if got := (BreakerConfig{Window: time.Nanosecond}).normalized().bucketWidth(); got < time.Millisecond {
		t.Errorf("bucketWidth = %s, mau minimal 1ms", got)
	}
}

// --- Kontrak pelaporan -------------------------------------------------------

// TestStateGauge menahan pemetaan ke gauge Prometheus tetap sama dengan yang dijanjikan
// Help metriknya. Angka yang tertukar di sini tidak memunculkan error apa pun — hanya alert
// yang menyebut breaker yang salah.
func TestStateGauge(t *testing.T) {
	tests := []struct {
		state State
		mau   float64
	}{
		{StateClosed, observability.BreakerClosed},
		{StateHalfOpen, observability.BreakerHalfOpen},
		{StateOpen, observability.BreakerOpen},
		// Nilai yang tidak dikenal dilaporkan sebagai closed, bukan sebagai angka lepas
		// yang tidak punya arti di dashboard.
		{State("entah"), observability.BreakerClosed},
	}
	for _, tc := range tests {
		if got := tc.state.Gauge(); got != tc.mau {
			t.Errorf("State(%q).Gauge() = %v, mau %v", tc.state, got, tc.mau)
		}
	}
}

// TestBreakerNilDanTanpaRedisMeloloskan menjaga agar deployment tanpa Redis — dan pemanggil
// yang belum menyiapkan breaker — memakai jalur kode yang sama, bukan bercabang sendiri.
func TestBreakerNilDanTanpaRedisMeloloskan(t *testing.T) {
	ctx := context.Background()

	for name, b := range map[string]*Breaker{
		"penerima nil": nil,
		"redis nil":    NewBreaker(nil, cfgUji(), slog.New(slog.DiscardHandler)),
	} {
		t.Run(name, func(t *testing.T) {
			izin, state, err := b.Allow(ctx, breakerProviderA, breakerModelA)
			if !izin || state != StateClosed || err != nil {
				t.Fatalf("Allow = (%v, %s, %v), mau (true, %s, nil)", izin, state, err, StateClosed)
			}
			if err := b.Record(ctx, breakerProviderA, breakerModelA, false); err != nil {
				t.Fatalf("Record: %v", err)
			}
			if state, err := b.State(ctx, breakerProviderA, breakerModelA); state != StateClosed || err != nil {
				t.Fatalf("State = (%s, %v), mau (%s, nil)", state, err, StateClosed)
			}
			if err := b.Reset(ctx, breakerProviderA, breakerModelA); err != nil {
				t.Fatalf("Reset: %v", err)
			}
			if err := b.Warm(ctx); err != nil {
				t.Fatalf("Warm: %v", err)
			}
		})
	}
}

// TestBreakerProviderKosongTetapMeloloskan memeriksa penjagaan bug pemanggil: kunci tanpa
// segmen provider akan dipakai bersama oleh semua pemanggil tanpa provider, sehingga breaker
// yang satu membuka-tutup breaker yang lain. Permintaannya tetap lolos — ini bug pemanggil,
// bukan alasan menolak lalu lintas pengguna — tetapi errornya harus terlihat.
func TestBreakerProviderKosongTetapMeloloskan(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()

	for _, providerID := range []string{"", "   "} {
		izin, state, err := e.breaker.Allow(ctx, providerID, breakerModelA)
		if !izin || state != StateClosed {
			t.Fatalf("Allow(%q) = (%v, %s), mau (true, %s)", providerID, izin, state, StateClosed)
		}
		if err == nil {
			t.Fatalf("Allow(%q) tidak melaporkan error", providerID)
		}
		if err := e.breaker.Record(ctx, providerID, breakerModelA, false); err == nil {
			t.Fatalf("Record(%q) tidak melaporkan error", providerID)
		}
		if _, err := e.breaker.State(ctx, providerID, breakerModelA); err == nil {
			t.Fatalf("State(%q) tidak melaporkan error", providerID)
		}
		if err := e.breaker.Reset(ctx, providerID, breakerModelA); err == nil {
			t.Fatalf("Reset(%q) tidak melaporkan error", providerID)
		}
	}

	// Model kosong justru SAH: cache.CircuitBreakerKey menjanjikan breaker level provider.
	if _, _, err := e.breaker.Allow(ctx, breakerProviderA, ""); err != nil {
		t.Fatalf("Allow dengan model kosong: %v", err)
	}
}

// TestBreakerAllowTidakMenulisSaatClosed menjaga jalur terpanas tetap hanya membaca. Kalau
// Allow ikut menulis, setiap permintaan yang ditolak breaker yang macet akan memperbarui TTL
// kuncinya sendiri, dan jaring pengaman terakhir — kunci kedaluwarsa lalu breaker pulih —
// tidak pernah menjala.
func TestBreakerAllowTidakMenulisSaatClosed(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()
	key := cache.CircuitBreakerKey(breakerProviderA, breakerModelA)

	for i := 0; i < 3; i++ {
		e.assertKeputusan(t, "closed", breakerProviderA, breakerModelA, true, StateClosed)
		e.advance(memoTTL)
	}

	n, err := e.rdb.Client().Exists(ctx, key).Result()
	if err != nil {
		t.Fatalf("EXISTS: %v", err)
	}
	if n != 0 {
		t.Fatalf("Allow membuat kunci %q padahal breaker closed", key)
	}
}

// TestBreakerWarm memeriksa pemanasan cache skrip: setelahnya ketiga skrip harus sudah
// dikenal Redis, sehingga permintaan pertama tidak menanggung EVALSHA yang gagal ditambah
// EVAL berisi badan skrip.
func TestBreakerWarm(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()

	if err := e.breaker.Warm(ctx); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	for nama, skrip := range map[string]string{
		"allow":  allowScript.Hash(),
		"record": recordScript.Hash(),
		"state":  stateScript.Hash(),
	} {
		ada, err := e.rdb.Client().ScriptExists(ctx, skrip).Result()
		if err != nil {
			t.Fatalf("SCRIPT EXISTS: %v", err)
		}
		if len(ada) != 1 || !ada[0] {
			t.Fatalf("skrip %s tidak ter-cache setelah Warm", nama)
		}
	}
}
