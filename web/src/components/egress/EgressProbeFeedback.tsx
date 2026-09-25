import React from 'react';
import { CheckCircle2, AlertCircle } from 'lucide-react';
import type { EgressProbeResult } from '../../types';

export interface EgressProbeFeedbackData {
  poolId: string;
  result: EgressProbeResult;
}

export interface EgressProbeFeedbackProps {
  feedback: EgressProbeFeedbackData | null;
  onDismiss: () => void;
}

export const EgressProbeFeedback: React.FC<EgressProbeFeedbackProps> = ({
  feedback,
  onDismiss,
}) => {
  if (!feedback) return null;

  const { result } = feedback;
  const isHealthy = result.status === 'healthy';

  return (
    <div
      role={isHealthy ? 'status' : 'alert'}
      aria-live="polite"
      className={`p-4 rounded-xl text-xs border flex items-start gap-3 transition-all ${
        isHealthy
          ? 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
          : 'bg-red-500/10 text-red-300 border-red-500/30'
      }`}
    >
      {isHealthy ? (
        <CheckCircle2 className="w-5 h-5 shrink-0 text-emerald-400 mt-0.5" />
      ) : (
        <AlertCircle className="w-5 h-5 shrink-0 text-red-400 mt-0.5" />
      )}
      <div className="flex-1 space-y-1">
        <div className="flex items-center justify-between">
          <span className="font-bold uppercase tracking-wider">
            {isHealthy ? 'Uji Koneksi Berhasil' : 'Uji Koneksi Gagal'}
          </span>
          {result.latency_ms !== undefined && (
            <span className="font-mono text-[11px] bg-bg-surface-2 px-2 py-0.5 rounded border border-border text-emerald-400">
              {result.latency_ms} ms
            </span>
          )}
        </div>
        <p>{result.message}</p>
        {result.exit_ip && (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pt-1 text-[11px] text-text-secondary font-mono">
            <span>
              Exit IP: <strong className="text-white">{result.exit_ip}</strong>
            </span>
            {result.country && (
              <span>
                Wilayah: <strong className="text-accent">{result.country}</strong>
              </span>
            )}
            {result.datacenter && (
              <span>
                Edge PoP: <strong className="text-white">{result.datacenter}</strong>
              </span>
            )}
          </div>
        )}
      </div>
      <button
        onClick={onDismiss}
        className="text-text-muted hover:text-white text-xs font-bold px-1 cursor-pointer"
        aria-label="Tutup notifikasi"
      >
        ✕
      </button>
    </div>
  );
};
