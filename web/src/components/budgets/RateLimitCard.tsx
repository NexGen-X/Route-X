import React from 'react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Tooltip } from '../common/Tooltip';
import { Gauge, Trash2, Shield } from 'lucide-react';
import type { RateLimitCardProps } from './types';

export const RateLimitCard: React.FC<RateLimitCardProps> = ({
  limit,
  scopeDisplay,
  onDelete,
}) => {
  const rpm = limit.requests_per_minute ?? 0;
  const tpm = limit.tokens_per_minute ?? 0;
  const rps = limit.requests_per_second ?? 0;

  // Batang "kapasitas bucket" lama diskalakan ke konstanta karangan
  // (120 RPM / 200rb TPM, dilantai 15%) dan tidak mengukur pemakaian apa
  // pun — API tidak menyediakan sisa kuota jendela berjalan per aturan.
  // Batangnya DIHAPUS (audit3 V-FJ-05): kartu ini hanya menampilkan angka
  // konfigurasi nyata di grid di bawah.

  return (
    <Card className="p-5 flex flex-col justify-between" data-testid={`rate-limit-card-${limit.id}`}>
      <div>
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-md bg-bg-surface-2 border border-border-hover/60 text-text-secondary flex items-center justify-center shrink-0">
              <Gauge className="w-4 h-4" aria-hidden="true" />
            </div>
            <div>
              <h4 className="text-sm font-bold text-white uppercase tracking-wider">{limit.scope}</h4>
              <span className="text-xs text-text-muted font-mono">
                {scopeDisplay.label}
              </span>
            </div>
          </div>
          <Badge variant="info">Aktif</Badge>
        </div>

        {/* 3 Metrik Batas Laju */}
        <div className="mt-5 grid grid-cols-3 gap-2 p-3 bg-bg-surface-2 rounded-lg border border-border/50 text-center">
          <div>
            <span className="block text-[10px] text-text-muted uppercase">Req / Mnt</span>
            <span className="font-mono text-sm font-bold text-white mt-0.5 block">
              {rpm > 0 ? rpm.toLocaleString() : '∞'}
            </span>
          </div>
          <div>
            <span className="block text-[10px] text-text-muted uppercase">Tok / Mnt</span>
            <span className="font-mono text-sm font-bold text-accent-text mt-0.5 block">
              {tpm > 0 ? tpm.toLocaleString() : '∞'}
            </span>
          </div>
          <div>
            <span className="block text-[10px] text-text-muted uppercase">Burst / Dtk</span>
            <span className="font-mono text-sm font-bold text-white mt-0.5 block">
              {rps > 0 ? `${rps}/s` : '∞'}
            </span>
          </div>
        </div>

      </div>

      <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-[11px] text-text-muted font-mono">
          <Shield className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" />
          <span>Active Token Guard</span>
        </div>
        <Tooltip content="Hapus Aturan Limit" position="left">
          <button
            type="button"
            onClick={() => void onDelete(limit.id)}
            className="min-h-[44px] min-w-[44px] sm:min-h-[32px] sm:min-w-[32px] flex items-center justify-center rounded-md text-text-muted hover:text-rose-400 hover:bg-rose-500/10 transition-colors cursor-pointer"
            aria-label="Hapus limit"
          >
            <Trash2 className="w-4 h-4" aria-hidden="true" />
          </button>
        </Tooltip>
      </div>
    </Card>
  );
};
