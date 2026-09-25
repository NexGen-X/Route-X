import React from 'react';
import { Server } from 'lucide-react';
import { Button } from '../common/Button';
import { Badge } from '../common/Badge';
import { ProviderBrandIcon } from '../providers/ProviderIcons';
import type { Provider } from '../../types';

export interface ProviderHealthMatrixProps {
  providers: Provider[];
  onNavigate: (path: string) => void;
}

export const ProviderHealthMatrix: React.FC<ProviderHealthMatrixProps> = ({
  providers,
  onNavigate,
}) => {
  return (
    <div className="space-y-3 sm:space-y-4">
      <div className="flex flex-row items-center justify-between gap-3">
        <div className="flex items-center gap-2 min-w-0">
          <Server className="w-4 h-4 text-emerald-400 flex-shrink-0" />
          <h3 className="text-sm font-semibold text-white tracking-tight truncate">
            Status Kesehatan Upstream Providers
          </h3>
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
              type="button"
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
                      <ProviderBrandIcon
                        providerIdOrKind={p.kind}
                        name={p.name}
                        className="w-4 h-4 sm:w-5 sm:h-5"
                      />
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
  );
};
