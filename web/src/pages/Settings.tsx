import React, { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { DomainConfig } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Select } from '../components/common/Select';
import { Badge } from '../components/common/Badge';
import { Modal } from '../components/common/Modal';
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
  Copy,
  Check,
  Zap,
  Plus,
} from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { copyTextToClipboard } from '../utils/clipboard';
import { QueryError } from '../components/common/QueryError';

export const Settings: React.FC = () => {
  const { toast, confirmModal } = useToast();
  // Runtime Settings state
  const [settings, setSettings] = useState<{ key: string; value: any; description?: string }[]>([]);
  const [editValues, setEditValues] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [isCreateSettingOpen, setIsCreateSettingOpen] = useState(false);
  const [newSetting, setNewSetting] = useState({ key: '', value: '', description: '' });
  const [isCreatingSetting, setIsCreatingSetting] = useState(false);
  const [copiedLinkKey, setCopiedLinkKey] = useState<string | null>(null);
  // Timer indikator salin; dibatalkan saat unmount agar tidak ada setState basi.
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Batalkan timer salin yang tersisa saat unmount.
  useEffect(() => {
    return () => {
      if (copyTimerRef.current) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);
  // Domain & HTTPS state
  const [domainConfig, setDomainConfig] = useState<DomainConfig | null>(null);
  const [inputDomain, setInputDomain] = useState('');
  const [inputMode, setInputMode] = useState<'letsencrypt' | 'cloudflare'>('letsencrypt');
  const [isDomainLoading, setIsDomainLoading] = useState(false);
  const [isDomainSaving, setIsDomainSaving] = useState(false);
  const [domainFeedback, setDomainFeedback] = useState<{ type: 'success' | 'error' | 'info'; message: string } | null>(null);
  const [protocolFilter, setProtocolFilter] = useState<string>('all');

  const formatSettingValue = (val: any): string => {
    if (val === null || val === undefined) return '';
    if (typeof val === 'object') {
      try {
        return JSON.stringify(val, null, 2);
      } catch {
        return String(val);
      }
    }
    return String(val);
  };

  const isReservedSetting = (key: string): boolean => {
    return key.startsWith('cli:config:') || key.startsWith('system:');
  };

  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const res = await api.system.settings();
      setSettings(res.items || []);
      const map: Record<string, string> = {};
      (res.items || []).forEach((s) => {
        map[s.key] = formatSettingValue(s.value);
      });
      setEditValues(map);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
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
      toast.error('Gagal memuat status domain: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsDomainLoading(false);
    }
  };

  useEffect(() => {
    loadSettings();
    loadDomainConfig();
  }, []);

  const handleSave = async (key: string) => {
    if (isReservedSetting(key)) {
      toast.error('Parameter ini dicadangkan untuk subsistem internal dan tidak dapat diubah dari sini.');
      return;
    }
    setSavingKey(key);
    try {
      const original = settings.find((s) => s.key === key);
      await api.system.updateSetting(key, editValues[key] || '', original?.description);
      toast.success(`Parameter "${key}" berhasil diperbarui.`);
      loadSettings();
    } catch (err) {
      toast.error('Gagal menyimpan pengaturan: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setSavingKey(null);
    }
  };

  const handleDeleteSetting = async (key: string) => {
    const ok = await confirmModal({
      title: 'Hapus Parameter Runtime?',
      message: `Hapus parameter "${key}" dari basis data? Pengaturan akan kembali ke nilai bawaan sistem.`,
      confirmText: 'Ya, Hapus',
      danger: true,
    });
    if (!ok) return;

    try {
      await api.system.deleteSetting(key);
      toast.success(`Parameter "${key}" berhasil dihapus.`);
      loadSettings();
    } catch (err) {
      toast.error('Gagal menghapus parameter: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleCreateSetting = async (e: React.FormEvent) => {
    e.preventDefault();
    const key = newSetting.key.trim();
    if (!key) {
      toast.error('Kunci parameter tidak boleh kosong.');
      return;
    }
    if (isReservedSetting(key)) {
      toast.error('Kunci awalan "cli:config:" dan "system:" dicadangkan untuk modul internal.');
      return;
    }
    setIsCreatingSetting(true);
    try {
      await api.system.updateSetting(key, newSetting.value, newSetting.description.trim() || undefined);
      toast.success(`Parameter "${key}" berhasil dibuat.`);
      setIsCreateSettingOpen(false);
      setNewSetting({ key: '', value: '', description: '' });
      loadSettings();
    } catch (err) {
      toast.error('Gagal membuat parameter: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsCreatingSetting(false);
    }
  };

  const handleSaveDomain = async (e: React.FormEvent) => {
    e.preventDefault();
    const domain = inputDomain.trim().toLowerCase();
    if (!domain || !/^(?!-)(?!.*--)([a-z0-9-]{1,63}\.)+[a-z]{2,}$/.test(domain)) {
      setDomainFeedback({ type: 'error', message: 'Nama domain tidak valid (contoh: id-tech.cloud).' });
      return;
    }

    setIsDomainSaving(true);
    setDomainFeedback(null);
    try {
      const res = await api.system.domain.update({
        domain,
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
    const confirmed = await confirmModal({
      title: 'Lepas Domain Kustom',
      message: 'Apakah Anda yakin ingin melepas domain dan mengembalikan akses ke IP publik default?',
      danger: true,
      confirmText: 'Ya, Lepas Domain',
    });
    if (!confirmed) {
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

  const handleCopyLink = async (key: string, text?: string) => {
    if (!text) return;
    try {
      await copyTextToClipboard(text);
      setCopiedLinkKey(key);
      toast.success('Tautan disalin ke clipboard.');
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
      copyTimerRef.current = setTimeout(() => {
        copyTimerRef.current = null;
        setCopiedLinkKey(null);
      }, 2500);
    } catch (err) {
      toast.error('Gagal menyalin tautan: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const serverIP = domainConfig?.server_ip || null;

  return (
    <div className="space-y-8">
      {loadError && (
        <QueryError message={loadError} onRetry={() => void loadSettings()} />
      )}
      {/* 1. Pengaturan Domain & HTTPS Otomatis */}
      <Card className="p-6 border-accent/20 bg-bg-surface-2">
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
        <form onSubmit={handleSaveDomain} className="mt-6 space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="md:col-span-2 space-y-1.5">
              <label htmlFor="custom-domain" className="text-xs font-semibold text-white">Nama Domain FQDN</label>
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
                onChange={(val) => setInputMode(val as any)}
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
                  <span className="text-emerald-400 font-mono font-semibold truncate max-w-full" title={domainConfig.base_url}>
                    {domainConfig.base_url}
                  </span>
                </div>
              </div>

              {/* Integrasi Lengkap Multi-Protokol Xray-Core */}
              {domainConfig.xray_enabled && (
                <div className="mt-4 pt-4 border-t border-accent/20 space-y-4">
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <div className="w-6 h-6 rounded-md bg-amber-500/10 text-amber-400 flex items-center justify-center">
                        <Zap className="w-3.5 h-3.5" />
                      </div>
                      <div>
                        <span className="text-xs font-bold text-white block">Xray Multi-Protokol Stealth Suite</span>
                        <span className="text-[10px] text-text-muted">Port 443 Multiplexed TLS, Reality, Shadowsocks & Bridge Internal</span>
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <span className="text-[10px] font-semibold text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20">
                        {domainConfig.xray_protocols?.length ?? 0} Protokol Siap Pakai
                      </span>
                    </div>
                  </div>

                  {/* Kategori Filter Tabs */}
                  <div className="flex flex-wrap items-center gap-1 p-1 bg-bg-surface-2 rounded-lg border border-border text-[11px]">
                    {[
                      { id: 'all', label: 'Semua Protokol' },
                      { id: 'vless', label: 'VLESS (WS/gRPC/Reality/XHTTP)' },
                      { id: 'trojan', label: 'Trojan (WS/gRPC)' },
                      { id: 'vmess', label: 'VMess (WS/gRPC)' },
                      { id: 'shadowsocks', label: 'Shadowsocks' },
                      { id: 'socks5', label: 'Bridge Internal' },
                    ].map((tab) => (
                      <button
                        key={tab.id}
                        type="button"
                        onClick={() => setProtocolFilter(tab.id)}
                        className={`px-2.5 py-1 rounded-md font-medium transition-colors ${
                          protocolFilter === tab.id
                            ? 'bg-accent text-white shadow-sm'
                            : 'text-text-muted hover:text-white hover:bg-bg-surface-2'
                        }`}
                      >
                        {tab.label}
                      </button>
                    ))}
                  </div>

                  {/* Daftar Kartu Protokol */}
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                    {(domainConfig.xray_protocols || [])
                      .filter((p) => {
                        if (protocolFilter === 'all') return true;
                        if (protocolFilter === 'vless') return p.protocol === 'vless';
                        if (protocolFilter === 'trojan') return p.protocol === 'trojan';
                        if (protocolFilter === 'vmess') return p.protocol === 'vmess';
                        if (protocolFilter === 'shadowsocks') return p.protocol === 'shadowsocks';
                        if (protocolFilter === 'socks5') return p.protocol === 'socks5' || p.protocol === 'http';
                        return true;
                      })
                      .map((p) => (
                        <div
                          key={p.id}
                          className="p-3 rounded-lg bg-bg-surface-2 border border-border flex flex-col justify-between gap-2.5 hover:border-accent/40 transition-colors"
                        >
                          <div>
                            <div className="flex items-start justify-between gap-2">
                              <span className="text-[12px] font-bold text-white leading-tight">{p.name}</span>
                              <span
                                className={`text-[9px] font-mono px-1.5 py-0.5 rounded font-semibold whitespace-nowrap uppercase ${
                                  p.security === 'reality'
                                    ? 'bg-purple-500/20 text-purple-400 border border-purple-500/30'
                                    : p.security === 'tls'
                                    ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                                    : 'bg-blue-500/20 text-blue-400 border border-blue-500/30'
                                }`}
                              >
                                {p.security === 'none' ? `Port ${p.port ?? '?'}` : `${(p.security || '?').toUpperCase()} ${p.port ?? '?'}`}
                              </span>
                            </div>
                            <div className="flex items-center gap-1.5 mt-1">
                              <span className="text-[10px] text-accent font-mono bg-accent/10 px-1.5 py-0.2 rounded">
                                {(p.transport || '?').toUpperCase()}
                              </span>
                              <span className="text-[10px] text-text-muted font-mono truncate max-w-[140px]" title={p.path_or_sni || ''}>
                                {p.path_or_sni || '-'}
                              </span>
                            </div>
                            <p className="text-[11px] text-text-secondary mt-1.5 line-clamp-2">{p.description || ''}</p>
                          </div>

                          <div className="pt-2 border-t border-border/50">
                            <button
                              type="button"
                              onClick={() => handleCopyLink(p.id, p.share_link)}
                              className="w-full py-1.5 px-2 rounded bg-bg-surface-2 hover:bg-accent/20 text-[11px] text-white font-medium border border-border flex items-center justify-center gap-1.5 transition-colors"
                            >
                              {copiedLinkKey === p.id ? (
                                <>
                                  <Check className="w-3.5 h-3.5 text-emerald-400" />
                                  <span className="text-emerald-400 font-semibold">Tautan Tersalin!</span>
                                </>
                              ) : (
                                <>
                                  <Copy className="w-3.5 h-3.5 text-text-muted" />
                                  <span>Salin Tautan Klien</span>
                                </>
                              )}
                            </button>
                          </div>
                        </div>
                      ))}
                  </div>
                </div>
              )}
            </div>
          )}

          <div className="flex flex-col-reverse sm:flex-row sm:items-center justify-between gap-3 pt-2">
            <div>
              {domainConfig?.domain && (
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={handleDeleteDomain}
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

      {/* 2. Runtime Parameters Section */}
      <div className="space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <h3 className="text-lg font-bold tracking-tight text-white flex items-center gap-2">
              <Sliders className="w-5 h-5 text-accent" />
              Runtime Parameters
            </h3>
            <p className="text-xs text-text-secondary mt-0.5">
              Setelan parameter operasional dinamis yang tersimpan di basis data tanpa perlu restart biner.
            </p>
          </div>
          <div className="flex items-center gap-2 self-end sm:self-auto">
            <Button variant="secondary" size="sm" onClick={loadSettings} isLoading={isLoading}>
              <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin' : ''}`} />
              <span className="hidden sm:inline ml-1.5">Segarkan</span>
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateSettingOpen(true)}
              icon={<Plus className="w-4 h-4" />}
            >
              Tambah Parameter
            </Button>
          </div>
        </div>

        {/* Custom Operasional Parameters */}
        {settings.filter((s) => !isReservedSetting(s.key)).length === 0 ? (
          <Card className="py-8 px-6 text-center border border-border/60">
            <div className="max-w-md mx-auto space-y-3">
              <div className="w-10 h-10 rounded-xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center mx-auto">
                <Sliders className="w-5 h-5" />
              </div>
              <div>
                <h4 className="text-sm font-bold text-white">Belum Ada Parameter Runtime Kustom</h4>
                <p className="text-xs text-text-muted mt-1 leading-relaxed">
                  Gunakan parameter runtime untuk menyimpan nilai operasional dinamis (seperti flag fitur, batas jeda timeout, atau konfigurasi eksperimental).
                </p>
              </div>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setIsCreateSettingOpen(true)}
                icon={<Plus className="w-3.5 h-3.5" />}
                className="text-xs mx-auto"
              >
                Buat Parameter Baru
              </Button>
            </div>
          </Card>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {settings
              .filter((s) => !isReservedSetting(s.key))
              .map((s) => (
                <Card key={s.key} className="p-5 flex flex-col justify-between">
                  <div>
                    <div className="flex items-start justify-between gap-2 mb-2">
                      <div className="flex items-center gap-2.5 min-w-0">
                        <div className="w-7 h-7 rounded-lg bg-accent/10 text-accent flex items-center justify-center flex-shrink-0">
                          <Sliders className="w-3.5 h-3.5" />
                        </div>
                        <div className="min-w-0">
                          <h4 className="text-sm font-bold text-white font-mono truncate">{s.key}</h4>
                          <span className="text-[11px] text-text-muted block truncate">{s.description || 'Parameter runtime kustom'}</span>
                        </div>
                      </div>
                      <Badge variant="neutral">Kustom</Badge>
                    </div>

                    <div className="mt-3">
                      <textarea
                        rows={3}
                        value={editValues[s.key] ?? ''}
                        onChange={(e) => setEditValues({ ...editValues, [s.key]: e.target.value })}
                        className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-white font-mono focus:outline-none focus:border-accent"
                        placeholder="Nilai parameter (teks biasa atau JSON)..."
                      />
                    </div>
                  </div>

                  <div className="mt-4 pt-3 border-t border-border flex items-center justify-between">
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleDeleteSetting(s.key)}
                      icon={<Trash2 className="w-3.5 h-3.5 text-red-400" />}
                      className="text-red-400 hover:text-red-300"
                    >
                      Hapus
                    </Button>
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
        )}

        {/* Managed Subsystem Settings (CLI & System) */}
        {settings.filter((s) => isReservedSetting(s.key)).length > 0 && (
          <div className="pt-6 border-t border-border/60 space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-sm font-bold text-white flex items-center gap-2">
                  <Lock className="w-4 h-4 text-sky-400" />
                  Setelan Terkelola Subsistem (Read-Only)
                </h4>
                <p className="text-[11px] text-text-secondary mt-0.5">
                  Parameter ini dikelola otomatis oleh modul terkait (CLI Integrations & Domain). Perubahan dilakukan melalui menu khusus untuk menjamin integritas konfigurasi.
                </p>
              </div>
              <Badge variant="info">
                {settings.filter((s) => isReservedSetting(s.key)).length} Terkelola
              </Badge>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {settings
                .filter((s) => isReservedSetting(s.key))
                .map((s) => {
                  const isCLI = s.key.startsWith('cli:config:');
                  return (
                    <Card key={s.key} className="p-4 rounded-xl bg-bg-surface-1 border border-border/60 flex flex-col justify-between space-y-3">
                      <div>
                        <div className="flex items-start justify-between gap-2 mb-1.5">
                          <h5 className="text-xs font-bold text-white font-mono truncate">{s.key}</h5>
                          <Badge variant={isCLI ? 'neutral' : 'info'}>
                            {isCLI ? 'CLI Integrations' : 'Sistem'}
                          </Badge>
                        </div>
                        <p className="text-[11px] text-text-muted mb-2">{s.description || 'Konfigurasi subsistem internal'}</p>
                        
                        <pre className="p-2.5 rounded-lg bg-bg-base/80 border border-border/50 text-[10px] font-mono text-emerald-400/90 overflow-x-auto max-h-32 leading-relaxed">
                          {editValues[s.key] || '(kosong)'}
                        </pre>
                      </div>

                      <div className="pt-2 border-t border-border/40 flex items-center justify-end">
                        {isCLI ? (
                          <a href="#/cli">
                            <Button variant="secondary" size="sm" icon={<ExternalLink className="w-3.5 h-3.5" />}>
                              Buka CLI Integrations
                            </Button>
                          </a>
                        ) : (
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
                          >
                            Kelola di Atas
                          </Button>
                        )}
                      </div>
                    </Card>
                  );
                })}
            </div>
          </div>
        )}
      </div>

      {/* Modal Tambah Parameter Runtime */}
      <Modal
        isOpen={isCreateSettingOpen}
        onClose={() => setIsCreateSettingOpen(false)}
        title="Tambah Parameter Runtime"
        subtitle="Tambahkan variabel operasional baru ke basis data Route-X."
        footer={
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setIsCreateSettingOpen(false)}>
              Batal
            </Button>
            <Button
              variant="primary"
              size="sm"
              type="submit"
              form="create-setting-form"
              isLoading={isCreatingSetting}
              icon={<Save className="w-3.5 h-3.5" />}
            >
              Simpan Parameter
            </Button>
          </div>
        }
      >
        <form id="create-setting-form" onSubmit={handleCreateSetting} className="space-y-4 text-xs">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Kunci Parameter (Key) *
            </label>
            <input
              type="text"
              required
              placeholder="contoh: gateway:maintenance_mode atau timeout_ms"
              value={newSetting.key}
              onChange={(e) => setNewSetting({ ...newSetting, key: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
            <span className="text-[10px] text-text-muted mt-1 block">
              Gunakan huruf kecil, angka, dan titik/titik dua sebagai pemisah namespace.
            </span>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Deskripsi Singkat (Opsional)
            </label>
            <input
              type="text"
              placeholder="Keterangan fungsi atau tujuan parameter ini..."
              value={newSetting.description}
              onChange={(e) => setNewSetting({ ...newSetting, description: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Nilai Parameter (Value) *
            </label>
            <textarea
              rows={4}
              required
              placeholder="Masukkan teks biasa, angka, boolean, atau format JSON..."
              value={newSetting.value}
              onChange={(e) => setNewSetting({ ...newSetting, value: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
        </form>
      </Modal>
    </div>
  );
};

