import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { TimeSeriesPoint, BreakdownItem, Diagnostics, ObservabilitySummary } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from 'recharts';
import { RefreshCw, Database, Activity, Coins, Clock } from 'lucide-react';
import { QueryError } from '../components/common/QueryError';
import { formatUSD } from '../utils/money';

export const Observability: React.FC = () => {
  const [windowTime, setWindowTime] = useState('24h');
  const [metric, setMetric] = useState('requests');
  const [series, setSeries] = useState<TimeSeriesPoint[]>([]);
  const [breakdowns, setBreakdowns] = useState<BreakdownItem[]>([]);
  const [breakdownBy, setBreakdownBy] = useState<'provider' | 'model' | 'api_key'>('provider');
  const [summary, setSummary] = useState<ObservabilitySummary | null>(null);
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [diagnosticsError, setDiagnosticsError] = useState<string | null>(null);

  const formatXAxis = (tickItem: string) => {
    try {
      if (!tickItem) return '';
      const d = new Date(tickItem);
      if (isNaN(d.getTime())) return tickItem;
      if (windowTime === '1h' || windowTime === '24h') {
        return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
      }
      return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
    } catch {
      return tickItem;
    }
  };

  const formatYAxis = (val: number) => {
    if (metric === 'cost') return `$${val}`;
    if (metric === 'latency') return `${val}ms`;
    if (val >= 1000000) return `${(val / 1000000).toFixed(1)}M`;
    if (val >= 1000) return `${(val / 1000).toFixed(0)}k`;
    return String(val);
  };

  const formatTooltipValue = (value: any) => {
    const num = Number(value);
    if (!Number.isFinite(num)) return value;
    if (metric === 'cost') return [`$${num.toFixed(4)}`, 'Biaya USD'];
    if (metric === 'latency') return [`${num.toFixed(1)} ms`, 'Latensi P95'];
    if (metric === 'tokens') return [`${num.toLocaleString()}`, 'Total Token'];
    return [`${num.toLocaleString()} reqs`, 'Permintaan'];
  };

  const formatTooltipLabel = (label: string) => {
    try {
      if (!label) return '';
      const d = new Date(label);
      if (isNaN(d.getTime())) return label;
      return d.toLocaleString([], {
        dateStyle: 'medium',
        timeStyle: 'short',
      });
    } catch {
      return label;
    }
  };

  const loadData = async () => {
    setIsLoading(true);
    try {
      const [sRes, bRes, sumRes] = await Promise.all([
        api.observability.series(metric, windowTime),
        api.observability.breakdown(breakdownBy, windowTime),
        api.observability.summary(windowTime).catch(() => null),
      ]);
      setSeries(sRes.points || []);
      setBreakdowns(bRes.items || []);
      setSummary(sumRes);
      setLoadError(null);

      try {
        const dRes = await api.system.diagnostics();
        setDiag(dRes);
        setDiagnosticsError(null);
      } catch (err) {
        setDiag(null);
        setDiagnosticsError(err instanceof Error ? err.message : String(err));
      }
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
    }
  };

  const totalRequestsFromSeries = series.reduce((acc, p) => acc + (p.requests || 0), 0);
  const totalCostFromSeries = series.reduce((acc, p) => acc + (Number(p.cost_usd) || 0), 0).toFixed(4);
  const totalTokensFromSeries = series.reduce((acc, p) => acc + (p.tokens || 0), 0);
  const avgLatencyFromSeries = series.length > 0 ? series.reduce((acc, p) => acc + (p.p50_latency_ms || 0), 0) / series.length : 0;
  const p95LatencyFromSeries = series.length > 0 ? Math.max(...series.map((p) => p.p95_latency_ms || 0)) : 0;
  const errorRate = summary?.error_rate ?? (totalRequestsFromSeries > 0 ? (series.reduce((acc, p) => acc + (p.errors || 0), 0) / totalRequestsFromSeries) * 100 : 0);

  useEffect(() => {
    loadData();
  }, [windowTime, metric, breakdownBy]);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Observabilitas & Telemetri"
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex items-center gap-1 bg-bg-surface-2 p-1 rounded-nav border border-border" role="group" aria-label="Pilihan rentang waktu telemetri">
              {['1h', '24h', '7d', '30d'].map((w) => (
                <button
                  key={w}
                  type="button"
                  aria-pressed={windowTime === w}
                  aria-label={`Rentang waktu ${w}`}
                  onClick={() => setWindowTime(w)}
                  className={`px-2.5 py-1 rounded-inner text-xs font-semibold transition-colors cursor-pointer ${
                    windowTime === w
                      ? 'bg-accent text-black font-bold shadow-sm'
                      : 'text-text-secondary hover:text-white'
                  }`}
                >
                  {w}
                </button>
              ))}
            </div>
            <Button
              variant="secondary"
              size="sm"
              onClick={loadData}
              isLoading={isLoading}
              aria-label="Segarkan data observabilitas"
            >
              <RefreshCw className="w-3.5 h-3.5" aria-hidden="true" />
            </Button>
          </div>
        }
      />

      {loadError && <QueryError message={loadError} onRetry={() => void loadData()} />}

      {/* 3 Kartu Ringkasan KPI Eksekutif */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* KPI 1: Total Permintaan */}
        <Card className="border-border bg-bg-surface">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <span className="text-[11px] font-medium text-text-muted">Total Permintaan ({windowTime})</span>
              <div className="text-2xl font-bold text-white tracking-tight">
                {(summary?.total_requests ?? totalRequestsFromSeries).toLocaleString()}
              </div>
              <div className="text-[10px] text-text-secondary flex items-center gap-1.5">
                <span className={`w-1.5 h-1.5 rounded-full ${errorRate > 5 ? 'bg-red-400' : 'bg-emerald-400'}`} />
                <span>Error Rate: {errorRate.toFixed(1)}%</span>
              </div>
            </div>
            <div className="w-10 h-10 rounded-xl bg-accent/10 border border-accent/20 flex items-center justify-center text-accent">
              <Activity className="w-5 h-5" />
            </div>
          </div>
        </Card>

        {/* KPI 2: Estimasi Biaya USD */}
        <Card className="border-border bg-bg-surface">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <span className="text-[11px] font-medium text-text-muted">Estimasi Biaya ({windowTime})</span>
              <div className="text-2xl font-bold text-white tracking-tight">
                {formatUSD(summary?.total_cost_usd || totalCostFromSeries)}
              </div>
              <div className="text-[10px] text-text-secondary">
                Total Token: {(summary?.total_tokens ?? totalTokensFromSeries).toLocaleString()}
              </div>
            </div>
            <div className="w-10 h-10 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
              <Coins className="w-5 h-5" />
            </div>
          </div>
        </Card>

        {/* KPI 3: Rata-rata Latensi */}
        <Card className="border-border bg-bg-surface">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <span className="text-[11px] font-medium text-text-muted">Rata-rata Latensi</span>
              <div className="text-2xl font-bold text-white tracking-tight">
                {(summary?.avg_latency_ms ?? avgLatencyFromSeries).toFixed(0)} ms
              </div>
              <div className="text-[10px] text-text-secondary">
                P95 Latensi: {(summary?.p95_latency_ms ?? p95LatencyFromSeries).toFixed(0)} ms
              </div>
            </div>
            <div className="w-10 h-10 rounded-xl bg-blue-500/10 border border-blue-500/20 flex items-center justify-center text-blue-400">
              <Clock className="w-5 h-5" />
            </div>
          </div>
        </Card>
      </div>

      {/* Metric Selector & Main Time Series Chart */}
      <Card
        title="Deret Waktu Telemetri"
        action={
          <div className="grid grid-cols-2 sm:flex sm:items-center gap-1 bg-bg-surface-2 p-1 rounded-nav border border-border max-w-full" role="group" aria-label="Pilihan metrik deret waktu">
            {[
              { key: 'requests', label: 'Requests' },
              { key: 'tokens', label: 'Tokens' },
              { key: 'cost', label: 'Cost USD' },
              { key: 'latency', label: 'Latency P95' },
            ].map((m) => (
              <button
                key={m.key}
                type="button"
                aria-pressed={metric === m.key}
                aria-label={`Metrik ${m.label}`}
                onClick={() => setMetric(m.key)}
                className={`px-2 sm:px-2.5 py-1 text-[11px] sm:text-xs rounded-inner transition-colors font-medium whitespace-nowrap text-center cursor-pointer ${
                  metric === m.key
                    ? 'bg-bg-surface text-accent font-semibold shadow-sm'
                    : 'text-text-muted hover:text-text-primary'
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>
        }
      >
        <div className="h-72 w-full pt-4">
          {isLoading && series.length === 0 ? (
            <div className="h-full flex items-center justify-center">
              <div className="animate-pulse flex flex-col items-center gap-2.5 text-text-muted text-xs">
                <RefreshCw className="w-5 h-5 animate-spin text-accent" />
                <span>Memuat data telemetri...</span>
              </div>
            </div>
          ) : series.length === 0 ? (
            <div className="h-full flex items-center justify-center text-xs text-text-muted">
              Tidak ada data deret waktu untuk rentang yang dipilih.
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              {/* TrafficPointDTO.cost_usd dikirim backend sebagai string presisi desimal
                  (internal/admin/dto.go:137); recharts hanya bisa memplot angka, jadi
                  konversi ke number per titik sebelum masuk chart. */}
              <AreaChart data={series.map((p) => ({ ...p, cost_usd: Number(p.cost_usd ?? 0) }))}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1F1F1F" />
                <XAxis
                  dataKey="timestamp"
                  stroke="#6B7280"
                  fontSize={11}
                  tickLine={false}
                  tickFormatter={formatXAxis}
                />
                <YAxis
                  stroke="#6B7280"
                  fontSize={11}
                  tickLine={false}
                  tickFormatter={formatYAxis}
                />
                <Tooltip
                  formatter={formatTooltipValue}
                  labelFormatter={formatTooltipLabel}
                  contentStyle={{
                    backgroundColor: '#101010',
                    borderColor: '#1F1F1F',
                    borderRadius: '8px',
                    fontSize: '12px',
                    color: '#F5F5F5',
                  }}
                />
                <Area
                  type="monotone"
                  dataKey={
                    metric === 'cost'
                      ? 'cost_usd'
                      : metric === 'latency'
                      ? 'p95_latency_ms'
                      : metric
                  }
                  stroke="#BEF264"
                  fill="#BEF264"
                  fillOpacity={0.15}
                  strokeWidth={2}
                />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>
      </Card>

      {/* Breakdown Composition & Runtime Stats */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card
          className="lg:col-span-2"
          title="Komposisi Lalu Lintas"
          action={
            <div className="flex items-center gap-1 bg-bg-surface-2 p-1 rounded-nav border border-border max-w-full">
              {(['provider', 'model', 'api_key'] as const).map((b) => (
                <button
                  key={b}
                  onClick={() => setBreakdownBy(b)}
                  className={`px-2 py-0.5 text-xs rounded-inner uppercase font-mono whitespace-nowrap text-center ${
                    breakdownBy === b ? 'bg-accent text-black font-bold' : 'text-text-muted hover:text-white'
                  }`}
                >
                  {b.replace('_', ' ')}
                </button>
              ))}
            </div>
          }
        >
          <div className="space-y-3 mt-2">
            {isLoading && breakdowns.length === 0 ? (
              <div className="space-y-3 py-2 animate-pulse">
                {[1, 2, 3].map((i) => (
                  <div key={i} className="space-y-1.5">
                    <div className="flex justify-between">
                      <div className="h-3.5 bg-bg-surface-2 rounded w-24" />
                      <div className="h-3.5 bg-bg-surface-2 rounded w-16" />
                    </div>
                    <div className="w-full h-2 bg-bg-surface-2 rounded-full" />
                  </div>
                ))}
              </div>
            ) : breakdowns.length === 0 ? (
              <p className="text-xs text-text-muted py-6 text-center">Belum ada data breakdown untuk kategori ini.</p>
            ) : (
              breakdowns.map((item) => (
                <div key={item.id} className="space-y-1">
                  <div className="flex justify-between text-xs">
                    <span className="font-semibold text-text-primary">{item.name || item.id}</span>
                    <span className="font-mono text-text-secondary">
                      {(item.requests ?? 0).toLocaleString()} reqs ({(item.percentage ?? 0).toFixed(1)}%)
                    </span>
                  </div>
                  <div className="w-full h-2 bg-bg-surface-2 rounded-full overflow-hidden">
                    <div
                      className="h-full bg-accent transition-all duration-500 rounded-full"
                      style={{ width: `${Math.min(100, Math.max(2, item.percentage ?? 0))}%` }}
                    />
                  </div>
                </div>
              ))
            )}
          </div>
        </Card>

        {/* Live Pool & Diagnostics */}
        <Card title="Live Server & Connection Pool">
          {isLoading && !diag && !diagnosticsError ? (
            <div className="space-y-4 animate-pulse">
              <div className="p-3 bg-bg-surface-2/60 rounded-inner border border-border h-16" />
              <div className="space-y-2 text-xs pt-1">
                <div className="h-4 bg-bg-surface-2 rounded w-full" />
                <div className="h-4 bg-bg-surface-2 rounded w-4/5" />
                <div className="h-4 bg-bg-surface-2 rounded w-3/4" />
                <div className="h-4 bg-bg-surface-2 rounded w-2/3" />
              </div>
            </div>
          ) : diagnosticsError ? (
            <QueryError message={diagnosticsError} onRetry={() => void loadData()} />
          ) : (
          <div className="space-y-4">
            <div className="p-3 bg-bg-surface-2/60 rounded-inner border border-border">
              <div className="flex items-center justify-between text-xs mb-1">
                <span className="text-text-secondary flex items-center gap-1.5">
                  <Database className="w-3.5 h-3.5 text-accent" />
                  PostgreSQL Pool
                </span>
                <span className="font-mono font-bold text-accent">
                  {diag?.db_pool?.total_conns ?? 0} / {diag?.db_pool?.max_conns ?? 0}
                </span>
              </div>
              <div className="text-[11px] text-text-muted font-mono flex justify-between">
                <span>Idle: {diag?.db_pool?.idle_conns ?? 0}</span>
                <span>Acquired: {diag?.db_pool?.acquired_conns ?? 0}</span>
              </div>
            </div>

            <div className="space-y-2 text-xs">
              <div className="flex justify-between py-1 border-b border-border/40">
                <span className="text-text-muted">Versi Gateway</span>
                <span className="font-mono font-semibold text-white">{diag?.version || 'tidak tersedia'}</span>
              </div>
              <div className="flex justify-between py-1 border-b border-border/40">
                <span className="text-text-muted">Go Runtime</span>
                <span className="font-mono text-text-secondary">{diag?.go_version || 'tidak tersedia'}</span>
              </div>
              <div className="flex justify-between py-1 border-b border-border/40">
                <span className="text-text-muted">Goroutines</span>
                <span className="font-mono text-text-secondary">{diag?.num_goroutine ?? 0}</span>
              </div>
              <div className="flex justify-between py-1">
                <span className="text-text-muted">Memory In-Use</span>
                <span className="font-mono text-accent">{diag?.memory_allocated_mb ?? 0} MB</span>
              </div>
            </div>
          </div>
          )}
        </Card>
      </div>
    </div>
  );
};
