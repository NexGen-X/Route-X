import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Diagnostics as DiagType, BackgroundJob } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { RefreshCw, Clock, Play } from 'lucide-react';

export const Diagnostics: React.FC = () => {
  const [diag, setDiag] = useState<DiagType | null>(null);
  const [jobs, setJobs] = useState<BackgroundJob[]>([]);
  const [triggeringJob, setTriggeringJob] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const loadData = async () => {
    setIsLoading(true);
    try {
      const [resDiag, resJobs] = await Promise.all([
        api.system.diagnostics(),
        api.system.jobs().catch(() => ({ items: [] })),
      ]);
      setDiag(resDiag);
      setJobs(resJobs.items || []);
    } catch (err) {
      console.error(err);
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
      alert(res.message || 'Job berhasil dipicu.');
      loadData();
    } catch (err) {
      alert('Gagal memicu job: ' + err);
    } finally {
      setTriggeringJob(null);
    }
  };

  const formatUptime = (sec: number) => {
    const d = Math.floor(sec / 86400);
    const h = Math.floor((sec % 86400) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    return `${d}h ${h}j ${m}m ${s}d`;
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">System Diagnostics</h2>
          <p className="text-xs text-text-secondary mt-1">
            Status runtime Go, alokasi memori heap/stack, garbage collector, dan koneksi pgxpool.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={loadData} isLoading={isLoading}>
          <RefreshCw className="w-3.5 h-3.5" />
        </Button>
      </div>

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
              <span className="text-text-muted">Memory Allocated</span>
              <span className="font-mono font-bold text-white">
                {diag?.memory?.alloc_bytes ? (diag.memory.alloc_bytes / 1024 / 1024).toFixed(1) : (diag?.memory_allocated_mb ?? '-')} MB
              </span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Total Allocations</span>
              <span className="font-mono text-text-secondary">
                {diag?.memory?.total_alloc_bytes ? (diag.memory.total_alloc_bytes / 1024 / 1024).toFixed(1) : (diag?.memory_total_alloc_mb ?? '-')} MB
              </span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">System Reserved</span>
              <span className="font-mono text-text-secondary">
                {diag?.memory?.sys_bytes ? (diag.memory.sys_bytes / 1024 / 1024).toFixed(1) : (diag?.memory_sys_mb ?? '-')} MB
              </span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">GC Cycles</span>
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
          {jobs.length === 0 ? (
            <div className="col-span-3 p-6 text-center rounded-box bg-bg-surface-1 border border-border/40 text-xs text-text-muted">
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
