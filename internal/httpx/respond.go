package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/NexGen-X/Route-X/internal/observability"
)

// Tipe error mengikuti taksonomi OpenAI. Klien resmi mereka mencocokkan nilai ini
// untuk memutuskan apakah sebuah kegagalan layak diulang, jadi daftarnya tidak boleh
// diperluas dengan istilah karangan sendiri.
const (
	// ErrTypeInvalidRequest: request salah bentuk atau melanggar aturan validasi.
	ErrTypeInvalidRequest = "invalid_request_error"
	// ErrTypeAuthentication: kredensial tidak ada, salah, atau sudah dicabut.
	ErrTypeAuthentication = "authentication_error"
	// ErrTypePermission: kredensial sah tapi tidak berhak atas sumber daya ini.
	ErrTypePermission = "permission_error"
	// ErrTypeRateLimit: kuota atau batas laju terlampaui.
	ErrTypeRateLimit = "rate_limit_error"
	// ErrTypeAPI: kegagalan tak terduga di sisi kami.
	ErrTypeAPI = "api_error"
	// ErrTypeOverloaded: kapasitas penuh sementara, layak dicoba lagi.
	ErrTypeOverloaded = "overloaded_error"
)

// Kode error yang dipakai helper di file ini. Kode adalah bagian yang dibaca mesin,
// jadi nilainya harus stabil walau teks pesan berubah.
const (
	CodeInvalidRequest         = "invalid_request"
	CodeInvalidAPIKey          = "invalid_api_key"
	CodeInsufficientPermission = "insufficient_permissions"
	CodeNotFound               = "not_found"
	CodeMethodNotAllowed       = "method_not_allowed"
	CodeRateLimitExceeded      = "rate_limit_exceeded"
	CodePayloadTooLarge        = "payload_too_large"
	CodeInternalError          = "internal_error"
	CodeServiceUnavailable     = "service_unavailable"
)

// Pesan default. Untuk 5xx pesannya generik dan tidak bisa ditimpa pemanggil: teks
// yang datang dari galat internal (nama tabel, error driver pgx, alamat upstream)
// tidak boleh pernah sampai ke klien. Detail aslinya ditulis ke log lewat
// observability.LoggerFrom(r.Context()) di titik kegagalan.
const (
	msgBadRequest         = "request tidak valid"
	msgUnauthorized       = "kredensial tidak sah atau tidak disertakan"
	msgForbidden          = "akses ke sumber daya ini tidak diizinkan"
	msgNotFound           = "sumber daya tidak ditemukan"
	msgTooManyRequests    = "batas laju terlampaui, coba lagi beberapa saat lagi"
	msgInternalError      = "terjadi kesalahan internal, coba lagi nanti"
	msgServiceUnavailable = "layanan sedang tidak tersedia, coba lagi nanti"
)

const contentTypeJSON = "application/json"

// maxErrorMessageLen memotong pesan yang kepanjangan. Ini higienis, bukan penyaring:
// pemanggil tetap bertanggung jawab tidak memasukkan detail internal sejak awal.
const maxErrorMessageLen = 512

// ErrorDetail adalah isi field "error" pada envelope. Urutan deklarasi menentukan
// urutan field di JSON, dan urutan itu bagian dari kontrak dengan klien OpenAI.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	// Code dan Param bertipe pointer supaya terserialisasi sebagai null saat tidak
	// diisi, sama seperti respons OpenAI. Field-nya selalu ada, hanya nilainya null.
	Code  *string `json:"code"`
	Param *string `json:"param"`
	// RequestID adalah pegangan pelanggan saat menghubungi dukungan: nilainya sama
	// dengan header X-Request-Id dan dengan request_id di log.
	RequestID string `json:"request_id"`
}

// ErrorResponse adalah envelope error lengkap:
//
//	{"error":{"message":"...","type":"invalid_request_error","code":"...","param":null,"request_id":"..."}}
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// JSON menulis v sebagai JSON dengan status yang diberikan.
//
// Marshal dilakukan sebelum satu byte pun terkirim, sehingga kegagalan encoding
// tidak meninggalkan respons setengah jadi dengan status yang sudah terkunci.
func JSON(w http.ResponseWriter, status int, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("menyusun respons JSON: %w", err)
	}
	// Baris baru di akhir membuat keluaran curl terbaca tanpa mengubah isi JSON.
	body = append(body, '\n')

	h := w.Header()
	h.Set("Content-Type", contentTypeJSON)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	// Respons API memuat data milik pelanggan; jangan biarkan perantara menyimpannya.
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("menulis respons JSON: %w", err)
	}
	return nil
}

// WriteError membalas dengan envelope error kompatibel OpenAI, lengkap dengan
// request_id dari context supaya satu keluhan pelanggan bisa langsung dipetakan ke
// baris log yang tepat.
//
// message harus aman dilihat pelanggan. Jangan pernah meneruskan error internal ke
// sini; catat error aslinya ke log, lalu kirim pesan yang sudah dirumuskan.
func WriteError(w http.ResponseWriter, r *http.Request, status int, errType, code, message string) {
	detail := ErrorDetail{
		Message: truncateMessage(message),
		Type:    errType,
	}
	if code != "" {
		detail.Code = &code
	}
	if r != nil {
		detail.RequestID = observability.RequestIDFrom(r.Context())
	}
	// Kegagalan tulis di sini praktis selalu berarti klien sudah menutup koneksi.
	// Tidak ada jalur lain untuk memberi tahu klien, jadi tidak ada yang bisa
	// dilakukan selain melanjutkan.
	_ = JSON(w, status, ErrorResponse{Error: detail})
}

func truncateMessage(message string) string {
	if len(message) <= maxErrorMessageLen {
		return message
	}
	return message[:maxErrorMessageLen] + "..."
}

// BadRequest membalas 400. code menjelaskan pelanggaran secara spesifik, misalnya
// "model_not_supported" atau "missing_required_parameter".
func BadRequest(w http.ResponseWriter, r *http.Request, code, message string) {
	if code == "" {
		code = CodeInvalidRequest
	}
	WriteError(w, r, http.StatusBadRequest, ErrTypeInvalidRequest, code, orDefault(message, msgBadRequest))
}

// Unauthorized membalas 401. Pesannya tidak boleh membedakan "kunci tidak ada" dari
// "kunci salah" secara rinci, agar tidak membantu penebakan kunci.
func Unauthorized(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusUnauthorized, ErrTypeAuthentication, CodeInvalidAPIKey,
		orDefault(message, msgUnauthorized))
}

// Forbidden membalas 403: kredensial dikenali tapi tidak berhak.
func Forbidden(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusForbidden, ErrTypePermission, CodeInsufficientPermission,
		orDefault(message, msgForbidden))
}

// NotFound membalas 404.
func NotFound(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusNotFound, ErrTypeInvalidRequest, CodeNotFound,
		orDefault(message, msgNotFound))
}

// TooManyRequests membalas 429. Bila retryAfter positif, header Retry-After ikut
// dipasang supaya klien resmi OpenAI bisa menunggu dengan benar alih-alih menebak.
func TooManyRequests(w http.ResponseWriter, r *http.Request, message string, retryAfter time.Duration) {
	setRetryAfter(w, retryAfter)
	WriteError(w, r, http.StatusTooManyRequests, ErrTypeRateLimit, CodeRateLimitExceeded,
		orDefault(message, msgTooManyRequests))
}

// PayloadTooLarge membalas 413 dan menyebutkan batasnya, karena itu satu-satunya
// informasi yang bisa dipakai klien untuk memperbaiki request.
func PayloadTooLarge(w http.ResponseWriter, r *http.Request, limit int64) {
	WriteError(w, r, http.StatusRequestEntityTooLarge, ErrTypeInvalidRequest, CodePayloadTooLarge,
		fmt.Sprintf("ukuran body request melebihi batas %d byte", limit))
}

// InternalError membalas 500 dengan pesan generik. Pesannya tidak bisa diatur:
// catat penyebab aslinya lewat observability.LoggerFrom(r.Context()) sebelum
// memanggil ini, dan biarkan klien hanya menerima request_id untuk dilaporkan.
func InternalError(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusInternalServerError, ErrTypeAPI, CodeInternalError, msgInternalError)
}

// ServiceUnavailable membalas 503 dengan tipe overloaded_error, yang oleh klien
// OpenAI diperlakukan sebagai kegagalan sementara yang layak diulang.
func ServiceUnavailable(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	setRetryAfter(w, retryAfter)
	WriteError(w, r, http.StatusServiceUnavailable, ErrTypeOverloaded, CodeServiceUnavailable, msgServiceUnavailable)
}

// setRetryAfter menulis Retry-After dalam detik. Nilai di bawah satu detik
// dibulatkan ke atas menjadi 1, karena satuan header ini hanya detik dan 0 akan
// mengundang klien mencoba ulang seketika.
func setRetryAfter(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter <= 0 {
		return
	}
	seconds := int64(math.Ceil(retryAfter.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// PrepareSSE menyiapkan respons untuk Server-Sent Events: memasang header stream,
// melepas tenggat baca/tulis koneksi, lalu mengirim header ke klien.
//
// Panggil ini SETELAH semua validasi lolos — begitu header terkirim, status respons
// tidak bisa diubah lagi, jadi kegagalan setelah titik ini hanya bisa dilaporkan
// sebagai event di dalam stream. Setelah itu tulis event dengan Write biasa dan
// kirimkan dengan http.NewResponseController(w).Flush().
//
// Pelepasan tenggat adalah bagian yang paling mudah terlewat. Server memasang
// tenggat baca dari ReadTimeout dan tenggat tulis dari WriteTimeout pada seluruh
// request. Untuk sisi tulis akibatnya jelas: stream terputus begitu WriteTimeout
// lewat. Untuk sisi baca akibatnya tidak kelihatan tapi sama fatalnya — net/http
// menyimpan satu pembacaan latar untuk mendeteksi klien yang menutup koneksi, dan
// pembacaan itulah yang kena tenggat, lalu membatalkan context request. Jadi dengan
// READ_TIMEOUT=30s, setiap completion yang lebih panjang dari 30 detik akan mati di
// tengah jalan tanpa pesan yang jelas kecuali tenggatnya dilepas di sini.
func PrepareSSE(w http.ResponseWriter, r *http.Request) error {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	// Connection hanya bermakna di HTTP/1.x; pada HTTP/2 header ini terlarang dan
	// akan dibuang net/http.
	if r == nil || r.ProtoMajor < 2 {
		h.Set("Connection", "keep-alive")
	}
	// Memberi tahu nginx dan proxy sejenis agar tidak menahan respons di buffer;
	// tanpa ini token baru muncul menumpuk di akhir walau server sudah flush.
	h.Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("melepas tenggat baca untuk SSE: %w", err)
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("melepas tenggat tulis untuk SSE: %w", err)
	}

	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("mengirim header SSE: %w", err)
	}
	return nil
}
