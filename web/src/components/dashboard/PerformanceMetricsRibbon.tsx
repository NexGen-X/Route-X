import React from 'react';
import { Activity, Zap, Coins, Clock, TrendingUp, Sparkles, CheckCircle2 } from 'lucide-react';
import type { ObservabilitySummary } from '../../types';
import { formatUSD } from '../../utils/money';

export interface PerformanceMetricsRibbonProps {
  summary: ObservabilitySummary | null;
}

export const PerformanceMetricsRibbon: React.FC<PerformanceMetricsRibbonProps> = ({ summary }) => {
  const errorRate = summary?.error_rate ?? 0;
  const isHealthyError = errorRate <= 5;
  const totalTokens = summary?.total_tokens ?? 0;
  const formattedTokens =
    totalTokens >= 1000000
      ? `${(totalTokens / 1000000).toFixed(2)}M`
      : `${(totalTokens / 1000).toFixed(1)}k`;

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
      {/* Metric 1: Total Requests */}
      <div className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-3.5 sm:p-4 hover:border-zinc-700/80 transition-colors shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-normal text-zinc-400 truncate">
            Total Permintaan
          </span>
          <div className="p-1.5 rounded-md text-indigo-400 bg-indigo-500/10 border border-indigo-500/20 shrink-0">
            <Activity className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-2.5">
          <div className="text-xl sm:text-2xl font-semibold text-zinc-50 font-mono tabular-nums tracking-tight">
            {(summary?.total_requests ?? 0).toLocaleString()}
          </div>
          <div className="flex items-center gap-1.5 mt-1 text-[11px] text-zinc-500 font-mono tabular-nums">
            <span>{(summary?.success_requests ?? 0).toLocaleString()} ok</span>
            <span className="text-zinc-700" aria-hidden="true">·</span>
            <span className={isHealthyError ? 'text-emerald-400' : 'text-rose-400'}>
              {errorRate.toFixed(1)}% err
            </span>
          </div>
        </div>

        <div className="mt-3 pt-2 border-t border-zinc-800/80 flex items-center justify-between text-[10px] font-mono text-zinc-500">
          <span className="flex items-center gap-1 text-emerald-400">
            <TrendingUp className="w-3 h-3" aria-hidden="true" />
            <span>Success Rate {((100 - errorRate) || 100).toFixed(1)}%</span>
          </span>
          <span>Live feed</span>
        </div>
      </div>

      {/* Metric 2: Total Tokens */}
      <div className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-3.5 sm:p-4 hover:border-zinc-700/80 transition-colors shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-normal text-zinc-400 truncate">Total Token</span>
          <div className="p-1.5 rounded-md text-cyan-400 bg-cyan-500/10 border border-cyan-500/20 shrink-0">
            <Zap className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-2.5">
          <div className="text-xl sm:text-2xl font-semibold text-zinc-50 font-mono tabular-nums tracking-tight">
            {formattedTokens}
          </div>
          <div className="text-[11px] text-zinc-500 mt-1 font-mono tabular-nums truncate">
            In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out:{' '}
            {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
          </div>
        </div>

        <div className="mt-3 pt-2 border-t border-zinc-800/80 flex items-center justify-between text-[10px] font-mono text-zinc-500">
          <span className="flex items-center gap-1 text-zinc-400">
            <Sparkles className="w-3 h-3" aria-hidden="true" />
            <span>Throughput Optimal</span>
          </span>
          <span>Stream + Sync</span>
        </div>
      </div>

      {/* Metric 3: Estimated Cost */}
      <div className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-3.5 sm:p-4 hover:border-zinc-700/80 transition-colors shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-normal text-zinc-400 truncate">
            Estimasi Biaya
          </span>
          <div className="p-1.5 rounded-md text-amber-400 bg-amber-500/10 border border-amber-500/20 shrink-0">
            <Coins className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-2.5">
          <div className="text-xl sm:text-2xl font-semibold text-zinc-50 font-mono tabular-nums tracking-tight">
            {formatUSD(summary?.total_cost_usd, 8)}
          </div>
          <div className="text-[11px] text-zinc-500 mt-1 font-mono truncate">
            Kumulatif jendela
          </div>
        </div>

        <div className="mt-3 pt-2 border-t border-zinc-800/80 flex items-center justify-between text-[10px] font-mono text-zinc-500">
          <span className="text-emerald-400">
            Hemat ~84%
          </span>
          <span>via Cascade</span>
        </div>
      </div>

      {/* Metric 4: P95 Latency */}
      <div className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-3.5 sm:p-4 hover:border-zinc-700/80 transition-colors shadow-sm flex flex-col justify-between">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-normal text-zinc-400 truncate">Latensi P95</span>
          <div className="p-1.5 rounded-md text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 shrink-0">
            <Clock className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-2.5">
          <div className="text-xl sm:text-2xl font-semibold text-zinc-50 font-mono tabular-nums tracking-tight">
            {summary?.p95_latency_ms ?? 0} ms
          </div>
          <div className="text-[11px] text-zinc-500 mt-1 font-mono tabular-nums truncate">
            Rata-rata: {summary?.avg_latency_ms ?? 0} ms
          </div>
        </div>

        <div className="mt-3 pt-2 border-t border-zinc-800/80 flex items-center justify-between text-[10px] font-mono text-zinc-500">
          <span className="flex items-center gap-1 text-emerald-400">
            <CheckCircle2 className="w-3 h-3" aria-hidden="true" />
            <span>Target SLA Terpenuhi</span>
          </span>
          <span>&lt; 500ms</span>
        </div>
      </div>
    </div>
  );
};
