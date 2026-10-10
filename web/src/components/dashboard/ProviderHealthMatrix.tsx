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
      return 'OpenAI REST';
    case 'anthropic':
      return 'Anthropic Messages';
    case 'gemini':
    case 'google':
      return 'Google Gemini';
    case 'ollama':
      return 'Ollama Local RPC';
    case 'groq':
      return 'Groq LPU Engine';
    case 'deepseek':
      return 'DeepSeek V3 / R1';
    case 'azure':
      return 'Azure OpenAI';
    case 'mistral':
      return 'Mistral AI';
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
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2 min-w-0">
          <Server className="w-4 h-4 text-zinc-400 shrink-0" aria-hidden="true" />
          <h3 className="text-sm font-medium text-white tracking-tight truncate">
            Status Kesehatan Upstream Providers
          </h3>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onNavigate('/upstreams/providers')}
          aria-label="Buka halaman kelola provider"
          className="text-xs text-zinc-400 hover:text-white"
        >
          Kelola &rarr;
        </Button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {providers.length === 0 ? (
          <div className="col-span-full py-8 text-center text-xs text-zinc-500 bg-zinc-900/40 border border-zinc-800/80 rounded-lg space-y-2">
            <p>Belum ada provider upstream yang terdaftar.</p>
            <button
              type="button"
              onClick={() => onNavigate('/upstreams/providers')}
              className="text-zinc-300 hover:text-white hover:underline font-medium cursor-pointer inline-flex items-center gap-1"
            >
              <span>Tambahkan provider sekarang</span>
              <ArrowUpRight className="w-3.5 h-3.5" aria-hidden="true" />
            </button>
          </div>
        ) : (
          providers.map((p) => {
            const isHealthy = p.last_health_status === 'healthy';
            const isDegraded = p.last_health_status === 'degraded';
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
                className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-3.5 hover:border-zinc-700/80 transition-colors flex flex-col justify-between shadow-sm group"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="w-8 h-8 rounded-md bg-zinc-800/80 border border-zinc-700/60 flex items-center justify-center text-zinc-200 shrink-0">
                      <ProviderBrandIcon
                        providerIdOrKind={p.kind}
                        name={p.name}
                        className="w-4 h-4"
                      />
                    </div>
                    <div className="min-w-0">
                      <h4 className="text-xs font-medium text-white group-hover:text-zinc-200 transition-colors truncate">
                        {p.display_name || p.name}
                      </h4>
                      <p className="text-[11px] text-zinc-400 font-mono capitalize truncate mt-0.5">
                        {p.kind}
                        {p.last_latency_ms ? ` · ${p.last_latency_ms} ms` : ''}
                      </p>
                    </div>
                  </div>

                  {/* Subtle Clean Status Dot */}
                  <div className="flex items-center gap-1.5 shrink-0">
                    <span
                      className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                        isHealthy
                          ? 'bg-emerald-500'
                          : isDegraded
                          ? 'bg-amber-500'
                          : 'bg-rose-500'
                      }`}
                      aria-hidden="true"
                    />
                    <StatusDot
                      status={statusKey}
                      label={p.last_health_status || (p.enabled ? 'unknown' : 'disabled')}
                    />
                  </div>
                </div>

                {/* Footer kartu: Badge Protokol & Info Berat/Prioritas */}
                <div className="mt-3 pt-2 border-t border-zinc-800/80 flex items-center justify-between gap-2 text-[10px] font-mono">
                  <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-zinc-900 text-zinc-400 border border-zinc-800">
                    <Cpu className="w-3 h-3 text-zinc-500" aria-hidden="true" />
                    <span>{protocolLabel}</span>
                  </span>

                  <span className="text-zinc-500 truncate">
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
