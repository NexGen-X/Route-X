import React from 'react';
import { Activity, Zap, Coins, Clock } from 'lucide-react';
import type { ObservabilitySummary } from '../../types';
import { formatUSD } from '../../utils/money';

export interface PerformanceMetricsRibbonProps {
  summary: ObservabilitySummary | null;
}

export const PerformanceMetricsRibbon: React.FC<PerformanceMetricsRibbonProps> = ({ summary }) => {
  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-2.5 sm:gap-4">
      {/* Metric 1: Total Requests */}
      <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-primary/40 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">
            Total Permintaan
          </span>
          <div className="p-1.5 sm:p-2 rounded-lg bg-primary/10 text-primary border border-primary/20 shrink-0">
            <Activity className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          </div>
        </div>
        <div className="mt-2 sm:mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {(summary?.total_requests ?? 0).toLocaleString()}
          </div>
          <div className="flex flex-wrap items-center gap-1 sm:gap-2 mt-1 sm:mt-1.5">
            <span className="text-[10px] sm:text-xs text-text-muted font-mono">
              {(summary?.success_requests ?? 0).toLocaleString()} ok
            </span>
            <span className="text-[#3A404F] text-[10px] sm:text-xs hidden xs:inline">•</span>
            <span
              className={`text-[10px] sm:text-xs font-mono font-semibold ${
                (summary?.error_rate ?? 0) > 5 ? 'text-rose-400' : 'text-emerald-400'
              }`}
            >
              {(summary?.error_rate ?? 0).toFixed(1)}% err
            </span>
          </div>
        </div>
      </div>

      {/* Metric 2: Total Tokens */}
      <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-emerald-500/40 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">Total Token</span>
          <div className="p-1.5 sm:p-2 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shrink-0">
            <Zap className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          </div>
        </div>
        <div className="mt-2 sm:mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {(summary?.total_tokens ?? 0) >= 1000000
              ? `${((summary?.total_tokens ?? 0) / 1000000).toFixed(2)}M`
              : `${((summary?.total_tokens ?? 0) / 1000).toFixed(1)}k`}
          </div>
          <div className="text-[10px] sm:text-xs text-text-muted mt-1 sm:mt-1.5 font-mono truncate">
            In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out:{' '}
            {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
          </div>
        </div>
      </div>

      {/* Metric 3: Estimated Cost */}
      <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-amber-500/40 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">
            Estimasi Biaya
          </span>
          <div className="p-1.5 sm:p-2 rounded-lg bg-amber-500/10 text-amber-400 border border-amber-500/20 shrink-0">
            <Coins className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          </div>
        </div>
        <div className="mt-2 sm:mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {formatUSD(summary?.total_cost_usd, 8)}
          </div>
          <div className="text-[10px] sm:text-xs text-text-muted mt-1 sm:mt-1.5 font-mono truncate">
            Presisi 8 desimal USD
          </div>
        </div>
      </div>

      {/* Metric 4: P95 Latency */}
      <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-purple-500/40 transition-all shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-secondary truncate">Latensi P95</span>
          <div className="p-1.5 sm:p-2 rounded-lg bg-purple-500/10 text-purple-400 border border-purple-500/20 shrink-0">
            <Clock className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
          </div>
        </div>
        <div className="mt-2 sm:mt-3">
          <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
            {summary?.p95_latency_ms ?? 0} ms
          </div>
          <div className="text-[10px] sm:text-xs text-text-muted mt-1 sm:mt-1.5 font-mono truncate">
            Rata-rata: {summary?.avg_latency_ms ?? 0} ms
          </div>
        </div>
      </div>
    </div>
  );
};
