import React from 'react';
import { RefreshCw, ArrowUpRight, Terminal } from 'lucide-react';
import { Button } from '../common/Button';
import { formatUptime } from './utils';
import { useSystemOverview, useProvidersList } from './hooks';

export interface DashboardHeroProps {
  isLoading: boolean;
  isRefreshing: boolean;
  onRefresh: () => void;
  onNavigate: (path: string) => void;
}

export const DashboardHero: React.FC<DashboardHeroProps> = ({
  isLoading,
  isRefreshing,
  onRefresh,
  onNavigate,
}) => {
  const { data: overview, error: overviewError } = useSystemOverview();
  const { data: providersList } = useProvidersList();

  const loadError = overviewError ? String(overviewError) : null;
  const providers = providersList?.items || [];
  const healthyProvidersCount = providers.filter((p) => p.last_health_status === 'healthy').length;
  const totalProvidersCount = providers.length;

  return (
    <div className="bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl shadow-lg px-5 py-3.5 ">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 justify-between">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 min-w-0">
          <span
            role="status"
            aria-live="polite"
            className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border shrink-0 ${
              loadError
                ? 'bg-rose-500/10 border-rose-500/20 text-rose-400'
                : 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400'
            }`}
          >
            <span
              className={`w-1.5 h-1.5 rounded-full ${loadError ? 'bg-rose-400' : 'bg-emerald-400 animate-pulse'}`}
            />
            {loadError ? 'Gateway Tidak Tersedia' : 'Beroperasi'}
          </span>

          <h2 className="text-sm font-bold tracking-tight text-white shrink-0">
            Route-X Gateway Control
          </h2>

          <span className="hidden sm:inline text-border">·</span>

          <div className="hidden sm:flex items-center gap-x-4 text-xs font-mono text-text-muted">
            <span>
              <span className="text-text-secondary font-medium">{healthyProvidersCount}/{totalProvidersCount}</span> upstream sehat
            </span>
            <span className="text-border">·</span>
            <span>
              <span className="text-text-secondary font-medium">{overview?.in_flight_requests ?? 0}</span> in-flight
            </span>
            {overview?.uptime_seconds !== undefined && (
              <>
                <span className="text-border">·</span>
                <span>uptime <span className="text-text-secondary font-medium">{formatUptime(overview.uptime_seconds)}</span></span>
              </>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          <button
            type="button"
            onClick={() => onNavigate('/cli-integrations')}
            className="hidden md:flex items-center gap-1 text-xs text-text-muted hover:text-accent transition-colors font-mono cursor-pointer"
            aria-label="Buka Integrasi CLI"
          >
            <Terminal className="w-3.5 h-3.5" /> CLI →
          </button>

          <Button
            variant="secondary"
            size="sm"
            onClick={onRefresh}
            isLoading={isLoading || isRefreshing}
            icon={<RefreshCw className={`w-3.5 h-3.5 ${isRefreshing ? 'animate-spin' : ''}`} />}
          >
            Segarkan
          </Button>

          <Button
            variant="primary"
            size="sm"
            onClick={() => onNavigate('/requests')}
            icon={<ArrowUpRight className="w-3.5 h-3.5 text-black" />}
          >
            Request Log
          </Button>
        </div>
      </div>
    </div>
  );
};
