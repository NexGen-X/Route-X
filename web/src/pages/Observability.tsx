import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { TimeSeriesPoint, BreakdownItem, Diagnostics, ObservabilitySummary, ResponseCacheStats } from '../types';
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
import {
  RefreshCw,
  Database,
  Zap,
} from 'lucide-react';
import { QueryError } from '../components/common/QueryError';
import { SystemHealthSection } from '../components/observability/SystemHealthSection';
import { formatUSD } from '../utils/money';

export const Observability: React.FC = () => {
  const [windowTime, setWindowTime] = useState('24h');
  const [metric, setMetric] = useState('requests');
  const [series, setSeries] = useState<TimeSeriesPoint[]>([]);
  const [breakdowns, setBreakdowns] = useState<BreakdownItem[]>([]);
  const [breakdownBy, setBreakdownBy] = useState<'provider' | 'model' | 'api_key'>('provider');
  const [summary, setSummary] = useState<ObservabilitySummary | null>(null);
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  const [cacheStats, setCacheStats] = useState<ResponseCacheStats | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [diagnosticsError, setDiagnosticsError] = useState<string | null>(null);

  // Live test ping
  const [isTestingLive, setIsTestingLive] = useState(false);
  const [liveTestLatency, setLiveTestLatency] = useState<number | null>(null);

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
    if (metric === 'error_rate') return `${val}%`;
    if (metric === 'throughput') return `${val}/s`;
    if (val >= 1000000) return `${(val / 1000000).toFixed(1)}M`;
    if (val >= 1000) return `${(val / 1000).toFixed(0)}k`;
    return String(val);
  };

  const formatTooltipValue = (value: unknown): [string, string] | string => {
    const num = Number(value);
    if (!Number.isFinite(num)) return String(value ?? '');
    if (metric === 'cost') return [`$${num.toFixed(4)}`, 'Biaya USD'];
    if (metric === 'latency') return [`${num.toFixed(1)} ms`, 'Latensi P95'];
    if (metric === 'tokens') return [`${num.toLocaleString()}`, 'Total Token'];
    if (metric === 'throughput') return [`${num.toFixed(2)} req/dtk`, 'Throughput'];
    if (metric === 'error_rate') return [`${num.toFixed(2)}%`, 'Tingkat Kesalahan'];
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

  // Penjaga urutan respons (audit F7): tanpa ini, respons lambat dari
  // pilihan metrik/jendela SEBELUMNYA bisa tiba sesudah respons pilihan
  // terbaru dan menimpa tampilannya — grafik tidak lagi cocok dengan
  // kontrol yang sedang dipilih operator.
  const loadReqRef = React.useRef(0);

  const loadData = async () => {
    const reqId = ++loadReqRef.current;
    setIsLoading(true);
    try {
      const metricQuery = (metric === 'throughput' || metric === 'error_rate') ? 'requests' : metric;
      const [sRes, bRes, sumRes, cRes] = await Promise.allSettled([
        api.observability.series(metricQuery, windowTime),
        api.observability.breakdown(breakdownBy, windowTime),
        api.observability.summary(windowTime),
        api.system.cacheStats(),
      ]);

      if (loadReqRef.current !== reqId) return;
      if (sRes.status === 'fulfilled') setSeries(sRes.value?.points || []);
      if (bRes.status === 'fulfilled') setBreakdowns(bRes.value?.items || []);
      if (sumRes.status === 'fulfilled') setSummary(sumRes.value);
      if (cRes.status === 'fulfilled') setCacheStats(cRes.value);

      const allFailed = sRes.status === 'rejected' && bRes.status === 'rejected' && sumRes.status === 'rejected';
      if (allFailed) {
        setLoadError('Seluruh layanan observabilitas gagal merespons. Periksa koneksi jaringan.');
      } else {
        setLoadError(null);
      }

      try {
        const dRes = await api.system.diagnostics();
        if (loadReqRef.current !== reqId) return;
        setDiag(dRes);
        setDiagnosticsError(null);
      } catch (err: unknown) {
        if (loadReqRef.current !== reqId) return;
        setDiag(null);
        setDiagnosticsError(err instanceof Error ? err.message : String(err));
      }
    } catch (err: unknown) {
      if (loadReqRef.current === reqId) {
        setLoadError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (loadReqRef.current === reqId) {
        setIsLoading(false);
      }
    }
  };

  const handleTestLive = async () => {
    setIsTestingLive(true);
    const start = performance.now();
    try {
      await api.system.diagnostics();
      const end = performance.now();
      setLiveTestLatency(Math.round(end - start));
    } catch {
      setLiveTestLatency(null);
    } finally {
      setIsTestingLive(false);
    }
  };

  const totalRequestsFromSeries = series.reduce((acc, p) => acc + (p.requests || 0), 0);
  const totalCostFromSeries = series.reduce((acc, p) => acc + (Number(p.cost_usd) || 0), 0).toFixed(4);
  const totalTokensFromSeries = series.reduce((acc, p) => acc + (p.tokens || 0), 0);
  const errorRate = summary?.error_rate ?? (totalRequestsFromSeries > 0 ? (series.reduce((acc, p) => acc + (p.errors || 0), 0) / totalRequestsFromSeries) * 100 : 0);

  // Perhitungan Throughput rata-rata (req/s)
  const windowSeconds = windowTime === '1h' ? 3600 : windowTime === '24h' ? 86400 : windowTime === '7d' ? 604800 : 2592000;
  const avgThroughputRps = (summary?.total_requests ?? totalRequestsFromSeries) / windowSeconds;

  // Cache Hit calculations
  const totalCacheReqs = (cacheStats?.hits ?? 0) + (cacheStats?.misses ?? 0);
  const cacheHitRatio = totalCacheReqs > 0 ? (((cacheStats?.hits ?? 0) / totalCacheReqs) * 100).toFixed(1) : '0.0';

  // Data series yang diperkaya untuk Recharts (mendukung throughput & error rate dinamis)
  const chartData = series.map((p) => {
    const reqs = p.requests || 0;
    const errs = p.errors || 0;
    const intervalSec = windowTime === '1h' ? 60 : windowTime === '24h' ? 900 : 3600;
    return {
      ...p,
      cost_usd: Number(p.cost_usd ?? 0),
      throughput: Number((reqs / intervalSec).toFixed(3)),
      error_rate: reqs > 0 ? Number(((errs / reqs) * 100).toFixed(2)) : 0,
    };
  });

  useEffect(() => {
    void loadData();
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
                  className={`min-h-[44px] sm:min-h-[32px] px-3 py-1 rounded-inner text-xs font-semibold transition-colors cursor-pointer ${
                    windowTime === w
                      ? 'bg-bg-surface-2 text-white font-medium border border-border-hover/60'
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
              onClick={() => void loadData()}
              isLoading={isLoading}
              aria-label="Segarkan data observabilitas"
              icon={<RefreshCw className="w-3.5 h-3.5" aria-hidden="true" />}
            >
              Segarkan
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => void handleTestLive()}
              isLoading={isTestingLive}
              aria-label="Uji latensi live gateway"
              icon={<Zap className="w-3.5 h-3.5 text-amber-300" aria-hidden="true" />}
            >
              {liveTestLatency !== null ? `Live: ${liveTestLatency}ms` : 'Test Live'}
            </Button>
          </div>
        }
      />

      {loadError && <QueryError message={loadError} onRetry={() => void loadData()} />}

      {/* 4 KPI Analitik Utama — Fase 4: strip polos berpemisah garis, tanpa kartu/tile ikon */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-x-6 gap-y-5">
        <div className="lg:border-l lg:border-border/60 lg:pl-6 lg:first:border-l-0 lg:first:pl-0">
          <span className="text-[11px] font-medium uppercase tracking-wider text-text-muted block">
            Throughput &amp; Permintaan ({windowTime})
          </span>
          <div className="mt-1.5 text-2xl sm:text-[28px] leading-none font-semibold text-white tabular-nums tracking-tight">
            {(summary?.total_requests ?? totalRequestsFromSeries).toLocaleString()}
          </div>
          <div className="mt-2 text-[11px] text-text-muted font-mono tabular-nums">
            Rata-rata: {avgThroughputRps.toFixed(2)} req/dtk
          </div>
        </div>

        <div className="lg:border-l lg:border-border/60 lg:pl-6 lg:first:border-l-0 lg:first:pl-0">
          <span className="text-[11px] font-medium uppercase tracking-wider text-text-muted block">
            Tingkat Kesalahan (Error Rate)
          </span>
          <div className="mt-1.5 flex items-center gap-2">
            <span className={`text-2xl sm:text-[28px] leading-none font-semibold tabular-nums tracking-tight ${errorRate > 5 ? 'text-rose-400' : 'text-emerald-400'}`}>
              {errorRate.toFixed(2)}%
            </span>
            <span className={`text-[10px] px-1.5 py-0.5 rounded border ${
              errorRate > 5 ? 'bg-rose-500/10 text-rose-400 border-rose-500/30' : 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
            }`}>
              {errorRate > 5 ? 'Degraded' : 'Nominal'}
            </span>
          </div>
          <div className="mt-2 text-[11px] text-text-muted font-mono tabular-nums">
            {summary?.error_requests ?? series.reduce((acc, p) => acc + (p.errors || 0), 0)} gagal
          </div>
        </div>

        <div className="lg:border-l lg:border-border/60 lg:pl-6 lg:first:border-l-0 lg:first:pl-0">
          <span className="text-[11px] font-medium uppercase tracking-wider text-text-muted block">
            Konsumsi Token &amp; Biaya
          </span>
          <div className="mt-1.5 text-2xl sm:text-[28px] leading-none font-semibold text-white tabular-nums tracking-tight">
            {(summary?.total_tokens ?? totalTokensFromSeries).toLocaleString()} tok
          </div>
          <div className="mt-2 text-[11px] text-text-muted font-mono tabular-nums">
            Estimasi: {formatUSD(summary?.total_cost_usd || totalCostFromSeries)}
          </div>
        </div>

        <div className="lg:border-l lg:border-border/60 lg:pl-6 lg:first:border-l-0 lg:first:pl-0">
          <span className="text-[11px] font-medium uppercase tracking-wider text-text-muted block">
            Status Response Cache Hit
          </span>
          <div className="mt-1.5 flex items-center gap-2">
            <span className="text-2xl sm:text-[28px] leading-none font-semibold tabular-nums tracking-tight text-emerald-400">
              {cacheHitRatio}%
            </span>
            <span className="text-[10px] px-1.5 py-0.5 rounded border bg-emerald-500/10 text-emerald-400 border-emerald-500/30">
              &lt; 2ms
            </span>
          </div>
          <div className="mt-2 text-[11px] text-text-muted font-mono tabular-nums">
            {cacheStats?.hits ?? 0} hits / {cacheStats?.total_entries ?? 0} entri
          </div>
        </div>
      </div>

      {/* Metric Selector & Main Time Series Chart */}
      <Card
        title="Deret Waktu Telemetri & Analitik"
        action={
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 max-w-full" role="group" aria-label="Pilihan metrik deret waktu">
            {[
              { key: 'requests', label: 'Permintaan' },
              { key: 'throughput', label: 'Throughput' },
              { key: 'tokens', label: 'Token' },
              { key: 'error_rate', label: 'Tingkat Error' },
              { key: 'cost', label: 'Biaya USD' },
              { key: 'latency', label: 'Latensi P95' },
            ].map((m) => (
              <button
                key={m.key}
                type="button"
                aria-pressed={metric === m.key}
                aria-label={`Metrik ${m.label}`}
                onClick={() => setMetric(m.key)}
                className={`min-h-[44px] sm:min-h-[30px] py-1 text-[11px] sm:text-xs transition-colors whitespace-nowrap text-center cursor-pointer border-b-2 ${
                  metric === m.key
                    ? 'border-accent text-white font-semibold'
                    : 'border-transparent text-text-muted hover:text-text-primary'
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
                <RefreshCw className="w-5 h-5 animate-spin text-accent-text" aria-hidden="true" />
                <span>Memuat data telemetri...</span>
              </div>
            </div>
          ) : series.length === 0 ? (
            <div className="h-full flex items-center justify-center text-xs text-text-muted">
              Tidak ada data deret waktu untuk rentang yang dipilih.
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#262626" />
                <XAxis
                  dataKey="timestamp"
                  stroke="#737373"
                  fontSize={11}
                  tickLine={false}
                  tickFormatter={formatXAxis}
                />
                <YAxis
                  stroke="#737373"
                  fontSize={11}
                  tickLine={false}
                  tickFormatter={formatYAxis}
                />
                <Tooltip
                  formatter={formatTooltipValue}
                  labelFormatter={formatTooltipLabel}
                  contentStyle={{
                    backgroundColor: '#1E1E1E',
                    borderColor: '#333333',
                    borderRadius: '8px',
                    fontSize: '12px',
                    color: '#F8FAFC',
                  }}
                />
                <Area
                  type="monotone"
                  dataKey={
                    metric === 'cost'
                      ? 'cost_usd'
                      : metric === 'latency'
                      ? 'p95_latency_ms'
                      : metric === 'throughput'
                      ? 'throughput'
                      : metric === 'error_rate'
                      ? 'error_rate'
                      : metric
                  }
                  stroke={
                    metric === 'error_rate'
                      ? '#F43F5E'
                      : metric === 'cost'
                      ? '#3ECF8E'
                      : '#5BACE6'
                  }
                  fill={
                    metric === 'error_rate'
                      ? '#F43F5E'
                      : metric === 'cost'
                      ? '#3ECF8E'
                      : '#5BACE6'
                  }
                  fillOpacity={0.15}
                  strokeWidth={2}
                />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>
      </Card>

      {/* Breakdown Komposisi & Runtime Server */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card
          className="lg:col-span-2"
          title="Komposisi Lalu Lintas"
          action={
            <div className="flex items-center gap-4 max-w-full">
              {(['provider', 'model', 'api_key'] as const).map((b) => (
                <button
                  key={b}
                  type="button"
                  onClick={() => setBreakdownBy(b)}
                  className={`min-h-[44px] sm:min-h-[28px] py-0.5 text-xs uppercase font-mono whitespace-nowrap text-center cursor-pointer transition-colors border-b-2 ${
                    breakdownBy === b
                      ? 'border-accent text-white font-semibold'
                      : 'border-transparent text-text-muted hover:text-white'
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
                  <div className="w-full h-[5px] bg-bg-surface-2 rounded-full overflow-hidden">
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
        <Card title="Server Live & Pool Koneksi">
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
              <div className="py-1">
                <div className="flex items-center justify-between text-xs mb-1">
                  <span className="text-text-secondary flex items-center gap-1.5">
                    <Database className="w-3.5 h-3.5 text-accent-text" aria-hidden="true" />
                    PostgreSQL Pool
                  </span>
                  <span className="font-mono font-bold text-accent-text">
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
                  <span className="font-mono text-accent-text">{diag?.memory_allocated_mb ?? 0} MB</span>
                </div>
              </div>
            </div>
          )}
        </Card>
      </div>

      {/* Kesehatan Sistem — konsolidasi isi Diagnostics (Fase 2 R6) */}
      <SystemHealthSection />
    </div>
  );
};
