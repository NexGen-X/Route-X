import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { TimeSeriesPoint, BreakdownItem, Diagnostics } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from 'recharts';
import { RefreshCw, Database } from 'lucide-react';
import { QueryError } from '../components/common/QueryError';

export const Observability: React.FC = () => {
  const [windowTime, setWindowTime] = useState('24h');
  const [metric, setMetric] = useState('requests');
  const [series, setSeries] = useState<TimeSeriesPoint[]>([]);
  const [breakdowns, setBreakdowns] = useState<BreakdownItem[]>([]);
  const [breakdownBy, setBreakdownBy] = useState<'provider' | 'model' | 'api_key'>('provider');
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [diagnosticsError, setDiagnosticsError] = useState<string | null>(null);

  const loadData = async () => {
    setIsLoading(true);
    try {
      const [sRes, bRes] = await Promise.all([
        api.observability.series(metric, windowTime),
        api.observability.breakdown(breakdownBy, windowTime),
      ]);
      setSeries(sRes.points || []);
      setBreakdowns(bRes.items || []);
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

  useEffect(() => {
    loadData();
  }, [windowTime, metric, breakdownBy]);

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Observabilitas & Telemetri</h2>
          <p className="text-xs text-text-secondary mt-1">
            Analisis metrik terperinci, latensi persentil, dan distribusi lalu lintas model & provider.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {['1h', '24h', '7d', '30d'].map((w) => (
            <button
              key={w}
              onClick={() => setWindowTime(w)}
              className={`px-3 py-1 rounded-nav text-xs font-semibold transition-colors ${
                windowTime === w
                  ? 'bg-accent text-black font-bold'
                  : 'bg-bg-surface text-text-secondary border border-border hover:text-white'
              }`}
            >
              {w}
            </button>
          ))}
          <Button variant="secondary" size="sm" onClick={loadData} isLoading={isLoading}>
            <RefreshCw className="w-3.5 h-3.5" />
          </Button>
        </div>
      </div>

      {loadError && <QueryError message={loadError} onRetry={() => void loadData()} />}

      {/* Metric Selector & Main Time Series Chart */}
      <Card
        title="Deret Waktu Telemetri"
        subtitle={`Visualisasi metrik ${metric} pada rentang ${windowTime}`}
        action={
          <div className="flex flex-wrap sm:flex-nowrap items-center gap-1 bg-bg-surface-2 p-1 rounded-nav border border-border">
            {[
              { key: 'requests', label: 'Requests' },
              { key: 'tokens', label: 'Tokens' },
              { key: 'cost', label: 'Cost USD' },
              { key: 'latency', label: 'Latency P95' },
            ].map((m) => (
              <button
                key={m.key}
                onClick={() => setMetric(m.key)}
                className={`px-2 sm:px-2.5 py-1 text-[11px] sm:text-xs rounded-inner transition-colors font-medium ${
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
          {series.length === 0 ? (
            <div className="h-full flex items-center justify-center text-xs text-text-muted">
              Tidak ada data deret waktu untuk rentang yang dipilih.
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={series}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1F1F1F" />
                <XAxis dataKey="timestamp" stroke="#6B7280" fontSize={11} tickLine={false} />
                <YAxis stroke="#6B7280" fontSize={11} tickLine={false} />
                <Tooltip
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
          subtitle="Distribusi volume permintaan dan pemakaian token"
          action={
            <div className="flex items-center gap-1 bg-bg-surface-2 p-1 rounded-nav border border-border">
              {(['provider', 'model', 'api_key'] as const).map((b) => (
                <button
                  key={b}
                  onClick={() => setBreakdownBy(b)}
                  className={`px-2 py-0.5 text-xs rounded-inner uppercase font-mono ${
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
            {breakdowns.length === 0 ? (
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
        <Card title="Live Server & Connection Pool" subtitle="Statistik koneksi pgxpool dan runtime Go">
          {diagnosticsError ? (
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
