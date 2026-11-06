import React from 'react';
import { RefreshCw, ArrowUpRight, Terminal, Shield, Zap, Sparkles } from 'lucide-react';
import { Button } from '../common/Button';
import type { SystemOverview } from '../../types';
import { formatUptime } from './utils';

export interface DashboardHeroProps {
  loadError: string | null;
  healthyProvidersCount: number;
  totalProvidersCount: number;
  // B-346: apakah daftar provider PERNAH benar-benar tiba. Tanpa
  // ini hero tidak bisa membedakan "0/0 karena belum/gagal dimuat"
  // dari "0/0 terukur" — Dashboard hanya menyetel loadError untuk
  // kegagalan ringkasan atau kegagalan total, jadi kegagalan parsial
  // sumber provider lolos sebagai hitungan sehat karangan.
  providersTerukur: boolean;
  overview: SystemOverview | null;
  isLoading: boolean;
  isRefreshing: boolean;
  onRefresh: () => void;
  onNavigate: (path: string) => void;
  // Jalan kembali membuka Panduan Cepat yang sudah ditutup (R1 Fase 2);
  // diisi Dashboard hanya selama panduan berstatus dismissed.
  onShowGuide?: () => void;
}

export const DashboardHero: React.FC<DashboardHeroProps> = ({
  loadError,
  healthyProvidersCount,
  totalProvidersCount,
  providersTerukur,
  overview,
  isLoading,
  isRefreshing,
  onRefresh,
  onNavigate,
  onShowGuide,
}) => {
  // B-346: klaim "Beroperasi" digerbangi keberadaan overview —
  // sumber status gateway itu sendiri. Muatan parsial yang
  // menggagalkan overview (tetapi bukan ringkasan) dulu tetap
  // mengklaim Beroperasi emerald karena loadError null dan
  // isLoading false, padahal status gateway tidak pernah tiba.
  const statusTidakDiketahui = !loadError && !isLoading && overview === null;
  const labelStatus = loadError
    ? 'Gateway Tidak Tersedia'
    : isLoading
      ? 'Memuat Status…'
      : statusTidakDiketahui
        ? 'Status Tidak Diketahui'
        : 'Beroperasi';
  return (
    <div className="border-b border-border/60 pb-4">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 sm:gap-4">
        {/* Kiri: Status Dot, Minimal Label & Telemetry Summary Strip */}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 min-w-0">
          {/* B-285: status menghormati keadaan muat — selama muatan
              pertama (loadError masih null, data belum tiba) hero
              TIDAK boleh mengklaim "Beroperasi" dengan titik emerald;
              klaim sehat hanya dari data yang benar-benar tiba.
              Pola yang sama dengan strip KPI B-228. */}
          <div
            role="status"
            aria-live="polite"
            className={`inline-flex items-center gap-2 text-xs font-medium shrink-0 ${
              loadError
                ? 'text-rose-400'
                : isLoading || statusTidakDiketahui
                  ? 'text-text-muted'
                  : 'text-emerald-400'
            }`}
          >
            <span
              className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                loadError
                  ? 'bg-rose-500'
                  : isLoading || statusTidakDiketahui
                    ? 'bg-text-muted'
                    : 'bg-emerald-500'
              }`}
              aria-hidden="true"
            />
            <span>{labelStatus}</span>
          </div>

          <h2 className="text-xs sm:text-sm font-medium text-text-secondary tracking-tight shrink-0">
            Route-X Gateway Control
          </h2>

          <span className="hidden sm:inline text-text-muted" aria-hidden="true">·</span>

          {/* Clean Metric Strip */}
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs font-mono text-text-secondary">
            <span className="inline-flex items-center gap-1.5">
              <Shield className="w-3.5 h-3.5 text-text-muted inline" aria-hidden="true" />
              <span>
                {/* B-285: "0/0 upstream sehat" selama muatan pertama
                    adalah angka karangan — tampilkan penanda belum
                    terukur sampai data pertama benar-benar tiba.
                    B-346: gerbangnya keberadaan data provider, bukan
                    isLoading — muatan parsial yang menggagalkan
                    sumber provider selesai TANPA isLoading dan
                    tanpa loadError, tetapi hitungannya tetap belum
                    terukur. */}
                <span className="text-text-primary font-medium">{providersTerukur ? `${healthyProvidersCount}/${totalProvidersCount}` : '—'}</span> upstream sehat
              </span>
            </span>

            <span className="text-text-muted" aria-hidden="true">·</span>

            <span className="inline-flex items-center gap-1.5">
              <Zap className="w-3.5 h-3.5 text-text-muted inline" aria-hidden="true" />
              <span>
                {/* B-346: in-flight dari overview; tanpa overview
                    angkanya belum terukur ("—"), bukan 0 karangan. */}
                <span className="text-text-primary font-medium">{overview ? overview.in_flight_requests : '—'}</span> in-flight
              </span>
            </span>

            {overview?.uptime_seconds !== undefined && (
              <>
                <span className="text-text-muted" aria-hidden="true">·</span>
                <span>
                  uptime <span className="text-text-primary font-medium">{formatUptime(overview.uptime_seconds)}</span>
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
            className="hidden sm:flex items-center gap-1.5 px-2.5 py-1.5 rounded-md bg-bg-surface-1 hover:bg-bg-surface-2 text-xs text-text-secondary hover:text-white transition-colors font-mono cursor-pointer border border-border hover:border-border-hover"
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

          {onShowGuide && (
            <button
              type="button"
              onClick={onShowGuide}
              className="flex items-center gap-1.5 px-2.5 py-1.5 min-h-[44px] sm:min-h-[32px] rounded-md bg-bg-surface-1 hover:bg-bg-surface-2 text-xs text-text-secondary hover:text-white transition-colors font-mono cursor-pointer border border-border hover:border-border-hover"
              aria-label="Buka Panduan Cepat"
            >
              <Sparkles className="w-3.5 h-3.5" aria-hidden="true" />
              <span>Panduan</span>
            </button>
          )}

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
