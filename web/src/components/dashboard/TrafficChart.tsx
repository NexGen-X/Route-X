import React, { useState } from 'react';
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
import { useObservabilitySeries, TimeWindow, MetricType } from './hooks';

export interface TrafficChartProps {
  timeWindow: TimeWindow;
  setTimeWindow: (w: TimeWindow) => void;
}

interface MemoizedChartInnerProps {
  series: TimeSeriesPoint[];
  metricType: MetricType;
}

const MemoizedChartInner = React.memo<MemoizedChartInnerProps>(({ series, metricType }) => (
  <ResponsiveContainer width="100%" aspect={2}>
    <AreaChart data={series}>
      <defs>
        <linearGradient id="chartGradient" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#BEF264" stopOpacity={0.16} />
          <stop offset="100%" stopColor="#BEF264" stopOpacity={0.0} />
        </linearGradient>
      </defs>
      <CartesianGrid stroke="rgba(255,255,255,0.02)" vertical={false} />
      <XAxis dataKey="timestamp" stroke="#71717A" fontSize={11} tickLine={false} axisLine={false} />
      <YAxis stroke="#71717A" fontSize={11} tickLine={false} axisLine={false} />
      <Tooltip
        cursor={{ stroke: 'rgba(255,255,255,0.08)', strokeWidth: 1, strokeDasharray: '3 3' }}
        contentStyle={{
          backgroundColor: 'rgba(17, 17, 19, 0.85)',
          borderColor: 'rgba(255, 255, 255, 0.08)',
          borderRadius: '10px',
          fontSize: '12px',
          color: '#F4F4F5',
          boxShadow: '0 8px 32px rgba(0, 0, 0, 0.4)',
          backdropFilter: 'blur(12px)',
          WebkitBackdropFilter: 'blur(12px)',
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
  timeWindow,
  setTimeWindow,
}) => {
  const [metricType, setMetricType] = useState<MetricType>('requests');
  const { data } = useObservabilitySeries(metricType, timeWindow);
  const series = data?.points || [];

  return (
    <div className="lg:col-span-2 bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] rounded-xl shadow-sm p-4 sm:p-5 flex flex-col justify-between min-w-0">
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
          className="flex items-center overflow-x-auto max-w-full bg-white/[0.03] border border-white/[0.06] p-0.5 sm:p-1 rounded-lg text-[11px] sm:text-xs font-mono scrollbar-none"
        >
          {(['requests', 'tokens', 'latency'] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMetricType(m as MetricType)}
              aria-pressed={metricType === m}
              aria-label={
                m === 'requests' ? 'Jumlah request' : m === 'tokens' ? 'Jumlah token' : 'Latensi'
              }
              className={`px-2 sm:px-3 py-1 sm:py-1.5 min-h-[28px] sm:min-h-[30px] rounded-md capitalize transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-accent cursor-pointer ${
                metricType === m
                  ? 'bg-white/10 text-white font-medium shadow-sm border border-white/[0.08]'
                  : 'text-zinc-400 hover:text-zinc-200'
              }`}
            >
              {m === 'requests' ? 'Req' : m === 'tokens' ? 'Token' : 'Latensi'}
            </button>
          ))}
          <span
            aria-hidden="true"
            className="w-px self-stretch my-1 mx-0.5 sm:mx-1 bg-white/[0.06]"
          />
          {(['1h', '6h', '24h', '7d'] as const).map((w) => (
            <button
              key={w}
              type="button"
              onClick={() => setTimeWindow(w as TimeWindow)}
              aria-pressed={timeWindow === w}
              aria-label={`Rentang waktu ${w}`}
              className={`px-2 sm:px-2.5 py-1 sm:py-1.5 min-h-[28px] sm:min-h-[30px] rounded-md transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-accent cursor-pointer ${
                timeWindow === w
                  ? 'bg-white/10 text-white font-medium border border-white/[0.08]'
                  : 'text-zinc-400 hover:text-zinc-200'
              }`}
            >
              {w}
            </button>
          ))}
        </div>
      </div>

      <div className="w-full min-w-0 pt-2 min-h-[16rem]">
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
