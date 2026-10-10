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
      <div className="bg-zinc-900/95 backdrop-blur-md border border-zinc-800 rounded-md px-3 py-2 shadow-lg text-xs font-mono space-y-1">
        <p className="text-[10px] text-zinc-500">{label}</p>
        <div className="flex items-center gap-2">
          <span className="w-1.5 h-1.5 rounded-full bg-indigo-400" aria-hidden="true" />
          <span className="text-white font-semibold tabular-nums">{val}</span>
          <span className="text-zinc-400 text-[10px] capitalize">{metricName}</span>
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
        {/* SOTA Electric Indigo Area Gradient */}
        <linearGradient id="electricIndigoTrafficGradient" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#6366F1" stopOpacity={0.20} />
          <stop offset="100%" stopColor="#6366F1" stopOpacity={0.0} />
        </linearGradient>
      </defs>
      <CartesianGrid strokeDasharray="3 3" stroke="#27272A" vertical={false} opacity={0.6} />
      <XAxis dataKey="timestamp" stroke="#71717A" fontSize={11} tickLine={false} />
      <YAxis stroke="#71717A" fontSize={11} tickLine={false} />
      <Tooltip content={<CustomDarkTooltip />} />
      <Area
        type="monotone"
        dataKey={metricType === 'latency' ? 'p95_latency_ms' : metricType}
        stroke="#6366F1"
        fillOpacity={1}
        fill="url(#electricIndigoTrafficGradient)"
        strokeWidth={1.8}
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
    <div className="lg:col-span-2 bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-4 sm:p-5 shadow-sm flex flex-col justify-between">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3 sm:mb-4">
        <div className="flex items-center gap-2">
          <BarChart3 className="w-4 h-4 text-indigo-400 flex-shrink-0" aria-hidden="true" />
          <h3 className="text-sm font-medium text-white tracking-tight truncate">
            Volume &amp; Dinamika Lalu Lintas
          </h3>
        </div>

        <div
          role="group"
          aria-label="Pilih jenis metrik dan rentang waktu grafik"
          className="flex items-center overflow-x-auto max-w-full bg-zinc-900 p-0.5 rounded-md border border-zinc-800 text-[11px] font-mono scrollbar-none"
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
              className={`px-2.5 py-1 rounded capitalize transition-colors focus:outline-none focus-visible:ring-1 focus-visible:ring-indigo-500/40 cursor-pointer ${
                metricType === m
                  ? 'bg-indigo-600 text-white font-medium shadow-sm'
                  : 'text-zinc-400 hover:text-white'
              }`}
            >
              {m === 'requests' ? 'Req' : m === 'tokens' ? 'Token' : 'Latensi'}
            </button>
          ))}
          <span
            aria-hidden="true"
            className="w-px self-stretch my-1 mx-1 bg-zinc-800"
          />
          {/* Window switcher */}
          {(['1h', '6h', '24h', '7d'] as const).map((w) => (
            <button
              key={w}
              type="button"
              onClick={() => setTimeWindow(w)}
              aria-pressed={timeWindow === w}
              aria-label={`Rentang waktu ${w}`}
              className={`px-2 py-1 rounded transition-colors focus:outline-none focus-visible:ring-1 focus-visible:ring-indigo-500/40 cursor-pointer ${
                timeWindow === w
                  ? 'bg-zinc-800 text-indigo-300 font-medium border border-indigo-500/30'
                  : 'text-zinc-400 hover:text-white'
              }`}
            >
              {w}
            </button>
          ))}
        </div>
      </div>

      <div className="h-52 sm:h-64 w-full pt-1 sm:pt-2">
        {series.length === 0 ? (
          <div className="h-full flex flex-col items-center justify-center text-xs text-zinc-500 space-y-2">
            <Activity className="w-5 h-5 text-zinc-600" aria-hidden="true" />
            <span>Belum ada sampel metrik untuk rentang waktu {timeWindow}</span>
          </div>
        ) : (
          <MemoizedChartInner series={series} metricType={metricType} />
        )}
      </div>
    </div>
  );
};
