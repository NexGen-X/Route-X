// Berkas ini menghubungkan jalur permintaan /v1 dengan pencatat pemakaian: mengumpulkan
// fakta satu permintaan sambil permintaan itu berjalan, lalu menyerahkannya satu kali di
// akhir.
//
// # Kenapa fakta dikumpulkan di satu objek, bukan dicatat di setiap titik keluar
//
// Handler di paket ini punya belasan titik keluar — body cacat, model tak dikenal, batas
// laju, anggaran, penyaring konten, semua kandidat gagal, aliran terputus di tengah — dan
// setiap titik itu berarti satu permintaan yang harus muncul di log. Memanggil pencatat di
// setiap titik berarti satu titik yang lupa memanggilnya adalah satu jenis kegagalan yang
// TIDAK PERNAH muncul di dashboard, dan tidak ada yang menyadarinya karena yang hilang
// adalah baris yang seharusnya ada.
//
// Karena itu pencatatannya lewat defer di satu tempat per handler, dan status HTTP-nya
// tidak dikumpulkan dari titik-titik itu melainkan dibaca dari respons yang BENAR-BENAR
// tertulis. Titik keluar baru ikut tercatat tanpa harus diingat.
//
// # Kenapa envelope error dibaca ulang dari respons
//
// Kode dan pesan error yang dicatat adalah kode dan pesan yang DITERIMA KLIEN, dibaca dari
// body yang tertulis. Dua sebabnya: penyelidikan keluhan selalu dimulai dari kode yang
// dilihat pengguna, dan envelope itu sudah pasti bersih karena ia memang dikirim keluar.
// Menyusun ulang klasifikasi di sisi pencatat berarti dua tempat memutuskan hal yang sama
// lalu bisa berbeda.
package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
	"github.com/NexGen-X/Route-X/internal/usage"
)

// UsageSink menerima catatan hasil satu permintaan. Dipenuhi *usage.Recorder.
//
// Antarmuka, bukan tipe konkret, karena handler harus bisa diuji tanpa database — dan
// karena satu-satunya hal yang dibutuhkan paket ini dari pencatat adalah "terima ini".
type UsageSink interface {
	Record(ctx context.Context, ev usage.Event)
}

// maxTangkapBadan membatasi jumlah byte badan error yang ditahan untuk dibaca ulang.
//
// Envelope error kami paling panjang beberapa ratus byte; batas ini longgar dan ada supaya
// respons error yang tidak terduga besar tidak ikut disalin ke memori pada setiap
// kegagalan.
const maxTangkapBadan = 4 << 10

// pencatatRespons membaca status dan envelope error dari respons yang benar-benar tertulis.
//
// Membungkus, bukan menebak: status yang dilaporkan handler dan status yang benar-benar
// terkirim bisa berbeda, dan yang harus masuk log adalah yang kedua.
type pencatatRespons struct {
	http.ResponseWriter

	status  int
	bytes   int64
	ditulis bool

	tangkap bool
	badan   []byte
}

// WriteHeader mencatat status pertama yang dikirim.
func (p *pencatatRespons) WriteHeader(status int) {
	// Respons informasional boleh dikirim berulang dan bukan penutup header.
	if status >= 100 && status < 200 {
		p.ResponseWriter.WriteHeader(status)
		return
	}
	if p.ditulis {
		return
	}
	p.status, p.ditulis = status, true
	// Badan hanya ditangkap untuk kegagalan berbentuk JSON, yaitu envelope error kami
	// sendiri. Respons sukses tidak ditangkap sama sekali: isinya jawaban model, dan
	// menyalinnya ke memori pada setiap permintaan adalah biaya tanpa guna sekaligus data
	// pengguna yang beredar lebih jauh daripada perlu.
	p.tangkap = status >= 400 && strings.HasPrefix(p.Header().Get("Content-Type"), "application/json")
	p.ResponseWriter.WriteHeader(status)
}

// Write meneruskan penulisan sambil menghitung byte dan menangkap envelope error.
func (p *pencatatRespons) Write(b []byte) (int, error) {
	if !p.ditulis {
		// net/http mengirim 200 sendiri bila handler menulis body tanpa WriteHeader.
		p.status, p.ditulis = http.StatusOK, true
	}
	if p.tangkap && len(p.badan) < maxTangkapBadan {
		p.badan = append(p.badan, b[:min(len(b), maxTangkapBadan-len(p.badan))]...)
	}
	n, err := p.ResponseWriter.Write(b)
	p.bytes += int64(n)
	return n, err
}

// Unwrap membuat http.ResponseController menemukan Flush, SetReadDeadline, dan
// SetWriteDeadline di writer di bawah.
//
// WAJIB ada: tanpa ini httpx.PrepareSSE tidak bisa melepas tenggat baca dan tulis, dan
// setiap respons streaming mati pada detik READ_TIMEOUT.
func (p *pencatatRespons) Unwrap() http.ResponseWriter { return p.ResponseWriter }

// envelope membaca kode dan pesan dari badan error yang tertangkap.
func (p *pencatatRespons) envelope() (kode, pesan string) {
	if len(p.badan) == 0 {
		return "", ""
	}
	var out httpx.ErrorResponse
	if err := json.Unmarshal(p.badan, &out); err != nil {
		// Badan terpotong batas penangkapan, atau bukan envelope kami. Tidak ada yang bisa
		// dibaca, dan itu bukan alasan menggagalkan pencatatan.
		return "", ""
	}
	if out.Error.Code != nil {
		kode = *out.Error.Code
	}
	return kode, out.Error.Message
}

// tipeKesalahan memetakan kode envelope menjadi kategori kegagalan yang STABIL.
//
// Kategori inilah yang diagregasi rollup dan dashboard, jadi ejaannya bagian dari kontrak
// data — bukan teks yang bisa diperbaiki sewaktu-waktu. Kode envelope dipakai sebagai kunci
// karena kode itu sendiri sudah stabil (klien mencocokkannya), sementara pesannya tidak.
//
// Kegagalan upstream TIDAK lewat peta ini: kategorinya diambil langsung dari
// providers.Error.Kind, karena satu kode envelope seperti "upstream_error" mewakili
// beberapa kategori sekaligus (network, server, stream_aborted) dan memetakannya balik
// akan kehilangan justru perbedaan yang dipakai untuk mengukur tingkat timeout.
var tipeKesalahan = map[string]string{
	CodeContentFilter:  "content_filter",
	CodePayloadTooBig:  "payload_too_large",
	CodeBudgetExceeded: "budget",
	CodeRateLimited:    "rate_limit",

	"model_not_found":         "model_not_found",
	"model_disabled":          "model_disabled",
	"model_not_allowed":       "permission",
	"no_provider_for_model":   "no_provider",
	"streaming_not_supported": "invalid_request",
	"tools_not_supported":     "invalid_request",
	"context_length_exceeded": "invalid_request",

	httpx.CodeInvalidRequest:   "invalid_request",
	httpx.CodeInvalidAPIKey:    "auth",
	httpx.CodeNotFound:         "not_found",
	httpx.CodeMethodNotAllowed: "invalid_request",
	httpx.CodeInternalError:    "internal",
}

// tipeKesalahanLain adalah kategori untuk kegagalan yang kodenya belum ada di peta.
//
// Bukan string kosong: kolom error_type yang NULL pada baris berstatus 4xx atau 5xx tidak
// bisa dibedakan dari baris yang berhasil, dan "gateway" setidaknya menyatakan bahwa
// penolakannya datang dari sini, bukan dari upstream. Status di baris yang sama yang
// menjelaskan sisanya.
const tipeKesalahanLain = "gateway"

// peristiwaKebijakan memetakan kode envelope ke jenis peristiwa timeline.
//
// Hanya penolakan kebijakan yang punya padanan di request_events. Penolakan lain sudah
// sepenuhnya terjelaskan oleh baris requests-nya sendiri, dan menambahkan peristiwa timeline
// untuknya hanya menggandakan informasi yang sama.
var peristiwaKebijakan = map[string]string{
	CodeContentFilter:  traffic.EventContentBlocked,
	CodeBudgetExceeded: traffic.EventBudgetExceeded,
	CodeRateLimited:    traffic.EventRateLimited,
}

// jejak mengumpulkan fakta satu permintaan /v1 sambil permintaan itu berjalan.
//
// Diisi bertahap karena faktanya memang baru ada bertahap: model setelah alias
// diselesaikan, provider setelah rute dipilih, token setelah jawaban selesai. Yang TIDAK
// dikumpulkan bertahap adalah status HTTP — itu dibaca dari respons yang benar-benar
// tertulis, sehingga titik keluar yang baru tidak perlu diingat.
type jejak struct {
	sink   UsageSink
	logger *slog.Logger
	w      *pencatatRespons

	mulai    time.Time
	method   string
	endpoint string
	ip       netip.Addr
	ua       string

	requested string
	modelID   string
	modelName string
	stream    bool

	principal *apikey.Principal
	targets   []policy.Target

	strategi string
	rule     *routingRingkas
	kandidat []*upstream.RouteCandidate

	attempts []Attempt
	menang   *upstream.RouteCandidate

	pemakaian providers.Usage
	ttft      time.Duration
	hulu      time.Duration

	// kind adalah kategori kegagalan yang sudah diketahui pasti, yaitu kegagalan upstream
	// yang klasifikasinya datang dari providers.Error. Kosong berarti kategorinya
	// disimpulkan dari envelope error yang tertulis.
	kind string
}

// routingRingkas adalah bagian aturan routing yang ikut dicatat.
type routingRingkas struct {
	id   string
	name string
}

// mulaiJejak membuat jejak baru dan writer yang mencatat responsnya.
//
// Pemanggil WAJIB memakai writer yang dikembalikan, bukan writer aslinya: writer itulah
// yang tahu status dan envelope apa yang benar-benar terkirim.
//
// Waktu mulainya diambil di sini, yaitu saat handler dijalankan — beberapa mikrodetik
// setelah server menerima permintaan. Selisih itu tidak diukur karena middleware log yang
// memegang waktu masuk tidak membaginya, dan mikrodetik tidak mengubah satu pun angka pada
// skala latensi panggilan model.
func (h *Handlers) mulaiJejak(w http.ResponseWriter, r *http.Request) (*jejak, http.ResponseWriter) {
	pw := &pencatatRespons{ResponseWriter: w}
	ip, _ := httpx.ClientIPFrom(r.Context())
	j := &jejak{
		sink:     h.usage,
		logger:   h.logger,
		w:        pw,
		mulai:    time.Now(),
		method:   r.Method,
		endpoint: r.URL.Path,
		ip:       ip,
		ua:       r.UserAgent(),
	}
	return j, pw
}

// pasangDiminta mencatat nama model yang dikirim klien.
//
// Dipasang sebelum resolusi, supaya permintaan ke model yang tidak dikenal pun tercatat
// dengan nama yang diminta — dan justru baris itulah yang dicari orang ketika kliennya
// menerima 404.
func (j *jejak) pasangDiminta(nama string) {
	if j != nil {
		j.requested = nama
	}
}

// pasangModel mencatat model kanonik hasil resolusi.
func (j *jejak) pasangModel(m *upstream.Model) {
	if j == nil || m == nil {
		return
	}
	j.modelID, j.modelName = m.ID, m.ModelID
}

// pasangKebijakan mencatat pemilik kuota dan cakupan kebijakannya.
//
// targets-nya harus daftar yang SAMA dengan yang dipakai menggerbangi permintaan, karena
// daftar itu juga yang menentukan anggaran mana yang pemakaiannya dinaikkan. Daftar yang
// disusun dua kali adalah daftar yang bisa berbeda.
func (j *jejak) pasangKebijakan(p *apikey.Principal, targets []policy.Target) {
	if j == nil {
		return
	}
	j.principal, j.targets = p, targets
}

// pasangKeputusan mencatat hasil routing: strategi, aturan, dan kandidat berurut.
//
// Strategi bawaan dicatat eksplisit ketika tidak ada aturan yang cocok, bukan dibiarkan
// kosong. Kolom yang kosong akan dibaca sebagai "tidak ada strategi", padahal permintaan
// itu memang dirutekan — dengan strategi prioritas — dan itu jawaban yang dicari operator
// saat bertanya kenapa permintaannya tidak mengikuti aturan yang baru ia buat.
func (j *jejak) pasangKeputusan(d routerDecision, stream bool) {
	if j == nil {
		return
	}
	j.strategi, j.kandidat, j.stream = string(router.StrategyPriority), d.Candidates, stream
	if d.Rule != nil {
		if d.Rule.Strategy.Valid() {
			j.strategi = string(d.Rule.Strategy)
		}
		j.rule = &routingRingkas{id: d.Rule.ID, name: d.Rule.Name}
	}
}

// routerDecision adalah alias supaya tanda tangan di atas tetap terbaca sebaris.
type routerDecision = router.Decision

// pasangPercobaan mencatat jejak percobaan dan kandidat yang menang.
func (j *jejak) pasangPercobaan(att []Attempt, menang *upstream.RouteCandidate) {
	if j == nil {
		return
	}
	j.attempts, j.menang = att, menang
	j.hulu = 0
	for _, a := range att {
		// Kandidat yang dilewati tidak pernah dihubungi, jadi ia tidak menghabiskan waktu
		// hulu sama sekali — memasukkan durasinya akan menghitung waktu yang tidak ada.
		if a.SkipReason == "" {
			j.hulu += a.Duration
		}
	}
}

// tambahHulu menambah waktu yang dihabiskan menunggu upstream di luar percobaan.
//
// Diperlukan untuk streaming: durasi percobaan hanya mencakup PEMBUKAAN aliran, sementara
// sisa waktu upstream dihabiskan membaca potongan. Tanpa penambahan ini, waktu hulu satu
// respons streaming tercatat beberapa milidetik untuk aliran yang berjalan setengah menit.
func (j *jejak) tambahHulu(d time.Duration) {
	if j != nil && d > 0 {
		j.hulu += d
	}
}

// pasangPemakaian mencatat token yang dilaporkan upstream.
func (j *jejak) pasangPemakaian(u providers.Usage) {
	if j != nil {
		j.pemakaian = u
	}
}

// tandaiTTFT mencatat waktu potongan konten pertama diteruskan ke klien.
//
// Diukur DI SINI, bukan di adapter, dan bukan pada potongan pertama apa pun. Dua alasannya
// terpisah:
//
//   - Di lapisan ini, karena yang diukur adalah waktu sampai pengguna melihat sesuatu, dan
//     itu baru terjadi setelah byte-nya benar-benar melewati penulisan dan flush. Potongan
//     yang sudah diterima adapter tetapi tertahan di buffer belum dilihat siapa pun.
//   - Pada potongan KONTEN, karena aliran berdialek OpenAI dibuka dengan potongan yang
//     hanya membawa peran ("delta": {"role": "assistant"}). Potongan itu datang sebelum
//     token pertama dihasilkan, dan memakainya akan melaporkan TTFT yang jauh lebih cepat
//     daripada yang dialami pengguna — persis arah kesalahan yang paling menyenangkan untuk
//     dilihat dan paling menyesatkan untuk dipercaya.
//
// Hanya pemanggilan PERTAMA yang berlaku.
func (j *jejak) tandaiTTFT() {
	if j != nil && j.ttft == 0 {
		j.ttft = time.Since(j.mulai)
	}
}

// pasangKegagalanUpstream mencatat kategori kegagalan dari klasifikasi upstream.
//
// Kategori dari sini menang atas kategori yang disimpulkan dari envelope: satu kode
// envelope seperti "upstream_error" mewakili beberapa kategori sekaligus, dan yang dipakai
// mengukur tingkat timeout adalah kategori aslinya.
func (j *jejak) pasangKegagalanUpstream(e *providers.Error) {
	if j == nil || e == nil || e.Kind == "" {
		return
	}
	j.kind = string(e.Kind)
}

// selesai menyerahkan catatan permintaan ini ke pencatat pemakaian.
//
// Dipanggil lewat defer, sekali per permintaan, apa pun jalan keluarnya. Tidak pernah
// mengembalikan error dan tidak pernah memblokir: pada titik ini jawaban sudah terkirim,
// dan tidak ada yang bisa diperbaiki dengan menggagalkan apa pun.
func (j *jejak) selesai(ctx context.Context) {
	if j == nil || j.sink == nil {
		return
	}

	status := j.w.status
	if !j.w.ditulis {
		// Tidak satu byte pun tertulis. Satu-satunya cara ini terjadi adalah klien menutup
		// koneksi sebelum jawaban dimulai; 499 dari konvensi nginx menyatakan itu tanpa
		// menaikkan tingkat kesalahan gateway atas hal yang bukan kesalahannya.
		status = statusClientClosed
	}
	kode, pesan := j.w.envelope()

	ev := usage.Event{
		RequestID: observability.RequestIDFrom(ctx),
		Method:    j.method,
		Endpoint:  j.endpoint,

		APIKeyID:   j.principal.ID(),
		APIKeyName: namaKey(j.principal),
		UserID:     j.principal.OwnerUserID(),

		RequestedModel: j.requested,
		ModelID:        j.modelID,
		ModelName:      j.modelName,

		StatusCode:   status,
		ErrorType:    j.tipeKesalahan(status, kode),
		ErrorCode:    kode,
		ErrorMessage: pesan,

		Stream: j.stream,

		InputTokens:       j.pemakaian.InputTokens,
		CachedInputTokens: j.pemakaian.CachedInputTokens,
		OutputTokens:      j.pemakaian.OutputTokens,
		ReasoningTokens:   j.pemakaian.ReasoningTokens,
		TotalTokens:       j.pemakaian.TotalTokens,

		Latency:         time.Since(j.mulai),
		TTFT:            j.ttft,
		UpstreamLatency: j.hulu,

		RoutingStrategy: j.strategi,
		Routing:         j.routing(),
		Attempts:        percobaanPemakaian(j.attempts),
		Notes:           j.catatan(kode, pesan),
		Targets:         j.targets,

		ClientIP:  j.ip,
		UserAgent: j.ua,
	}
	ev.RetryCount, ev.FailoverCount = hitungUlangDanAlih(j.attempts)

	if j.menang != nil {
		ev.ProviderID = j.menang.ProviderID
		ev.ProviderName = j.menang.ProviderName
		ev.ProviderModelID = j.menang.ProviderModelID
		ev.UpstreamModel = j.menang.UpstreamModelName
	}

	j.sink.Record(ctx, ev)
}

// tipeKesalahan menentukan kategori kegagalan yang dicatat.
//
// Urutannya: klasifikasi upstream bila ada, lalu peta kode envelope, lalu kategori umum.
// Permintaan yang berhasil TIDAK punya kategori — kecuali satu keadaan: aliran yang
// terputus setelah sebagian terkirim. Klien menerima 200 pada aliran itu dan status yang
// dicatat memang 200, karena itu yang benar-benar terjadi; kategorinyalah yang menjadikan
// kegagalan itu bisa dihitung. Karena itu agregasi memakai "error_type is not null", bukan
// hanya "status >= 400".
func (j *jejak) tipeKesalahan(status int, kode string) string {
	if j.kind != "" {
		return j.kind
	}
	if status < 400 {
		return ""
	}
	if t, ok := tipeKesalahan[kode]; ok {
		return t
	}
	if status == statusClientClosed {
		return string(providers.ErrKindCanceled)
	}
	return tipeKesalahanLain
}

// routing menyusun ringkasan keputusan routing untuk kolom routing_decision.
func (j *jejak) routing() usage.Routing {
	rt := usage.Routing{Strategy: j.strategi}
	if j.rule != nil {
		rt.RuleID, rt.RuleName = j.rule.id, j.rule.name
	}
	if len(j.kandidat) == 0 {
		return rt
	}
	rt.Candidates = make([]usage.RoutingCandidate, 0, len(j.kandidat))
	for i, c := range j.kandidat {
		rt.Candidates = append(rt.Candidates, usage.RoutingCandidate{
			Position:        i + 1,
			ProviderID:      c.ProviderID,
			ProviderName:    c.ProviderName,
			ProviderModelID: c.ProviderModelID,
			UpstreamModel:   c.UpstreamModelName,
			Priority:        c.Priority,
			Weight:          c.Weight,
			Chosen:          j.menang != nil && c.ProviderModelID == j.menang.ProviderModelID,
		})
	}
	return rt
}

// catatan menyusun peristiwa timeline yang bukan percobaan upstream.
func (j *jejak) catatan(kode, pesan string) []usage.Note {
	kind, ok := peristiwaKebijakan[kode]
	if !ok {
		return nil
	}
	return []usage.Note{{Kind: kind, Message: pesan}}
}

// percobaanPemakaian menerjemahkan jejak percobaan ke bentuk yang dipakai pencatat.
//
// Penerjemahan, bukan pemakaian tipe yang sama, supaya paket usage tidak perlu bergantung
// pada paket ini — arah ketergantungannya sebaliknya.
func percobaanPemakaian(att []Attempt) []usage.Attempt {
	if len(att) == 0 {
		return nil
	}
	out := make([]usage.Attempt, 0, len(att))
	for _, a := range att {
		u := usage.Attempt{
			ProviderID:    a.ProviderID,
			ProviderName:  a.ProviderName,
			UpstreamModel: a.UpstreamModel,
			Nth:           a.Nth,
			Duration:      a.Duration,
			BreakerState:  string(a.BreakerState),
			SkipReason:    a.SkipReason,
		}
		if a.Err != nil {
			u.ErrorKind = string(a.Err.Kind)
			// Message dari providers.Error SUDAH disaring — itu kontrak tipe itu — jadi ia
			// aman masuk database dan aman dibaca operator.
			u.ErrorMessage = a.Err.Message
			u.StatusCode = a.Err.StatusCode
		}
		out = append(out, u)
	}
	return out
}

// hitungUlangDanAlih menghitung jumlah percobaan ulang dan perpindahan kandidat.
//
// Pembedanya ProviderModelID, bukan ProviderID: pemetaan model-provider adalah satuan yang
// benar-benar dicoba, dan itulah yang berpindah saat failover.
//
// Kandidat yang DILEWATI ikut dihitung sebagai perpindahan. Itu disengaja: pemutus arus
// yang terbuka membuat permintaan pindah ke provider berikutnya, dan bagi pembaca dashboard
// itu perpindahan yang sama nyatanya dengan perpindahan karena kegagalan.
func hitungUlangDanAlih(att []Attempt) (ulang, alih int) {
	var sebelum string
	for i, a := range att {
		switch {
		case i == 0:
		case a.ProviderModelID != sebelum:
			alih++
		default:
			ulang++
		}
		sebelum = a.ProviderModelID
	}
	return ulang, alih
}

// namaKey mengambil nama API key untuk cuplikan di log request.
//
// Nama, bukan nilai key: keys.Key tidak pernah memuat nilainya, dan yang berguna di
// dashboard adalah nama yang diberi operator.
func namaKey(p *apikey.Principal) string {
	if p == nil || p.Key == nil {
		return ""
	}
	return p.Key.Name
}
