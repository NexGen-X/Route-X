import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ObservabilitySummary, TimeSeriesPoint, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import {
  Activity,
  ArrowUpRight,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Coins,
  Cpu,
  Server,
  Zap,
  RefreshCw,
} from 'lucide-react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from 'recharts';

export const Dashboard: React.FC<{ onNavigate: (path: string) => void }> = ({ onNavigate }) => {
  const [summary, setSummary] = useState<ObservabilitySummary | null>(null);
  const [series, setSeries] = useState<TimeSeriesPoint[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  const loadData = async () => {
    setIsLoading(true);
    try {
      const [sumRes, serRes, provRes] = await Promise.all([
        api.observability.summary('24h'),
        api.observability.series('requests', '24h'),
        api.providers.list(),
      ]);
      setSummary(sumRes);
      setSeries(serRes.points || []);
      setProviders(provRes.items || []);
    } catch (err) {
      console.error('Failed to load dashboard data:', err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 30000); // Polling tiap 30 detik
    return () => clearInterval(interval);
  }, []);

  const formatUSD = (valStr?: string) => {
    if (!valStr) return '$0.00';
    const num = parseFloat(valStr);
    return isNaN(num) ? '$0.00' : `$${num.toFixed(4)}`;
  };

  return (
    <div className="space-y-6">
      {/* Top Banner / Actions */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Ringkasan Sistem</h2>
          <p className="text-xs text-text-secondary mt-1">
            Status operasional gerbang AI, metrik inferensi 24 jam terakhir, dan ketersediaan upstream.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={loadData}
            isLoading={isLoading}
            icon={<RefreshCw className="w-3.5 h-3.5" />}
          >
            Segarkan
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={() => onNavigate('/requests')}
            icon={<ArrowUpRight className="w-3.5 h-3.5" />}
          >
            Buka Request Log
          </Button>
        </div>
      </div>

      {/* 4 Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="p-5">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-text-muted uppercase tracking-wider">Total Permintaan</span>
            <div className="p-2 rounded-nav bg-accent/10 text-accent">
              <Activity className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-3">
            <div className="text-2xl font-bold text-white font-mono">
              {(summary?.total_requests ?? 0).toLocaleString()}
            </div>
            <div className="flex items-center gap-2 mt-1">
              <span className="text-xs text-text-secondary">
                {(summary?.success_requests ?? 0).toLocaleString()} berhasil
              </span>
              <span className="text-text-muted text-xs">•</span>
              <span className={`text-xs font-mono ${(summary?.error_rate ?? 0) > 5 ? 'text-status-error' : 'text-text-muted'}`}>
                {((summary?.error_rate ?? 0)).toFixed(1)}% error
              </span>
            </div>
          </div>
        </Card>

        <Card className="p-5">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-text-muted uppercase tracking-wider">Total Token</span>
            <div className="p-2 rounded-nav bg-emerald-500/10 text-emerald-400">
              <Zap className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-3">
            <div className="text-2xl font-bold text-white font-mono">
              {((summary?.total_tokens ?? 0) / 1000).toFixed(1)}k
            </div>
            <div className="text-xs text-text-secondary mt-1">
              Prompt: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k | Out: {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
            </div>
          </div>
        </Card>

        <Card className="p-5">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-text-muted uppercase tracking-wider">Estimasi Biaya</span>
            <div className="p-2 rounded-nav bg-amber-500/10 text-amber-400">
              <Coins className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-3">
            <div className="text-2xl font-bold text-white font-mono">
              {formatUSD(summary?.total_cost_usd)}
            </div>
            <div className="text-xs text-text-secondary mt-1">
              Akumulasi biaya upstream USD (skala 8 desimal)
            </div>
          </div>
        </Card>

        <Card className="p-5">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-text-muted uppercase tracking-wider">Latensi P95</span>
            <div className="p-2 rounded-nav bg-purple-500/10 text-purple-400">
              <Clock className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-3">
            <div className="text-2xl font-bold text-white font-mono">
              {summary?.p95_latency_ms ?? 0} ms
            </div>
            <div className="text-xs text-text-secondary mt-1">
              Rata-rata: {summary?.avg_latency_ms ?? 0} ms
            </div>
          </div>
        </Card>
      </div>

      {/* Main Chart Section */}
      <Card
        title="Volume Lalu Lintas Permintaan (24 Jam)"
        subtitle="Fluktuasi inferensi request per interval waktu secara kontinu"
      >
        <div className="h-64 w-full pt-2">
          {series.length === 0 ? (
            <div className="h-full flex items-center justify-center text-xs text-text-muted">
              Belum ada data metrik 24 jam terakhir
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={series}>
                <defs>
                  <linearGradient id="reqGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#BEF264" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="#BEF264" stopOpacity={0} />
                  </linearGradient>
                </defs>
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
                <Area type="monotone" dataKey="requests" stroke="#BEF264" fillOpacity={1} fill="url(#reqGrad)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>
      </Card>

      {/* Upstream Providers Grid */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-base font-bold text-white tracking-tight">Status Kesehatan Upstream Providers</h3>
          <Button variant="ghost" size="sm" onClick={() => onNavigate('/upstreams/providers')}>
            Kelola Provider &rarr;
          </Button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {providers.map((p) => {
            const isHealthy = p.last_health_status === 'healthy';
            const isDegraded = p.last_health_status === 'degraded';
            return (
              <Card key={p.id} className="p-4 hover:border-accent/40 transition-colors">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 rounded-full bg-bg-surface-2 flex items-center justify-center text-accent">
                      <Server className="w-4 h-4" />
                    </div>
                    <div>
                      <h4 className="text-sm font-semibold text-white">{p.display_name || p.name}</h4>
                      <p className="text-[11px] text-text-muted font-mono">{p.kind}</p>
                    </div>
                  </div>
                  <Badge variant={isHealthy ? 'success' : isDegraded ? 'warn' : 'error'}>
                    {p.last_health_status || (p.enabled ? 'unknown' : 'disabled')}
                  </Badge>
                </div>

                <div className="mt-4 pt-3 border-t border-border flex items-center justify-between text-xs">
                  <span className="text-text-muted">Latensi Terakhir</span>
                  <span className="font-mono text-text-primary">{p.last_latency_ms ? `${p.last_latency_ms} ms` : '-'}</span>
                </div>
              </Card>
            );
          })}
        </div>
      </div>
    </div>
  );
};
