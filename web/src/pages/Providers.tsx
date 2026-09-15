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
} from 'lucide-react';
import {
  KNOWN_PROVIDERS,
  ProviderBrandIcon,
  type KnownProviderPreset,
} from '../components/providers/ProviderIcons';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { copyTextToClipboard } from '../utils/clipboard';

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

  // Form Tambah Key Inline di Drawer
  const [isAddingKeyInline, setIsAddingKeyInline] = useState(false);
  const [newKeyForm, setNewKeyForm] = useState({
    label: 'Primary API Key',
    api_key: '',
  });
  const [showNewKeySecret, setShowNewKeySecret] = useState(false);
  const [isSavingKey, setIsSavingKey] = useState(false);

  // State Form Create Provider Baru / Preset
  const [selectedPreset, setSelectedPreset] = useState<KnownProviderPreset | null>(null);
  const [quickApiKey, setQuickApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [selectedEgressPoolId, setSelectedEgressPoolId] = useState<string>('');
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

  // State Form Tambah Model Manual
  const [newModelName, setNewModelName] = useState('');
  const [isSavingModel, setIsSavingModel] = useState(false);

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
    setQuickApiKey('');
    setSelectedEgressPoolId('');
    setSyncAfterSave(true);
    setNewProv({
      name: preset.name,
      display_name: preset.displayName,
      kind: preset.kind,
      base_url: preset.baseUrl,
      priority: preset.defaultPriority,
      weight: preset.defaultWeight,
      timeout_ms: 30000,
      egress_pool_id: undefined,
    });
    setIsCreateModalOpen(true);
  };

  const handleOpenCustomCreate = () => {
    setSelectedPreset(null);
    setQuickApiKey('');
    setSelectedEgressPoolId('');
    setSyncAfterSave(false);
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
    setIsCreateModalOpen(true);
  };

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

      if (quickApiKey.trim()) {
        setSavingStep('Menyimpan dan mengenkripsi API Key (AES-256-GCM)...');
        try {
          await api.credentials.create(created.id, {
            label: 'Primary API Key',
            api_key: quickApiKey.trim(),
          });
        } catch (keyErr: any) {
          toast.warn('Provider dibuat, namun kunci API gagal disimpan: ' + keyErr.message);
        }
      }

      let pulledCount = 0;
      if (syncAfterSave && quickApiKey.trim()) {
        setSavingStep('Melakukan discovery & menarik model upstream...');
        try {
          const syncRes = await api.providers.syncModels(created.id);
          pulledCount = syncRes.count;
        } catch (err) {
          toast.warn('Provider dibuat, tetapi sinkronisasi model awal gagal: ' + (err instanceof Error ? err.message : String(err)));
        }
      }

      await loadData();
      setIsCreateModalOpen(false);

      if (pulledCount > 0) {
        toast.success(`Provider "${created.display_name || created.name}" berhasil didaftarkan (${pulledCount} model ditarik)`);
      } else {
        toast.success(`Provider "${created.display_name || created.name}" berhasil didaftarkan`);
      }

      // Langsung buka drawer untuk provider baru
      handleOpenDrawer(created, 'models');
    } catch (err: any) {
      toast.error('Gagal mendaftarkan provider: ' + (err.message || err));
    } finally {
      setIsSaving(false);
      setSavingStep('');
    }
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
        selectedPreset={null}
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
  const [newKeyForm, setNewKeyForm] = useState({ label: 'Primary API Key', api_key: '' });
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

  const handleCreateKey = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;
    const keyLabel = newKeyForm.label.trim();
    const keySecret = newKeyForm.api_key.trim();

    if (!keySecret) {
      toast.error('Secret token API key wajib diisi');
      return;
    }

    setIsSavingKey(true);
    try {
      await api.credentials.create(selectedProvider.id, { label: keyLabel || 'Primary API Key', api_key: keySecret });
      toast.success('API Key berhasil ditambahkan dengan enkripsi AES-256-GCM');
      setIsAddingKeyInline(false);
      setNewKeyForm({ label: 'Backup API Key', api_key: '' });
      await loadProviderDetails(selectedProvider.id);
    } catch (err: any) {
      toast.error('Gagal menambahkan API Key: ' + (err.message || err));
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
        <div className="p-3 rounded-xl border border-accent/40 bg-accent/5 space-y-2.5">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-semibold text-white">Key Baru</span>
            <button type="button" onClick={() => setIsAddingKeyInline(false)} className="text-[11px] text-text-muted hover:text-white">Batal</button>
          </div>
          <form onSubmit={handleCreateKey} className="space-y-2.5 text-xs">
            <input type="text" required placeholder="Label — mis. Primary API Key" value={newKeyForm.label} onChange={(e) => setNewKeyForm({ ...newKeyForm, label: e.target.value })} className="w-full px-2.5 py-1.5 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs placeholder:text-text-muted outline-none focus:border-accent" />
            <div className="relative">
              <input type={showNewKeySecret ? 'text' : 'password'} required placeholder="Secret / Token — mis. sk-ant-..." value={newKeyForm.api_key} onChange={(e) => setNewKeyForm({ ...newKeyForm, api_key: e.target.value })} className="w-full px-2.5 py-1.5 pr-9 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs placeholder:text-text-muted outline-none focus:border-accent" />
              <button type="button" onClick={() => setShowNewKeySecret(!showNewKeySecret)} className="absolute right-2 top-2 text-text-muted hover:text-white">
                {showNewKeySecret ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
              </button>
            </div>
            <div className="flex justify-end gap-1.5">
              <button type="button" onClick={() => setIsAddingKeyInline(false)} className="px-2.5 py-1.5 text-xs rounded-lg bg-bg-surface-2 text-text-primary border border-border hover:text-white transition-colors">Batal</button>
              <button type="submit" disabled={isSavingKey} className="px-2.5 py-1.5 text-xs rounded-lg bg-accent text-black font-bold hover:bg-accent-hover transition-colors disabled:opacity-50 flex items-center gap-1">
                {isSavingKey ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Check className="w-3.5 h-3.5" />}
                Simpan
              </button>
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


export const CreateProviderModalChild: React.FC<any> = ({ isOpen, onClose, selectedPreset, egressPools, loadData, toast, handleOpenDrawer, api }) => {
  const [quickApiKey, setQuickApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
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
      setSelectedEgressPoolId('');
      if (selectedPreset) {
        setSyncAfterSave(true);
        setNewProv({
          name: selectedPreset.name,
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
  }, [isOpen, selectedPreset]);

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

      if (quickApiKey.trim()) {
        setSavingStep('Menyimpan dan mengenkripsi API Key (AES-256-GCM)...');
        try {
          await api.credentials.create(created.id, {
            label: 'Primary API Key',
            api_key: quickApiKey.trim(),
          });
        } catch (keyErr: any) {
          toast.warn('Provider dibuat, namun kunci API gagal disimpan: ' + keyErr.message);
        }
      }

      let pulledCount = 0;
      if (syncAfterSave && quickApiKey.trim()) {
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
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5" title="ID teknis unik tanpa spasi, contoh nama-provider-unik">ID *</label>
            <input type="text" required placeholder="nama-provider-unik" value={newProv.name} onChange={(e) => setNewProv({ ...newProv, name: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent" />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama *</label>
            <input type="text" required placeholder="Nama Provider" value={newProv.display_name} onChange={(e) => setNewProv({ ...newProv, display_name: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent" />
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div className="col-span-1">
            <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Format protokol upstream">Kind</label>
            <select value={newProv.kind} onChange={(e) => setNewProv({ ...newProv, kind: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent">
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
              <option value="google">Google Gemini</option>
              <option value="openai_compatible">OpenAI Compatible</option>
              <option value="custom">Custom Engine</option>
            </select>
          </div>
          <div className="col-span-2">
            <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Alamat endpoint HTTP upstream">Base URL *</label>
            <input type="text" required value={newProv.base_url} onChange={(e) => setNewProv({ ...newProv, base_url: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent" />
          </div>
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Boleh kosong, bisa ditambah belakangan dari tab Kredensial">API Key (opsional)</label>
          <div className="relative">
            <input type={showApiKey ? 'text' : 'password'} placeholder={selectedPreset?.apiKeyPlaceholder || 'sk-... atau Bearer Token'} value={quickApiKey} onChange={(e) => setQuickApiKey(e.target.value)} className="w-full px-3 py-2 pr-10 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent" />
            <button type="button" onClick={() => setShowApiKey(!showApiKey)} className="absolute right-2.5 top-2.5 text-text-muted hover:text-white">
              {showApiKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
          </div>
          <p className="text-[11px] text-text-muted mt-1.5 flex items-center gap-1.5" title="Token langsung dienkripsi sebelum disimpan ke basis data PostgreSQL.">
            <Shield className="w-3.5 h-3.5 text-accent flex-shrink-0" /> Dienkripsi sebelum disimpan
          </p>
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5" title="Jalur koneksi keluar dari server ke upstream">Egress</label>
          <select value={selectedEgressPoolId} onChange={(e) => setSelectedEgressPoolId(e.target.value)} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent">
            <option value="">Direct Outbound (Tanpa Proxy)</option>
            {egressPools.map((pool: any) => (
              <option key={pool.id} value={pool.id}>{pool.name} ({pool.kind})</option>
            ))}
          </select>
        </div>

        {quickApiKey.trim() && (
          <div className="flex items-center gap-2 pt-1">
            <input type="checkbox" id="syncModelsToggle" checked={syncAfterSave} onChange={(e) => setSyncAfterSave(e.target.checked)} className="rounded bg-bg-surface-2 border-border" />
            <label htmlFor="syncModelsToggle" className="font-medium text-white cursor-pointer" title="Menarik daftar model dari upstream setelah provider tersimpan">Tarik model otomatis</label>
          </div>
        )}

        {isSaving && savingStep && (
          <div className="p-3 rounded-lg bg-accent/10 border border-accent/20 text-accent text-xs flex items-center gap-2 animate-pulse">
            <RefreshCw className="w-4 h-4 animate-spin" /><span>{savingStep}</span>
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
