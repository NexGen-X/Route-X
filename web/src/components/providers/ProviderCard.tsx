import React from 'react';
import type { Provider, EgressPool, Credential, OAuthSession } from '../../types';
import { Button } from '../common/Button';
import { StatusDot } from '../common/StatusDot';
import { Tooltip } from '../common/Tooltip';
import {
  Sliders,
  Copy,
  Users,
  FlaskConical,
  ArrowRight,
  Shield,
  Zap,
} from 'lucide-react';
import { ProviderBrandIcon } from './ProviderIcons';

export interface ProviderCardProps {
  provider: Provider;
  egressPool?: EgressPool;
  poolSummary?: {
    credentials: Credential[];
    oauthSessions: OAuthSession[];
  };
  isCustom: boolean;
  onOpenDrawer: (provider: Provider, tab?: 'models' | 'credentials' | 'settings') => void;
  onProbe: (provider: Provider) => Promise<void> | void;
  onQuickAddAccount: (provider: Provider) => void;
  onCopyBaseUrl: (url: string) => Promise<void> | void;
  isProbing?: boolean;
}

// Fase 4 (Calm Console): kartu provider menjadi baris daftar polos —
// tanpa kotak bersarang, bilah indikator, dan aksen ungu khusus custom.
// Seluruh teks, aria-label, dan aksi dipertahankan persis.
export const ProviderCard: React.FC<ProviderCardProps> = ({
  provider: p,
  egressPool,
  poolSummary,
  isCustom,
  onOpenDrawer,
  onProbe,
  onQuickAddAccount,
  onCopyBaseUrl,
  isProbing = false,
}) => {
  const pool = poolSummary || { credentials: [], oauthSessions: [] };
  const directCreds = (pool.credentials || []).filter(
    (c: Credential) =>
      !c.label.startsWith('oauth:') &&
      !(pool.oauthSessions || []).some((s: OAuthSession) => s.credential_id === c.id)
  );
  const activeOAuth = (pool.oauthSessions || []).filter((s: OAuthSession) => s.enabled).length;
  const activeDirect = directCreds.filter((c: Credential) => c.enabled).length;
  const totalAccounts = (pool.oauthSessions || []).length + directCreds.length;
  const activeAccounts = activeOAuth + activeDirect;

  const providerStatus: 'healthy' | 'degraded' | 'unhealthy' | 'disabled' | 'neutral' =
    !p.enabled
      ? 'disabled'
      : p.last_health_status === 'healthy'
      ? 'healthy'
      : p.last_health_status === 'degraded'
      ? 'degraded'
      : p.last_health_status === 'unhealthy'
      ? 'unhealthy'
      : 'neutral';

  const cleanBaseUrl = p.base_url.replace(/^https?:\/\//, '');

  return (
    <div className="py-4 flex flex-col gap-3" data-testid={`provider-card-${p.name}`}>
      {/* Baris identitas: ikon, nama, status */}
      <div className="flex items-center gap-3 min-w-0">
        <button
          type="button"
          onClick={() => onOpenDrawer(p, 'settings')}
          aria-label={`Buka pengaturan provider ${p.display_name || p.name}`}
          title={
            isCustom
              ? 'Klik icon custom provider ini untuk membuka drawer konfigurasi & kelola'
              : 'Klik icon provider untuk membuka drawer konfigurasi'
          }
          className="relative w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0 transition-colors cursor-pointer bg-bg-surface-2/70 border border-border text-text-primary hover:border-border-hover"
        >
          <ProviderBrandIcon
            providerIdOrKind={p.kind}
            name={p.name}
            className="w-5 h-5"
            isCustom={isCustom}
          />
          <span
            className="absolute -bottom-1 -right-1 w-4 h-4 rounded-full bg-bg-surface border border-border text-text-muted flex items-center justify-center"
            aria-hidden="true"
          >
            <Sliders className="w-2.5 h-2.5" aria-hidden="true" />
          </span>
        </button>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h4 className="font-semibold text-white text-sm sm:text-base truncate">
              {p.display_name || p.name}
            </h4>
            {isCustom && (
              <span className="px-1.5 py-0.5 text-[10px] font-mono rounded bg-bg-surface-2 text-text-secondary border border-border flex-shrink-0">
                Custom
              </span>
            )}
          </div>
          <div className="flex items-center gap-2 text-xs text-text-muted font-mono">
            <span className="truncate">{p.kind}</span>
            {p.last_latency_ms != null && p.last_latency_ms > 0 && (
              <>
                <span className="text-border" aria-hidden="true">•</span>
                <span className="text-text-secondary font-medium flex items-center gap-0.5">
                  <Zap className="w-3 h-3" aria-hidden="true" />
                  {p.last_latency_ms}ms
                </span>
              </>
            )}
          </div>
        </div>

        <div className="flex items-center flex-shrink-0">
          <StatusDot status={providerStatus} latencyMs={p.last_latency_ms} />
        </div>
      </div>

      {/* Baris meta & aksi: base URL, egress, ringkasan kredensial, tombol */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs">
        <span className="inline-flex items-center gap-1.5 font-mono text-text-secondary min-w-0">
          <span className="truncate" title={p.base_url}>
            {cleanBaseUrl}
          </span>
          <Tooltip content="Salin Base URL" position="top">
            <button
              type="button"
              onClick={() => void onCopyBaseUrl(p.base_url)}
              aria-label={`Salin Base URL provider ${p.display_name || p.name}`}
              className="p-1.5 rounded-nav text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors flex-shrink-0 cursor-pointer min-h-[32px] min-w-[32px] flex items-center justify-center focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
            >
              <Copy className="w-3.5 h-3.5" aria-hidden="true" />
            </button>
          </Tooltip>
        </span>

        <span className="text-text-muted font-mono">
          {egressPool ? egressPool.name : 'Direct Outbound'}
        </span>

        <span className="inline-flex items-center gap-2 text-text-secondary">
          <Users className="w-3.5 h-3.5 text-text-muted shrink-0" aria-hidden="true" />
          <span className="font-medium">
            {totalAccounts === 0
              ? 'Belum ada kredensial'
              : `${activeAccounts} Kredensial Aktif · ${
                  p.credential_strategy === 'priority'
                    ? 'Priority Failover'
                    : 'Round Robin'
                }`}
          </span>
          {totalAccounts === 0 ? (
            <button
              type="button"
              onClick={() => onQuickAddAccount(p)}
              aria-label={`Tambah API key untuk ${p.display_name || p.name}`}
              className="text-[11px] text-accent-text hover:underline shrink-0 font-medium cursor-pointer min-h-[32px] flex items-center"
            >
              + Tambah Key
            </button>
          ) : (
            <span className="text-[10px] text-text-muted font-mono flex items-center gap-1 shrink-0">
              <Shield className="w-3 h-3 text-emerald-400/80" aria-hidden="true" />
              AES-GCM
            </span>
          )}
        </span>

        <span className="flex items-center gap-2 ml-auto">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void onProbe(p)}
            isLoading={isProbing}
            icon={<FlaskConical className="w-3.5 h-3.5 text-accent-text" aria-hidden="true" />}
            className="min-h-[38px] sm:min-h-[36px] text-xs"
            aria-label={`Uji latensi koneksi ${p.display_name || p.name}`}
          >
            Uji Latensi
          </Button>

          <Button
            variant="primary"
            size="sm"
            onClick={() => onOpenDrawer(p, 'models')}
            icon={<ArrowRight className="w-3.5 h-3.5 text-black" aria-hidden="true" />}
            className="min-h-[38px] sm:min-h-[36px] text-xs font-semibold"
            aria-label={`Kelola model dan akun provider ${p.display_name || p.name}`}
          >
            Kelola &amp; Akun &rarr;
          </Button>
        </span>
      </div>
    </div>
  );
};
