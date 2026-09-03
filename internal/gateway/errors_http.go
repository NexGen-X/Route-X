package gateway

import (
	"net/http"
	"strconv"
	"time"

	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Pemetaan kegagalan upstream ke jawaban HTTP.
//
// Satu keputusan menentukan seluruh tabel di bawah: status yang dikirim harus
// menggambarkan SIAPA yang bisa memperbaikinya, bukan status apa yang dikirim upstream.
// Gateway ini duduk di antara klien dan provider, dan status upstream diteruskan apa adanya
// akan menyesatkan pada kasus yang justru paling sering:
//
//   - Upstream membalas 401 karena kredensial OPERATOR ditolak. Meneruskannya sebagai 401
//     memberi tahu klien bahwa API key MEREKA yang salah — dan klien yang patuh akan
//     berhenti mencoba, memutar kunci yang sehat, lalu membuka tiket. Yang benar 502:
//     kegagalan ada di hulu, dan klien tidak punya apa pun untuk diperbaiki.
//   - Upstream membalas 429 karena kuota berbayar habis. Itu bukan pembatasan laju yang
//     mereda dengan menunggu, jadi mengirim 429 mengajak klien mengulang selamanya.
//
// Yang diteruskan hanya kegagalan yang memang milik klien: permintaan cacat, model yang
// tidak ada, dan penolakan kebijakan konten.

// httpUntuk memetakan satu kegagalan upstream menjadi status, tipe, dan kode envelope.
//
// Kode yang dikembalikan mengikuti kosakata error OpenAI di mana ada padanannya, karena
// klien yang sudah ada mencocokkannya. Di mana tidak ada padanan, kodenya menyebut hulu
// secara eksplisit supaya tidak tertukar dengan kegagalan milik klien.
func httpUntuk(e *providers.Error) (status int, errType, code, message string) {
	if e == nil {
		return http.StatusInternalServerError, httpx.ErrTypeAPI, httpx.CodeInternalError,
			"terjadi kesalahan internal, coba lagi nanti"
	}

	switch e.Kind {
	// --- Kegagalan yang memang milik klien: diteruskan apa adanya. ---
	case providers.ErrKindInvalidRequest:
		return http.StatusBadRequest, httpx.ErrTypeInvalidRequest, "invalid_request_error", e.Message
	case providers.ErrKindModelNotFound:
		return http.StatusNotFound, httpx.ErrTypeInvalidRequest, "model_not_found", e.Message
	case providers.ErrKindContentFilter:
		return http.StatusBadRequest, httpx.ErrTypeInvalidRequest, "content_filter", e.Message

	// --- Pembatasan laju: satu-satunya kegagalan hulu yang benar-benar mereda dengan
	// menunggu, jadi satu-satunya yang pantas mengajak klien mengulang. ---
	case providers.ErrKindRateLimit:
		return http.StatusTooManyRequests, httpx.ErrTypeRateLimit, "rate_limit_exceeded",
			"seluruh provider untuk model ini sedang membatasi laju, coba lagi beberapa saat lagi"

	// --- Provider penuh: sementara, tetapi bukan soal laju permintaan klien. ---
	case providers.ErrKindOverloaded:
		return http.StatusServiceUnavailable, httpx.ErrTypeOverloaded, "upstream_overloaded",
			"provider untuk model ini sedang penuh, coba lagi beberapa saat lagi"

	case providers.ErrKindTimeout:
		return http.StatusGatewayTimeout, httpx.ErrTypeAPI, "upstream_timeout",
			"provider tidak menjawab sebelum batas waktu"

	// --- Kegagalan yang hanya bisa diperbaiki operator. Pesannya TIDAK menyebut provider
	// mana, karena pada provider BYOK nama itu milik pengguna lain. ---
	case providers.ErrKindAuth:
		return http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_auth_failed",
			"kredensial gateway ke provider ditolak; ini bukan soal API key Anda"
	case providers.ErrKindPermission:
		return http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_permission_denied",
			"gateway tidak berhak memakai model ini di provider yang tersedia"
	case providers.ErrKindQuota:
		return http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_quota_exhausted",
			"kuota gateway pada provider untuk model ini sudah habis"

	case providers.ErrKindNetwork, providers.ErrKindServer, providers.ErrKindStreamAborted:
		return http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_error",
			"provider gagal menjawab permintaan ini"

	// --- Klien sudah pergi. Statusnya tidak akan pernah terkirim; ia ada supaya log dan
	// metrik tidak mencatatnya sebagai kegagalan gateway. 499 mengikuti konvensi nginx,
	// yang sudah dikenal perkakas pemantauan. ---
	case providers.ErrKindCanceled:
		return statusClientClosed, httpx.ErrTypeAPI, "client_closed_request",
			"permintaan dibatalkan sebelum selesai"

	default:
		return http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_error",
			"provider gagal menjawab permintaan ini"
	}
}

// statusClientClosed adalah 499 dari konvensi nginx: klien menutup koneksi sebelum
// respons selesai. Bukan status standar, dan sengaja dipakai — alternatifnya mencatat
// kepergian klien sebagai 500, yang membuat tingkat kesalahan gateway naik karena hal
// yang bukan kesalahannya.
const statusClientClosed = 499

// tulisKegagalan membalas kegagalan upstream dengan envelope error OpenAI.
//
// Hanya boleh dipanggil SEBELUM satu byte pun respons terkirim. Setelah itu status sudah
// terkunci dan satu-satunya jalur pelaporan adalah peristiwa di dalam stream — lihat
// tulisKegagalanSSE.
func tulisKegagalan(w http.ResponseWriter, r *http.Request, e *providers.Error) {
	status, errType, code, message := httpUntuk(e)

	// Klien yang sudah pergi tidak akan membaca apa pun. Status tetap ditulis supaya
	// middleware log mencatat angka yang benar.
	if status == statusClientClosed {
		w.WriteHeader(status)
		return
	}
	if e != nil && e.RetryAfter > 0 && (status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable) {
		// Retry-After hanya dipasang untuk status yang memang berarti "coba lagi nanti".
		// Memasangnya di 502 akan mengajak klien mengulang kegagalan yang tidak mereda.
		w.Header().Set("Retry-After", retryAfterHeader(e.RetryAfter))
	}
	httpx.WriteError(w, r, status, errType, code, message)
}

// retryAfterHeader memformat Retry-After sebagai jumlah detik.
//
// Dibulatkan ke ATAS, dan minimal satu detik. Membulatkan ke bawah menghasilkan
// "Retry-After: 0" untuk jeda di bawah satu detik, dan klien yang mematuhinya akan
// mengulang seketika — yaitu kebalikan dari yang diminta header itu.
func retryAfterHeader(d time.Duration) string {
	detik := int64(d / time.Second)
	if d%time.Second != 0 {
		detik++
	}
	return strconv.FormatInt(max(detik, 1), 10)
}
