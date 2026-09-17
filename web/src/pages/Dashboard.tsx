
import React, { useEffect, useState } from 'react';

const MemoizedChart = React.memo(({ series, metricType }: any) => (
  <ResponsiveContainer width="100%" height="100%">
    <AreaChart data={series}>
      <defs>
        <linearGradient id="chartGradient" x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor="#BEF264" stopOpacity={0.3} />
          <stop offset="95%" stopColor="#BEF264" stopOpacity={0.0} />
        </linearGradient>
      </defs>
      <CartesianGrid strokeDasharray="3 3" stroke="#1C2029" vertical={false} />
      <XAxis dataKey="timestamp" stroke="#525866" fontSize={11} tickLine={false} />
      <YAxis stroke="#525866" fontSize={11} tickLine={false} />
      <Tooltip
        contentStyle={{
          backgroundColor: '#16181F',
          borderColor: '#2E3342',
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
        strokeWidth={2}
      />
    </AreaChart>
  </ResponsiveContainer>
));

import { formatUSD } from '../utils/money';
import { api } from '../api/client';
import type { ObservabilitySummary, TimeSeriesPoint, Provider, SystemOverview, RequestLog } from '../types';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { copyTextToClipboard } from '../utils/clipboard';
import {
  Activity,
  ArrowUpRight,
  Clock,
  Coins,
  Server,
  Zap,
  RefreshCw,
  Cpu,
  Share2,
  ShieldCheck,
  CheckCircle2,
  Terminal,
  ArrowDown,
  ArrowUp,
  BarChart3,
  Sparkles,
  X,
  KeyRound,
  Copy,
  Check,
  ChevronRight,
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
import { ProviderBrandIcon } from '../components/providers/ProviderIcons';
import { useToast } from '../context/ToastContext';

export const Dashboard: React.FC<{ onNavigate: (path: string) => void }> = ({ onNavigate }) => {
  const { toast } = useToast();
  const [summary, setSummary] = useState<ObservabilitySummary | null>(null);
  const [series, setSeries] = useState<TimeSeriesPoint[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [overview, setOverview] = useState<SystemOverview | null>(null);
  const [recentRequests, setRecentRequests] = useState<RequestLog[]>([]);
  const [timeWindow, setTimeWindow] = useState<'1h' | '6h' | '24h' | '7d'>('24h');
  const [metricType, setMetricType] = useState<'requests' | 'latency' | 'tokens'>('requests');
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [pollError, setPollError] = useState<string | null>(null);
  const [showOnboarding, setShowOnboarding] = useState<boolean>(() => {
    try {
      return localStorage.getItem('routex_dismiss_onboarding') !== 'true';
    } catch {
      return true;
    }
  });

  const [selectedRecipe, setSelectedRecipe] = useState<'coding' | 'budget' | 'uptime' | null>(null);
  const [recipeCopied, setRecipeCopied] = useState(false);
  const recipeCopyTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleCopyRecipeText = (text: string) => {
    copyTextToClipboard(text);
    setRecipeCopied(true);
    if (recipeCopyTimerRef.current) clearTimeout(recipeCopyTimerRef.current);
    recipeCopyTimerRef.current = setTimeout(() => setRecipeCopied(false), 2000);
  };

  const dismissOnboarding = () => {
    setShowOnboarding(false);
    try {
      localStorage.setItem('routex_dismiss_onboarding', 'true');
    } catch {}
  };

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
      setLoadError(err instanceof Error ? err.message : String(err));
      if (showToast) {
        toast.error('Gagal menyegarkan data: ' + (err instanceof Error ? err.message : String(err)));
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

  const formatBytes = (bytes?: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
  };

  const formatUptime = (sec?: number) => {
    if (!sec || sec < 0) return '0s';
    const d = Math.floor(sec / 86400);
    const h = Math.floor((sec % 86400) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = Math.floor(sec % 60);
    if (d > 0) return `${d}d ${h}h ${m}m`;
    if (h > 0) return `${h}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  };

  const formatTimeAgo = (dateStr: string) => {
    const t = new Date(dateStr).getTime();
    if (Number.isNaN(t)) return 'waktu tak dikenal';
    const diff = Math.floor((Date.now() - t) / 1000);
    if (diff < 5) return 'baru saja';
    if (diff < 60) return `${diff} detik lalu`;
    const m = Math.floor(diff / 60);
    if (m < 60) return `${m}m lalu`;
    const h = Math.floor(m / 60);
    return `${h}j lalu`;
  };

  // Kalkulasi persentase RAM host riil
  const hostRAMPct = overview?.host_ram_total_bytes && overview.host_ram_total_bytes > 0
    ? Math.min(100, Math.max(0, ((overview.host_ram_used_bytes || 0) / overview.host_ram_total_bytes) * 100))
    : 0;

  const healthyProvidersCount = providers.filter((p) => p.last_health_status === 'healthy').length;

  return (
    <div className="space-y-6 pb-12 animate-fade-in">
      {pollError && (
        <div role="status" aria-live="polite" className="text-xs text-amber-300 border border-amber-500/30 bg-amber-500/10 rounded-lg p-3">
          Pembaruan live gagal: {pollError}. Data terakhir tetap ditampilkan.
        </div>
      )}

      {/* ==================================================================== */}
      {/* 1. HERO OPERATIONAL STATUS & ACTION BAR                             */}
      {/* ==================================================================== */}
      <div className="bg-bg-surface border border-border rounded-card p-6 shadow-sm">
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-6">
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-3">
              <span role="status" aria-live="polite" className={`inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-semibold border ${loadError ? 'bg-rose-500/10 border-rose-500/30 text-rose-400' : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400'}`}>
                <span className={`w-2 h-2 rounded-full ${loadError ? 'bg-rose-400' : 'bg-emerald-400'}`} />
                {loadError ? 'Gateway Tidak Tersedia' : 'Gateway Siap & Beroperasi'}
              </span>
              <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-mono bg-bg-surface-2 border border-border text-text-secondary">
                {healthyProvidersCount}/{providers.length || 0} Upstream Sehat
              </span>
            </div>

            <h2 className="text-2xl font-bold tracking-tight text-white">
              Route-X Gateway Control
            </h2>
            <p className="text-sm text-text-secondary max-w-2xl leading-relaxed">
              Gateway AI enterprise dengan perutean cerdas multi-provider, failover instan, proteksi SSRF,
              dan isolasi egress berkecepatan tinggi.
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-2.5 px-3.5 py-2 rounded-inner bg-bg-surface-2 border border-border text-xs font-mono text-text-primary">
              <span className="w-2 h-2 rounded-full bg-emerald-400" />
              <span className="font-semibold text-white">{overview?.in_flight_requests ?? 0}</span>
              <span className="text-text-muted">in-flight</span>
            </div>

            <Button
              variant="secondary"
              size="sm"
              onClick={() => loadData(true)}
              isLoading={isLoading || isRefreshing}
              icon={<RefreshCw className={`w-3.5 h-3.5 ${isRefreshing ? 'animate-spin' : ''}`} />}
            >
              Segarkan
            </Button>

            <Button
              variant="primary"
              size="sm"
              onClick={() => onNavigate('/requests')}
              icon={<ArrowUpRight className="w-3.5 h-3.5 text-black" />}
            >
              Request Log
            </Button>
          </div>
        </div>

        {/* Quick Cluster Info Pill Strip */}
        <div className="mt-6 pt-4 border-t border-border/70 flex flex-wrap items-center justify-between gap-4 text-xs font-mono text-text-secondary">
          <div className="flex flex-wrap items-center gap-4 sm:gap-6">
            <div className="flex items-center gap-1.5">
              <span className="text-text-muted">Runtime:</span>
              <span className="text-text-primary font-medium">{overview?.go_version || '-'}</span>
            </div>
            <div className="flex items-center gap-1.5">
              <span className="text-text-muted">OS/Arch:</span>
              <span className="text-text-primary font-medium">{overview?.os_arch || '-'}</span>
            </div>
            <div className="flex items-center gap-1.5">
              <span className="text-text-muted">Goroutines:</span>
              <span className="text-emerald-400 font-medium">{overview?.num_goroutine ?? 0}</span>
            </div>
            <div className="flex items-center gap-1.5">
              <span className="text-text-muted">Uptime:</span>
              <span className="text-text-primary font-medium">{overview ? formatUptime(overview.uptime_seconds) : '-'}</span>
            </div>
          </div>

          <button
            onClick={() => onNavigate('/cli-integrations')}
            className="text-xs text-accent hover:underline flex items-center gap-1 transition-colors"
          >
            <Terminal className="w-3.5 h-3.5" /> Integrasi CLI & SDK &rarr;
          </button>
        </div>
      </div>

      {/* ==================================================================== */}
      {/* 2. ONBOARDING & QUICK-START GUIDE (Ramah Pemula)                     */}
      {/* ==================================================================== */}
      {showOnboarding && (
        <div className="bg-bg-surface border border-accent/20 rounded-card p-6 shadow-sm relative overflow-hidden">
          <div className="flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Sparkles className="w-4 h-4 text-accent" />
                <h3 className="text-sm font-bold text-white tracking-tight">
                  Panduan Cepat Memulai Route-X
                </h3>
              </div>
              <p className="text-xs text-text-secondary leading-relaxed">
                Tiga langkah mudah untuk menghubungkan editor atau aplikasi Anda ke Route-X AI Gateway:
              </p>
            </div>
            <button
              onClick={dismissOnboarding}
              aria-label="Tutup panduan"
              className="text-text-muted hover:text-white p-1 rounded-nav hover:bg-bg-surface-2 transition-colors cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-5">
            {/* Step 1 */}
            <div
              onClick={() => onNavigate('/upstreams/providers')}
              className="p-4 rounded-inner bg-bg-surface-2 border border-border/80 hover:border-accent/50 cursor-pointer transition-all space-y-2 group"
            >
              <div className="flex items-center justify-between">
                <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-accent/10 text-accent font-semibold">
                  Langkah 1
                </span>
                <Server className="w-4 h-4 text-text-muted group-hover:text-accent transition-colors" />
              </div>
              <h4 className="text-xs font-bold text-white group-hover:text-accent transition-colors">
                Tambah Penyedia AI Upstream
              </h4>
              <p className="text-[11px] text-text-secondary leading-relaxed">
                Hubungkan API Key OpenAI, Anthropic, Gemini, DeepSeek, Groq, atau Ollama lokal Anda.
              </p>
            </div>

            {/* Step 2 */}
            <div
              onClick={() => onNavigate('/access/api-keys')}
              className="p-4 rounded-inner bg-bg-surface-2 border border-border/80 hover:border-accent/50 cursor-pointer transition-all space-y-2 group"
            >
              <div className="flex items-center justify-between">
                <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-accent/10 text-accent font-semibold">
                  Langkah 2
                </span>
                <KeyRound className="w-4 h-4 text-text-muted group-hover:text-accent transition-colors" />
              </div>
              <h4 className="text-xs font-bold text-white group-hover:text-accent transition-colors">
                Terbitkan Kunci API Klien
              </h4>
              <p className="text-[11px] text-text-secondary leading-relaxed">
                Buat kunci API terenkripsi untuk mengamankan dan mengontrol akses dari aplikasi/editor Anda.
              </p>
            </div>

            {/* Step 3 */}
            <div
              onClick={() => onNavigate('/cli-integrations')}
              className="p-4 rounded-inner bg-bg-surface-2 border border-border/80 hover:border-accent/50 cursor-pointer transition-all space-y-2 group"
            >
              <div className="flex items-center justify-between">
                <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-accent/10 text-accent font-semibold">
                  Langkah 3
                </span>
                <Terminal className="w-4 h-4 text-text-muted group-hover:text-accent transition-colors" />
              </div>
              <h4 className="text-xs font-bold text-white group-hover:text-accent transition-colors">
                Sambungkan Editor atau CLI
              </h4>
              <p className="text-[11px] text-text-secondary leading-relaxed">
                Gunakan template 1-klik untuk Claude Code, Cursor, Aider, Open WebUI, atau pustaka SDK Anda.
              </p>
            </div>
          </div>

          {/* Strip Resep Cepat 1-Klik untuk Pemula */}
          <div className="mt-5 pt-4 border-t border-border/70">
            <div className="flex items-center justify-between mb-3">
              <span className="text-xs font-bold text-white uppercase tracking-wider flex items-center gap-1.5 font-mono">
                <Sparkles className="w-3.5 h-3.5 text-accent" />
                Resep Cepat 1-Klik (Siap Pakai untuk Pemula)
              </span>
              <span className="text-[11px] text-text-muted hidden sm:inline">Pilih skenario penggunaan Anda</span>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <button
                type="button"
                onClick={() => setSelectedRecipe('coding')}
                className="p-3.5 rounded-inner bg-bg-surface-2/70 border border-border hover:border-sky-400/50 hover:bg-sky-500/5 transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-xs font-bold text-white group-hover:text-sky-300 transition-colors flex items-center gap-1.5">
                    🛠️ Coding Asisten
                  </span>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-sky-500/15 text-sky-300 font-mono">Cursor / Claude</span>
                </div>
                <p className="text-[11px] text-text-muted leading-relaxed">
                  Panduan &amp; konfigurasi instan untuk menghubungkan Cursor, Claude Code, atau Cline.
                </p>
              </button>

              <button
                type="button"
                onClick={() => setSelectedRecipe('budget')}
                className="p-3.5 rounded-inner bg-bg-surface-2/70 border border-border hover:border-emerald-400/50 hover:bg-emerald-500/5 transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-xs font-bold text-white group-hover:text-emerald-300 transition-colors flex items-center gap-1.5">
                    💰 Hemat Biaya 90%
                  </span>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-300 font-mono">DeepSeek / Groq</span>
                </div>
                <p className="text-[11px] text-text-muted leading-relaxed">
                  Alihkan prompt harian ke model hemat dengan fallback cerdas ke model flagship.
                </p>
              </button>

              <button
                type="button"
                onClick={() => setSelectedRecipe('uptime')}
                className="p-3.5 rounded-inner bg-bg-surface-2/70 border border-border hover:border-purple-400/50 hover:bg-purple-500/5 transition-all text-left group cursor-pointer"
              >
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-xs font-bold text-white group-hover:text-purple-300 transition-colors flex items-center gap-1.5">
                    🛡️ Anti-Downtime
                  </span>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-purple-500/15 text-purple-300 font-mono">Auto Failover</span>
                </div>
                <p className="text-[11px] text-text-muted leading-relaxed">
                  Kombinasi multi-upstream: jika OpenAI sibuk/error, otomatis beralih ke Claude/Gemini.
                </p>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ==================================================================== */}
      {/* 3. SYSTEM TELEMETRY — 4 LIVE RUNTIME CARDS                           */}
      {/* ==================================================================== */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <h3 className="text-sm font-bold text-white tracking-tight flex items-center gap-2">
              <Cpu className="w-4 h-4 text-accent" />
              Telemetri Runtime Gateway
            </h3>
            <span className="text-[10px] px-2 py-0.5 rounded-full bg-bg-surface-2 text-text-secondary border border-border font-mono">
              polling 5 detik
            </span>
          </div>
          <span className="text-xs text-text-secondary hidden sm:inline">
            Status alokasi memori, throughput jaringan, dan pool tunnel egress
          </span>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {/* Card 1: Memory */}
          <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
            <div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5">
                  <div className="p-2 rounded-inner bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                    <Cpu className="w-4 h-4" />
                  </div>
                  <div>
                    <span className="text-xs font-semibold text-white block">Memori Gateway</span>
                    <span className="text-[10px] text-text-muted font-mono">RSS Process</span>
                  </div>
                </div>
                <Badge variant="neutral" className="text-[10px] font-mono">
                  {overview?.go_version || '-'}
                </Badge>
              </div>

              <div className="mt-3">
                <div className="text-2xl font-bold text-white font-mono tracking-tight">
                  {overview ? formatBytes(overview.process_rss_bytes) : '-'}
                </div>
              </div>

              {/* Progress bar host RAM */}
              <div className="mt-3">
                <div className="flex items-center justify-between text-[10px] text-text-secondary mb-1 font-mono">
                  <span>{hostRAMPct.toFixed(1)}% dari host RAM</span>
                  <span>
                    {overview ? `${formatBytes(overview.host_ram_used_bytes)} / ${formatBytes(overview.host_ram_total_bytes)}` : '-'}
                  </span>
                </div>
                <div className="w-full h-1.5 bg-bg-surface-3 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-accent rounded-full transition-all duration-500"
                    style={{ width: `${hostRAMPct}%` }}
                  />
                </div>
              </div>
            </div>

            {/* Breakdown rows */}
            <div className="mt-4 pt-3 border-t border-border/60 space-y-1.5 text-xs">
              <div className="flex items-center justify-between">
                <span className="text-text-muted">RAM Kontainer</span>
                <span className="font-mono text-text-secondary">
                  {overview ? formatBytes(overview.container_ram_bytes) : '-'}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-text-muted">Heap Go</span>
                <span className="font-mono text-text-secondary">{overview ? formatBytes(overview.go_heap_bytes) : '-'}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-text-muted">Siklus GC</span>
                <span className="font-mono text-emerald-400 font-semibold">{overview?.num_gc ?? 0}</span>
              </div>
            </div>
          </div>

          {/* Card 2: Network */}
          <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
            <div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5">
                  <div className="p-2 rounded-inner bg-cyan-500/10 text-cyan-400 border border-cyan-500/20">
                    <Share2 className="w-4 h-4" />
                  </div>
                  <div>
                    <span className="text-xs font-semibold text-white block">Throughput I/O</span>
                    <span className="text-[10px] text-text-muted font-mono">Total Trafik Jaringan</span>
                  </div>
                </div>
                <div className="px-2 py-0.5 rounded-full bg-cyan-500/10 border border-cyan-500/20 text-[10px] font-mono text-cyan-400 font-semibold">
                  {((overview?.net_rate_mb_s ?? 0)).toFixed(1)} MB/s
                </div>
              </div>

              <div className="mt-3">
                <div className="text-2xl font-bold text-white font-mono tracking-tight">
                  {overview ? formatBytes(overview.net_total_bytes) : '-'}
                </div>
                <span className="text-[10px] text-text-muted font-mono block mt-0.5">Sejak server boot</span>
              </div>
            </div>

            {/* Breakdown rows with indicator bars */}
            <div className="mt-4 pt-3 border-t border-border/60 space-y-2.5 text-xs">
              <div>
                <div className="flex items-center justify-between mb-1">
                  <span className="text-text-muted flex items-center gap-1">
                    <ArrowDown className="w-3 h-3 text-cyan-400" /> Masuk (Rx)
                  </span>
                  <span className="font-mono text-text-secondary">
                    {overview ? `${formatBytes(overview.net_recv_bytes)} · ${(overview.net_recv_rate_mb_s ?? 0).toFixed(1)} MB/s` : '-'}
                  </span>
                </div>
                <div className="w-full h-1 bg-bg-surface-3 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-cyan-400 rounded-full transition-all duration-500"
                    style={{
                      width: `${
                        overview?.net_total_bytes && overview.net_total_bytes > 0
                          ? Math.min(100, Math.round(((overview.net_recv_bytes || 0) / overview.net_total_bytes) * 100))
                          : 0
                      }%`,
                    }}
                  />
                </div>
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <span className="text-text-muted flex items-center gap-1">
                    <ArrowUp className="w-3 h-3 text-emerald-400" /> Keluar (Tx)
                  </span>
                  <span className="font-mono text-text-secondary">
                    {overview ? `${formatBytes(overview.net_sent_bytes)} · ${(overview.net_sent_rate_mb_s ?? 0).toFixed(1)} MB/s` : '-'}
                  </span>
                </div>
                <div className="w-full h-1 bg-bg-surface-3 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-emerald-400 rounded-full transition-all duration-500"
                    style={{
                      width: `${
                        overview?.net_total_bytes && overview.net_total_bytes > 0
                          ? Math.min(100, Math.round(((overview.net_sent_bytes || 0) / overview.net_total_bytes) * 100))
                          : 0
                      }%`,
                    }}
                  />
                </div>
              </div>
            </div>
          </div>

          {/* Card 3: Egress Pool */}
          <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
            <div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5">
                  <div className="p-2 rounded-inner bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                    <ShieldCheck className="w-4 h-4" />
                  </div>
                  <div>
                    <span className="text-xs font-semibold text-white block">Egress & Tunnel Pool</span>
                    <span className="text-[10px] text-text-muted font-mono">Xray & Proxies</span>
                  </div>
                </div>
                <div className="px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-[10px] font-mono text-emerald-400 font-semibold">
                  {overview?.egress_active_mode || 'DIRECT'}
                </div>
              </div>

              <div className="mt-3">
                <div className="text-2xl font-bold text-white font-mono tracking-tight">
                  {overview?.egress_total_routes ?? 0}
                </div>
                <span className="text-[10px] text-text-muted font-mono block mt-0.5">Total jalur keluar terdaftar</span>
              </div>
            </div>

            {/* Breakdown rows */}
            <div className="mt-4 pt-3 border-t border-border/60 space-y-1.5 text-xs">
              <div className="flex items-center justify-between">
                <span className="text-text-muted">Xray & WARP</span>
                <span className="font-mono text-white font-semibold">{overview?.egress_xray_count ?? 0}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-text-muted">HTTP/SOCKS</span>
                <span className="font-mono text-text-secondary">{overview?.egress_http_count ?? 0}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-text-muted">Status Tunnel</span>
                {(overview?.egress_total_routes ?? 0) > 0 ? (
                  <span className="text-emerald-400 font-mono text-[11px] flex items-center gap-1 font-semibold">
                    <CheckCircle2 className="w-3 h-3" /> Beroperasi
                  </span>
                ) : (
                  <span className="text-text-muted font-mono text-[11px] flex items-center gap-1">
                    Direct Routing
                  </span>
                )}
              </div>
            </div>
          </div>

          {/* Card 4: Uptime & CPU */}
          <div className="bg-bg-surface border border-border rounded-card p-5 flex flex-col justify-between hover:border-border/80 transition-all shadow-sm">
            <div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5">
                  <div className="p-2 rounded-inner bg-amber-500/10 text-amber-400 border border-amber-500/20">
                    <Clock className="w-4 h-4" />
                  </div>
                  <div>
                    <span className="text-xs font-semibold text-white block">Uptime & Komputasi</span>
                    <span className="text-[10px] text-text-muted font-mono">PID {overview?.pid || '-'}</span>
                  </div>
                </div>
                <Badge variant="neutral" className="text-[10px] font-mono">
                  {((overview?.container_cpu_cap ?? 1)).toFixed(1)} cores
                </Badge>
              </div>

              <div className="mt-3">
                <div className="text-2xl font-bold text-white font-mono tracking-tight">
                  {overview ? formatUptime(overview.uptime_seconds) : '-'}
                </div>
              </div>

              {/* Progress bar Proxy CPU */}
              <div className="mt-3">
                <div className="flex items-center justify-between text-[10px] text-text-secondary mb-1 font-mono">
                  <span>Proxy CPU</span>
                  <span className="text-amber-400 font-bold">{((overview?.proxy_cpu_pct ?? 0)).toFixed(1)}%</span>
                </div>
                <div className="w-full h-1.5 bg-bg-surface-3 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-amber-400 rounded-full transition-all duration-500"
                    style={{ width: `${Math.min(100, overview?.proxy_cpu_pct || 0)}%` }}
                  />
                </div>
              </div>
            </div>

            {/* Breakdown rows */}
            <div className="mt-4 pt-3 border-t border-border/60 space-y-1.5 text-xs">
              <div className="flex items-center justify-between">
                <span className="text-text-muted">Host CPU</span>
                <span className="font-mono text-text-secondary">{((overview?.host_cpu_pct ?? 0)).toFixed(1)}%</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-text-muted">Goroutines</span>
                <span className="font-mono text-emerald-400 font-semibold">{overview?.num_goroutine ?? 0}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* ==================================================================== */}
      {/* 3. INFERENCE PERFORMANCE METRICS RIBBON (4 CARDS)                    */}
      {/* ==================================================================== */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-2.5 sm:gap-4">
        {/* Metric 1 */}
        <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-primary/40 transition-all shadow-sm flex flex-col justify-between">
          <div className="flex items-center justify-between gap-1">
            <span className="text-xs font-medium text-text-secondary truncate">
              Total Permintaan
            </span>
            <div className="p-1.5 sm:p-2 rounded-lg bg-primary/10 text-primary border border-primary/20 shrink-0">
              <Activity className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
            </div>
          </div>
          <div className="mt-2 sm:mt-3">
            <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
              {(summary?.total_requests ?? 0).toLocaleString()}
            </div>
            <div className="flex flex-wrap items-center gap-1 sm:gap-2 mt-1 sm:mt-1.5">
              <span className="text-[10px] sm:text-xs text-text-muted font-mono">
                {(summary?.success_requests ?? 0).toLocaleString()} ok
              </span>
              <span className="text-[#3A404F] text-[10px] sm:text-xs hidden xs:inline">•</span>
              <span className={`text-[10px] sm:text-xs font-mono font-semibold ${(summary?.error_rate ?? 0) > 5 ? 'text-rose-400' : 'text-emerald-400'}`}>
                {((summary?.error_rate ?? 0)).toFixed(1)}% err
              </span>
            </div>
          </div>
        </div>

        {/* Metric 2 */}
        <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-emerald-500/40 transition-all shadow-sm flex flex-col justify-between">
          <div className="flex items-center justify-between gap-1">
            <span className="text-xs font-medium text-text-secondary truncate">
              Total Token
            </span>
            <div className="p-1.5 sm:p-2 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shrink-0">
              <Zap className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
            </div>
          </div>
          <div className="mt-2 sm:mt-3">
            <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
              {((summary?.total_tokens ?? 0) >= 1000000)
                ? `${((summary?.total_tokens ?? 0) / 1000000).toFixed(2)}M`
                : `${((summary?.total_tokens ?? 0) / 1000).toFixed(1)}k`}
            </div>
            <div className="text-[10px] sm:text-xs text-text-muted mt-1 sm:mt-1.5 font-mono truncate">
              In: {((summary?.prompt_tokens ?? 0) / 1000).toFixed(1)}k · Out: {((summary?.completion_tokens ?? 0) / 1000).toFixed(1)}k
            </div>
          </div>
        </div>

        {/* Metric 3 */}
        <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-amber-500/40 transition-all shadow-sm flex flex-col justify-between">
          <div className="flex items-center justify-between gap-1">
            <span className="text-xs font-medium text-text-secondary truncate">
              Estimasi Biaya
            </span>
            <div className="p-1.5 sm:p-2 rounded-lg bg-amber-500/10 text-amber-400 border border-amber-500/20 shrink-0">
              <Coins className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
            </div>
          </div>
          <div className="mt-2 sm:mt-3">
            <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
              {formatUSD(summary?.total_cost_usd, 8)}
            </div>
            <div className="text-[10px] sm:text-xs text-text-muted mt-1 sm:mt-1.5 font-mono truncate">
              Presisi 8 desimal USD
            </div>
          </div>
        </div>

        {/* Metric 4 */}
        <div className="bg-[#121316] border border-[#20242D] rounded-xl p-3 sm:p-4.5 hover:border-purple-500/40 transition-all shadow-sm flex flex-col justify-between">
          <div className="flex items-center justify-between gap-1">
            <span className="text-xs font-medium text-text-secondary truncate">
              Latensi P95
            </span>
            <div className="p-1.5 sm:p-2 rounded-lg bg-purple-500/10 text-purple-400 border border-purple-500/20 shrink-0">
              <Clock className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
            </div>
          </div>
          <div className="mt-2 sm:mt-3">
            <div className="text-xl sm:text-2xl font-bold text-white font-mono tracking-tight truncate">
              {summary?.p95_latency_ms ?? 0} ms
            </div>
            <div className="text-[10px] sm:text-xs text-text-muted mt-1 sm:mt-1.5 font-mono truncate">
              Rata-rata: {summary?.avg_latency_ms ?? 0} ms
            </div>
          </div>
        </div>
      </div>

      {/* ==================================================================== */}
      {/* 4. TRAFFIC CHART & RECENT REQUESTS FEED (SIDE-BY-SIDE ON DESKTOP)     */}
      {/* ==================================================================== */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6">
        {/* Chart Column (2 cols) */}
        <div className="lg:col-span-2 bg-[#121316] border border-[#20242D] rounded-xl p-4 sm:p-5 shadow-lg flex flex-col justify-between">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-3 mb-3 sm:mb-4">
            <div className="flex items-center gap-2">
              <BarChart3 className="w-4 h-4 text-primary flex-shrink-0" />
              <h3 className="text-sm font-semibold text-white tracking-tight truncate">Volume & Dinamika Lalu Lintas</h3>
            </div>

            <div
              role="group"
              aria-label="Pilih jenis metrik dan rentang waktu grafik"
              className="flex items-center overflow-x-auto max-w-full bg-[#191C24] p-0.5 sm:p-1 rounded-lg border border-[#282D39] text-[11px] sm:text-xs font-mono scrollbar-none"
            >
              {/* Metric switcher */}
              {(['requests', 'tokens', 'latency'] as const).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => setMetricType(m)}
                  aria-pressed={metricType === m}
                  aria-label={m === 'requests' ? 'Jumlah request' : m === 'tokens' ? 'Jumlah token' : 'Latensi'}
                  className={`px-2 sm:px-3 py-1 sm:py-1.5 min-h-[28px] sm:min-h-[30px] rounded-md capitalize transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-primary ${
                    metricType === m
                      ? 'bg-primary text-black font-bold shadow'
                      : 'text-text-muted hover:text-white'
                  }`}
                >
                  {m === 'requests' ? 'Req' : m === 'tokens' ? 'Token' : 'Latensi'}
                </button>
              ))}
              <span aria-hidden="true" className="w-px self-stretch my-1 mx-0.5 sm:mx-1 bg-[#282D39]" />
              {/* Window switcher */}
              {(['1h', '6h', '24h', '7d'] as const).map((w) => (
                <button
                  key={w}
                  type="button"
                  onClick={() => setTimeWindow(w)}
                  aria-pressed={timeWindow === w}
                  aria-label={`Rentang waktu ${w}`}
                  className={`px-2 sm:px-2.5 py-1 sm:py-1.5 min-h-[28px] sm:min-h-[30px] rounded-md transition-all focus:outline-none focus-visible:ring-1 focus-visible:ring-primary ${
                    timeWindow === w
                      ? 'bg-[#2A2F3D] text-white font-semibold'
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
                <Activity className="w-6 h-6 text-[#2A2F3D]" />
                <span>Belum ada sampel metrik untuk rentang waktu {timeWindow}</span>
              </div>
            ) : (
              <MemoizedChart series={series} metricType={metricType} />
            )}
          </div>
        </div>

        {/* Live Recent Inference Requests Column (1 col) */}
        <div className="bg-[#121316] border border-[#20242D] rounded-xl p-5 shadow-lg flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between pb-3 border-b border-[#1C2029]">
              <div className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
                <h3 className="text-sm font-semibold text-white tracking-tight">Live Requests Feed</h3>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onNavigate('/requests')}
                className="text-xs text-primary hover:text-primary-hover p-0 h-auto"
              >
                Semua &rarr;
              </Button>
            </div>

            <div className="mt-3 space-y-2.5">
              {recentRequests.length === 0 ? (
                <div className="py-12 text-center text-xs text-text-muted space-y-2">
                  <Activity className="w-5 h-5 mx-auto text-[#2A2F3D]" />
                  <p>Belum ada permintaan inferensi yang tercatat.</p>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => onNavigate('/cli-integrations')}
                    className="mt-2 text-xs"
                  >
                    Buka Panduan Integrasi
                  </Button>
                </div>
              ) : (
                recentRequests.map((req) => {
                  const isOk = req.status_code >= 200 && req.status_code < 300;
                  const isErr = req.status_code >= 400;
                  return (
                    <div
                      key={req.id || req.request_id}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onNavigate('/requests'); } }}
                      onClick={() => onNavigate('/requests')}
                      className="p-2.5 rounded-lg bg-[#181A20] border border-[#232732] hover:border-primary/40 hover:bg-[#1D2028] transition-all cursor-pointer group"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2 min-w-0">
                          <span
                            className={`px-1.5 py-0.5 rounded text-[10px] font-mono font-bold ${
                              isOk
                                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                                : isErr
                                ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                                : 'bg-amber-500/10 text-amber-400'
                            }`}
                          >
                            {req.status_code}
                          </span>
                          <span className="text-xs font-mono font-semibold text-white truncate max-w-[140px] group-hover:text-primary transition-colors">
                            {req.requested_model || req.model_id || 'unknown'}
                          </span>
                        </div>
                        <span className="text-[10px] text-text-muted font-mono whitespace-nowrap">
                          {formatTimeAgo(req.created_at)}
                        </span>
                      </div>

                      <div className="mt-1.5 flex items-center justify-between text-[11px] font-mono text-text-secondary">
                        <span className="text-text-muted flex items-center gap-1">
                          {req.provider_name || 'auto-routed'}
                          {req.is_stream && (
                            <span className="text-[9px] px-1 rounded bg-[#252A36] text-primary">SSE</span>
                          )}
                        </span>
                        <div className="flex items-center gap-2">
                          <span className="text-text-primary">{req.duration_ms}ms</span>
                          <span className="text-text-muted">·</span>
                          <span className="text-text-muted">{req.total_tokens || 0} tok</span>
                        </div>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>

          <div className="mt-4 pt-3 border-t border-[#1C2029]">
            <button
              onClick={() => onNavigate('/requests')}
              className="w-full py-1.5 text-center text-xs text-text-secondary hover:text-white font-mono rounded bg-[#16181F] border border-[#262B37] hover:border-[#383E4F] transition-colors"
            >
              Inspeksi Seluruh Jejak Audit Request &rarr;
            </button>
          </div>
        </div>
      </div>

      {/* ==================================================================== */}
      {/* 5. UPSTREAM PROVIDER HEALTH & STATUS MATRIX                          */}
      {/* ==================================================================== */}
      <div className="space-y-3 sm:space-y-4">
        <div className="flex flex-row items-center justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <Server className="w-4 h-4 text-emerald-400 flex-shrink-0" />
            <h3 className="text-sm font-semibold text-white tracking-tight truncate">Status Kesehatan Upstream Providers</h3>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onNavigate('/upstreams/providers')}
            aria-label="Buka halaman kelola provider"
            className="text-xs text-primary hover:underline whitespace-nowrap flex-shrink-0 p-0 h-auto"
          >
            Kelola &rarr;
          </Button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4">
          {providers.length === 0 ? (
            <div className="col-span-full py-8 text-center text-xs text-text-muted bg-[#121316] border border-[#20242D] rounded-xl">
              Belum ada provider upstream yang terdaftar.{' '}
              <button
                onClick={() => onNavigate('/upstreams/providers')}
                className="text-primary hover:underline font-semibold"
              >
                Tambahkan provider sekarang
              </button>
            </div>
          ) : (
            providers.map((p) => {
              const isHealthy = p.last_health_status === 'healthy';
              const isDegraded = p.last_health_status === 'degraded';
              return (
                <div
                  key={p.id}
                  className="bg-bg-surface border border-border rounded-card p-4 hover:border-border/80 transition-all flex flex-col justify-between shadow-sm group"
                >
                  <div className="flex items-center justify-between gap-2.5 sm:gap-3">
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div className="w-8 h-8 sm:w-9 sm:h-9 rounded-lg sm:rounded-inner bg-bg-surface-2 border border-border flex items-center justify-center text-primary flex-shrink-0">
                        <ProviderBrandIcon providerIdOrKind={p.kind} name={p.name} className="w-4 h-4 sm:w-5 sm:h-5" />
                      </div>
                      <div className="min-w-0">
                        <h4 className="text-sm font-semibold text-white group-hover:text-primary transition-colors truncate">
                          {p.display_name || p.name}
                        </h4>
                        <p className="text-[11px] text-text-muted font-mono capitalize truncate">
                          {p.kind}
                          {p.last_latency_ms ? ` · ${p.last_latency_ms} ms` : ''}
                        </p>
                      </div>
                    </div>
                    <span className="flex-shrink-0">
                      <Badge variant={isHealthy ? 'success' : isDegraded ? 'warn' : 'error'}>
                        {p.last_health_status || (p.enabled ? 'unknown' : 'disabled')}
                      </Badge>
                    </span>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* Modal Panduan Resep 1-Klik */}
      <Modal
        isOpen={!!selectedRecipe}
        onClose={() => setSelectedRecipe(null)}
        title={
          selectedRecipe === 'coding'
            ? '🛠️ Resep: Hubungkan Coding Assistant (Cursor / Claude Code / Cline)'
            : selectedRecipe === 'budget'
            ? '💰 Resep: Hemat Biaya Inferensi Hingga 90%'
            : '🛡️ Resep: Anti-Downtime dengan Auto-Failover Multi-Provider'
        }
        subtitle="Panduan ramah pemula langkah-demi-langkah tanpa konfigurasi rumit"
        maxWidth="2xl"
        footer={
          <div className="flex items-center justify-between w-full">
            <Button variant="ghost" onClick={() => setSelectedRecipe(null)}>
              Tutup
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                const target = selectedRecipe === 'coding' ? '/cli-integrations' : '/upstreams/routing';
                setSelectedRecipe(null);
                onNavigate(target);
              }}
              icon={<ChevronRight className="w-4 h-4" />}
            >
              {selectedRecipe === 'coding' ? 'Buka Pengaturan CLI Lengkap' : 'Atur Routing Cerdas'}
            </Button>
          </div>
        }
      >
        <div className="space-y-4 text-xs">
          {selectedRecipe === 'coding' && (
            <>
              <div className="p-3.5 rounded-xl bg-sky-500/10 border border-sky-500/20 text-sky-200 leading-relaxed">
                Route-X mendukung <strong>Dual-Protocol</strong> native: OpenAI API (<code className="font-mono text-white">/v1/chat/completions</code>) dan Anthropic Claude (<code className="font-mono text-white">/v1/messages</code>) secara bersamaan!
              </div>

              <div className="space-y-2">
                <h4 className="font-semibold text-white">1. Parameter Sambungan Gateway</h4>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 font-mono text-[11px]">
                  <div className="p-2.5 rounded-lg bg-bg-surface-2 border border-border">
                    <span className="text-text-muted block text-[10px]">OpenAI Endpoint:</span>
                    <span className="text-accent">http://localhost:8080/v1</span>
                  </div>
                  <div className="p-2.5 rounded-lg bg-bg-surface-2 border border-border">
                    <span className="text-text-muted block text-[10px]">Anthropic Endpoint:</span>
                    <span className="text-accent">http://localhost:8080</span>
                  </div>
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <h4 className="font-semibold text-white">2. Ekspor Variabel Shell Seketika</h4>
                  <button
                    type="button"
                    onClick={() =>
                      handleCopyRecipeText(
                        `export OPENAI_BASE_URL="http://localhost:8080/v1"\nexport ANTHROPIC_BASE_URL="http://localhost:8080"\nexport OPENAI_API_KEY="your_routex_api_key"\nexport ANTHROPIC_API_KEY="your_routex_api_key"`
                      )
                    }
                    className="inline-flex items-center gap-1 text-[11px] text-accent hover:underline font-mono cursor-pointer"
                  >
                    {recipeCopied ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                    <span>{recipeCopied ? 'Tersalin!' : 'Salin Snippet'}</span>
                  </button>
                </div>
                <pre className="p-3 rounded-xl bg-bg-surface-2 border border-border font-mono text-[11px] text-text-primary overflow-x-auto leading-relaxed">
{`export OPENAI_BASE_URL="http://localhost:8080/v1"
export ANTHROPIC_BASE_URL="http://localhost:8080"
export OPENAI_API_KEY="your_routex_api_key"
export ANTHROPIC_API_KEY="your_routex_api_key"`}
                </pre>
              </div>
            </>
          )}

          {selectedRecipe === 'budget' && (
            <>
              <div className="p-3.5 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-200 leading-relaxed">
                Hemat anggaran token hingga <strong>90%</strong> dengan mengarahkan percakapan rutin ke model ultra-hemat (DeepSeek V3 / Groq LLaMA 3.3) dan hanya beralih ke model flagship saat diperlukan.
              </div>

              <div className="space-y-2">
                <h4 className="font-semibold text-white">Langkah Mudah Penerapan:</h4>
                <ol className="list-decimal list-inside space-y-1.5 text-text-secondary">
                  <li>Buka menu <strong className="text-white">Penyedia AI</strong> dan tambahkan API Key DeepSeek atau Groq.</li>
                  <li>Buka menu <strong className="text-white">Perutean Cerdas</strong>, pilih mode <strong className="text-accent">Combo Cascade</strong>.</li>
                  <li>Setel <em>Tier 1 (Utama)</em> ke DeepSeek V3, dan <em>Tier 2 (Cadangan)</em> ke Claude 3.5 Sonnet / GPT-4o.</li>
                </ol>
              </div>
            </>
          )}

          {selectedRecipe === 'uptime' && (
            <>
              <div className="p-3.5 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-200 leading-relaxed">
                Jaminan keandalan tinggi (*High Availability*): Jika OpenAI mengalami lonjakan error (503 / 500) atau batas kuota (429), Route-X otomatis mengalihkan request ke Anthropic atau Google dalam &lt; 200 milidetik.
              </div>

              <div className="space-y-2">
                <h4 className="font-semibold text-white">Langkah Pengaktifan:</h4>
                <ol className="list-decimal list-inside space-y-1.5 text-text-secondary">
                  <li>Pastikan minimal 2 Penyedia AI upstream terhubung (misal: OpenAI + Anthropic).</li>
                  <li>Buka menu <strong className="text-white">Perutean Cerdas</strong>, pilih mode <strong className="text-purple-300">Failover (Priority)</strong>.</li>
                  <li>Centang kedua provider; Route-X akan otomatis menangani pemulihan saat provider utama sibuk.</li>
                </ol>
              </div>
            </>
          )}
        </div>
      </Modal>
    </div>
  );
};
