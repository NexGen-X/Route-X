import React from 'react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  type TooltipProps,
} from 'recharts';
import { BarChart3, Activity } from 'lucide-react';
import type { TimeSeriesPoint } from '../../types';

export interface TrafficChartProps {
  series: TimeSeriesPoint[];
  metricType: 'requests' | 'latency' | 'tokens';
  setMetricType: (m: 'requests' | 'latency' | 'tokens') => void;
  timeWindow: '1h' | '6h' | '24h' | '7d';
  setTimeWindow: (w: '1h' | '6h' | '24h' | '7d') => void;
}

interface MemoizedChartInnerProps {
  series: TimeSeriesPoint[];
  metricType: 'requests' | 'latency' | 'tokens';
}

const CustomDarkTooltip: React.FC<TooltipProps<number, string>> = ({ active, payload, label }) => {
  if (active && payload && payload.length) {
    const item = payload[0];
    const val = typeof item.value === 'number' ? item.value.toLocaleString() : item.value;
    const metricName = item.dataKey === 'p95_latency_ms' ? 'P95 Latency (ms)' : String(item.dataKey);

    return (
      <div className="bg-[#0F1523]/95 backdrop-blur-md border border-border/80 rounded-xl px-3 py-2 shadow-2xl text-xs font-mono ring-1 ring-white/10 space-y-1">
        <p className="text-[10px] text-text-muted">{label}</p>
        <div className="flex items-center gap-2">
          <span
            className="w-2 h-2 rounded-full bg-blue-500 shadow-[0_0_6px_rgba(59,130,246,0.9)]"
            aria-hidden="true"
          />
          <span className="text-white font-semibold tabular-nums">{val}</span>
          <span className="text-text-muted text-[10px] capitalize">{metricName}</span>
        </div>
      </div>
    );
  }
  return null;
};

const MemoizedChartInner = React.memo<MemoizedChartInnerProps>(({ series, metricType }) => (
  <ResponsiveContainer width="100%" height="100%">
    <AreaChart data={series} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
      <defs>
        {/* Neon Area Gradient */}
        <linearGradient id="neonTrafficGradient" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#3B82F6" stopOpacity={0.35} />
          <stop offset="50%" stopColor="#6366F1" stopOpacity={0.15} />
          <stop offset="100%" stopColor="#3B82F6" stopOpacity={0.0} />
        </linearGradient>
      </defs>
      <CartesianGrid strokeDasharray="3 3" stroke="#1E293B" vertical={false} opacity={0.6} />
      <XAxis dataKey="timestamp" stroke="#64748B" fontSize={11} tickLine={false} />
      <YAxis stroke="#64748B" fontSize={11} tickLine={false} />
      <Tooltip content={<CustomDarkTooltip />} />
      <Area
        type="monotone"
        dataKey={metricType === 'latency' ? 'p95_latency_ms' : metricType}
        stroke="#3B82F6"
        fillOpacity={1}
        fill="url(#neonTrafficGradient)"
        strokeWidth={2}
      />
    </AreaChart>
  </ResponsiveContainer>
));

MemoizedChartInner.displayName = 'MemoizedChartInner';

export const TrafficChart: React.FC<TrafficChartProps> = ({
  series,
  metricType,
  setMetricType,
  timeWindow,
  setTimeWindow,
}) => {
  return (
    <div className="lg:col-span-2 bg-bg-surface border border-border/80 rounded-card p-4 sm:p-5 shadow-sm flex flex-col justify-between">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3 sm:mb-4">
        <div className="flex items-center gap-2">
          <div className="p-1.5 rounded-lg bg-blue-500/10 text-blue-400 border border-blue-500/20">
            <BarChart3 className="w-4 h-4 flex-shrink-0" aria-hidden="true" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white tracking-tight truncate">
              Volume &amp; Dinamika Lalu Lintas
            </h3>
          </div>
        </div>

        <div
          role="group"
          aria-label="Pilih jenis metrik dan rentang waktu grafik"
          className="flex items-center overflow-x-auto max-w-full bg-bg-surface-2/80 p-1 rounded-xl border border-border/80 text-[11px] sm:text-xs font-mono scrollbar-none"
        >
          {/* Metric switcher */}
          {(['requests', 'tokens', 'latency'] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMetricType(m)}
              aria-pressed={metricType === m}
              aria-label={
                m === 'requests' ? 'Jumlah request' : m === 'tokens' ? 'Jumlah token' : 'Latensi'
              }
              className={`px-2.5 sm:px-3 py-1.5 min-h-[32px] rounded-lg capitalize transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-accent cursor-pointer ${
                metricType === m
                  ? 'bg-blue-600 text-white font-semibold shadow-sm shadow-blue-600/30'
                  : 'text-text-muted hover:text-white hover:bg-bg-surface-3/50'
              }`}
            >
              {m === 'requests' ? 'Req' : m === 'tokens' ? 'Token' : 'Latensi'}
            </button>
          ))}
          <span
            aria-hidden="true"
            className="w-px self-stretch my-1 mx-1 sm:mx-1.5 bg-border/80"
          />
          {/* Window switcher */}
          {(['1h', '6h', '24h', '7d'] as const).map((w) => (
            <button
              key={w}
              type="button"
              onClick={() => setTimeWindow(w)}
              aria-pressed={timeWindow === w}
              aria-label={`Rentang waktu ${w}`}
              className={`px-2 sm:px-2.5 py-1.5 min-h-[32px] rounded-lg transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-accent cursor-pointer ${
                timeWindow === w
                  ? 'bg-bg-surface-3 text-white font-medium border border-border/80'
                  : 'text-text-muted hover:text-white hover:bg-bg-surface-3/50'
              }`}
            >
              {w}
            </button>
          ))}
        </div>
      </div>

      <div className="h-52 sm:h-64 w-full pt-1 sm:pt-2">
        {series.length === 0 ? (
          <div className="h-full flex flex-col items-center justify-center text-xs text-text-muted space-y-2">
            <Activity className="w-6 h-6 text-text-muted" aria-hidden="true" />
            <span>Belum ada sampel metrik untuk rentang waktu {timeWindow}</span>
          </div>
        ) : (
          <MemoizedChartInner series={series} metricType={metricType} />
        )}
      </div>
    </div>
  );
};
