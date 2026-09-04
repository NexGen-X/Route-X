import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { AuditLogEntry } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { RefreshCw, Eye } from 'lucide-react';

export const AuditLogs: React.FC = () => {
  const [logs, setLogs] = useState<AuditLogEntry[]>([]);
  const [selectedEntry, setSelectedEntry] = useState<AuditLogEntry | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const loadLogs = async () => {
    setIsLoading(true);
    try {
      const res = await api.system.auditLogs({ limit: 50 });
      setLogs(res.items || []);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadLogs();
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">System Audit Trail</h2>
          <p className="text-xs text-text-secondary mt-1">
            Rekaman aktivitas administratif yang tidak dapat diubah untuk kepatuhan dan audit keamanan.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={loadLogs} isLoading={isLoading}>
          <RefreshCw className="w-3.5 h-3.5" />
        </Button>
      </div>

      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-border text-text-muted uppercase tracking-wider text-[11px] bg-bg-surface-2/40">
                <th className="py-3 px-4 font-semibold">Waktu & IP</th>
                <th className="py-3 px-4 font-semibold">Aktor / Email</th>
                <th className="py-3 px-4 font-semibold">Aksi</th>
                <th className="py-3 px-4 font-semibold">Entitas & Target</th>
                <th className="py-3 px-4 text-right">Detail</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/60">
              {logs.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-text-muted">
                    {isLoading ? 'Memuat rekaman jejak audit...' : 'Belum ada rekaman audit log.'}
                  </td>
                </tr>
              ) : (
                logs.map((l) => (
                  <tr key={l.id} className="hover:bg-bg-surface-2/40">
                    <td className="py-3 px-4">
                      <div className="text-white">{new Date(l.occurred_at).toLocaleTimeString()}</div>
                      <div className="text-[11px] text-text-muted font-mono">{l.ip || '-'}</div>
                    </td>
                    <td className="py-3 px-4">
                      <div className="font-semibold text-text-primary">{l.actor_email || 'Sistem'}</div>
                      <div className="text-[11px] text-accent font-mono">{l.actor_role || '-'}</div>
                    </td>
                    <td className="py-3 px-4">
                      <Badge variant="lime" size="sm">
                        {l.action.toUpperCase()}
                      </Badge>
                    </td>
                    <td className="py-3 px-4">
                      <div className="font-semibold text-white capitalize">{l.resource_type}</div>
                      <div className="text-[11px] text-text-muted font-mono truncate max-w-[160px]">
                        {l.resource_id || '-'}
                      </div>
                    </td>
                    <td className="py-3 px-4 text-right">
                      <button
                        onClick={() => setSelectedEntry(l)}
                        className="p-1.5 text-text-muted hover:text-accent rounded-nav transition-colors"
                        title="Lihat Metadata"
                      >
                        <Eye className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {selectedEntry && (
        <Modal
          isOpen={true}
          onClose={() => setSelectedEntry(null)}
          title="Detail Jejak Audit"
          subtitle={`Aksi: ${selectedEntry.action} pada ${selectedEntry.resource_type}`}
        >
          <div className="space-y-3 text-xs">
            <div>
              <span className="text-text-muted block mb-1">Metadata Perubahan (JSON):</span>
              <pre className="p-3 bg-black/60 rounded-inner border border-border font-mono text-[11px] overflow-x-auto text-accent">
                {JSON.stringify(selectedEntry.metadata || {}, null, 2)}
              </pre>
            </div>
            <div className="text-[11px] text-text-muted font-mono">
              User Agent: {selectedEntry.user_agent || '-'}
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
};
