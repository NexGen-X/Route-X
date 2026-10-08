import React from 'react';
import { Server, ArrowUpRight, Cpu } from 'lucide-react';
import { Button } from '../common/Button';
import { StatusDot } from '../common/StatusDot';
import { ProviderBrandIcon } from '../providers/ProviderIcons';
import type { Provider } from '../../types';

export interface ProviderHealthMatrixProps {
  providers: Provider[];
  onNavigate: (path: string) => void;
}

const getProtocolBadge = (kind: string): string => {
  const k = kind.toLowerCase();
  switch (k) {
    case 'openai':
      return 'OpenAI REST / SSE';
    case 'anthropic':
      return 'Anthropic Messages';
    case 'gemini':
    case 'google':
      return 'Google Gemini API';
    case 'ollama':
      return 'Ollama Local RPC';
    case 'groq':
      return 'Groq LPU Engine';
    case 'deepseek':
      return 'DeepSeek V3 / R1';
    case 'azure':
      return 'Azure OpenAI SDK';
    case 'mistral':
      return 'Mistral AI API';
    case 'cohere':
      return 'Cohere Command';
    default:
      return `${k.toUpperCase()} Protocol`;
  }
};

export const ProviderHealthMatrix: React.FC<ProviderHealthMatrixProps> = ({
  providers,
  onNavigate,
}) => {
  return (
    <div className="space-y-3 sm:space-y-4">
      <div className="flex flex-row items-center justify-between gap-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="p-1.5 rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
            <Server className="w-4 h-4 flex-shrink-0" aria-hidden="true" />
          </div>
          <h3 className="text-sm font-semibold text-white tracking-tight truncate">
            Status Kesehatan Upstream Providers
          </h3>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onNavigate('/upstreams/providers')}
          aria-label="Buka halaman kelola provider"
          className="text-xs text-text-secondary hover:text-white whitespace-nowrap flex-shrink-0 p-0 h-auto"
        >
          Kelola &rarr;
        </Button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-2 gap-3 sm:gap-4">
        {providers.length === 0 ? (
          <div className="col-span-full py-10 text-center text-xs text-text-muted bg-bg-surface border border-border/80 rounded-card space-y-2">
            <p>Belum ada provider upstream yang terdaftar.</p>
            <button
              type="button"
              onClick={() => onNavigate('/upstreams/providers')}
              className="text-blue-400 hover:underline font-semibold cursor-pointer inline-flex items-center gap-1"
            >
              <span>Tambahkan provider sekarang</span>
              <ArrowUpRight className="w-3.5 h-3.5" aria-hidden="true" />
            </button>
          </div>
        ) : (
          providers.map((p) => {
            const isHealthy = p.last_health_status === 'healthy';
            const isDegraded = p.last_health_status === 'degraded';
            const isUnhealthy = p.last_health_status === 'unhealthy' || (!isHealthy && !isDegraded && p.enabled);
            const statusKey = isHealthy
              ? 'healthy'
              : isDegraded
              ? 'degraded'
              : p.enabled
              ? 'unhealthy'
              : 'disabled';

            const protocolLabel = getProtocolBadge(p.kind);

            return (
              <div
                key={p.id}
                className="bg-bg-surface border border-border/80 rounded-card p-4 hover:border-border-hover transition-all flex flex-col justify-between shadow-sm group hover:shadow-md"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="w-9 h-9 sm:w-10 sm:h-10 rounded-xl bg-bg-surface-2 border border-border/80 flex items-center justify-center text-text-primary flex-shrink-0 shadow-inner group-hover:border-blue-500/30 transition-colors">
                      <ProviderBrandIcon
                        providerIdOrKind={p.kind}
                        name={p.name}
                        className="w-5 h-5 sm:w-5.5 sm:h-5.5"
                      />
                    </div>
                    <div className="min-w-0">
                      <h4 className="text-sm font-semibold text-white group-hover:text-blue-400 transition-colors truncate">
                        {p.display_name || p.name}
                      </h4>
                      <p className="text-[11px] text-text-muted font-mono capitalize truncate mt-0.5">
                        {p.kind}
                        {p.last_latency_ms ? ` · ${p.last_latency_ms} ms` : ''}
                      </p>
                    </div>
                  </div>

                  {/* Ping dot menyala + Status dot */}
                  <div className="flex items-center gap-2 flex-shrink-0">
                    <div
                      className="relative flex items-center justify-center"
                      title={p.last_health_status || 'status'}
                    >
                      {isHealthy ? (
                        <span className="relative flex h-2.5 w-2.5">
                          <span
                            className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
                            aria-hidden="true"
                          />
                          <span
                            className="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.8)]"
                            aria-hidden="true"
                          />
                        </span>
                      ) : isDegraded ? (
                        <span className="relative flex h-2.5 w-2.5">
                          <span
                            className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"
                            aria-hidden="true"
                          />
                          <span
                            className="relative inline-flex rounded-full h-2.5 w-2.5 bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.8)]"
                            aria-hidden="true"
                          />
                        </span>
                      ) : isUnhealthy ? (
                        <span className="relative flex h-2.5 w-2.5">
                          <span
                            className="animate-ping absolute inline-flex h-full w-full rounded-full bg-rose-400 opacity-75"
                            aria-hidden="true"
                          />
                          <span
                            className="relative inline-flex rounded-full h-2.5 w-2.5 bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.8)]"
                            aria-hidden="true"
                          />
                        </span>
                      ) : (
                        <span
                          className="h-2 w-2 rounded-full bg-text-muted"
                          aria-hidden="true"
                        />
                      )}
                    </div>
                    <StatusDot
                      status={statusKey}
                      label={p.last_health_status || (p.enabled ? 'unknown' : 'disabled')}
                    />
                  </div>
                </div>

                {/* Footer kartu: Badge Protokol & Info Berat/Prioritas */}
                <div className="mt-3.5 pt-2.5 border-t border-border/50 flex items-center justify-between gap-2 text-[10px] font-mono">
                  {/* Badge protokol */}
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-bg-surface-2 text-text-secondary border border-border/60">
                    <Cpu className="w-3 h-3 text-blue-400" aria-hidden="true" />
                    <span>{protocolLabel}</span>
                  </span>

                  <span className="text-text-muted truncate">
                    P{p.priority} · W{p.weight} · {p.timeout_ms ? `${p.timeout_ms / 1000}s` : '30s'}
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
