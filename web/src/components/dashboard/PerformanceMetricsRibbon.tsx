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
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
      <div className="bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl shadow-lg p-4 hover:ring-white/20 transition-all  flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-muted truncate">
            Total Permintaan
          </span>
          <div className="p-1.5 rounded-xl bg-bg-surface-2/60 backdrop-blur-sm ring-1 ring-white/5 border-transparent shrink-0">
            <Activity className="w-3.5 h-3.5 text-accent" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-base sm:text-lg font-bold text-white font-mono tracking-tight truncate">
            {(summary?.total_requests ?? 0).toLocaleString()}
          </div>
          <div className="flex flex-wrap items-center gap-1.5 mt-1">
            <span className="text-[11px] text-text-muted font-mono">
              {(summary?.success_requests ?? 0).toLocaleString()} ok
            </span>
            <span className="text-border text-[11px] hidden xs:inline">•</span>
            <span
              className={`text-[11px] font-mono font-medium ${
                (summary?.error_rate ?? 0) > 5 ? 'text-status-error' : 'text-status-success'
              }`}
            >
              {(summary?.error_rate ?? 0).toFixed(1)}% err
            </span>
          </div>
        </div>
      </div>

      <div className="bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl shadow-lg p-4 hover:ring-white/20 transition-all  flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-muted truncate">Total Token</span>
          <div className="p-1.5 rounded-xl bg-bg-surface-2/60 backdrop-blur-sm ring-1 ring-white/5 border-transparent shrink-0">
            <Zap className="w-3.5 h-3.5 text-status-success" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-base sm:text-lg font-bold text-white font-mono tracking-tight truncate">
            {(summary?.total_tokens ?? 0) >= 1000000
              ? `${((summary?.total_tokens ?? 0) / 1000000).toFixed(2)}M`
              : `${((summary?.total_tokens ?? 0) / 1000).toFixed(1)}k`}
          </div>
          <div className="text-[11px] text-text-muted mt-1 font-mono truncate">
            In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out:{' '}
            {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
          </div>
        </div>
      </div>

      <div className="bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl shadow-lg p-4 hover:ring-white/20 transition-all  flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-muted truncate">
            Estimasi Biaya
          </span>
          <div className="p-1.5 rounded-xl bg-bg-surface-2/60 backdrop-blur-sm ring-1 ring-white/5 border-transparent shrink-0">
            <Coins className="w-3.5 h-3.5 text-status-warn" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-base sm:text-lg font-bold text-white font-mono tracking-tight truncate">
            {formatUSD(summary?.total_cost_usd, 8)}
          </div>
          <div className="text-[11px] text-text-muted mt-1 font-mono truncate">
            Kumulatif jendela
          </div>
        </div>
      </div>

      <div className="bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl shadow-lg p-4 hover:ring-white/20 transition-all  flex flex-col justify-between">
        <div className="flex items-center justify-between gap-1">
          <span className="text-xs font-medium text-text-muted truncate">Latensi P95</span>
          <div className="p-1.5 rounded-xl bg-bg-surface-2/60 backdrop-blur-sm ring-1 ring-white/5 border-transparent shrink-0">
            <Clock className="w-3.5 h-3.5 text-status-info" />
          </div>
        </div>
        <div className="mt-3">
          <div className="text-base sm:text-lg font-bold text-white font-mono tracking-tight truncate">
            {summary?.p95_latency_ms ?? 0} ms
          </div>
          <div className="text-[11px] text-text-muted mt-1 font-mono truncate">
            Rata-rata: {summary?.avg_latency_ms ?? 0} ms
          </div>
        </div>
      </div>
    </div>
  );
};
