package repo

import "strings"

// likeEscaper meloloskan tiga karakter yang bermakna khusus di dalam pola
// LIKE/ILIKE PostgreSQL: garis miring terbalik itu sendiri, %, dan _.
// Karakter pelolos bawaannya memang garis miring terbalik, jadi hasil fungsi
// ini bisa langsung dipakai sebagai argumen pola tanpa klausa ESCAPE tambahan.
// strings.NewReplacer bekerja serentak (keluarannya tidak dipindai ulang),
// jadi urutan penggantian tidak bisa saling menimpa.
var likeEscaper = strings.NewReplacer(
	`\`, `\\`,
	"%", `\%`,
	"_", `\_`,
)

// EscapeLikePattern meloloskan teks pencarian pengguna sebelum disisipkan ke
// pola ILIKE (audit3 V-DT-23).
//
// Tanpa ini, mengetik "%" atau "_" di kolom pencarian admin bertindak sebagai
// wildcard: pencarian "%" mengembalikan SELURUH baris. Teks yang dicari
// pengguna adalah teks harfiah — wildcard adalah milik pola yang disusun
// kode (mis. '%' || $n || '%'), bukan milik masukan.
func EscapeLikePattern(s string) string {
	return likeEscaper.Replace(s)
}
