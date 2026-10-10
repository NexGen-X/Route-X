import React, { useEffect, useState, useRef, useCallback } from 'react';
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
  LiveRequestsFeed,
  ProviderHealthMatrix,
} from '../components/dashboard';
import { WidgetErrorBoundary } from '../components/common/WidgetErrorBoundary';
import { ChevronDown, Cpu } from 'lucide-react';

const TrafficChart = React.lazy(() =>
  import('../components/dashboard/TrafficChart').then((m) => ({ default: m.TrafficChart }))
);

export const TrafficChartSkeleton: React.FC = () => (
  <div
    role="status"
    aria-label="Memuat grafik analitik lalu lintas"
    className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-4 sm:p-5 shadow-sm flex flex-col justify-between animate-pulse min-h-[340px]"
  >
    {/* Minimalist zinc header skeleton */}
    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3 sm:mb-4">
      <div className="flex items-center gap-2">
        <div className="w-4 h-4 rounded bg-zinc-800" />
        <div>
          <div className="h-4 w-40 bg-zinc-800 rounded mb-1" />
          <div className="h-2.5 w-20 bg-zinc-800/60 rounded" />
        </div>
      </div>
      <div className="flex items-center overflow-x-auto max-w-full bg-zinc-900 p-0.5 rounded-md border border-zinc-800 gap-1">
        <div className="h-6 w-10 bg-zinc-800 rounded" />
        <div className="h-6 w-10 bg-zinc-850 rounded" />
        <div className="h-6 w-12 bg-zinc-850 rounded" />
        <span className="w-px h-4 bg-zinc-800 mx-0.5" />
        <div className="h-6 w-7 bg-zinc-850 rounded" />
        <div className="h-6 w-7 bg-zinc-850 rounded" />
        <div className="h-6 w-8 bg-zinc-850 rounded" />
      </div>
    </div>

    {/* Minimalist zinc area chart silhouette */}
    <div className="h-52 sm:h-64 w-full pt-1 sm:pt-2 flex flex-col justify-end relative overflow-hidden rounded-md bg-zinc-950 border border-zinc-800/80 p-4">
      <div className="absolute inset-0 flex flex-col justify-between p-4 pointer-events-none opacity-20">
        <div className="border-b border-dashed border-zinc-700 w-full" />
        <div className="border-b border-dashed border-zinc-700 w-full" />
        <div className="border-b border-dashed border-zinc-700 w-full" />
      </div>
      <div className="w-full h-28 bg-zinc-900/40 rounded-t relative overflow-hidden flex items-end justify-between px-2 gap-1.5 sm:gap-3">
        <div className="w-full h-[25%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[40%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[30%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[55%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[45%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[70%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[60%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[85%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[65%] bg-zinc-800 rounded-t" />
        <div className="w-full h-[95%] bg-zinc-800 rounded-t" />
      </div>
    </div>
  </div>
);

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

  // Penanda unmount & sequence ID untuk mencegah memory leak dan stale state race conditions (P2-6)
  const mountedRef = useRef<boolean>(true);
  const loadSeqRef = useRef<number>(0);
  const refreshSeqRef = useRef<number>(0);
  const toastRef = useRef(toast);
  toastRef.current = toast;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const loadData = useCallback(
    async (showToast = false) => {
      const seq = ++loadSeqRef.current;
      if (showToast) setIsRefreshing(true);
      else setIsLoading(true);

      try {
        const [sumRes, serRes, provRes, overRes, reqRes] = await Promise.allSettled([
          api.observability.summary(timeWindow),
          api.observability.series(metricType, timeWindow),
          api.providers.list(),
          api.system.overview(),
          api.requests.list({ limit: 6 }),
        ]);

        // Abaikan hasil jika komponen sudah unmount atau ada permintaan data yang lebih baru
        if (!mountedRef.current || seq !== loadSeqRef.current) {
          return;
        }

        const failedEndpoints: string[] = [];

        if (sumRes.status === 'fulfilled' && sumRes.value) {
          setSummary(sumRes.value);
        } else if (sumRes.status === 'rejected') {
          failedEndpoints.push('Ringkasan Telemetri');
        }

        if (serRes.status === 'fulfilled' && serRes.value) {
          setSeries(serRes.value.points || []);
        } else if (serRes.status === 'rejected') {
          failedEndpoints.push('Grafik Trafik');
        }

        if (provRes.status === 'fulfilled' && provRes.value) {
          const pItems = Array.isArray(provRes.value) ? provRes.value : provRes.value.items;
          setProviders(pItems || []);
        } else if (provRes.status === 'rejected') {
          failedEndpoints.push('Upstream Providers');
        }

        if (overRes.status === 'fulfilled' && overRes.value) {
          setOverview(overRes.value);
        } else if (overRes.status === 'rejected') {
          failedEndpoints.push('Status Gateway');
        }

        if (reqRes.status === 'fulfilled' && reqRes.value) {
          setRecentRequests(reqRes.value.items || []);
        } else if (reqRes.status === 'rejected') {
          failedEndpoints.push('Log Permintaan');
        }

        if (sumRes.status === 'rejected') {
          const err = sumRes.reason;
          setLoadError(err instanceof Error ? err.message : String(err));
        } else if (failedEndpoints.length === 5) {
          setLoadError('Seluruh layanan upstream tidak merespons. Periksa status jaringan gateway.');
        } else {
          setLoadError(null);
        }

        if (failedEndpoints.length > 0) {
          setPollError(
            `Layanan upstream ${failedEndpoints.join(', ')} sedang tidak merespons. Komponen lainnya tetap aktif.`
          );
        } else {
          setPollError(null);
        }

        if (showToast) {
          if (failedEndpoints.length === 0) {
            toastRef.current.success('Metrik telemetri dashboard berhasil disegarkan');
          } else {
            toastRef.current.info(`Data diperbarui sebagian (${5 - failedEndpoints.length}/5 layanan merespons)`);
          }
        }
      } catch (err: unknown) {
        if (!mountedRef.current || seq !== loadSeqRef.current) return;
        console.error('Failed to load dashboard data:', err);
        const errMsg = err instanceof Error ? err.message : String(err);
        setLoadError(errMsg);
        if (showToast) {
          toastRef.current.error('Gagal menyegarkan data: ' + errMsg);
        }
      } finally {
        if (mountedRef.current && seq === loadSeqRef.current) {
          setIsLoading(false);
          setIsRefreshing(false);
        }
      }
    },
    [timeWindow, metricType]
  );

  // Polling data overview cepat setiap 5 detik dengan isolasi kegagalan
  const refreshOverview = useCallback(async () => {
    const seq = ++refreshSeqRef.current;
    try {
      const [overRes, reqRes] = await Promise.allSettled([
        api.system.overview(),
        api.requests.list({ limit: 6 }),
      ]);

      if (!mountedRef.current || seq !== refreshSeqRef.current) return;

      if (overRes.status === 'fulfilled' && overRes.value) {
        setOverview(overRes.value);
      }
      if (reqRes.status === 'fulfilled' && reqRes.value) {
        setRecentRequests(reqRes.value.items || []);
      }

      if (overRes.status === 'rejected' && reqRes.status === 'rejected') {
        setPollError('Pembaruan live gagal: Gateway overview dan recent requests tidak merespons.');
      } else {
        setPollError(null);
      }
    } catch (err: unknown) {
      if (!mountedRef.current || seq !== refreshSeqRef.current) return;
      setPollError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    loadData();
    const slowInterval = setInterval(() => {
      if (mountedRef.current) void loadData(false);
    }, 30000);
    const fastInterval = setInterval(() => {
      if (mountedRef.current) void refreshOverview();
    }, 5000);
    return () => {
      clearInterval(slowInterval);
      clearInterval(fastInterval);
    };
  }, [loadData, refreshOverview]);

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
      <WidgetErrorBoundary title="Metrik Kinerja Inferensi" onRetry={() => void loadData(true)}>
        <PerformanceMetricsRibbon summary={summary} />
      </WidgetErrorBoundary>

      {/* 4. TRAFFIC CHART & RECENT REQUESTS FEED */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6">
        <div className="lg:col-span-2">
          <WidgetErrorBoundary title="Grafik Analitik Trafik" onRetry={() => void loadData(true)}>
            <React.Suspense fallback={<TrafficChartSkeleton />}>
              <TrafficChart
                series={series}
                metricType={metricType}
                setMetricType={setMetricType}
                timeWindow={timeWindow}
                setTimeWindow={setTimeWindow}
              />
            </React.Suspense>
          </WidgetErrorBoundary>
        </div>
        <div>
          <WidgetErrorBoundary title="Feed Permintaan Terbaru" onRetry={() => void refreshOverview()}>
            <LiveRequestsFeed recentRequests={recentRequests} onNavigate={onNavigate} />
          </WidgetErrorBoundary>
        </div>
      </div>

      {/* 5. UPSTREAM PROVIDER HEALTH & STATUS MATRIX */}
      <WidgetErrorBoundary title="Status Matriks Provider" onRetry={() => void loadData(true)}>
        <ProviderHealthMatrix providers={providers} onNavigate={onNavigate} />
      </WidgetErrorBoundary>

      {/* 6. SYSTEM TELEMETRY — collapsible, tersembunyi secara default */}
      <WidgetErrorBoundary title="Runtime Telemetri Gateway">
        <div className="border border-zinc-800/80 rounded-lg overflow-hidden bg-zinc-900/40 shadow-sm transition-colors">
          <button
            type="button"
            onClick={() => setTelemetryExpanded((v) => !v)}
            className="w-full flex items-center justify-between px-4 py-3 bg-zinc-900/40 hover:bg-zinc-900/70 transition-colors text-left cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-zinc-400 min-h-[40px]"
            aria-expanded={telemetryExpanded}
            aria-label="Tampilkan atau sembunyikan runtime telemetri gateway"
          >
            <div className="flex items-center gap-2">
              <Cpu className="w-4 h-4 text-zinc-400" aria-hidden="true" />
              <span className="text-xs font-medium text-zinc-300">
                Runtime Telemetri Gateway
              </span>
            </div>
            <ChevronDown
              className={`w-4 h-4 text-zinc-500 transition-transform duration-150 ${
                telemetryExpanded ? 'rotate-180' : ''
              }`}
              aria-hidden="true"
            />
          </button>
          {telemetryExpanded && (
            <div className="p-4 sm:p-5 border-t border-zinc-800/80 bg-zinc-950/40">
              <SystemTelemetrySection overview={overview} />
            </div>
          )}
        </div>
      </WidgetErrorBoundary>
    </div>
  );
};
