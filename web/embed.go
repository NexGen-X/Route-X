// Package web menyematkan hasil build frontend ke dalam biner.
//
// Direktif go:embed hanya bisa menjangkau direktori paketnya sendiri dan turunannya,
// jadi paket ini harus berada di web/ — cmd/ai-gateway tidak bisa menyematkan
// web/dist secara langsung.
package web

import (
	"embed"
	"io/fs"
)

// distFS memuat seluruh isi web/dist. Prefiks "all:" dipakai supaya berkas yang
// namanya dimulai titik atau garis bawah ikut tersemat — termasuk .gitkeep, yang
// membuat direktif ini tetap sah walau frontend belum pernah di-build.
//
//go:embed all:dist
var distFS embed.FS

// DistFS mengembalikan isi web/dist dengan prefiks "dist" sudah dilepas, sehingga
// "index.html" bisa diakses langsung di akar.
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
