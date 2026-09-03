import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { BackgroundJob } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Clock, Play, RefreshCw } from 'lucide-react';

export const Jobs: React.FC = () => {
  const [jobs, setJobs] = useState<BackgroundJob[]>([]);
  const [triggeringJob, setTriggeringJob] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const loadJobs = async () => {
    setIsLoading(true);
    try {
      const res = await api.system.jobs();
      setJobs(res.items || []);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadJobs();
  }, []);

  const handleTrigger = async (name: string) => {
    setTriggeringJob(name);
    try {
      const res = await api.system.triggerJob(name);
      alert(res.message || 'Job berhasil dipicu.');
      loadJobs();
    } catch (err) {
      alert('Gagal memicu job: ' + err);
    } finally {
      setTriggeringJob(null);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Background Workers & Jobs</h2>
          <p className="text-xs text-text-secondary mt-1">
            Supervisor tugas latar belakang Route-X (Health Checker, Usage Rollup, Retention, Partitions, Webhook Worker).
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={loadJobs} isLoading={isLoading}>
          <RefreshCw className="w-3.5 h-3.5" />
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {jobs.map((j) => (
          <Card key={j.name} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-accent/10 text-accent flex items-center justify-center">
                    <Clock className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white font-mono">{j.name}</h4>
                    <span className="text-[11px] text-text-muted">Interval: {j.interval}</span>
                  </div>
                </div>
                <Badge variant={j.last_status === 'ok' ? 'success' : 'error'}>
                  {j.last_status?.toUpperCase() || 'IDLE'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Eksekusi Terakhir</span>
                  <span className="font-mono text-text-secondary">
                    {j.last_run_at ? new Date(j.last_run_at).toLocaleTimeString() : 'Belum pernah'}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Durasi Terakhir</span>
                  <span className="font-mono text-white">{j.last_duration_ms ?? 0} ms</span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex justify-end">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleTrigger(j.name)}
                isLoading={triggeringJob === j.name}
                icon={<Play className="w-3.5 h-3.5" />}
              >
                Jalankan Sekarang
              </Button>
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
};
