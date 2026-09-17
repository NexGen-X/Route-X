import React, { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { DomainConfig, BackupStatusResponse } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Select } from '../components/common/Select';
import { Badge } from '../components/common/Badge';
import { Modal } from '../components/common/Modal';
import { PageHeader } from '../components/common/PageHeader';
import {
  Sliders,
  Save,
  RefreshCw,
  Globe,
  Lock,
  ShieldCheck,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ExternalLink,
  Trash2,
  Info,
  Copy,
  Check,
  Zap,
  Plus,
  Database,
  Download,
  Upload,
  FileText,
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
  // Backup & Disaster Recovery state
  const [backupStatus, setBackupStatus] = useState<BackupStatusResponse | null>(null);
  const [isBackupLoading, setIsBackupLoading] = useState(false);
  const [exportFormat, setExportFormat] = useState<'sql.gz' | 'sql'>('sql.gz');
  const [selectedBackupFile, setSelectedBackupFile] = useState<File | null>(null);
  const [isRestoring, setIsRestoring] = useState(false);
  const [isRestoreModalOpen, setIsRestoreModalOpen] = useState(false);
  const [backupFeedback, setBackupFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

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

  const loadBackupStatus = async () => {
    setIsBackupLoading(true);
    try {
      const res = await api.system.backup.status();
      setBackupStatus(res);
    } catch (err) {
      console.error('Gagal memuat status backup:', err);
    } finally {
      setIsBackupLoading(false);
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const file = e.target.files[0];
      const name = file.name.toLowerCase();
      if (!name.endsWith('.sql') && !name.endsWith('.sql.gz')) {
        toast.error('Berkas cadangan harus berupa format .sql atau .sql.gz');
        return;
      }
      if (file.size > 50 * 1024 * 1024) {
        toast.error('Ukuran berkas melebihi batas 50 MB.');
        return;
      }
      setSelectedBackupFile(file);
      setBackupFeedback(null);
    }
  };

  const handleExecuteRestore = async () => {
    if (!selectedBackupFile) return;
    setIsRestoring(true);
    setBackupFeedback(null);
    try {
      await api.system.backup.restore(selectedBackupFile);
      toast.success('Basis data berhasil dipulihkan!');
      setBackupFeedback({
        type: 'success',
        message: 'Pemulihan berhasil dilakukan. Seluruh konfigurasi dan tabel sistem telah disinkronkan kembali.',
      });
      setIsRestoreModalOpen(false);
      setSelectedBackupFile(null);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
      void loadBackupStatus();
      void loadSettings();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal memulihkan database: ' + msg);
      setBackupFeedback({
        type: 'error',
        message: `Gagal memulihkan database: ${msg}`,
      });
    } finally {
      setIsRestoring(false);
    }
  };

  useEffect(() => {
    loadSettings();
    loadDomainConfig();
    loadBackupStatus();
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
      setDomainFeedback({ type: 'error', message: 'Nama domain tidak valid (contoh: gateway.example.com).' });
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
    <div className="space-y-6">
      <PageHeader
        title="Pengaturan Sistem"
        description="Konfigurasi domain publik HTTPS, pencadangan basis data, dan parameter operasional gateway."
      />

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
        <form noValidate onSubmit={handleSaveDomain} className="mt-6 space-y-4">
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
                  <span className="text-emerald-400 font-mono font-semibold truncate max-w-full">
                    {domainConfig.base_url}
                  </span>
                </div>
              </div>

              {/* Integrasi Xray-Core: VLESS Reality Stealth Tunnel & Local Internal Bridge */}
              {domainConfig.xray_enabled && (
                <div className="mt-4 pt-4 border-t border-accent/20 space-y-4">
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <div className="w-6 h-6 rounded-md bg-amber-500/10 text-amber-400 flex items-center justify-center">
                        <Zap className="w-3.5 h-3.5" />
                      </div>
                      <div>
                        <span className="text-xs font-bold text-white block">Xray Stealth Egress & Internal Bridge Suite</span>
                        <span className="text-[10px] text-text-muted">Port 8443 XTLS-Vision Reality Tunnel & Port 10808/10809 Local Bridges</span>
                      </div>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <span className="text-[10px] font-semibold text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20">
                        {domainConfig.xray_protocols?.length ?? 0} Protokol Aktif
                      </span>
                    </div>
                  </div>

                  {/* Daftar Kartu Protokol (VLESS-Reality & Internal Bridges) */}
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                    {(domainConfig.xray_protocols || []).map((p) => (
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
                            <span className="text-[10px] text-text-muted font-mono truncate max-w-[140px]">
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
                                <span>{p.protocol === 'socks5' || p.protocol === 'http' ? 'Salin URL Proxy' : 'Salin Tautan Klien'}</span>
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

      {/* 2. Pencadangan & Pemulihan Database (SQL Backup & Disaster Recovery) */}
      <Card className="p-6 border-accent/20 bg-bg-surface-2">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-5 border-b border-border">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-accent/15 text-accent flex items-center justify-center">
              <Database className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white flex items-center gap-2">
                Pencadangan & Pemulihan Database (SQL)
                <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-accent bg-accent/10 px-2 py-0.5 rounded-full border border-accent/20">
                  PostgreSQL Native
                </span>
              </h2>
              <p className="text-xs text-text-secondary mt-0.5">
                Ekspor dan pemulihan komprehensif seluruh skema basis data, model, provider, aturan routing, dan kunci API dalam format SQL standar (<code className="text-text-muted font-mono">.sql</code> / <code className="text-text-muted font-mono">.sql.gz</code>).
              </p>
            </div>
          </div>
          <Button variant="secondary" size="sm" onClick={loadBackupStatus} isLoading={isBackupLoading}>
            <RefreshCw className={`w-3.5 h-3.5 ${isBackupLoading ? 'animate-spin' : ''}`} />
            <span className="ml-1.5 hidden sm:inline">Segarkan Status</span>
          </Button>
        </div>

        {/* Feedback Alert */}
        {backupFeedback && (
          <div
            role={backupFeedback.type === 'error' ? 'alert' : 'status'}
            aria-live="polite"
            className={`mt-4 p-3.5 rounded-lg text-xs flex items-start gap-2.5 border ${
              backupFeedback.type === 'success'
                ? 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
                : 'bg-red-500/10 text-red-300 border-red-500/30'
            }`}
          >
            {backupFeedback.type === 'success' ? (
              <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-400 mt-0.5" />
            ) : (
              <AlertCircle className="w-4 h-4 shrink-0 text-red-400 mt-0.5" />
            )}
            <div className="flex-1">{backupFeedback.message}</div>
          </div>
        )}

        {/* Ringkasan Status Database */}
        <div className="mt-5 grid grid-cols-2 sm:grid-cols-4 gap-3">
          <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
            <span className="text-text-muted block text-[11px] font-medium">Basis Data</span>
            <span className="font-mono text-white text-sm font-semibold truncate block">
              {backupStatus?.database_name || 'Memuat...'}
            </span>
            <span className="text-[10px] text-accent font-mono mt-0.5 block">
              Ukuran: {backupStatus?.database_size || '-'}
            </span>
          </div>
          <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
            <span className="text-text-muted block text-[11px] font-medium">Total Tabel Sistem</span>
            <span className="font-mono text-white text-sm font-semibold block">
              {backupStatus ? `${backupStatus.total_tables} Tabel` : '-'}
            </span>
            <span className="text-[10px] text-text-muted font-mono mt-0.5 block">
              DDL & DML terindeks
            </span>
          </div>
          <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
            <span className="text-text-muted block text-[11px] font-medium">Entitas Terkonfigurasi</span>
            <span className="font-mono text-white text-xs font-semibold block mt-0.5">
              {backupStatus ? `${backupStatus.models_count} Model · ${backupStatus.providers_count} Provider` : '-'}
            </span>
            <span className="text-[10px] text-text-muted font-mono mt-0.5 block">
              {backupStatus ? `${backupStatus.routing_rules_count} Rules · ${backupStatus.api_keys_count} Keys` : '-'}
            </span>
          </div>
          <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
            <span className="text-text-muted block text-[11px] font-medium">Cadangan Server Terakhir</span>
            <span className="font-mono text-emerald-400 text-xs font-semibold truncate block">
              {backupStatus?.last_server_backup ? backupStatus.last_server_backup.file_size : 'Belum ada'}
            </span>
            <span className="text-[10px] text-text-muted font-mono mt-0.5 truncate block">
              {backupStatus?.last_server_backup
                ? new Date(backupStatus.last_server_backup.created_at).toLocaleString('id-ID')
                : 'Sistem siap di-backup'}
            </span>
          </div>
        </div>

        {/* Dua Kolom Aksi Ekspor vs Pemulihan */}
        <div className="mt-6 grid grid-cols-1 lg:grid-cols-2 gap-6 pt-5 border-t border-border">
          {/* Kolom Kiri: Ekspor Cadangan SQL */}
          <div className="flex flex-col justify-between p-5 rounded-xl bg-bg-surface-2 border border-border/80">
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-white font-semibold text-sm">
                <Download className="w-4 h-4 text-accent" />
                <span>Ekspor Snapshot Database (SQL)</span>
              </div>
              <p className="text-xs text-text-secondary leading-relaxed">
                Mengekstrak seluruh data Route-X menggunakan utilitas native <code className="text-text-muted font-mono">pg_dump</code> dengan opsi <code className="text-text-muted font-mono">--clean --if-exists</code>. File cadangan dapat langsung diimpor ke Route-X atau server PostgreSQL lain.
              </p>

              <div className="pt-2">
                <label className="block text-[11px] font-medium text-text-muted mb-2">Pilih Format Berkas:</label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setExportFormat('sql.gz')}
                    className={`p-2.5 rounded-lg border text-left transition-all ${
                      exportFormat === 'sql.gz'
                        ? 'border-accent bg-accent/10 text-white'
                        : 'border-border bg-bg-surface-2 text-text-muted hover:border-border/80'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-mono text-xs font-bold text-accent">.sql.gz</span>
                      <span className="text-[9px] bg-accent/20 text-accent font-semibold px-1.5 py-0.2 rounded">Rekomendasi</span>
                    </div>
                    <span className="text-[10px] text-text-secondary block mt-1">Kompresi Gzip ringkas, cepat diunduh.</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setExportFormat('sql')}
                    className={`p-2.5 rounded-lg border text-left transition-all ${
                      exportFormat === 'sql'
                        ? 'border-accent bg-accent/10 text-white'
                        : 'border-border bg-bg-surface-2 text-text-muted hover:border-border/80'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-mono text-xs font-bold text-white">.sql</span>
                      <span className="text-[9px] bg-bg-surface-2 text-text-muted font-medium px-1.5 py-0.2 rounded">Plain</span>
                    </div>
                    <span className="text-[10px] text-text-secondary block mt-1">Teks SQL mentah yang dapat dibaca manusia.</span>
                  </button>
                </div>
              </div>
            </div>

            <div className="pt-5 mt-4 border-t border-border/60">
              <a
                href={api.system.backup.exportUrl(exportFormat)}
                download
                className="inline-flex items-center justify-center gap-2 w-full px-4 py-2.5 bg-accent hover:bg-accent/90 text-bg-base font-semibold text-xs rounded-nav transition-colors shadow-sm"
              >
                <Download className="w-4 h-4" />
                <span>Unduh Cadangan SQL ({exportFormat === 'sql.gz' ? '.sql.gz' : '.sql'})</span>
              </a>
            </div>
          </div>

          {/* Kolom Kanan: Pemulihan Database (Restore) */}
          <div className="flex flex-col justify-between p-5 rounded-xl bg-bg-surface-2 border border-border/80">
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-white font-semibold text-sm">
                <Upload className="w-4 h-4 text-amber-400" />
                <span>Pemulihan Database (Restore SQL)</span>
              </div>
              <p className="text-xs text-text-secondary leading-relaxed">
                Memulihkan skema dan data Route-X dari file cadangan SQL. Mendukung file <code className="text-text-muted font-mono">.sql</code> maupun terkompresi <code className="text-text-muted font-mono">.sql.gz</code> (Maksimum 50 MB).
              </p>

              {/* Area File Picker */}
              <div className="pt-2">
                <input
                  ref={fileInputRef}
                  type="file"
                  id="restore-file-input"
                  accept=".sql,.sql.gz,application/x-gzip,application/gzip,application/sql,text/sql"
                  onChange={handleFileChange}
                  className="hidden"
                />
                <label
                  htmlFor="restore-file-input"
                  className={`flex flex-col items-center justify-center p-4 border-2 border-dashed rounded-xl cursor-pointer transition-all ${
                    selectedBackupFile
                      ? 'border-accent/60 bg-accent/5'
                      : 'border-border hover:border-border/80 bg-bg-surface-2'
                  }`}
                >
                  {selectedBackupFile ? (
                    <div className="flex items-center gap-2.5 text-xs text-white">
                      <FileText className="w-5 h-5 text-accent shrink-0" />
                      <div className="text-left overflow-hidden">
                        <span className="font-mono font-semibold block truncate max-w-[220px]">
                          {selectedBackupFile.name}
                        </span>
                        <span className="text-[10px] text-text-muted font-mono">
                          {(selectedBackupFile.size / 1024).toFixed(1)} KB (
                          {(selectedBackupFile.size / (1024 * 1024)).toFixed(2)} MB)
                        </span>
                      </div>
                    </div>
                  ) : (
                    <div className="text-center">
                      <Upload className="w-6 h-6 text-text-muted mx-auto mb-1.5" />
                      <span className="text-xs text-white font-medium block">Pilih Berkas Cadangan</span>
                      <span className="text-[10px] text-text-muted block mt-0.5">
                        Klik untuk memilih berkas .sql atau .sql.gz
                      </span>
                    </div>
                  )}
                </label>
              </div>

              {/* Peringatan Bahaya */}
              <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-300 flex items-start gap-2">
                <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
                <span>
                  <strong>Perhatian:</strong> Pemulihan data akan menggantikan data konfigurasi saat ini. Disarankan melakukan ekspor cadangan terlebih dahulu sebelum melanjutkan.
                </span>
              </div>
            </div>

            <div className="pt-5 mt-4 border-t border-border/60">
              <Button
                type="button"
                variant="danger"
                size="sm"
                disabled={!selectedBackupFile || isRestoring}
                onClick={() => setIsRestoreModalOpen(true)}
                icon={<Upload className="w-4 h-4" />}
                className="w-full justify-center"
              >
                {selectedBackupFile ? 'Mulai Pemulihan Database...' : 'Pilih Berkas Dahulu'}
              </Button>
            </div>
          </div>
        </div>
      </Card>

      {/* 3. Runtime Parameters Section */}
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
        <form id="create-setting-form" noValidate onSubmit={handleCreateSetting} className="space-y-4 text-xs">
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

      {/* Modal Konfirmasi Pemulihan Database */}
      <Modal
        isOpen={isRestoreModalOpen}
        onClose={() => {
          if (!isRestoring) setIsRestoreModalOpen(false);
        }}
        title="Konfirmasi Pemulihan Database"
        footer={
          <div className="flex items-center justify-end gap-3 w-full">
            <Button
              variant="secondary"
              size="sm"
              disabled={isRestoring}
              onClick={() => setIsRestoreModalOpen(false)}
            >
              Batal
            </Button>
            <Button
              variant="danger"
              size="sm"
              isLoading={isRestoring}
              onClick={handleExecuteRestore}
              icon={<Upload className="w-4 h-4" />}
            >
              {isRestoring ? 'Memulihkan Basis Data...' : 'Ya, Pulihkan Sekarang'}
            </Button>
          </div>
        }
      >
        <div className="space-y-4 text-xs">
          <div className="p-3.5 rounded-lg bg-red-500/10 border border-red-500/30 text-red-300 flex items-start gap-3">
            <AlertCircle className="w-5 h-5 text-red-400 shrink-0 mt-0.5" />
            <div className="space-y-1">
              <span className="font-bold block text-white text-sm">Tindakan Ini Berdampak Besar</span>
              <p className="text-xs text-red-300/90 leading-relaxed">
                Basis data Route-X akan dieksekusi ulang menggunakan skrip SQL yang diunggah. Seluruh konfigurasi, model, aturan, dan kunci API akan diselaraskan dengan isi berkas tersebut.
              </p>
            </div>
          </div>

          <div className="p-3 bg-bg-surface-2 rounded-lg border border-border space-y-1.5 font-mono">
            <div className="flex justify-between text-text-secondary">
              <span>Berkas Cadangan:</span>
              <span className="text-white font-bold truncate max-w-[200px]">
                {selectedBackupFile?.name}
              </span>
            </div>
            <div className="flex justify-between text-text-secondary">
              <span>Ukuran Berkas:</span>
              <span className="text-accent">
                {selectedBackupFile ? `${(selectedBackupFile.size / 1024).toFixed(1)} KB` : '-'}
              </span>
            </div>
            <div className="flex justify-between text-text-secondary">
              <span>Target Database:</span>
              <span className="text-white">{backupStatus?.database_name || 'routex_prod'}</span>
            </div>
          </div>

          <p className="text-text-muted text-[11px]">
            Setelah proses pemulihan selesai, sistem akan otomatis memperbarui tampilan dashboard dan parameter yang tersimpan.
          </p>
        </div>
      </Modal>
    </div>
  );
};

