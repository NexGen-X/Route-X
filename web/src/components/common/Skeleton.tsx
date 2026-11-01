import React from 'react';

// Skeleton — primitif bahasa loading baku (Fase 3 P5).
//
// Satu balok placeholder: permukaan surface-2 membulat. Pulsa animasi
// ditaruh di WADAH pemakai (animate-pulse pada kartu/baris), bukan di
// tiap balok, supaya seluruh kelompok loading berdenyut serempak
// seperti pola yang sudah hidup di halaman Routing dan Pengguna.
//
// Contoh:
//   <Card className="animate-pulse"><Skeleton className="h-4 w-2/3" /></Card>
export const Skeleton: React.FC<{ className?: string }> = ({ className = '' }) => (
  <div aria-hidden="true" className={`rounded bg-bg-surface-2 ${className}`} />
);
