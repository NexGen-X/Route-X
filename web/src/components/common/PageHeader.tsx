import React from 'react';

// Kepala halaman bersama: judul + deskripsi di atas, aksi full-width di
// bawah pada layar kecil; sebaris hanya mulai breakpoint sm ke atas.
// Teks judul dan tombol tidak diubah komponen ini, jadi aman untuk e2e.
interface PageHeaderProps {
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
}

export const PageHeader: React.FC<PageHeaderProps> = ({ title, description, actions }) => {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <div className="min-w-0">
        <h2 className="text-xl sm:text-2xl font-bold tracking-tight text-white">{title}</h2>
        {description && <p className="text-xs text-text-secondary mt-1">{description}</p>}
      </div>
      {actions && <div className="flex flex-col sm:flex-row gap-2 sm:items-center shrink-0">{actions}</div>}
    </div>
  );
};
