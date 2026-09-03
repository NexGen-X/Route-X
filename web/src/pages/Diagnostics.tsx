import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Diagnostics as DiagType } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { HeartPulse, Cpu, Database, RefreshCw } from 'lucide-react';

export const Diagnostics: React.FC = () => {
  const [diag, setDiag] = useState<DiagType | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const loadDiag = async () => {
    setIsLoading(true);
    try {
      const res = await api.system.diagnostics();
      setDiag(res);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadDiag();
  }, []);

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
        <Button variant="secondary" size="sm" onClick={loadDiag} isLoading={isLoading}>
          <RefreshCw className="w-3.5 h-3.5" />
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <Card title="Runtime & Versi" subtitle="Identifikasi biner dan uptime proses">
          <div className="space-y-3 text-xs">
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Versi Gateway</span>
              <span className="font-mono font-bold text-accent">{diag?.version}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Git Commit</span>
              <span className="font-mono text-white">{diag?.commit}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Go Compiler</span>
              <span className="font-mono text-text-secondary">{diag?.go_version}</span>
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
              <span className="font-mono font-bold text-white">{diag?.memory_allocated_mb} MB</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Total Allocations</span>
              <span className="font-mono text-text-secondary">{diag?.memory_total_alloc_mb} MB</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">System Reserved</span>
              <span className="font-mono text-text-secondary">{diag?.memory_sys_mb} MB</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">GC Cycles</span>
              <span className="font-mono text-accent">{diag?.num_gc} kali</span>
            </div>
          </div>
        </Card>

        <Card title="PostgreSQL Pool" subtitle="Koneksi terkelola pgxpool">
          <div className="space-y-3 text-xs">
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Total Koneksi</span>
              <span className="font-mono font-bold text-accent">{diag?.db_pool.total_conns}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Koneksi Idle</span>
              <span className="font-mono text-white">{diag?.db_pool.idle_conns}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Koneksi Aktif (Acquired)</span>
              <span className="font-mono text-white">{diag?.db_pool.acquired_conns}</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">Batas Maksimal</span>
              <span className="font-mono text-text-secondary">{diag?.db_pool.max_conns}</span>
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
};
