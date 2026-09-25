import React from 'react';
import { RefreshCw, ArrowUpRight, Terminal } from 'lucide-react';
import { Button } from '../common/Button';
import type { SystemOverview } from '../../types';
import { formatUptime } from './utils';

export interface DashboardHeroProps {
  loadError: string | null;
  healthyProvidersCount: number;
  totalProvidersCount: number;
  overview: SystemOverview | null;
  isLoading: boolean;
  isRefreshing: boolean;
  onRefresh: () => void;
  onNavigate: (path: string) => void;
}

export const DashboardHero: React.FC<DashboardHeroProps> = ({
  loadError,
  healthyProvidersCount,
  totalProvidersCount,
  overview,
  isLoading,
  isRefreshing,
  onRefresh,
  onNavigate,
}) => {
  return (
    <div className="bg-bg-surface border border-border rounded-card p-6 shadow-sm">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-6">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-3">
            <span
              role="status"
              aria-live="polite"
              className={`inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-semibold border ${
                loadError
                  ? 'bg-rose-500/10 border-rose-500/30 text-rose-400'
                  : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400'
              }`}
            >
              <span
                className={`w-2 h-2 rounded-full ${loadError ? 'bg-rose-400' : 'bg-emerald-400'}`}
              />
              {loadError ? 'Gateway Tidak Tersedia' : 'Gateway Siap & Beroperasi'}
            </span>
            <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-mono bg-bg-surface-2 border border-border text-text-secondary">
              {healthyProvidersCount}/{totalProvidersCount} Upstream Sehat
            </span>
          </div>

          <h2 className="text-2xl font-bold tracking-tight text-white">Route-X Gateway Control</h2>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2.5 px-3.5 py-2 rounded-inner bg-bg-surface-2 border border-border text-xs font-mono text-text-primary">
            <span className="w-2 h-2 rounded-full bg-emerald-400" />
            <span className="font-semibold text-white">{overview?.in_flight_requests ?? 0}</span>
            <span className="text-text-muted">in-flight</span>
          </div>

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

      {/* Quick Cluster Info Pill Strip */}
      <div className="mt-6 pt-4 border-t border-border/70 flex flex-wrap items-center justify-between gap-4 text-xs font-mono text-text-secondary">
        <div className="flex flex-wrap items-center gap-4 sm:gap-6">
          <div className="flex items-center gap-1.5">
            <span className="text-text-muted">Runtime:</span>
            <span className="text-text-primary font-medium">{overview?.go_version || '-'}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="text-text-muted">OS/Arch:</span>
            <span className="text-text-primary font-medium">{overview?.os_arch || '-'}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="text-text-muted">Goroutines:</span>
            <span className="text-emerald-400 font-medium">{overview?.num_goroutine ?? 0}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="text-text-muted">Uptime:</span>
            <span className="text-text-primary font-medium">
              {overview ? formatUptime(overview.uptime_seconds) : '-'}
            </span>
          </div>
        </div>

        <button
          type="button"
          onClick={() => onNavigate('/cli-integrations')}
          className="text-xs text-accent hover:underline flex items-center gap-1 transition-colors"
        >
          <Terminal className="w-3.5 h-3.5" /> Integrasi CLI &amp; SDK &rarr;
        </button>
      </div>
    </div>
  );
};
