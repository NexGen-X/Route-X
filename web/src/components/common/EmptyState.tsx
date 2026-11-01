import React from 'react';
import { Card } from './Card';

// EmptyState — pola keadaan kosong baku (Fase 3 P5).
//
// Menggantikan kartu kosong raksasa yang ditulis tangan per halaman
// (kasus pertama: Routing dengan padding p-10 dan warna ikon di luar
// token). Ukuran kini satu skala: padding p-6 di mobile / sm:p-8,
// ikon dalam kotak aksen token tema, judul text-sm, deskripsi text-xs,
// dan slot aksi opsional di bawahnya.
export interface EmptyStateProps {
  icon: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  icon,
  title,
  description,
  action,
  className = '',
}) => (
  <Card
    className={`p-6 sm:p-8 text-center rounded-box bg-bg-surface-1 border border-border/60 ${className}`}
  >
    <div className="w-11 h-11 rounded-xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center mx-auto mb-3">
      {icon}
    </div>
    <h3 className="text-sm font-bold text-white mb-1">{title}</h3>
    {description && (
      <p className="text-xs text-text-secondary max-w-md mx-auto leading-relaxed">
        {description}
      </p>
    )}
    {action && <div className="mt-4 flex justify-center">{action}</div>}
  </Card>
);
