import React from 'react';
import { Activity, Zap, Coins, Clock } from 'lucide-react';
import type { ObservabilitySummary } from '../../types';
import { formatUSD } from '../../utils/money';

export interface PerformanceMetricsRibbonProps {
  summary: ObservabilitySummary | null;
}

export const PerformanceMetricsRibbon: React.FC<PerformanceMetricsRibbonProps> = ({ summary }) => {
  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 sm:gap-5">
      {/* Metric 1: Total Requests */}
      <div className="bg-bg-surface border border-border rounded-card p-4 sm:p-5 hover:border-border/80 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">
            Total Permintaan
          </span>
          <div className="p-1.5 sm:p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border shrink-0">
            <Activity className="w-3.5 h-3.5 sm:w-4 sm:h-4 text-accent" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {(summary?.total_requests ?? 0).toLocaleString()}
          </div>
          <div className="flex flex-wrap items-center gap-1.5 mt-1.5">
            <span className="text-[10px] sm:text-xs text-text-muted font-mono">
              {(summary?.success_requests ?? 0).toLocaleString()} ok
            </span>
            <span className="text-border text-[10px] sm:text-xs hidden xs:inline">•</span>
            <span
              className={`text-[10px] sm:text-xs font-mono font-medium ${
                (summary?.error_rate ?? 0) > 5 ? 'text-rose-400' : 'text-emerald-400'
              }`}
            >
              {(summary?.error_rate ?? 0).toFixed(1)}% err
            </span>
          </div>
        </div>
      </div>

      {/* Metric 2: Total Tokens */}
      <div className="bg-bg-surface border border-border rounded-card p-4 sm:p-5 hover:border-border/80 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">Total Token</span>
          <div className="p-1.5 sm:p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border shrink-0">
            <Zap className="w-3.5 h-3.5 sm:w-4 sm:h-4 text-emerald-400" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {(summary?.total_tokens ?? 0) >= 1000000
              ? `${((summary?.total_tokens ?? 0) / 1000000).toFixed(2)}M`
              : `${((summary?.total_tokens ?? 0) / 1000).toFixed(1)}k`}
          </div>
          <div className="text-[10px] sm:text-xs text-text-muted mt-1.5 font-mono truncate">
            In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out:{' '}
            {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
          </div>
        </div>
      </div>

      {/* Metric 3: Estimated Cost */}
      <div className="bg-bg-surface border border-border rounded-card p-4 sm:p-5 hover:border-border/80 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">
            Estimasi Biaya
          </span>
          <div className="p-1.5 sm:p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border shrink-0">
            <Coins className="w-3.5 h-3.5 sm:w-4 sm:h-4 text-amber-400" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {formatUSD(summary?.total_cost_usd, 8)}
          </div>
          <div className="text-[10px] sm:text-xs text-text-muted mt-1.5 font-mono truncate">
            Kumulatif jendela
          </div>
        </div>
      </div>

      {/* Metric 4: P95 Latency */}
      <div className="bg-bg-surface border border-border rounded-card p-4 sm:p-5 hover:border-border/80 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">Latensi P95</span>
          <div className="p-1.5 sm:p-2 rounded-inner bg-bg-surface-2 text-text-secondary border border-border shrink-0">
            <Clock className="w-3.5 h-3.5 sm:w-4 sm:h-4 text-purple-400" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {summary?.p95_latency_ms ?? 0} ms
          </div>
          <div className="text-[10px] sm:text-xs text-text-muted mt-1.5 font-mono truncate">
            Rata-rata: {summary?.avg_latency_ms ?? 0} ms
          </div>
        </div>
      </div>
    </div>
  );
};
