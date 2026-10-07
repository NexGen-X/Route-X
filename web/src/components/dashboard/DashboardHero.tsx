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
    <div className="bg-bg-surface border border-border rounded-card px-5 py-3.5 shadow-sm">
      {/* Baris utama — semua dalam satu flex row */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 justify-between">
        {/* Kiri: status + judul + info strip */}
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

          {/* Info strip — font mono kecil, hanya tampil di sm+ */}
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

        {/* Kanan: action buttons */}
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
            icon={<ArrowUpRight className="w-3.5 h-3.5 text-white" />}
          >
            Request Log
          </Button>
        </div>
      </div>
    </div>
  );
};

