import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type {
  ObservabilitySummary,
  TimeSeriesPoint,
  Provider,
  SystemOverview,
  RequestLog,
} from '../types';
import { useToast } from '../context/ToastContext';
import {
  DashboardHero,
  QuickStartGuide,
  SystemTelemetrySection,
  PerformanceMetricsRibbon,
  TrafficChart,
  LiveRequestsFeed,
  ProviderHealthMatrix,
} from '../components/dashboard';
import { ChevronDown, Cpu } from 'lucide-react';

export interface DashboardProps {
  onNavigate: (path: string) => void;
}

export const Dashboard: React.FC<DashboardProps> = ({ onNavigate }) => {
  const { toast } = useToast();
  const [summary, setSummary] = useState<ObservabilitySummary | null>(null);
  const [series, setSeries] = useState<TimeSeriesPoint[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [overview, setOverview] = useState<SystemOverview | null>(null);
  const [recentRequests, setRecentRequests] = useState<RequestLog[]>([]);
  const [timeWindow, setTimeWindow] = useState<'1h' | '6h' | '24h' | '7d'>('24h');
  const [metricType, setMetricType] = useState<'requests' | 'latency' | 'tokens'>('requests');
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [pollError, setPollError] = useState<string | null>(null);
  // QuickStart tersembunyi setelah di-dismiss (disimpan ke localStorage)
  const [quickStartDismissed, setQuickStartDismissed] = useState<boolean>(
    () => localStorage.getItem('routex-quickstart-dismissed') === '1'
  );
  // System Telemetry collapsible — tersembunyi secara default agar Dashboard lebih ringkas
  const [telemetryExpanded, setTelemetryExpanded] = useState<boolean>(false);

  const loadData = async (showToast = false) => {
    if (showToast) setIsRefreshing(true);
    else setIsLoading(true);

    try {
      const [sumRes, serRes, provRes, overRes, reqRes] = await Promise.all([
        api.observability.summary(timeWindow),
        api.observability.series(metricType, timeWindow),
        api.providers.list(),
        api.system.overview(),
        api.requests.list({ limit: 6 }),
      ]);

      if (sumRes) setSummary(sumRes);
      setSeries(serRes.points || []);
      setProviders(provRes.items || []);
      if (overRes) setOverview(overRes);
      setRecentRequests(reqRes.items || []);
      setLoadError(null);

      if (showToast) {
        toast.success('Metrik telemetri dashboard berhasil disegarkan');
      }
    } catch (err: unknown) {
      console.error('Failed to load dashboard data:', err);
      const errMsg = err instanceof Error ? err.message : String(err);
      setLoadError(errMsg);
      if (showToast) {
        toast.error('Gagal menyegarkan data: ' + errMsg);
      }
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  };

  // Polling data overview cepat setiap 5 detik
  const refreshOverview = async () => {
    try {
      const [overRes, reqRes] = await Promise.all([
        api.system.overview(),
        api.requests.list({ limit: 6 }),
      ]);
      setOverview(overRes);
      setRecentRequests(reqRes.items || []);
      setPollError(null);
    } catch (err: unknown) {
      setPollError(err instanceof Error ? err.message : String(err));
    }
  };

  useEffect(() => {
    loadData();
    const slowInterval = setInterval(() => loadData(false), 30000);
    const fastInterval = setInterval(refreshOverview, 5000);
    return () => {
      clearInterval(slowInterval);
      clearInterval(fastInterval);
    };
  }, [timeWindow, metricType]);

  const handleDismissQuickStart = () => {
    localStorage.setItem('routex-quickstart-dismissed', '1');
    setQuickStartDismissed(true);
  };

  const healthyProvidersCount = providers.filter((p) => p.last_health_status === 'healthy').length;

  return (
    <div className="space-y-6 pb-12 animate-fade-in">
      {pollError && (
        <div
          role="status"
          aria-live="polite"
          className="text-xs text-amber-300 border border-amber-500/30 bg-amber-500/10 rounded-xl p-3 flex items-center gap-2"
        >
          <span className="w-2 h-2 rounded-full bg-amber-400 shrink-0" aria-hidden="true" />
          <span>Pembaruan live gagal: {pollError}. Data terakhir tetap ditampilkan.</span>
        </div>
      )}

      {/* 1. HERO OPERATIONAL STATUS & ACTION BAR */}
      <DashboardHero
        loadError={loadError}
        healthyProvidersCount={healthyProvidersCount}
        totalProvidersCount={providers.length}
        overview={overview}
        isLoading={isLoading}
        isRefreshing={isRefreshing}
        onRefresh={() => loadData(true)}
        onNavigate={onNavigate}
      />

      {/* 2. ONBOARDING & QUICK-START GUIDE — muncul hanya sampai di-dismiss */}
      {!quickStartDismissed && (
        <QuickStartGuide onNavigate={onNavigate} onDismiss={handleDismissQuickStart} />
      )}

      {/* 3. INFERENCE PERFORMANCE METRICS RIBBON (4 CARDS) */}
      <PerformanceMetricsRibbon summary={summary} />

      {/* 4. TRAFFIC CHART & RECENT REQUESTS FEED */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6">
        <TrafficChart
          series={series}
          metricType={metricType}
          setMetricType={setMetricType}
          timeWindow={timeWindow}
          setTimeWindow={setTimeWindow}
        />
        <LiveRequestsFeed recentRequests={recentRequests} onNavigate={onNavigate} />
      </div>

      {/* 5. UPSTREAM PROVIDER HEALTH & STATUS MATRIX */}
      <ProviderHealthMatrix providers={providers} onNavigate={onNavigate} />

      {/* 6. SYSTEM TELEMETRY — collapsible, tersembunyi secara default */}
      <div className="border border-border/80 rounded-card overflow-hidden bg-bg-surface shadow-sm transition-all">
        <button
          type="button"
          onClick={() => setTelemetryExpanded((v) => !v)}
          className="w-full flex items-center justify-between px-5 py-3.5 bg-bg-surface hover:bg-bg-surface-2 transition-colors text-left cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
          aria-expanded={telemetryExpanded}
          aria-label="Tampilkan atau sembunyikan runtime telemetri gateway"
        >
          <div className="flex items-center gap-2.5">
            <Cpu className="w-4 h-4 text-blue-400" aria-hidden="true" />
            <span className="text-xs font-semibold text-text-secondary">
              Runtime Telemetri Gateway
            </span>
          </div>
          <ChevronDown
            className={`w-4 h-4 text-text-muted transition-transform duration-200 ${
              telemetryExpanded ? 'rotate-180' : ''
            }`}
            aria-hidden="true"
          />
        </button>
        {telemetryExpanded && (
          <div className="p-5 border-t border-border/70 bg-bg-surface-1/40">
            <SystemTelemetrySection overview={overview} />
          </div>
        )}
      </div>
    </div>
  );
};
