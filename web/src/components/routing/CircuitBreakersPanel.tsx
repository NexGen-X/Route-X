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
  AlertTriangle,
  Radio,
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
              className={`w-10 h-10 rounded-xl flex items-center justify-center flex-shrink-0 mt-0.5 ${
                totalAbnormalBreakers > 0
                  ? 'bg-rose-500/10 text-rose-400 border border-rose-500/30 shadow-sm'
                  : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 shadow-sm'
              }`}
            >
              {totalAbnormalBreakers > 0 ? (
                <ZapOff className="w-5 h-5" aria-hidden="true" />
              ) : (
                <ShieldCheck className="w-5 h-5" aria-hidden="true" />
              )}
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-base font-bold text-white tracking-tight">
                  Pemutus Sirkuit (Circuit Breakers)
                </h3>
                {totalAbnormalBreakers > 0 ? (
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-rose-500/15 text-rose-400 border border-rose-500/30">
                    <span className="w-2 h-2 rounded-full bg-rose-400 animate-ping" aria-hidden="true" />
                    {totalAbnormalBreakers} Sirkuit Terganggu
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30">
                    <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" aria-hidden="true" />
                    Semua Beroperasi Normal
                  </span>
                )}
              </div>
              <p className="text-xs text-text-secondary mt-0.5 leading-relaxed">
                Proteksi isolasi otomatis yang mengkarantina provider saat terjadi lonjakan kegagalan dan failover darurat.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 self-end sm:self-auto flex-shrink-0">
            <Button
              variant="secondary"
              size="sm"
              onClick={loadBreakers}
              isLoading={isBreakersLoading}
              className="min-h-[38px] sm:min-h-[34px] px-3 text-xs"
              aria-label="Segarkan status circuit breakers"
            >
              <RefreshCw className={`w-3.5 h-3.5 mr-1.5 ${isBreakersLoading ? 'animate-spin' : ''}`} aria-hidden="true" />
              Segarkan
            </Button>
          </div>
        </div>

        {breakersError && <QueryError message={breakersError} onRetry={() => void loadBreakers()} />}

        {/* Body: Healthy State vs Tripped State */}
        {breakers.length === 0 || totalAbnormalBreakers === 0 ? (
          <div className="p-4 sm:p-5 rounded-xl bg-bg-surface-2/60 border border-border/40 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 flex-shrink-0 mt-0.5">
                <CheckCircle2 className="w-4 h-4" aria-hidden="true" />
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
              <div className="px-3 py-2 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Provider Dipantau</span>
                <span className="text-xs font-bold font-mono text-white">
                  {providers.length} Terhubung
                </span>
              </div>
              <div className="px-3 py-2 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Sirkuit Terputus</span>
                <span className="text-xs font-bold font-mono text-emerald-400">
                  0 (Aman)
                </span>
              </div>
              <div className="col-span-2 sm:col-span-1 px-3 py-2 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
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
                  className={`p-4 rounded-xl border flex flex-col justify-between transition-colors shadow-xs ${
                    isTrip
                      ? 'bg-rose-500/5 border-rose-500/30'
                      : isHalf
                      ? 'bg-amber-500/5 border-amber-500/30'
                      : 'bg-bg-surface-2 border-border/60'
                  }`}
                  data-testid={`breaker-${b.provider_id}-${b.model || 'all'}`}
                >
                  <div>
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <span className="text-[10px] text-text-muted font-sans uppercase tracking-wider block font-semibold">
                          Provider Upstream
                        </span>
                        <h4 className="text-xs font-bold text-white truncate">
                          {providerName}
                        </h4>
                        <span className="text-[11px] font-mono text-sky-300 block truncate font-medium">
                          {b.model || 'Semua Model Provider'}
                        </span>
                      </div>
                      <Badge variant={isTrip ? 'error' : isHalf ? 'warn' : 'success'}>
                        {isTrip ? 'Terisolasi (Open)' : isHalf ? 'Uji Coba (Half-Open)' : 'Normal'}
                      </Badge>
                    </div>

                    <div className="mt-3 p-2.5 rounded-lg bg-bg-surface-1/90 border border-border/50 text-xs space-y-1.5 font-mono">
                      <div className="flex justify-between items-center text-text-muted text-[11px]">
                        <span className="flex items-center gap-1">
                          <AlertTriangle className="w-3 h-3 text-rose-400" aria-hidden="true" />
                          Kegagalan Konsekutif:
                        </span>
                        <span className={`font-bold px-1.5 py-0.2 rounded ${isTrip ? 'bg-rose-500/20 text-rose-300' : 'text-white'}`}>
                          {b.failure_count ?? 0}x
                        </span>
                      </div>
                      {b.next_probe_at && (
                        <div className="flex justify-between items-center text-text-muted text-[11px]">
                          <span className="flex items-center gap-1">
                            <Radio className="w-3 h-3 text-amber-400" aria-hidden="true" />
                            Jadwal Uji Probe:
                          </span>
                          <span className="text-amber-300 font-semibold">
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
                    <div className="mt-4 pt-3 border-t border-border/40">
                      <Button
                        variant="secondary"
                        size="sm"
                        className="w-full text-xs min-h-[38px] border-border/70 hover:border-accent text-white font-semibold"
                        onClick={() => handleResetBreaker(b.provider_id, b.model || '')}
                        aria-label={`Pulihkan sirkuit untuk provider ${providerName}`}
                      >
                        <RotateCcw className="w-3.5 h-3.5 mr-1.5 text-accent" aria-hidden="true" />
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
