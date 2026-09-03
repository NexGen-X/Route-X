import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Session } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Smartphone, LogOut, RefreshCw } from 'lucide-react';

export const Sessions: React.FC = () => {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [isLoading, setIsLoading] = useState(false);

  const loadSessions = async () => {
    setIsLoading(true);
    try {
      const res = await api.sessions.list();
      setSessions(res.items || []);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadSessions();
  }, []);

  const handleRevoke = async (id: string) => {
    if (!confirm('Putuskan sesi aktif ini?')) return;
    try {
      await api.sessions.revoke(id);
      loadSessions();
    } catch (err) {
      alert('Gagal memutuskan sesi: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Active Sessions</h2>
          <p className="text-xs text-text-secondary mt-1">
            Daftar sesi login aktif di seluruh perangkat dengan pemutusan sesi instan.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={loadSessions} isLoading={isLoading}>
          <RefreshCw className="w-3.5 h-3.5" />
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {sessions.map((s) => (
          <Card key={s.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-blue-500/10 text-blue-400 flex items-center justify-center">
                  <Smartphone className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-bold text-white">{s.user_name || s.user_email}</h4>
                  <span className="text-[11px] text-text-muted font-mono">{s.ip}</span>
                </div>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="py-1 border-b border-border/40">
                  <span className="text-text-muted block text-[10px] uppercase">User Agent</span>
                  <span className="font-mono text-text-secondary truncate block" title={s.user_agent}>
                    {s.user_agent}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Terakhir Aktif</span>
                  <span className="font-mono text-white">{new Date(s.last_seen_at).toLocaleTimeString()}</span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex justify-end">
              <Button
                variant="danger"
                size="sm"
                onClick={() => handleRevoke(s.id)}
                icon={<LogOut className="w-3.5 h-3.5" />}
              >
                Putuskan Sesi
              </Button>
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
};
