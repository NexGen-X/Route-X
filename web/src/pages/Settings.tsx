import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { DomainConfig } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import {
  Sliders,
  Save,
  RefreshCw,
  Globe,
  Lock,
  ShieldCheck,
  CheckCircle2,
  AlertCircle,
  ExternalLink,
  Trash2,
  Info,
} from 'lucide-react';

export const Settings: React.FC = () => {
  // Runtime Settings state
  const [settings, setSettings] = useState<{ key: string; value: string; description?: string }[]>([]);
  const [editValues, setEditValues] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [savingKey, setSavingKey] = useState<string | null>(null);

  // Domain & HTTPS state
  const [domainConfig, setDomainConfig] = useState<DomainConfig | null>(null);
  const [inputDomain, setInputDomain] = useState('');
  const [inputMode, setInputMode] = useState<'letsencrypt' | 'cloudflare'>('letsencrypt');
  const [isDomainLoading, setIsDomainLoading] = useState(false);
  const [isDomainSaving, setIsDomainSaving] = useState(false);
  const [domainFeedback, setDomainFeedback] = useState<{ type: 'success' | 'error' | 'info'; message: string } | null>(null);

  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const res = await api.system.settings();
      setSettings(res.items || []);
      const map: Record<string, string> = {};
      (res.items || []).forEach((s) => {
        map[s.key] = s.value;
      });
      setEditValues(map);
    } catch (err) {
      console.error('Gagal memuat setelan runtime:', err);
    } finally {
      setIsLoading(false);
    }
  };

  const loadDomainConfig = async () => {
    setIsDomainLoading(true);
    try {
      const res = await api.system.domain.get();
      setDomainConfig(res);
      if (res.domain) {
        setInputDomain(res.domain);
        setInputMode(res.mode || 'letsencrypt');
      }
    } catch (err) {
      console.error('Gagal memuat status domain:', err);
    } finally {
      setIsDomainLoading(false);
    }
  };

  useEffect(() => {
    loadSettings();
    loadDomainConfig();
  }, []);

  const handleSave = async (key: string) => {
    setSavingKey(key);
    try {
      await api.system.updateSetting(key, editValues[key] || '');
      alert('Pengaturan berhasil diperbarui.');
      loadSettings();
    } catch (err) {
      alert('Gagal menyimpan pengaturan: ' + err);
    } finally {
      setSavingKey(null);
    }
  };

  const handleSaveDomain = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!inputDomain.trim()) {
      setDomainFeedback({ type: 'error', message: 'Silakan masukkan nama domain yang valid.' });
      return;
    }

    setIsDomainSaving(true);
    setDomainFeedback(null);
    try {
      const res = await api.system.domain.update({
        domain: inputDomain.trim(),
        mode: inputMode,
      });
      setDomainConfig(res);
      setDomainFeedback({
        type: 'success',
        message: res.message || 'Domain berhasil diverifikasi dan HTTPS otomatis telah diaktifkan!',
      });
    } catch (err: any) {
      setDomainFeedback({
        type: 'error',
        message: err.message || 'Gagal mengonfigurasi domain. Periksa kembali DNS Record Anda.',
      });
    } finally {
      setIsDomainSaving(false);
    }
  };

  const handleDeleteDomain = async () => {
    if (!window.confirm('Apakah Anda yakin ingin melepas domain dan mengembalikan akses ke IP publik default?')) {
      return;
    }

    setIsDomainSaving(true);
    setDomainFeedback(null);
    try {
      await api.system.domain.delete();
      setInputDomain('');
      setDomainFeedback({
        type: 'info',
        message: 'Domain kustom berhasil dilepas. Sistem kembali menggunakan alamat IP server.',
      });
      loadDomainConfig();
    } catch (err: any) {
      setDomainFeedback({
        type: 'error',
        message: 'Gagal menghapus domain: ' + (err.message || err),
      });
    } finally {
      setIsDomainSaving(false);
    }
  };

  const serverIP = domainConfig?.server_ip || '54.179.116.100';

  return (
    <div className="space-y-8">
      {/* 1. Pengaturan Domain & HTTPS Otomatis */}
      <Card className="p-6 border-accent/20 bg-bg-surface-1">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-5 border-b border-border">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-accent/15 text-accent flex items-center justify-center">
              <Globe className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white flex items-center gap-2">
                Domain Publik & Otomatis HTTPS (SSL)
                {domainConfig?.status === 'active' && (
                  <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
                    <ShieldCheck className="w-3 h-3" /> Aktif & Terlindungi
                  </span>
                )}
              </h2>
              <p className="text-xs text-text-secondary mt-0.5">
                Hubungkan nama domain kustom Anda (mis. Cloudflare atau DNS Registrar). Backend akan otomatis mengonfigurasi sertifikat TLS/HTTPS untuk panel dan endpoint API.
              </p>
            </div>
          </div>
          <Button variant="secondary" size="sm" onClick={loadDomainConfig} isLoading={isDomainLoading}>
            <RefreshCw className="w-3.5 h-3.5" />
          </Button>
        </div>

        {/* Feedback Alert */}
        {domainFeedback && (
          <div
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
            <div className="bg-bg-surface-1 p-2.5 rounded-lg border border-border">
              <span className="text-text-muted block text-[10px] uppercase font-bold">Tipe Record</span>
              <span className="font-mono text-white font-semibold">A Record</span>
            </div>
            <div className="bg-bg-surface-1 p-2.5 rounded-lg border border-border">
              <span className="text-text-muted block text-[10px] uppercase font-bold">Nama Host / Subdomain</span>
              <span className="font-mono text-white font-semibold">ai atau gateway (atau @)</span>
            </div>
            <div className="bg-bg-surface-1 p-2.5 rounded-lg border border-border">
              <span className="text-text-muted block text-[10px] uppercase font-bold">Nilai Target (IP Publik Server)</span>
              <span className="font-mono text-accent font-semibold">{serverIP}</span>
            </div>
          </div>
        </div>

        {/* Form Domain */}
        <form onSubmit={handleSaveDomain} className="mt-6 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="md:col-span-2 space-y-1.5">
              <label className="text-xs font-semibold text-white">Nama Domain FQDN</label>
              <div className="relative">
                <input
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
              <label className="text-xs font-semibold text-white">Metode Penyedia SSL</label>
              <select
                value={inputMode}
                onChange={(e) => setInputMode(e.target.value as any)}
                className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-xs text-white font-sans focus:outline-none focus:border-accent"
              >
                <option value="letsencrypt">Let's Encrypt / Standar DNS</option>
                <option value="cloudflare">Cloudflare Proxy (Awan Oranye)</option>
              </select>
              <span className="text-[11px] text-text-muted block">
                {inputMode === 'cloudflare'
                  ? 'Gunakan opsi ini jika domain Anda di-proxy oleh Cloudflare CDN.'
                  : 'Sertifikat otomatis diterbitkan langsung dari Let’s Encrypt / ZeroSSL.'}
              </span>
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
                <div className="flex items-center justify-between p-2 rounded bg-bg-surface-2">
                  <span className="text-text-secondary">Web Panel:</span>
                  <a
                    href={domainConfig.public_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-accent hover:underline font-mono inline-flex items-center gap-1 font-semibold"
                  >
                    {domainConfig.public_url} <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
                <div className="flex items-center justify-between p-2 rounded bg-bg-surface-2">
                  <span className="text-text-secondary">Base URL Endpoint:</span>
                  <span className="text-emerald-400 font-mono font-semibold">{domainConfig.base_url}</span>
                </div>
              </div>
            </div>
          )}

          <div className="flex items-center justify-between pt-2">
            <div>
              {domainConfig?.domain && (
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={handleDeleteDomain}
                  isLoading={isDomainSaving}
                  icon={<Trash2 className="w-3.5 h-3.5 text-red-400" />}
                  className="text-red-400 hover:text-red-300"
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
            >
              {domainConfig?.domain ? 'Perbarui & Verifikasi Domain' : 'Verifikasi & Aktifkan HTTPS'}
            </Button>
          </div>
        </form>
      </Card>

      {/* 2. Runtime Parameters Table */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-lg font-bold tracking-tight text-white">Runtime Parameters</h3>
            <p className="text-xs text-text-secondary mt-0.5">
              Setelan parameter operasional tingkat rendah yang tersimpan di database.
            </p>
          </div>
          <Button variant="secondary" size="sm" onClick={loadSettings} isLoading={isLoading}>
            <RefreshCw className="w-3.5 h-3.5" />
          </Button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {settings.map((s) => (
            <Card key={s.key} className="p-5 flex flex-col justify-between">
              <div>
                <div className="flex items-center gap-3 mb-2">
                  <div className="w-8 h-8 rounded-full bg-accent/10 text-accent flex items-center justify-center">
                    <Sliders className="w-4 h-4" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white font-mono">{s.key}</h4>
                    <span className="text-[11px] text-text-muted">{s.description || 'Pengaturan runtime'}</span>
                  </div>
                </div>

                <div className="mt-4">
                  <input
                    type="text"
                    value={editValues[s.key] ?? s.value}
                    onChange={(e) => setEditValues({ ...editValues, [s.key]: e.target.value })}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-white font-mono"
                  />
                </div>
              </div>

              <div className="mt-4 pt-3 border-t border-border flex justify-end">
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => handleSave(s.key)}
                  isLoading={savingKey === s.key}
                  icon={<Save className="w-3.5 h-3.5" />}
                >
                  Simpan Perubahan
                </Button>
              </div>
            </Card>
          ))}
        </div>
      </div>
    </div>
  );
};

