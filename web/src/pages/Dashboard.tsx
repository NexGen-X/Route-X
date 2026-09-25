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
    } catch (err) {
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
    } catch (err) {
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

  const healthyProvidersCount = providers.filter((p) => p.last_health_status === 'healthy').length;

  return (
    <div className="space-y-6 pb-12 animate-fade-in">
      {pollError && (
        <div
          role="status"
          aria-live="polite"
          className="text-xs text-amber-300 border border-amber-500/30 bg-amber-500/10 rounded-lg p-3"
        >
          Pembaruan live gagal: {pollError}. Data terakhir tetap ditampilkan.
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

      {/* 2. ONBOARDING & QUICK-START GUIDE */}
      <QuickStartGuide onNavigate={onNavigate} />

      {/* 3. SYSTEM TELEMETRY — 4 LIVE RUNTIME CARDS */}
      <SystemTelemetrySection overview={overview} />

      {/* 4. INFERENCE PERFORMANCE METRICS RIBBON (4 CARDS) */}
      <PerformanceMetricsRibbon summary={summary} />

      {/* 5. TRAFFIC CHART & RECENT REQUESTS FEED */}
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

      {/* 6. UPSTREAM PROVIDER HEALTH & STATUS MATRIX */}
      <ProviderHealthMatrix providers={providers} onNavigate={onNavigate} />
    </div>
  );
};
