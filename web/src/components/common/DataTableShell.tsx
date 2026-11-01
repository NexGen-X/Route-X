import React from 'react';

// DataTableShell — pembungkus tabel baku halaman-halaman dashboard
// (Fase 2 R7). Mengkodekan pelajaran QA final 1 Nov 2026 agar tidak
// dihafal ulang per halaman:
//
// 1. Wadah selalu `overflow-x-auto` — halaman tidak pernah scroll
//    horizontal; yang bergeser hanya tabel di dalam kartunya.
// 2. Tabel selalu `w-full text-left text-xs` dengan token tema yang sama.
// 3. Kepadatan sel mengikuti satu skala padding responsif (kelasSelKepala /
//    kelasSelIsi): sempit di bawah `sm`, lega dari `sm` ke atas, supaya
//    kolom aksi tetap terjangkau di viewport kecil (kasus tabel Pengguna
//    yang melebar 445px di layar 412px, PR #172–#173).
//
// Shell TIDAK menebak kolom mana yang fleksibel: sel berisi teks panjang
// satu token (email, ID) tetap wajib dibatasi pemakai dengan max-w-* +
// truncate, seperti pola di halaman Pengguna.
export interface DataTableShellProps {
  children: React.ReactNode;
  // Kelas tambahan pada wadah (mis. menyembunyikan tabel di layar kecil
  // seperti halaman Requests yang menampilkan daftar kartu di mobile).
  className?: string;
  testId?: string;
}

export const DataTableShell: React.FC<DataTableShellProps> = ({
  children,
  className = '',
  testId,
}) => (
  <div className={`overflow-x-auto ${className}`.trim()}>
    <table className="w-full text-left text-xs" data-testid={testId}>
      {children}
    </table>
  </div>
);

// Kelas baku sel — satu skala padding untuk semua tabel. kelasSelIsi
// adalah skala murni (dipakai juga oleh tabel bergaya baris seperti
// Requests/Models); kelasSelKepala menambahkan perlakuan visual kepala
// tabel (huruf kapital redup) ala halaman Pengguna.
export const kelasSelKepala =
  'px-2.5 py-3 sm:px-4 text-[11px] font-semibold text-text-muted uppercase tracking-wide';
export const kelasSelKepalaKanan = `${kelasSelKepala} text-right`;
export const kelasSelIsi = 'px-2.5 py-3 sm:px-4';
