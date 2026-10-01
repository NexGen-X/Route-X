import React from 'react';
import { Network, Play, Edit2, Trash2 } from 'lucide-react';
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
    <div className="relative overflow-hidden p-5 flex flex-col justify-between bg-bg-surface/40 backdrop-blur-md rounded-2xl ring-1 ring-white/5 shadow-2xl transition-all hover:bg-bg-surface/60 hover:ring-white/10 group">
      <div className="absolute inset-0 bg-gradient-to-br from-accent/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-500 pointer-events-none" />
      <div className="relative z-10">
        {/* Header Kartu */}
        <div className="flex items-start justify-between mb-6">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 rounded-2xl flex items-center justify-center bg-blue-500/10 text-blue-400 ring-1 ring-blue-500/20 shadow-inner">
              <Network className="w-6 h-6" />
            </div>
            <div>
              <h4 className="text-base font-semibold text-white tracking-wide">
                {pool.name || 'tanpa nama'}
              </h4>
              <span className="text-xs text-text-muted font-mono uppercase tracking-wider">
                {(pool.kind || 'PROXY')} • {pool.region || 'global'}
              </span>
            </div>
          </div>
          <Badge variant={pool.enabled ? 'success' : 'neutral'} className="shadow-sm">
            {pool.enabled ? 'active' : 'disabled'}
          </Badge>
        </div>

        {/* Detail Info & Status Kesehatan */}
        <div className="space-y-3 p-4 bg-black/20 rounded-xl ring-1 ring-white/5 backdrop-blur-sm">
          <div className="flex justify-between items-center">
            <span className="text-xs text-text-muted">Status Koneksi</span>
            {pool.last_health_status === 'healthy' ? (
              <span className="inline-flex items-center gap-1.5 font-mono text-xs font-medium text-emerald-400 select-none">
                <span className="w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.6)] animate-[shimmer_1.5s_infinite] bg-gradient-to-r from-bg-surface-2 via-bg-surface-3 to-bg-surface-2 bg-[length:400%_100%]" />
                <span>Terhubung ({pool.last_latency_ms || 0} ms)</span>
              </span>
            ) : pool.last_health_status === 'unhealthy' ? (
              <span className="inline-flex items-center gap-1.5 font-mono text-xs font-medium text-rose-400 select-none">
                <span className="w-2 h-2 rounded-full bg-rose-400 shadow-[0_0_8px_rgba(244,63,94,0.6)]" />
                <span>Tidak Terhubung</span>
              </span>
            ) : (
              <span className="inline-flex items-center gap-1.5 font-mono text-xs text-text-muted select-none">
                <span className="w-2 h-2 rounded-full bg-zinc-500" />
                <span>Belum Diuji</span>
              </span>
            )}
          </div>

          <div className="flex justify-between items-center">
            <span className="text-xs text-text-muted">Target Proxy</span>
            <span className="font-mono text-accent text-xs truncate max-w-[180px]">
              {pool.masked_hint || '[ENCRYPTED AT REST]'}
            </span>
          </div>

          <div className="flex justify-between items-center">
            <span className="text-xs text-text-muted">Bobot Alokasi</span>
            <span className="font-mono text-white text-xs bg-white/10 px-2 py-0.5 rounded-md">{pool.weight}</span>
          </div>
        </div>
      </div>

      <div className="mt-6 pt-4 relative z-10 flex items-center justify-between gap-3">
        {/* Subtle divider */}
        <div className="absolute top-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-white/10 to-transparent" />
        
        <Button
          variant="secondary"
          size="sm"
          onClick={() => onTestProbe(pool.id)}
          isLoading={isTesting}
          icon={<Play className="w-4 h-4 text-emerald-400" />}
          className="text-xs text-white bg-white/5 hover:bg-white/10 border-transparent shadow-sm flex-1 justify-center"
        >
          Uji Ping
        </Button>

        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => onEdit(pool)}
            icon={<Edit2 className="w-4 h-4" />}
            className="text-xs bg-white/5 hover:bg-white/10 border-transparent w-10 px-0 flex justify-center"
            title="Edit"
          />
          <Button
            variant="danger"
            size="sm"
            onClick={() => onDelete(pool.id, pool.name)}
            icon={<Trash2 className="w-4 h-4" />}
            className="text-xs bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border-transparent w-10 px-0 flex justify-center"
            title="Hapus"
          />
        </div>
      </div>
    </div>
  );
};
