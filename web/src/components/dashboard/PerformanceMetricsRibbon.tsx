import React from 'react';
import type { ObservabilitySummary } from '../../types';
import { formatUSD } from '../../utils/money';

export interface PerformanceMetricsRibbonProps {
  summary: ObservabilitySummary | null;
}

// Fase 4 (Calm Console): empat kartu metrik menjadi strip KPI polos —
// tanpa kotak, tile ikon berwarna, dan footer dekoratif. Label, nilai,
// dan sub-info dipertahankan persis agar jangkar uji tidak berubah;
// hierarki kini dibawa tipografi dan garis pemisah tipis.
export const PerformanceMetricsRibbon: React.FC<PerformanceMetricsRibbonProps> = ({ summary }) => {
  const errorRate = summary?.error_rate ?? 0;
  const isHealthyError = errorRate <= 5;
  const totalTokens = summary?.total_tokens ?? 0;
  const formattedTokens =
    totalTokens >= 1000000
      ? `${(totalTokens / 1000000).toFixed(2)}M`
      : `${(totalTokens / 1000).toFixed(1)}k`;

  const selKelas =
    'lg:border-l lg:border-border/60 lg:pl-6 lg:first:border-l-0 lg:first:pl-0';
  const labelKelas =
    'text-[11px] font-medium uppercase tracking-wider text-text-muted truncate block';
  const nilaiKelas =
    'mt-1.5 text-2xl sm:text-[28px] leading-none font-semibold text-text-primary tabular-nums tracking-tight';
  const subKelas = 'mt-2 text-[11px] text-text-muted font-mono tabular-nums truncate';

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-5">
      {/* Metrik 1: Total Requests */}
      <div className={selKelas}>
        <span className={labelKelas}>Total Permintaan</span>
        <div className={nilaiKelas}>{(summary?.total_requests ?? 0).toLocaleString()}</div>
        <div className={subKelas}>
          <span>{(summary?.success_requests ?? 0).toLocaleString()} ok</span>
          <span aria-hidden="true"> · </span>
          <span className={isHealthyError ? 'text-emerald-400' : 'text-rose-400'}>
            {errorRate.toFixed(1)}% err
          </span>
        </div>
      </div>

      {/* Metrik 2: Total Tokens */}
      <div className={selKelas}>
        <span className={labelKelas}>Total Token</span>
        <div className={nilaiKelas}>{formattedTokens}</div>
        <div className={subKelas}>
          In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out:{' '}
          {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
        </div>
      </div>

      {/* Metrik 3: Estimated Cost */}
      <div className={selKelas}>
        <span className={labelKelas}>Estimasi Biaya</span>
        <div className={nilaiKelas}>{formatUSD(summary?.total_cost_usd, 8)}</div>
        <div className={subKelas}>Kumulatif jendela</div>
      </div>

      {/* Metrik 4: P95 Latency */}
      <div className={selKelas}>
        <span className={labelKelas}>Latensi P95</span>
        <div className={nilaiKelas}>{`${Math.round(summary?.p95_latency_ms ?? 0)} ms`}</div>
        <div className={subKelas}>
          Rata-rata: {Math.round(summary?.avg_latency_ms ?? 0)} ms
        </div>
      </div>
    </div>
  );
};
