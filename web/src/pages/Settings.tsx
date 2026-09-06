import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { DomainConfig } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Select } from '../components/common/Select';
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
} from 'lucide-react';
import { useToast } from '../context/ToastContext';

export const Settings: React.FC = () => {
  const { toast, confirmModal } = useToast();
  // Runtime Settings state
  const [settings, setSettings] = useState<{ key: string; value: string; description?: string }[]>([]);
  const [editValues, setEditValues] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [copiedLinkKey, setCopiedLinkKey] = useState<string | null>(null);
  // Domain & HTTPS state
  const [domainConfig, setDomainConfig] = useState<DomainConfig | null>(null);
  const [inputDomain, setInputDomain] = useState('');
  const [inputMode, setInputMode] = useState<'letsencrypt' | 'cloudflare'>('letsencrypt');
  const [isDomainLoading, setIsDomainLoading] = useState(false);
  const [isDomainSaving, setIsDomainSaving] = useState(false);
  const [domainFeedback, setDomainFeedback] = useState<{ type: 'success' | 'error' | 'info'; message: string } | null>(null);
  const [protocolFilter, setProtocolFilter] = useState<string>('all');

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
      toast.success('Pengaturan berhasil diperbarui');
      loadSettings();
    } catch (err) {
      toast.error('Gagal menyimpan pengaturan: ' + (err instanceof Error ? err.message : String(err)));
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

  const handleCopyLink = (key: string, text?: string) => {
    if (!text) return;
    navigator.clipboard.writeText(text);
    setCopiedLinkKey(key);
    setTimeout(() => setCopiedLinkKey(null), 2500);
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
                        {domainConfig.xray_protocols?.length || 12} Protokol Siap Pakai
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
                            : 'text-text-muted hover:text-white hover:bg-bg-surface-1'
                        }`}
                      >
                        {tab.label}
                      </button>
                    ))}
                  </div>

                  {/* Daftar Kartu Protokol */}
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                    {(domainConfig.xray_protocols || [
                      {
                        id: 'vless-grpc',
                        name: 'VLESS over gRPC',
                        protocol: 'vless',
                        transport: 'grpc',
                        security: 'tls',
                        port: 443,
                        path_or_sni: 'routex-grpc',
                        share_link: domainConfig.xray_vless_grpc || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Latensi ultra-rendah dengan multiplexing HTTP/2 gRPC resmi.',
                      },
                      {
                        id: 'vless-ws',
                        name: 'VLESS over WebSocket',
                        protocol: 'vless',
                        transport: 'ws',
                        security: 'tls',
                        port: 443,
                        path_or_sni: '/routex-xray-ws',
                        share_link: domainConfig.xray_vless_ws || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Kompatibilitas universal untuk melewati firewall dan CDN reverse proxy.',
                      },
                      {
                        id: 'vless-xhttp',
                        name: 'VLESS over SplitHTTP (XHTTP)',
                        protocol: 'vless',
                        transport: 'splithttp',
                        security: 'tls',
                        port: 443,
                        path_or_sni: '/routex-xhttp',
                        share_link: domainConfig.xray_vless_xhttp || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Transport tercanggih anti pemutusan sambungan CDN buffer.',
                      },
                      {
                        id: 'vless-reality',
                        name: 'VLESS Reality (XTLS-Vision)',
                        protocol: 'vless',
                        transport: 'tcp',
                        security: 'reality',
                        port: 8443,
                        path_or_sni: 'www.apple.com',
                        share_link: domainConfig.xray_vless_reality || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Teknologi kamuflase TLS tanpa domain dengan meminjam sertifikat Apple.',
                      },
                      {
                        id: 'trojan-grpc',
                        name: 'Trojan over gRPC',
                        protocol: 'trojan',
                        transport: 'grpc',
                        security: 'tls',
                        port: 443,
                        path_or_sni: 'routex-trojan-grpc',
                        share_link: domainConfig.xray_trojan_grpc || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Autentikasi sandi murni dengan transport multiplexing gRPC.',
                      },
                      {
                        id: 'trojan-ws',
                        name: 'Trojan over WebSocket',
                        protocol: 'trojan',
                        transport: 'ws',
                        security: 'tls',
                        port: 443,
                        path_or_sni: '/routex-trojan-ws',
                        share_link: domainConfig.xray_trojan_ws || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Penyamaran HTTPS WebSocket di balik terminasi TLS Caddy.',
                      },
                      {
                        id: 'vmess-grpc',
                        name: 'VMess over gRPC',
                        protocol: 'vmess',
                        transport: 'grpc',
                        security: 'tls',
                        port: 443,
                        path_or_sni: 'routex-vmess-grpc',
                        share_link: domainConfig.xray_vmess_grpc || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Format VMess Base64 terenkripsi dengan transport gRPC.',
                      },
                      {
                        id: 'vmess-ws',
                        name: 'VMess over WebSocket',
                        protocol: 'vmess',
                        transport: 'ws',
                        security: 'tls',
                        port: 443,
                        path_or_sni: '/routex-vmess-ws',
                        share_link: domainConfig.xray_vmess_ws || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Format tautan VMess Base64 terstandarisasi untuk semua v2ray client.',
                      },
                      {
                        id: 'ss-ws',
                        name: 'Shadowsocks WebSocket',
                        protocol: 'shadowsocks',
                        transport: 'ws',
                        security: 'tls',
                        port: 443,
                        path_or_sni: '/routex-ss-ws',
                        share_link: domainConfig.xray_shadowsocks_ws || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Shadowsocks AES-128-GCM dimultipleks dalam WebSocket Port 443.',
                      },
                      {
                        id: 'ss-standalone',
                        name: 'Shadowsocks 2022 Standalone',
                        protocol: 'shadowsocks',
                        transport: 'tcp',
                        security: 'none',
                        port: 8388,
                        path_or_sni: '-',
                        share_link: domainConfig.xray_shadowsocks || '',
                        egress_url: 'socks5://xray:10808',
                        description: 'Port langsung 8388 berkecepatan tinggi untuk router OpenWrt / IoT.',
                      },
                      {
                        id: 'socks-internal',
                        name: '⚡ Xray SOCKS5 Bridge',
                        protocol: 'socks5',
                        transport: 'tcp',
                        security: 'none',
                        port: 10808,
                        path_or_sni: 'xray:10808',
                        share_link: 'socks5://xray:10808',
                        egress_url: 'socks5://xray:10808',
                        description: 'Bridge internal Docker untuk routing proxy upstream AI di Egress Pool.',
                      },
                      {
                        id: 'http-internal',
                        name: '⚡ Xray HTTP Bridge',
                        protocol: 'socks5',
                        transport: 'tcp',
                        security: 'none',
                        port: 10809,
                        path_or_sni: 'xray:10809',
                        share_link: 'http://xray:10809',
                        egress_url: 'http://xray:10809',
                        description: 'Bridge HTTP CONNECT internal untuk aplikasi klien standar.',
                      },
                    ])
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
                                {p.security === 'none' ? `Port ${p.port}` : `${p.security.toUpperCase()} ${p.port}`}
                              </span>
                            </div>
                            <div className="flex items-center gap-1.5 mt-1">
                              <span className="text-[10px] text-accent font-mono bg-accent/10 px-1.5 py-0.2 rounded">
                                {p.transport.toUpperCase()}
                              </span>
                              <span className="text-[10px] text-text-muted font-mono truncate max-w-[140px]">
                                {p.path_or_sni}
                              </span>
                            </div>
                            <p className="text-[11px] text-text-secondary mt-1.5 line-clamp-2">{p.description}</p>
                          </div>

                          <div className="pt-2 border-t border-border/50">
                            <button
                              type="button"
                              onClick={() => handleCopyLink(p.id, p.share_link)}
                              className="w-full py-1.5 px-2 rounded bg-bg-surface-1 hover:bg-accent/20 text-[11px] text-white font-medium border border-border flex items-center justify-center gap-1.5 transition-colors"
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

