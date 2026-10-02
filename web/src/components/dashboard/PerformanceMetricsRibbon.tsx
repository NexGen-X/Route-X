import React from 'react';
import { Activity, Zap, Coins, Clock } from 'lucide-react';
import { formatUSD } from '../../utils/money';
import { useObservabilitySummary, TimeWindow } from './hooks';

export interface PerformanceMetricsRibbonProps {
  timeWindow: TimeWindow;
}

export const PerformanceMetricsRibbon: React.FC<PerformanceMetricsRibbonProps> = ({ timeWindow }) => {
  const { data: summary } = useObservabilitySummary(timeWindow);

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
      <div className="bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] hover:border-white/[0.12] rounded-xl p-4 transition-all duration-200 flex flex-col justify-between shadow-sm">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-zinc-400 truncate">
            Total Permintaan
          </span>
          <div className="p-1.5 rounded-lg bg-white/[0.03] border border-white/[0.06] shrink-0">
            <Activity className="w-3.5 h-3.5 text-accent" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-2xl font-semibold tracking-tight text-white font-mono truncate">
            {(summary?.total_requests ?? 0).toLocaleString()}
          </div>
          <div className="flex flex-wrap items-center gap-1.5 mt-1">
            <span className="text-[11px] text-zinc-500 font-mono">
              {(summary?.success_requests ?? 0).toLocaleString()} ok
            </span>
            <span className="text-white/10 text-[11px] hidden xs:inline">•</span>
            <span
              className={`text-[11px] font-mono font-medium ${
                (summary?.error_rate ?? 0) > 5 ? 'text-rose-400' : 'text-emerald-400'
              }`}
            >
              {(summary?.error_rate ?? 0).toFixed(1)}% err
            </span>
          </div>
        </div>
      </div>

      <div className="bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] hover:border-white/[0.12] rounded-xl p-4 transition-all duration-200 flex flex-col justify-between shadow-sm">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-zinc-400 truncate">Total Token</span>
          <div className="p-1.5 rounded-lg bg-white/[0.03] border border-white/[0.06] shrink-0">
            <Zap className="w-3.5 h-3.5 text-emerald-400" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-2xl font-semibold tracking-tight text-white font-mono truncate">
            {(summary?.total_tokens ?? 0) >= 1000000
              ? `${((summary?.total_tokens ?? 0) / 1000000).toFixed(2)}M`
              : `${((summary?.total_tokens ?? 0) / 1000).toFixed(1)}k`}
          </div>
          <div className="text-[11px] text-zinc-500 mt-1 font-mono truncate">
            In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out:{' '}
            {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
          </div>
        </div>
      </div>

      <div className="bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] hover:border-white/[0.12] rounded-xl p-4 transition-all duration-200 flex flex-col justify-between shadow-sm">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-zinc-400 truncate">
            Estimasi Biaya
          </span>
          <div className="p-1.5 rounded-lg bg-white/[0.03] border border-white/[0.06] shrink-0">
            <Coins className="w-3.5 h-3.5 text-amber-400" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-2xl font-semibold tracking-tight text-white font-mono truncate">
            {formatUSD(summary?.total_cost_usd, 8)}
          </div>
          <div className="text-[11px] text-zinc-500 mt-1 font-mono truncate">
            Kumulatif jendela
          </div>
        </div>
      </div>

      <div className="bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] hover:border-white/[0.12] rounded-xl p-4 transition-all duration-200 flex flex-col justify-between shadow-sm">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-zinc-400 truncate">Latensi P95</span>
          <div className="p-1.5 rounded-lg bg-white/[0.03] border border-white/[0.06] shrink-0">
            <Clock className="w-3.5 h-3.5 text-sky-400" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-2xl font-semibold tracking-tight text-white font-mono truncate">
            {summary?.p95_latency_ms ?? 0} ms
          </div>
          <div className="text-[11px] text-zinc-500 mt-1 font-mono truncate">
            Rata-rata: {summary?.avg_latency_ms ?? 0} ms
          </div>
        </div>
      </div>
    </div>
  );
};
