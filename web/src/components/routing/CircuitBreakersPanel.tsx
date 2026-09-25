import React from 'react';
import type { CircuitBreakerStatus, Provider } from '../../types';
import { Button } from '../common/Button';
import { Badge } from '../common/Badge';
import { QueryError } from '../common/QueryError';
import {
  ZapOff,
  ShieldCheck,
  RefreshCw,
  CheckCircle2,
  RotateCcw,
} from 'lucide-react';

export interface CircuitBreakersPanelProps {
  breakers: CircuitBreakerStatus[];
  providers: Provider[];
  isBreakersLoading: boolean;
  breakersError: string | null;
  loadBreakers: () => Promise<void>;
  handleResetBreaker: (providerId: string, model: string) => Promise<void>;
}

export const CircuitBreakersPanel: React.FC<CircuitBreakersPanelProps> = ({
  breakers,
  providers,
  isBreakersLoading,
  breakersError,
  loadBreakers,
  handleResetBreaker,
}) => {
  const totalAbnormalBreakers = breakers.filter((b) => b.state !== 'closed').length;

  return (
    <div className="pt-6 border-t border-border/40">
      <div className="rounded-box bg-bg-surface-1 border border-border/60 p-4 sm:p-5 shadow-sm space-y-4">
        {/* Header Row: Responsive & Unbroken on Mobile */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div className="flex items-start gap-3">
            <div
              className={`w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 mt-0.5 ${
                totalAbnormalBreakers > 0
                  ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                  : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              }`}
            >
              {totalAbnormalBreakers > 0 ? (
                <ZapOff className="w-4 h-4" />
              ) : (
                <ShieldCheck className="w-4 h-4" />
              )}
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-base font-bold text-white tracking-tight">
                  Pemutus Sirkuit (Circuit Breakers)
                </h3>
                {totalAbnormalBreakers > 0 ? (
                  <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium bg-rose-500/10 text-rose-400 border border-rose-500/20">
                    <span className="w-1.5 h-1.5 rounded-full bg-rose-400 animate-ping" />
                    {totalAbnormalBreakers} Sirkuit Terganggu
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                    Semua Beroperasi Normal
                  </span>
                )}
              </div>
              <p className="text-xs text-text-secondary mt-0.5">
                Proteksi otomatis yang mengisolasi provider saat terjadi lonjakan kegagalan dan failover darurat.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 self-end sm:self-auto flex-shrink-0">
            <Button
              variant="secondary"
              size="sm"
              onClick={loadBreakers}
              isLoading={isBreakersLoading}
              className="h-8 px-3 text-xs"
            >
              <RefreshCw className={`w-3.5 h-3.5 mr-1.5 ${isBreakersLoading ? 'animate-spin' : ''}`} />
              Segarkan
            </Button>
          </div>
        </div>

        {breakersError && <QueryError message={breakersError} onRetry={() => void loadBreakers()} />}

        {/* Body: Healthy State vs Tripped State */}
        {breakers.length === 0 || totalAbnormalBreakers === 0 ? (
          <div className="p-4 sm:p-5 rounded-lg bg-bg-surface-2/60 border border-border/40 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 flex-shrink-0 mt-0.5">
                <CheckCircle2 className="w-4 h-4" />
              </div>
              <div>
                <h4 className="text-xs font-bold text-white">
                  Seluruh Sirkuit Provider Normal (Closed)
                </h4>
                <p className="text-xs text-text-secondary mt-0.5 leading-relaxed max-w-xl">
                  Proteksi isolasi otomatis aktif. Tidak ada upstream yang mengalami lonjakan kegagalan beruntun atau terisolasi dari perutean model.
                </p>
              </div>
            </div>

            {/* 3 Telemetry Chips */}
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 w-full md:w-auto flex-shrink-0 pt-2 md:pt-0 border-t md:border-t-0 border-border/40">
              <div className="px-3 py-1.5 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Provider Dipantau</span>
                <span className="text-xs font-bold font-mono text-white">
                  {providers.length} Terhubung
                </span>
              </div>
              <div className="px-3 py-1.5 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Sirkuit Terputus</span>
                <span className="text-xs font-bold font-mono text-emerald-400">
                  0 (Aman)
                </span>
              </div>
              <div className="col-span-2 sm:col-span-1 px-3 py-1.5 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Ambang Isolasi</span>
                <span className="text-xs font-bold font-mono text-sky-400">
                  5x Gagal
                </span>
              </div>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 pt-1">
            {breakers.map((b, idx) => {
              const provObj = providers.find((p) => p.id === b.provider_id);
              const providerName = provObj?.display_name || provObj?.name || b.provider_id;
              const isTrip = b.state === 'open';
              const isHalf = b.state === 'half-open';

              return (
                <div
                  key={idx}
                  className={`p-3.5 rounded-lg border flex flex-col justify-between transition-colors ${
                    isTrip
                      ? 'bg-rose-500/5 border-rose-500/30'
                      : isHalf
                      ? 'bg-amber-500/5 border-amber-500/30'
                      : 'bg-bg-surface-2 border-border/60'
                  }`}
                >
                  <div>
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <span className="text-[10px] text-text-muted font-sans uppercase tracking-wider block">
                          Provider Upstream
                        </span>
                        <h4 className="text-xs font-bold text-white truncate">
                          {providerName}
                        </h4>
                        <span className="text-[11px] font-mono text-sky-300 block truncate">
                          {b.model || 'Semua Model Provider'}
                        </span>
                      </div>
                      <Badge variant={isTrip ? 'error' : isHalf ? 'warn' : 'success'}>
                        {isTrip ? 'Terisolasi (Open)' : isHalf ? 'Uji Coba (Half-Open)' : 'Normal'}
                      </Badge>
                    </div>

                    <div className="mt-3 p-2 rounded bg-bg-surface-1/80 border border-border/40 text-xs space-y-1 font-mono">
                      <div className="flex justify-between text-text-muted text-[11px]">
                        <span>Kegagalan Konsekutif:</span>
                        <span className={`font-bold ${isTrip ? 'text-rose-400' : 'text-white'}`}>
                          {b.failure_count ?? 0}
                        </span>
                      </div>
                      {b.next_probe_at && (
                        <div className="flex justify-between text-text-muted text-[11px]">
                          <span>Jadwal Uji Probe:</span>
                          <span className="text-amber-300">
                            {new Date(b.next_probe_at).toLocaleTimeString('id-ID', {
                              hour: '2-digit',
                              minute: '2-digit',
                              second: '2-digit',
                            })}
                          </span>
                        </div>
                      )}
                    </div>
                  </div>

                  {b.state !== 'closed' && (
                    <div className="mt-3 pt-2 border-t border-border/40">
                      <Button
                        variant="secondary"
                        size="sm"
                        className="w-full text-xs h-8 border-border/60 hover:border-accent text-white"
                        onClick={() => handleResetBreaker(b.provider_id, b.model || '')}
                      >
                        <RotateCcw className="w-3.5 h-3.5 mr-1.5 text-accent" />
                        Pulihkan Sirkuit Sekarang
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};
