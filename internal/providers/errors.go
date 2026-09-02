package providers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrorKind mengelompokkan kegagalan upstream.
//
// Klasifikasi ini yang menentukan perilaku mesin routing, jadi setiap nilai punya
// konsekuensi yang tegas — bukan sekadar label untuk log:
//
//   - Retryable: aman diulang ke provider yang SAMA.
//   - Failover: aman dicoba ke provider LAIN.
//
// Keduanya tidak selalu berjalan bersama. Rate limit boleh diulang di provider yang sama
// setelah menunggu dan boleh juga dialihkan. Sebaliknya request yang tidak sah tidak
// boleh diulang ke mana pun: mengalihkan permintaan cacat ke provider lain hanya
// menghasilkan kegagalan kedua sambil membebani kuota, dan menyembunyikan penyebab
// aslinya dari pengguna.
type ErrorKind string

const (
	// ErrKindTimeout: batas waktu terlampaui, baik saat menghubungkan maupun membaca.
	ErrKindTimeout ErrorKind = "timeout"
	// ErrKindNetwork: gagal dial, DNS, atau koneksi terputus di tengah.
	ErrKindNetwork ErrorKind = "network"
	// ErrKindRateLimit: 429 dari upstream.
	ErrKindRateLimit ErrorKind = "rate_limit"
	// ErrKindQuota: kuota atau saldo provider habis. Berbeda dari rate limit karena
	// menunggu tidak menolong — hanya provider lain yang bisa.
	ErrKindQuota ErrorKind = "quota"
	// ErrKindAuth: kredensial ditolak. Tidak boleh diulang: kredensialnya tidak akan
	// berubah dalam hitungan milidetik, dan mengulang hanya mempercepat pemblokiran.
	ErrKindAuth ErrorKind = "auth"
	// ErrKindPermission: kredensial sah tetapi tidak berhak atas model atau fitur ini.
	ErrKindPermission ErrorKind = "permission"
	// ErrKindInvalidRequest: 4xx karena bentuk permintaan, mis. terlalu panjang.
	ErrKindInvalidRequest ErrorKind = "invalid_request"
	// ErrKindModelNotFound: model tidak dikenal di provider ini. Provider lain mungkin
	// mengenalnya, jadi ini alasan failover yang sah.
	ErrKindModelNotFound ErrorKind = "model_not_found"
	// ErrKindContentFilter: upstream menolak karena kebijakan kontennya sendiri.
	// Tidak dialihkan: provider lain kemungkinan besar menolak dengan alasan sama, dan
	// mencoba semuanya berarti mengirim muatan yang sama ke banyak pihak.
	ErrKindContentFilter ErrorKind = "content_filter"
	// ErrKindOverloaded: 503 atau 529, provider sedang penuh.
	ErrKindOverloaded ErrorKind = "overloaded"
	// ErrKindServer: 5xx lain.
	ErrKindServer ErrorKind = "server"
	// ErrKindStreamAborted: aliran terputus setelah sebagian data terkirim.
	ErrKindStreamAborted ErrorKind = "stream_aborted"
	// ErrKindCanceled: klien membatalkan. Bukan kegagalan provider.
	ErrKindCanceled ErrorKind = "canceled"
	// ErrKindUnknown: tidak terklasifikasi.
	ErrKindUnknown ErrorKind = "unknown"
)

// retryable menyatakan kegagalan yang aman diulang ke provider yang sama.
var retryable = map[ErrorKind]bool{
	ErrKindTimeout:    true,
	ErrKindNetwork:    true,
	ErrKindRateLimit:  true,
	ErrKindOverloaded: true,
	ErrKindServer:     true,
}

// failoverable menyatakan kegagalan yang aman dicoba ke provider lain.
var failoverable = map[ErrorKind]bool{
	ErrKindTimeout:       true,
	ErrKindNetwork:       true,
	ErrKindRateLimit:     true,
	ErrKindQuota:         true,
	ErrKindOverloaded:    true,
	ErrKindServer:        true,
	ErrKindModelNotFound: true,
	// Kredensial satu provider tidak berkaitan dengan provider lain, jadi kegagalan
	// autentikasi di satu tempat justru alasan kuat mencoba yang berikutnya.
	ErrKindAuth:       true,
	ErrKindPermission: true,
}

// Error adalah kegagalan upstream yang sudah terklasifikasi.
//
// Message SUDAH disaring dan aman masuk log, database, maupun respons ke klien. Detail
// mentah dari provider tidak disimpan di sini: pesan error upstream bisa memuat kembali
// potongan permintaan, dan pada gateway itu berarti data satu pengguna muncul di log
// yang dibaca operator.
type Error struct {
	Kind ErrorKind
	// Provider adalah nama provider yang gagal.
	Provider string
	// StatusCode adalah status HTTP upstream, 0 bila kegagalannya sebelum ada respons.
	StatusCode int
	// Message adalah keterangan singkat yang sudah disaring.
	Message string
	// UpstreamCode adalah kode error milik provider bila ada, mis. "insufficient_quota".
	UpstreamCode string
	// RetryAfter berasal dari header Retry-After bila upstream mengirimkannya.
	RetryAfter time.Duration
	// StreamedBytes > 0 berarti sebagian respons sudah terkirim ke klien. Ini mengubah
	// segalanya: mengulang atau mengalihkan request akan menghasilkan aliran kedua yang
	// tersambung di tengah aliran pertama, dan klien menerima jawaban yang tidak
	// koheren. Mesin routing harus memeriksanya sebelum memutuskan.
	StreamedBytes int64
}

func (e *Error) Error() string {
	if e.Provider == "" {
		return fmt.Sprintf("%s: %s", e.Kind, e.Message)
	}
	return fmt.Sprintf("%s pada provider %s: %s", e.Kind, e.Provider, e.Message)
}

// Retryable melaporkan apakah aman mengulang ke provider yang sama.
//
// Selalu false setelah sebagian aliran terkirim — lihat StreamedBytes.
func (e *Error) Retryable() bool {
	if e.StreamedBytes > 0 {
		return false
	}
	return retryable[e.Kind]
}

// Failoverable melaporkan apakah aman mencoba provider lain.
func (e *Error) Failoverable() bool {
	if e.StreamedBytes > 0 {
		return false
	}
	return failoverable[e.Kind]
}

// Newf membuat Error dengan pesan terformat.
func Newf(kind ErrorKind, provider string, format string, args ...any) *Error {
	return &Error{Kind: kind, Provider: provider, Message: fmt.Sprintf(format, args...)}
}

// Classify menerjemahkan status HTTP menjadi ErrorKind.
//
// upstreamCode ikut diperiksa karena status saja sering tidak cukup: OpenAI memakai 429
// baik untuk pembatasan laju maupun untuk kuota yang habis, dan keduanya menuntut
// keputusan routing yang berbeda — menunggu menolong yang pertama, tidak yang kedua.
func Classify(status int, upstreamCode string) ErrorKind {
	switch upstreamCode {
	case "insufficient_quota", "billing_hard_limit_reached", "credit_balance_too_low":
		return ErrKindQuota
	case "model_not_found", "model_not_available":
		return ErrKindModelNotFound
	case "content_filter", "content_policy_violation":
		return ErrKindContentFilter
	}

	switch {
	case status == 401:
		return ErrKindAuth
	case status == 403:
		return ErrKindPermission
	case status == 404:
		return ErrKindModelNotFound
	case status == 408:
		return ErrKindTimeout
	case status == 413:
		return ErrKindInvalidRequest
	case status == 429:
		return ErrKindRateLimit
	case status == 503, status == 529:
		return ErrKindOverloaded
	case status >= 500:
		return ErrKindServer
	case status >= 400:
		return ErrKindInvalidRequest
	default:
		return ErrKindUnknown
	}
}

// ClassifyTransport menerjemahkan kegagalan tingkat transport menjadi ErrorKind.
//
// Pembatalan diperiksa lebih dulu: context yang dibatalkan sering muncul terbungkus
// sebagai error jaringan, dan kalau salah diklasifikasi, gateway akan mengulang
// permintaan yang kliennya sudah pergi.
func ClassifyTransport(err error) ErrorKind {
	switch {
	case err == nil:
		return ErrKindUnknown
	case errors.Is(err, context.Canceled):
		return ErrKindCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return ErrKindTimeout
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrKindTimeout
	}
	return ErrKindNetwork
}

// FromTransport membuat Error dari kegagalan transport, tanpa menyertakan pesan asli.
//
// Pesan dari paket net bisa memuat host dan port upstream. Pada provider BYOK itu
// alamat milik pengguna lain, jadi yang keluar hanya kategorinya.
func FromTransport(provider string, err error) *Error {
	kind := ClassifyTransport(err)

	var msg string
	switch kind {
	case ErrKindCanceled:
		msg = "permintaan dibatalkan sebelum selesai"
	case ErrKindTimeout:
		msg = "upstream tidak menjawab sebelum batas waktu"
	default:
		msg = "gagal menghubungi upstream"
	}
	return &Error{Kind: kind, Provider: provider, Message: msg}
}

// AsError mengambil *Error dari rantai error, nil bila bukan kegagalan upstream.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
