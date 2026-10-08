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
    <div
      className="bg-bg-surface border border-border hover:border-border-hover/80 rounded-card p-5 transition-all flex flex-col justify-between shadow-sm group hover:shadow-glow-subtle relative overflow-hidden"
      data-testid={`provider-card-${p.name}`}
    >
      {/* Subtle top indicator bar */}
      <div
        className={`absolute top-0 left-0 right-0 h-[2px] transition-opacity ${
          providerStatus === 'healthy'
            ? 'bg-emerald-500/60'
            : providerStatus === 'unhealthy'
            ? 'bg-rose-500/60'
            : providerStatus === 'degraded'
            ? 'bg-amber-500/60'
            : 'bg-border'
        }`}
      />

      {/* Bagian Atas: Icon, Identitas & Status Dot */}
      <div className="space-y-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            {/* Icon Provider Interaktif: Klik untuk Buka Drawer */}
            <button
              type="button"
              onClick={() => onOpenDrawer(p, 'settings')}
              aria-label={`Buka pengaturan provider ${p.display_name || p.name}`}
              title={
                isCustom
                  ? 'Klik icon custom provider ini untuk membuka drawer konfigurasi & kelola'
                  : 'Klik icon provider untuk membuka drawer konfigurasi'
              }
              className={`relative w-11 h-11 rounded-inner flex items-center justify-center flex-shrink-0 transition-transform active:scale-95 cursor-pointer shadow-sm ${
                isCustom
                  ? 'bg-purple-950/50 border border-purple-500/40 text-purple-300 hover:scale-105 hover:border-purple-400'
                  : 'bg-bg-surface-2 border border-border text-text-primary hover:border-accent/50 hover:scale-105'
              }`}
            >
              <ProviderBrandIcon
                providerIdOrKind={p.kind}
                name={p.name}
                className="w-5 h-5"
                isCustom={isCustom}
              />
              <span
                className={`absolute -bottom-1 -right-1 w-4 h-4 rounded-full bg-bg-surface border flex items-center justify-center shadow-sm ${
                  isCustom
                    ? 'border-purple-500/60 text-purple-300'
                    : 'border-border text-text-muted'
                }`}
                aria-hidden="true"
              >
                <Sliders className="w-2.5 h-2.5" aria-hidden="true" />
              </span>
            </button>

            <div className="space-y-0.5 truncate">
              <div className="flex items-center gap-2">
                <h4 className="font-semibold text-white text-sm sm:text-base truncate">
                  {p.display_name || p.name}
                </h4>
                {isCustom && (
                  <span className="px-1.5 py-0.5 text-[10px] font-mono rounded bg-purple-500/10 text-purple-300 border border-purple-500/20 flex-shrink-0">
                    Custom
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2 text-xs text-text-muted font-mono">
                <span className="truncate">{p.kind}</span>
                {p.last_latency_ms != null && p.last_latency_ms > 0 && (
                  <>
                    <span className="text-border" aria-hidden="true">•</span>
                    <span className="text-emerald-400/90 font-medium flex items-center gap-0.5">
                      <Zap className="w-3 h-3" aria-hidden="true" />
                      {p.last_latency_ms}ms
                    </span>
                  </>
                )}
              </div>
            </div>
          </div>

          {/* Status Dot Minimalis & Interaktif */}
          <div className="flex items-center flex-shrink-0 pt-0.5">
            <StatusDot
              status={providerStatus}
              latencyMs={p.last_latency_ms}
            />
          </div>
        </div>

        {/* Base URL & Egress Proxy 1-Baris Ringkas */}
        <div className="flex items-center justify-between text-xs font-mono text-text-secondary bg-bg-surface-2/60 px-3 py-2 rounded-inner border border-border">
          <span className="truncate pr-2 font-mono" title={p.base_url}>
            {cleanBaseUrl}
            <span className="text-border mx-1.5" aria-hidden="true">·</span>
            <span className="text-text-muted">{egressPool ? egressPool.name : 'Direct Outbound'}</span>
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
        </div>

        {/* Ringkasan Pool Kredensial Multi-Account */}
        <div className="px-3 py-2.5 rounded-inner bg-bg-surface-2/40 border border-border flex items-center justify-between text-xs">
          <div className="flex items-center gap-2 truncate">
            <Users className="w-3.5 h-3.5 text-text-muted shrink-0" aria-hidden="true" />
            <span className="text-text-secondary font-medium truncate">
              {totalAccounts === 0
                ? 'Belum ada kredensial'
                : `${activeAccounts} Kredensial Aktif · ${
                    p.credential_strategy === 'priority'
                      ? 'Priority Failover'
                      : 'Round Robin'
                  }`}
            </span>
          </div>
          {totalAccounts === 0 ? (
            <button
              type="button"
              onClick={() => onQuickAddAccount(p)}
              aria-label={`Tambah API key untuk ${p.display_name || p.name}`}
              className="text-[11px] text-accent hover:underline shrink-0 font-medium cursor-pointer min-h-[32px] flex items-center"
            >
              + Tambah Key
            </button>
          ) : (
            <span className="text-[10px] text-text-muted font-mono flex items-center gap-1 shrink-0">
              <Shield className="w-3 h-3 text-emerald-400/80" aria-hidden="true" />
              AES-GCM
            </span>
          )}
        </div>
      </div>

      {/* Footer Kartu: 2 Tombol Aksi Bersih & Ergonomis (Target sentuh mobile >= 44px) */}
      <div className="mt-5 pt-3.5 border-t border-border flex items-center justify-between gap-3">
        <Button
          variant="secondary"
          size="sm"
          onClick={() => void onProbe(p)}
          isLoading={isProbing}
          icon={<FlaskConical className="w-3.5 h-3.5 text-accent" aria-hidden="true" />}
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
      </div>
    </div>
  );
};
