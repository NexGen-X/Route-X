// Package gateway memuat perkakas jalur request yang menjaga gateway tetap melayani
// ketika upstream tidak. Berkas ini memuat circuit breaker per pasangan
// (provider, model).
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// --- State -------------------------------------------------------------------

// State adalah kondisi satu breaker.
//
// Bertipe string, bukan angka, mengikuti router.Strategy dan health.Status: nilainya
// ikut ke log dan ke isi hash Redis, dan di kedua tempat "half_open" jauh lebih
// menolong orang yang sedang membaca daripada angka 1. Pemetaan ke gauge Prometheus
// ada di Gauge().
type State string

const (
	// StateClosed berarti lalu lintas mengalir dan kegagalan sedang dihitung.
	StateClosed State = "closed"
	// StateOpen berarti semua permintaan ditolak sampai OpenDuration lewat.
	StateOpen State = "open"
	// StateHalfOpen berarti breaker sedang menguji upstream dengan sejumlah kecil
	// permintaan percobaan. Permintaan di luar kuota probe tetap ditolak, jadi state ini
	// sekaligus berarti "sedang menunggu hasil probe".
	StateHalfOpen State = "half_open"
)

// Gauge mengubah state menjadi nilai observability.Metrics.CircuitBreaker.
//
// Ada supaya angkanya hanya ditulis di satu tempat. Help gauge itu sudah menjanjikan
// "0=closed, 1=half_open, 2=open", dan pemanggil yang memetakannya sendiri gampang
// menukar 1 dengan 2 — kekeliruan yang tidak memunculkan error apa pun, hanya dashboard
// dan alert yang salah tentang breaker mana yang sedang menolak lalu lintas.
func (s State) Gauge() float64 {
	switch s {
	case StateHalfOpen:
		return observability.BreakerHalfOpen
	case StateOpen:
		return observability.BreakerOpen
	default:
		return observability.BreakerClosed
	}
}

// --- Konfigurasi -------------------------------------------------------------

// Nilai bawaan satu breaker.
//
// Tiga di antaranya sengaja sama dengan bawaan kolom routing_rules di migrasi 0008,
// supaya aturan routing yang membiarkan kolomnya apa adanya mendapat breaker dengan
// kepekaan yang sama seperti yang tertulis di skema — bukan angka lain yang hanya ada
// di kode Go.
const (
	// DefaultFailureThreshold: setengah dari sampel di dalam window gagal.
	DefaultFailureThreshold = 0.5

	// DefaultMinSamples mengambil angka routing_rules.failure_threshold (bawaan 5).
	// Kolom itu bertipe hitungan, bukan rasio, jadi angkanya dipakai sebagai jumlah
	// sampel minimum — lihat catatan di BreakerConfig.MinSamples.
	DefaultMinSamples = 5

	// DefaultWindow: satu menit. Panjangnya menentukan berapa banyak lalu lintas yang
	// perlu ada sebelum breaker bisa membuka sama sekali. Dengan MinSamples 5, window
	// satu menit berarti pasangan (provider, model) yang menerima kurang dari lima
	// permintaan per menit tidak akan pernah memicu breaker; itu disengaja, karena rasio
	// atas dua atau tiga sampel adalah kebisingan, bukan bukti.
	DefaultWindow = time.Minute

	// DefaultOpenDuration mengambil routing_rules.open_duration_ms (bawaan 30000).
	DefaultOpenDuration = 30 * time.Second

	// DefaultHalfOpenProbes mengambil routing_rules.half_open_probes (bawaan 1).
	DefaultHalfOpenProbes = 1
)

// Batas yang mengikuti check constraint routing_rules di migrasi 0008.
//
// Diulang di sini bukan karena kurang percaya pada database, tetapi karena konfigurasi
// bisa datang dari tempat lain juga (berkas konfigurasi, test, pemanggil yang menyusun
// BreakerConfig dengan tangan). Nilai di luar rentang ini tidak ditolak melainkan
// dijepit: konfigurasi yang aneh tidak boleh membuat breaker menolak jalan, karena breaker
// yang tidak jalan berarti tidak ada apa pun yang melindungi upstream — kegagalan yang
// jauh lebih besar daripada ambang yang bergeser sedikit dari yang diminta.
const (
	minOpenDuration   = time.Second // routing_rules_open_duration
	maxOpenDuration   = time.Hour   // routing_rules_open_duration
	minHalfOpenProbes = 1           // routing_rules_half_open_range
	maxHalfOpenProbes = 100         // routing_rules_half_open_range

	// Window tidak punya kolom sendiri di routing_rules; rentangnya mengikuti
	// OpenDuration karena keduanya menjawab pertanyaan yang sejenis, "seberapa jauh ke
	// belakang breaker ini mengingat".
	minWindow = time.Second
	maxWindow = time.Hour
)

// BreakerConfig adalah konfigurasi satu breaker.
//
// Per pemanggil, bukan konstanta paket: kesabaran yang pantas berbeda per jenis lalu
// lintas, dan itulah sebabnya routing_rules menyimpan angka-angka ini per aturan
// alih-alih sekali untuk seluruh gateway.
//
// Nilai nol pada setiap field berarti "pakai bawaan", sehingga pemanggil boleh mengisi
// sebagian saja. Normalisasinya terjadi sekali di NewBreaker.
type BreakerConfig struct {
	// FailureThreshold adalah RASIO kegagalan pemicu (0,5 berarti setengah), bukan
	// jumlah kegagalan.
	//
	// Hitungan mentah salah untuk gateway ini karena volume per (provider, model)
	// berbeda ribuan kali: "10 kegagalan" adalah tanda bahaya yang jelas bagi model yang
	// menerima 20 permintaan per menit, dan kebisingan biasa bagi model yang menerima
	// 20.000 — pada yang kedua, breaker akan membuka terus-menerus padahal 99,9%
	// permintaan berhasil.
	//
	// Nilai di atas 1 dijepit menjadi 1. Rasio tidak bisa melebihi 1, dan membiarkannya
	// berarti breaker yang tidak pernah membuka — mati tanpa satu pun tanda.
	FailureThreshold float64

	// MinSamples adalah jumlah sampel minimum di dalam window sebelum rasio dipercaya.
	//
	// Tanpa ini breaker praktis rusak: kegagalan pertama setelah window kosong
	// menghasilkan rasio 1/1 = 100%, jadi SATU permintaan gagal — timeout tunggal,
	// klien yang membatalkan, 500 sekali dari upstream yang sehat — sudah cukup untuk
	// menutup satu provider selama OpenDuration. Ambang rasio hanya bermakna kalau
	// penyebutnya cukup besar untuk membedakan pola dari kecelakaan.
	//
	// Nilai 1 sah dan berarti tepat perilaku di atas; itu pilihan pemanggil, bukan
	// bawaan.
	MinSamples int

	// Window adalah panjang sliding window penghitungan.
	//
	// Geser, bukan tetap: pada jendela tetap, kegagalan yang menumpuk persis di
	// perbatasan hilang saat penghitung berpindah, sehingga upstream yang gagal terus
	// bisa lolos hanya karena kegagalannya terbelah dua jendela.
	Window time.Duration

	// OpenDuration adalah lama breaker menolak lalu lintas sebelum boleh diuji lagi.
	OpenDuration time.Duration

	// HalfOpenProbes adalah jumlah permintaan yang diizinkan lewat saat menguji.
	HalfOpenProbes int
}

// DefaultBreakerConfig mengembalikan konfigurasi bawaan.
func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		FailureThreshold: DefaultFailureThreshold,
		MinSamples:       DefaultMinSamples,
		Window:           DefaultWindow,
		OpenDuration:     DefaultOpenDuration,
		HalfOpenProbes:   DefaultHalfOpenProbes,
	}
}

// normalized mengisi nilai nol dengan bawaan lalu menjepit sisanya ke rentang yang sah.
//
// Dipanggil sekali di NewBreaker, bukan di setiap permintaan: hasilnya dipakai untuk
// menyusun argumen skrip Lua pada jalur request, dan menjepit ulang angka yang tidak
// berubah adalah pekerjaan yang tidak pernah menghasilkan apa pun.
func (c BreakerConfig) normalized() BreakerConfig {
	// NaN ikut ditangkap: seluruh perbandingan dengan NaN bernilai false, jadi ambang
	// NaN menghasilkan breaker yang tidak pernah membuka — kegagalan yang paling sunyi
	// dari semuanya.
	if c.FailureThreshold <= 0 || math.IsNaN(c.FailureThreshold) {
		c.FailureThreshold = DefaultFailureThreshold
	}
	if c.FailureThreshold > 1 {
		c.FailureThreshold = 1
	}
	if c.MinSamples <= 0 {
		c.MinSamples = DefaultMinSamples
	}
	if c.Window <= 0 {
		c.Window = DefaultWindow
	}
	c.Window = min(max(c.Window, minWindow), maxWindow)
	if c.OpenDuration <= 0 {
		c.OpenDuration = DefaultOpenDuration
	}
	c.OpenDuration = min(max(c.OpenDuration, minOpenDuration), maxOpenDuration)
	if c.HalfOpenProbes <= 0 {
		c.HalfOpenProbes = DefaultHalfOpenProbes
	}
	c.HalfOpenProbes = min(max(c.HalfOpenProbes, minHalfOpenProbes), maxHalfOpenProbes)
	return c
}

// bucketWidth adalah lebar satu ember penghitung di dalam window.
//
// Window dipecah menjadi windowBuckets ember, dan yang kedaluwarsa dibuang seluruhnya.
// Konsekuensinya sampel bisa hidup sedikit lebih lama daripada Window — paling lama
// Window + bucketWidth, karena ember terlama baru dibuang setelah seluruh isinya keluar
// dari jendela. Alternatifnya — satu entri per permintaan di sorted set — membuat memori
// dan biaya per permintaan tumbuh mengikuti lalu lintas: pada 1.000 permintaan per detik
// dengan window satu menit, itu 60.000 anggota per pasangan (provider, model) yang harus
// dipangkas terus-menerus. Ember menahan biayanya tetap, dan ketidaktelitian sebesar
// sepersepuluh window pada ambang yang memang perkiraan tidak mengubah keputusan apa pun.
func (c BreakerConfig) bucketWidth() time.Duration {
	w := c.Window / windowBuckets
	if w < time.Millisecond {
		w = time.Millisecond
	}
	return w
}

// --- Tetapan internal --------------------------------------------------------

const (
	// windowBuckets adalah jumlah ember per window. Sepuluh adalah kompromi antara
	// ketelitian kedaluwarsa (Window/10) dan ukuran hash di Redis: setiap ember yang
	// pernah terpakai memakan dua field, dan skrip membacanya seluruhnya sekali per
	// pencatatan.
	windowBuckets = 10

	// memoTTL adalah umur cache keputusan di memori proses.
	//
	// Sengaja hanya ratusan milidetik. TTL ini adalah SELISIH WAKTU maksimum antara
	// sebuah instance dan kenyataan di Redis, dan ia berlaku di dua arah yang keduanya
	// merugikan: breaker yang baru membuka di instance lain masih dilewati instance ini
	// selama memo hidup, dan breaker yang sudah boleh dicoba lagi (atau baru saja
	// di-Reset operator dari dashboard) masih ditolak selama itu juga. Pada 1.000
	// permintaan per detik, memo lima detik berarti sampai 5.000 permintaan tetap
	// dikirim ke upstream yang sudah dinyatakan mati — persis kerugian yang menjadi
	// alasan breaker ini ada. Dua ratus milidetik lebih pendek daripada hampir semua
	// panggilan model, jadi ia menghemat satu putaran Redis pada lonjakan permintaan
	// serentak tanpa membuat keputusannya basi dalam ukuran yang berarti.
	//
	// Yang menjaga memo tetap segar bukan TTL ini saja: Record menuliskan kembali hasil
	// yang dikembalikan Redis, jadi pada lalu lintas normal — satu Allow lalu satu Record
	// per permintaan — memo diperbarui oleh penulisan yang memang sudah harus terjadi,
	// dan usianya tidak pernah mendekati TTL.
	memoTTL = 200 * time.Millisecond

	// memoMaxEntries membatasi jumlah entri cache di memori.
	//
	// Kuncinya berasal dari pasangan (provider, model) yang jumlahnya ditentukan katalog
	// operator, jadi normalnya ratusan. Batas ini menjaga agar pemanggil yang keliru
	// meneruskan nama model mentah dari klien tidak mengubah cache ini menjadi kebocoran
	// memori yang bisa dipicu dari luar.
	memoMaxEntries = 4096
)

// errNoProvider dikembalikan bila ID provider kosong.
//
// Bukan sekadar kerapian: cache.CircuitBreakerKey("", m) menghasilkan kunci tanpa segmen
// provider, sehingga SELURUH pemanggil tanpa provider akan berbagi satu breaker dan
// saling membuka-tutup. Permintaannya tetap diloloskan — ini bug pemanggil, bukan alasan
// menolak lalu lintas pengguna — tetapi errornya harus terlihat.
var errNoProvider = errors.New("gateway: ID provider kosong, breaker dilewati")

// --- Skrip Lua ---------------------------------------------------------------

// Prelude yang dipakai bersama ketiga skrip.
//
// Penurunan state duduk di sini, bukan disalin ke masing-masing skrip, karena Allow dan
// State WAJIB menjawab hal yang sama. Kalau keduanya menurunkannya sendiri-sendiri,
// dashboard bisa menampilkan "closed" untuk breaker yang sedang menolak seluruh lalu
// lintas, dan perbedaan semacam itu tidak memunculkan error apa pun — ia hanya membuat
// operator mencari penyebab pemadaman di tempat yang salah.
//
// Angka Lua tidak diserahkan langsung ke redis.call di bawah; yang dikirim selalu ARGV
// (sudah berupa teks) atau hasil string.format('%d', ...). Bentuk teks sebuah angka Lua
// ditentukan format pecahan bawaan mesinnya (%.14g pada Redis), yang beralih ke notasi
// bereksponen begitu angkanya cukup panjang. Nama field ember dicocokkan huruf per huruf
// saat dibaca kembali, dan argumen PEXPIRE harus berupa bilangan bulat; menuliskan
// keduanya secara eksplisit membuat keduanya tidak lagi bergantung pada kesepakatan
// antara Redis dan implementasi Lua mana pun yang dipakai test.
const breakerLuaPrelude = `
local function read_hash(key)
	local flat = redis.call('HGETALL', key)
	local h = {}
	for i = 1, #flat, 2 do
		h[flat[i]] = flat[i + 1]
	end
	return h
end

local function num(h, field)
	return tonumber(h[field]) or 0
end

-- State DITURUNKAN dari open_until, tidak disimpan sebagai field.
--
-- Perpindahan open -> half_open terjadi karena waktu berlalu, bukan karena ada yang
-- menuliskannya. Kalau state disimpan, satu-satunya cara ia berubah adalah lewat
-- penulisan, sehingga pembacaan murni untuk dashboard akan melaporkan "open" untuk
-- breaker yang sebenarnya sudah boleh diuji — dan satu-satunya jalan keluarnya adalah
-- membuat pembacaan ikut menulis, yang menghapus arti "membaca tanpa mengubah apa pun".
local function state_of(h, now)
	local open_until = num(h, 'open_until')
	if open_until == 0 then
		return 'closed'
	end
	if now < open_until then
		return 'open'
	end
	return 'half_open'
end
`

// allowScript memutuskan apakah satu permintaan boleh lewat.
//
// KEYS[1] kunci breaker.
// ARGV[1] sekarang (ms epoch), ARGV[2] kuota probe, ARGV[3] batas tunggu probe (ms),
// ARGV[4] TTL kunci (ms).
// Kembalian: {boleh (0/1), state, sisa open (ms)}.
//
// Harus atomik dan tidak bisa dipecah menjadi GET lalu HSET: kuota probe adalah kuota,
// dan dua instance yang membacanya bersamaan akan sama-sama menyimpulkan masih ada sisa,
// lalu sama-sama mengirim permintaan ke upstream yang baru saja dinyatakan sakit. Justru
// pada saat itulah lalu lintas paling padat, karena semua breaker di semua instance
// beralih ke half_open pada detik yang sama.
var allowScript = cache.NewScript(breakerLuaPrelude + `
local now = tonumber(ARGV[1])
local h = read_hash(KEYS[1])
local state = state_of(h, now)

if state == 'closed' then
	-- Jalur terpanas, dan ia tidak menulis apa pun: satu HGETALL, tanpa PEXPIRE.
	-- Memperbarui TTL di sini akan membuat kunci yang macet di half_open ikut
	-- diperpanjang oleh permintaan yang ditolaknya sendiri, sehingga jaring pengaman
	-- terakhir (kunci kedaluwarsa lalu breaker pulih) tidak pernah menjala.
	return {1, state, 0}
end

if state == 'open' then
	return {0, state, num(h, 'open_until') - now}
end

local granted = num(h, 'probes')
local since = num(h, 'probes_at')
if since == 0 or now - since >= tonumber(ARGV[3]) then
	-- Episode probe baru. Ini yang menyelamatkan breaker dari macet: kalau proses yang
	-- memegang probe sebelumnya mati sebelum melaporkan hasil, kuotanya tidak akan
	-- pernah kembali, dan tanpa kedaluwarsa episode breaker ini akan menolak SELURUH
	-- lalu lintas ke (provider, model) itu tanpa batas waktu.
	granted, since = 0, now
end
if granted >= tonumber(ARGV[2]) then
	return {0, state, 0}
end
redis.call('HSET', KEYS[1], 'probes', string.format('%d', granted + 1),
	'probes_at', string.format('%d', since))
redis.call('PEXPIRE', KEYS[1], ARGV[4])
return {1, state, 0}
`)

// recordScript mencatat hasil satu percobaan dan memindahkan state bila perlu.
//
// KEYS[1] kunci breaker.
// ARGV[1] sekarang (ms epoch), ARGV[2] sukses (0/1), ARGV[3] ambang rasio,
// ARGV[4] sampel minimum, ARGV[5] lebar ember (ms), ARGV[6] jumlah ember,
// ARGV[7] OpenDuration (ms), ARGV[8] TTL kunci (ms).
// Kembalian: {state, sisa open (ms), total sampel, jumlah gagal, state berubah (0/1)}.
//
// "Catat, hitung rasio, lalu buka bila terlampaui" harus menjadi satu langkah. Dipecah,
// dua instance yang mencatat kegagalan bersamaan akan sama-sama membaca rasio sebelum
// kegagalan yang lain masuk, dan keduanya bisa menyimpulkan ambang belum terlampaui —
// breaker yang seharusnya membuka pada permintaan kesepuluh tidak membuka sampai
// permintaan yang jauh setelahnya.
var recordScript = cache.NewScript(breakerLuaPrelude + `
local now = tonumber(ARGV[1])
local ok = tonumber(ARGV[2])
local open_ms = tonumber(ARGV[7])
local h = read_hash(KEYS[1])
local state = state_of(h, now)

if state == 'open' then
	-- Hasil ini milik permintaan yang sudah lolos SEBELUM breaker membuka, jadi ia tidak
	-- boleh mengubah apa pun. Kalau kegagalan seperti ini ikut memperpanjang open_until,
	-- upstream yang lambat mati justru menahan breaker di open lebih lama daripada yang
	-- diminta operator — dan pada kasus terburuk, aliran permintaan yang belum selesai
	-- membuat breaker tidak pernah mencapai half_open sama sekali.
	return {state, num(h, 'open_until') - now, 0, 0, 0}
end

if state == 'half_open' then
	-- DEL, bukan HDEL per field: episode berikutnya — closed maupun open — dimulai
	-- dengan jendela bersih dan kuota probe kosong, dan menyisakan salah satunya berarti
	-- breaker yang baru menutup langsung membawa rasio yang sudah pernah memicunya.
	redis.call('DEL', KEYS[1])
	if ok == 1 then
		return {'closed', 0, 0, 0, 1}
	end
	redis.call('HSET', KEYS[1], 'open_until', string.format('%d', now + open_ms))
	redis.call('PEXPIRE', KEYS[1], ARGV[8])
	return {'open', open_ms, 0, 0, 1}
end

local bucket = math.floor(now / tonumber(ARGV[5]))
local oldest = bucket - tonumber(ARGV[6]) + 1
-- Sampel ini ikut dihitung sebelum ditulis, supaya keputusan di bawah memakai jendela
-- yang sudah memuatnya. Tanpa itu, permintaan yang tepat melampaui ambang baru terdeteksi
-- pada pencatatan berikutnya — yang mungkin tidak pernah datang.
local total, failures = 1, 0
if ok == 0 then
	failures = 1
end

local expired = {}
for field, value in pairs(h) do
	local kind = string.sub(field, 1, 1)
	local idx = tonumber(string.sub(field, 2))
	if idx ~= nil and (kind == 't' or kind == 'f') then
		-- Tidak ada batas atas: ember yang indeksnya di depan "sekarang" tetap dihitung,
		-- bukan dibuang. Ia hanya bisa muncul dari instance yang jamnya berjalan sedikit
		-- lebih cepat, dan membuangnya berarti instance yang paling lambat jamnya
		-- menghapus sampel nyata milik semua instance lain.
		if idx >= oldest then
			if kind == 't' then
				total = total + (tonumber(value) or 0)
			else
				failures = failures + (tonumber(value) or 0)
			end
		else
			expired[#expired + 1] = field
		end
	end
end

redis.call('HINCRBY', KEYS[1], string.format('t%d', bucket), '1')
if ok == 0 then
	redis.call('HINCRBY', KEYS[1], string.format('f%d', bucket), '1')
end
-- Ember di luar jendela dibuang di sini, bukan diserahkan ke TTL kunci: TTL berlaku untuk
-- seluruh kunci, sedangkan yang kedaluwarsa hanya sebagian isinya. Karena daftar field
-- didapat dari HGETALL di atas, pembersihannya lengkap — termasuk ember yang tertinggal
-- dari lalu lintas yang sempat berhenti lama.
for i = 1, #expired do
	redis.call('HDEL', KEYS[1], expired[i])
end

-- Dua syarat, dan yang kedua yang menyelamatkan: tanpa sampel minimum, kegagalan pertama
-- atas jendela kosong menghasilkan rasio 1/1 dan langsung membuka breaker.
if total >= tonumber(ARGV[4]) and failures >= tonumber(ARGV[3]) * total then
	redis.call('DEL', KEYS[1])
	redis.call('HSET', KEYS[1], 'open_until', string.format('%d', now + open_ms))
	redis.call('PEXPIRE', KEYS[1], ARGV[8])
	return {'open', open_ms, total, failures, 1}
end

redis.call('PEXPIRE', KEYS[1], ARGV[8])
return {'closed', 0, total, failures, 0}
`)

// stateScript membaca state tanpa mengubah apa pun.
//
// KEYS[1] kunci breaker, ARGV[1] sekarang (ms epoch).
// Kembalian: {state, sisa open (ms)}.
//
// Berupa skrip walaupun isinya hanya satu HGETALL, semata supaya penurunan state-nya
// benar-benar kode yang sama dengan yang dipakai Allow. Menyalinnya ke Go akan bekerja
// hari ini dan menyimpang pada perubahan berikutnya.
var stateScript = cache.NewScript(breakerLuaPrelude + `
local now = tonumber(ARGV[1])
local h = read_hash(KEYS[1])
local state = state_of(h, now)
local remaining = 0
if state == 'open' then
	remaining = num(h, 'open_until') - now
end
return {state, remaining}
`)

// --- Breaker -----------------------------------------------------------------

// Breaker adalah circuit breaker per pasangan (provider, model).
//
// Alasan pemecahannya sampai level model — bukan per provider — sudah ditulis di
// cache.CircuitBreakerKey dan tidak diulang di sini.
//
// # State di Redis, keputusan di memori
//
// State tinggal di Redis karena ia harus dibagi: gateway berjalan lebih dari satu
// instance, dan breaker yang hanya tahu kegagalan yang dilihat instance-nya sendiri butuh
// N kali lebih banyak permintaan gagal sebelum membuka, lalu membuka di instance yang
// berbeda-beda pada waktu yang berbeda-beda. Yang paling menyesatkan, dashboard tidak akan
// pernah bisa menjawab "breaker mana yang sedang terbuka" — jawabannya berbeda di setiap
// instance.
//
// Di atasnya ada cache keputusan berumur memoTTL di memori proses, supaya lonjakan
// permintaan serentak ke pasangan yang sama tidak menjadi lonjakan putaran Redis sebelum
// setiap permintaan. Yang TIDAK pernah masuk cache itu adalah keputusan saat half_open:
// probe adalah kuota, dan kuota yang di-cache berhenti menjadi kuota — satu izin probe
// akan dipakai ulang oleh setiap permintaan yang datang selama memo hidup, sehingga
// seluruh lalu lintas mengalir ke upstream yang justru sedang diuji dengan hati-hati.
//
// # Gagal terbuka
//
// Redis yang mati atau lambat membuat permintaan DILOLOSKAN, bukan ditolak, dengan log
// peringatan. Alasannya sama seperti pembatas laju login di internal/auth: kalau ia gagal
// tertutup, matinya Redis berubah menjadi matinya seluruh gateway — setiap permintaan ke
// setiap provider ditolak sekaligus, termasuk yang upstream-nya sehat sempurna. Yang
// hilang saat gagal terbuka hanyalah perlindungan terhadap upstream yang sakit, dan
// upstream yang sakit tetap menolak permintaannya sendiri.
//
// # Penerima nil
//
// Penerima nil sah, begitu pula Redis nil, dan keduanya berarti tanpa breaker: seluruh
// permintaan lolos. Deployment tanpa Redis dan test yang tidak memerlukannya memakai jalur
// kode yang sama.
type Breaker struct {
	redis  *cache.Redis
	logger *slog.Logger
	cfg    BreakerConfig

	// now bisa diganti test untuk memeriksa perpindahan state tanpa menunggu waktu nyata
	// berjalan. Waktu diambil di Go, bukan lewat redis.call('TIME'), mengikuti pembatas
	// laju di apikey dan auth. Konsekuensinya diketahui: seluruh tenggat di Redis ditulis
	// dengan jam instance yang kebetulan menulisnya, jadi instance yang jamnya melenceng
	// jauh bisa memperpanjang atau memperpendek satu episode open bagi semua instance
	// lain. Itu ditukar dengan jam yang bisa dikuasai test — tanpanya, satu-satunya cara
	// menguji perpindahan open -> half_open adalah benar-benar menunggu OpenDuration,
	// yang berarti test yang lambat, atau yang cepat tetapi tidak menguji perpindahannya.
	now func() time.Time

	mu            sync.RWMutex
	memo          map[string]memoEntry
	onStateChange StateChangeListener
}

// memoEntry adalah satu keputusan yang di-cache di memori proses.
type memoEntry struct {
	allow   bool
	state   State
	expires time.Time
}

// NewBreaker membuat breaker di atas Redis.
//
// rdb nil menghasilkan breaker yang selalu meloloskan. logger nil jatuh ke slog.Default().
// cfg dinormalkan sekali di sini, jadi pemanggil boleh mengisi sebagian field saja.
func NewBreaker(rdb *cache.Redis, cfg BreakerConfig, logger *slog.Logger) *Breaker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Breaker{
		redis:  rdb,
		logger: logger,
		cfg:    cfg.normalized(),
		now:    time.Now,
		memo:   make(map[string]memoEntry, 16),
	}
}

// StateChangeListener adalah fungsi pemanggil saat status circuit breaker berpindah.
//
// Dipakai terutama untuk memproduksi webhook event circuit.opened dan circuit.closed.
type StateChangeListener func(ctx context.Context, providerID, model string, state State, total, failures int64)

// SetStateChangeListener memasang pemantau perpindahan status circuit breaker.
func (b *Breaker) SetStateChangeListener(fn StateChangeListener) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.onStateChange = fn
	b.mu.Unlock()
}

// Warm memuat ketiga skrip ke cache skrip Redis.

// Tidak wajib — jalur keputusan tetap benar tanpanya — tetapi ia memindahkan satu putaran
// EVALSHA yang gagal ditambah EVAL berisi badan skrip dari permintaan pertama ke jalur
// start.
func (b *Breaker) Warm(ctx context.Context) error {
	if b == nil || b.redis == nil {
		return nil
	}
	if err := b.redis.LoadScript(ctx, allowScript); err != nil {
		return err
	}
	if err := b.redis.LoadScript(ctx, recordScript); err != nil {
		return err
	}
	return b.redis.LoadScript(ctx, stateScript)
}

// Allow melaporkan apakah permintaan ke (providerID, model) boleh jalan.
//
// State yang dikembalikan adalah state pada saat keputusan diambil, supaya pemanggil bisa
// mencatatnya ke log dan ke observability.Metrics.CircuitBreaker lewat State.Gauge().
//
// PENTING: nilai pertama yang menentukan, BUKAN error. Error non-nil berarti keputusannya
// diambil tanpa Redis, dan pada keadaan itu jawabannya selalu "boleh" — lihat catatan
// gagal-terbuka di dokumentasi tipe. Pemanggil yang menulis `if err != nil { return err }`
// membalikkan sifat itu dan mengubah matinya Redis menjadi matinya gateway.
func (b *Breaker) Allow(ctx context.Context, providerID, model string) (bool, State, error) {
	if b == nil || b.redis == nil {
		return true, StateClosed, nil
	}
	if strings.TrimSpace(providerID) == "" {
		return true, StateClosed, errNoProvider
	}

	key := cache.CircuitBreakerKey(providerID, model)
	now := b.now()
	if e, ok := b.memoGet(key, now); ok {
		return e.allow, e.state, nil
	}

	res, err := b.redis.RunScript(ctx, allowScript, []string{key},
		now.UnixMilli(), b.cfg.HalfOpenProbes, b.probeWait().Milliseconds(), b.keyTTL().Milliseconds())
	if err != nil {
		b.log(ctx).Warn("circuit breaker tidak bisa menghubungi redis, permintaan diloloskan",
			"provider", providerID, "model", model, "error", err.Error())
		return true, StateClosed, err
	}

	allow, state, openRemaining := parseAllow(res)
	if state != StateHalfOpen {
		b.memoPut(key, memoEntry{allow: allow, state: state, expires: b.memoExpiry(now, openRemaining)})
	}
	return allow, state, nil
}

// Record mencatat hasil satu percobaan ke (providerID, model).
//
// Wajib dipanggil untuk setiap permintaan yang lolos Allow, termasuk yang gagal karena
// dibatalkan klien atau kehabisan waktu — tanpa hasil, kuota probe half_open tidak
// dikembalikan dan jendela tidak pernah terisi.
//
// Errornya untuk log, bukan untuk dijadikan kegagalan permintaan: yang dicatat adalah
// sesuatu yang SUDAH terjadi, dan Redis yang tidak bisa dihubungi tidak membuatnya
// belum terjadi.
func (b *Breaker) Record(ctx context.Context, providerID, model string, sukses bool) error {
	if b == nil || b.redis == nil {
		return nil
	}
	if strings.TrimSpace(providerID) == "" {
		return errNoProvider
	}

	key := cache.CircuitBreakerKey(providerID, model)
	now := b.now()
	ok := 0
	if sukses {
		ok = 1
	}

	res, err := b.redis.RunScript(ctx, recordScript, []string{key},
		now.UnixMilli(), ok, b.cfg.FailureThreshold, b.cfg.MinSamples,
		b.cfg.bucketWidth().Milliseconds(), windowBuckets,
		b.cfg.OpenDuration.Milliseconds(), b.keyTTL().Milliseconds())
	if err != nil {
		b.log(ctx).Warn("circuit breaker tidak bisa mencatat hasil ke redis",
			"provider", providerID, "model", model, "sukses", sukses, "error", err.Error())
		return fmt.Errorf("gateway: mencatat hasil breaker %s/%s: %w", providerID, model, err)
	}

	state, openRemaining, total, failures, changed := parseRecord(res)
	// Hasil yang baru saja ditulis dipakai untuk menyegarkan jalur cepat. Ini yang membuat
	// memo tetap muda tanpa satu pun putaran Redis tambahan: pada lalu lintas normal
	// setiap Allow diikuti satu Record, jadi keputusan yang dibaca permintaan berikutnya
	// berumur satu permintaan, bukan memoTTL. Record tidak pernah mengembalikan half_open
	// — dari state itu ia selalu menutup atau membuka — sehingga larangan menyimpan
	// keputusan half_open tidak perlu diperiksa lagi di sini.
	b.memoPut(key, memoEntry{
		allow:   state != StateOpen,
		state:   state,
		expires: b.memoExpiry(now, openRemaining),
	})

	if changed {
		// Hanya perpindahan yang dicatat, bukan setiap pencatatan: yang perlu dilihat
		// operator adalah kapan sebuah pasangan (provider, model) berhenti dilayani dan
		// kapan ia kembali, dan satu baris log per permintaan akan menenggelamkan keduanya.
		//
		// Pemulihan dicatat pada level info, bukan warn: alert yang berbunyi ketika sesuatu
		// KEMBALI normal mengajari operator mengabaikan levelnya.
		level := slog.LevelWarn
		if state == StateClosed {
			level = slog.LevelInfo
		}
		b.log(ctx).LogAttrs(ctx, level, "state circuit breaker berubah",
			slog.String("provider", providerID),
			slog.String("model", model),
			slog.String("state", string(state)),
			slog.Int64("sampel", total),
			slog.Int64("gagal", failures),
			slog.Int64("sisa_open_ms", openRemaining.Milliseconds()),
		)

		b.mu.RLock()
		listener := b.onStateChange
		b.mu.RUnlock()
		if listener != nil {
			go listener(context.WithoutCancel(ctx), providerID, model, state, total, failures)
		}
	}
	return nil
}

// State membaca state tanpa mengubah apa pun, untuk dashboard dan metrik.
//
// Tidak memakai maupun mengisi cache di memori: pembacaan dashboard tidak berada di jalur
// request, jadi tidak ada yang perlu dihemat, dan menampilkan keputusan yang di-cache akan
// membuat halaman status menyembunyikan perpindahan yang baru saja terjadi.
func (b *Breaker) State(ctx context.Context, providerID, model string) (State, error) {
	if b == nil || b.redis == nil {
		return StateClosed, nil
	}
	if strings.TrimSpace(providerID) == "" {
		return StateClosed, errNoProvider
	}

	res, err := b.redis.RunScript(ctx, stateScript,
		[]string{cache.CircuitBreakerKey(providerID, model)}, b.now().UnixMilli())
	if err != nil {
		// Closed dikembalikan bersama error, sejalan dengan gagal-terbuka: pembacaan yang
		// gagal tidak boleh membuat dashboard melaporkan pemadaman yang tidak ada.
		return StateClosed, fmt.Errorf("gateway: membaca state breaker %s/%s: %w", providerID, model, err)
	}
	state, _ := parseState(res)
	return state, nil
}

// Reset memaksa breaker menutup, untuk tombol operator di dashboard.
//
// Yang dihapus adalah seluruh kunci: jendela, tenggat open, dan kuota probe sekaligus.
// Menyisakan jendela berarti breaker yang baru ditutup paksa oleh operator langsung membawa
// rasio yang tadi memicunya, sehingga kegagalan pertama setelah reset membukanya lagi.
//
// Berbeda dari Allow dan Record, fungsi ini TIDAK gagal terbuka: ia menjawab tindakan
// operator, dan operator yang menekan tombol harus tahu tombolnya tidak bekerja.
//
// Cache di memori instance ini ikut dibuang, tetapi instance lain baru menyusul setelah
// memo miliknya kedaluwarsa — jadi lalu lintas bisa masih tertolak hingga memoTTL setelah
// reset dilaporkan berhasil.
func (b *Breaker) Reset(ctx context.Context, providerID, model string) error {
	if b == nil || b.redis == nil {
		return nil
	}
	if strings.TrimSpace(providerID) == "" {
		return errNoProvider
	}

	key := cache.CircuitBreakerKey(providerID, model)
	// DEL satu kunci sudah merupakan satu perintah yang tak bisa disalip, jadi skrip Lua
	// di sini hanya akan menambah lapisan tanpa menambah jaminan.
	if err := b.redis.Client().Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("gateway: mereset breaker %s/%s: %w", providerID, model, err)
	}
	b.memoDrop(key)
	return nil
}

// --- Tenggat turunan ---------------------------------------------------------

// probeWait adalah batas menunggu hasil sebuah probe half_open.
//
// Tanpa batas ini breaker bisa macet: proses yang mendapat izin probe lalu mati — atau
// permintaan yang dibatalkan sebelum Record dipanggil — tidak pernah mengembalikan
// kuotanya, dan breaker berhenti di half_open dengan kuota habis sambil menolak SELURUH
// lalu lintas ke pasangan itu. Pemadaman itu lebih buruk daripada yang dicegah breaker,
// dan ia tidak pulih sendiri sampai kuncinya kedaluwarsa.
//
// Nilainya OpenDuration, bukan knob tersendiri: operator sudah menyatakan berapa lama ia
// bersedia menunggu sebelum upstream yang sakit dicoba lagi, dan menunggu selama itu untuk
// hasil yang tidak datang adalah kesabaran yang sama. Akibatnya diketahui — permintaan yang
// berjalan lebih lama daripada OpenDuration bisa membuat probe berikutnya diberikan
// sebelum yang pertama menjawab, sehingga beberapa permintaan percobaan, bukan tepat
// HalfOpenProbes, sampai ke upstream. Melebihi kuota probe sesaat jauh lebih murah
// daripada breaker yang macet.
func (b *Breaker) probeWait() time.Duration { return b.cfg.OpenDuration }

// keyTTL adalah masa berlaku kunci breaker di Redis.
//
// Dua gunanya. Pertama, mengumpulkan sampah: pasangan (provider, model) yang tidak dipakai
// lagi — model yang ditarik dari peredaran, provider yang dihapus operator — tidak
// menyisakan kunci selamanya. Kedua, jaring pengaman terakhir: keadaan apa pun yang membuat
// sebuah breaker berhenti diperbarui berakhir dengan kunci yang hilang dan breaker yang
// pulih ke closed, bukan yang menolak lalu lintas tanpa batas waktu.
//
// Jumlah ketiga tenggat, bukan yang terpanjang. TTL diperbarui pada setiap penulisan, jadi
// yang harus dijamin adalah ia selalu melebihi jarak antar penulisan yang masih bermakna:
// Window untuk sampel yang masih dihitung, OpenDuration untuk episode open yang sedang
// berjalan, probeWait untuk probe yang belum bersuara. Menjumlahkannya memenuhi ketiganya
// sekaligus dengan margin, tanpa kode ini perlu tahu state mana yang sedang berlaku.
func (b *Breaker) keyTTL() time.Duration {
	return b.cfg.Window + b.cfg.OpenDuration + b.probeWait()
}

// --- Jalur cepat di memori ---------------------------------------------------

// memoExpiry menghitung batas berlaku satu entri jalur cepat.
//
// Untuk state open, batasnya dipotong pada saat episode open berakhir. Tanpa pemotongan
// itu, memo yang dibuat sesaat sebelum tenggat open lewat akan terus menolak permintaan
// setelah breaker sebenarnya sudah boleh diuji, dan penundaan itu terjadi di setiap
// instance sekaligus — pemulihan yang tertunda tanpa alasan.
func (b *Breaker) memoExpiry(now time.Time, openRemaining time.Duration) time.Time {
	if openRemaining > 0 && openRemaining < memoTTL {
		return now.Add(openRemaining)
	}
	return now.Add(memoTTL)
}

func (b *Breaker) memoGet(key string, now time.Time) (memoEntry, bool) {
	b.mu.RLock()
	e, ok := b.memo[key]
	b.mu.RUnlock()
	if !ok || !now.Before(e.expires) {
		return memoEntry{}, false
	}
	return e, true
}

func (b *Breaker) memoPut(key string, e memoEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.memo == nil {
		b.memo = make(map[string]memoEntry, 16)
	}
	// Seluruh cache dibuang, bukan satu entri terlama: tanpa urutan pemakaian yang
	// disimpan, "terlama" hanya bisa ditebak, dan Redis tetap menjadi sumber kebenaran —
	// yang hilang cuma penghematan satu putaran untuk beberapa ratus milidetik.
	if _, replacing := b.memo[key]; !replacing && len(b.memo) >= memoMaxEntries {
		b.memo = make(map[string]memoEntry, 16)
	}
	b.memo[key] = e
}

func (b *Breaker) memoDrop(key string) {
	b.mu.Lock()
	delete(b.memo, key)
	b.mu.Unlock()
}

// log mengembalikan logger yang sudah dibubuhi request_id bila context punya.
func (b *Breaker) log(ctx context.Context) *slog.Logger {
	if id := observability.RequestIDFrom(ctx); id != "" {
		return b.logger.With(slog.String("request_id", id))
	}
	return b.logger
}

// --- Pembacaan hasil skrip ---------------------------------------------------
//
// Ketiga pembaca di bawah ditulis defensif dan tidak pernah mengembalikan error. Hasil
// skrip melewati pemetaan tipe go-redis (angka Lua menjadi int64, string menjadi string,
// table menjadi []any), dan bentuk yang tidak dikenali hanya bisa muncul kalau skrip dan
// Go tidak lagi sepakat. Pada keadaan itu jawaban yang aman adalah closed: sama seperti
// gagal terbuka, keraguan tidak boleh menahan lalu lintas.

// parseAllow membaca {boleh, state, sisa open} dari allowScript.
func parseAllow(res any) (allow bool, state State, openRemaining time.Duration) {
	v, ok := res.([]any)
	if !ok || len(v) < 3 {
		return true, StateClosed, 0
	}
	n, _ := v[0].(int64)
	name, _ := v[1].(string)
	ms, _ := v[2].(int64)
	return n == 1, stateFromLua(name), msToDuration(ms)
}

// parseRecord membaca {state, sisa open, total, gagal, berubah} dari recordScript.
func parseRecord(res any) (state State, openRemaining time.Duration, total, failures int64, changed bool) {
	v, ok := res.([]any)
	if !ok || len(v) < 5 {
		return StateClosed, 0, 0, 0, false
	}
	name, _ := v[0].(string)
	ms, _ := v[1].(int64)
	total, _ = v[2].(int64)
	failures, _ = v[3].(int64)
	ch, _ := v[4].(int64)
	return stateFromLua(name), msToDuration(ms), total, failures, ch == 1
}

// parseState membaca {state, sisa open} dari stateScript.
func parseState(res any) (State, time.Duration) {
	v, ok := res.([]any)
	if !ok || len(v) < 2 {
		return StateClosed, 0
	}
	name, _ := v[0].(string)
	ms, _ := v[1].(int64)
	return stateFromLua(name), msToDuration(ms)
}

// stateFromLua menerjemahkan nama state dari skrip menjadi State.
func stateFromLua(name string) State {
	switch s := State(name); s {
	case StateClosed, StateOpen, StateHalfOpen:
		return s
	}
	return StateClosed
}

// msToDuration mengubah milidetik dari skrip menjadi durasi, membuang nilai negatif.
//
// Sisa open bisa terbaca negatif bila tenggatnya lewat persis di antara pembacaan hash dan
// pembandingannya; nol adalah jawaban yang benar untuk itu, dan durasi negatif akan membuat
// pemotongan umur memo di memoExpiry salah arah.
func msToDuration(ms int64) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
