import React from 'react';
import { Network, Play, Edit2, Trash2 } from 'lucide-react';
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
    <Card className="p-5 flex flex-col justify-between border-border hover:border-border/80 transition-all">
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
          <div className="flex justify-between py-1 border-b border-border/40 items-center">
            <span className="text-text-muted">Status Koneksi</span>
            {pool.last_health_status === 'healthy' ? (
              <span className="inline-flex items-center gap-1.5 font-mono text-[11px] font-medium text-emerald-400 select-none">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 shrink-0" />
                <span>Terhubung ({pool.last_latency_ms || 0} ms)</span>
              </span>
            ) : pool.last_health_status === 'unhealthy' ? (
              <span className="inline-flex items-center gap-1.5 font-mono text-[11px] font-medium text-rose-400 select-none">
                <span className="w-1.5 h-1.5 rounded-full bg-rose-400 shrink-0" />
                <span>Tidak Terhubung</span>
              </span>
            ) : (
              <span className="inline-flex items-center gap-1.5 font-mono text-[11px] text-text-muted select-none">
                <span className="w-1.5 h-1.5 rounded-full bg-zinc-500 shrink-0" />
                <span>Belum Diuji</span>
              </span>
            )}
          </div>

          <div className="flex justify-between py-1 border-b border-border/40 items-center">
            <span className="text-text-muted">Target Proxy</span>
            <span className="font-mono text-accent text-[11px] truncate max-w-[200px]">
              {pool.masked_hint || '[ENCRYPTED AT REST]'}
            </span>
          </div>

          <div className="flex justify-between py-1 border-b border-border/40 items-center">
            <span className="text-text-muted">Bobot Alokasi</span>
            <span className="font-mono text-white">{pool.weight}</span>
          </div>
        </div>
      </div>

      <div className="mt-5 pt-3 border-t border-border flex items-center justify-between gap-2">
        <Button
          variant="secondary"
          size="sm"
          onClick={() => onTestProbe(pool.id)}
          isLoading={isTesting}
          icon={<Play className="w-3.5 h-3.5 text-emerald-400" />}
          className="text-xs text-white"
        >
          Uji Ping
        </Button>

        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => onEdit(pool)}
            icon={<Edit2 className="w-3.5 h-3.5" />}
            className="text-xs"
          >
            Edit
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => onDelete(pool.id, pool.name)}
            icon={<Trash2 className="w-3.5 h-3.5" />}
            className="text-xs"
          >
            Hapus
          </Button>
        </div>
      </div>
    </Card>
  );
};
