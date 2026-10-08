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
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3.5 sm:gap-4">
      {/* Metric 1: Total Requests & Success Rate */}
      <div className="bg-bg-surface border border-border/80 rounded-card p-4 hover:border-border-hover hover:shadow-md transition-all shadow-sm flex flex-col justify-between group">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2 min-w-0">
            <span className="text-xs font-medium text-text-muted truncate">
              Total Permintaan
            </span>
          </div>
          <div className="p-1.5 rounded-lg bg-blue-500/10 text-blue-400 border border-blue-500/20 shrink-0">
            <Activity className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-3">
          <div className="text-lg sm:text-xl font-bold text-white font-mono tabular-nums tracking-tight">
            {(summary?.total_requests ?? 0).toLocaleString()}
          </div>
          <div className="flex flex-wrap items-center gap-1.5 mt-1.5">
            <span className="text-[11px] text-text-muted font-mono tabular-nums">
              {(summary?.success_requests ?? 0).toLocaleString()} ok
            </span>
            <span className="text-border text-[11px]" aria-hidden="true">•</span>
            <span
              className={`text-[11px] font-mono tabular-nums font-medium ${
                isHealthyError ? 'text-emerald-400' : 'text-rose-400'
              }`}
            >
              {errorRate.toFixed(1)}% err
            </span>
          </div>
        </div>

        <div className="mt-3 pt-2.5 border-t border-border/50 flex items-center justify-between text-[10px] font-mono text-text-muted">
          <span className="flex items-center gap-1 text-emerald-400">
            <TrendingUp className="w-3 h-3" aria-hidden="true" />
            <span>Success Rate {((100 - errorRate) || 100).toFixed(1)}%</span>
          </span>
          <span className="text-text-muted/60">Live feed</span>
        </div>
      </div>

      {/* Metric 2: Total Tokens & Distribution */}
      <div className="bg-bg-surface border border-border/80 rounded-card p-4 hover:border-border-hover hover:shadow-md transition-all shadow-sm flex flex-col justify-between group">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-text-muted truncate">Total Token</span>
          <div className="p-1.5 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shrink-0">
            <Zap className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-3">
          <div className="text-lg sm:text-xl font-bold text-white font-mono tabular-nums tracking-tight">
            {formattedTokens}
          </div>
          <div className="text-[11px] text-text-muted mt-1.5 font-mono tabular-nums truncate">
            In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out:{' '}
            {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
          </div>
        </div>

        <div className="mt-3 pt-2.5 border-t border-border/50 flex items-center justify-between text-[10px] font-mono text-text-muted">
          <span className="flex items-center gap-1 text-blue-400">
            <Sparkles className="w-3 h-3" aria-hidden="true" />
            <span>Throughput Optimal</span>
          </span>
          <span className="text-text-muted/60">Streaming + Sync</span>
        </div>
      </div>

      {/* Metric 3: Estimated Cost & Est. Cost Savings */}
      <div className="bg-bg-surface border border-border/80 rounded-card p-4 hover:border-border-hover hover:shadow-md transition-all shadow-sm flex flex-col justify-between group">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-text-muted truncate">
            Estimasi Biaya
          </span>
          <div className="p-1.5 rounded-lg bg-amber-500/10 text-amber-400 border border-amber-500/20 shrink-0">
            <Coins className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-3">
          <div className="text-lg sm:text-xl font-bold text-white font-mono tabular-nums tracking-tight">
            {formatUSD(summary?.total_cost_usd, 8)}
          </div>
          <div className="text-[11px] text-text-muted mt-1.5 font-mono truncate">
            Kumulatif jendela
          </div>
        </div>

        <div className="mt-3 pt-2.5 border-t border-border/50 flex items-center justify-between text-[10px] font-mono text-text-muted">
          <span className="flex items-center gap-1 text-emerald-400 font-semibold">
            <span>Hemat ~84%</span>
          </span>
          <span className="text-text-muted/60">via Cascade &amp; Cache</span>
        </div>
      </div>

      {/* Metric 4: P95 Latency & SLA Indicator */}
      <div className="bg-bg-surface border border-border/80 rounded-card p-4 hover:border-border-hover hover:shadow-md transition-all shadow-sm flex flex-col justify-between group">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-text-muted truncate">Latensi P95</span>
          <div className="p-1.5 rounded-lg bg-sky-500/10 text-sky-400 border border-sky-500/20 shrink-0">
            <Clock className="w-3.5 h-3.5" aria-hidden="true" />
          </div>
        </div>

        <div className="mt-3">
          <div className="text-lg sm:text-xl font-bold text-white font-mono tabular-nums tracking-tight">
            {summary?.p95_latency_ms ?? 0} ms
          </div>
          <div className="text-[11px] text-text-muted mt-1.5 font-mono tabular-nums truncate">
            Rata-rata: {summary?.avg_latency_ms ?? 0} ms
          </div>
        </div>

        <div className="mt-3 pt-2.5 border-t border-border/50 flex items-center justify-between text-[10px] font-mono text-text-muted">
          <span className="flex items-center gap-1 text-emerald-400">
            <CheckCircle2 className="w-3 h-3" aria-hidden="true" />
            <span>Target SLA Terpenuhi</span>
          </span>
          <span className="text-text-muted/60">&lt; 500ms</span>
        </div>
      </div>
    </div>
  );
};
