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
} from 'lucide-react';
import {
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
  selectedPreset,
  providers = [],
  egressPools = [],
  loadData,
  toast,
  handleOpenDrawer,
  api,
  targetProviderOverride,
}) => {
  const [modalMode, setModalMode] = useState<'add_to_pool' | 'create_new'>('create_new');
  const [targetExistingProvider, setTargetExistingProvider] = useState<Provider | null>(null);
  const [accountLabel, setAccountLabel] = useState('');

  const [authTab, setAuthTab] = useState<'authlogin' | 'apikey'>('apikey');
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

      const matched = targetProviderOverride || matchExistingProvider(selectedPreset, providers);

      if (matched) {
        setTargetExistingProvider(matched);
        setModalMode('add_to_pool');
        setSyncAfterSave(true);
        setShowAdvanced(false);

        const isOauth = selectedPreset?.authLoginType === 'oauth_fallback' || matched.kind === 'google';
        setAuthTab(isOauth ? 'authlogin' : 'apikey');
        setAccountLabel(
          isOauth
            ? ''
            : selectedPreset?.accountLabelPlaceholder
            ? ''
            : 'Akun Tambahan'
        );
      } else if (selectedPreset) {
        setTargetExistingProvider(null);
        setModalMode('create_new');
        setSyncAfterSave(true);
        setShowAdvanced(false);
        setAuthTab(selectedPreset.authLoginType ? 'authlogin' : 'apikey');
        setAccountLabel('Akun Utama');
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
        setTargetExistingProvider(null);
        setModalMode('create_new');
        setSyncAfterSave(false);
        setShowAdvanced(true);
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
    }
  }, [isOpen, selectedPreset, providers, targetProviderOverride]);

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
    if (selectedPreset) {
      const existingCount = (providers || []).filter(
        (p) => p.kind === selectedPreset.kind || p.name === selectedPreset.name || p.name.startsWith(selectedPreset.name + '-')
      ).length;
      const uniqueName = existingCount > 0 ? `${selectedPreset.name}-${existingCount + 1}` : selectedPreset.name;
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
      setAccountLabel('Akun Utama');
    }
  };

  const switchAddToPool = () => {
    if (targetExistingProvider) {
      setModalMode('add_to_pool');
      const isOauth = selectedPreset?.authLoginType === 'oauth_fallback' || targetExistingProvider.kind === 'google';
      setAuthTab(isOauth ? 'authlogin' : 'apikey');
      setAccountLabel(
        isOauth
          ? ''
          : selectedPreset?.accountLabelPlaceholder
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

        const isOauth = selectedPreset?.authLoginType === 'oauth_fallback' || targetExistingProvider.kind === 'google';

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
          if (selectedPreset?.authLoginType === 'oauth_fallback' || selectedPreset?.id === 'antigravity') {
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
              const finalLabel = accountLabel.trim() || (authTab === 'authlogin' ? 'Auth Login Credential' : 'Primary API Key');
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
        if (syncAfterSave && (effectiveApiKey || selectedPreset?.authLoginType === 'local_socket')) {
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
    : selectedPreset
    ? `Hubungkan ${selectedPreset.displayName}`
    : 'Tambah Provider AI Manual';

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={modalTitle}
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
            icon={isAddToPoolMode ? <Plus className="w-4 h-4" /> : <Check className="w-4 h-4" />}
            title={isAddToPoolMode ? 'Tambahkan akun ke pool provider' : 'Simpan provider baru ke database'}
          >
            {isAddToPoolMode ? 'Tambahkan ke Pool' : 'Daftarkan'}
          </Button>
        </>
      }
    >
      <form id="create-provider-form" noValidate onSubmit={handleSubmit} className="space-y-4 text-xs">
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

        {/* BANNER MODE 1: Add to Pool Terdeteksi */}
        {isAddToPoolMode && (
          <div className="p-3.5 rounded-xl border border-accent/40 bg-accent/10 space-y-2">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 min-w-0">
                <Users className="w-4 h-4 text-accent shrink-0" />
                <span className="font-bold text-white text-xs truncate">
                  Provider "{targetExistingProvider.display_name || targetExistingProvider.name}" Sudah Terdaftar
                </span>
              </div>
              <button
                type="button"
                onClick={switchToCreateNew}
                className="text-[11px] text-accent hover:underline font-medium shrink-0 cursor-pointer"
                title="Buat entitas provider baru terpisah di luar pool ini"
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
              className="text-accent hover:underline font-semibold flex items-center gap-1 shrink-0 cursor-pointer"
            >
              <RotateCcw className="w-3 h-3" />
              Gabungkan ke Pool Saja
            </button>
          </div>
        )}

        {/* FIELD REGISTRASI LENGKAP: Hanya tampil jika mode create_new */}
        {!isAddToPoolMode && (
          <>
            {!selectedPreset ? (
              <>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-text-secondary mb-1.5">ID Unik *</label>
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
                    <label className="block text-xs font-medium text-text-secondary mb-1.5">Base URL *</label>
                    <input
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
                <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Label di Route-X</label>
                <input
                  type="text"
                  required
                  placeholder={selectedPreset.displayName}
                  value={newProv.display_name}
                  onChange={(e) => setNewProv({ ...newProv, display_name: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white text-xs focus:outline-none focus:border-accent"
                />
              </div>
            )}
          </>
        )}

        {/* INPUT AKUN & KREDENSIAL: KONDISI ADD_TO_POOL VS CREATE_NEW */}
        {isAddToPoolMode ? (
          /* Form Ringkas Add to Pool */
          <div className="rounded-xl border border-border bg-bg-surface-2/60 p-3.5 space-y-3">
            {/* Opsi A: OAuth (Google Antigravity / Google Kind) */}
            {(selectedPreset?.authLoginType === 'oauth_fallback' || targetExistingProvider.kind === 'google') ? (
              <div className="space-y-3">
                <div className="p-2.5 rounded-lg bg-accent/10 border border-accent/20 text-xs text-text-secondary flex items-start gap-2">
                  <Shield className="w-4 h-4 text-accent shrink-0 mt-0.5" />
                  <div>
                    <span className="font-semibold text-white block">OAuth Multi-Account Auto-Refresh</span>
                    <span className="text-[11px] text-text-muted">
                      Identitas Nama & Email Akun Google akan dideteksi dan diverifikasi secara otomatis oleh Route-X melalui token exchange.
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
                        {selectedPreset?.authInstructions || 'Login dengan akun Google yang ingin ditambahkan ke pool.'}
                      </p>
                      {selectedPreset?.authLoginUrl && (
                        <a
                          href={selectedPreset.authLoginUrl}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1.5 mt-2 px-3 py-1.5 rounded-lg bg-accent text-black font-semibold text-xs hover:opacity-90 transition-opacity"
                        >
                          <span>{selectedPreset.authLoginLabel || 'Login Akun Google'}</span>
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
                        Salin URL Redirect / Kode Otorisasi:
                      </label>
                      <textarea
                        rows={2}
                        value={authFallbackInput}
                        onChange={(e) => handleFallbackInputChange(e.target.value)}
                        placeholder="Tempel seluruh URL redirect (misal: http://localhost:4567/?code=4/0A...) atau kode otorisasi di sini..."
                        className="w-full px-3 py-2 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                      />
                      {authExtractedToken && (
                        <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                          <Check className="w-3.5 h-3.5 flex-shrink-0" />
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
            ) : (
              /* Opsi B: API Key / Token Standar */
              <div className="space-y-3">
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1.5">
                    Nama Akun / Label Kredensial *
                  </label>
                  <input
                    type="text"
                    required
                    placeholder={
                      selectedPreset?.accountLabelPlaceholder || 'mis. Akun Tim Kantor, Key Cadangan, Akun Tier-2'
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
                    {selectedPreset?.apiKeyHelp && (
                      <span className="text-[10px] text-accent font-mono flex items-center gap-1">
                        <ExternalLink className="w-2.5 h-2.5" aria-hidden="true" />
                        {selectedPreset.apiKeyHelp}
                      </span>
                    )}
                  </div>

                  <div className="relative">
                    <input
                      id="quick-api-key"
                      type={showApiKey ? 'text' : 'password'}
                      required
                      placeholder={selectedPreset?.apiKeyPlaceholder || 'sk-... atau Bearer Token'}
                      value={quickApiKey}
                      onChange={(e) => setQuickApiKey(e.target.value)}
                      className="w-full px-3 py-2 pr-10 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs focus:border-accent focus:outline-none"
                    />
                    <button
                      type="button"
                      onClick={() => setShowApiKey(!showApiKey)}
                      aria-label={showApiKey ? "Sembunyikan kunci API" : "Tampilkan kunci API"}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white cursor-pointer"
                    >
                      {showApiKey ? <EyeOff className="w-4 h-4" aria-hidden="true" /> : <Eye className="w-4 h-4" aria-hidden="true" />}
                    </button>
                  </div>
                  <p className="text-[11px] text-text-muted flex items-center gap-1.5">
                    <Lock className="w-3.5 h-3.5 text-accent flex-shrink-0" aria-hidden="true" />
                    Dienkripsi amplop AES-256-GCM tingkat record PostgreSQL dengan AAD.
                  </p>
                </div>
              </div>
            )}

            {/* Jalur Keluar (Proxy) Opsional untuk Akun Tambahan */}
            <div className="pt-2.5 border-t border-border/50">
              <label className="block text-xs font-medium text-text-secondary mb-1.5 flex items-center gap-1.5">
                <Globe className="w-3.5 h-3.5 text-accent" />
                Jalur Keluar (Egress Proxy) Opsional
              </label>
              <Select
                value={selectedEgressPoolId}
                onChange={(val) => setSelectedEgressPoolId(val)}
                placeholder="Ikuti Provider / Direct (Default)"
                options={[
                  { value: '', label: 'Ikuti Provider / Direct (Default)', description: 'Koneksi langsung dari server atau default pool provider' },
                  ...egressPools.map((ep) => ({
                    value: ep.id,
                    label: `${ep.name} (${ep.kind.toUpperCase()})`,
                    description: `Protokol: ${ep.kind} · Region: ${ep.region || 'Default'}`,
                  })),
                ]}
              />
              <p className="text-[10px] text-text-muted mt-1">
                Opsional: Akun ini akan memiliki jalur keluar proxy sendiri yang terisolasi dari akun lainnya.
              </p>
            </div>
          </div>
        ) : (
          /* Form Standar Create New Provider */
          <div className="rounded-xl border border-border bg-bg-surface-2/60 p-3.5 space-y-3">
            {/* Dual-Auth Tab Switcher: Hanya untuk Provider Resmi NON-Antigravity */}
            {selectedPreset?.id !== 'antigravity' && selectedPreset !== null && (
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
            )}

            {/* Banner Khusus Google Antigravity */}
            {selectedPreset?.id === 'antigravity' && (
              <div className="p-2.5 rounded-lg bg-accent/10 border border-accent/20 text-xs text-text-secondary flex items-start gap-2">
                <Shield className="w-4 h-4 text-accent shrink-0 mt-0.5" />
                <div>
                  <span className="font-semibold text-white block">OAuth Token Exchange Engine</span>
                  <span className="text-[11px] text-text-muted">
                    Google Antigravity menggunakan autentikasi akun Google terdaftar dengan multi-account pooling & auto-refresh token otomatis. Cukup login dan salin seluruh URL redirect/callback (atau kode otorisasi) ke kolom di bawah. Tanpa perlu mengisi Client ID/Secret manual.
                  </span>
                </div>
              </div>
            )}

            {/* TAB 1: AUTH LOGIN TERPANDU */}
            {authTab === 'authlogin' && (
              <div className="space-y-3 pt-1">
                {/* Alur A: Redirect Callback URL Copying */}
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
              <div className="space-y-2.5 pt-1">
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">
                    Nama Akun / Label Kredensial
                  </label>
                  <input
                    type="text"
                    placeholder={selectedPreset?.accountLabelPlaceholder || 'mis. Akun Utama, Primary API Key'}
                    value={accountLabel}
                    onChange={(e) => setAccountLabel(e.target.value)}
                    className="w-full px-3 py-2 bg-bg-surface border border-border rounded-lg text-white text-xs focus:outline-none focus:border-accent"
                  />
                </div>

                <div className="space-y-1">
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
                  <p className="text-[11px] text-text-muted flex items-center gap-1.5">
                    <Lock className="w-3.5 h-3.5 text-accent flex-shrink-0" />
                    Dienkripsi amplop AES-256-GCM tingkat record PostgreSQL dengan AAD.
                  </p>
                </div>
              </div>
            )}
          </div>
        )}

        {(effectiveApiKey || selectedPreset?.authLoginType === 'local_socket') && (
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
        {!isAddToPoolMode && selectedPreset && (
          <div className="pt-2 border-t border-border/60">
            <button
              type="button"
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="inline-flex items-center gap-1.5 text-xs text-text-muted hover:text-accent font-medium transition-colors py-1 cursor-pointer"
            >
              {showAdvanced ? <ChevronUp className="w-3.5 h-3.5 text-accent" /> : <ChevronDown className="w-3.5 h-3.5 text-accent" />}
              <span>{showAdvanced ? 'Sembunyikan Pengaturan Lanjutan' : '⚙️ Pengaturan Lanjutan (ID Unik, Base URL, Egress Proxy)'}</span>
            </button>
            {showAdvanced && (
              <div className="mt-2.5 p-3.5 rounded-xl border border-border/80 bg-bg-surface-2/40 space-y-3">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-text-secondary mb-1.5">ID Unik *</label>
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
                    <label className="block text-xs font-medium text-text-secondary mb-1.5">Base URL *</label>
                    <input
                      type="text"
                      required
                      value={newProv.base_url}
                      onChange={(e) => setNewProv({ ...newProv, base_url: e.target.value })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                    />
                  </div>
                </div>
                <div>
                  <Select
                    label="Jalur Egress Outbound (Opsional)"
                    value={selectedEgressPoolId}
                    onChange={(val) => setSelectedEgressPoolId(val)}
                    placeholder="Direct Outbound (Tanpa Proxy)"
                    options={[
                      { value: '', label: 'Direct Outbound (Tanpa Proxy)', description: 'Koneksi langsung dari server tanpa melalui proxy' },
                      ...egressPools.map((pool) => ({
                        value: pool.id,
                        label: `${pool.name} (${pool.kind})`,
                        description: `Protokol: ${pool.kind} · Region: ${pool.region || 'Default'}`,
                      })),
                    ]}
                  />
                </div>
              </div>
            )}
          </div>
        )}

        {!isAddToPoolMode && !selectedPreset && (
          <div>
            <Select
              label="Jalur Egress Outbound"
              value={selectedEgressPoolId}
              onChange={(val) => setSelectedEgressPoolId(val)}
              placeholder="Direct Outbound (Tanpa Proxy)"
              options={[
                { value: '', label: 'Direct Outbound (Tanpa Proxy)', description: 'Koneksi langsung dari server tanpa melalui proxy' },
                ...egressPools.map((pool) => ({
                  value: pool.id,
                  label: `${pool.name} (${pool.kind})`,
                  description: `Protokol: ${pool.kind} · Region: ${pool.region || 'Default'}`,
                })),
              ]}
            />
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
