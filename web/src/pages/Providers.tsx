import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Provider, Credential } from '../types';
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
} from 'lucide-react';
import {
  KNOWN_PROVIDERS,
  ProviderBrandIcon,
  type KnownProviderPreset,
} from '../components/providers/ProviderIcons';

export const Providers: React.FC = () => {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [isCredModalOpen, setIsCredModalOpen] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [probingId, setProbingId] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [probeResult, setProbeResult] = useState<any | null>(null);
  const [syncFeedback, setSyncFeedback] = useState<Record<string, string>>({});

  // Preset & Form states
  const [selectedPreset, setSelectedPreset] = useState<KnownProviderPreset | null>(null);
  const [quickApiKey, setQuickApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [syncAfterSave, setSyncAfterSave] = useState(true);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [savingStep, setSavingStep] = useState('');

  const [newProv, setNewProv] = useState({
    name: '',
    display_name: '',
    kind: 'openai',
    base_url: 'https://api.openai.com/v1',
    priority: 100,
    weight: 100,
    timeout_ms: 30000,
  });

  const [newCred, setNewCred] = useState({
    label: '',
    api_key: '',
  });

  const loadProviders = async () => {
    try {
      const res = await api.providers.list();
      setProviders(res.items || []);
    } catch (err) {
      console.error('Gagal memuat daftar provider:', err);
    }
  };

  useEffect(() => {
    loadProviders();
  }, []);

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
    });
    setQuickApiKey('');
    setShowApiKey(false);
    setSyncAfterSave(true);
    setShowAdvanced(false);
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
    });
    setQuickApiKey('');
    setShowApiKey(false);
    setSyncAfterSave(true);
    setShowAdvanced(true);
    setIsCreateModalOpen(true);
  };

  const handleSaveProvider = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);
    setSavingStep('Mendaftarkan provider ke database...');
    try {
      // 1. Buat Upstream Provider
      const created = await api.providers.create(newProv);

      // 2. Jika API Key diisi, langsung tambahkan kredensial terenkripsi
      if (quickApiKey.trim()) {
        setSavingStep('Mengenkripsi & menyimpan kredensial API Key...');
        await api.credentials.create(created.id, {
          label: `${created.display_name || created.name} Primary Key`,
          api_key: quickApiKey.trim(),
        });
      }

      // 3. Jika dipilih tarik model otomatis, jalankan syncProviderModels
      let pulledCount = 0;
      if (syncAfterSave) {
        setSavingStep('Menghubungi upstream discovery (/v1/models)...');
        try {
          const syncRes = await api.providers.syncModels(created.id);
          pulledCount = syncRes.count;
        } catch (syncErr: any) {
          console.warn('Sync model pasca pendaftaran gagal:', syncErr);
        }
      }

      await loadProviders();
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
      await loadProviders();
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
      loadProviders();
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
      await loadProviders();
    } catch (err: any) {
      alert('Gagal menyinkronkan model: ' + (err.message || err));
    } finally {
      setSyncingId(null);
    }
  };

  const handleToggle = async (prov: Provider) => {
    try {
      await api.providers.toggle(prov.id, !prov.enabled);
      loadProviders();
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
            Koneksi ke penyedia AI hulu dengan rotasi kredensial terenkripsi AES-256 dan sinkronisasi model otomatis.
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
              Penyedia AI Populer (Klik Icon untuk Konfigurasi Cepat & Tarik Model)
            </h3>
          </div>
          <span className="text-[11px] font-mono text-text-muted">
            {KNOWN_PROVIDERS.length} Preset Tersedia
          </span>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-7 gap-2.5">
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
                    ? 'bg-bg-surface-2 border-emerald-500/30 hover:border-emerald-400'
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
              Klik salah satu icon provider populer di atas untuk langsung menghubungkan API Key.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {providers.map((p) => {
              const isHealthy = p.last_health_status === 'healthy';
              const feedback = syncFeedback[p.id];

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

      {/* Modal Konfigurasi Cepat & Tarik Model */}
      <Modal
        isOpen={isCreateModalOpen}
        onClose={() => !isSaving && setIsCreateModalOpen(false)}
        title={selectedPreset ? `Hubungkan ${selectedPreset.displayName}` : 'Daftarkan Upstream Provider'}
        subtitle={
          selectedPreset
            ? selectedPreset.description
            : 'Konfigurasi endpoint upstream untuk inferensi LLM dan katalog model'
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

          {/* Input Kredensial API Key Terdepan */}
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
                placeholder={selectedPreset?.apiKeyPlaceholder || 'Masukkan token rahasia API key...'}
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
            <p className="text-[10px] text-text-muted">
              🔒 Kredensial dienkripsi amplop AES-256-GCM tingkat record PostgreSQL dengan AAD unik.
            </p>
          </div>

          {/* Opsi Tarik Model Otomatis */}
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
                Route-X akan langsung menghubungi discovery endpoint (<span className="font-mono text-text-secondary">/v1/models</span>) untuk mendaftarkan semua model yang tersedia secara otomatis.
              </p>
            </div>
          </label>

          {/* Parameter Inti */}
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
                  <option value="openai-compatible">OpenAI Compatible (vLLM / Ollama / DeepSeek / Groq)</option>
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
            icon={syncAfterSave ? <DownloadCloud className="w-4 h-4" /> : <CheckCircle2 className="w-4 h-4" />}
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
