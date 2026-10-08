import React from 'react';
import { RefreshCw, ArrowUpRight, Terminal, Shield, Zap } from 'lucide-react';
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
    <div className="relative overflow-hidden bg-bg-surface border border-border/80 rounded-card p-4 sm:p-5 shadow-sm transition-all">
      {/* Ambient glow subtle khas Hermes & DeepSeek SOTA 2026 */}
      <div
        className="pointer-events-none absolute -top-20 -left-10 w-72 h-36 bg-blue-500/10 rounded-full blur-3xl"
        aria-hidden="true"
      />
      <div
        className="pointer-events-none absolute -bottom-20 right-1/4 w-80 h-36 bg-indigo-500/10 rounded-full blur-3xl"
        aria-hidden="true"
      />

      <div className="relative flex flex-col md:flex-row md:items-center justify-between gap-4">
        {/* Kiri: Status Badge, Title & Telemetry summary strip */}
        <div className="flex flex-wrap items-center gap-x-3.5 gap-y-2 min-w-0">
          <span
            role="status"
            aria-live="polite"
            className={`inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-mono font-medium border shrink-0 transition-all ${
              loadError
                ? 'bg-rose-500/10 border-rose-500/30 text-rose-400'
                : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400'
            }`}
          >
            <span className="relative flex h-2 w-2">
              <span
                className={`animate-ping absolute inline-flex h-full w-full rounded-full opacity-75 ${
                  loadError ? 'bg-rose-400' : 'bg-emerald-400'
                }`}
                aria-hidden="true"
              />
              <span
                className={`relative inline-flex rounded-full h-2 w-2 ${
                  loadError ? 'bg-rose-500' : 'bg-emerald-500'
                }`}
              />
            </span>
            <span>{loadError ? 'Gateway Tidak Tersedia' : 'Beroperasi'}</span>
          </span>

          <div className="flex items-center gap-2 shrink-0">
            <h2 className="text-sm sm:text-base font-bold tracking-tight text-white">
              Route-X Gateway Control
            </h2>
          </div>

          <span className="hidden sm:inline text-border" aria-hidden="true">·</span>

          {/* Info strip — font mono kecil */}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-mono text-text-muted">
            <span className="inline-flex items-center gap-1">
              <Shield className="w-3 h-3 text-blue-400 inline" aria-hidden="true" />
              <span>
                <span className="text-text-primary font-medium">{healthyProvidersCount}/{totalProvidersCount}</span> upstream sehat
              </span>
            </span>

            <span className="text-border" aria-hidden="true">·</span>

            <span className="inline-flex items-center gap-1">
              <Zap className="w-3 h-3 text-amber-400 inline" aria-hidden="true" />
              <span>
                <span className="text-text-primary font-medium">{overview?.in_flight_requests ?? 0}</span> in-flight
              </span>
            </span>

            {overview?.uptime_seconds !== undefined && (
              <>
                <span className="text-border" aria-hidden="true">·</span>
                <span>
                  uptime <span className="text-text-primary font-medium">{formatUptime(overview.uptime_seconds)}</span>
                </span>
              </>
            )}
          </div>
        </div>

        {/* Kanan: Action buttons */}
        <div className="flex items-center gap-2.5 shrink-0">
          <button
            type="button"
            onClick={() => onNavigate('/cli-integrations')}
            className="hidden sm:flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-bg-surface-2/60 hover:bg-bg-surface-2 text-xs text-text-muted hover:text-blue-400 transition-colors font-mono cursor-pointer border border-border/60 hover:border-blue-500/30"
            aria-label="Buka Integrasi CLI"
          >
            <Terminal className="w-3.5 h-3.5" aria-hidden="true" />
            <span>CLI Hub →</span>
          </button>

          <Button
            variant="secondary"
            size="sm"
            onClick={onRefresh}
            isLoading={isLoading || isRefreshing}
            icon={
              <RefreshCw
                className={`w-3.5 h-3.5 ${isRefreshing ? 'animate-spin' : ''}`}
                aria-hidden="true"
              />
            }
            aria-label="Segarkan metrik dashboard"
          >
            Segarkan
          </Button>

          <Button
            variant="primary"
            size="sm"
            onClick={() => onNavigate('/requests')}
            icon={<ArrowUpRight className="w-3.5 h-3.5 text-white" aria-hidden="true" />}
            aria-label="Buka halaman Request Log"
          >
            Request Log
          </Button>
        </div>
      </div>
    </div>
  );
};
