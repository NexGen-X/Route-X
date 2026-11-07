// Package responsecache menyediakan caching respons inferensi cerdas berbasis Redis
// untuk memotong latensi hingga < 2ms dan menghemat biaya token untuk prompt berulang.
package responsecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// DefaultTTL bawaan untuk entri cache respons (1 jam).
const DefaultTTL = 1 * time.Hour

// MaxEntryBytes adalah batas ukuran satu entri terserialisasi yang boleh
// disimpan ke Redis (2 MB). Jalur streaming sudah membatasi penampungan
// chunk pada angka yang sama; tanpa batas di titik simpan bersama ini,
// respons non-streaming sebesar apa pun tersalin utuh ke Redis
// (audit3 V-GW-08).
const MaxEntryBytes = 2 << 20

// DefaultStatsTTL bawaan untuk metrik statistik cache respons (30 hari).
const DefaultStatsTTL = 30 * 24 * time.Hour

// Entry adalah satu respons inferensi yang disimpan di Redis.
type Entry struct {
	Model string `json:"model"`
	// ProviderID adalah providers.id yang melayani, dipakai menegakkan
	// penyaring jawaban berlingkup provider pada jalur HIT.
	ProviderID string          `json:"provider_id,omitempty"`
	Raw        json.RawMessage `json:"raw"`
	Usage      providers.Usage `json:"usage"`
	// StreamChunks adalah warisan skema entri streaming pra-Batch C1.
	// Tahap awal aktivasi tidak lagi menyimpan maupun menyajikan
	// streaming, jadi field ini tidak pernah diisi kode saat ini; ia
	// dipertahankan hanya supaya payload entri lama tetap terbaca.
	StreamChunks []string `json:"stream_chunks,omitempty"`
	// Teks adalah teks jawaban yang sudah diekstrak saat menyimpan, supaya
	// jalur HIT non-streaming tetap bisa menjalankan penyaring jawaban tanpa
	// mengurai ulang Raw. Entri streaming ikut mengisinya dari gabungan
	// chunk (audit3 V-GW-04); kosong hanya pada entri warisan yang disimpan
	// sebelum teks mulai direkam — jalur HIT mengekstraknya dari Raw.
	Teks     string    `json:"teks,omitempty"`
	CachedAt time.Time `json:"cached_at"`
	// Generasi adalah cap generasi cache saat entri ini ditulis
	// (mekanisme generasi B-473). Entri hanya boleh disajikan selama
	// generasinya sama dengan generasi cache yang sedang berjalan;
	// setiap Flush dan setiap pergeseran konteks jawaban menaikkan
	// generasi, jadi entri dari generasi lama terbaca sebagai MISS
	// walau kunci Redis-nya masih hidup. Entri warisan yang ditulis
	// sebelum mekanisme ini ada membawa 0 — sama dengan generasi awal.
	Generasi int64 `json:"generasi,omitempty"`
}

// Stats merangkum metrik performa response cache.
type Stats struct {
	Enabled      bool  `json:"enabled"`
	TTLSeconds   int64 `json:"ttl_seconds"`
	Hits         int64 `json:"hits"`
	Misses       int64 `json:"misses"`
	TotalEntries int64 `json:"total_entries"`
}

// Engine mengelola pembacaan, penulisan, dan pembersihan cache respons.
type Engine struct {
	redis  *cache.Redis
	logger *slog.Logger

	mu      sync.RWMutex
	enabled bool
	ttl     time.Duration
}

// SettingKey adalah kunci baris settings tempat setelan cache yang
// diubah admin dipersistensikan (dipakai bersama cmd dan admin).
const SettingKey = "system:response_cache"

// PersistedSettings adalah bentuk JSON setelan response cache di tabel
// settings: satu-satunya sumber kebenaran yang bertahan melewati restart.
type PersistedSettings struct {
	Enabled    bool  `json:"enabled"`
	TTLSeconds int64 `json:"ttl_seconds"`
}

// NewEngine membuat Engine baru. Bila redis nil, caching otomatis tidak aktif.
//
// Bawaan engine adalah NONAKTIF (audit F3): versi lama menyala sejak boot
// padahal endpoint admin justru MENOLAK menyalakannya sebagai tidak aman
// (cache_policy_unsafe) dan setelan admin tidak dipersistensikan — jadi
// setiap restart diam-diam menyalakan lagi fitur yang oleh sistem sendiri
// dinyatakan belum aman. Kini cache hanya menyala dari setelan yang
// dipersistensikan secara eksplisit (pemulihan saat boot di cmd).
//
// Penolakan aktivasi itu dicabut pada Batch C1, sesudah kebijakan sidik
// kunci yang dituntutnya benar-benar diterapkan di ComputeScopedKey:
// isolasi per API key (identitas pemilik & kunci ter-hash terpisah
// domain), model kanonik, permukaan (OpenAI/Anthropic), dan sidik rute —
// perubahan routing/provider/pemetaan mengubah kunci sehingga entri lama
// yatim dan kedaluwarsa oleh TTL, bukan tersaji ke konteks yang berubah.
func NewEngine(r *cache.Redis, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		redis:   r,
		logger:  logger,
		enabled: false,
		ttl:     DefaultTTL,
	}
}

// SetSettings memperbarui setelan runtime response cache.
func (e *Engine) SetSettings(enabled bool, ttl time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = enabled
	if ttl > 0 {
		e.ttl = ttl
	}
}

// IsEnabled memeriksa apakah caching sedang aktif.
func (e *Engine) IsEnabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.enabled && e.redis != nil
}

// TTL mengembalikan durasi simpan cache saat ini.
func (e *Engine) TTL() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ttl
}

// --- Mekanisme generasi cache (B-473/B-474) ---
//
// Invariant: sebuah entri hanya boleh disajikan atau ditanam oleh
// request yang CAP generasinya sama dengan generasi cache yang sedang
// berjalan. Setiap Flush menaikkan generasi, dan pergeseran konteks
// jawaban yang tidak melewati Flush (pergeseran pemenang kredensial,
// B-474) menaikkannya lewat NaikkanGenerasi. Dengan itu balapan
// in-flight tidak bisa lagi menanam jawaban basi: request menghitung
// kunci pada generasi G, eksekusi berjalan lama, Flush/kenaikan terjadi
// di tengah (generasi menjadi G+1), lalu Set-nya tiba membawa cap G —
// dan ditolak, karena konteks yang menghasilkan jawaban itu sudah
// bukan konteks yang berlaku. Tanpa mekanisme ini keluarga flush
// selalu kalah oleh dimensi waktu: siraman hanya membuang entri yang
// SUDAH tertanam, bukan yang sedang dalam perjalanan.

// kunciGenerasi adalah kunci Redis counter generasi cache.
func (e *Engine) kunciGenerasi() string {
	return cache.Key(cache.NamespaceResponseCache, "meta", "generasi")
}

// GenerasiSaatIni membaca nomor generasi cache yang sedang berjalan.
// Kunci yang belum pernah lahir berarti generasi 0.
func (e *Engine) GenerasiSaatIni(ctx context.Context) (int64, error) {
	if e.redis == nil {
		return 0, nil
	}
	gen, err := e.redis.Client().Get(ctx, e.kunciGenerasi()).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("membaca generasi response cache: %w", err)
	}
	return gen, nil
}

// NaikkanGenerasi menaikkan generasi cache TANPA menghapus entri:
// entri generasi lama cukup berhenti disajikan (terbaca MISS) dan
// kedaluwarsa oleh TTL. Dipakai untuk pergeseran konteks yang tidak
// membenarkan siraman penuh — terutama pergeseran pemenang kredensial
// (B-474) — di mana menghapus seluruh isi cache akan membuang entri
// yang tidak berhubungan dengan pergeseran itu.
func (e *Engine) NaikkanGenerasi(ctx context.Context) (int64, error) {
	if e.redis == nil {
		return 0, nil
	}
	gen, err := e.redis.Client().Incr(ctx, e.kunciGenerasi()).Result()
	if err != nil {
		return 0, fmt.Errorf("menaikkan generasi response cache: %w", err)
	}
	return gen, nil
}

// ComputeKey menghitung signature SHA-256 unik dari permintaan chat completion.
//
// Kunci WAJIB memuat seluruh field yang mengubah respons upstream: tanpa itu,
// dua request berbeda (beda TopP, Stop, ToolChoice, Seed, dst.) mendapat kunci
// sama dan klien menerima respons milik request lain (BE-004).
//
// Kunci WAJIB memuat konteks pemilik lewat Scope: model kanonik (models.id),
// pemilik API key, dan key ID. Tanpa itu dua penyewa berbagi satu entri —
// tenant B menerima respons milik tenant A, kuota token tenant B tidak termakan,
// dan biaya tetap dicatat penuh padahal tidak ada panggilan upstream. Kunci
// TIDAK memakai nama model mentah dari klien (alias bisa berbeda antar
// penyewa untuk model yang sama), melainkan ID kanonik dari parameter.
// Status (triage B-005): pintu kompatibilitas mundur — mendelegasikan ke ComputeScopedKey dengan Scope kosong. Jalur produksi selalu memakai ComputeScopedKey dengan konteks pemilik terisi.
func (e *Engine) ComputeKey(req *providers.ChatRequest) string {
	return e.ComputeScopedKey(req, Scope{})
}

// Scope adalah konteks pemilik yang ikut membentuk kunci cache.
//
// Semuanya string polos, bukan tipe internal, supaya paket ini tidak bergantung
// pada paket apikey maupun upstream: arah ketergantungannya adalah gateway yang
// memakai paket ini, bukan sebaliknya.
type Scope struct {
	// ModelID adalah models.id kanonik hasil resolusi alias, bukan nama mentah
	// dari klien. Dua alias berbeda ke model yang sama tetap berbagi entri —
	// justru itu yang benar, karena respons upstream-nya identik.
	ModelID string
	// OwnerUserID adalah pemilik API key peminta (string kosong = tanpa pemilik).
	// Di dalam ComputeScopedKey nilai ini di-hash terpisah domain sebelum
	// dicampur ke kunci — tidak pernah tampil sebagai nilai mentah.
	OwnerUserID string
	// KeyID adalah UUID baris api_keys peminta. Ikut dimasukkan supaya key yang
	// dibatasi model/provider-nya tidak memakan entri milik key yang longgar,
	// dan sebaliknya. Sama seperti OwnerUserID, nilainya di-hash terpisah
	// domain di dalam ComputeScopedKey.
	KeyID string
	// Permukaan adalah dialek permukaan yang melayani ("openai" atau
	// "anthropic"). Tubuh respons kedua permukaan berbeda bentuk untuk
	// request kanonik yang sama, jadi entri satu permukaan tidak boleh
	// melayani permukaan lain.
	Permukaan string
	// SidikRute adalah hash keputusan routing saat kunci dihitung (aturan +
	// daftar kandidat terurut: provider, pemetaan model, base URL). Ia
	// menjawab keberatan historis V-GW-02/03: jawaban fallback combo dulu
	// tersimpan di bawah kunci model yang diminta dan tetap tersaji walau
	// routing berubah. Kini perubahan provider/routing/pemetaan mengubah
	// sidik ini, sehingga entri lama tidak terjangkau lagi dan kedaluwarsa
	// oleh TTL — invalidasi implisit tanpa pemindaian.
	SidikRute string
}

// sidikIdentitas meng-hash satu nilai identitas (pemilik / ID kunci API)
// dengan pemisah domain, supaya identitas ikut membentuk kunci cache tanpa
// pernah menjadi bagian yang terbaca dari bahan kunci. Kunci akhir memang
// sudah SHA-256 atas seluruh bahan, tetapi meng-hash identitas secara
// terpisah memastikan nilai mentahnya tidak bergantung pada lapisan itu
// saja (pertahanan berlapis, Batch C1) — dan dua konteks berbeda tidak
// bisa bertabrakan lewat penyusunan ulang field lain.
func sidikIdentitas(nilai string) string {
	if nilai == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("routex-responsecache-identitas-v1\x00" + nilai))
	return hex.EncodeToString(sum[:])
}

// ComputeScopedKey menghitung kunci cache dengan konteks pemilik.
//
// Kompatibel mundur dengan entri lama: Scope kosong menghasilkan kunci yang
// sama persis dengan ComputeKey lama, sehingga deploy tidak serta-merta
// membuang seluruh isi cache yang ada.
func (e *Engine) ComputeScopedKey(req *providers.ChatRequest, scope Scope) string {
	if req == nil {
		return ""
	}

	// B-257: nama model MENTAH dari klien (alias) tidak boleh ikut membentuk
	// kunci bila ID kanonik tersedia — bahan kunci di bawah meng-hash request
	// penuh, jadi tanpa normalisasi ini dua alias berbeda ke model yang sama
	// menghasilkan kunci berbeda dan entri tidak pernah terbagi, persis
	// menggagalkan maksud Scope.ModelID. Salinan dangkal cukup: hanya field
	// Model yang diganti, pesan-pesannya dibagi apa adanya. Scope kosong
	// mempertahankan perilaku lama persis (kompatibilitas ComputeKey).
	permintaan := req
	if scope.ModelID != "" {
		salinan := *req
		salinan.Model = scope.ModelID
		permintaan = &salinan
	}

	// encoding/json mempertahankan urutan field struct dan mengurutkan kunci map.
	// Meng-hash representasi request penuh juga menghindari delimiter ambigu serta
	// pembulatan float yang sebelumnya dapat menyatukan request berbeda.
	contents := make([]string, len(req.Messages))
	for i := range req.Messages {
		// providers.Message.Content sengaja bertag json:"-" karena adapter
		// menangani bentuk wire sendiri, jadi nilainya harus ditambahkan eksplisit.
		contents[i] = req.Messages[i].Content
	}
	canonical, err := json.Marshal(struct {
		Request         *providers.ChatRequest `json:"request"`
		MessageContents []string               `json:"message_contents"`
		ModelID         string                 `json:"model_id,omitempty"`
		OwnerUserID     string                 `json:"owner_user_id,omitempty"`
		KeyID           string                 `json:"key_id,omitempty"`
		Permukaan       string                 `json:"permukaan,omitempty"`
		SidikRute       string                 `json:"sidik_rute,omitempty"`
	}{Request: permintaan, MessageContents: contents,
		ModelID: scope.ModelID,
		// Identitas pemilik & kunci masuk sebagai HASH terpisah domain,
		// bukan nilai mentah (Batch C1): isolasi per API key adalah syarat
		// aktivasi, dan kunci lain tidak pernah menerima entri kunci ini.
		OwnerUserID: sidikIdentitas(scope.OwnerUserID), KeyID: sidikIdentitas(scope.KeyID),
		Permukaan: scope.Permukaan, SidikRute: scope.SidikRute})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// skripAmbilBerCap membaca entri dan generasi berjalan dalam SATU
// langkah atomik: keduanya harus berasal dari momen yang sama, kalau
// tidak request bisa memasangkan entri pra-siraman dengan generasi
// pasca-siraman (atau sebaliknya) dan invariant generasi bocor lewat
// jendela di antara dua perintah terpisah.
var skripAmbilBerCap = redis.NewScript(`
local gen = redis.call('GET', KEYS[2])
local val = redis.call('GET', KEYS[1])
return {val, gen}
`)

// GetBerCap mencari entri berdasarkan kunci DAN melaporkan generasi
// yang berlaku pada momen pembacaan itu. Pemanggil yang berniat
// menyimpan hasil eksekusi wajib meneruskan generasi ini ke SetBerCap:
// itulah cap yang membuktikan kunci dihitung sebelum mutasi konteks
// apa pun yang menaikkan generasi di tengah eksekusi (B-473).
//
// Entri yang cap generasinya tidak sama dengan generasi berjalan
// dilaporkan sebagai MISS — bukan HIT — walau kunci Redis-nya masih
// ada: isinya lahir dari konteks jawaban yang sudah tidak berlaku.
func (e *Engine) GetBerCap(ctx context.Context, key string) (*Entry, int64, bool, error) {
	if !e.IsEnabled() || key == "" {
		return nil, 0, false, nil
	}

	rKey := cache.Key(cache.NamespaceResponseCache, "entry", key)
	hasil, err := skripAmbilBerCap.Run(ctx, e.redis.Client(),
		[]string{rKey, e.kunciGenerasi()}).Result()
	if err != nil {
		e.logger.WarnContext(ctx, "gagal membaca response cache", "key", key, "error", err)
		return nil, 0, false, err
	}
	sepasang, ok := hasil.([]interface{})
	if !ok || len(sepasang) != 2 {
		return nil, 0, false, fmt.Errorf("hasil skrip baca response cache tidak berbentuk pasangan")
	}
	var generasi int64
	switch mentah := sepasang[1].(type) {
	case int64:
		generasi = mentah
	case string:
		// Nilai counter di Redis adalah teks angka; Lua meneruskannya
		// apa adanya sebagai bulk string.
		n, err := strconv.ParseInt(mentah, 10, 64)
		if err != nil {
			return nil, 0, false, fmt.Errorf("generasi response cache tidak terbaca: %w", err)
		}
		generasi = n
	}
	if sepasang[0] == nil {
		e.recordStat(ctx, "misses")
		return nil, generasi, false, nil
	}
	val, ok := sepasang[0].(string)
	if !ok {
		return nil, generasi, false, fmt.Errorf("isi response cache tidak berbentuk teks")
	}

	var entry Entry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil, generasi, false, err
	}
	if entry.Generasi != generasi {
		// Entri dari generasi lama (atau ditulis sebelum mekanisme
		// generasi ada, sementara generasi sudah bergerak): jangan
		// disajikan. Ia kedaluwarsa oleh TTL dengan sendirinya.
		e.recordStat(ctx, "misses")
		return nil, generasi, false, nil
	}

	e.recordStat(ctx, "hits")
	return &entry, generasi, true, nil
}

// Get mencari respons yang pernah disimpan berdasarkan kunci.
// Penegakan generasinya sama dengan GetBerCap; pemanggil yang akan
// menyimpan hasil eksekusi sesudah pembacaan ini harus memakai
// GetBerCap + SetBerCap supaya cap generasinya terbawa.
func (e *Engine) Get(ctx context.Context, key string) (*Entry, bool, error) {
	entry, _, hit, err := e.GetBerCap(ctx, key)
	return entry, hit, err
}

// skripSetEntri menulis entri sekaligus memelihara counter ringkas
// meta:entry_count secara atomik: counter hanya naik bila kuncinya BARU,
// sehingga penulisan ulang entri yang sama tidak menghitung ganda dan
// jalur cepat Stats() benar-benar hidup (audit3 V-GW-07 — sebelumnya
// counter ini tidak pernah ditulis oleh siapa pun sehingga Stats()
// selalu jatuh ke SCAN).
var skripSetEntri = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  redis.call('INCR', KEYS[2])
end
redis.call('PSETEX', KEYS[1], ARGV[2], ARGV[1])
return 1
`)

// Set menyimpan respons inferensi ke Redis dengan TTL aktif.
//
// Entri dicap dengan generasi yang berjalan pada saat penulisan, jadi
// ia hanya tersaji selama generasi itu belum bergerak. Pemanggil yang
// menghitung kunci SEBELUM eksekusi panjang (ketiga permukaan gateway)
// tidak boleh memakai Set ini — cap "saat penulisan" akan selalu cocok
// dan balapan B-473 tetap menang; mereka wajib memakai SetBerCap dengan
// generasi dari GetBerCap.
func (e *Engine) Set(ctx context.Context, key string, entry *Entry) error {
	if !e.IsEnabled() || key == "" || entry == nil {
		return nil
	}

	generasi, err := e.GenerasiSaatIni(ctx)
	if err != nil {
		return err
	}
	entry.Generasi = generasi

	rKey := cache.Key(cache.NamespaceResponseCache, "entry", key)
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("serialize cache entry: %w", err)
	}
	if len(data) > MaxEntryBytes {
		// Entri raksasa tidak disimpan: Redis bukan tempat menampung
		// respons tanpa batas, dan request berikutnya cukup menjadi MISS.
		e.logger.DebugContext(ctx, "entri response cache melewati batas ukuran, tidak disimpan",
			"key", key, "ukuran", len(data), "batas", MaxEntryBytes)
		return nil
	}

	entryCountKey := cache.Key(cache.NamespaceResponseCache, "meta", "entry_count")
	ttl := e.TTL()
	if err := skripSetEntri.Run(ctx, e.redis.Client(), []string{rKey, entryCountKey},
		data, ttl.Milliseconds()).Err(); err != nil {
		e.logger.WarnContext(ctx, "gagal menulis response cache", "key", key, "error", err)
		return err
	}
	return nil
}

// skripSetEntriBerCap adalah skripSetEntri plus gerbang generasi:
// penulisan hanya terjadi bila generasi berjalan masih sama dengan
// cap yang dibawa pemanggil, diperiksa dan ditulis dalam SATU langkah
// atomik — pemeriksaan terpisah di sisi Go akan selalu kalah oleh
// Flush yang menyela di antara periksa dan tulis.
var skripSetEntriBerCap = redis.NewScript(`
local gen = tonumber(redis.call('GET', KEYS[3]) or '0')
if gen ~= tonumber(ARGV[3]) then
  return 0
end
if redis.call('EXISTS', KEYS[1]) == 0 then
  redis.call('INCR', KEYS[2])
end
redis.call('PSETEX', KEYS[1], ARGV[2], ARGV[1])
return 1
`)

// SetBerCap menyimpan entri HANYA bila generasi cache belum bergerak
// sejak cap generasi diambil (lewat GetBerCap pada saat kunci
// dihitung). Return false berarti penulisan DITOLAK karena generasi
// sudah bergeser — jawaban yang hendak ditanam lahir dari konteks
// yang sudah tidak berlaku (B-473) dan membuangnya adalah perilaku
// yang benar, bukan kegagalan. Error hanya untuk kegagalan Redis /
// serialisasi yang sesungguhnya.
func (e *Engine) SetBerCap(ctx context.Context, key string, entry *Entry, generasi int64) (bool, error) {
	if !e.IsEnabled() || key == "" || entry == nil {
		return false, nil
	}

	entry.Generasi = generasi
	rKey := cache.Key(cache.NamespaceResponseCache, "entry", key)
	data, err := json.Marshal(entry)
	if err != nil {
		return false, fmt.Errorf("serialize cache entry: %w", err)
	}
	if len(data) > MaxEntryBytes {
		e.logger.DebugContext(ctx, "entri response cache melewati batas ukuran, tidak disimpan",
			"key", key, "ukuran", len(data), "batas", MaxEntryBytes)
		return false, nil
	}

	entryCountKey := cache.Key(cache.NamespaceResponseCache, "meta", "entry_count")
	ttl := e.TTL()
	tertulis, err := skripSetEntriBerCap.Run(ctx, e.redis.Client(),
		[]string{rKey, entryCountKey, e.kunciGenerasi()},
		data, ttl.Milliseconds(), generasi).Int64()
	if err != nil {
		e.logger.WarnContext(ctx, "gagal menulis response cache", "key", key, "error", err)
		return false, err
	}
	if tertulis == 0 {
		e.logger.DebugContext(ctx, "penanaman entri response cache ditolak: generasi sudah bergeser sejak kunci dihitung",
			"key", key, "generasi_cap", generasi)
	}
	return tertulis == 1, nil
}

// Flush membersihkan seluruh entri cache respons di Redis tanpa menyentuh namespace lain.
//
// Flush menaikkan generasi SEBELUM menghapus apa pun (B-473): request
// in-flight yang kuncinya dihitung sebelum flush tidak bisa menanam
// jawabannya kembali sesudah penghapusan selesai, karena SetBerCap-nya
// membawa cap generasi lama. Tanpa kenaikan ini siraman hanya menang
// melawan entri yang sudah tertanam, bukan yang sedang dalam
// perjalanan — persis celah dimensi waktu putaran bersih #7.
// Kunci generasi sendiri TIDAK ikut dihapus: ia adalah memori bahwa
// siraman pernah terjadi.
func (e *Engine) Flush(ctx context.Context) (int64, error) {
	if e.redis == nil {
		return 0, nil
	}

	if _, err := e.NaikkanGenerasi(ctx); err != nil {
		return 0, err
	}

	pattern := cache.Key(cache.NamespaceResponseCache, "entry") + ":*"
	entryCountKey := cache.Key(cache.NamespaceResponseCache, "meta", "entry_count")
	client := e.redis.Client()

	var deletedCount int64
	var cursor uint64

	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return deletedCount, fmt.Errorf("scan response cache: %w", err)
		}

		if len(keys) > 0 {
			n, err := client.Del(ctx, keys...).Result()
			if err != nil {
				return deletedCount, fmt.Errorf("del response cache: %w", err)
			}
			deletedCount += n
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	// Reset counter ringkas entry_count setelah flush berhasil, beserta
	// counter hits/misses: Flush adalah titik awal statistik yang baru,
	// bukan hanya pengosongan entri (audit3 V-GW-07 — sebelumnya hits dan
	// misses tetap kumulatif sesudah flush sehingga angka "sejak flush"
	// yang dibayangkan operator tidak pernah ada).
	hitsKey := cache.Key(cache.NamespaceResponseCache, "stats", "hits")
	missesKey := cache.Key(cache.NamespaceResponseCache, "stats", "misses")
	_ = client.Del(ctx, entryCountKey, hitsKey, missesKey)

	return deletedCount, nil
}

// Stats menghitung metrik statistik penggunaan cache respons.
func (e *Engine) Stats(ctx context.Context) (Stats, error) {
	s := Stats{
		Enabled:    e.IsEnabled(),
		TTLSeconds: int64(e.TTL().Seconds()),
	}
	if e.redis == nil {
		return s, nil
	}

	client := e.redis.Client()
	hitsKey := cache.Key(cache.NamespaceResponseCache, "stats", "hits")
	missesKey := cache.Key(cache.NamespaceResponseCache, "stats", "misses")
	entryCountKey := cache.Key(cache.NamespaceResponseCache, "meta", "entry_count")

	// Pipelining pembacaan hits, misses, dan counter ringkas entry_count dalam 1 round-trip
	pipe := client.Pipeline()
	hitsCmd := pipe.Get(ctx, hitsKey)
	missesCmd := pipe.Get(ctx, missesKey)
	entryCountCmd := pipe.Get(ctx, entryCountKey)
	_, _ = pipe.Exec(ctx)

	hits, err := hitsCmd.Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		e.logger.WarnContext(ctx, "gagal membaca counter hits response cache", "error", err)
	}
	misses, err := missesCmd.Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		e.logger.WarnContext(ctx, "gagal membaca counter misses response cache", "error", err)
	}
	s.Hits = hits
	s.Misses = misses

	// Bila counter ringkas meta:entry_count tersedia, langsung gunakan (O(1)) tanpa SCAN O(N).
	if entryCount, err := entryCountCmd.Int64(); err == nil && entryCount >= 0 {
		s.TotalEntries = entryCount
		return s, nil
	}

	// Fallback ke pipeline SCAN teroptimasi (batch 500) bila counter ringkas belum ada.
	pattern := cache.Key(cache.NamespaceResponseCache, "entry") + ":*"
	var count int64
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			// AUDIT FIX: log kegagalan SCAN; TotalEntries bisa jadi under-count.
			e.logger.WarnContext(ctx, "gagal scan kunci entry response cache", "error", err)
			break
		}
		count += int64(len(keys))
		cursor = next
		if cursor == 0 {
			break
		}
	}
	s.TotalEntries = count

	return s, nil
}

// RekonsiliasiJumlahEntri menghitung ulang jumlah entri yang BENAR-BENAR
// ada di Redis dan mengoreksi counter ringkas meta:entry_count ke angka itu.
//
// Set memelihara counter secara atomik pada penulisan dan Flush
// meresetnya, tetapi entri yang kedaluwarsa alami oleh TTL tidak pernah
// menguranginya — Redis membuang kunci itu tanpa memberi tahu siapa pun —
// sehingga sesudah banyak entri kedaluwarsa total_entries bisa terbaca
// lebih besar dari isi Redis sebenarnya (sisa audit3 B-023). Satu-satunya
// cara membuatnya akurat lagi adalah menghitung ulang dari kuncinya
// sendiri secara berkala: SCAN berpola yang sama seperti jalur fallback
// Stats(), dalam batch terbatas (500), lalu counter DITIMPA hasilnya.
//
// Perlombaan dengan Set yang berjalan bersamaan diterima apa adanya:
// koreksi berikutnya (atau Flush) memperbaikinya lagi. Yang dijanjikan
// fungsi ini bukan ketepatan di setiap milidetik, melainkan statistik
// yang tidak berbohong tanpa batas waktu — penyimpangan maksimalnya satu
// interval rekonsiliasi. Dipanggil berkala dari worker di cmd.
func (e *Engine) RekonsiliasiJumlahEntri(ctx context.Context) (int64, error) {
	if e.redis == nil {
		return 0, nil
	}

	client := e.redis.Client()
	pattern := cache.Key(cache.NamespaceResponseCache, "entry") + ":*"

	var count int64
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return 0, fmt.Errorf("scan rekonsiliasi response cache: %w", err)
		}
		count += int64(len(keys))
		cursor = next
		if cursor == 0 {
			break
		}
	}

	entryCountKey := cache.Key(cache.NamespaceResponseCache, "meta", "entry_count")
	if err := client.Set(ctx, entryCountKey, count, 0).Err(); err != nil {
		return count, fmt.Errorf("menulis koreksi entry_count response cache: %w", err)
	}
	return count, nil
}

// skripCatatStatistik menaikkan counter dan memasang TTL 30 hari HANYA
// saat counter lahir (nilai menjadi 1). Versi pipeline sebelumnya memasang
// ulang Expire pada setiap kenaikan, jadi jendela 30 hari bergeser terus
// selama cache aktif dipakai dan "counter 30 hari" tidak pernah berarti
// 30 hari (audit3 V-GW-07).
var skripCatatStatistik = redis.NewScript(`
local v = redis.call('INCR', KEYS[1])
if v == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return v
`)

func (e *Engine) recordStat(ctx context.Context, metric string) {
	if e.redis == nil {
		return
	}
	key := cache.Key(cache.NamespaceResponseCache, "stats", metric)

	if err := skripCatatStatistik.Run(ctx, e.redis.Client(), []string{key},
		int64(DefaultStatsTTL.Seconds())).Err(); err != nil {
		// AUDIT FIX: gagal Incr tidak lagi senyap; hit/miss statistic bisa meleset
		// tanpa jejak. Warning sekali per kejadian cukup — jangan banjiri log.
		e.logger.WarnContext(ctx, "gagal menaikkan counter response cache", "metric", metric, "error", err)
	}
}
