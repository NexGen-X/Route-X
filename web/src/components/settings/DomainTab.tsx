import React from 'react';

import { Button } from '../common/Button';
import { Select } from '../common/Select';
import {
  Globe,
  Lock,
  ShieldCheck,
  CheckCircle2,
  AlertCircle,
  Info,
  ExternalLink,
  Trash2,
  RefreshCw,
} from 'lucide-react';
import type { DomainTabProps, SSLProviderMode } from './types';

export const DomainTab: React.FC<DomainTabProps> = ({
  domainConfig,
  inputDomain,
  inputMode,
  isDomainLoading,
  isDomainSaving,
  domainFeedback,
  serverIP,
  setInputDomain,
  setInputMode,
  onRefresh,
  onSaveDomain,
  onDeleteDomain,
}) => {
  const handleModeChange = (val: string) => {
    if (val === 'letsencrypt' || val === 'cloudflare') {
      setInputMode(val as SSLProviderMode);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Section */}
      <div className="p-6 rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-2xl bg-gradient-to-br from-accent/20 to-accent/5 text-accent flex items-center justify-center border border-accent/10 shadow-inner">
            <Globe className="w-6 h-6" />
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="text-base font-semibold text-white tracking-wide">
                Domain Publik & Otomatis HTTPS (SSL)
              </h2>
              {domainConfig?.status === 'active' && (
                <span className="inline-flex items-center gap-1.5 text-[11px] font-semibold text-emerald-400 bg-emerald-500/10 px-2.5 py-1 rounded-full border border-emerald-500/20 whitespace-nowrap shadow-sm shadow-emerald-500/5">
                  <ShieldCheck className="w-3.5 h-3.5" /> Aktif & Terlindungi
                </span>
              )}
            </div>
          </div>
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={onRefresh}
          isLoading={isDomainLoading}
          icon={<RefreshCw className="w-4 h-4" />}
          title="Segarkan Konfigurasi Domain"
          className="bg-white/5 hover:bg-white/10 border-white/5 text-white shadow-sm"
        >
          <span className="hidden sm:inline">Segarkan</span>
        </Button>
      </div>

      {/* Feedback Alert */}
      {domainFeedback && (
        <div
          role={domainFeedback.type === 'error' ? 'alert' : 'status'}
          aria-live="polite"
          className={`p-4 rounded-xl text-xs flex items-start gap-3 backdrop-blur-md border shadow-lg ${
            domainFeedback.type === 'success'
              ? 'bg-emerald-500/10 text-emerald-200 border-emerald-500/20 shadow-emerald-500/5'
              : domainFeedback.type === 'error'
              ? 'bg-red-500/10 text-red-200 border-red-500/20 shadow-red-500/5'
              : 'bg-blue-500/10 text-blue-200 border-blue-500/20 shadow-blue-500/5'
          }`}
        >
          {domainFeedback.type === 'success' ? (
            <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-400 mt-0.5" />
          ) : domainFeedback.type === 'error' ? (
            <AlertCircle className="w-4 h-4 shrink-0 text-red-400 mt-0.5" />
          ) : (
            <Info className="w-4 h-4 shrink-0 text-blue-400 mt-0.5" />
          )}
          <div className="flex-1 font-medium leading-relaxed">{domainFeedback.message}</div>
        </div>
      )}

      {/* Main Content Area */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 lg:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {/* Left Column: Form */}
        <div className="lg:col-span-7 space-y-6">
          <form noValidate onSubmit={onSaveDomain} className="p-6 rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10 space-y-6">
            <div className="space-y-4">
              <div className="space-y-2">
                <label htmlFor="custom-domain" className="text-xs font-semibold text-text-muted uppercase tracking-wider ml-1">
                  Nama Domain FQDN
                </label>
                <div className="relative group">
                  <div className="absolute inset-0 bg-gradient-to-r from-accent/20 to-accent/0 rounded-xl opacity-0 group-focus-within:opacity-100 transition-opacity duration-500 blur-md pointer-events-none" />
                  <input
                    id="custom-domain"
                    name="domain"
                    type="text"
                    placeholder="ai.domainanda.com"
                    value={inputDomain}
                    onChange={(e) => setInputDomain(e.target.value)}
                    className="relative w-full px-4 py-3.5 bg-bg-surface-2/50 backdrop-blur-xl border border-white/5 rounded-xl text-sm text-white font-mono placeholder:text-text-muted/50 focus:outline-none focus:border-accent/50 focus:bg-bg-surface-2/80 transition-all shadow-inner"
                  />
                </div>
                <span className="text-[11px] text-text-muted block ml-1">
                  Masukkan nama domain tanpa <code className="text-accent/80 bg-accent/10 px-1 py-0.5 rounded">http://</code> atau <code className="text-accent/80 bg-accent/10 px-1 py-0.5 rounded">https://</code>.
                </span>
              </div>

              <div className="space-y-2">
                <div className="relative z-10">
                  <Select
                    label="Metode Penyedia SSL"
                    value={inputMode}
                    onChange={handleModeChange}
                    options={[
                      {
                        value: 'letsencrypt',
                        label: "Let's Encrypt / Standar DNS",
                        description: 'Sertifikat TLS otomatis diterbitkan langsung dari Let’s Encrypt / ZeroSSL',
                      },
                      {
                        value: 'cloudflare',
                        label: 'Cloudflare Proxy (Awan Oranye)',
                        description: 'Gunakan opsi ini jika domain Anda di-proxy oleh Cloudflare CDN (SSL Mode: Flexible / Full)',
                      },
                    ]}
                  />
                </div>
              </div>
            </div>

            <div className="flex flex-col-reverse sm:flex-row sm:items-center justify-between gap-3 pt-4 border-t border-white/5">
              <div>
                {domainConfig?.domain && (
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    onClick={onDeleteDomain}
                    isLoading={isDomainSaving}
                    icon={<Trash2 className="w-4 h-4 text-red-400" />}
                    className="w-full sm:w-auto text-red-400 hover:text-red-300 hover:bg-red-500/10 border-transparent bg-transparent shadow-none"
                  >
                    Lepas Domain
                  </Button>
                )}
              </div>

              <Button
                type="submit"
                variant="primary"
                size="sm"
                isLoading={isDomainSaving}
                icon={<Lock className="w-4 h-4" />}
                className="w-full sm:w-auto justify-center bg-gradient-to-r from-accent to-accent/80 hover:from-accent hover:to-accent text-white shadow-lg shadow-accent/25 border-transparent"
              >
                {domainConfig?.domain ? 'Perbarui & Verifikasi' : 'Verifikasi & Aktifkan HTTPS'}
              </Button>
            </div>
          </form>
        </div>

        {/* Right Column: Info */}
        <div className="lg:col-span-5 space-y-6">
          {/* Active Domain Live Info */}
          {domainConfig?.domain && (
            <div className="p-5 rounded-2xl bg-accent/5 backdrop-blur-md border border-accent/20 space-y-4 shadow-lg shadow-accent/5">
              <div className="text-sm font-semibold text-white flex items-center justify-between">
                <span>Status Koneksi Aktif</span>
                <span className="text-xs font-mono px-2 py-1 rounded bg-bg-surface-2/50 border border-white/5">
                  DNS:{' '}
                  <strong className={domainConfig.dns_matched ? 'text-emerald-400' : 'text-amber-400'}>
                    {domainConfig.dns_matched ? 'Match' : 'Pending'}
                  </strong>
                </span>
              </div>
              <div className="space-y-3 text-xs">
                <div className="flex flex-col gap-1.5">
                  <span className="text-text-muted font-medium uppercase tracking-wider text-[10px] ml-1">Web Panel Akses Publik</span>
                  <div className="p-3 rounded-xl bg-bg-surface-2/50 backdrop-blur-md border border-white/5 flex items-center justify-between group">
                    <span className="text-white font-mono truncate mr-2">{domainConfig.public_url}</span>
                    <a
                      href={domainConfig.public_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-accent hover:text-accent/80 p-1.5 rounded-lg hover:bg-accent/10 transition-colors"
                      title="Buka Panel"
                    >
                      <ExternalLink className="w-4 h-4" />
                    </a>
                  </div>
                </div>
                <div className="flex flex-col gap-1.5">
                  <span className="text-text-muted font-medium uppercase tracking-wider text-[10px] ml-1">Base URL Endpoint</span>
                  <div className="p-3 rounded-xl bg-bg-surface-2/50 backdrop-blur-md border border-white/5">
                    <span className="text-emerald-400 font-mono font-semibold truncate block">
                      {domainConfig.base_url}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* Panduan DNS */}
          <div className="p-5 rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10">
            <div className="flex items-center gap-2.5 text-sm font-semibold text-white mb-4">
              <div className="w-8 h-8 rounded-lg bg-blue-500/10 text-blue-400 flex items-center justify-center">
                <Info className="w-4 h-4" />
              </div>
              Panduan DNS A-Record
            </div>
            <div className="space-y-3 text-xs">
              <div className="bg-bg-surface-2/50 backdrop-blur-md p-3 rounded-xl border border-white/5 flex justify-between items-center">
                <span className="text-text-muted font-medium">Tipe</span>
                <span className="font-mono text-white font-semibold bg-white/5 px-2 py-0.5 rounded">A Record</span>
              </div>
              <div className="bg-bg-surface-2/50 backdrop-blur-md p-3 rounded-xl border border-white/5 flex flex-col gap-1">
                <span className="text-text-muted font-medium">Nama Host / Subdomain</span>
                <span className="font-mono text-white font-semibold bg-white/5 px-2 py-1 rounded w-fit">ai atau gateway (atau @)</span>
              </div>
              <div className="bg-bg-surface-2/50 backdrop-blur-md p-3 rounded-xl border border-white/5 flex flex-col gap-1">
                <span className="text-text-muted font-medium">Target IP Address</span>
                <div className="flex items-center gap-2">
                  <span className="font-mono text-accent font-semibold bg-accent/10 px-2 py-1 rounded w-fit">{serverIP || 'Belum tersedia'}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>


  );
};
