import React from 'react';
import { Network, Play, Edit2, Trash2, Activity, AlertCircle } from 'lucide-react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import type { EgressPool } from '../../types';

export interface EgressPoolCardProps {
  pool: EgressPool;
  isTesting: boolean;
  onTestProbe: (id: string) => void;
  onEdit: (pool: EgressPool) => void;
  onDelete: (id: string, name: string) => void;
}

export const EgressPoolCard: React.FC<EgressPoolCardProps> = ({
  pool,
  isTesting,
  onTestProbe,
  onEdit,
  onDelete,
}) => {
  return (
    <Card className="p-5 flex flex-col justify-between border-border hover:border-accent/30 transition-all">
      <div>
        {/* Header Kartu */}
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl flex items-center justify-center border bg-blue-500/10 border-blue-500/20 text-blue-400">
              <Network className="w-5 h-5" />
            </div>
            <div>
              <h4 className="text-sm font-bold text-white flex items-center gap-1.5">
                {pool.name || 'tanpa nama'}
              </h4>
              <span className="text-[11px] text-text-muted font-mono uppercase">
                {(pool.kind || 'PROXY')} • {pool.region || 'global'}
              </span>
            </div>
          </div>
          <Badge variant={pool.enabled ? 'success' : 'neutral'}>
            {pool.enabled ? 'active' : 'disabled'}
          </Badge>
        </div>

        {/* Detail Info & Status Kesehatan */}
        <div className="mt-4 space-y-2 text-xs">
          <div className="flex justify-between py-1 border-b border-border/40">
            <span className="text-text-muted">Status Koneksi</span>
            {pool.last_health_status === 'healthy' ? (
              <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-400">
                <Activity className="w-3 h-3 text-emerald-400" />
                Terhubung ({pool.last_latency_ms || 0} ms)
              </span>
            ) : pool.last_health_status === 'unhealthy' ? (
              <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-red-400">
                <AlertCircle className="w-3 h-3 text-red-400" />
                Tidak Terhubung
              </span>
            ) : (
              <span className="text-text-muted text-[11px]">Belum Diuji</span>
            )}
          </div>

          <div className="flex justify-between py-1 border-b border-border/40">
            <span className="text-text-muted">Target Proxy</span>
            <span className="font-mono text-accent text-[11px] truncate max-w-[190px]">
              {pool.masked_hint || '[ENCRYPTED AT REST]'}
            </span>
          </div>

          <div className="flex justify-between py-1 border-b border-border/40">
            <span className="text-text-muted">Bobot Alokasi</span>
            <span className="font-mono text-white">{pool.weight}</span>
          </div>
        </div>
      </div>

      <div className="mt-5 pt-3 border-t border-border flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
        <Button
          variant="secondary"
          size="sm"
          onClick={() => onTestProbe(pool.id)}
          isLoading={isTesting}
          icon={<Play className="w-3.5 h-3.5 text-emerald-400" />}
          className="w-full sm:w-auto justify-center text-xs text-white"
        >
          Uji Ping
        </Button>

        <div className="grid grid-cols-2 sm:flex items-center gap-2 w-full sm:w-auto">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => onEdit(pool)}
            icon={<Edit2 className="w-3.5 h-3.5" />}
            className="w-full sm:w-auto justify-center text-xs"
          >
            Edit
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => onDelete(pool.id, pool.name)}
            icon={<Trash2 className="w-3.5 h-3.5" />}
            className="w-full sm:w-auto justify-center text-xs"
          >
            Hapus
          </Button>
        </div>
      </div>
    </Card>
  );
};
