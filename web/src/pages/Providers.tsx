import React, { useEffect, useState, useMemo, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Provider, Credential, EgressPool, ProviderModel, OAuthSession } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { Tooltip } from '../components/common/Tooltip';
import {
  Server,
  Plus,
  Globe,
  Activity,
  ChevronDown,
  ChevronUp,
  Sliders,
  Sparkles,
  Shield,
  KeyRound,
  Copy,
  Users,
  Bot,
  FlaskConical,
  ArrowRight,
} from 'lucide-react';
import { StatusDot } from '../components/common/StatusDot';
import {
  KNOWN_PROVIDERS,
  ProviderBrandIcon,
  type KnownProviderPreset,
} from '../components/providers/ProviderIcons';
import {
  matchPresetForProvider,
  matchExistingProvider,
} from '../components/providers/providerMatching';
import { ModelsTab, type ModelsTabProps } from '../components/providers/ModelsTab';
import { CredentialsTab, type CredentialsTabProps } from '../components/providers/CredentialsTab';
import { SettingsTab, type SettingsTabProps } from '../components/providers/SettingsTab';
import { CreateProviderModal, type CreateProviderModalProps } from '../components/providers/CreateProviderModal';
import { AddModelModal, type AddModelModalProps } from '../components/providers/AddModelModal';
import type { ModelTestResult } from '../components/providers/types';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { copyTextToClipboard } from '../utils/clipboard';

export { matchPresetForProvider, matchExistingProvider };
export { ModelsTab as ModelsTabChild };
export { CredentialsTab as CredentialsTabChild };
export { SettingsTab as SettingsTabChild };
export { CreateProviderModal as CreateProviderModalChild };
export { AddModelModal as AddModelModalChild };
export type {
  ModelTestResult,
  ModelsTabProps,
  CredentialsTabProps,
  SettingsTabProps,
  CreateProviderModalProps,
  AddModelModalProps,
};

export const Providers: React.FC = () => {
  const { toast, confirmModal } = useToast();

  const queryClient = useQueryClient();

  // Data Utama (TanStack Query)
  const { data: provsData = [], isLoading: isProvidersLoading, isError: isProvidersError, error: providersError, refetch: refetchProviders } = useQuery({
    queryKey: ['providers'],
    queryFn: async () => {
      const res = await api.providers.list();
      return Array.isArray(res) ? res : (res as { items?: Provider[] }).items || [];
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

  // Ringkasan Pool Terpadu (Kredensial & Sesi OAuth untuk setiap Provider)
  const { data: poolsSummary = {} } = useQuery({
    queryKey: ['providersPoolSummary', providers.map((p) => p.id).join(',')],
    enabled: providers.length > 0,
    queryFn: async () => {
      const summaryMap: Record<
        string,
        { credentials: Credential[]; oauthSessions: OAuthSession[] }
      > = {};

      await Promise.all(
        providers.map(async (p) => {
          try {
            const [credsRes, oauthRes] = await Promise.all([
              api.credentials.list(p.id).catch(() => ({ items: [] })),
              api.providers.listOAuthSessions(p.id).catch(() => ({ items: [] })),
            ]);
            summaryMap[p.id] = {
              credentials: credsRes.items || [],
              oauthSessions: oauthRes.items || [],
            };
          } catch {
            summaryMap[p.id] = { credentials: [], oauthSessions: [] };
          }
        })
      );

      return summaryMap;
    },
    staleTime: 10_000,
  });

  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  // Penjaga race: abaikan respons basi bila user sudah pindah ke provider lain.
  const providerDetailsReqRef = useRef(0);

  // Data Child untuk Selected Provider
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [models, setModels] = useState<ProviderModel[]>([]);
  const [oauthSessions, setOauthSessions] = useState<OAuthSession[]>([]);
  const [providerDetailsError, setProviderDetailsError] = useState<string | null>(null);

  // Status Aksi & Pengujian
  const [probingId, setProbingId] = useState<string | null>(null);
  const [syncingId, setSyncingId] = useState<string | null>(null);
  const [testingModelId, setTestingModelId] = useState<string | null>(null);
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
  const [createModalTargetOverride, setCreateModalTargetOverride] = useState<Provider | null>(null);

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
      queryClient.invalidateQueries({ queryKey: ['providersPoolSummary'] }),
      queryClient.invalidateQueries({ queryKey: ['egressPools'] }),
    ]);
  };

  // Sinkronisasi selectedProvider ketika data query diperbarui
  useEffect(() => {
    if (selectedProvider) {
      const found = providers.find((p: Provider) => p.id === selectedProvider.id);
      if (found) {
        setSelectedProvider(found);
      }
    }
  }, [providers]);

  // ---------------------------------------------------------------------------
  // Load Detail Child untuk Provider Terpilih (Credentials, Models, OAuth)
  // ---------------------------------------------------------------------------
  const loadProviderDetails = async (providerId: string) => {
    const reqId = ++providerDetailsReqRef.current;
    setProviderDetailsError(null);
    try {
      const [credsRes, modsRes, oauthRes] = await Promise.all([
        api.credentials.list(providerId),
        api.providers.models(providerId),
        api.providers.listOAuthSessions(providerId).catch(() => ({ items: [] })),
      ]);
      if (providerDetailsReqRef.current !== reqId) return;
      setCredentials(credsRes.items || []);
      setModels(modsRes.items || []);
      setOauthSessions(oauthRes.items || []);
    } catch (err) {
      if (providerDetailsReqRef.current !== reqId) return;
      setCredentials([]);
      setModels([]);
      setOauthSessions([]);
      setProviderDetailsError(err instanceof Error ? err.message : String(err));
    }
  };

  // Buka Drawer Konfigurasi untuk Provider Tertentu
  const handleOpenDrawer = (prov: Provider, tab: 'models' | 'credentials' | 'settings' = 'models') => {
    setSelectedProvider(prov);
    setDrawerTab(tab);
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
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal melakukan probe provider: ' + errMsg);
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
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal menarik model upstream: ' + errMsg);
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
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      setModelTestResults((prev) => ({
        ...prev,
        [modelMapping.id]: {
          ok: false,
          latency_ms: 0,
          timestamp: new Date().toLocaleTimeString(),
          error: errMsg,
        },
      }));
      toast.error('Gagal menguji model: ' + errMsg);
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

  // ---------------------------------------------------------------------------
  // Handlers: Form Tambah Provider Baru / Preset
  // ---------------------------------------------------------------------------
  const handleSelectPreset = (preset: KnownProviderPreset) => {
    setSelectedPreset(preset);
    setCreateModalTargetOverride(null);
    setIsCreateModalOpen(true);
  };

  const handleOpenCustomCreate = () => {
    setSelectedPreset(null);
    setCreateModalTargetOverride(null);
    setIsCreateModalOpen(true);
  };

  const handleQuickAddAccount = (p: Provider) => {
    const matchedPreset = matchPresetForProvider(p);
    setSelectedPreset(matchedPreset);
    setCreateModalTargetOverride(p);
    setIsCreateModalOpen(true);
  };

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
            const isCustom = isCustomProvider(p);
            const egress = egressPools.find((ep) => ep.id === p.egress_pool_id);

            const pool = poolsSummary[p.id] || { credentials: [], oauthSessions: [] };
            const directCreds = (pool.credentials || []).filter(
              (c: Credential) => !c.label.startsWith('oauth:') && !(pool.oauthSessions || []).some((s: OAuthSession) => s.credential_id === c.id)
            );
            const activeOAuth = (pool.oauthSessions || []).filter((s: OAuthSession) => s.enabled).length;
            const activeDirect = directCreds.filter((c: Credential) => c.enabled).length;
            const totalAccounts = (pool.oauthSessions || []).length + directCreds.length;
            const activeAccounts = activeOAuth + activeDirect;

              const providerStatus: 'healthy' | 'degraded' | 'unhealthy' | 'disabled' | 'neutral' =
                !p.enabled
                  ? 'disabled'
                  : p.last_health_status === 'healthy'
                  ? 'healthy'
                  : p.last_health_status === 'degraded'
                  ? 'degraded'
                  : p.last_health_status === 'unhealthy'
                  ? 'unhealthy'
                  : 'neutral';

              return (
                <div
                  key={p.id}
                  className="bg-bg-surface border border-border hover:border-border/80 rounded-card p-5 transition-all flex flex-col justify-between shadow-sm group"
                >
                  {/* Bagian Atas: Icon, Identitas & Status Dot */}
                  <div className="space-y-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex items-center gap-3 min-w-0">
                        {/* Icon Provider Interaktif: Klik untuk Buka Drawer */}
                        <button
                          type="button"
                          onClick={() => handleOpenDrawer(p, 'settings')}
                          aria-label={`Buka pengaturan provider ${p.display_name || p.name}`}
                          title={
                            isCustom
                              ? 'Klik icon custom provider ini untuk membuka drawer konfigurasi & kelola'
                              : 'Klik icon provider untuk membuka drawer konfigurasi'
                          }
                          className={`relative w-10 h-10 sm:w-11 sm:h-11 rounded-inner flex items-center justify-center flex-shrink-0 transition-all cursor-pointer shadow-sm ${
                            isCustom
                              ? 'bg-purple-950/50 border border-purple-500/40 text-purple-300 hover:scale-105'
                              : 'bg-bg-surface-2 border border-border text-text-primary hover:border-accent/50 hover:scale-105'
                          }`}
                        >
                          <ProviderBrandIcon
                            providerIdOrKind={p.kind}
                            name={p.name}
                            className="w-5 h-5"
                            isCustom={isCustom}
                          />
                          <span
                            className={`absolute -bottom-1 -right-1 w-3.5 h-3.5 rounded-full bg-bg-surface border flex items-center justify-center shadow-sm ${
                              isCustom
                                ? 'border-purple-500/60 text-purple-300'
                                : 'border-border text-text-muted'
                            }`}
                            aria-hidden="true"
                          >
                            <Sliders className="w-2 h-2" aria-hidden="true" />
                          </span>
                        </button>

                        <div className="space-y-0.5 truncate">
                          <div className="flex items-center gap-2">
                            <h4 className="font-semibold text-white text-sm sm:text-base truncate">
                              {p.display_name || p.name}
                            </h4>
                            {isCustom && (
                              <span className="px-1.5 py-0.2 text-[10px] font-mono rounded bg-purple-500/10 text-purple-300 border border-purple-500/20 flex-shrink-0">
                                Custom
                              </span>
                            )}
                          </div>
                          <span className="text-xs text-text-muted font-mono block truncate">
                            {p.kind}
                          </span>
                        </div>
                      </div>

                      {/* Status Dot Minimalis */}
                      <div className="flex items-center flex-shrink-0 pt-0.5">
                        <StatusDot
                          status={providerStatus}
                          latencyMs={p.last_latency_ms}
                        />
                      </div>
                    </div>

                    {/* Base URL & Egress Proxy 1-Baris Ringkas */}
                    <div className="flex items-center justify-between text-xs font-mono text-text-secondary bg-bg-surface-2/60 px-3 py-2 rounded-inner border border-border">
                      <span className="truncate pr-2 font-mono">
                        {p.base_url.replace(/^https?:\/\//, '')}
                        <span className="text-border mx-1.5">·</span>
                        <span className="text-text-muted">{egress ? egress.name : 'Direct Outbound'}</span>
                      </span>
                      <Tooltip content="Salin Base URL" position="top">
                        <button
                          type="button"
                          onClick={() => void copyWithFeedback(p.base_url, 'Base URL disalin ke clipboard')}
                          aria-label="Salin Base URL"
                          className="p-1 rounded-nav text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors flex-shrink-0 cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
                        >
                          <Copy className="w-3.5 h-3.5" />
                        </button>
                      </Tooltip>
                    </div>

                    {/* Ringkasan Pool Kredensial Bersih */}
                    <div className="px-3 py-2.5 rounded-inner bg-bg-surface-2/40 border border-border flex items-center justify-between text-xs">
                      <div className="flex items-center gap-2 truncate">
                        <Users className="w-3.5 h-3.5 text-text-muted shrink-0" />
                        <span className="text-text-secondary font-medium truncate">
                          {totalAccounts === 0
                            ? 'Belum ada kredensial'
                            : `${activeAccounts} Kredensial Aktif · ${
                                p.credential_strategy === 'priority'
                                  ? 'Priority Failover'
                                  : 'Round Robin'
                              }`}
                        </span>
                      </div>
                      {totalAccounts === 0 && (
                        <button
                          type="button"
                          onClick={() => handleQuickAddAccount(p)}
                          className="text-[11px] text-accent hover:underline shrink-0 font-medium cursor-pointer"
                        >
                          + Tambah Key
                        </button>
                      )}
                    </div>
                  </div>

                  {/* Footer Kartu: 2 Tombol Aksi Bersih & Teratur */}
                  <div className="mt-5 pt-3.5 border-t border-border flex items-center justify-between gap-3">
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleProbe(p)}
                      isLoading={probingId === p.id}
                      icon={<FlaskConical className="w-3.5 h-3.5" />}
                    >
                      Uji Latensi
                    </Button>

                    <Button
                      variant="primary"
                      size="sm"
                      onClick={() => handleOpenDrawer(p, 'models')}
                      icon={<ArrowRight className="w-3.5 h-3.5 text-black" />}
                    >
                      Kelola &amp; Akun &rarr;
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
            <Tooltip content="Uji kesehatan provider sekarang" position="bottom">
              <button
                type="button"
                onClick={() => selectedProvider && handleProbe(selectedProvider)}
                aria-label="Periksa status kesehatan provider"
                className={`w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 transition-all cursor-pointer shadow-sm focus:outline-none focus-visible:ring-1 focus-visible:ring-accent ${
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
            </Tooltip>
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
            <div role="tablist" aria-label="Tab Konfigurasi Provider" className="grid grid-cols-3 gap-1 p-1 bg-bg-surface-2/80 rounded-xl border border-border">
              <button
                type="button"
                role="tab"
                id="provider-tab-models"
                aria-selected={drawerTab === 'models'}
                aria-controls="provider-tabpanel-models"
                onClick={() => setDrawerTab('models')}
                className={`flex items-center justify-center gap-1 sm:gap-1.5 py-1.5 sm:py-2 px-1 rounded-lg text-[11px] sm:text-xs font-semibold transition-all cursor-pointer ${
                  drawerTab === 'models'
                    ? 'bg-accent text-black font-bold shadow'
                    : 'text-text-secondary hover:text-white hover:bg-bg-surface/50'
                }`}
              >
                <Bot className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
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
                role="tab"
                id="provider-tab-credentials"
                aria-selected={drawerTab === 'credentials'}
                aria-controls="provider-tabpanel-credentials"
                onClick={() => setDrawerTab('credentials')}
                className={`flex items-center justify-center gap-1 sm:gap-1.5 py-1.5 sm:py-2 px-1 rounded-lg text-[11px] sm:text-xs font-semibold transition-all cursor-pointer ${
                  drawerTab === 'credentials'
                    ? 'bg-accent text-black font-bold shadow'
                    : 'text-text-secondary hover:text-white hover:bg-bg-surface/50'
                }`}
              >
                <KeyRound className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
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
                role="tab"
                id="provider-tab-settings"
                aria-selected={drawerTab === 'settings'}
                aria-controls="provider-tabpanel-settings"
                onClick={() => setDrawerTab('settings')}
                className={`flex items-center justify-center gap-1 sm:gap-1.5 py-1.5 sm:py-2 px-1 rounded-lg text-[11px] sm:text-xs font-semibold transition-all cursor-pointer ${
                  drawerTab === 'settings'
                    ? 'bg-accent text-black font-bold shadow'
                    : 'text-text-secondary hover:text-white hover:bg-bg-surface/50'
                }`}
              >
                <Sliders className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
                <span className="truncate">
                  <span className="sm:inline hidden">Pengaturan & Proxy</span>
                  <span className="sm:hidden inline">Setelan</span>
                </span>
              </button>
            </div>

            {/* TAB 1: MODEL UPSTREAM & DIAGNOSTIK */}
            {drawerTab === 'models' && (
              <ModelsTab
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

            {/* TAB 2: KREDENSIAL API KEY & OAUTH POOL */}
            {drawerTab === 'credentials' && (
              <CredentialsTab
                selectedProvider={selectedProvider}
                credentials={credentials}
                oauthSessions={oauthSessions}
                egressPools={egressPools}
                handleToggleKey={handleToggleKey}
                handleDeleteKey={handleDeleteKey}
                loadProviderDetails={loadProviderDetails}
                toast={toast}
                confirmModal={confirmModal}
                api={api}
                copyWithFeedback={copyWithFeedback}
                onUpdateProvider={(updated: Provider) => {
                  setSelectedProvider(updated);
                  loadData();
                }}
              />
            )}

            {/* TAB 3: PENGATURAN TEKNIS & JALUR PROXY */}
            {drawerTab === 'settings' && (
              <SettingsTab
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
      {/* 5. MODAL CREATE PROVIDER BARU / PRESET / ADD TO POOL */}
      <CreateProviderModal
        isOpen={isCreateModalOpen}
        onClose={() => {
          setIsCreateModalOpen(false);
          setCreateModalTargetOverride(null);
        }}
        selectedPreset={selectedPreset}
        providers={providers}
        egressPools={egressPools}
        loadData={loadData}
        handleOpenDrawer={handleOpenDrawer}
        toast={toast}
        api={api}
        targetProviderOverride={createModalTargetOverride}
      />

      {/* ------------------------------------------------------------------- */}
      <AddModelModal
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
