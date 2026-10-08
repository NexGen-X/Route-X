import React, { useEffect, useState } from 'react';
import type { Provider, EgressPool } from '../../types';
import type { api as apiClient } from '../../api/client';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { Checkbox } from '../common/Checkbox';
import {
  Server,
  Plus,
  RefreshCw,
  CheckCircle2,
  Globe,
  Activity,
  Eye,
  EyeOff,
  Check,
  ChevronDown,
  ChevronUp,
  Shield,
  KeyRound,
  ArrowUpRight,
  ExternalLink,
  Lock,
  XCircle,
  Users,
  RotateCcw,
  Sparkles,
} from 'lucide-react';
import {
  KNOWN_PROVIDERS,
  ProviderBrandIcon,
  type KnownProviderPreset,
} from './ProviderIcons';
import { matchExistingProvider } from './providerMatching';
import { extractAuthTokenFromInput } from '../../utils/authExtractor';

export interface CreateProviderModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedPreset: KnownProviderPreset | null;
  providers?: Provider[];
  egressPools?: EgressPool[];
  loadData: () => Promise<void>;
  handleOpenDrawer: (prov: Provider, tab?: 'models' | 'credentials' | 'settings') => void;
  toast: {
    success: (msg: string) => void;
    error: (msg: string) => void;
    warn: (msg: string) => void;
    info?: (msg: string) => void;
  };
  api: typeof apiClient;
  targetProviderOverride?: Provider | null;
}

export const CreateProviderModal: React.FC<CreateProviderModalProps> = ({
  isOpen,
  onClose,
  selectedPreset: initialPreset,
  providers = [],
  egressPools = [],
  loadData,
  toast,
  handleOpenDrawer,
  api,
  targetProviderOverride,
}) => {
  const [currentPreset, setCurrentPreset] = useState<KnownProviderPreset | null>(initialPreset);
  const [showPresetPicker, setShowPresetPicker] = useState<boolean>(false);

  const [modalMode, setModalMode] = useState<'add_to_pool' | 'create_new'>('create_new');
  const [targetExistingProvider, setTargetExistingProvider] = useState<Provider | null>(null);
  const [accountLabel, setAccountLabel] = useState('');

  const [authTab, setAuthTab] = useState<'apikey' | 'oauth'>('apikey');
  const [quickApiKey, setQuickApiKey] = useState('');
  const [showApiKey, setShowApiKey] = useState(false);
  const [authFallbackInput, setAuthFallbackInput] = useState('');
  const [authExtractedToken, setAuthExtractedToken] = useState('');
  const [authExtractionHint, setAuthExtractionHint] = useState('');
  const [socketPingStatus, setSocketPingStatus] = useState<{ testing: boolean; latency?: number; ok?: boolean } | null>(null);
  const [selectedEgressPoolId, setSelectedEgressPoolId] = useState('');
  const [syncAfterSave, setSyncAfterSave] = useState(true);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [savingStep, setSavingStep] = useState('');

  const [newProv, setNewProv] = useState({
    name: '',
    display_name: '',
    kind: 'openai' as Provider['kind'],
    base_url: 'https://api.openai.com/v1',
    priority: 100,
    weight: 100,
    timeout_ms: 30000,
    egress_pool_id: undefined as string | undefined,
  });

  const setupPreset = (preset: KnownProviderPreset | null) => {
    setCurrentPreset(preset);
    if (preset) {
      const matched = targetProviderOverride || matchExistingProvider(preset, providers);
      if (matched) {
        setTargetExistingProvider(matched);
        setModalMode('add_to_pool');
        const isOauth = preset.authLoginType === 'oauth_fallback' || matched.kind === 'google';
        setAuthTab(isOauth ? 'oauth' : 'apikey');
        setAccountLabel(
          isOauth
            ? ''
            : preset.accountLabelPlaceholder
            ? ''
            : 'Akun Tambahan'
        );
      } else {
        setTargetExistingProvider(null);
        setModalMode('create_new');
        setAuthTab(preset.authLoginType === 'oauth_fallback' ? 'oauth' : 'apikey');
        setAccountLabel('Akun Utama');
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
      }
    } else {
      setTargetExistingProvider(null);
      setModalMode('create_new');
      setAuthTab('apikey');
      setAccountLabel('Primary Key');
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
  };

  useEffect(() => {
    if (isOpen) {
      setQuickApiKey('');
      setAuthFallbackInput('');
      setAuthExtractedToken('');
      setAuthExtractionHint('');
      setShowApiKey(false);
      setSelectedEgressPoolId('');
      setSocketPingStatus(null);
      setIsSaving(false);
      setSavingStep('');
      setShowPresetPicker(!initialPreset);
      setupPreset(initialPreset);
    }
  }, [isOpen, initialPreset, providers, targetProviderOverride]);

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

  const switchToCreateNew = () => {
    setModalMode('create_new');
    if (currentPreset) {
      const existingCount = (providers || []).filter(
        (p) => p.kind === currentPreset.kind || p.name === currentPreset.name || p.name.startsWith(currentPreset.name + '-')
      ).length;
      const uniqueName = existingCount > 0 ? `${currentPreset.name}-${existingCount + 1}` : currentPreset.name;
      setNewProv({
        name: uniqueName,
        display_name: currentPreset.displayName,
        kind: currentPreset.kind,
        base_url: currentPreset.baseUrl,
        priority: currentPreset.defaultPriority,
        weight: currentPreset.defaultWeight,
        timeout_ms: 30000,
        egress_pool_id: undefined,
      });
      setAccountLabel('Akun Utama');
    }
  };

  const switchAddToPool = () => {
    if (targetExistingProvider) {
      setModalMode('add_to_pool');
      const isOauth = currentPreset?.authLoginType === 'oauth_fallback' || targetExistingProvider.kind === 'google';
      setAuthTab(isOauth ? 'oauth' : 'apikey');
      setAccountLabel(
        isOauth
          ? ''
          : currentPreset?.accountLabelPlaceholder
          ? ''
          : 'Akun Tambahan'
      );
    }
  };

  const effectiveApiKey = authTab === 'apikey'
    ? quickApiKey.trim()
    : (authExtractedToken.trim() || authFallbackInput.trim());

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);

    if (modalMode === 'add_to_pool' && targetExistingProvider) {
      try {
        if (!effectiveApiKey) {
          toast.error('Token, URL redirect, atau API Key wajib diisi');
          setIsSaving(false);
          return;
        }

        const isOauth = authTab === 'oauth' || currentPreset?.authLoginType === 'oauth_fallback' || targetExistingProvider.kind === 'google';

        if (isOauth) {
          setSavingStep('Menukarkan token OAuth ke Google & memverifikasi identitas akun...');
          const oauthRes = await api.providers.oauthExchange(targetExistingProvider.id, {
            code: effectiveApiKey,
            egress_pool_id: selectedEgressPoolId || undefined,
          });
          toast.success(`Akun Google (${oauthRes.account_email || 'Antigravity'}) berhasil ditambahkan ke pool! Auto-refresh aktif.`);
        } else {
          setSavingStep('Menyimpan dan mengenkripsi kredensial (AES-256-GCM)...');
          const finalLabel = accountLabel.trim() || `Kredensial #${Date.now().toString().slice(-4)}`;
          await api.credentials.create(targetExistingProvider.id, {
            label: finalLabel,
            api_key: effectiveApiKey,
            egress_pool_id: selectedEgressPoolId || undefined,
          });
          toast.success(`Kredensial "${finalLabel}" berhasil ditambahkan ke pool ${targetExistingProvider.display_name || targetExistingProvider.name}!`);
        }

        if (syncAfterSave) {
          setSavingStep('Menyinkronkan model upstream...');
          try {
            await api.providers.syncModels(targetExistingProvider.id);
          } catch {
            // Non-blocking sync error
          }
        }

        await loadData();
        onClose();
        handleOpenDrawer(targetExistingProvider, 'credentials');
      } catch (err: unknown) {
        const errMsg = err instanceof Error ? err.message : String(err);
        toast.error('Gagal menambahkan kredensial ke pool: ' + errMsg);
      } finally {
        setIsSaving(false);
        setSavingStep('');
      }
    } else {
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
          if (authTab === 'oauth' || currentPreset?.authLoginType === 'oauth_fallback' || currentPreset?.id === 'antigravity') {
            setSavingStep('Menukarkan token OAuth ke Google & mengaktifkan auto-refresh worker...');
            try {
              const oauthRes = await api.providers.oauthExchange(created.id, {
                code: effectiveApiKey,
              });
              toast.success(`Akun Google (${oauthRes.account_email || 'Antigravity'}) berhasil dihubungkan! Auto-refresh aktif.`);
            } catch (oauthErr: unknown) {
              const errMsg = oauthErr instanceof Error ? oauthErr.message : String(oauthErr);
              toast.warn('Provider dibuat, namun autentikasi OAuth gagal ditukar: ' + errMsg);
            }
          } else {
            setSavingStep('Menyimpan dan mengenkripsi Kredensial (AES-256-GCM)...');
            try {
              const finalLabel = accountLabel.trim() || 'Primary API Key';
              await api.credentials.create(created.id, {
                label: finalLabel,
                api_key: effectiveApiKey,
              });
            } catch (keyErr: unknown) {
              const errMsg = keyErr instanceof Error ? keyErr.message : String(keyErr);
              toast.warn('Provider dibuat, namun kredensial gagal disimpan: ' + errMsg);
            }
          }
        }

        let pulledCount = 0;
        if (syncAfterSave && (effectiveApiKey || currentPreset?.authLoginType === 'local_socket')) {
          setSavingStep('Melakukan discovery & menarik model upstream...');
          try {
            const syncRes = await api.providers.syncModels(created.id);
            pulledCount = syncRes.count;
          } catch (err: unknown) {
            const errMsg = err instanceof Error ? err.message : String(err);
            toast.warn('Provider dibuat, tetapi sinkronisasi model awal gagal: ' + errMsg);
          }
        }

        await loadData();
        onClose();

        if (pulledCount > 0) {
          toast.success(`Provider "${created.display_name || created.name}" berhasil didaftarkan (${pulledCount} model ditarik)`);
        } else {
          toast.success(`Provider "${created.display_name || created.name}" berhasil didaftarkan`);
        }

        handleOpenDrawer(created, effectiveApiKey ? 'credentials' : 'models');
      } catch (err: unknown) {
        const errMsg = err instanceof Error ? err.message : String(err);
        toast.error('Gagal mendaftarkan provider: ' + errMsg);
      } finally {
        setIsSaving(false);
        setSavingStep('');
      }
    }
  };

  const isAddToPoolMode = modalMode === 'add_to_pool' && !!targetExistingProvider;

  const modalTitle = isAddToPoolMode
    ? `Hubungkan Akun ke Pool ${targetExistingProvider.display_name || targetExistingProvider.name}`
    : currentPreset
    ? `Hubungkan ${currentPreset.displayName}`
    : 'Tambah Provider AI Manual';

  // Quick preset cards for 1-click selection inside modal
  const popularPresets = KNOWN_PROVIDERS.filter((p) =>
    ['openai', 'anthropic', 'google', 'deepseek', 'groq', 'ollama', 'antigravity'].includes(p.id)
  );

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={modalTitle}
      maxWidth="xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} className="min-h-[44px] sm:min-h-[38px]">
            Batal
          </Button>
          <Button
            type="submit"
            variant="primary"
            form="create-provider-form"
            isLoading={isSaving}
            icon={isAddToPoolMode ? <Plus className="w-4 h-4" aria-hidden="true" /> : <Check className="w-4 h-4" aria-hidden="true" />}
            title={isAddToPoolMode ? 'Tambahkan akun ke pool provider' : 'Simpan provider baru ke database'}
            className="min-h-[44px] sm:min-h-[38px] font-semibold"
          >
            {isAddToPoolMode ? 'Tambahkan ke Pool' : 'Daftarkan Provider'}
          </Button>
        </>
      }
    >
      <form id="create-provider-form" noValidate onSubmit={handleSubmit} className="space-y-4 text-xs">
        {/* 1-KLIK PRESET CARDS SELECTOR */}
        <div className="rounded-xl border border-border/80 bg-bg-surface-2/40 p-3 space-y-2.5">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-semibold text-text-secondary flex items-center gap-1.5 uppercase tracking-wide">
              <Sparkles className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
              Pilih Preset Resmi 1-Klik
            </span>
            <button
              type="button"
              onClick={() => setShowPresetPicker(!showPresetPicker)}
              className="text-[11px] text-accent hover:underline cursor-pointer flex items-center gap-1"
              aria-label={showPresetPicker ? 'Sembunyikan preset' : 'Lihat semua preset'}
            >
              <span>{showPresetPicker ? 'Kecilkan' : 'Lihat Preset'}</span>
              {showPresetPicker ? <ChevronUp className="w-3 h-3" aria-hidden="true" /> : <ChevronDown className="w-3 h-3" aria-hidden="true" />}
            </button>
          </div>

          {/* Quick preset chips bar */}
          <div className="grid grid-cols-4 sm:grid-cols-7 gap-1.5">
            {popularPresets.map((preset) => {
              const isSelected = currentPreset?.id === preset.id;
              return (
                <button
                  key={preset.id}
                  type="button"
                  onClick={() => setupPreset(preset)}
                  className={`p-2 rounded-lg border flex flex-col items-center justify-center text-center gap-1 transition-all cursor-pointer min-h-[50px] ${
                    isSelected
                      ? 'bg-accent/15 border-accent text-white shadow-sm ring-1 ring-accent'
                      : 'bg-bg-surface border-border/70 text-text-muted hover:border-accent/50 hover:text-white'
                  }`}
                  aria-label={`Pilih preset ${preset.displayName}`}
                >
                  <ProviderBrandIcon providerIdOrKind={preset.kind} name={preset.name} className="w-4 h-4 shrink-0" />
                  <span className="text-[10px] font-medium leading-tight truncate w-full">{preset.displayName}</span>
                </button>
              );
            })}
          </div>

          {showPresetPicker && (
            <div className="pt-2 border-t border-border/50 grid grid-cols-2 sm:grid-cols-4 gap-1.5 max-h-36 overflow-y-auto scrollbar-thin">
              {KNOWN_PROVIDERS.filter((p) => !popularPresets.some((pop) => pop.id === p.id)).map((preset) => (
                <button
                  key={preset.id}
                  type="button"
                  onClick={() => setupPreset(preset)}
                  className={`p-1.5 rounded-lg border text-left flex items-center gap-2 transition-all cursor-pointer ${
                    currentPreset?.id === preset.id
                      ? 'bg-accent/15 border-accent text-white'
                      : 'bg-bg-surface border-border/60 text-text-secondary hover:text-white hover:border-border'
                  }`}
                  aria-label={`Pilih preset ${preset.displayName}`}
                >
                  <ProviderBrandIcon providerIdOrKind={preset.kind} name={preset.name} className="w-3.5 h-3.5 shrink-0" />
                  <span className="text-[11px] font-medium truncate">{preset.displayName}</span>
                </button>
              ))}
              <button
                type="button"
                onClick={() => setupPreset(null)}
                className={`p-1.5 rounded-lg border text-left flex items-center gap-2 transition-all cursor-pointer ${
                  currentPreset === null
                    ? 'bg-purple-950/40 border-purple-500 text-purple-200'
                    : 'bg-bg-surface border-border/60 text-text-muted hover:text-white'
                }`}
                aria-label="Pilih Custom Manual Provider"
              >
                <Server className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
                <span className="text-[11px] font-medium truncate">Custom Manual</span>
              </button>
            </div>
          )}
        </div>

        {/* Preset Brand Banner */}
        {currentPreset && (
          <div
            className="p-3 rounded-xl border flex items-center justify-between gap-3 shadow-inner"
            style={{
              backgroundColor: currentPreset.bgColor,
              borderColor: currentPreset.borderColor,
            }}
          >
            <div className="flex items-center gap-3">
              <div
                className="w-9 h-9 rounded-lg flex items-center justify-center shadow-sm"
                style={{
                  backgroundColor: currentPreset.color,
                  color: '#000',
                }}
              >
                <ProviderBrandIcon providerIdOrKind={currentPreset.id} className="w-5 h-5 text-white" />
              </div>
              <div>
                <h4 className="font-bold text-white text-sm">{currentPreset.displayName}</h4>
                <span className="text-[11px] text-text-secondary font-mono">
                  Dialek: {currentPreset.kind} • {currentPreset.tag}
                </span>
              </div>
            </div>

            {currentPreset.highlightModels && currentPreset.highlightModels.length > 0 && (
              <div className="hidden sm:block text-right">
                <span className="text-[10px] text-text-muted block font-mono uppercase">Highlight Model:</span>
                <span className="text-[11px] font-mono text-white font-semibold">
                  {currentPreset.highlightModels.slice(0, 2).join(', ')}
                </span>
              </div>
            )}
          </div>
        )}

        {/* BANNER MODE 1: Add to Pool Terdeteksi */}
        {isAddToPoolMode && (
          <div className="p-3.5 rounded-xl border border-accent/40 bg-accent/10 space-y-2">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 min-w-0">
                <Users className="w-4 h-4 text-accent shrink-0" aria-hidden="true" />
                <span className="font-bold text-white text-xs truncate">
                  Provider "{targetExistingProvider.display_name || targetExistingProvider.name}" Sudah Terdaftar
                </span>
              </div>
              <button
                type="button"
                onClick={switchToCreateNew}
                className="text-[11px] text-accent hover:underline font-medium shrink-0 cursor-pointer min-h-[32px] flex items-center"
                title="Buat entitas provider baru terpisah di luar pool ini"
                aria-label="Buat entitas provider baru terpisah"
              >
                + Buat Provider Baru Terpisah
              </button>
            </div>
            <p className="text-[11px] text-text-secondary leading-relaxed">
              Akun baru akan otomatis digabungkan ke pool multi-account{' '}
              <strong className="text-white">{targetExistingProvider.display_name || targetExistingProvider.name}</strong>{' '}
              dengan strategi rotasi{' '}
              <strong className="text-accent font-mono">
                {targetExistingProvider.credential_strategy === 'priority'
                  ? 'Priority Failover (Utama → Cadangan)'
                  : 'Round Robin (Load Balanced 50:50)'}
              </strong>. Gateway akan merotasi permintaan inferensi secara otomatis tanpa perlu mengubah konfigurasi klien.
            </p>
          </div>
        )}

        {/* BANNER MODE 2: Create New (Jika ada existing provider yang cocok) */}
        {!isAddToPoolMode && targetExistingProvider && (
          <div className="p-2.5 rounded-lg bg-bg-surface-2 border border-border flex items-center justify-between gap-2 text-xs">
            <span className="text-text-muted truncate">
              Provider sudah ada di sistem ({targetExistingProvider.display_name || targetExistingProvider.name}).
            </span>
            <button
              type="button"
              onClick={switchAddToPool}
              className="text-accent hover:underline font-semibold flex items-center gap-1 shrink-0 cursor-pointer min-h-[32px]"
              aria-label="Gabungkan ke pool provider yang sudah ada"
            >
              <RotateCcw className="w-3 h-3" aria-hidden="true" />
              Gabungkan ke Pool Saja
            </button>
          </div>
        )}

        {/* FIELD REGISTRASI LENGKAP: Hanya tampil jika mode create_new */}
        {!isAddToPoolMode && (
          <>
            {!currentPreset ? (
              <>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="provider-unique-id">ID Unik *</label>
                    <input
                      id="provider-unique-id"
                      type="text"
                      required
                      placeholder="nama-provider-unik"
                      value={newProv.name}
                      onChange={(e) => setNewProv({ ...newProv, name: e.target.value })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="provider-display-name">Nama Tampilan *</label>
                    <input
                      id="provider-display-name"
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
                    <Select
                      label="Kind"
                      value={newProv.kind}
                      onChange={(val) => setNewProv({ ...newProv, kind: val })}
                      options={[
                        { value: 'openai', label: 'OpenAI', description: 'Model keluarga GPT & reasoning o-series' },
                        { value: 'anthropic', label: 'Anthropic', description: 'Model Claude 3.5 & 3.7 series' },
                        { value: 'google', label: 'Google Gemini', description: 'Model Gemini 1.5, 2.0 & Flash series' },
                        { value: 'openai_compatible', label: 'OpenAI Compatible', description: 'Groq, DeepSeek, Together, Ollama, dll.' },
                        { value: 'custom', label: 'Custom Engine', description: 'Format payload proprietary/internal' },
                      ]}
                    />
                  </div>
                  <div className="col-span-2">
                    <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="provider-base-url">Base URL *</label>
                    <input
                      id="provider-base-url"
                      type="text"
                      required
                      value={newProv.base_url}
                      onChange={(e) => setNewProv({ ...newProv, base_url: e.target.value })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                    />
                  </div>
                </div>
              </>
            ) : (
              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="provider-preset-label">Nama Label di Route-X</label>
                <input
                  id="provider-preset-label"
                  type="text"
                  required
                  placeholder={currentPreset.displayName}
                  value={newProv.display_name}
                  onChange={(e) => setNewProv({ ...newProv, display_name: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white text-xs focus:outline-none focus:border-accent"
                />
              </div>
            )}
          </>
        )}

        {/* TAB AUTENTIKASI: API KEY VS GOOGLE OAUTH */}
        <div className="rounded-xl border border-border bg-bg-surface-2/60 p-3.5 space-y-3">
          <div role="tablist" aria-label="Metode Autentikasi" className="flex border-b border-border/80 gap-2 pb-2 text-xs">
            <button
              type="button"
              role="tab"
              id="auth-tab-apikey"
              aria-selected={authTab === 'apikey'}
              aria-controls="auth-tabpanel-apikey"
              onClick={() => setAuthTab('apikey')}
              className={`pb-1 font-medium transition-colors flex items-center gap-1.5 min-h-[38px] px-2 rounded-t cursor-pointer ${
                authTab === 'apikey'
                  ? 'border-b-2 border-accent text-accent font-bold bg-accent/5'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <KeyRound className="w-3.5 h-3.5" aria-hidden="true" />
              API Key &amp; Token
            </button>
            <button
              type="button"
              role="tab"
              id="auth-tab-oauth"
              aria-selected={authTab === 'oauth'}
              aria-controls="auth-tabpanel-oauth"
              onClick={() => setAuthTab('oauth')}
              className={`pb-1 font-medium transition-colors flex items-center gap-1.5 min-h-[38px] px-2 rounded-t cursor-pointer ${
                authTab === 'oauth'
                  ? 'border-b-2 border-accent text-accent font-bold bg-accent/5'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <Shield className="w-3.5 h-3.5" aria-hidden="true" />
              Google OAuth / Antigravity
            </button>
          </div>

          {/* TAB 1: API KEY */}
          {authTab === 'apikey' && (
            <div id="auth-tabpanel-apikey" role="tabpanel" aria-labelledby="auth-tab-apikey" className="space-y-3 pt-1">
              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="provider-account-label">
                  Nama Akun / Label Kredensial *
                </label>
                <input
                  id="provider-account-label"
                  type="text"
                  required
                  placeholder={
                    currentPreset?.accountLabelPlaceholder || 'mis. Akun Tim Kantor, Key Cadangan, Akun Tier-2'
                  }
                  value={accountLabel}
                  onChange={(e) => setAccountLabel(e.target.value)}
                  className="w-full px-3 py-2 bg-bg-surface border border-border rounded-nav text-white text-xs focus:outline-none focus:border-accent"
                />
                <p className="text-[10px] text-text-muted mt-1">
                  Beri nama deskriptif untuk membedakan akun ini dalam rotasi pool Route-X.
                </p>
              </div>

              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <label htmlFor="quick-api-key" className="block font-bold text-white text-xs">
                    Secret API Key *
                  </label>
                  {currentPreset?.apiKeyHelp && (
                    <span className="text-[10px] text-accent font-mono flex items-center gap-1">
                      <ExternalLink className="w-2.5 h-2.5" aria-hidden="true" />
                      {currentPreset.apiKeyHelp}
                    </span>
                  )}
                </div>

                <div className="relative">
                  <input
                    id="quick-api-key"
                    type={showApiKey ? 'text' : 'password'}
                    required
                    placeholder={currentPreset?.apiKeyPlaceholder || 'sk-... atau Bearer Token'}
                    value={quickApiKey}
                    onChange={(e) => setQuickApiKey(e.target.value)}
                    className="w-full px-3 py-2 pr-10 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={() => setShowApiKey(!showApiKey)}
                    aria-label={showApiKey ? "Sembunyikan kunci API" : "Tampilkan kunci API"}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white cursor-pointer min-h-[36px] min-w-[36px] flex items-center justify-center"
                  >
                    {showApiKey ? <EyeOff className="w-4 h-4" aria-hidden="true" /> : <Eye className="w-4 h-4" aria-hidden="true" />}
                  </button>
                </div>
                <p className="text-[11px] text-text-muted flex items-center gap-1.5">
                  <Lock className="w-3.5 h-3.5 text-accent flex-shrink-0" aria-hidden="true" />
                  Dienkripsi amplop AES-256-GCM tingkat record PostgreSQL dengan AAD.
                </p>
              </div>

              {/* Local Socket Zero-Config Hint */}
              {(currentPreset?.authLoginType === 'local_socket' || (!currentPreset && (newProv.base_url.includes('localhost') || newProv.base_url.includes('127.0.0.1')))) && (
                <div className="pt-2 border-t border-border/50 space-y-2">
                  <div className="flex items-center gap-2 text-white font-semibold text-xs">
                    <Server className="w-4 h-4 text-accent" aria-hidden="true" />
                    <span>Socket Server Lokal (Zero-Config)</span>
                  </div>
                  <p className="text-[11px] text-text-muted">
                    {currentPreset?.authInstructions ||
                      'Provider lokal tidak memerlukan API key eksternal. Pastikan server lokal aktif di port yang sesuai.'}
                  </p>
                  <div className="pt-1 flex flex-wrap items-center gap-2">
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={handleTestSocket}
                      isLoading={socketPingStatus?.testing}
                      icon={<Activity className="w-3.5 h-3.5 text-accent" aria-hidden="true" />}
                      className="min-h-[38px] text-xs"
                      aria-label="Uji respons socket server lokal"
                    >
                      Uji Respons Socket ({newProv.base_url})
                    </Button>
                    {socketPingStatus && !socketPingStatus.testing && (
                      <span
                        className={`text-xs font-mono font-semibold flex items-center gap-1 ${
                          socketPingStatus.ok ? 'text-emerald-400' : 'text-red-400'
                        }`}
                      >
                        {socketPingStatus.ok ? <CheckCircle2 className="w-3.5 h-3.5" aria-hidden="true" /> : <XCircle className="w-3.5 h-3.5" aria-hidden="true" />}
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

          {/* TAB 2: GOOGLE OAUTH */}
          {authTab === 'oauth' && (
            <div id="auth-tabpanel-oauth" role="tabpanel" aria-labelledby="auth-tab-oauth" className="space-y-3 pt-1">
              <div className="p-2.5 rounded-lg bg-accent/10 border border-accent/20 text-xs text-text-secondary flex items-start gap-2">
                <Shield className="w-4 h-4 text-accent shrink-0 mt-0.5" aria-hidden="true" />
                <div>
                  <span className="font-semibold text-white block">OAuth Multi-Account Auto-Refresh</span>
                  <span className="text-[11px] text-text-muted">
                    Identitas Nama &amp; Email Akun Google akan dideteksi dan diverifikasi secara otomatis oleh Route-X melalui token exchange. Tanpa konfigurasi manual Client ID / Secret.
                  </span>
                </div>
              </div>

              <div className="space-y-3">
                <div className="flex items-start gap-2.5 text-xs text-text-secondary">
                  <span className="w-5 h-5 rounded-full bg-accent/20 text-accent font-bold flex items-center justify-center flex-shrink-0 text-[11px]">
                    1
                  </span>
                  <div>
                    <p className="font-semibold text-white">Buka Halaman Otorisasi Google</p>
                    <p className="text-[11px] text-text-muted mt-0.5">
                      {currentPreset?.authInstructions ||
                        'Klik tombol untuk membuka halaman persetujuan otorisasi Google di tab browser baru.'}
                    </p>
                    <a
                      href={
                        currentPreset?.authLoginUrl ||
                        'https://accounts.google.com/o/oauth2/v2/auth?client_id=1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com&redirect_uri=http://localhost:4567&response_type=code&scope=https://www.googleapis.com/auth/cloud-platform%20https://www.googleapis.com/auth/userinfo.email%20https://www.googleapis.com/auth/userinfo.profile%20openid&access_type=offline&prompt=consent'
                      }
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1.5 mt-2 px-3 py-2 rounded-lg bg-accent text-black font-semibold text-xs hover:opacity-90 transition-opacity min-h-[38px]"
                      aria-label="Login dengan Akun Google Otorisasi"
                    >
                      <span>{currentPreset?.authLoginLabel || 'Login dengan Akun Google'}</span>
                      <ArrowUpRight className="w-3.5 h-3.5" aria-hidden="true" />
                    </a>
                  </div>
                </div>

                <div className="flex items-start gap-2.5 text-xs text-text-secondary pt-2 border-t border-border/50">
                  <span className="w-5 h-5 rounded-full bg-accent/20 text-accent font-bold flex items-center justify-center flex-shrink-0 text-[11px]">
                    2
                  </span>
                  <div className="flex-1 space-y-1.5">
                    <label className="block font-semibold text-white" htmlFor="oauth-fallback-input">
                      Salin URL Redirect / Kode Otorisasi:
                    </label>
                    <textarea
                      id="oauth-fallback-input"
                      rows={2}
                      value={authFallbackInput}
                      onChange={(e) => handleFallbackInputChange(e.target.value)}
                      placeholder="Tempel seluruh URL redirect (misal: http://localhost:4567/?code=4/0A...) atau kode otorisasi di sini..."
                      className="w-full px-3 py-2 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                    />
                    {authExtractedToken && (
                      <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                        <Check className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
                        <span>
                          Kode Otorisasi Terdeteksi ({authExtractionHint}):{' '}
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
            </div>
          )}

          {/* Jalur Keluar (Egress Proxy) */}
          <div className="pt-2.5 border-t border-border/50">
            <label className="block text-xs font-medium text-text-secondary mb-1.5 flex items-center gap-1.5">
              <Globe className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
              Jalur Keluar (Egress Proxy) Opsional
            </label>
            <Select
              value={selectedEgressPoolId}
              onChange={(val) => setSelectedEgressPoolId(val)}
              placeholder="Ikuti Provider / Direct Outbound (Default)"
              options={[
                { value: '', label: 'Ikuti Provider / Direct (Default)', description: 'Koneksi langsung dari server atau default pool provider' },
                ...egressPools.map((ep) => ({
                  value: ep.id,
                  label: `${ep.name} (${ep.kind.toUpperCase()})`,
                  description: `Protokol: ${ep.kind} · Region: ${ep.region || 'Default'}`,
                })),
              ]}
            />
          </div>
        </div>

        {(effectiveApiKey || currentPreset?.authLoginType === 'local_socket') && (
          <div className="pt-1">
            <Checkbox
              id="syncModelsToggle"
              checked={syncAfterSave}
              onChange={(e) => setSyncAfterSave(e.target.checked)}
              label="Tarik model upstream otomatis setelah disimpan (Rekomendasi)"
            />
          </div>
        )}

        {/* Pengaturan Lanjutan: Hanya di mode create_new */}
        {!isAddToPoolMode && currentPreset && (
          <div className="pt-2 border-t border-border/60">
            <button
              type="button"
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="inline-flex items-center gap-1.5 text-xs text-text-muted hover:text-accent font-medium transition-colors py-1 cursor-pointer min-h-[36px]"
              aria-label={showAdvanced ? 'Sembunyikan Pengaturan Lanjutan' : 'Tampilkan Pengaturan Lanjutan'}
            >
              {showAdvanced ? <ChevronUp className="w-3.5 h-3.5 text-accent" aria-hidden="true" /> : <ChevronDown className="w-3.5 h-3.5 text-accent" aria-hidden="true" />}
              <span>{showAdvanced ? 'Sembunyikan Pengaturan Lanjutan' : '⚙️ Pengaturan Lanjutan (ID Unik, Base URL, Egress Proxy)'}</span>
            </button>
            {showAdvanced && (
              <div className="mt-2.5 p-3.5 rounded-xl border border-border/80 bg-bg-surface-2/40 space-y-3">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="adv-provider-name">ID Unik *</label>
                    <input
                      id="adv-provider-name"
                      type="text"
                      required
                      placeholder="nama-provider-unik"
                      value={newProv.name}
                      onChange={(e) => setNewProv({ ...newProv, name: e.target.value })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="adv-provider-baseurl">Base URL *</label>
                    <input
                      id="adv-provider-baseurl"
                      type="text"
                      required
                      value={newProv.base_url}
                      onChange={(e) => setNewProv({ ...newProv, base_url: e.target.value })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                    />
                  </div>
                </div>
              </div>
            )}
          </div>
        )}

        {isSaving && savingStep && (
          <div className="p-3 rounded-lg bg-accent/10 border border-accent/20 text-accent text-xs flex items-center gap-2 animate-pulse" role="status">
            <RefreshCw className="w-4 h-4 animate-spin flex-shrink-0" aria-hidden="true" />
            <span>{savingStep}</span>
          </div>
        )}
      </form>
    </Modal>
  );
};
