import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Provider, Credential, EgressPool } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import {
  Server,
  Plus,
  Key,
  Activity,
  Trash2,
  CheckCircle2,
  XCircle,
  DownloadCloud,
  Sparkles,
  Eye,
  EyeOff,
  ChevronDown,
  ChevronUp,
  RefreshCw,
  ExternalLink,
  Globe,
  Lock,
  ArrowUpRight,
  ShieldCheck,
  Check,
} from 'lucide-react';
import {
  KNOWN_PROVIDERS,
  ProviderBrandIcon,
  type KnownProviderPreset,
} from '../components/providers/ProviderIcons';

export const Providers: React.FC = () => {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [egressPools, setEgressPools] = useState<EgressPool[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [isCredModalOpen, setIsCredModalOpen] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [probingId, setProbingId] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [probeResult, setProbeResult] = useState<any | null>(null);
  const [syncFeedback, setSyncFeedback] = useState<Record<string, string>>({});

  // Preset & Dual-Auth Modal states
  const [selectedPreset, setSelectedPreset] = useState<KnownProviderPreset | null>(null);
  const [authTab, setAuthTab] = useState<'apikey' | 'authlogin'>('apikey');
  const [quickApiKey, setQuickApiKey] = useState('');
  const [authFallbackInput, setAuthFallbackInput] = useState('');
  const [authExtractedToken, setAuthExtractedToken] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [selectedEgressPoolId, setSelectedEgressPoolId] = useState<string>('');
  const [syncAfterSave, setSyncAfterSave] = useState(true);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [savingStep, setSavingStep] = useState('');
  const [socketPingStatus, setSocketPingStatus] = useState<{ testing: boolean; latency?: number; ok?: boolean } | null>(null);

  const [newProv, setNewProv] = useState({
    name: '',
    display_name: '',
    kind: 'openai',
    base_url: 'https://api.openai.com/v1',
    priority: 100,
    weight: 100,
    timeout_ms: 30000,
    egress_pool_id: undefined as string | undefined,
  });

  const [newCred, setNewCred] = useState({
    label: '',
    api_key: '',
  });

  const loadData = async () => {
    try {
      const [provRes, egressRes] = await Promise.all([
        api.providers.list(),
        api.egress.list().catch(() => ({ items: [] })),
      ]);
      setProviders(provRes.items || []);
      setEgressPools(egressRes.items || []);
    } catch (err) {
      console.error('Gagal memuat data provider dan egress pools:', err);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  // Logika ekstraksi token dari URL callback/fallback secara otomatis
  const handleFallbackInputChange = (val: string) => {
    setAuthFallbackInput(val);
    const trimmed = val.trim();
    if (!trimmed) {
      setAuthExtractedToken('');
      return;
    }

    // Periksa apakah input berupa URL (http/https)
    if (trimmed.startsWith('http://') || trimmed.startsWith('https://')) {
      try {
        const parsed = new URL(trimmed);
        const codeParam =
          parsed.searchParams.get('code') ||
          parsed.searchParams.get('token') ||
          parsed.searchParams.get('access_token') ||
          parsed.searchParams.get('api_key') ||
          parsed.searchParams.get('key');

        if (codeParam) {
          setAuthExtractedToken(codeParam);
          return;
        }

        // Periksa hash (#access_token=... atau #token=...)
        if (parsed.hash) {
          const hashParams = new URLSearchParams(parsed.hash.replace(/^#/, ''));
          const hashToken =
            hashParams.get('access_token') ||
            hashParams.get('token') ||
            hashParams.get('code');
          if (hashToken) {
            setAuthExtractedToken(hashToken);
            return;
          }
        }
      } catch {
        // Abaikan parse error, gunakan raw
      }
    }

    // Jika bukan URL lengkap tetapi langsung token
    setAuthExtractedToken(trimmed);
  };

  const handleSelectPreset = (preset: KnownProviderPreset) => {
    setSelectedPreset(preset);
    const existingCount = providers.filter((p) => p.kind === preset.kind || p.name.includes(preset.id)).length;
    const uniqueName = existingCount > 0 ? `${preset.name}-${existingCount + 1}` : preset.name;

    setNewProv({
      name: uniqueName,
      display_name: preset.displayName,
      kind: preset.kind,
      base_url: preset.baseUrl,
      priority: preset.defaultPriority,
      weight: preset.defaultWeight,
      timeout_ms: 30000,
      egress_pool_id: undefined,
    });
    setQuickApiKey('');
    setAuthFallbackInput('');
    setAuthExtractedToken('');
    setShowApiKey(false);
    setSelectedEgressPoolId('');
    setSyncAfterSave(true);
    setShowAdvanced(false);
    setSocketPingStatus(null);

    // Buka tab auth default sesuai jenis provider
    if (preset.authLoginType === 'local_socket') {
      setAuthTab('authlogin');
    } else {
      setAuthTab('apikey');
    }

    setIsCreateModalOpen(true);
  };

  const handleOpenCustomCreate = () => {
    setSelectedPreset(null);
    setNewProv({
      name: '',
      display_name: '',
      kind: 'openai',
      base_url: 'https://api.openai.com/v1',
      priority: 100,
      weight: 100,
      timeout_ms: 30000,
      egress_pool_id: undefined,
    });
    setAuthTab('apikey');
    setQuickApiKey('');
    setAuthFallbackInput('');
    setAuthExtractedToken('');
    setShowApiKey(false);
    setSelectedEgressPoolId('');
    setSyncAfterSave(true);
    setShowAdvanced(true);
    setSocketPingStatus(null);
    setIsCreateModalOpen(true);
  };

  const handleTestSocket = async () => {
    setSocketPingStatus({ testing: true });
    const start = performance.now();
    try {
      // Tes koneksi ke base_url yang diinputkan
      const res = await fetch(`${newProv.base_url}/models`, { method: 'GET' }).catch(() =>
        fetch(`${newProv.base_url}/api/tags`, { method: 'GET' })
      );
      const latency = Math.round(performance.now() - start);
      setSocketPingStatus({ testing: false, latency, ok: res.ok });
    } catch {
      setSocketPingStatus({ testing: false, latency: 0, ok: false });
    }
  };

  const handleSaveProvider = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);
    setSavingStep('Mendaftarkan provider ke database...');

    // Tentukan token akhir berdasarkan tab yang dipilih
    let finalApiKey = '';
    if (authTab === 'apikey') {
      finalApiKey = quickApiKey.trim();
    } else {
      // Jika mode auth login
      finalApiKey = authExtractedToken || authFallbackInput.trim();
    }

    try {
      // 1. Buat Upstream Provider dengan EgressPoolID jika dipilih
      const createPayload: any = {
        ...newProv,
        egress_pool_id: selectedEgressPoolId || undefined,
      };
      const created = await api.providers.create(createPayload);

      // 2. Jika API Key diisi, langsung tambahkan kredensial terenkripsi
      if (finalApiKey) {
        setSavingStep('Mengenkripsi & menyimpan kredensial dengan amplop AES-256-GCM...');
        await api.credentials.create(created.id, {
          label: `${created.display_name || created.name} Primary Key`,
          api_key: finalApiKey,
        });
      }

      // 3. Jika dipilih tarik model otomatis, jalankan syncProviderModels
      let pulledCount = 0;
      if (syncAfterSave) {
        setSavingStep('Menghubungi upstream discovery endpoint (/v1/models)...');
        try {
          const syncRes = await api.providers.syncModels(created.id);
          pulledCount = syncRes.count;
        } catch (syncErr: any) {
          console.warn('Sync model pasca pendaftaran gagal:', syncErr);
        }
      }

      await loadData();
      setIsCreateModalOpen(false);

      if (pulledCount > 0) {
        setSyncFeedback((prev) => ({
          ...prev,
          [created.id]: `Berhasil menarik ${pulledCount} model upstream!`,
        }));
      }
    } catch (err: any) {
      alert('Gagal mendaftarkan provider: ' + (err.message || err));
    } finally {
      setIsSaving(false);
      setSavingStep('');
    }
  };

  const handleDeleteProvider = async (prov: Provider) => {
    if (
      !confirm(
        `Yakin ingin menghapus provider "${prov.display_name || prov.name}"?\nSemua pemetaan model ke provider ini akan dicabut.`
      )
    ) {
      return;
    }
    try {
      await api.providers.delete(prov.id);
      await loadData();
    } catch (err: any) {
      alert('Gagal menghapus provider: ' + (err.message || err));
    }
  };

  const handleOpenCredentials = async (prov: Provider) => {
    setSelectedProvider(prov);
    setIsCredModalOpen(true);
    try {
      const res = await api.credentials.list(prov.id);
      setCredentials(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  const handleAddCredential = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProvider) return;
    try {
      await api.credentials.create(selectedProvider.id, newCred);
      setNewCred({ label: '', api_key: '' });
      const res = await api.credentials.list(selectedProvider.id);
      setCredentials(res.items || []);
    } catch (err) {
      alert('Gagal menambah kredensial: ' + err);
    }
  };

  const handleDeleteCredential = async (credId: string) => {
    if (!selectedProvider || !confirm('Yakin ingin menghapus kredensial ini?')) return;
    try {
      await api.credentials.delete(selectedProvider.id, credId);
      setCredentials((prev) => prev.filter((c) => c.id !== credId));
    } catch (err) {
      alert('Gagal menghapus kredensial: ' + err);
    }
  };

  const handleProbe = async (prov: Provider) => {
    setProbingId(prov.id);
    setProbeResult(null);
    try {
      const res = await api.providers.probe(prov.id);
      setProbeResult({ providerId: prov.id, ...res });
      loadData();
    } catch (err: any) {
      setProbeResult({ providerId: prov.id, status: 'error', error: err.message });
    } finally {
      setProbingId(null);
    }
  };

  const handleSyncModels = async (prov: Provider) => {
    setSyncingId(prov.id);
    setSyncFeedback((prev) => ({ ...prev, [prov.id]: '' }));
    try {
      const res = await api.providers.syncModels(prov.id);
      setSyncFeedback((prev) => ({
        ...prev,
        [prov.id]: res.message || `Berhasil menyinkronkan ${res.count} model upstream.`,
      }));
      await loadData();
    } catch (err: any) {
      alert('Gagal menyinkronkan model: ' + (err.message || err));
    } finally {
      setSyncingId(null);
    }
  };

  const handleToggle = async (prov: Provider) => {
    try {
      await api.providers.toggle(prov.id, !prov.enabled);
      loadData();
    } catch (err) {
      alert('Gagal mengubah status: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Halaman */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Upstream Providers</h2>
          <p className="text-xs text-text-secondary mt-1">
            Koneksi ke penyedia AI hulu dengan rotasi kredensial AES-256, jalur Egress Proxy Pool, dan penarikan model upstream.
          </p>
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={handleOpenCustomCreate}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Custom Provider
        </Button>
      </div>

      {/* Preset AI Provider Showcase Grid */}
      <div className="p-4 rounded-xl bg-bg-surface border border-border space-y-3 shadow-sm">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Sparkles className="w-4 h-4 text-accent" />
            <h3 className="text-xs font-bold text-white uppercase tracking-wider">
              Katalog Penyedia AI Resmi (Klik Icon untuk Konfigurasi & Tarik Model)
            </h3>
          </div>
          <span className="text-[11px] font-mono text-text-muted">
            {KNOWN_PROVIDERS.length} Preset Tersedia
          </span>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-2.5">
          {KNOWN_PROVIDERS.map((preset) => {
            const installed = providers.find(
              (p) =>
                p.kind === preset.kind ||
                p.name.toLowerCase().includes(preset.id) ||
                (p.display_name && p.display_name.toLowerCase().includes(preset.displayName.toLowerCase()))
            );

            return (
              <button
                key={preset.id}
                type="button"
                onClick={() => handleSelectPreset(preset)}
                className={`group p-3 rounded-inner border text-left transition-all duration-200 flex flex-col justify-between hover:scale-[1.02] active:scale-[0.98] ${
                  installed
                    ? 'bg-bg-surface-2 border-emerald-500/40 shadow-sm hover:border-emerald-400'
                    : 'bg-bg-surface-2/40 border-border/70 hover:border-accent/40 hover:bg-bg-surface-2'
                }`}
              >
                <div className="flex items-start justify-between gap-1 mb-2 w-full">
                  <div
                    className="w-8 h-8 rounded-lg flex items-center justify-center transition-transform group-hover:scale-110 shadow-sm"
                    style={{
                      backgroundColor: preset.bgColor,
                      color: preset.color,
                      border: `1px solid ${preset.borderColor}`,
                    }}
                  >
                    <ProviderBrandIcon providerIdOrKind={preset.id} className="w-4 h-4" />
                  </div>

                  {installed ? (
                    <span className="inline-flex items-center gap-0.5 text-[9px] font-semibold text-emerald-400 bg-emerald-500/10 px-1 py-0.5 rounded border border-emerald-500/20">
                      <CheckCircle2 className="w-2.5 h-2.5" />
                      Aktif
                    </span>
                  ) : (
                    <span className="text-[10px] font-mono text-accent opacity-0 group-hover:opacity-100 transition-opacity">
                      + Pasang
                    </span>
                  )}
                </div>

                <div>
                  <div className="font-bold text-xs text-white group-hover:text-accent transition-colors truncate">
                    {preset.displayName}
                  </div>
                  <p className="text-[10px] text-text-muted line-clamp-1 mt-0.5 font-mono">
                    {preset.tag}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Daftar Provider Terdaftar */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-bold text-white tracking-tight">
            Daftar Provider Terpasang ({providers.length})
          </h3>
        </div>

        {providers.length === 0 ? (
          <div className="text-center py-12 border border-border rounded-xl bg-bg-surface">
            <Server className="w-10 h-10 text-text-muted mx-auto mb-2 opacity-50" />
            <p className="text-sm font-medium text-white">Belum ada provider yang dikonfigurasi</p>
            <p className="text-xs text-text-muted mt-1">
              Klik salah satu icon provider populer di atas untuk langsung menghubungkan API Key atau login otorisasi.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {providers.map((p) => {
              const isHealthy = p.last_health_status === 'healthy';
              const feedback = syncFeedback[p.id];
              const linkedPool = egressPools.find((ep) => ep.id === p.egress_pool_id);

              return (
                <Card key={p.id} className="flex flex-col justify-between transition-all hover:border-border/80">
                  <div>
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex items-center gap-3">
                        <div className="w-10 h-10 rounded-xl bg-bg-surface-2 border border-border flex items-center justify-center text-accent flex-shrink-0 shadow-sm">
                          <ProviderBrandIcon providerIdOrKind={p.kind} name={p.name} className="w-5 h-5" />
                        </div>
                        <div className="min-w-0">
                          <h4 className="text-sm font-bold text-white truncate">{p.display_name || p.name}</h4>
                          <span className="text-[11px] text-text-muted font-mono truncate block">
                            {p.name} ({p.kind})
                          </span>
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5 flex-shrink-0">
                        <Badge variant={isHealthy ? 'success' : 'error'}>
                          {p.last_health_status || 'unknown'}
                        </Badge>
                        <button
                          type="button"
                          onClick={() => handleDeleteProvider(p)}
                          className="p-1 text-text-muted hover:text-status-error transition-colors rounded hover:bg-status-error/10"
                          title="Hapus Provider"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>

                    <div className="mt-4 space-y-2 text-xs">
                      <div className="flex justify-between py-1 border-b border-border/40">
                        <span className="text-text-muted">Base URL</span>
                        <span className="font-mono text-text-secondary truncate max-w-[170px]" title={p.base_url}>
                          {p.base_url}
                        </span>
                      </div>
                      <div className="flex justify-between py-1 border-b border-border/40">
                        <span className="text-text-muted">Jalur Egress Proxy</span>
                        {linkedPool ? (
                          <span className="inline-flex items-center gap-1 font-mono text-[11px] text-cyan-400 bg-cyan-500/10 px-1.5 py-0.5 rounded border border-cyan-500/20 truncate max-w-[170px]">
                            <Globe className="w-2.5 h-2.5" />
                            {linkedPool.name}
                          </span>
                        ) : (
                          <span className="font-mono text-text-muted">Direct Outbound</span>
                        )}
                      </div>
                      <div className="flex justify-between py-1 border-b border-border/40">
                        <span className="text-text-muted">Prioritas / Bobot</span>
                        <span className="font-mono text-white">{p.priority} / {p.weight}</span>
                      </div>
                      <div className="flex justify-between py-1 border-b border-border/40">
                        <span className="text-text-muted">Latensi Terakhir</span>
                        <span className="font-mono text-accent">{p.last_latency_ms ? `${p.last_latency_ms} ms` : '-'}</span>
                      </div>
                    </div>

                    {/* Feedback hasil sync */}
                    {feedback && (
                      <div className="mt-3 p-2 rounded-inner bg-accent/10 border border-accent/20 text-accent text-xs font-mono flex items-center gap-1.5">
                        <CheckCircle2 className="w-3.5 h-3.5 flex-shrink-0" />
                        <span>{feedback}</span>
                      </div>
                    )}

                    {/* Hasil Probe */}
                    {probeResult && probeResult.providerId === p.id && (
                      <div className={`mt-3 p-2.5 rounded-inner text-xs flex items-center gap-2 ${
                        probeResult.status === 'healthy'
                          ? 'bg-status-success/10 text-status-success border border-status-success/20'
                          : 'bg-status-error/10 text-status-error border border-status-error/20'
                      }`}>
                        {probeResult.status === 'healthy' ? <CheckCircle2 className="w-4 h-4" /> : <XCircle className="w-4 h-4" />}
                        <span>{probeResult.status.toUpperCase()} ({probeResult.latency_ms ?? 0} ms) {probeResult.error && `- ${probeResult.error}`}</span>
                      </div>
                    )}
                  </div>

                  <div className="mt-5 pt-3 border-t border-border flex items-center justify-between gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleOpenCredentials(p)}
                      icon={<Key className="w-3.5 h-3.5" />}
                    >
                      Kredensial
                    </Button>

                    <div className="flex items-center gap-1.5">
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => handleSyncModels(p)}
                        isLoading={syncingId === p.id}
                        icon={<DownloadCloud className="w-3.5 h-3.5 text-accent" />}
                        title="Tarik katalog model otomatis dari endpoint upstream"
                      >
                        Tarik Model
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => handleProbe(p)}
                        isLoading={probingId === p.id}
                        icon={<Activity className="w-3.5 h-3.5" />}
                      >
                        Probe
                      </Button>
                      <Button
                        variant={p.enabled ? 'danger' : 'secondary'}
                        size="sm"
                        onClick={() => handleToggle(p)}
                      >
                        {p.enabled ? 'Nonaktifkan' : 'Aktifkan'}
                      </Button>
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>
        )}
      </div>

      {/* Modal Konfigurasi Cepat, Dual-Auth & Egress Proxy */}
      <Modal
        isOpen={isCreateModalOpen}
        onClose={() => !isSaving && setIsCreateModalOpen(false)}
        title={selectedPreset ? `Hubungkan ${selectedPreset.displayName}` : 'Daftarkan Upstream Provider'}
        subtitle={
          selectedPreset
            ? selectedPreset.description
            : 'Konfigurasi endpoint upstream untuk inferensi LLM, proxy pool, dan katalog model'
        }
        maxWidth="xl"
      >
        <form onSubmit={handleSaveProvider} className="space-y-4 text-xs">
          {/* Preset Brand Banner */}
          {selectedPreset && (
            <div
              className="p-3.5 rounded-inner border flex items-center justify-between gap-3"
              style={{
                backgroundColor: selectedPreset.bgColor,
                borderColor: selectedPreset.borderColor,
              }}
            >
              <div className="flex items-center gap-3">
                <div
                  className="w-10 h-10 rounded-xl flex items-center justify-center shadow-sm"
                  style={{
                    backgroundColor: selectedPreset.color,
                    color: '#000',
                  }}
                >
                  <ProviderBrandIcon providerIdOrKind={selectedPreset.id} className="w-5 h-5 text-white" />
                </div>
                <div>
                  <h4 className="font-bold text-white text-sm">{selectedPreset.displayName}</h4>
                  <span className="text-[11px] text-text-secondary font-mono">
                    Dialek: {selectedPreset.kind} • {selectedPreset.tag}
                  </span>
                </div>
              </div>

              <div className="hidden sm:block text-right">
                <span className="text-[10px] text-text-muted block font-mono uppercase">Highlight Model:</span>
                <span className="text-[11px] font-mono text-white font-semibold">
                  {selectedPreset.highlightModels.slice(0, 2).join(', ')}
                </span>
              </div>
            </div>
          )}

          {/* DUAL-AUTH TAB SELECTOR */}
          <div className="space-y-3">
            <div className="flex border-b border-border">
              <button
                type="button"
                onClick={() => setAuthTab('apikey')}
                className={`pb-2 px-3 text-xs font-semibold border-b-2 transition-all ${
                  authTab === 'apikey'
                    ? 'border-accent text-accent'
                    : 'border-transparent text-text-secondary hover:text-white'
                }`}
              >
                1. Input API Key Manual
              </button>
              <button
                type="button"
                onClick={() => setAuthTab('authlogin')}
                className={`pb-2 px-3 text-xs font-semibold border-b-2 transition-all flex items-center gap-1.5 ${
                  authTab === 'authlogin'
                    ? 'border-accent text-accent'
                    : 'border-transparent text-text-secondary hover:text-white'
                }`}
              >
                <ArrowUpRight className="w-3.5 h-3.5" />
                2. Auth Login Terpandu
              </button>
            </div>

            {/* TAB 1: API KEY MANUAL */}
            {authTab === 'apikey' && (
              <div className="p-3.5 bg-bg-surface-2/80 rounded-inner border border-border space-y-2">
                <div className="flex items-center justify-between">
                  <label className="block font-bold text-white text-xs">
                    API Key / Secret Token
                  </label>
                  {selectedPreset?.apiKeyHelp && (
                    <span className="text-[10px] text-accent font-mono flex items-center gap-1">
                      <ExternalLink className="w-2.5 h-2.5" />
                      {selectedPreset.apiKeyHelp}
                    </span>
                  )}
                </div>

                <div className="relative">
                  <input
                    type={showApiKey ? 'text' : 'password'}
                    placeholder={selectedPreset?.apiKeyPlaceholder || 'Masukkan secret key rahasia...'}
                    value={quickApiKey}
                    onChange={(e) => setQuickApiKey(e.target.value)}
                    className="w-full px-3 py-2 pr-10 bg-bg-base border border-border rounded-nav text-white font-mono text-xs focus:border-accent focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={() => setShowApiKey(!showApiKey)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white"
                  >
                    {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
                <p className="text-[10px] text-text-muted flex items-center gap-1">
                  <Lock className="w-3 h-3 text-accent" />
                  Kredensial langsung dienkripsi amplop AES-256-GCM tingkat record PostgreSQL dengan AAD.
                </p>
              </div>
            )}

            {/* TAB 2: AUTH LOGIN TERPANDU */}
            {authTab === 'authlogin' && (
              <div className="p-3.5 bg-bg-surface-2/80 rounded-inner border border-border space-y-3">
                {/* Mode A: OAuth Fallback Callback URL */}
                {selectedPreset?.authLoginType === 'oauth_fallback' && (
                  <div className="space-y-3">
                    <div className="flex items-start gap-2.5 text-xs text-text-secondary">
                      <span className="w-5 h-5 rounded-full bg-accent/20 text-accent font-bold flex items-center justify-center flex-shrink-0 text-[11px]">
                        1
                      </span>
                      <div>
                        <p className="font-semibold text-white">Buka Halaman Otorisasi Resmi</p>
                        <p className="text-[11px] text-text-muted mt-0.5">
                          {selectedPreset.authInstructions}
                        </p>
                        {selectedPreset.authLoginUrl && (
                          <a
                            href={selectedPreset.authLoginUrl}
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex items-center gap-1.5 mt-2 px-3 py-1.5 rounded-nav bg-accent text-black font-semibold text-xs hover:opacity-90 transition-opacity"
                          >
                            <span>{selectedPreset.authLoginLabel || 'Buka Halaman Otorisasi'}</span>
                            <ArrowUpRight className="w-3.5 h-3.5" />
                          </a>
                        )}
                      </div>
                    </div>

                    <div className="flex items-start gap-2.5 text-xs text-text-secondary pt-2 border-t border-border/50">
                      <span className="w-5 h-5 rounded-full bg-accent/20 text-accent font-bold flex items-center justify-center flex-shrink-0 text-[11px]">
                        2
                      </span>
                      <div className="flex-1 space-y-1.5">
                        <label className="block font-semibold text-white">
                          Salin URL Fallback / Callback dan Tempelkan di Sini:
                        </label>
                        <textarea
                          rows={2}
                          value={authFallbackInput}
                          onChange={(e) => handleFallbackInputChange(e.target.value)}
                          placeholder="Tempel seluruh URL redirect (misal: https://.../?code=xxx atau token) di sini..."
                          className="w-full px-3 py-2 bg-bg-base border border-border rounded-nav text-white font-mono text-xs focus:border-accent focus:outline-none"
                        />
                        {authExtractedToken && (
                          <div className="p-2 rounded bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                            <Check className="w-3.5 h-3.5 flex-shrink-0" />
                            <span>
                              Token/Kode Otorisasi Terdeteksi:{' '}
                              <strong className="text-white">
                                {authExtractedToken.length > 25
                                  ? `${authExtractedToken.slice(0, 12)}...${authExtractedToken.slice(-6)}`
                                  : authExtractedToken}
                              </strong>
                            </span>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                )}

                {/* Mode B: Direct Console Token Portal */}
                {selectedPreset?.authLoginType === 'console_token' && (
                  <div className="space-y-3">
                    <div className="flex items-start gap-2.5 text-xs text-text-secondary">
                      <span className="w-5 h-5 rounded-full bg-accent/20 text-accent font-bold flex items-center justify-center flex-shrink-0 text-[11px]">
                        1
                      </span>
                      <div>
                        <p className="font-semibold text-white">Ambil Token dari Konsol Resmi</p>
                        <p className="text-[11px] text-text-muted mt-0.5">
                          {selectedPreset.authInstructions}
                        </p>
                        {selectedPreset.authLoginUrl && (
                          <a
                            href={selectedPreset.authLoginUrl}
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex items-center gap-1.5 mt-2 px-3 py-1.5 rounded-nav bg-accent text-black font-semibold text-xs hover:opacity-90 transition-opacity"
                          >
                            <span>{selectedPreset.authLoginLabel || 'Buka Konsol Provider'}</span>
                            <ExternalLink className="w-3.5 h-3.5" />
                          </a>
                        )}
                      </div>
                    </div>

                    <div className="flex items-start gap-2.5 text-xs text-text-secondary pt-2 border-t border-border/50">
                      <span className="w-5 h-5 rounded-full bg-accent/20 text-accent font-bold flex items-center justify-center flex-shrink-0 text-[11px]">
                        2
                      </span>
                      <div className="flex-1 space-y-1.5">
                        <label className="block font-semibold text-white">
                          Tempelkan Token yang Anda Peroleh:
                        </label>
                        <div className="relative">
                          <input
                            type={showApiKey ? 'text' : 'password'}
                            placeholder={selectedPreset.authFallbackHint || 'Tempel token di sini...'}
                            value={authFallbackInput}
                            onChange={(e) => handleFallbackInputChange(e.target.value)}
                            className="w-full px-3 py-2 pr-10 bg-bg-base border border-border rounded-nav text-white font-mono text-xs focus:border-accent focus:outline-none"
                          />
                          <button
                            type="button"
                            onClick={() => setShowApiKey(!showApiKey)}
                            className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white"
                          >
                            {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {/* Mode C: Local Socket Zero-Config */}
                {(!selectedPreset || selectedPreset.authLoginType === 'local_socket') && (
                  <div className="space-y-2">
                    <div className="flex items-center gap-2 text-white font-semibold text-xs">
                      <Server className="w-4 h-4 text-accent" />
                      <span>Socket Server Lokal (Port Otomatis)</span>
                    </div>
                    <p className="text-[11px] text-text-muted">
                      {selectedPreset?.authInstructions ||
                        'Provider lokal tidak memerlukan API key eksternal. Pastikan service berjalan di port yang ditentukan.'}
                    </p>
                    <div className="pt-2 flex items-center gap-2">
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        onClick={handleTestSocket}
                        isLoading={socketPingStatus?.testing}
                        icon={<Activity className="w-3.5 h-3.5 text-accent" />}
                      >
                        Uji Respons Socket ({newProv.base_url})
                      </Button>
                      {socketPingStatus && !socketPingStatus.testing && (
                        <span
                          className={`text-xs font-mono font-semibold flex items-center gap-1 ${
                            socketPingStatus.ok ? 'text-emerald-400' : 'text-red-400'
                          }`}
                        >
                          {socketPingStatus.ok ? <CheckCircle2 className="w-3.5 h-3.5" /> : <XCircle className="w-3.5 h-3.5" />}
                          {socketPingStatus.ok
                            ? `Tersambung (${socketPingStatus.latency} ms)`
                            : 'Gagal tersambung ke socket'}
                        </span>
                      )}
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>

          {/* INTEGRASI EGRESS PROXY POOL */}
          <div className="p-3.5 bg-bg-surface-2/60 rounded-inner border border-border space-y-2">
            <div className="flex items-center justify-between">
              <label className="block font-bold text-white text-xs flex items-center gap-1.5">
                <Globe className="w-3.5 h-3.5 text-cyan-400" />
                <span>Jalur Proxy Keluar (Egress Pool)</span>
              </label>
              <span className="text-[10px] text-text-muted">
                {egressPools.length} Proxy Pool Tersedia
              </span>
            </div>

            <select
              value={selectedEgressPoolId}
              onChange={(e) => setSelectedEgressPoolId(e.target.value)}
              className="w-full px-3 py-2 bg-bg-base border border-border rounded-nav text-white font-mono text-xs focus:border-accent focus:outline-none"
            >
              <option value="">Direct Outbound (Tanpa Proxy / IP Server Langsung)</option>
              {egressPools.map((pool) => (
                <option key={pool.id} value={pool.id}>
                  {pool.name} ({pool.kind.toUpperCase()}) {pool.region ? `• Region: ${pool.region}` : ''}
                </option>
              ))}
            </select>
            <p className="text-[10px] text-text-muted">
              Seluruh permintaan inferensi dan penarikan model ke provider ini akan dialirkan melalui proxy pool yang dipilih.
            </p>
          </div>

          {/* OPSI TARIK MODEL OTOMATIS */}
          <label className="flex items-start gap-3 p-3 rounded-inner bg-bg-surface-2/50 border border-border cursor-pointer hover:bg-bg-surface-2 transition-colors">
            <input
              type="checkbox"
              checked={syncAfterSave}
              onChange={(e) => setSyncAfterSave(e.target.checked)}
              className="mt-0.5 rounded border-border bg-bg-base text-accent focus:ring-0 cursor-pointer"
            />
            <div>
              <div className="font-semibold text-xs text-white flex items-center gap-1.5">
                <DownloadCloud className="w-3.5 h-3.5 text-accent" />
                <span>Otomatis Tarik Katalog Model dari Upstream setelah Disimpan</span>
              </div>
              <p className="text-[11px] text-text-muted mt-0.5">
                Route-X akan langsung memanggil discovery endpoint (<span className="font-mono text-text-secondary">/v1/models</span>) untuk mendaftarkan semua model yang tersedia secara otomatis.
              </p>
            </div>
          </label>

          {/* Parameter Inti Provider */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Identifier (Unik)</label>
              <input
                type="text"
                required
                placeholder="openai-main"
                value={newProv.name}
                onChange={(e) => setNewProv({ ...newProv, name: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Tampilan</label>
              <input
                type="text"
                required
                placeholder="OpenAI Global"
                value={newProv.display_name}
                onChange={(e) => setNewProv({ ...newProv, display_name: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
          </div>

          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Base URL Upstream</label>
            <input
              type="url"
              required
              value={newProv.base_url}
              onChange={(e) => setNewProv({ ...newProv, base_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>

          {/* Tombol Toggle Pengaturan Lanjutan */}
          <button
            type="button"
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="flex items-center gap-1 text-xs text-text-muted hover:text-white font-medium"
          >
            {showAdvanced ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
            <span>Pengaturan Lanjutan (Prioritas, Bobot, Adaptor)</span>
          </button>

          {showAdvanced && (
            <div className="p-3 bg-bg-surface-2/40 rounded-inner border border-border space-y-3">
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">Jenis Adaptor</label>
                <select
                  value={newProv.kind}
                  onChange={(e) => setNewProv({ ...newProv, kind: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
                >
                  <option value="openai">OpenAI</option>
                  <option value="anthropic">Anthropic</option>
                  <option value="google">Google Gemini</option>
                  <option value="openai-compatible">OpenAI Compatible (vLLM / Ollama / DeepSeek / Groq / Cerebras / Qwen)</option>
                  <option value="custom">Custom HTTP</option>
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block font-semibold text-text-secondary uppercase mb-1">Prioritas</label>
                  <input
                    type="number"
                    value={newProv.priority}
                    onChange={(e) => setNewProv({ ...newProv, priority: parseInt(e.target.value) || 0 })}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
                  />
                </div>
                <div>
                  <label className="block font-semibold text-text-secondary uppercase mb-1">Bobot (Weight)</label>
                  <input
                    type="number"
                    value={newProv.weight}
                    onChange={(e) => setNewProv({ ...newProv, weight: parseInt(e.target.value) || 0 })}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
                  />
                </div>
              </div>
            </div>
          )}

          {/* Progress Step if Saving */}
          {isSaving && savingStep && (
            <div className="p-3 rounded-inner bg-accent/10 border border-accent/30 text-accent font-mono text-xs flex items-center gap-2">
              <RefreshCw className="w-4 h-4 animate-spin flex-shrink-0" />
              <span>{savingStep}</span>
            </div>
          )}

          <Button
            type="submit"
            variant="primary"
            size="md"
            className="w-full mt-2 font-bold"
            disabled={isSaving}
            isLoading={isSaving}
            icon={syncAfterSave ? <DownloadCloud className="w-4 h-4" /> : <ShieldCheck className="w-4 h-4" />}
          >
            {syncAfterSave ? 'Simpan Provider & Tarik Model Otomatis' : 'Simpan Provider'}
          </Button>
        </form>
      </Modal>

      {/* Modal Kredensial Provider */}
      {selectedProvider && (
        <Modal
          isOpen={isCredModalOpen}
          onClose={() => setIsCredModalOpen(false)}
          title={`Kredensial API Key: ${selectedProvider.display_name}`}
          subtitle="Enkripsi amplop AES-256-GCM tingkat record dengan AAD"
        >
          <div className="space-y-4">
            <form onSubmit={handleAddCredential} className="p-3 bg-bg-surface-2 rounded-inner border border-border space-y-3">
              <h4 className="text-xs font-semibold text-white">Tambah Kredensial Baru</h4>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                <input
                  type="text"
                  placeholder="Label (contoh: Prod Key 1)"
                  required
                  value={newCred.label}
                  onChange={(e) => setNewCred({ ...newCred, label: e.target.value })}
                  className="px-2.5 py-1.5 bg-bg-surface border border-border rounded-nav text-xs text-white"
                />
                <input
                  type="password"
                  placeholder="API Key / Token Rahasia"
                  required
                  value={newCred.api_key}
                  onChange={(e) => setNewCred({ ...newCred, api_key: e.target.value })}
                  className="px-2.5 py-1.5 bg-bg-surface border border-border rounded-nav text-xs text-white"
                />
              </div>
              <Button type="submit" variant="primary" size="sm" className="w-full">
                Enkripsi & Simpan Kredensial
              </Button>
            </form>

            <div className="space-y-2">
              <h4 className="text-xs font-semibold text-text-muted uppercase">Daftar Kredensial Aktif</h4>
              {credentials.length === 0 ? (
                <p className="text-xs text-text-muted italic">Belum ada kredensial yang tersimpan.</p>
              ) : (
                credentials.map((c) => (
                  <div key={c.id} className="p-3 bg-bg-surface-2/60 rounded-inner border border-border flex items-center justify-between text-xs">
                    <div>
                      <div className="font-semibold text-white">{c.label}</div>
                      <div className="font-mono text-[11px] text-accent mt-0.5">{c.masked_hint}</div>
                    </div>
                    <button
                      onClick={() => handleDeleteCredential(c.id)}
                      className="p-1 text-text-muted hover:text-status-error transition-colors"
                      title="Hapus Kredensial"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                ))
              )}
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
};
