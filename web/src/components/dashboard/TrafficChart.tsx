import React from 'react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
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

const MemoizedChartInner = React.memo<MemoizedChartInnerProps>(({ series, metricType }) => (
  <ResponsiveContainer width="100%" height="100%">
    <AreaChart data={series}>
      <defs>
        <linearGradient id="chartGradient" x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor="#BEF264" stopOpacity={0.12} />
          <stop offset="95%" stopColor="#BEF264" stopOpacity={0.0} />
        </linearGradient>
      </defs>
      <CartesianGrid strokeDasharray="3 3" stroke="#1F1F1F" vertical={false} />
      <XAxis dataKey="timestamp" stroke="#6B7280" fontSize={11} tickLine={false} />
      <YAxis stroke="#6B7280" fontSize={11} tickLine={false} />
      <Tooltip
        contentStyle={{
          backgroundColor: '#141414',
          borderColor: '#1F1F1F',
          borderRadius: '10px',
          fontSize: '12px',
          color: '#F5F5F5',
          boxShadow: '0 10px 25px -5px rgba(0, 0, 0, 0.5)',
        }}
      />
      <Area
        type="monotone"
        dataKey={metricType === 'latency' ? 'p95_latency_ms' : metricType}
        stroke="#BEF264"
        fillOpacity={1}
        fill="url(#chartGradient)"
        strokeWidth={1.5}
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
    <div className="lg:col-span-2 bg-bg-surface border border-border rounded-card p-4 sm:p-5 shadow-sm flex flex-col justify-between">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-3 mb-3 sm:mb-4">
        <div className="flex items-center gap-2">
          <BarChart3 className="w-4 h-4 text-accent flex-shrink-0" />
          <h3 className="text-sm font-semibold text-white tracking-tight truncate">
            Volume &amp; Dinamika Lalu Lintas
          </h3>
        </div>

        <div
          role="group"
          aria-label="Pilih jenis metrik dan rentang waktu grafik"
          className="flex items-center overflow-x-auto max-w-full bg-bg-surface-2 p-0.5 sm:p-1 rounded-nav border border-border text-[11px] sm:text-xs font-mono scrollbar-none"
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
              className={`px-2 sm:px-3 py-1 sm:py-1.5 min-h-[28px] sm:min-h-[30px] rounded-md capitalize transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-accent cursor-pointer ${
                metricType === m
                  ? 'bg-zinc-200 text-black font-semibold shadow'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              {m === 'requests' ? 'Req' : m === 'tokens' ? 'Token' : 'Latensi'}
            </button>
          ))}
          <span
            aria-hidden="true"
            className="w-px self-stretch my-1 mx-0.5 sm:mx-1 bg-border"
          />
          {/* Window switcher */}
          {(['1h', '6h', '24h', '7d'] as const).map((w) => (
            <button
              key={w}
              type="button"
              onClick={() => setTimeWindow(w)}
              aria-pressed={timeWindow === w}
              aria-label={`Rentang waktu ${w}`}
              className={`px-2 sm:px-2.5 py-1 sm:py-1.5 min-h-[28px] sm:min-h-[30px] rounded-md transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-accent cursor-pointer ${
                timeWindow === w
                  ? 'bg-bg-surface-3 text-white font-medium'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              {w}
            </button>
          ))}
        </div>
      </div>

      <div className="h-56 sm:h-64 w-full pt-1 sm:pt-2">
        {series.length === 0 ? (
          <div className="h-full flex flex-col items-center justify-center text-xs text-text-muted space-y-2">
            <Activity className="w-6 h-6 text-text-muted" />
            <span>Belum ada sampel metrik untuk rentang waktu {timeWindow}</span>
          </div>
        ) : (
          <MemoizedChartInner series={series} metricType={metricType} />
        )}
      </div>
    </div>
  );
};
