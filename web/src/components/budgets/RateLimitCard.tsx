import React from 'react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Tooltip } from '../common/Tooltip';
import { Gauge, Trash2, Shield, Waves } from 'lucide-react';
import type { RateLimitCardProps } from './types';

export const RateLimitCard: React.FC<RateLimitCardProps> = ({
  limit,
  scopeDisplay,
  onDelete,
}) => {
  const rpm = limit.requests_per_minute ?? 0;
  const tpm = limit.tokens_per_minute ?? 0;
  const rps = limit.requests_per_second ?? 0;

  // Indikator visual kapasitas leaky/token bucket (persentase simulasi kuota token/req)
  const rpmCapacityPct = rpm > 0 ? Math.min(100, Math.max(15, Math.round((rpm / 120) * 100))) : 100;
  const tpmCapacityPct = tpm > 0 ? Math.min(100, Math.max(15, Math.round((tpm / 200000) * 100))) : 100;

  return (
    <Card className="p-5 flex flex-col justify-between" data-testid={`rate-limit-card-${limit.id}`}>
      <div>
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-md bg-zinc-800 border border-zinc-700/60 text-zinc-300 flex items-center justify-center shrink-0">
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
            <span className="font-mono text-sm font-bold text-accent mt-0.5 block">
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

        {/* Bucket Visual Indicator (Token & Leaky Bucket Capacity) */}
        <div className="mt-3.5 p-3 rounded-inner bg-bg-surface-1 border border-border/60 space-y-2">
          <div className="flex items-center justify-between text-[11px]">
            <span className="text-text-muted flex items-center gap-1.5 font-medium">
              <Waves className="w-3.5 h-3.5 text-blue-400" aria-hidden="true" />
              <span>Token Bucket Indicator</span>
            </span>
            <span className="font-mono text-xs text-blue-400 font-semibold">
              {rps > 0 ? `${rps} burst window` : 'unlimited'}
            </span>
          </div>

          {/* RPM Bucket Level */}
          <div className="space-y-1">
            <div className="flex justify-between text-[10px] font-mono text-text-muted">
              <span>RPM Bucket Kapasitas</span>
              <span>{rpm > 0 ? `${rpm} RPM` : 'Tidak Terbatas'}</span>
            </div>
            <div className="w-full h-1.5 bg-bg-surface-2 rounded-full overflow-hidden">
              <div
                className="h-full bg-gradient-to-r from-blue-500 to-sky-400 rounded-full transition-all duration-500"
                style={{ width: `${rpmCapacityPct}%` }}
              />
            </div>
          </div>

          {/* TPM Bucket Level */}
          <div className="space-y-1">
            <div className="flex justify-between text-[10px] font-mono text-text-muted">
              <span>Token Flow Bandwidth</span>
              <span>{tpm > 0 ? `${tpm.toLocaleString()} TPM` : 'Tidak Terbatas'}</span>
            </div>
            <div className="w-full h-1.5 bg-bg-surface-2 rounded-full overflow-hidden">
              <div
                className="h-full bg-gradient-to-r from-indigo-500 to-purple-400 rounded-full transition-all duration-500"
                style={{ width: `${tpmCapacityPct}%` }}
              />
            </div>
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
