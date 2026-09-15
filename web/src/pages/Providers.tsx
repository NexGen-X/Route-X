// @ts-nocheck
import React, { useEffect, useState, useMemo, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Provider, Credential, EgressPool, ProviderModel } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Drawer } from '../components/common/Drawer';
import {
  Server,
  Plus,
  Power,
  RefreshCw,
  Trash2,
  CheckCircle2,
  Globe,
  Activity,
  DownloadCloud,
  Eye,
  EyeOff,
  Copy,
  Check,
  Sparkles,
  ChevronDown,
  ChevronUp,
  Shield,
  Bot,
  KeyRound,
  Sliders,
  Search,
  FlaskConical,
  Zap,
  AlertTriangle,
  ArrowUpRight,
  ExternalLink,
  Lock,
  XCircle,
} from 'lucide-react';
import {
  KNOWN_PROVIDERS,
  ProviderBrandIcon,
  type KnownProviderPreset,
} from '../components/providers/ProviderIcons';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { copyTextToClipboard } from '../utils/clipboard';
import { extractAuthTokenFromInput } from '../utils/authExtractor';

interface ModelTestResult {
  ok: boolean;
  latency_ms: number;
  timestamp: string;
  error?: string;
  message?: string;
}

export const Providers: React.FC = () => {
  const { toast, confirmModal } = useToast();

  const queryClient = useQueryClient();

  // Data Utama (TanStack Query)
  const { data: provsData = [], isLoading: isProvidersLoading, isError: isProvidersError, error: providersError, refetch: refetchProviders } = useQuery({
    queryKey: ['providers'],
    queryFn: async () => {
      const res = await api.providers.list();
      return Array.isArray(res) ? res : (res as any).items || [];
    },
  });
  const providers: Provider[] = provsData;

  const { data: poolsData = [], isError: isPoolsError, error: poolsError } = useQuery({
    queryKey: ['egressPools'],
    queryFn: async () => {
      const res = await api.egress.list();
      return res.items || [];
    },
  });
  const egressPools: EgressPool[] = poolsData;

  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  // Penjaga race: abaikan respons basi bila user sudah pindah ke provider lain.
  const providerDetailsReqRef = useRef(0);

  // Data Child untuk Selected Provider
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [models, setModels] = useState<ProviderModel[]>([]);
  const [providerDetailsError, setProviderDetailsError] = useState<string | null>(null);
  const [modelSearch, setModelSearch] = useState('');

  // Status Aksi & Pengujian
  const [probingId, setProbingId] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [testingModelId, setTestingModelId] = useState<string | null>(null);
  const [copiedModelId, setCopiedModelId] = useState<string | null>(null);
  const [copiedTokenId, setCopiedTokenId] = useState<string | null>(null);
  const [modelTestResults, setModelTestResults] = useState<Record<string, ModelTestResult>>({});
  // Timer indikator salin; disimpan agar bisa dibatalkan saat unmount.
  const copyTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  // Batalkan timer salin yang tersisa saat unmount.
  useEffect(() => {
    return () => {
      copyTimersRef.current.forEach((t) => clearTimeout(t));
      copyTimersRef.current = [];
    };
  }, []);

  // Drawer Konfigurasi & Tab Aktif
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [drawerTab, setDrawerTab] = useState<'models' | 'credentials' | 'settings'>('models');

  // Modals & Accordion
  const [isCatalogExpanded, setIsCatalogExpanded] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isAddModelModalOpen, setIsAddModelModalOpen] = useState(false);

  // State Form Create Provider Baru / Preset
  const [selectedPreset, setSelectedPreset] = useState<KnownProviderPreset | null>(null);

  // State Form Konfigurasi Provider
  const [configForm, setConfigForm] = useState({
    display_name: '',
    base_url: '',
    priority: 100,
    weight: 100,
    timeout_ms: 30000,
    enabled: true,
  });
  const [isSavingConfig, setIsSavingConfig] = useState(false);

  // ---------------------------------------------------------------------------
  // Helper: Deteksi Custom / Manual Provider
  // ---------------------------------------------------------------------------
  const isCustomProvider = (p: Provider | null | undefined): boolean => {
    if (!p) return false;
    const nameLower = p.name.toLowerCase();
    const displayLower = (p.display_name || '').toLowerCase();
    return !KNOWN_PROVIDERS.some(
      (kp) =>
        nameLower.includes(kp.id.toLowerCase()) ||
        nameLower.includes(kp.name.toLowerCase()) ||
        (displayLower && displayLower.includes(kp.displayName.toLowerCase()))
    );
  };

  const isManualProvider = useMemo(() => isCustomProvider(selectedProvider), [selectedProvider]);

  // ---------------------------------------------------------------------------
  // Load Data Utama
  // ---------------------------------------------------------------------------
  const loadData = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['providers'] }),
      queryClient.invalidateQueries({ queryKey: ['egressPools'] }),
    ]);
  };

  // Sinkronisasi selectedProvider ketika data query diperbarui
  useEffect(() => {
    if (selectedProvider) {
      const found = providers.find((p: Provider) => p.id === selectedProvider.id);
      if (found) {
        setSelectedProvider(found);
        setConfigForm({
          display_name: found.display_name || found.name,
          base_url: found.base_url,
          priority: found.priority,
          weight: found.weight,
          timeout_ms: found.timeout_ms || 30000,
          enabled: found.enabled,
        });
      }
    }
  }, [providers]);

  // ---------------------------------------------------------------------------
  // Load Detail Child untuk Provider Terpilih (Credentials & Models)
  // ---------------------------------------------------------------------------
  const loadProviderDetails = async (providerId: string) => {
    const reqId = ++providerDetailsReqRef.current;
    setProviderDetailsError(null);
    try {
      const [credsRes, modsRes] = await Promise.all([
        api.credentials.list(providerId),
        api.providers.models(providerId),
      ]);
      if (providerDetailsReqRef.current !== reqId) return;
      setCredentials(credsRes.items || []);
      setModels(modsRes.items || []);
    } catch (err) {
      if (providerDetailsReqRef.current !== reqId) return;
      setCredentials([]);
      setModels([]);
      setProviderDetailsError(err instanceof Error ? err.message : String(err));
    }
  };

  // Buka Drawer Konfigurasi untuk Provider Tertentu
  const handleOpenDrawer = (prov: Provider, tab: 'models' | 'credentials' | 'settings' = 'models') => {
    setSelectedProvider(prov);
    setDrawerTab(tab);
    setConfigForm({
      display_name: prov.display_name || prov.name,
      base_url: prov.base_url,
      priority: prov.priority,
      weight: prov.weight,
      timeout_ms: prov.timeout_ms || 30000,
      enabled: prov.enabled,
    });
    setIsAddingKeyInline(false);
    setIsDrawerOpen(true);
    loadProviderDetails(prov.id);
  };

  // ---------------------------------------------------------------------------
  // Handlers: Aksi Cepat Provider
  // ---------------------------------------------------------------------------
  const handleProbe = async (prov: Provider) => {
    setProbingId(prov.id);
    try {
      const res = await api.providers.probe(prov.id);
      toast.success(`Probe ${prov.display_name || prov.name}: ${res.status.toUpperCase()} (${res.latency_ms} ms)`);
      await loadData();
    } catch (err: any) {
      toast.error('Gagal melakukan probe provider: ' + (err.message || err));
    } finally {
      setProbingId(null);
    }
  };

  const handleSyncModels = async (prov: Provider) => {
    setSyncingId(prov.id);
    try {
      const res = await api.providers.syncModels(prov.id);
      const msg = res.message || `Berhasil menyinkronkan ${res.count} model upstream.`;
      toast.success(msg);
      await loadProviderDetails(prov.id);
    } catch (err: any) {
      toast.error('Gagal menarik model upstream: ' + (err.message || err));
    } finally {
      setSyncingId(null);
    }
  };

  const handleDeleteProvider = async (prov: Provider) => {
    const confirmed = await confirmModal({
      title: `Hapus Provider "${prov.display_name || prov.name}"?`,
      message: `Tindakan ini akan menghapus permanen provider ini beserta seluruh kredensial API key dan pemetaan model upstream dari basis data.`,
      confirmText: 'Hapus Permanen',
      danger: true,
    });
    if (!confirmed) return;

    try {
      await api.providers.delete(prov.id);
      toast.success(`Provider "${prov.display_name || prov.name}" berhasil dihapus`);
      setIsDrawerOpen(false);
      setSelectedProvider(null);
      await loadData();
    } catch (err) {
      toast.error('Gagal menghapus provider: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  // ---------------------------------------------------------------------------
  // Handlers: API Keys
  // ---------------------------------------------------------------------------
  const handleCreateKey = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;
    const formData = new FormData(e.currentTarget);
    const keyLabel = ((formData.get('label') as string) || newKeyForm.label || '').trim();
    const keySecret = ((formData.get('api_key') as string) || newKeyForm.api_key || '').trim();

    if (!keySecret) {
      toast.error('Secret token API key wajib diisi');
      return;
    }

    setIsSavingKey(true);
    try {
      await api.credentials.create(selectedProvider.id, {
        label: keyLabel || 'Primary API Key',
        api_key: keySecret,
      });
      toast.success('API Key berhasil ditambahkan dengan enkripsi AES-256-GCM');
      setIsAddingKeyInline(false);
      setNewKeyForm({ label: 'Backup API Key', api_key: '' });
      await loadProviderDetails(selectedProvider.id);
    } catch (err) {
      toast.error('Gagal menambahkan API Key: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsSavingKey(false);
    }
  };

  const handleToggleKey = async (cred: Credential) => {
    if (!selectedProvider) return;
    try {
      await api.credentials.toggle(selectedProvider.id, cred.id, !cred.enabled);
      toast.success(`API Key "${cred.label}" ${!cred.enabled ? 'diaktifkan' : 'dinonaktifkan'}`);
      await loadProviderDetails(selectedProvider.id);
    } catch (err) {
      toast.error('Gagal mengubah status key: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDeleteKey = async (cred: Credential) => {
    if (!selectedProvider) return;
    const confirmed = await confirmModal({
      title: `Hapus API Key "${cred.label}"?`,
      message: 'Kredensial ini akan dihapus permanen. Worker gateway tidak dapat lagi menggunakan token ini untuk inferensi.',
      confirmText: 'Hapus Kredensial',
      danger: true,
    });
    if (!confirmed) return;

    try {
      await api.credentials.delete(selectedProvider.id, cred.id);
      toast.success(`API Key "${cred.label}" berhasil dihapus`);
      await loadProviderDetails(selectedProvider.id);
    } catch (err) {
      toast.error('Gagal menghapus API Key: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  // ---------------------------------------------------------------------------
  // Handlers: Model Upstream Terhubung
  // ---------------------------------------------------------------------------
  const handleTestModel = async (modelMapping: ProviderModel) => {
    if (!selectedProvider) return;
    setTestingModelId(modelMapping.id);
    try {
      const res = await api.providers.testModel(
        selectedProvider.id,
        modelMapping.upstream_model_name
      );
      const isOk = res.status === 'healthy' || res.status === 'ok' || res.status === 'success';

      setModelTestResults((prev) => ({
        ...prev,
        [modelMapping.id]: {
          ok: isOk,
          latency_ms: res.latency_ms,
          timestamp: new Date().toLocaleTimeString(),
          error: res.error,
          message: res.message,
        },
      }));

      if (isOk) {
        toast.success(`Model ${modelMapping.upstream_model_name} aktif (200 OK, ${res.latency_ms} ms)`);
      } else {
        toast.error(`Model ${modelMapping.upstream_model_name} gagal: ${res.error || 'Galat inferensi'}`);
      }
    } catch (err: any) {
      setModelTestResults((prev) => ({
        ...prev,
        [modelMapping.id]: {
          ok: false,
          latency_ms: 0,
          timestamp: new Date().toLocaleTimeString(),
          error: err.message || String(err),
        },
      }));
      toast.error('Gagal menguji model: ' + (err.message || err));
    } finally {
      setTestingModelId(null);
    }
  };

  const handleDeleteModel = async (modelMapping: ProviderModel) => {
    if (!selectedProvider) return;
    const confirmed = await confirmModal({
      title: `Hapus Model "${modelMapping.upstream_model_name}"?`,
      message: `Pemetaan model ini akan dihapus dari provider ${selectedProvider.display_name || selectedProvider.name}.`,
      confirmText: 'Hapus Model',
      danger: true,
    });
    if (!confirmed) return;

    try {
      await api.providers.deleteModel(selectedProvider.id, modelMapping.id);
      toast.success(`Model "${modelMapping.upstream_model_name}" berhasil dicopot`);
      await loadProviderDetails(selectedProvider.id);
    } catch (err) {
      toast.error('Gagal menghapus model: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleAddModelManual = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;
    const formData = new FormData(e.currentTarget);
    const trimmed = ((formData.get('model_name') as string) || newModelName || '').trim();
    if (!trimmed) {
      toast.error('Nama model wajib diisi');
      return;
    }

    setIsSavingModel(true);
    try {
      await api.providers.addModel(selectedProvider.id, trimmed);
      toast.success(`Model "${trimmed}" berhasil didaftarkan`);
      setNewModelName('');
      setIsAddModelModalOpen(false);
      await loadProviderDetails(selectedProvider.id);
    } catch (err: any) {
      toast.error('Gagal menambahkan model: ' + (err.message || err));
    } finally {
      setIsSavingModel(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Handlers: Katalog Proxy & Konfigurasi Teknis
  // ---------------------------------------------------------------------------
  const handleSelectProxyPreset = async (poolId: string | null, poolName: string) => {
    if (!selectedProvider) return;
    try {
      await api.providers.update(selectedProvider.id, {
        egress_pool_id: poolId === null ? "" : poolId,
      });
      toast.success(`Jalur proxy diubah ke: ${poolName}`);
      await loadData();
    } catch (err) {
      toast.error('Gagal mengubah jalur proxy: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleSaveConfig = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProvider) return;
    setIsSavingConfig(true);
    try {
      await api.providers.update(selectedProvider.id, {
        display_name: configForm.display_name.trim(),
        base_url: configForm.base_url.trim(),
        priority: configForm.priority,
        weight: configForm.weight,
        timeout_ms: configForm.timeout_ms,
        enabled: configForm.enabled,
      });
      toast.success('Konfigurasi provider berhasil disimpan');
      await loadData();
    } catch (err) {
      toast.error('Gagal menyimpan konfigurasi: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsSavingConfig(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Handlers: Form Tambah Provider Baru / Preset
  // ---------------------------------------------------------------------------
  const handleSelectPreset = (preset: KnownProviderPreset) => {
    setSelectedPreset(preset);
    setIsCreateModalOpen(true);
  };

  const handleOpenCustomCreate = () => {
    setSelectedPreset(null);
    setIsCreateModalOpen(true);
  };

  const getModelBadges = (modelName: string, providerKind: string) => {
    const lower = modelName.toLowerCase();
    let brand = 'Upstream';
    let context = 'Standard';

    if (lower.includes('claude')) {
      brand = 'Anthropic';
      context = '200K';
    } else if (lower.includes('gpt-4') || lower.includes('o1') || lower.includes('o3')) {
      brand = 'OpenAI';
      context = '128K';
    } else if (lower.includes('deepseek')) {
      brand = 'DeepSeek';
      context = lower.includes('r1') ? '128K' : '64K';
    } else if (lower.includes('gemini')) {
      brand = 'Google';
      context = '1M';
    } else if (lower.includes('llama')) {
      brand = 'Meta';
      context = '128K';
    } else if (lower.includes('mistral') || lower.includes('codestral')) {
      brand = 'Mistral';
      context = '32K';
    } else if (providerKind === 'ollama') {
      brand = 'Local';
      context = 'Local Host';
    }

    return { brand, context };
  };

  const filteredModels = useMemo(() => {
    if (!modelSearch.trim()) return models;
    const term = modelSearch.toLowerCase();
    return models.filter((m) => m.upstream_model_name.toLowerCase().includes(term));
  }, [models, modelSearch]);

  const copyWithFeedback = async (text: string, successMessage: string, onSuccess?: () => void) => {
    try {
      await copyTextToClipboard(text);
      onSuccess?.();
      toast.success(successMessage);
    } catch (err) {
      toast.error('Gagal menyalin ke clipboard: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const healthyCount = useMemo(
    () => providers.filter((p) => p.last_health_status === 'healthy').length,
    [providers]
  );

  return (
    <div className="space-y-6">
      {/* ------------------------------------------------------------------- */}
      {/* 1. HEADER HALAMAN & KATALOG PRESET SHOWCASE                         */}
      {/* ------------------------------------------------------------------- */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white flex items-center gap-2.5">
            <Server className="w-6 h-6 text-accent" />
            <span>Upstream Providers</span>
          </h2>
          <p className="text-xs text-text-secondary mt-1">
            Koneksi ke penyedia AI, manajemen kredensial terenkripsi AES-256-GCM, katalog model terhubung, dan perutean proxy.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={handleOpenCustomCreate}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Provider Manual
        </Button>
      </div>

      {/* Preset Showcase Accordion */}
      <div className="rounded-xl border border-border/80 bg-bg-surface-2/40 p-4 transition-all">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="w-7 h-7 rounded-lg bg-accent/10 border border-accent/20 flex items-center justify-center text-accent">
              <Sparkles className="w-3.5 h-3.5" />
            </div>
            <div>
              <h4 className="text-xs font-semibold text-white">
                Katalog Preset Provider
              </h4>
              <p className="text-[11px] text-text-muted">
                {KNOWN_PROVIDERS.length} preset resmi siap dihubungkan dengan 1-klik
              </p>
            </div>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setIsCatalogExpanded(!isCatalogExpanded)}
            icon={isCatalogExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
          >
            {isCatalogExpanded ? 'Tutup' : 'Lihat Semua'}
          </Button>
        </div>

        {isCatalogExpanded && (
          <div className="grid grid-cols-1 sm:grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-2.5 mt-4 pt-4 border-t border-border/60">
            {KNOWN_PROVIDERS.map((preset) => (
              <button
                key={preset.id}
                onClick={() => handleSelectPreset(preset)}
                className="p-3 rounded-xl border border-border/70 bg-bg-surface hover:border-accent/60 hover:bg-bg-surface-2/80 transition-all flex flex-col items-center text-center gap-2 group shadow-sm cursor-pointer"
              >
                <div
                  className="w-9 h-9 rounded-xl flex items-center justify-center shadow-inner transition-transform group-hover:scale-105"
                  style={{ backgroundColor: preset.bgColor, border: `1px solid ${preset.borderColor}` }}
                >
                  <ProviderBrandIcon providerIdOrKind={preset.kind} name={preset.name} className="w-5 h-5" />
                </div>
                <div className="w-full">
                  <span className="font-bold text-xs text-white block truncate">{preset.displayName}</span>
                  <span className="text-[10px] text-text-muted block truncate font-mono">{preset.tag}</span>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      {/* ------------------------------------------------------------------- */}
      {/* 2. STATS SUMMARY BAR                                                */}
      {/* ------------------------------------------------------------------- */}
      <div className="grid grid-cols-1 sm:grid-cols-2 sm:grid-cols-4 gap-3">
        <div className="p-3.5 rounded-xl border border-border bg-bg-surface flex items-center justify-between">
          <div className="space-y-0.5">
            <span className="text-xs font-medium text-text-muted">Total Provider</span>
            <div className="text-xl font-bold text-white">{providers.length}</div>
          </div>
          <div className="w-8 h-8 rounded-lg bg-bg-surface-2 border border-border flex items-center justify-center text-accent">
            <Server className="w-4 h-4" />
          </div>
        </div>

        <div className="p-3.5 rounded-xl border border-border bg-bg-surface flex items-center justify-between">
          <div className="space-y-0.5">
            <span className="text-xs font-medium text-text-muted">Status Sehat</span>
            <div className="text-xl font-bold text-emerald-400">
              {healthyCount} / {providers.length}
            </div>
          </div>
          <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Activity className="w-4 h-4" />
          </div>
        </div>

        <div className="p-3.5 rounded-xl border border-border bg-bg-surface flex items-center justify-between">
          <div className="space-y-0.5">
            <span className="text-xs font-medium text-text-muted">Pool Egress</span>
            <div className="text-xl font-bold text-white">{egressPools.length}</div>
          </div>
          <div className="w-8 h-8 rounded-lg bg-blue-500/10 border border-blue-500/20 flex items-center justify-center text-blue-400">
            <Globe className="w-4 h-4" />
          </div>
        </div>

        <div className="p-3.5 rounded-xl border border-border bg-bg-surface flex items-center justify-between">
          <div className="space-y-0.5">
            <span className="text-xs font-medium text-text-muted">Enkripsi Kunci</span>
            <div className="text-xs font-bold text-accent mt-1 flex items-center gap-1">
              <Shield className="w-3.5 h-3.5" />
              <span>AES-256-GCM</span>
            </div>
          </div>
          <div className="w-8 h-8 rounded-lg bg-accent/10 border border-accent/20 flex items-center justify-center text-accent">
            <KeyRound className="w-4 h-4" />
          </div>
        </div>
      </div>

      {/* ------------------------------------------------------------------- */}
      {/* 3. DAFTAR KARTU PROVIDER (RINGKAS & BERSIH)                         */}
      {/* ------------------------------------------------------------------- */}
      {isProvidersError ? (
        <QueryError
          message={providersError instanceof Error ? providersError.message : 'Daftar provider tidak tersedia.'}
          onRetry={() => void refetchProviders()}
        />
      ) : isProvidersLoading ? (
        <Card className="p-10 text-center text-xs text-text-muted">
          <span role="status" aria-live="polite">Memuat daftar provider...</span>
        </Card>
      ) : providers.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {providers.map((p) => {
            const isHealthy = p.last_health_status === 'healthy';
            const isDegraded = p.last_health_status === 'degraded';
            const isCustom = isCustomProvider(p);
            const egress = egressPools.find((ep) => ep.id === p.egress_pool_id);

            return (
              <div
                key={p.id}
                className={`p-4 sm:p-5 rounded-2xl border transition-all flex flex-col justify-between group shadow-sm ${
                  isCustom
                    ? 'bg-[#15121e] border-purple-900/40 hover:border-purple-500/60 hover:shadow-[0_0_20px_rgba(168,85,247,0.15)]'
                    : 'bg-[#131316] border-border hover:border-accent/40 hover:shadow-[0_0_20px_rgba(202,240,69,0.1)]'
                }`}
              >
                {/* Bagian Atas: Icon, Identitas & Status Kesehatan */}
                <div className="space-y-3.5">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-center gap-3">
                      {/* Icon Provider Interaktif: Klik untuk Buka Drawer */}
                      <button
                        type="button"
                        onClick={() => handleOpenDrawer(p, 'settings')}
                        title={
                          isCustom
                            ? 'Klik icon custom provider ini untuk membuka drawer konfigurasi & kelola'
                            : 'Klik icon provider untuk membuka drawer konfigurasi'
                        }
                        className={`relative w-10 h-10 sm:w-12 sm:h-12 rounded-xl flex items-center justify-center flex-shrink-0 transition-all cursor-pointer shadow-md ${
                          isCustom
                            ? 'bg-purple-950/50 border-2 border-purple-500/40 text-purple-300 hover:scale-110 hover:border-purple-300 hover:ring-2 hover:ring-purple-400/40'
                            : 'bg-bg-surface-2 border border-border text-accent hover:scale-110 hover:border-accent hover:ring-2 hover:ring-accent/40'
                        }`}
                      >
                        <ProviderBrandIcon
                          providerIdOrKind={p.kind}
                          name={p.name}
                          className="w-5 h-5 sm:w-6 sm:h-6"
                          isCustom={isCustom}
                        />
                        <span
                          className={`absolute -bottom-1 -right-1 w-4 h-4 rounded-full bg-bg-surface border flex items-center justify-center shadow-sm ${
                            isCustom
                              ? 'border-purple-500/60 text-purple-300'
                              : 'border-border text-text-muted'
                          }`}
                        >
                          <Sliders className="w-2 h-2" />
                        </span>
                      </button>

                      <div className="space-y-0.5 truncate">
                        <div className="flex items-center gap-2">
                          <h4 className="font-bold text-white text-base truncate">
                            {p.display_name || p.name}
                          </h4>
                          {isCustom && (
                            <span className="px-1.5 py-0.2 text-[10px] font-mono rounded bg-purple-500/20 text-purple-300 border border-purple-500/30 flex-shrink-0">
                              Custom
                            </span>
                          )}
                        </div>
                        <span className="text-xs text-text-muted font-mono block truncate">
                          {p.kind}
                        </span>
                      </div>
                    </div>

                    {/* Status Pill */}
                    <div className="flex items-center gap-1.5 flex-shrink-0">
                      <span
                        className={`px-2 py-0.5 rounded-full text-[10px] font-bold border flex items-center gap-1.5 ${
                          isHealthy
                            ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
                            : 'bg-red-500/10 text-red-400 border-red-500/20'
                        }`}
                      >
                        <span
                          className={`w-1.5 h-1.5 rounded-full ${
                            isHealthy ? 'bg-emerald-400 animate-pulse' : 'bg-red-400'
                          }`}
                        />
                        <span>
                          {isHealthy
                            ? `HEALTHY ${p.last_latency_ms ? `(${p.last_latency_ms} ms)` : ''}`
                            : 'UNHEALTHY'}
                        </span>
                      </span>
                    </div>
                  </div>

                  {/* Base URL */}
                  <div className="p-2 rounded-lg bg-bg-base/70 border border-border/60 flex items-center justify-between text-xs font-mono text-text-secondary">
                    <span className="truncate pr-2">{p.base_url}</span>
                    <button
                      type="button"
                      onClick={() => void copyWithFeedback(p.base_url, 'Base URL disalin ke clipboard')}
                      title="Salin Base URL"
                      aria-label="Salin Base URL"
                      className="p-2 -mr-1 rounded-md text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors flex-shrink-0 min-w-[36px] min-h-[36px] flex items-center justify-center cursor-pointer"
                    >
                      <Copy className="w-3.5 h-3.5" />
                    </button>
                  </div>

                  {/* Jalur Proxy Egress */}
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-text-muted">Rute Proxy:</span>
                    <span className="font-mono text-[11px] text-text-secondary flex items-center gap-1">
                      <Globe className="w-3 h-3 text-accent" />
                      <span>{egress ? egress.name : 'Direct Outbound'}</span>
                    </span>
                  </div>
                </div>

                {/* Bagian Bawah: Tombol Buka Drawer & Probe Latensi */}
                <div className="mt-4 pt-3 sm:mt-5 sm:pt-3.5 border-t border-border/60 flex items-center justify-between gap-2">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleProbe(p)}
                    isLoading={probingId === p.id}
                    icon={<Activity className="w-3.5 h-3.5" />}
                    title="Uji kesehatan dan latensi provider sekarang"
                  >
                    Probe
                  </Button>

                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => handleOpenDrawer(p, 'models')}
                    icon={<Sliders className="w-3.5 h-3.5" />}
                    title="Buka konfigurasi, model, kredensial, dan setelan provider"
                  >
                    Kelola
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <Card className="p-10 text-center space-y-4">
          <Server className="w-12 h-12 text-text-muted mx-auto" />
          <h3 className="text-lg font-bold text-white">Belum Ada Upstream Provider</h3>
          <p className="text-xs text-text-secondary max-w-md mx-auto leading-relaxed">
            Hubungkan provider AI pertama Anda dari Katalog Preset resmi di atas, atau daftarkan server AI custom / proxy lokal secara mandiri.
          </p>
          <Button variant="primary" size="md" onClick={handleOpenCustomCreate} icon={<Plus className="w-4 h-4" />}>
            Daftarkan Provider Sekarang
          </Button>
        </Card>
      )}

      {/* ------------------------------------------------------------------- */}
      {/* 4. PROVIDER CONFIGURATION DRAWER (SLIDE-OVER DARI SISI KANAN)       */}
      {/* ------------------------------------------------------------------- */}
      <Drawer
        isOpen={isDrawerOpen && !!selectedProvider}
        onClose={() => setIsDrawerOpen(false)}
        maxWidth="2xl"
        footer={
          <div className="flex items-center justify-between w-full">
            <span className="text-xs text-text-muted truncate max-w-[200px]">
              {selectedProvider?.display_name || selectedProvider?.name}
            </span>
            <Button variant="ghost" onClick={() => setIsDrawerOpen(false)}>
              Tutup
            </Button>
          </div>
        }
        title={
          <div className="flex items-center gap-2 min-w-0">
            {/* Icon Provider di Drawer Header: Klik untuk trigger probe */}
            <button
              type="button"
              onClick={() => selectedProvider && handleProbe(selectedProvider)}
              title="Klik icon untuk memeriksa status kesehatan provider"
              className={`w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 transition-all cursor-pointer shadow-sm ${
                isManualProvider
                  ? 'bg-purple-950/50 border border-purple-500/50 text-purple-300 hover:scale-105 hover:border-purple-300'
                  : 'bg-bg-surface border border-border text-accent hover:scale-105 hover:border-accent'
              }`}
            >
              {selectedProvider && (
                <ProviderBrandIcon
                  providerIdOrKind={selectedProvider.kind}
                  name={selectedProvider.name}
                  className="w-5 h-5"
                  isCustom={isManualProvider}
                />
              )}
            </button>
            <div className="truncate min-w-0">
              <div className="flex items-center gap-1.5 min-w-0">
                <span className="truncate font-bold text-white text-sm">
                  {selectedProvider?.display_name || selectedProvider?.name}
                </span>
                {isManualProvider && (
                  <span className="px-1.5 py-0.2 text-[9px] font-mono rounded bg-purple-500/20 text-purple-300 border border-purple-500/30 flex-shrink-0">
                    Custom Provider
                  </span>
                )}
              </div>
              <span className="text-[11px] font-mono text-text-muted block truncate font-normal">
                {selectedProvider?.kind}
              </span>
            </div>
          </div>
        }
        subtitle={
          selectedProvider && (
            <div className="flex items-center gap-1.5 min-w-0">
              <span className="font-mono text-[11px] text-text-secondary truncate flex-1 min-w-0">
                {selectedProvider.base_url}
              </span>
              <button
                type="button"
                onClick={() => void copyWithFeedback(selectedProvider.base_url, 'Base URL disalin')}
                className="p-1 rounded text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors flex-shrink-0 cursor-pointer"
                title="Salin Base URL"
                aria-label="Salin Base URL"
              >
                <Copy className="w-3 h-3" />
              </button>
            </div>
          )
        }
        headerExtra={
          selectedProvider && (
            <div className="flex items-center gap-1.5 flex-wrap">
              <span
                className={`px-2 py-0.5 rounded-full text-[10px] font-bold border flex items-center gap-1.5 flex-shrink-0 ${
                  selectedProvider.last_health_status === 'healthy'
                    ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
                    : 'bg-red-500/10 text-red-400 border-red-500/20'
                }`}
              >
                <span
                  className={`w-1.5 h-1.5 rounded-full ${
                    selectedProvider.last_health_status === 'healthy'
                      ? 'bg-emerald-400 animate-pulse'
                      : 'bg-red-400'
                  }`}
                />
                <span>
                  {selectedProvider.last_health_status === 'healthy'
                    ? `HEALTHY ${selectedProvider.last_latency_ms ? `(${selectedProvider.last_latency_ms} ms)` : ''}`
                    : 'UNHEALTHY'}
                </span>
              </span>
              <span className="text-[10px] text-text-muted font-mono">
                {models.length} Model • {credentials.length} Key
              </span>
            </div>
          )
        }
      >
        {selectedProvider && (
          <div className="space-y-4 sm:space-y-6">
            {providerDetailsError && (
              <QueryError
                message={providerDetailsError}
                onRetry={() => void loadProviderDetails(selectedProvider.id)}
              />
            )}
            {isPoolsError && (
              <div role="alert" className="text-xs text-amber-300 border border-amber-500/30 bg-amber-500/10 rounded-lg p-3">
                Daftar egress gagal dimuat: {poolsError instanceof Error ? poolsError.message : 'galat tidak diketahui'}
              </div>
            )}
            {/* Navigasi Tab di Dalam Drawer */}
            <div className="grid grid-cols-3 gap-1 p-1 bg-bg-surface-2/80 rounded-xl border border-border">
              <button
                type="button"
                onClick={() => setDrawerTab('models')}
                className={`flex items-center justify-center gap-1 sm:gap-1.5 py-1.5 sm:py-2 px-1 rounded-lg text-[11px] sm:text-xs font-semibold transition-all cursor-pointer ${
                  drawerTab === 'models'
                    ? 'bg-accent text-black font-bold shadow'
                    : 'text-text-secondary hover:text-white hover:bg-bg-surface/50'
                }`}
              >
                <Bot className="w-3.5 h-3.5 flex-shrink-0" />
                <span className="truncate">
                  <span className="sm:inline hidden">Model Upstream</span>
                  <span className="sm:hidden inline">Model</span>
                </span>
                <span
                  className={`px-1.5 py-0.2 rounded-full text-[10px] font-mono flex-shrink-0 ${
                    drawerTab === 'models' ? 'bg-black/20 text-black' : 'bg-bg-base text-text-muted'
                  }`}
                >
                  {models.length}
                </span>
              </button>

              <button
                type="button"
                onClick={() => setDrawerTab('credentials')}
                className={`flex items-center justify-center gap-1 sm:gap-1.5 py-1.5 sm:py-2 px-1 rounded-lg text-[11px] sm:text-xs font-semibold transition-all cursor-pointer ${
                  drawerTab === 'credentials'
                    ? 'bg-accent text-black font-bold shadow'
                    : 'text-text-secondary hover:text-white hover:bg-bg-surface/50'
                }`}
              >
                <KeyRound className="w-3.5 h-3.5 flex-shrink-0" />
                <span className="truncate">
                  <span className="sm:inline hidden">API Keys</span>
                  <span className="sm:hidden inline">Keys</span>
                </span>
                <span
                  className={`px-1.5 py-0.2 rounded-full text-[10px] font-mono flex-shrink-0 ${
                    drawerTab === 'credentials' ? 'bg-black/20 text-black' : 'bg-bg-base text-text-muted'
                  }`}
                >
                  {credentials.length}
                </span>
              </button>

              <button
                type="button"
                onClick={() => setDrawerTab('settings')}
                className={`flex items-center justify-center gap-1 sm:gap-1.5 py-1.5 sm:py-2 px-1 rounded-lg text-[11px] sm:text-xs font-semibold transition-all cursor-pointer ${
                  drawerTab === 'settings'
                    ? 'bg-accent text-black font-bold shadow'
                    : 'text-text-secondary hover:text-white hover:bg-bg-surface/50'
                }`}
              >
                <Sliders className="w-3.5 h-3.5 flex-shrink-0" />
                <span className="truncate">
                  <span className="sm:inline hidden">Pengaturan & Proxy</span>
                  <span className="sm:hidden inline">Setelan</span>
                </span>
              </button>
            </div>

            {/* TAB 1: MODEL UPSTREAM & DIAGNOSTIK */}
            {drawerTab === 'models' && (
              <ModelsTabChild
                models={models}
                selectedProvider={selectedProvider}
                syncingId={syncingId}
                testingModelId={testingModelId}
                modelTestResults={modelTestResults}
                handleSyncModels={handleSyncModels}
                handleTestModel={handleTestModel}
                handleDeleteModel={handleDeleteModel}
                onOpenAddModelModal={() => setIsAddModelModalOpen(true)}
                copyWithFeedback={copyWithFeedback}
              />
            )}

            {/* TAB 2: KREDENSIAL API KEY */}
            {drawerTab === 'credentials' && (
              <CredentialsTabChild
                selectedProvider={selectedProvider}
                credentials={credentials}
                handleToggleKey={handleToggleKey}
                handleDeleteKey={handleDeleteKey}
                loadProviderDetails={loadProviderDetails}
                toast={toast}
                api={api}
                copyWithFeedback={copyWithFeedback}
              />
            )}

            {/* TAB 3: PENGATURAN TEKNIS & JALUR PROXY */}
            {drawerTab === 'settings' && (
              <SettingsTabChild
                selectedProvider={selectedProvider}
                isManualProvider={isManualProvider}
                egressPools={egressPools}
                handleSelectProxyPreset={handleSelectProxyPreset}
                handleDeleteProvider={handleDeleteProvider}
                loadData={loadData}
                handleOpenDrawer={handleOpenDrawer}
                toast={toast}
                api={api}
              />
            )}
          </div>
        )}
      </Drawer>

      {/* ------------------------------------------------------------------- */}
      {/* 5. MODAL CREATE PROVIDER BARU / PRESET */}
      <CreateProviderModalChild
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        selectedPreset={selectedPreset}
        providers={providers}
        egressPools={egressPools}
        loadData={loadData}
        handleOpenDrawer={handleOpenDrawer}
        toast={toast}
        api={api}
      />

      {/* ------------------------------------------------------------------- */}
      <AddModelModalChild
        isOpen={isAddModelModalOpen}
        onClose={() => setIsAddModelModalOpen(false)}
        selectedProvider={selectedProvider}
        loadProviderDetails={loadProviderDetails}
        toast={toast}
        api={api}
      />
    </div>
  );
};

export const ModelsTabChild: React.FC<any> = ({
  models, selectedProvider, syncingId, testingModelId, modelTestResults,
  handleSyncModels, handleTestModel, handleDeleteModel, onOpenAddModelModal, copyWithFeedback
}) => {
  const [modelSearch, setModelSearch] = useState('');
  const [copiedModelId, setCopiedModelId] = useState<string | null>(null);
  const copyTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    return () => {
      copyTimersRef.current.forEach((t) => clearTimeout(t));
      copyTimersRef.current = [];
    };
  }, []);

  const getModelBadges = (modelName: string, providerKind: string) => {
    const lower = modelName.toLowerCase();
    let brand = 'Upstream';
    let context = 'Standard';
    if (lower.includes('claude')) { brand = 'Anthropic'; context = '200K'; }
    else if (lower.includes('gpt-4') || lower.includes('o1') || lower.includes('o3')) { brand = 'OpenAI'; context = '128K'; }
    else if (lower.includes('deepseek')) { brand = 'DeepSeek'; context = lower.includes('r1') ? '128K' : '64K'; }
    else if (lower.includes('gemini')) { brand = 'Google'; context = '1M'; }
    else if (lower.includes('llama')) { brand = 'Meta'; context = '128K'; }
    else if (lower.includes('mistral') || lower.includes('codestral')) { brand = 'Mistral'; context = '32K'; }
    else if (providerKind === 'ollama') { brand = 'Local'; context = 'Local Host'; }
    return { brand, context };
  };

  const filteredModels = useMemo(() => {
    if (!modelSearch.trim()) return models;
    const term = modelSearch.toLowerCase();
    return models.filter((m: any) => m.upstream_model_name.toLowerCase().includes(term));
  }, [models, modelSearch]);

  return (
    <div className="space-y-2.5 sm:space-y-3">
      <div className="flex items-center gap-1.5 sm:gap-2">
        <div className="relative flex-1 min-w-0">
          <Search className="w-3.5 h-3.5 text-text-muted absolute left-2.5 top-2" />
          <input
            type="text"
            placeholder="Cari model..."
            value={modelSearch}
            onChange={(e) => setModelSearch(e.target.value)}
            className="w-full pl-8 pr-2 py-1.5 text-xs bg-bg-surface-2 border border-border rounded-lg text-white font-mono placeholder:text-text-muted outline-none focus:border-accent"
          />
        </div>
        <button
          type="button"
          onClick={() => handleSyncModels(selectedProvider)}
          disabled={syncingId === selectedProvider.id}
          title="Tarik daftar model dari upstream"
          className="p-1.5 sm:p-2 rounded-lg bg-bg-surface-2 text-text-primary border border-border hover:bg-border/60 hover:text-white transition-colors cursor-pointer disabled:opacity-50 flex-shrink-0"
        >
          {syncingId === selectedProvider.id ? <RefreshCw className="w-4 h-4 animate-spin" /> : <DownloadCloud className="w-4 h-4" />}
        </button>
        <button
          type="button"
          onClick={onOpenAddModelModal}
          title="Tambah model manual"
          className="p-1.5 sm:p-2 rounded-lg bg-accent text-black hover:bg-accent-hover transition-colors cursor-pointer flex-shrink-0"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>

      {filteredModels.length > 0 ? (
        <div className="space-y-2.5 min-h-0 max-h-[45dvh] sm:max-h-[40vh] overflow-y-auto overscroll-contain pr-0.5 scrollbar-thin">
          {filteredModels.map((model: any) => {
            const { brand, context } = getModelBadges(model.upstream_model_name, selectedProvider.kind);
            const isTesting = testingModelId === model.id;
            const testResult = modelTestResults[model.id];

            return (
              <div key={model.id} className="px-2.5 py-2 sm:p-3 rounded-xl border border-border bg-bg-surface-2/40 hover:border-border/80 transition-all space-y-1.5">
                <div className="flex items-center gap-1.5 min-w-0">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 flex-shrink-0" />
                  <span className="font-mono text-xs sm:text-sm font-bold text-white truncate min-w-0 flex-1">{model.upstream_model_name}</span>
                  <span className="hidden md:inline px-1.5 py-0.2 text-[10px] rounded bg-purple-500/10 text-purple-300 border border-purple-500/20 font-mono flex-shrink-0">{brand}</span>
                  <button
                    type="button"
                    onClick={() => void copyWithFeedback(model.upstream_model_name, `Model ID "${model.upstream_model_name}" disalin`, () => {
                      setCopiedModelId(model.id);
                      copyTimersRef.current.push(setTimeout(() => setCopiedModelId(null), 2000));
                    })}
                    className="p-1.5 rounded-md text-text-muted hover:text-white hover:bg-bg-surface transition-colors cursor-pointer flex-shrink-0"
                  >
                    {copiedModelId === model.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                  <button
                    type="button"
                    onClick={() => handleTestModel(model)}
                    disabled={isTesting}
                    className="p-1.5 rounded-md text-text-muted hover:text-white hover:bg-bg-surface transition-colors cursor-pointer disabled:opacity-50 flex-shrink-0"
                  >
                    {isTesting ? <RefreshCw className="w-3.5 h-3.5 animate-spin text-accent" /> : <FlaskConical className="w-3.5 h-3.5 text-accent" />}
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDeleteModel(model)}
                    className="p-1.5 rounded-md text-text-muted hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer flex-shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>

                {testResult && (
                  <div className={`px-2 py-1.5 rounded-lg border text-[11px] flex items-center justify-between gap-2 animate-in fade-in duration-150 ${testResult.ok ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300' : 'bg-red-500/10 border-red-500/30 text-red-300'}`}>
                    <span className="flex items-center gap-1.5 min-w-0 font-bold truncate">
                      {testResult.ok ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" /> : <AlertTriangle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" />}
                      <span className="truncate">{testResult.ok ? 'Uji ok' : 'Uji gagal'}{testResult.latency_ms > 0 && ` · ${testResult.latency_ms} ms`}</span>
                    </span>
                    <span className="opacity-70 font-mono flex-shrink-0 text-[10px]">{testResult.timestamp}</span>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="p-5 rounded-xl border border-dashed border-border text-center space-y-2">
          <Bot className="w-8 h-8 text-text-muted mx-auto" />
          <p className="text-xs text-text-secondary">{modelSearch ? 'Tidak ada model yang cocok dengan kata kunci.' : 'Belum ada model upstream yang terhubung.'}</p>
          <div className="pt-2 flex justify-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => handleSyncModels(selectedProvider)} icon={<DownloadCloud className="w-3.5 h-3.5" />}>Tarik Model Upstream</Button>
            <Button variant="primary" size="sm" onClick={onOpenAddModelModal} icon={<Plus className="w-3.5 h-3.5" />}>Tambah Manual</Button>
          </div>
        </div>
      )}
    </div>
  );
};


export const CredentialsTabChild: React.FC<any> = ({
  selectedProvider, credentials, handleToggleKey, handleDeleteKey, loadProviderDetails, toast, api, copyWithFeedback
}) => {
  const [isAddingKeyInline, setIsAddingKeyInline] = useState(false);
  const [inlineAuthTab, setInlineAuthTab] = useState<'apikey' | 'authlogin'>('apikey');
  const [newKeyForm, setNewKeyForm] = useState({ label: 'Primary API Key', api_key: '' });
  const [authFallbackInput, setAuthFallbackInput] = useState('');
  const [authExtractedToken, setAuthExtractedToken] = useState('');
  const [authExtractionHint, setAuthExtractionHint] = useState('');
  const [showNewKeySecret, setShowNewKeySecret] = useState(false);
  const [isSavingKey, setIsSavingKey] = useState(false);
  const [copiedTokenId, setCopiedTokenId] = useState<string | null>(null);
  const copyTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    return () => {
      copyTimersRef.current.forEach((t) => clearTimeout(t));
      copyTimersRef.current = [];
    };
  }, []);

  const matchedPreset = useMemo(() => {
    if (!selectedProvider) return null;
    return KNOWN_PROVIDERS.find((p) => p.kind === selectedProvider.kind || p.name === selectedProvider.name || p.id === selectedProvider.name.split('-')[0]) || null;
  }, [selectedProvider]);

  const handleInlineFallbackChange = (val: string) => {
    setAuthFallbackInput(val);
    const res = extractAuthTokenFromInput(val);
    setAuthExtractedToken(res.token);
    setAuthExtractionHint(res.cleanHint || '');
    if (res.token) {
      setNewKeyForm((prev) => ({ ...prev, api_key: res.token }));
    }
  };

  const resetInlineForm = () => {
    setIsAddingKeyInline(false);
    setInlineAuthTab('apikey');
    setAuthFallbackInput('');
    setAuthExtractedToken('');
    setAuthExtractionHint('');
    setNewKeyForm({
      label: (credentials || []).length === 0 ? 'Primary API Key' : 'Backup API Key',
      api_key: '',
    });
  };

  const handleCreateKey = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;
    const keyLabel = newKeyForm.label.trim() || (inlineAuthTab === 'authlogin' ? 'Auth Login Credential' : 'Primary API Key');
    const keySecret = inlineAuthTab === 'apikey'
      ? newKeyForm.api_key.trim()
      : (authExtractedToken.trim() || authFallbackInput.trim());

    if (!keySecret) {
      toast.error('Secret token atau kredensial API key wajib diisi');
      return;
    }

    setIsSavingKey(true);
    try {
      await api.credentials.create(selectedProvider.id, { label: keyLabel, api_key: keySecret });
      toast.success('Kredensial berhasil ditambahkan dengan enkripsi AES-256-GCM');
      resetInlineForm();
      await loadProviderDetails(selectedProvider.id);
    } catch (err: any) {
      toast.error('Gagal menambahkan kredensial: ' + (err.message || err));
    } finally {
      setIsSavingKey(false);
    }
  };

  return (
    <div className="space-y-2.5 sm:space-y-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-white flex items-center gap-1.5 min-w-0" title="Token otentikasi disimpan dengan enkripsi amplop AES-256-GCM">
          <KeyRound className="w-3.5 h-3.5 text-accent flex-shrink-0" />
          <span className="truncate">Kredensial · AES-256-GCM</span>
        </span>
        {!isAddingKeyInline && (
          <button type="button" onClick={() => setIsAddingKeyInline(true)} className="p-1.5 rounded-lg bg-accent text-black hover:bg-accent-hover transition-colors flex-shrink-0">
            <Plus className="w-4 h-4" />
          </button>
        )}
      </div>

      {isAddingKeyInline && (
        <div className="p-3 rounded-xl border border-accent/40 bg-accent/5 space-y-3">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-semibold text-white">Tambah Kredensial Baru</span>
            <button type="button" onClick={resetInlineForm} className="text-[11px] text-text-muted hover:text-white">Batal</button>
          </div>

          {/* Mode Switcher Tabs */}
          <div className="flex border-b border-border/60 gap-3 text-xs">
            <button
              type="button"
              onClick={() => setInlineAuthTab('apikey')}
              className={`pb-1.5 font-medium transition-colors flex items-center gap-1 ${
                inlineAuthTab === 'apikey'
                  ? 'border-b-2 border-accent text-accent font-semibold'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <KeyRound className="w-3 h-3" /> Input API Key Manual
            </button>
            <button
              type="button"
              onClick={() => setInlineAuthTab('authlogin')}
              className={`pb-1.5 font-medium transition-colors flex items-center gap-1 ${
                inlineAuthTab === 'authlogin'
                  ? 'border-b-2 border-accent text-accent font-semibold'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <ArrowUpRight className="w-3 h-3" /> Auth Login / Salin Redirect URL
            </button>
          </div>

          <form onSubmit={handleCreateKey} className="space-y-2.5 text-xs">
            <div>
              <label className="block text-[11px] font-medium text-text-secondary mb-1">Label Kredensial</label>
              <input
                type="text"
                required
                placeholder="mis. Primary API Key atau Claude OAuth Token"
                value={newKeyForm.label}
                onChange={(e) => setNewKeyForm({ ...newKeyForm, label: e.target.value })}
                className="w-full px-2.5 py-1.5 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs placeholder:text-text-muted outline-none focus:border-accent"
              />
            </div>

            {inlineAuthTab === 'apikey' ? (
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-[11px] font-medium text-text-secondary">Secret API Key *</label>
                  {matchedPreset?.apiKeyHelp && (
                    <span className="text-[10px] text-accent font-mono">{matchedPreset.apiKeyHelp}</span>
                  )}
                </div>
                <div className="relative">
                  <input
                    type={showNewKeySecret ? 'text' : 'password'}
                    required
                    placeholder={matchedPreset?.apiKeyPlaceholder || 'sk-... atau Secret Key'}
                    value={newKeyForm.api_key}
                    onChange={(e) => setNewKeyForm({ ...newKeyForm, api_key: e.target.value })}
                    className="w-full px-2.5 py-1.5 pr-9 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs placeholder:text-text-muted outline-none focus:border-accent"
                  />
                  <button type="button" onClick={() => setShowNewKeySecret(!showNewKeySecret)} className="absolute right-2 top-2 text-text-muted hover:text-white">
                    {showNewKeySecret ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                  </button>
                </div>
              </div>
            ) : (
              <div className="space-y-2 pt-0.5">
                {matchedPreset?.authLoginUrl && (
                  <div className="flex items-center justify-between p-2 rounded-lg bg-bg-surface/80 border border-border/60">
                    <span className="text-[11px] text-text-secondary truncate">{matchedPreset.authLoginLabel || 'Buka Otorisasi Resmi'}</span>
                    <a
                      href={matchedPreset.authLoginUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="px-2 py-1 rounded bg-accent text-black font-semibold text-[11px] flex items-center gap-1 hover:opacity-90 flex-shrink-0"
                    >
                      Buka Portal <ExternalLink className="w-3 h-3" />
                    </a>
                  </div>
                )}
                <p className="text-[11px] text-text-muted">
                  {matchedPreset?.authInstructions || 'Salin URL redirect / callback atau token yang didapatkan dari portal login dan tempelkan ke bawah.'}
                </p>
                <div>
                  <textarea
                    rows={2}
                    placeholder="Tempel seluruh URL redirect (misal: http://.../callback?code=xxx) atau token di sini..."
                    value={authFallbackInput}
                    onChange={(e) => handleInlineFallbackChange(e.target.value)}
                    className="w-full px-2.5 py-1.5 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs placeholder:text-text-muted outline-none focus:border-accent"
                  />
                  {authExtractedToken && (
                    <div className="mt-1.5 p-1.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                      <Check className="w-3.5 h-3.5 flex-shrink-0" />
                      <span className="truncate">
                        Token Terdeteksi ({authExtractionHint}):{' '}
                        <strong className="text-white">
                          {authExtractedToken.length > 20 ? `${authExtractedToken.slice(0, 8)}...${authExtractedToken.slice(-6)}` : authExtractedToken}
                        </strong>
                      </span>
                    </div>
                  )}
                </div>
              </div>
            )}

            <div className="flex items-center justify-between pt-1">
              <span className="text-[10px] text-text-muted flex items-center gap-1" title="Terenkripsi AES-256-GCM">
                <Lock className="w-3 h-3 text-accent flex-shrink-0" /> AES-256-GCM
              </span>
              <div className="flex gap-1.5">
                <button type="button" onClick={resetInlineForm} className="px-2.5 py-1.5 text-xs rounded-lg bg-bg-surface-2 text-text-primary border border-border hover:text-white transition-colors">Batal</button>
                <button type="submit" disabled={isSavingKey} className="px-2.5 py-1.5 text-xs rounded-lg bg-accent text-black font-bold hover:bg-accent-hover transition-colors disabled:opacity-50 flex items-center gap-1">
                  {isSavingKey ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Check className="w-3.5 h-3.5" />}
                  Simpan Kredensial
                </button>
              </div>
            </div>
          </form>
        </div>
      )}

      {credentials.length > 0 ? (
        <div className="space-y-2">
          {credentials.map((cred: any) => (
            <div key={cred.id} className="px-2.5 py-2 rounded-xl border border-border bg-bg-surface-2/40 flex items-center justify-between gap-2">
              <div className="flex items-center gap-1.5 min-w-0 flex-1">
                <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${cred.enabled ? 'bg-emerald-400' : 'bg-zinc-400'}`} />
                <span className="font-bold text-white text-xs truncate">{cred.label}</span>
                <span className="text-[11px] text-text-muted font-mono truncate hidden sm:inline">{cred.masked_hint || 'sk-****'}</span>
                <button type="button" onClick={() => void copyWithFeedback(cred.masked_hint || '', 'Masked key disalin', () => { setCopiedTokenId(cred.id); copyTimersRef.current.push(setTimeout(() => setCopiedTokenId(null), 2000)); })} className="text-text-muted hover:text-white flex-shrink-0">
                  {copiedTokenId === cred.id ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                </button>
              </div>
              <div className="flex items-center gap-0.5 flex-shrink-0">
                <button type="button" onClick={() => handleToggleKey(cred)} className="p-1.5 rounded-md text-text-muted hover:text-white hover:bg-bg-surface transition-colors cursor-pointer">
                  {cred.enabled ? <Power className="w-3.5 h-3.5 text-amber-400" /> : <Check className="w-3.5 h-3.5 text-emerald-400" />}
                </button>
                <button type="button" onClick={() => handleDeleteKey(cred)} className="p-1.5 rounded-md text-text-muted hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer">
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="p-5 rounded-xl border border-dashed border-border text-center space-y-2">
          <KeyRound className="w-8 h-8 text-text-muted mx-auto" />
          <p className="text-xs text-text-secondary">Belum ada kredensial API key untuk provider ini.</p>
          <Button variant="primary" size="sm" onClick={() => setIsAddingKeyInline(true)} icon={<Plus className="w-3.5 h-3.5" />}>Tambah API Key Pertama</Button>
        </div>
      )}
    </div>
  );
};


export const SettingsTabChild: React.FC<any> = ({
  selectedProvider, isManualProvider, egressPools, handleSelectProxyPreset, handleDeleteProvider, loadData, handleOpenDrawer, toast, api
}) => {
  const [configForm, setConfigForm] = useState({
    display_name: selectedProvider?.display_name || '',
    base_url: selectedProvider?.base_url || '',
    priority: selectedProvider?.priority || 100,
    weight: selectedProvider?.weight || 100,
  });
  const [isSavingConfig, setIsSavingConfig] = useState(false);

  useEffect(() => {
    if (selectedProvider) {
      setConfigForm({
        display_name: selectedProvider.display_name || '',
        base_url: selectedProvider.base_url || '',
        priority: selectedProvider.priority || 100,
        weight: selectedProvider.weight || 100,
      });
    }
  }, [selectedProvider]);

  const handleSaveConfig = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;

    setIsSavingConfig(true);
    try {
      await api.providers.update(selectedProvider.id, configForm);
      toast.success('Konfigurasi teknis berhasil diperbarui');
      await loadData();
      handleOpenDrawer(
        { ...selectedProvider, ...configForm } as any,
        'settings'
      );
    } catch (err: any) {
      toast.error('Gagal menyimpan konfigurasi: ' + (err.message || err));
    } finally {
      setIsSavingConfig(false);
    }
  };

  return (
    <div className="space-y-4">
      {isManualProvider && (
        <form onSubmit={handleSaveConfig} className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Nama tampilan provider di antarmuka">Nama Tampilan</label>
              <input type="text" value={configForm.display_name} onChange={(e) => setConfigForm({ ...configForm, display_name: e.target.value })} className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white outline-none focus:border-accent" />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5" title="URL endpoint API upstream">Base URL Endpoint</label>
              <input type="text" value={configForm.base_url} onChange={(e) => setConfigForm({ ...configForm, base_url: e.target.value })} className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white font-mono outline-none focus:border-accent" />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Semakin kecil angka, semakin tinggi prioritas pemilihan rute">Prioritas Routing</label>
              <input type="number" value={configForm.priority} onChange={(e) => setConfigForm({ ...configForm, priority: parseInt(e.target.value) || 100 })} className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white font-mono outline-none focus:border-accent" />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Bobot load-balancing antar provider (semakin besar semakin sering dipanggil)">Bobot Load Balance</label>
              <input type="number" value={configForm.weight} onChange={(e) => setConfigForm({ ...configForm, weight: parseInt(e.target.value) || 100 })} className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white font-mono outline-none focus:border-accent" />
            </div>
          </div>
          <div className="flex justify-end pt-1">
            <Button type="submit" variant="primary" size="sm" isLoading={isSavingConfig} icon={<Check className="w-3.5 h-3.5" />}>Simpan Konfigurasi</Button>
          </div>
        </form>
      )}
      
      <div>
        <label className="block text-xs font-medium text-text-secondary mb-2">Jalur Egress Outbound</label>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
          <div onClick={() => handleSelectProxyPreset(null, 'Direct Outbound (Tanpa Proxy)')} title="Koneksi langsung dari server tanpa menggunakan proxy" className={`px-2.5 py-2 rounded-lg border cursor-pointer transition-all flex items-center justify-between gap-2 ${!selectedProvider?.egress_pool_id ? 'border-accent bg-accent/10 shadow-sm' : 'border-border bg-bg-surface-2/40 hover:border-border/80'}`}>
            <span className="flex items-center gap-1.5 min-w-0"><Globe className="w-3.5 h-3.5 text-accent flex-shrink-0" /><span className="font-bold text-white truncate">Direct (Tanpa Proxy)</span></span>
            {!selectedProvider?.egress_pool_id && <span className="px-1.5 py-0.2 text-[10px] rounded bg-accent text-black font-bold flex-shrink-0">Aktif</span>}
          </div>
          {egressPools.map((pool: any) => {
            const isSelected = selectedProvider?.egress_pool_id === pool.id;
            return (
              <div key={pool.id} onClick={() => handleSelectProxyPreset(pool.id, pool.name)} title={`${pool.name} · ${pool.kind} · Region: ${pool.region || 'Default'}`} className={`px-2.5 py-2 rounded-lg border cursor-pointer transition-all flex items-center justify-between gap-2 ${isSelected ? 'border-accent bg-accent/10 shadow-sm' : 'border-border bg-bg-surface-2/40 hover:border-border/80'}`}>
                <span className="flex items-center gap-1.5 min-w-0"><Globe className="w-3.5 h-3.5 text-accent flex-shrink-0" /><span className="font-bold text-white truncate">{pool.name}</span></span>
                {isSelected && <span className="px-1.5 py-0.2 text-[10px] rounded bg-accent text-black font-bold flex-shrink-0">Aktif</span>}
              </div>
            );
          })}
        </div>
      </div>

      <div className="pt-3 border-t border-border">
        <div className="px-2.5 py-2 rounded-lg border border-red-500/30 bg-red-500/10 flex items-center justify-between gap-2">
          <span className="text-[11px] text-red-400 font-bold flex items-center gap-1.5 min-w-0" title="Menghapus provider ini beserta seluruh kredensial API key dan pemetaan model upstream secara permanen"><AlertTriangle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" /><span className="truncate">Hapus · permanen, ikut key + model</span></span>
          <button type="button" onClick={() => handleDeleteProvider(selectedProvider)} title="Hapus provider beserta key dan model secara permanen" aria-label="Hapus provider" className="p-1.5 rounded-md bg-red-500/15 text-red-400 border border-red-500/30 hover:bg-red-500/25 transition-colors cursor-pointer flex-shrink-0"><Trash2 className="w-3.5 h-3.5" /></button>
        </div>
      </div>
    </div>
  );
};


export const CreateProviderModalChild: React.FC<any> = ({
  isOpen,
  onClose,
  selectedPreset,
  providers = [],
  egressPools = [],
  loadData,
  toast,
  handleOpenDrawer,
  api,
}) => {
  const [authTab, setAuthTab] = useState<'authlogin' | 'apikey'>('apikey');
  const [quickApiKey, setQuickApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [authFallbackInput, setAuthFallbackInput] = useState('');
  const [authExtractedToken, setAuthExtractedToken] = useState('');
  const [authExtractionHint, setAuthExtractionHint] = useState('');
  const [socketPingStatus, setSocketPingStatus] = useState<{ testing: boolean; latency?: number; ok?: boolean } | null>(null);
  const [selectedEgressPoolId, setSelectedEgressPoolId] = useState('');
  const [syncAfterSave, setSyncAfterSave] = useState(true);
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
    egress_pool_id: undefined as string | undefined,
  });

  useEffect(() => {
    if (isOpen) {
      setQuickApiKey('');
      setAuthFallbackInput('');
      setAuthExtractedToken('');
      setAuthExtractionHint('');
      setShowApiKey(false);
      setSelectedEgressPoolId('');
      setSocketPingStatus(null);

      if (selectedPreset) {
        const existingCount = (providers || []).filter(
          (p: any) => p.kind === selectedPreset.kind || p.name === selectedPreset.name || p.name.startsWith(selectedPreset.name + '-')
        ).length;
        const uniqueName = existingCount > 0 ? `${selectedPreset.name}-${existingCount + 1}` : selectedPreset.name;

        setSyncAfterSave(true);
        setAuthTab(selectedPreset.authLoginType ? 'authlogin' : 'apikey');
        setNewProv({
          name: uniqueName,
          display_name: selectedPreset.displayName,
          kind: selectedPreset.kind,
          base_url: selectedPreset.baseUrl,
          priority: selectedPreset.defaultPriority,
          weight: selectedPreset.defaultWeight,
          timeout_ms: 30000,
          egress_pool_id: undefined,
        });
      } else {
        setSyncAfterSave(false);
        setAuthTab('apikey');
        setNewProv({
          name: 'custom-provider',
          display_name: 'Custom Provider',
          kind: 'openai_compatible',
          base_url: 'https://api.openai-proxy.local/v1',
          priority: 100,
          weight: 100,
          timeout_ms: 30000,
          egress_pool_id: undefined,
        });
      }
    }
  }, [isOpen, selectedPreset, providers]);

  const handleFallbackInputChange = (val: string) => {
    setAuthFallbackInput(val);
    const res = extractAuthTokenFromInput(val);
    setAuthExtractedToken(res.token);
    setAuthExtractionHint(res.cleanHint || '');
  };

  const handleTestSocket = async () => {
    setSocketPingStatus({ testing: true });
    const start = performance.now();
    try {
      const targetUrl = (newProv.base_url || '').replace(/\/+$/, '');
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 4000);

      const res = await fetch(`${targetUrl}/models`, { method: 'GET', signal: controller.signal })
        .catch(() => fetch(`${targetUrl}/api/tags`, { method: 'GET', signal: controller.signal }))
        .catch(() => fetch(`${targetUrl}/v1/models`, { method: 'GET', signal: controller.signal }));

      clearTimeout(timeoutId);
      const latency = Math.round(performance.now() - start);
      setSocketPingStatus({ testing: false, latency, ok: Boolean(res && res.ok) });
    } catch {
      setSocketPingStatus({ testing: false, latency: 0, ok: false });
    }
  };

  const effectiveApiKey = authTab === 'apikey'
    ? quickApiKey.trim()
    : (authExtractedToken.trim() || authFallbackInput.trim());

  const handleCreateSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);
    setSavingStep('Mendaftarkan provider ke PostgreSQL...');

    try {
      const created = await api.providers.create({
        ...newProv,
        name: newProv.name.trim().toLowerCase().replace(/[^a-z0-9_-]/g, '-'),
        display_name: newProv.display_name.trim(),
        base_url: newProv.base_url.trim(),
        egress_pool_id: selectedEgressPoolId || undefined,
      });

      if (effectiveApiKey) {
        setSavingStep('Menyimpan dan mengenkripsi Kredensial (AES-256-GCM)...');
        try {
          await api.credentials.create(created.id, {
            label: authTab === 'authlogin' ? 'Auth Login Credential' : 'Primary API Key',
            api_key: effectiveApiKey,
          });
        } catch (keyErr: any) {
          toast.warn('Provider dibuat, namun kredensial gagal disimpan: ' + (keyErr.message || keyErr));
        }
      }

      let pulledCount = 0;
      if (syncAfterSave && (effectiveApiKey || selectedPreset?.authLoginType === 'local_socket')) {
        setSavingStep('Melakukan discovery & menarik model upstream...');
        try {
          const syncRes = await api.providers.syncModels(created.id);
          pulledCount = syncRes.count;
        } catch (err) {
          toast.warn('Provider dibuat, tetapi sinkronisasi model awal gagal: ' + (err instanceof Error ? err.message : String(err)));
        }
      }

      await loadData();
      onClose();

      if (pulledCount > 0) {
        toast.success(`Provider "${created.display_name || created.name}" berhasil didaftarkan (${pulledCount} model ditarik)`);
      } else {
        toast.success(`Provider "${created.display_name || created.name}" berhasil didaftarkan`);
      }

      handleOpenDrawer(created, 'models');
    } catch (err: any) {
      toast.error('Gagal mendaftarkan provider: ' + (err.message || err));
    } finally {
      setIsSaving(false);
      setSavingStep('');
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={selectedPreset ? `Hubungkan ${selectedPreset.displayName}` : 'Tambah Provider AI Manual'}
      subtitle={selectedPreset ? selectedPreset.description : 'Daftarkan endpoint upstream LLM kustom (vLLM, Ollama, OpenRouter, atau server privat)'}
      maxWidth="xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Batal
          </Button>
          <Button
            type="submit"
            variant="primary"
            form="create-provider-form"
            isLoading={isSaving}
            icon={<Check className="w-4 h-4" />}
            title="Simpan provider baru ke database"
          >
            Daftarkan
          </Button>
        </>
      }
    >
      <form id="create-provider-form" onSubmit={handleCreateSubmit} className="space-y-4 text-xs">
        {/* Preset Brand Banner */}
        {selectedPreset && (
          <div
            className="p-3 rounded-xl border flex items-center justify-between gap-3"
            style={{
              backgroundColor: selectedPreset.bgColor,
              borderColor: selectedPreset.borderColor,
            }}
          >
            <div className="flex items-center gap-3">
              <div
                className="w-9 h-9 rounded-lg flex items-center justify-center shadow-sm"
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

            {selectedPreset.highlightModels && selectedPreset.highlightModels.length > 0 && (
              <div className="hidden sm:block text-right">
                <span className="text-[10px] text-text-muted block font-mono uppercase">Highlight Model:</span>
                <span className="text-[11px] font-mono text-white font-semibold">
                  {selectedPreset.highlightModels.slice(0, 2).join(', ')}
                </span>
              </div>
            )}
          </div>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5" title="ID teknis unik tanpa spasi, contoh nama-provider-unik">ID Unik *</label>
            <input
              type="text"
              required
              placeholder="nama-provider-unik"
              value={newProv.name}
              onChange={(e) => setNewProv({ ...newProv, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Tampilan *</label>
            <input
              type="text"
              required
              placeholder="Nama Provider"
              value={newProv.display_name}
              onChange={(e) => setNewProv({ ...newProv, display_name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div className="col-span-1">
            <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Format protokol upstream">Kind</label>
            <select
              value={newProv.kind}
              onChange={(e) => setNewProv({ ...newProv, kind: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            >
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
              <option value="google">Google Gemini</option>
              <option value="openai_compatible">OpenAI Compatible</option>
              <option value="custom">Custom Engine</option>
            </select>
          </div>
          <div className="col-span-2">
            <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Alamat endpoint HTTP upstream">Base URL *</label>
            <input
              type="text"
              required
              value={newProv.base_url}
              onChange={(e) => setNewProv({ ...newProv, base_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
        </div>

        {/* DUAL-AUTH SECTION */}
        <div className="rounded-xl border border-border bg-bg-surface-2/60 p-3.5 space-y-3">
          {/* Dual-Auth Tab Switcher */}
          <div className="flex border-b border-border/80 gap-3 pb-2 text-xs">
            <button
              type="button"
              onClick={() => setAuthTab('authlogin')}
              className={`pb-1 font-medium transition-colors flex items-center gap-1.5 ${
                authTab === 'authlogin'
                  ? 'border-b-2 border-accent text-accent font-bold'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <ArrowUpRight className="w-3.5 h-3.5" />
              1. Auth Login & Otorisasi Terpandu
            </button>
            <button
              type="button"
              onClick={() => setAuthTab('apikey')}
              className={`pb-1 font-medium transition-colors flex items-center gap-1.5 ${
                authTab === 'apikey'
                  ? 'border-b-2 border-accent text-accent font-bold'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <KeyRound className="w-3.5 h-3.5" />
              2. Input API Key Manual
            </button>
          </div>

          {/* TAB 1: AUTH LOGIN TERPANDU */}
          {authTab === 'authlogin' && (
            <div className="space-y-3 pt-1">
              {/* Alur A: Redirect Callback URL Copying (HuggingFace, Claude, Cloudflare, OAuth) */}
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
                          className="inline-flex items-center gap-1.5 mt-2 px-3 py-1.5 rounded-lg bg-accent text-black font-semibold text-xs hover:opacity-90 transition-opacity"
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
                        Salin URL Callback / Redirect atau Token Fallback:
                      </label>
                      <textarea
                        rows={2}
                        value={authFallbackInput}
                        onChange={(e) => handleFallbackInputChange(e.target.value)}
                        placeholder="Tempel seluruh URL redirect (misal: http://localhost:54321/callback?code=xxx atau token) di sini..."
                        className="w-full px-3 py-2 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                      />
                      {authExtractedToken && (
                        <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                          <Check className="w-3.5 h-3.5 flex-shrink-0" />
                          <span>
                            Token/Kode Otorisasi Terdeteksi ({authExtractionHint}):{' '}
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

              {/* Alur B: Console Token Portal */}
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
                          className="inline-flex items-center gap-1.5 mt-2 px-3 py-1.5 rounded-lg bg-accent text-black font-semibold text-xs hover:opacity-90 transition-opacity"
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
                        Tempelkan Token atau URL Redirect yang Anda Peroleh:
                      </label>
                      <div className="relative">
                        <input
                          type={showApiKey ? 'text' : 'password'}
                          placeholder={selectedPreset.authFallbackHint || 'Tempel token di sini...'}
                          value={authFallbackInput}
                          onChange={(e) => handleFallbackInputChange(e.target.value)}
                          className="w-full px-3 py-2 pr-10 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                        />
                        <button
                          type="button"
                          onClick={() => setShowApiKey(!showApiKey)}
                          className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white"
                        >
                          {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                        </button>
                      </div>
                      {authExtractedToken && (
                        <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                          <Check className="w-3.5 h-3.5 flex-shrink-0" />
                          <span>
                            Token Terdeteksi ({authExtractionHint}):{' '}
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

              {/* Alur C: Local Socket Zero-Config */}
              {(selectedPreset?.authLoginType === 'local_socket' || (!selectedPreset && (newProv.base_url.includes('localhost') || newProv.base_url.includes('127.0.0.1')))) && (
                <div className="space-y-2">
                  <div className="flex items-center gap-2 text-white font-semibold text-xs">
                    <Server className="w-4 h-4 text-accent" />
                    <span>Socket Server Lokal (Zero-Config)</span>
                  </div>
                  <p className="text-[11px] text-text-muted">
                    {selectedPreset?.authInstructions ||
                      'Provider lokal tidak memerlukan API key eksternal. Pastikan server lokal aktif di port yang sesuai.'}
                  </p>
                  <div className="pt-2 flex flex-wrap items-center gap-2">
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

              {/* Alur D: Custom Manual Provider Default */}
              {!selectedPreset && !(newProv.base_url.includes('localhost') || newProv.base_url.includes('127.0.0.1')) && (
                <div className="space-y-2">
                  <p className="text-[11px] text-text-muted">
                    Jika Anda memiliki URL redirect callback OAuth atau token sementara dari server upstream, tempelkan di bawah. Sistem akan mengekstrak kode atau token secara otomatis.
                  </p>
                  <textarea
                    rows={2}
                    value={authFallbackInput}
                    onChange={(e) => handleFallbackInputChange(e.target.value)}
                    placeholder="Tempel seluruh URL redirect atau token di sini..."
                    className="w-full px-3 py-2 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                  />
                  {authExtractedToken && (
                    <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                      <Check className="w-3.5 h-3.5 flex-shrink-0" />
                      <span>
                        Token Terdeteksi ({authExtractionHint}):{' '}
                        <strong className="text-white">{authExtractedToken}</strong>
                      </span>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          {/* TAB 2: INPUT MANUAL API KEY */}
          {authTab === 'apikey' && (
            <div className="space-y-2 pt-1">
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
                  placeholder={selectedPreset?.apiKeyPlaceholder || 'sk-... atau Bearer Token'}
                  value={quickApiKey}
                  onChange={(e) => setQuickApiKey(e.target.value)}
                  className="w-full px-3 py-2 pr-10 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                />
                <button
                  type="button"
                  onClick={() => setShowApiKey(!showApiKey)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white"
                >
                  {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
              <p className="text-[11px] text-text-muted flex items-center gap-1.5" title="Kredensial langsung dienkripsi sebelum disimpan ke database.">
                <Lock className="w-3.5 h-3.5 text-accent flex-shrink-0" />
                Dienkripsi amplop AES-256-GCM tingkat record PostgreSQL dengan AAD.
              </p>
            </div>
          )}
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Jalur koneksi keluar dari server ke upstream">Jalur Egress Outbound</label>
          <select
            value={selectedEgressPoolId}
            onChange={(e) => setSelectedEgressPoolId(e.target.value)}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
          >
            <option value="">Direct Outbound (Tanpa Proxy)</option>
            {egressPools.map((pool: any) => (
              <option key={pool.id} value={pool.id}>{pool.name} ({pool.kind})</option>
            ))}
          </select>
        </div>

        {(effectiveApiKey || selectedPreset?.authLoginType === 'local_socket') && (
          <div className="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="syncModelsToggle"
              checked={syncAfterSave}
              onChange={(e) => setSyncAfterSave(e.target.checked)}
              className="rounded bg-bg-surface-2 border-border"
            />
            <label htmlFor="syncModelsToggle" className="font-medium text-white cursor-pointer" title="Menarik daftar model dari upstream setelah provider tersimpan">
              Tarik model upstream otomatis setelah disimpan
            </label>
          </div>
        )}

        {isSaving && savingStep && (
          <div className="p-3 rounded-lg bg-accent/10 border border-accent/20 text-accent text-xs flex items-center gap-2 animate-pulse">
            <RefreshCw className="w-4 h-4 animate-spin flex-shrink-0" /><span>{savingStep}</span>
          </div>
        )}
      </form>
    </Modal>
  );
};


export const AddModelModalChild: React.FC<any> = ({ isOpen, onClose, selectedProvider, loadProviderDetails, toast, api }) => {
  const [newModelName, setNewModelName] = useState('');
  const [isSavingModel, setIsSavingModel] = useState(false);

  const handleAddModelManual = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;
    const trimmed = newModelName.trim();
    if (!trimmed) {
      toast.error('Nama model wajib diisi');
      return;
    }

    setIsSavingModel(true);
    try {
      await api.providers.addModel(selectedProvider.id, trimmed);
      toast.success(`Model "${trimmed}" berhasil didaftarkan`);
      setNewModelName('');
      onClose();
      await loadProviderDetails(selectedProvider.id);
    } catch (err: any) {
      toast.error('Gagal menambahkan model: ' + (err.message || err));
    } finally {
      setIsSavingModel(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={`Tambah Model Manual — ${selectedProvider?.display_name || selectedProvider?.name}`}
      subtitle="Daftarkan ID model khusus yang diterima oleh upstream Anda"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Batal
          </Button>
          <Button
            type="submit"
            variant="primary"
            form="add-model-manual-form"
            isLoading={isSavingModel}
            icon={<Plus className="w-4 h-4" />}
          >
            Simpan Model
          </Button>
        </>
      }
    >
      <form id="add-model-manual-form" onSubmit={handleAddModelManual} className="space-y-4 text-xs">
        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5" title="String ID model persis sesuai dokumentasi provider Anda">ID Model *</label>
          <input type="text" name="model_name" required placeholder="claude-3-7-sonnet, gpt-4o, atau deepseek/deepseek-chat" value={newModelName} onChange={(e) => setNewModelName(e.target.value)} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent" />
          <p className="text-[11px] text-text-muted mt-1" title="Masukkan string ID model persis sesuai dokumentasi provider Anda.">Sesuai dokumentasi provider.</p>
        </div>
      </form>
    </Modal>
  );
};
