import React from 'react';
import { Card } from '../common/Card';
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
    <Card className="p-6 border-accent/20 bg-bg-surface-2">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-5 border-b border-border">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-accent/15 text-accent flex items-center justify-center">
            <Globe className="w-5 h-5" />
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-base sm:text-lg font-bold text-white">
                Domain Publik & Otomatis HTTPS (SSL)
              </h2>
              {domainConfig?.status === 'active' && (
                <span className="inline-flex items-center gap-1 text-[10px] sm:text-[11px] font-semibold text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20 whitespace-nowrap">
                  <ShieldCheck className="w-3 h-3" /> Aktif & Terlindungi
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
        >
          <span className="hidden sm:inline">Segarkan</span>
        </Button>
      </div>

      {/* Feedback Alert */}
      {domainFeedback && (
        <div
          role={domainFeedback.type === 'error' ? 'alert' : 'status'}
          aria-live="polite"
          className={`mt-4 p-3.5 rounded-lg text-xs flex items-start gap-2.5 border ${
            domainFeedback.type === 'success'
              ? 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
              : domainFeedback.type === 'error'
              ? 'bg-red-500/10 text-red-300 border-red-500/30'
              : 'bg-blue-500/10 text-blue-300 border-blue-500/30'
          }`}
        >
          {domainFeedback.type === 'success' ? (
            <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-400 mt-0.5" />
          ) : domainFeedback.type === 'error' ? (
            <AlertCircle className="w-4 h-4 shrink-0 text-red-400 mt-0.5" />
          ) : (
            <Info className="w-4 h-4 shrink-0 text-blue-400 mt-0.5" />
          )}
          <div className="flex-1">{domainFeedback.message}</div>
        </div>
      )}

      {/* Panduan DNS A-Record */}
      <div className="mt-5 p-4 rounded-xl bg-bg-surface-2 border border-border">
        <div className="flex items-center gap-2 text-xs font-semibold text-white mb-2">
          <Info className="w-4 h-4 text-accent" />
          Petunjuk Konfigurasi DNS di Registrar / Cloudflare:
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs">
          <div className="bg-bg-surface-2 p-2.5 rounded-lg border border-border">
            <span className="text-text-muted block text-xs font-medium">Tipe Record</span>
            <span className="font-mono text-white font-semibold">A Record</span>
          </div>
          <div className="bg-bg-surface-2 p-2.5 rounded-lg border border-border">
            <span className="text-text-muted block text-xs font-medium">Nama Host / Subdomain</span>
            <span className="font-mono text-white font-semibold">ai atau gateway (atau @)</span>
          </div>
          <div className="bg-bg-surface-2 p-2.5 rounded-lg border border-border">
            <span className="text-text-muted block text-xs font-medium">Nilai Target (IP Publik Server)</span>
            <span className="font-mono text-accent font-semibold">{serverIP || 'Belum tersedia'}</span>
          </div>
        </div>
      </div>

      {/* Form Domain */}
      <form noValidate onSubmit={onSaveDomain} className="mt-6 space-y-4">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="md:col-span-2 space-y-1.5">
            <label htmlFor="custom-domain" className="text-xs font-semibold text-white">
              Nama Domain FQDN
            </label>
            <div className="relative">
              <input
                id="custom-domain"
                name="domain"
                type="text"
                placeholder="ai.domainanda.com"
                value={inputDomain}
                onChange={(e) => setInputDomain(e.target.value)}
                className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-xs text-white font-mono placeholder:text-text-muted focus:outline-none focus:border-accent"
              />
            </div>
            <span className="text-[11px] text-text-muted block">
              Masukkan nama domain tanpa <code>http://</code> atau <code>https://</code>.
            </span>
          </div>

          <div className="space-y-1.5">
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

        {/* Active Domain Live Info */}
        {domainConfig?.domain && (
          <div className="p-4 rounded-xl bg-accent/5 border border-accent/20 space-y-2">
            <div className="text-xs font-semibold text-white flex items-center justify-between">
              <span>Alamat Aktif Terpasang:</span>
              <span className="text-[11px] font-mono text-text-muted">
                DNS Match:{' '}
                <strong className={domainConfig.dns_matched ? 'text-emerald-400' : 'text-amber-400'}>
                  {domainConfig.dns_matched ? 'Terverifikasi' : 'Belum Cocok'}
                </strong>
              </span>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-xs">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between p-2.5 rounded bg-bg-surface-2 gap-1 sm:gap-2 min-w-0">
                <span className="text-text-secondary shrink-0 text-[11px] sm:text-xs">Web Panel:</span>
                <a
                  href={domainConfig.public_url}
                  target="_blank"
                  rel="noreferrer"
                  className="text-accent hover:underline font-mono inline-flex items-center gap-1 font-semibold truncate max-w-full"
                  title={domainConfig.public_url}
                >
                  <span className="truncate">{domainConfig.public_url}</span>
                  <ExternalLink className="w-3 h-3 shrink-0" />
                </a>
              </div>
              <div className="flex flex-col sm:flex-row sm:items-center justify-between p-2.5 rounded bg-bg-surface-2 gap-1 sm:gap-2 min-w-0">
                <span className="text-text-secondary shrink-0 text-[11px] sm:text-xs">Base URL Endpoint:</span>
                <span className="text-emerald-400 font-mono font-semibold truncate max-w-full">
                  {domainConfig.base_url}
                </span>
              </div>
            </div>
          </div>
        )}

        <div className="flex flex-col-reverse sm:flex-row sm:items-center justify-between gap-3 pt-2">
          <div>
            {domainConfig?.domain && (
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={onDeleteDomain}
                isLoading={isDomainSaving}
                icon={<Trash2 className="w-3.5 h-3.5 text-red-400" />}
                className="w-full sm:w-auto text-red-400 hover:text-red-300 justify-center"
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
            icon={<Lock className="w-3.5 h-3.5" />}
            className="w-full sm:w-auto justify-center"
          >
            {domainConfig?.domain ? 'Perbarui & Verifikasi Domain' : 'Verifikasi & Aktifkan HTTPS'}
          </Button>
        </div>
      </form>
    </Card>
  );
};
