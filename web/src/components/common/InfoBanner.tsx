import React, { useState } from 'react';
import { AlertCircle, ChevronDown, Info, ShieldAlert } from 'lucide-react';

// InfoBanner — banner informasi ringkasan + detail terlipat (Fase 3 P5).
//
// Menggantikan dinding teks satu paragraf (kasus pertama: penjelasan
// kebijakan cache di Settings). Baris ringkasan selalu terlihat;
// penjelasan panjang tinggal di balik tombol "Detail" yang bisa
// dibuka-tutup, dengan aria-expanded yang benar. Detail tetap
// terpasang di DOM (disembunyikan dengan kelas) supaya pencarian
// teks dan pengujian tidak kehilangan isinya.
export interface InfoBannerProps {
  variant?: 'warning' | 'danger' | 'info';
  summary: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
}

const GAYA = {
  warning: 'border-amber-500/30 bg-amber-500/10 text-amber-200',
  danger: 'border-red-500/30 bg-red-500/10 text-red-300',
  info: 'border-accent/30 bg-accent/10 text-text-secondary',
} as const;

const IKON = {
  warning: ShieldAlert,
  danger: AlertCircle,
  info: Info,
} as const;

export const InfoBanner: React.FC<InfoBannerProps> = ({
  variant = 'info',
  summary,
  children,
  className = '',
}) => {
  const [terbuka, setTerbuka] = useState(false);
  const Ikon = IKON[variant];
  const adaDetail = React.Children.count(children) > 0;
  return (
    <div className={`rounded-lg border p-3 ${GAYA[variant]} ${className}`}>
      <div className="flex gap-2.5">
        <Ikon className="w-4 h-4 shrink-0 mt-0.5" aria-hidden="true" />
        <div className="flex-1 min-w-0">
          <p className="text-[11px] leading-relaxed font-medium">{summary}</p>
          {adaDetail && (
            <div className={terbuka ? 'mt-1.5' : 'hidden'}>
              <div className="text-[11px] leading-relaxed opacity-90">{children}</div>
            </div>
          )}
        </div>
        {adaDetail && (
          <button
            type="button"
            onClick={() => setTerbuka((v) => !v)}
            aria-expanded={terbuka}
            className="shrink-0 inline-flex items-center gap-1 text-[11px] font-semibold underline-offset-2 hover:underline cursor-pointer min-h-[28px]"
          >
            Detail
            <ChevronDown
              className={`w-3.5 h-3.5 transition-transform ${terbuka ? 'rotate-180' : ''}`}
              aria-hidden="true"
            />
          </button>
        )}
      </div>
    </div>
  );
};
