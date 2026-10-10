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
    <div className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-3 sm:p-4 shadow-sm transition-colors">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 sm:gap-4">
        {/* Kiri: Status Dot, Minimal Label & Telemetry Summary Strip */}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 min-w-0">
          <div
            role="status"
            aria-live="polite"
            className={`inline-flex items-center gap-2 px-2.5 py-1 rounded-full text-xs font-mono font-medium border shrink-0 transition-colors ${
              loadError
                ? 'bg-rose-950/40 border-rose-800/60 text-rose-400'
                : 'bg-zinc-900/80 border-zinc-800 text-emerald-400'
            }`}
          >
            <span
              className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                loadError ? 'bg-rose-500' : 'bg-emerald-500'
              }`}
              aria-hidden="true"
            />
            <span>{loadError ? 'Gateway Tidak Tersedia' : 'Beroperasi'}</span>
          </div>

          <h2 className="text-xs sm:text-sm font-medium text-zinc-300 tracking-tight shrink-0">
            Route-X Gateway Control
          </h2>

          <span className="hidden sm:inline text-zinc-700" aria-hidden="true">·</span>

          {/* Clean Metric Strip */}
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs font-mono text-zinc-400">
            <span className="inline-flex items-center gap-1.5">
              <Shield className="w-3.5 h-3.5 text-zinc-500 inline" aria-hidden="true" />
              <span>
                <span className="text-zinc-200 font-medium">{healthyProvidersCount}/{totalProvidersCount}</span> upstream sehat
              </span>
            </span>

            <span className="text-zinc-700" aria-hidden="true">·</span>

            <span className="inline-flex items-center gap-1.5">
              <Zap className="w-3.5 h-3.5 text-zinc-500 inline" aria-hidden="true" />
              <span>
                <span className="text-zinc-200 font-medium">{overview?.in_flight_requests ?? 0}</span> in-flight
              </span>
            </span>

            {overview?.uptime_seconds !== undefined && (
              <>
                <span className="text-zinc-700" aria-hidden="true">·</span>
                <span>
                  uptime <span className="text-zinc-200 font-medium">{formatUptime(overview.uptime_seconds)}</span>
                </span>
              </>
            )}
          </div>
        </div>

        {/* Kanan: Monochrome High-Contrast Buttons */}
        <div className="flex items-center gap-2 shrink-0">
          <button
            type="button"
            onClick={() => onNavigate('/cli-integrations')}
            className="hidden sm:flex items-center gap-1.5 px-2.5 py-1.5 rounded-md bg-zinc-900 hover:bg-zinc-800 text-xs text-zinc-400 hover:text-zinc-200 transition-colors font-mono cursor-pointer border border-zinc-800 hover:border-zinc-700"
            aria-label="Buka Integrasi CLI"
          >
            <Terminal className="w-3.5 h-3.5" aria-hidden="true" />
            <span>CLI Hub</span>
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
            icon={<ArrowUpRight className="w-3.5 h-3.5 text-zinc-950" aria-hidden="true" />}
            aria-label="Buka halaman Request Log"
          >
            Request Log
          </Button>
        </div>
      </div>
    </div>
  );
};
