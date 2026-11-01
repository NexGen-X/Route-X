import React from 'react';
import { ArrowUpRight } from 'lucide-react';
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

// Fase 4 (Calm Console): matriks kartu provider menjadi daftar baris
// polos berpemisah garis tipis — satu titik status per baris (StatusDot),
// meta protokol/prioritas sebagai teks senyap di kanan.
export const ProviderHealthMatrix: React.FC<ProviderHealthMatrixProps> = ({
  providers,
  onNavigate,
}) => {
  return (
    <div>
      <div className="flex items-center justify-between gap-3 mb-1">
        <h3 className="text-sm font-medium text-white tracking-tight truncate">
          Status Kesehatan Penyedia Upstream
        </h3>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onNavigate('/upstreams/providers')}
          aria-label="Buka halaman kelola provider"
          className="text-xs text-text-secondary hover:text-white"
        >
          Kelola &rarr;
        </Button>
      </div>

      {providers.length === 0 ? (
        <div className="py-8 text-center text-xs text-text-muted space-y-2 border-y border-border/60">
          <p>Belum ada provider upstream yang terdaftar.</p>
          <button
            type="button"
            onClick={() => onNavigate('/upstreams/providers')}
            className="text-text-secondary hover:text-white hover:underline font-medium cursor-pointer inline-flex items-center gap-1"
          >
            <span>Tambahkan provider sekarang</span>
            <ArrowUpRight className="w-3.5 h-3.5" aria-hidden="true" />
          </button>
        </div>
      ) : (
        <div className="divide-y divide-border/60 border-y border-border/60">
          {providers.map((p) => {
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
              <div key={p.id} className="py-3 flex items-center gap-3 min-w-0">
                <div className="w-8 h-8 rounded-md bg-bg-surface-2/60 border border-border/60 flex items-center justify-center text-text-primary shrink-0">
                  <ProviderBrandIcon
                    providerIdOrKind={p.kind}
                    name={p.name}
                    className="w-4 h-4"
                  />
                </div>
                <div className="min-w-0 flex-1">
                  <h4 className="text-[13px] font-medium text-white truncate">
                    {p.display_name || p.name}
                  </h4>
                  <p className="text-[11px] text-text-secondary font-mono capitalize truncate mt-0.5">
                    {p.kind}
                    {p.last_latency_ms ? ` · ${p.last_latency_ms} ms` : ''}
                  </p>
                </div>
                <span className="hidden md:inline text-[11px] font-mono text-text-muted truncate">
                  {protocolLabel} · P{p.priority} · W{p.weight} ·{' '}
                  {p.timeout_ms ? `${p.timeout_ms / 1000}s` : '30s'}
                </span>
                <StatusDot
                  status={statusKey}
                  label={p.last_health_status || (p.enabled ? 'unknown' : 'disabled')}
                />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
