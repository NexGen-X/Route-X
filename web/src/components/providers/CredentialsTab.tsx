import React, { useEffect, useState, useMemo, useRef } from 'react';
import type { Provider, Credential, EgressPool, OAuthSession } from '../../types';
import type { api as apiClient } from '../../api/client';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import {
  Plus,
  Power,
  RefreshCw,
  Trash2,
  Globe,
  Eye,
  EyeOff,
  Copy,
  Check,
  Shield,
  KeyRound,
  Sliders,
  ArrowUpRight,
  ExternalLink,
  Lock,
  Users,
  Layers,
  Clock,
  RotateCcw,
} from 'lucide-react';
import {
  type KnownProviderPreset,
} from './ProviderIcons';
import { matchPresetForProvider } from './providerMatching';
import { extractAuthTokenFromInput } from '../../utils/authExtractor';

export interface CredentialsTabProps {
  selectedProvider: Provider;
  credentials: Credential[];
  oauthSessions?: OAuthSession[];
  egressPools?: EgressPool[];
  handleToggleKey: (cred: Credential) => void | Promise<void>;
  handleDeleteKey: (cred: Credential) => void | Promise<void>;
  loadProviderDetails: (providerId: string) => Promise<void>;
  toast: {
    success: (msg: string) => void;
    error: (msg: string) => void;
    info?: (msg: string) => void;
  };
  confirmModal: (opts: {
    title: string;
    message: string;
    confirmText?: string;
    danger?: boolean;
  }) => Promise<boolean>;
  api: typeof apiClient;
  copyWithFeedback: (text: string, message: string, onCopied?: () => void) => Promise<void> | void;
  onUpdateProvider?: (updated: Provider) => void;
}

export const CredentialsTab: React.FC<CredentialsTabProps> = ({
  selectedProvider,
  credentials,
  oauthSessions = [],
  egressPools = [],
  handleToggleKey,
  handleDeleteKey,
  loadProviderDetails,
  toast,
  confirmModal,
  api,
  copyWithFeedback,
  onUpdateProvider,
}) => {
  const [isAddingKeyInline, setIsAddingKeyInline] = useState(false);
  const [inlineAuthTab, setInlineAuthTab] = useState<'apikey' | 'authlogin'>('apikey');
  const [newKeyForm, setNewKeyForm] = useState({ label: 'Primary API Key', api_key: '' });
  const [authFallbackInput, setAuthFallbackInput] = useState('');
  const [authExtractedToken, setAuthExtractedToken] = useState('');
  const [authExtractionHint, setAuthExtractionHint] = useState('');
  const [inlineEgressPoolId, setInlineEgressPoolId] = useState<string>('');
  const [showNewKeySecret, setShowNewKeySecret] = useState(false);
  const [isSavingKey, setIsSavingKey] = useState(false);
  const [copiedTokenId, setCopiedTokenId] = useState<string | null>(null);
  const [isUpdatingStrategy, setIsUpdatingStrategy] = useState(false);
  const [isRefreshingOAuth, setIsRefreshingOAuth] = useState<string | null>(null);
  const copyTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    return () => {
      copyTimersRef.current.forEach((t) => clearTimeout(t));
      copyTimersRef.current = [];
    };
  }, []);

  const matchedPreset = useMemo<KnownProviderPreset | null>(() => {
    return matchPresetForProvider(selectedProvider);
  }, [selectedProvider]);

  const isAntigravity =
    matchedPreset?.id === 'antigravity' ||
    (selectedProvider?.name || '').toLowerCase().includes('antigravity') ||
    (selectedProvider?.display_name || '').toLowerCase().includes('antigravity') ||
    (oauthSessions && oauthSessions.length > 0);
  const isCustomProvider = !matchedPreset;

  const directCredentials = useMemo(
    () => (credentials || []).filter(
      (c: Credential) => !c.label.startsWith('oauth:') && !(oauthSessions || []).some((s: OAuthSession) => s.credential_id === c.id)
    ),
    [credentials, oauthSessions]
  );

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
    setInlineAuthTab(isAntigravity ? 'authlogin' : 'apikey');
    setAuthFallbackInput('');
    setAuthExtractedToken('');
    setAuthExtractionHint('');
    setInlineEgressPoolId('');
    setNewKeyForm({
      label: isAntigravity
        ? 'Antigravity Session Token'
        : directCredentials.length === 0 ? 'Primary API Key' : `Akun Cadangan #${directCredentials.length + 1}`,
      api_key: '',
    });
  };

  const handleUpdateAccountEgress = async (credId: string, poolId: string | null) => {
    if (!selectedProvider) return;
    try {
      await api.credentials.setEgressPool(selectedProvider.id, credId, poolId);
      toast.success('Jalur keluar akun berhasil diperbarui');
      await loadProviderDetails(selectedProvider.id);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal mengubah jalur keluar: ' + errMsg);
    }
  };

  const handleStrategyChange = async (newStrategy: 'round_robin' | 'priority') => {
    if (!selectedProvider) return;
    setIsUpdatingStrategy(true);
    try {
      await api.providers.setCredentialStrategy(selectedProvider.id, newStrategy);
      toast.success(`Strategi antar-akun diubah ke: ${newStrategy === 'round_robin' ? 'Round Robin (Load Balanced)' : 'Priority Failover (Utama -> Cadangan)'}`);
      if (onUpdateProvider) {
        onUpdateProvider({ ...selectedProvider, credential_strategy: newStrategy });
      }
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal memperbarui strategi: ' + errMsg);
    } finally {
      setIsUpdatingStrategy(false);
    }
  };

  const handleRefreshOAuthSession = async (sessionId: string) => {
    if (!selectedProvider) return;
    setIsRefreshingOAuth(sessionId);
    try {
      await api.providers.refreshOAuth(selectedProvider.id, sessionId);
      toast.success('Token akun berhasil diperbarui oleh OAuth engine!');
      await loadProviderDetails(selectedProvider.id);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal merefresh token: ' + errMsg);
    } finally {
      setIsRefreshingOAuth(null);
    }
  };

  const handleToggleOAuthSession = async (session: OAuthSession) => {
    if (!selectedProvider) return;
    try {
      await api.providers.toggleOAuthSession(selectedProvider.id, session.id, !session.enabled);
      toast.success(`Akun ${session.account_email} ${!session.enabled ? 'diaktifkan' : 'dialihkan ke Standby'}`);
      await loadProviderDetails(selectedProvider.id);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal mengubah status akun: ' + errMsg);
    }
  };

  const handleDeleteOAuthSession = async (session: OAuthSession) => {
    if (!selectedProvider) return;
    const confirmed = await confirmModal({
      title: `Hapus Akun "${session.account_email}"?`,
      message: 'Sesi OAuth dan kredensial akses untuk akun ini akan dihapus dari pool. Gateway tidak akan lagi menggunakan akun ini.',
      confirmText: 'Hapus Akun',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.providers.deleteOAuthSession(selectedProvider.id, session.id);
      toast.success(`Akun ${session.account_email} berhasil dihapus dari pool`);
      await loadProviderDetails(selectedProvider.id);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal menghapus akun: ' + errMsg);
    }
  };

  const handleCreateKey = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;
    const keyLabel = newKeyForm.label.trim() || (isAntigravity ? 'Antigravity Session Token' : inlineAuthTab === 'authlogin' ? 'Auth Login Credential' : 'Primary API Key');
    const keySecret = inlineAuthTab === 'apikey'
      ? newKeyForm.api_key.trim()
      : (authExtractedToken.trim() || authFallbackInput.trim());

    if (!keySecret) {
      toast.error('Secret token atau URL redirect / kode otorisasi wajib diisi');
      return;
    }

    setIsSavingKey(true);
    try {
      if (isAntigravity) {
        const oauthRes = await api.providers.oauthExchange(selectedProvider.id, {
          code: keySecret,
          egress_pool_id: inlineEgressPoolId || undefined,
        });
        toast.success(`Akun Google (${oauthRes.account_email || 'Antigravity'}) berhasil dihubungkan ke pool! Auto-refresh aktif.`);
      } else {
        await api.credentials.create(selectedProvider.id, {
          label: keyLabel,
          api_key: keySecret,
          egress_pool_id: inlineEgressPoolId || undefined,
        });
        toast.success('Kredensial berhasil ditambahkan dengan enkripsi AES-256-GCM');
      }
      resetInlineForm();
      await loadProviderDetails(selectedProvider.id);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal menambahkan kredensial: ' + errMsg);
    } finally {
      setIsSavingKey(false);
    }
  };

  const formatExpiry = (expiresAtStr?: string) => {
    if (!expiresAtStr) return null;
    const expiresAt = new Date(expiresAtStr).getTime();
    const now = Date.now();
    const diffMs = expiresAt - now;
    if (diffMs <= 0) {
      return { text: 'Kedaluwarsa (Menunggu Auto-Refresh)', color: 'text-red-400 bg-red-500/10 border-red-500/30' };
    }
    const diffMins = Math.round(diffMs / 60000);
    if (diffMins <= 15) {
      return { text: `Kedaluwarsa dlm ${diffMins}m (Segera direfresh)`, color: 'text-amber-400 bg-amber-500/10 border-amber-500/30' };
    }
    const hours = Math.floor(diffMins / 60);
    const mins = diffMins % 60;
    return {
      text: hours > 0 ? `Aktif dlm ${hours}j ${mins}m` : `Aktif dlm ${mins}m`,
      color: 'text-emerald-400 bg-emerald-500/10 border-emerald-500/30',
    };
  };

  const currentStrategy = selectedProvider?.credential_strategy || 'round_robin';

  return (
    <div className="space-y-3">
      {/* Header Row */}
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-white flex items-center gap-1.5 min-w-0">
          <KeyRound className="w-3.5 h-3.5 text-accent flex-shrink-0" />
          <span className="truncate">Kredensial & Pool Akun · AES-256-GCM</span>
        </span>
        {!isAddingKeyInline && (
          <button
            type="button"
            onClick={() => {
              setInlineAuthTab(isAntigravity ? 'authlogin' : 'apikey');
              setNewKeyForm({
                label: isAntigravity
                  ? 'Antigravity Session Token'
                  : directCredentials.length === 0 ? 'Primary API Key' : `Akun Cadangan #${directCredentials.length + 1}`,
                api_key: '',
              });
              setIsAddingKeyInline(true);
            }}
            className="px-2.5 py-1.5 rounded-lg bg-accent text-black font-bold hover:bg-accent-hover transition-colors flex items-center gap-1 text-xs"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isAntigravity ? 'Hubungkan Akun Google' : 'Tambah Key'}</span>
          </button>
        )}
      </div>

      {/* Universal Credential Strategy Control */}
      <div className="p-3 rounded-xl border border-border bg-bg-surface-2/60 space-y-2">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Layers className="w-4 h-4 text-accent flex-shrink-0" />
            <div>
              <span className="text-xs font-bold text-white block">Strategi Rotasi Antar-Akun</span>
              <span className="text-[10px] text-text-muted">
                Aturan pemilihan akun / kredensial aktif saat inferensi
              </span>
            </div>
          </div>
          <div className="flex items-center gap-1 bg-bg-surface p-1 rounded-lg border border-border">
            <button
              type="button"
              disabled={isUpdatingStrategy}
              onClick={() => handleStrategyChange('round_robin')}
              className={`px-2 py-1 rounded-md text-[11px] font-semibold transition-colors flex items-center gap-1.5 ${
                currentStrategy === 'round_robin'
                  ? 'bg-accent text-black shadow-sm'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <RotateCcw className="w-3 h-3" />
              Round Robin (50:50)
            </button>
            <button
              type="button"
              disabled={isUpdatingStrategy}
              onClick={() => handleStrategyChange('priority')}
              className={`px-2 py-1 rounded-md text-[11px] font-semibold transition-colors flex items-center gap-1.5 ${
                currentStrategy === 'priority'
                  ? 'bg-accent text-black shadow-sm'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <Sliders className="w-3 h-3" />
              Priority Failover
            </button>
          </div>
        </div>
        <p className="text-[11px] text-text-secondary leading-relaxed pl-6">
          {currentStrategy === 'round_robin'
            ? '🔄 Round Robin: Permintaan LLM dibagi merata secara bergantian ke seluruh akun aktif untuk memaksimalkan throughput bersamaan.'
            : '⚡ Priority Failover: Menggunakan Akun Utama terlebih dahulu. Jika kuota habis atau terkena rate limit (429), otomatis beralih ke akun Cadangan.'}
        </p>
      </div>

      {/* Form Tambah Kredensial / Akun Baru Inline */}
      {isAddingKeyInline && (
        <div className="p-3 rounded-xl border border-accent/40 bg-accent/5 space-y-3">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-semibold text-white">
              {isAntigravity ? 'Hubungkan Akun Google Antigravity Tambahan' : 'Tambah Kredensial Baru'}
            </span>
            <button type="button" onClick={resetInlineForm} className="text-[11px] text-text-muted hover:text-white">Batal</button>
          </div>

          {/* Mode Switcher Tabs: Hanya untuk Provider Resmi NON-Antigravity */}
          {!isAntigravity && !isCustomProvider && (
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
          )}

          {/* Banner Khusus Google Antigravity */}
          {isAntigravity && (
            <div className="p-2.5 rounded-lg bg-accent/10 border border-accent/20 text-xs text-text-secondary flex items-start gap-2">
              <Shield className="w-4 h-4 text-accent shrink-0 mt-0.5" />
              <div>
                <span className="font-semibold text-white block">OAuth Token Exchange Engine</span>
                <span className="text-[11px] text-text-muted">
                  Buka login Google di bawah, beri izin, lalu salin seluruh URL redirect browser (mis. http://localhost:4567/?code=...) atau kodenya ke kolom di bawah. Backend Route-X otomatis menukarkan token dan mengaktifkan auto-refresh tanpa input Client ID/Secret.
                </span>
              </div>
            </div>
          )}

          <form noValidate onSubmit={handleCreateKey} className="space-y-2.5 text-xs">
            {!isAntigravity && (
              <div>
                <label className="block text-[11px] font-medium text-text-secondary mb-1">Label Kredensial</label>
                <input
                  type="text"
                  required
                  placeholder="mis. Primary API Key atau Backup Token"
                  value={newKeyForm.label}
                  onChange={(e) => setNewKeyForm({ ...newKeyForm, label: e.target.value })}
                  className="w-full px-2.5 py-1.5 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs placeholder:text-text-muted outline-none focus:border-accent"
                />
              </div>
            )}

            {inlineAuthTab === 'apikey' && !isAntigravity ? (
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
                  <button
                    type="button"
                    onClick={() => setShowNewKeySecret(!showNewKeySecret)}
                    aria-label={showNewKeySecret ? "Sembunyikan API key" : "Tampilkan API key"}
                    className="absolute right-2 top-2 text-text-muted hover:text-white"
                  >
                    {showNewKeySecret ? <EyeOff className="w-3.5 h-3.5" aria-hidden="true" /> : <Eye className="w-3.5 h-3.5" aria-hidden="true" />}
                  </button>
                </div>
              </div>
            ) : (
              <div className="space-y-2 pt-0.5">
                {matchedPreset?.authLoginUrl && (
                  <div className="flex items-center justify-between p-2 rounded-lg bg-bg-surface/80 border border-border/60">
                    <span className="text-[11px] text-text-secondary truncate">{matchedPreset.authLoginLabel || 'Buka Halaman Login'}</span>
                    <a
                      href={matchedPreset.authLoginUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="px-2.5 py-1 rounded bg-accent text-black font-semibold text-[11px] flex items-center gap-1 hover:opacity-90 flex-shrink-0"
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
                    placeholder="Tempel seluruh URL redirect (misal: http://localhost:4567/?code=...) atau kode di sini..."
                    value={authFallbackInput}
                    onChange={(e) => handleInlineFallbackChange(e.target.value)}
                    className="w-full px-2.5 py-1.5 bg-bg-surface border border-border rounded-lg text-white font-mono text-xs placeholder:text-text-muted outline-none focus:border-accent"
                  />
                  {authExtractedToken && (
                    <div className="mt-1.5 p-1.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-[11px] font-mono text-emerald-400 flex items-center gap-1.5">
                      <Check className="w-3.5 h-3.5 flex-shrink-0" />
                      <span className="truncate">
                        Kode Terdeteksi ({authExtractionHint}):{' '}
                        <strong className="text-white">
                          {authExtractedToken.length > 20 ? `${authExtractedToken.slice(0, 8)}...${authExtractedToken.slice(-6)}` : authExtractedToken}
                        </strong>
                      </span>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Opsi Jalur Keluar (Egress Proxy) */}
            <div>
              <label className="block text-[11px] font-medium text-text-secondary mb-1 flex items-center gap-1">
                <Globe className="w-3 h-3 text-accent" />
                Jalur Keluar / Proxy Egress (Opsional)
              </label>
              <Select
                value={inlineEgressPoolId}
                onChange={(val) => setInlineEgressPoolId(val)}
                placeholder="Ikuti Provider / Direct (Default)"
                options={[
                  { value: '', label: 'Ikuti Provider / Direct (Default)', description: 'Mengikuti konfigurasi default provider atau direct outbound' },
                  ...egressPools.map((ep) => ({
                    value: ep.id,
                    label: `${ep.name} (${ep.kind.toUpperCase()})`,
                    description: `Protokol: ${ep.kind} · Region: ${ep.region || 'Default'}`,
                  })),
                ]}
              />
            </div>

            <div className="flex items-center justify-between pt-1">
              <span className="text-[10px] text-text-muted flex items-center gap-1">
                <Lock className="w-3 h-3 text-accent flex-shrink-0" /> Enkripsi AES-256-GCM
              </span>
              <div className="flex gap-1.5">
                <button type="button" onClick={resetInlineForm} className="px-2.5 py-1.5 text-xs rounded-lg bg-bg-surface-2 text-text-primary border border-border hover:text-white transition-colors">Batal</button>
                <button type="submit" disabled={isSavingKey} className="px-2.5 py-1.5 text-xs rounded-lg bg-accent text-black font-bold hover:bg-accent-hover transition-colors disabled:opacity-50 flex items-center gap-1">
                  {isSavingKey ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Check className="w-3.5 h-3.5" />}
                  {isAntigravity ? 'Verifikasi & Hubungkan Akun' : 'Simpan Kredensial'}
                </button>
              </div>
            </div>
          </form>
        </div>
      )}

      {/* POOL 1: OAuth Sessions (Google Antigravity & Multi-Account OAuth) */}
      {oauthSessions.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center justify-between text-xs px-0.5">
            <span className="font-semibold text-white flex items-center gap-1.5">
              <Users className="w-3.5 h-3.5 text-accent" />
              Pool Akun Google ({oauthSessions.length} Terdaftar · {oauthSessions.filter((s) => s.enabled).length} Aktif)
            </span>
            <span className="text-[10px] text-emerald-400 flex items-center gap-1 font-mono">
              <Clock className="w-3 h-3" /> Worker Auto-Refresh (10m)
            </span>
          </div>

          <div className="space-y-2">
            {oauthSessions.map((session, index: number) => {
              const expiry = formatExpiry(session.expires_at);
              const isRefreshing = isRefreshingOAuth === session.id;

              return (
                <div
                  key={session.id}
                  className={`p-3 rounded-xl border transition-all ${
                    session.enabled
                      ? 'border-border bg-bg-surface-2/70 shadow-sm'
                      : 'border-border/60 bg-bg-surface-2/20 opacity-70'
                  }`}
                >
                  <div className="flex items-start justify-between gap-2.5">
                    <div className="flex items-center gap-2.5 min-w-0 flex-1">
                      {session.avatar_url ? (
                        <img
                          src={session.avatar_url}
                          alt=""
                          className="w-8 h-8 rounded-full border border-border shrink-0 object-cover"
                        />
                      ) : (
                        <div className="w-8 h-8 rounded-full bg-accent/20 border border-accent/40 flex items-center justify-center font-bold text-accent text-xs shrink-0">
                          {(session.account_email || 'G')[0].toUpperCase()}
                        </div>
                      )}
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <span className="font-bold text-white text-xs truncate">
                            {session.account_email || 'Akun Google'}
                          </span>
                          {session.account_name && (
                            <span className="text-[10px] text-text-muted truncate hidden sm:inline">
                              ({session.account_name})
                            </span>
                          )}
                          {currentStrategy === 'priority' && (
                            <span className={`px-1.5 py-0.2 rounded text-[9px] font-bold uppercase ${
                              index === 0 ? 'bg-accent/20 text-accent border border-accent/40' : 'bg-bg-surface text-text-muted border border-border'
                            }`}>
                              {index === 0 ? 'Utama' : `Cadangan #${index}`}
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-1.5 flex-wrap mt-1">
                          <span
                            className={`px-1.5 py-0.5 rounded text-[10px] font-semibold border flex items-center gap-1 ${
                              session.enabled
                                ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
                                : 'bg-zinc-500/10 text-zinc-400 border-zinc-500/30'
                            }`}
                          >
                            <span className={`w-1.5 h-1.5 rounded-full ${session.enabled ? 'bg-emerald-400' : 'bg-zinc-400'}`} />
                            {session.enabled ? 'Aktif' : 'Standby'}
                          </span>

                          {expiry && (
                            <span className={`px-1.5 py-0.5 rounded text-[10px] font-mono border ${expiry.color}`}>
                              {expiry.text}
                            </span>
                          )}

                          {session.last_refresh_error ? (
                            <span className="px-1.5 py-0.5 rounded text-[10px] bg-red-500/10 text-red-400 border border-red-500/30 truncate max-w-[200px]" title={session.last_refresh_error}>
                              Gagal Refresh: {session.last_refresh_error}
                            </span>
                          ) : (
                            <span className="text-[10px] text-text-muted hidden md:inline">
                              Auto-refresh aktif
                            </span>
                          )}
                        </div>
                      </div>
                    </div>

                    {/* Actions: Standby Toggle, Refresh Now, Delete */}
                    <div className="flex items-center gap-1 shrink-0 pt-0.5">
                      <button
                        type="button"
                        onClick={() => handleToggleOAuthSession(session)}
                        title={session.enabled ? 'Alihkan ke Standby (Nonaktifkan sementara tanpa menghapus)' : 'Aktifkan Akun'}
                        aria-label={session.enabled ? "Nonaktifkan akun OAuth ke mode standby" : "Aktifkan akun OAuth"}
                        className={`p-1.5 rounded-lg border text-xs transition-colors cursor-pointer flex items-center gap-1 ${
                          session.enabled
                            ? 'bg-bg-surface border-border text-amber-400 hover:bg-amber-500/10 hover:border-amber-500/30'
                            : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400 hover:bg-emerald-500/20'
                        }`}
                      >
                        <Power className="w-3.5 h-3.5" aria-hidden="true" />
                        <span className="text-[10px] font-medium hidden sm:inline">
                          {session.enabled ? 'Standby' : 'Aktifkan'}
                        </span>
                      </button>

                      <button
                        type="button"
                        onClick={() => handleRefreshOAuthSession(session.id)}
                        disabled={isRefreshing}
                        title="Refresh Access Token Sekarang via Google OAuth"
                        aria-label="Segarkan access token OAuth sekarang"
                        className="p-1.5 rounded-lg border border-border bg-bg-surface text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors cursor-pointer disabled:opacity-50"
                      >
                        <RefreshCw className={`w-3.5 h-3.5 ${isRefreshing ? 'animate-spin text-accent' : ''}`} aria-hidden="true" />
                      </button>

                      <button
                        type="button"
                        onClick={() => handleDeleteOAuthSession(session)}
                        title="Hapus Akun dari Pool"
                        aria-label="Hapus akun OAuth dari pool"
                        className="p-1.5 rounded-lg border border-border bg-bg-surface text-text-muted hover:text-red-400 hover:bg-red-500/10 hover:border-red-500/30 transition-colors cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                    </div>
                  </div>

                  {/* Selector Jalur Keluar (Egress Proxy) */}
                  <div className="mt-2.5 pt-2 border-t border-border/40 flex flex-col sm:flex-row sm:items-center justify-between text-xs gap-1.5 sm:gap-2">
                    <span className="text-text-muted text-[11px] flex items-center gap-1 shrink-0">
                      <Globe className="w-3 h-3 text-accent" aria-hidden="true" />
                      Jalur Keluar (Proxy):
                    </span>
                    <div className="w-full sm:w-56 shrink-0">
                      <Select
                        variant="compact"
                        value={session.egress_pool_id || ''}
                        onChange={(val) => handleUpdateAccountEgress(session.credential_id, val || null)}
                        placeholder="Ikuti Provider / Direct (Default)"
                        options={[
                          { value: '', label: 'Ikuti Provider / Direct (Default)' },
                          ...egressPools.map((ep) => ({
                            value: ep.id,
                            label: `${ep.name} (${ep.kind.toUpperCase()})`,
                            description: `Protokol: ${ep.kind} · Region: ${ep.region || 'Default'}`,
                          })),
                        ]}
                      />
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* POOL 2: Kredensial Standard (Non-OAuth atau API Key Reguler) */}
      {directCredentials.length > 0 && (
        <div className="space-y-2">
          {oauthSessions.length > 0 && (
            <div className="flex items-center justify-between text-xs px-0.5 pt-2">
              <span className="font-semibold text-white flex items-center gap-1.5">
                <KeyRound className="w-3.5 h-3.5 text-accent" />
                API Key Reguler ({directCredentials.length} Kunci · {directCredentials.filter((c) => c.enabled).length} Aktif)
              </span>
            </div>
          )}
          {directCredentials.map((cred, index: number) => (
            <div
              key={cred.id}
              className={`px-3 py-2 rounded-xl border flex flex-col gap-1.5 transition-all ${
                cred.enabled
                  ? 'border-border bg-bg-surface-2/40'
                  : 'border-border/60 bg-bg-surface-2/15 opacity-70'
              }`}
            >
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-1.5 min-w-0 flex-1">
                  <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${cred.enabled ? 'bg-emerald-400' : 'bg-zinc-400'}`} />
                  <span className="font-bold text-white text-xs truncate">{cred.label}</span>
                  {currentStrategy === 'priority' && (
                    <span className={`px-1.5 py-0.2 rounded text-[9px] font-bold uppercase shrink-0 ${
                      index === 0 ? 'bg-accent/20 text-accent border border-accent/40' : 'bg-bg-surface text-text-muted border border-border'
                    }`}>
                      {index === 0 ? 'Utama' : `Cadangan #${index}`}
                    </span>
                  )}
                  <span className="text-[11px] text-text-muted font-mono truncate hidden sm:inline">{cred.masked_hint || 'sk-****'}</span>
                  <button
                    type="button"
                    onClick={() => void copyWithFeedback(cred.masked_hint || '', 'Masked key disalin', () => {
                      setCopiedTokenId(cred.id);
                      copyTimersRef.current.push(setTimeout(() => setCopiedTokenId(null), 2000));
                    })}
                    aria-label="Salin petunjuk token"
                    className="text-text-muted hover:text-white flex-shrink-0"
                  >
                    {copiedTokenId === cred.id ? <Check className="w-3 h-3 text-emerald-400" aria-hidden="true" /> : <Copy className="w-3 h-3" aria-hidden="true" />}
                  </button>
                </div>
                <div className="flex items-center gap-1 flex-shrink-0">
                  <button
                    type="button"
                    onClick={() => handleToggleKey(cred)}
                    title={cred.enabled ? 'Alihkan ke Standby' : 'Aktifkan'}
                    aria-label={cred.enabled ? "Nonaktifkan API key ke mode standby" : "Aktifkan API key"}
                    className={`p-1.5 rounded-md transition-colors cursor-pointer flex items-center gap-1 text-[11px] ${
                      cred.enabled
                        ? 'text-amber-400 hover:bg-amber-500/10'
                        : 'text-emerald-400 hover:bg-emerald-500/10'
                    }`}
                  >
                    <Power className="w-3.5 h-3.5" aria-hidden="true" />
                    <span className="text-[10px] hidden sm:inline">{cred.enabled ? 'Standby' : 'Aktif'}</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDeleteKey(cred)}
                    aria-label="Hapus API key"
                    className="p-1.5 rounded-md text-text-muted hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                  >
                    <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
                  </button>
                </div>
              </div>
              <div className="pt-1.5 border-t border-border/40 flex flex-col sm:flex-row sm:items-center justify-between text-xs gap-1.5 sm:gap-2">
                <span className="text-text-muted text-[11px] flex items-center gap-1 shrink-0">
                  <Globe className="w-3 h-3 text-accent" aria-hidden="true" />
                  Jalur Keluar (Proxy):
                </span>
                <div className="w-full sm:w-56 shrink-0">
                  <Select
                    variant="compact"
                    value={cred.egress_pool_id || ''}
                    onChange={(val) => handleUpdateAccountEgress(cred.id, val || null)}
                    placeholder="Ikuti Provider / Direct (Default)"
                    options={[
                      { value: '', label: 'Ikuti Provider / Direct (Default)' },
                      ...egressPools.map((ep) => ({
                        value: ep.id,
                        label: `${ep.name} (${ep.kind.toUpperCase()})`,
                        description: `Protokol: ${ep.kind} · Region: ${ep.region || 'Default'}`,
                      })),
                    ]}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Empty State */}
      {directCredentials.length === 0 && oauthSessions.length === 0 && !isAddingKeyInline && (
        <div className="p-5 rounded-xl border border-dashed border-border text-center space-y-2">
          <KeyRound className="w-8 h-8 text-text-muted mx-auto" />
          <p className="text-xs text-text-secondary">
            {isAntigravity
              ? 'Belum ada akun Google Antigravity yang terhubung ke pool.'
              : 'Belum ada kredensial API key untuk provider ini.'}
          </p>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsAddingKeyInline(true)}
            icon={<Plus className="w-3.5 h-3.5" />}
          >
            {isAntigravity ? 'Hubungkan Akun Google Pertama' : 'Tambah API Key Pertama'}
          </Button>
        </div>
      )}
    </div>
  );
};
