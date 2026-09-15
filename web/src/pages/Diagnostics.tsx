import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Diagnostics as DiagType, BackgroundJob, ResponseCacheStats } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Tooltip } from '../components/common/Tooltip';
import { RefreshCw, Clock, Play, Zap, Trash2, CheckCircle2 } from 'lucide-react';
import { PageHeader } from '../components/common/PageHeader';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export const Diagnostics: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [diag, setDiag] = useState<DiagType | null>(null);
  const [jobs, setJobs] = useState<BackgroundJob[]>([]);
  const [cache, setCache] = useState<ResponseCacheStats | null>(null);
  const [triggeringJob, setTriggeringJob] = useState<string | null>(null);
  const [flushingCache, setFlushingCache] = useState(false);
  const [updatingCache, setUpdatingCache] = useState(false);
  const [ttlMinutes, setTtlMinutes] = useState<number>(60);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const loadData = async () => {
    setIsLoading(true);
    try {
      // Muat bertahap: cache boleh gagal (engine tidak terpasang di
      // produksi) tanpa menenggelamkan diag + jobs yang sudah lolos 200.
      const [resDiag, resJobs] = await Promise.all([
        api.system.diagnostics(),
        api.system.jobs(),
      ]);
      setDiag(resDiag);
      setJobs(resJobs.items || []);
      try {
        const resCache = await api.system.cacheStats();
        setCache(resCache);
        const ttl = Number(resCache.ttl_seconds);
        setTtlMinutes(Number.isFinite(ttl) ? Math.min(10080, Math.max(1, Math.floor(ttl / 60))) : 60);
      } catch (errCache) {
        setCache(null);
      }
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleTriggerJob = async (name: string) => {
    setTriggeringJob(name);
    try {
      const res = await api.system.triggerJob(name);
      toast.success(res.message || 'Pekerjaan berhasil dipicu di latar belakang.', 'Worker Triggered');
      loadData();
    } catch (err: any) {
      toast.error('Gagal memicu worker: ' + (err.message || err));
    } finally {
      setTriggeringJob(null);
    }
  };

  const handleFlushCache = async () => {
    const ok = await confirmModal({
      title: 'Kosongkan Response Cache?',
      message: 'Apakah Anda yakin ingin mengosongkan seluruh entri cache inferensi di Redis? Permintaan berikutnya akan langsung menuju ke upstream.',
      confirmText: 'Ya, Kosongkan',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;

    setFlushingCache(true);
    try {
      const res = await api.system.flushCache();
      toast.success(res.message || 'Response cache berhasil dibersihkan.', 'Cache Flushed');
      loadData();
    } catch (err: any) {
      toast.error('Gagal membersihkan cache: ' + (err.message || err));
    } finally {
      setFlushingCache(false);
    }
  };

  const handleToggleCache = async (newEnabled: boolean) => {
    setUpdatingCache(true);
    try {
      await api.system.updateCacheSettings(newEnabled, ttlMinutes * 60);
      toast.info(newEnabled ? 'Response caching diaktifkan (< 2ms)' : 'Response caching dinonaktifkan', 'Pengaturan Cache');
      loadData();
    } catch (err: any) {
      toast.error('Gagal memperbarui pengaturan cache: ' + (err.message || err));
    } finally {
      setUpdatingCache(false);
    }
  };

  const handleSaveTTL = async () => {
    if (!cache) return;
    const ttl = Math.min(10080, Math.max(1, Math.floor(Number(ttlMinutes) || 60)));
    setUpdatingCache(true);
    try {
      await api.system.updateCacheSettings(cache.enabled, ttl * 60);
      toast.success(`TTL Response cache berhasil diperbarui ke ${ttl} menit.`, 'TTL Tersimpan');
      loadData();
    } catch (err: any) {
      toast.error('Gagal menyimpan TTL: ' + (err.message || err));
    } finally {
      setUpdatingCache(false);
    }
  };

  const formatUptime = (sec: number) => {
    const d = Math.floor(sec / 86400);
    const h = Math.floor((sec % 86400) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    return `${d} hari ${h} jam ${m} mnt ${s} dtk`;
  };

  const totalReq = (cache?.hits || 0) + (cache?.misses || 0);
  const hitRatio = totalReq > 0 ? (((cache?.hits || 0) / totalReq) * 100).toFixed(1) : '0.0';

  return (
    <div className="space-y-6">
      <PageHeader
        title="System Diagnostics"
        description="Status runtime Go, alokasi memori heap/stack, garbage collector, dan koneksi pgxpool."
        actions={
          <Button variant="secondary" size="sm" onClick={loadData} isLoading={isLoading} title="Muat ulang data diagnostik">
            <RefreshCw className="w-3.5 h-3.5" />
          </Button>
        }
      />

      {loadError && <QueryError message={loadError} onRetry={() => void loadData()} />}

      {/* Response Cache Card (Fitur 1) */}
      <div className="bg-bg-surface-2/40 border border-border/80 rounded-xl p-5 shadow-lg backdrop-blur-sm">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-border/60">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-lg bg-accent/10 border border-accent/20 text-accent">
              <Zap className="w-5 h-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="text-base font-semibold text-white">⚡ Redis Response Caching</h3>
                <Badge variant={cache?.enabled ? 'success' : 'neutral'}>
                  {cache?.enabled ? 'ACTIVE (< 2ms)' : 'DISABLED'}
                </Badge>
              </div>
              <p className="text-xs text-text-secondary mt-0.5">
                Mengembalikan respons inferensi identik dalam &lt; 2ms tanpa biaya token ($0.00) ke upstream provider.
              </p>
            </div>
          </div>
          <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 w-full sm:w-auto">
            <Button
              variant={cache?.enabled ? 'danger' : 'primary'}
              size="sm"
              isLoading={updatingCache}
              onClick={() => handleToggleCache(!cache?.enabled)}
            >
              {cache?.enabled ? 'Nonaktifkan Cache' : 'Aktifkan Cache'}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              isLoading={flushingCache}
              onClick={handleFlushCache}
              className="text-red-400 hover:text-red-300 border-red-900/40 hover:bg-red-950/20"
            >
              <Trash2 className="w-3.5 h-3.5 mr-1" />
              Kosongkan Cache
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mt-4">
          <div className="bg-bg-surface-2 rounded-lg p-3 border border-border/40">
            <span className="text-[11px] text-text-muted block">Rasio Hit Cache</span>
            <span className="text-xl font-bold font-mono text-emerald-400 mt-1 block">
              {hitRatio}%
            </span>
            <span className="text-[10px] text-text-secondary block mt-0.5">
              {cache?.hits || 0} hit / {totalReq} total
            </span>
          </div>

          <div className="bg-bg-surface-2 rounded-lg p-3 border border-border/40">
            <span className="text-[11px] text-text-muted block">Hit Cache (Hemat Biaya)</span>
            <span className="text-xl font-bold font-mono text-white mt-1 block">
              {cache?.hits || 0}
            </span>
            <span className="text-[10px] text-emerald-400/80 block mt-0.5">
              Latensi &lt; 2ms, $0.00
            </span>
          </div>

          <div className="bg-bg-surface-2 rounded-lg p-3 border border-border/40">
            <span className="text-[11px] text-text-muted block">Total Entri di Redis</span>
            <span className="text-xl font-bold font-mono text-accent mt-1 block">
              {cache?.total_entries || 0}
            </span>
            <span className="text-[10px] text-text-secondary block mt-0.5">
              Prompt aktif tersimpan
            </span>
          </div>

          <div className="bg-bg-surface-2 rounded-lg p-3 border border-border/40 flex flex-col justify-between">
            <div>
              <span className="text-[11px] text-text-muted block">Durasi Simpan (TTL)</span>
              <div className="flex items-center gap-1.5 mt-1">
                <input
                  id="cache-ttl-minutes"
                  aria-label="Durasi simpan cache dalam menit"
                  type="number"
                  min="1"
                  max="10080"
                  value={ttlMinutes}
                  onChange={(e) => setTtlMinutes(Math.max(1, parseInt(e.target.value) || 1))}
                  className="w-16 bg-bg-base border border-border/80 rounded px-1.5 py-0.5 text-xs text-white font-mono focus:border-accent focus:outline-none"
                />
                <span className="text-xs text-text-secondary">menit</span>
                <Tooltip content="Simpan TTL" position="left">
                  <button
                    type="button"
                    onClick={handleSaveTTL}
                    disabled={updatingCache}
                    className="p-1.5 rounded bg-accent/20 hover:bg-accent/30 text-accent ml-auto text-xs min-w-[28px] min-h-[28px] flex items-center justify-center cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
                    aria-label="Simpan TTL"
                  >
                    <CheckCircle2 className="w-3.5 h-3.5" />
                  </button>
                </Tooltip>
              </div>
            </div>
            <span className="text-[10px] text-text-muted mt-1 block">
              {ttlMinutes >= 60
                ? `Sekitar ${(ttlMinutes / 60).toFixed(1)} jam masa berlaku`
                : `Sekitar ${ttlMinutes} menit masa berlaku`}
            </span>
          </div>
        </div>
      </div>

      {isLoading && !diag ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <Card key={i} className="p-5 space-y-4 animate-pulse">
              <div className="h-4 bg-bg-surface-2 rounded w-1/2" />
              <div className="space-y-2 pt-2">
                <div className="h-3 bg-bg-surface-2 rounded w-full" />
                <div className="h-3 bg-bg-surface-2 rounded w-4/5" />
                <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
              </div>
            </Card>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <Card title="Runtime & Versi" subtitle="Identifikasi biner dan uptime proses">
          <div className="space-y-3 text-xs">
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Versi Gateway</span>
              <span className="font-mono font-bold text-accent">{diag?.version || 'tidak tersedia'}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Git Commit</span>
              <span className="font-mono text-white">{diag?.commit || 'tidak tersedia'}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Go Compiler</span>
              <span className="font-mono text-text-secondary">{diag?.go_version || 'tidak tersedia'}</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">Uptime Proses</span>
              <span className="font-mono text-accent font-semibold">
                {diag ? formatUptime(diag.uptime_seconds) : '-'}
              </span>
            </div>
          </div>
        </Card>

        <Card title="Penggunaan Memori" subtitle="Statistik memori Go runtime">
          <div className="space-y-3 text-xs">
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Memori Terpakai</span>
              <span className="font-mono font-bold text-white">
                {diag?.memory?.alloc_bytes ? (diag.memory.alloc_bytes / 1024 / 1024).toFixed(1) : (diag?.memory_allocated_mb ?? '-')} MB
              </span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Total Alokasi</span>
              <span className="font-mono text-text-secondary">
                {diag?.memory?.total_alloc_bytes ? (diag.memory.total_alloc_bytes / 1024 / 1024).toFixed(1) : (diag?.memory_total_alloc_mb ?? '-')} MB
              </span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Memori Sistem</span>
              <span className="font-mono text-text-secondary">
                {diag?.memory?.sys_bytes ? (diag.memory.sys_bytes / 1024 / 1024).toFixed(1) : (diag?.memory_sys_mb ?? '-')} MB
              </span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">Siklus GC</span>
              <span className="font-mono text-accent">{diag?.memory?.num_gc ?? diag?.num_gc ?? 0} kali</span>
            </div>
          </div>
        </Card>

        <Card title="PostgreSQL Pool" subtitle="Koneksi terkelola pgxpool">
          <div className="space-y-3 text-xs">
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Total Koneksi</span>
              <span className="font-mono font-bold text-accent">{diag?.db_pool?.total_conns ?? '-'}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Koneksi Idle</span>
              <span className="font-mono text-white">{diag?.db_pool?.idle_conns ?? '-'}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Koneksi Aktif (Acquired)</span>
              <span className="font-mono text-white">{diag?.db_pool?.acquired_conns ?? '-'}</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">Batas Maksimal</span>
              <span className="font-mono text-text-secondary">{diag?.db_pool?.max_conns ?? '-'}</span>
            </div>
          </div>
        </Card>
      </div>
      )}

      {/* Background Workers Section */}
      <div className="pt-6 border-t border-border/40">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-bold text-white tracking-tight flex items-center gap-2">
              <Clock className="w-4 h-4 text-accent" />
              Supervisor Background Workers & Tasks
            </h3>
            <p className="text-xs text-text-secondary mt-0.5">
              Status eksekusi rutin latar belakang: health check provider, rollup penggunaan, dan retensi data.
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {isLoading && jobs.length === 0 ? (
            [1, 2, 3].map((i) => (
              <Card key={i} className="p-4 space-y-3 animate-pulse">
                <div className="flex justify-between">
                  <div className="h-4 bg-bg-surface-2 rounded w-1/2" />
                  <div className="h-5 bg-bg-surface-2 rounded w-12" />
                </div>
                <div className="space-y-1.5 pt-1">
                  <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
                  <div className="h-3 bg-bg-surface-2 rounded w-1/2" />
                </div>
              </Card>
            ))
          ) : jobs.length === 0 ? (
            <div className="col-span-3 p-6 text-center rounded-box bg-bg-surface-2 border border-border/40 text-xs text-text-muted">
              Tidak ada worker latar belakang yang terdaftar atau sedang berjalan.
            </div>
          ) : (
            jobs.map((j) => (
              <Card key={j.name} className="p-4 flex flex-col justify-between">
                <div>
                  <div className="flex items-start justify-between">
                    <div>
                      <h4 className="text-sm font-bold text-white font-mono">{j.name}</h4>
                      <span className="text-[11px] text-text-muted">Interval: {j.interval}</span>
                    </div>
                    <Badge variant={j.last_status === 'ok' ? 'success' : j.last_status === 'running' ? 'warn' : 'error'}>
                      {j.last_status?.toUpperCase() || 'IDLE'}
                    </Badge>
                  </div>

                  <div className="mt-3 text-xs space-y-1">
                    <div className="flex justify-between text-text-muted">
                      <span>Eksekusi Terakhir:</span>
                      <span className="font-mono text-white">
                        {j.last_run_at ? new Date(j.last_run_at).toLocaleTimeString() : 'Belum pernah'}
                      </span>
                    </div>
                    <div className="flex justify-between text-text-muted">
                      <span>Durasi Terakhir:</span>
                      <span className="font-mono text-accent">
                        {j.last_duration_ms ?? 0} ms
                      </span>
                    </div>
                  </div>
                </div>

                <div className="mt-4 pt-3 border-t border-border">
                  <Button
                    variant="secondary"
                    size="sm"
                    className="w-full"
                    onClick={() => handleTriggerJob(j.name)}
                    isLoading={triggeringJob === j.name}
                    icon={<Play className="w-3.5 h-3.5 text-accent" />}
                  >
                    Jalankan Sekarang
                  </Button>
                </div>
              </Card>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
