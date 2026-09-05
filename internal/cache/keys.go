package cache

import "strings"

// KeyPrefix adalah namespace akar untuk SEMUA kunci yang ditulis aplikasi ini.
//
// Instance Redis bisa dipakai bersama aplikasi lain di host yang sama, jadi tidak ada
// kunci yang boleh ditulis tanpa melewati Key(). Prefix ini juga yang dipakai operator
// untuk menyapu data aplikasi (SCAN MATCH routex:*) tanpa menyentuh milik tetangga —
// itulah sebabnya FLUSHDB/FLUSHALL tidak pernah dipakai di kode maupun di test.
const KeyPrefix = "routex"

// keySeparator memisahkan segmen kunci. Karakter ini dilarang muncul di dalam segmen
// (escapeKeyPart yang memastikan), sehingga batas antar segmen selalu tidak ambigu.
const keySeparator = ":"

// Namespace segmen kedua, satu per subsistem. Diekspor supaya pemilik subsistem bisa
// menyusun pola SCAN sendiri (mis. Key(NamespaceHealth) + ":*") tanpa menyalin string.
const (
	NamespaceRateLimit      = "rl"
	NamespaceCircuitBreaker = "cb"
	NamespaceHealth         = "health"
	NamespaceResponseCache  = "rc"
)

// Key menggabungkan beberapa bagian menjadi satu kunci Redis bernamespace:
//
//	Key("rl", "apikey", "01H8Z") => "routex:rl:apikey:01H8Z"
//
// Aturannya:
//   - Selalu diawali KeyPrefix, termasuk ketika dipanggil tanpa argumen (=> "routex").
//   - Bagian kosong — atau yang hanya berisi spasi — dilewati, sehingga pemanggil boleh
//     meneruskan nilai opsional tanpa menghasilkan kunci ber-"::" yang ambigu.
//   - Setiap bagian di-escape oleh escapeKeyPart sebelum digabung.
func Key(parts ...string) string {
	var b strings.Builder
	// Perkiraan kasar supaya umumnya cukup sekali alokasi.
	b.Grow(len(KeyPrefix) + len(parts)*16)
	b.WriteString(KeyPrefix)

	for _, p := range parts {
		// Spasi di pinggir hampir selalu kecelakaan (nilai dari env atau form yang
		// belum dirapikan), bukan bagian dari identitas. Dibuang lebih dulu supaya
		// "abc" dan " abc" tidak menjadi dua kunci berbeda.
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString(keySeparator)
		b.WriteString(escapeKeyPart(p))
	}
	return b.String()
}

const hexDigits = "0123456789ABCDEF"

// escapeKeyPart mengubah satu bagian kunci menjadi bentuk aman lewat percent-encoding
// gaya URL.
//
// Nilai yang masuk ke kunci berasal dari data, bukan dari konstanta: ID API key, ID
// provider, nama model. Kalau nilai seperti "gpt-4:evil" dipakai mentah, penyusun nilai
// itu bisa mengarang kunci yang seolah-olah berada di namespace lain
// ("routex:rl:gpt-4:evil" tampak seperti kunci milik subsistem "evil") atau menabrak
// kunci milik tenant lain. Karena itu hanya karakter yang jelas aman yang dilewatkan,
// sisanya menjadi "%XX".
//
// Percent-encoding dipilih karena reversibel dan injektif: dua nilai berbeda tidak
// mungkin menghasilkan kunci yang sama. Bandingkan dengan mengganti karakter terlarang
// dengan "_", yang membuat "a:b" dan "a_b" bertabrakan — persis celah yang mau
// dihindari. "%" sendiri ikut di-escape menjadi "%25" supaya encoding-nya tidak ambigu.
func escapeKeyPart(part string) string {
	// Jalur cepat: mayoritas segmen (UUID, slug model, nama subsistem) sudah aman
	// sehingga tidak perlu alokasi sama sekali.
	needsEscape := false
	for i := 0; i < len(part); i++ {
		if !isSafeKeyByte(part[i]) {
			needsEscape = true
			break
		}
	}
	if !needsEscape {
		return part
	}

	var b strings.Builder
	b.Grow(len(part) + 8)
	// Diproses per byte, bukan per rune: itu membuat karakter non-ASCII ikut ter-encode
	// sebagai deretan "%XX" yang sah, sama seperti perilaku encoding URL.
	for i := 0; i < len(part); i++ {
		c := part[i]
		if isSafeKeyByte(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0F])
	}
	return b.String()
}

// isSafeKeyByte melaporkan apakah satu byte boleh muncul apa adanya di dalam segmen.
//
// Daftarnya sengaja sempit: huruf, angka, dan beberapa tanda yang lazim ada di
// pengenal nyata — "-" dan "_" untuk slug, "." untuk versi model ("gemini-1.5-pro"),
// "/" untuk nama model bergaya vendor ("openai/gpt-4o"), "@" untuk email admin.
// Di luar itu semuanya di-escape, termasuk ":" (pemisah segmen), spasi, "*" dan "?"
// (glob SCAN), serta "{" dan "}" (hash tag Redis Cluster).
func isSafeKeyByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '-', '_', '.', '/', '@':
		return true
	}
	return false
}

// --- Konstruktor kunci bertipe ----------------------------------------------
//
// Daftar di bawah hanya memuat kunci yang bentuknya sudah pasti sekarang. Ia akan
// bertambah di fase berikutnya (kuota & penagihan, cache respons, sesi admin, antrean
// webhook). Aturannya tetap: subsistem baru menambahkan konstruktornya di sini, bukan
// menyusun string kunci sendiri di tempat lain, supaya bentuk kunci tetap bisa diaudit
// dari satu berkas.

// RateLimitKey menyusun kunci penghitung rate limit.
//
// scope adalah sumbu pembatasan ("apikey", "ip", "org"), id adalah pemilik kuota, dan
// window adalah label jendela waktu (mis. nomor bucket atau lebar jendela). Nomor
// jendela ikut ke dalam kunci supaya penghitung jendela lama hilang sendiri lewat TTL,
// bukan harus dibersihkan pekerjaan latar.
func RateLimitKey(scope, id, window string) string {
	return Key(NamespaceRateLimit, scope, id, window)
}

// CircuitBreakerKey menyusun kunci status circuit breaker untuk satu pasangan
// provider+model.
//
// Dipecah sampai level model karena kegagalan upstream biasanya spesifik per model:
// satu model bisa kelebihan beban atau ditarik dari peredaran sementara model lain di
// provider yang sama tetap sehat. Kalau breaker dipasang per provider saja, satu model
// bermasalah akan mematikan seluruh provider.
//
// model boleh kosong (mis. breaker level provider); Key() akan melewatinya.
func CircuitBreakerKey(providerID, model string) string {
	return Key(NamespaceCircuitBreaker, providerID, model)
}

// HealthKey menyusun kunci hasil health check terakhir sebuah provider, yang dibagi
// antar instance gateway supaya tidak setiap instance memeriksa provider yang sama.
func HealthKey(providerID string) string {
	return Key(NamespaceHealth, providerID)
}
