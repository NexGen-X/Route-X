import React, { useState, useEffect, useRef, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Tooltip } from '../components/common/Tooltip';
import { PageHeader } from '../components/common/PageHeader';
import { KeyRound, Plus, RotateCw, Trash2, Copy, Check, Search, ChevronDown, ChevronUp } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { copyTextToClipboard } from '../utils/clipboard';
import { Checkbox } from '../components/common/Checkbox';
import type { Model } from '../types';

interface ScopeOption {
  id: string; // unique key: `${providerId || 'none'}::${modelId}`
  modelId: string;
  modelSlug: string;
  modelDisplayName: string;
  providerId: string;
  providerName: string;
  providerDisplayName: string;
  family?: string;
  contextWindow?: number;
  capabilities?: string[];
}

export const APIKeys: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const queryClient = useQueryClient();
  const { data: keysData, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['apiKeys'],
    queryFn: () => api.apiKeys.list(),
  });
  const keys = keysData?.items || [];

  const { data: modelsData } = useQuery({
    queryKey: ['models'],
    queryFn: () => api.models.list(),
  });
  const models = (modelsData?.items || []) as Model[];

  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [monthlyBudgetUsd, setMonthlyBudgetUsd] = useState('');
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [newKey, setNewKey] = useState({
    name: '',
    rpm_limit: 120,
    tpm_limit: 100000,
  });
  const [createdRawKey, setCreatedRawKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [isAllowedOpen, setIsAllowedOpen] = useState(false);
  const [selectedKey, setSelectedKey] = useState<any>(null);
  const [selectedScopeIds, setSelectedScopeIds] = useState<string[]>([]);
  const [scopeSearch, setScopeSearch] = useState('');
  const [isLoadingAllowed, setIsLoadingAllowed] = useState(false);

  const openCreateModal = () => {
    setNewKey({ name: '', rpm_limit: 120, tpm_limit: 100000 });
    setMonthlyBudgetUsd('');
    setShowAdvanced(false);
    setIsCreateOpen(true);
  };

  // Timer indikator salin; dibatalkan saat unmount agar tidak ada setState basi.
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copyTimerRef.current) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);

  // Format list item: Provider / Model
  const scopeOptions = useMemo<ScopeOption[]>(() => {
    const list: ScopeOption[] = [];
    for (const m of models) {
      if (m.providers && m.providers.length > 0) {
        for (const p of m.providers) {
          list.push({
            id: `${p.provider_id || p.provider_name}::${m.id}`,
            modelId: m.id,
            modelSlug: m.model_id,
            modelDisplayName: p.upstream_model_name || m.display_name || m.model_id,
            providerId: p.provider_id || '',
            providerName: p.provider_name || '',
            providerDisplayName: p.display_name || p.provider_name || 'Upstream',
            family: m.family,
            contextWindow: m.context_window,
            capabilities: m.capabilities,
          });
        }
      } else {
        list.push({
          id: `gateway::${m.id}`,
          modelId: m.id,
          modelSlug: m.model_id,
          modelDisplayName: m.display_name || m.model_id,
          providerId: '',
          providerName: 'gateway',
          providerDisplayName: 'Gateway',
          family: m.family,
          contextWindow: m.context_window,
          capabilities: m.capabilities,
        });
      }
    }

    return list.sort((a, b) => {
      const pCompare = a.providerDisplayName.localeCompare(b.providerDisplayName);
      if (pCompare !== 0) return pCompare;
      return a.modelDisplayName.localeCompare(b.modelDisplayName);
    });
  }, [models]);

  const filteredScopeOptions = useMemo(() => {
    if (!scopeSearch.trim()) return scopeOptions;
    const q = scopeSearch.toLowerCase().trim();
    return scopeOptions.filter((opt) => {
      return (
        opt.providerDisplayName.toLowerCase().includes(q) ||
        opt.providerName.toLowerCase().includes(q) ||
        opt.modelDisplayName.toLowerCase().includes(q) ||
        opt.modelSlug.toLowerCase().includes(q) ||
        (opt.family && opt.family.toLowerCase().includes(q))
      );
    });
  }, [scopeOptions, scopeSearch]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newKey.name.trim()) {
      toast.error('Nama kunci wajib diisi.');
      return;
    }
    if (newKey.rpm_limit < 0 || newKey.tpm_limit < 0) {
      toast.error('Batas RPM/TPM tidak boleh negatif (0 = tanpa batas).');
      return;
    }
    try {
      const res = await api.apiKeys.create({
        name: newKey.name.trim(),
        rate_limit_rpm: newKey.rpm_limit || undefined,
        rate_limit_tpm: newKey.tpm_limit || undefined,
        scopes: ['inference'],
        model_ids: [],
        provider_ids: [],
      });

      const keyId = (res as any)?.key?.id || res.id;
      const budgetNum = Number(monthlyBudgetUsd);
      if (keyId && Number.isFinite(budgetNum) && budgetNum > 0) {
        try {
          await api.budgets.create({
            name: `Anggaran ${newKey.name.trim()}`,
            scope: 'api_key',
            scope_id: keyId,
            period: 'monthly',
            limit_usd: budgetNum.toFixed(2),
            alert_threshold_pct: 80,
            action_on_exceed: 'block',
          });
        } catch (bErr: any) {
          toast.warn('Kunci dibuat, namun alokasi anggaran otomatis gagal: ' + (bErr.message || bErr));
        }
      }

      setIsCreateOpen(false);
      setShowAdvanced(false);
      setMonthlyBudgetUsd('');
      const rawKeyRevealed = (res as any)?.raw_key || (res as any)?.key?.raw_key || res.raw_key || null;
      setCreatedRawKey(rawKeyRevealed);
      toast.success(
        budgetNum > 0
          ? `Kunci API dan alokasi anggaran $${budgetNum.toFixed(2)}/bulan berhasil dibuat.`
          : 'Kunci API baru berhasil dibuat.',
        'API Key Dibuat'
      );
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
      queryClient.invalidateQueries({ queryKey: ['budgets'] });
    } catch (err: any) {
      toast.error('Gagal membuat API key: ' + (err.message || err));
    }
  };

  const handleRotate = async (id: string) => {
    const ok = await confirmModal({
      title: 'Putar Kunci API?',
      message: 'Kunci lama akan langsung tidak berlaku dan kunci baru akan diterbitkan seketika.',
      confirmText: 'Putar Kunci',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;

    try {
      const res = await api.apiKeys.rotate(id);
      setCreatedRawKey(res.raw_key || null);
      toast.success('Kunci API berhasil diputar.', 'Kunci Diperbarui');
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: any) {
      toast.error('Gagal rotasi key: ' + (err.message || err));
    }
  };

  const handleRevoke = async (id: string) => {
    const ok = await confirmModal({
      title: 'Cabut Kunci API Permanen?',
      message: 'Cabut kunci API ini secara permanen? Seluruh klien dan skrip yang menggunakannya akan langsung ditolak.',
      confirmText: 'Ya, Cabut Kunci',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;

    try {
      await api.apiKeys.revoke(id);
      toast.success('Kunci API berhasil dicabut secara permanen.', 'Kunci Dicabut');
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: any) {
      toast.error('Gagal mencabut key: ' + (err.message || err));
    }
  };

  const copyToClipboard = async (text: string) => {
    try {
      await copyTextToClipboard(text);
      setCopied(true);
      toast.success('Kunci API disalin ke clipboard.');
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
      copyTimerRef.current = setTimeout(() => {
        copyTimerRef.current = null;
        setCopied(false);
      }, 2000);
    } catch (err) {
      toast.error('Gagal menyalin kunci API: ' + (err instanceof Error ? err.message : String(err)));
    }
  };


  const handleOpenAllowed = async (k: any) => {
    setSelectedKey(k);
    setIsAllowedOpen(true);
    setScopeSearch('');

    const initialModels: string[] = k.allowed_models || k.model_ids || [];
    const initialProviders: string[] = k.allowed_providers || k.provider_ids || [];

    const resolveSelected = (modelsList: string[], providersList: string[]) => {
      if (modelsList.length === 0) {
        return [];
      }
      return scopeOptions
        .filter((opt) => {
          const modelMatches = modelsList.includes(opt.modelId) || modelsList.includes(opt.modelSlug);
          if (!modelMatches) return false;
          if (providersList.length === 0 || !opt.providerId) return true;
          return providersList.includes(opt.providerId) || providersList.includes(opt.providerName);
        })
        .map((opt) => opt.id);
    };

    setSelectedScopeIds(resolveSelected(initialModels, initialProviders));

    try {
      setIsLoadingAllowed(true);
      const res = await api.apiKeys.getAllowed(k.id);
      if (res) {
        setSelectedScopeIds(resolveSelected(res.model_ids || [], res.provider_ids || []));
      }
    } catch {
      // Gunakan initial value jika endpoint spesifik gagal
    } finally {
      setIsLoadingAllowed(false);
    }
  };

  const handleSaveAllowed = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedKey) return;

    let modelIds: string[] = [];
    let providerIds: string[] = [];

    if (selectedScopeIds.length > 0) {
      const selectedOpts = scopeOptions.filter((opt) => selectedScopeIds.includes(opt.id));
      modelIds = Array.from(new Set(selectedOpts.map((opt) => opt.modelId)));
      providerIds = Array.from(
        new Set(selectedOpts.map((opt) => opt.providerId).filter((id): id is string => Boolean(id)))
      );
    }

    try {
      await api.apiKeys.setAllowed(selectedKey.id, {
        model_ids: modelIds,
        provider_ids: providerIds,
      });
      toast.success('Scope Hak Akses Kunci API berhasil diperbarui.');
      setIsAllowedOpen(false);
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: any) {
      toast.error('Gagal menyimpan allowed scope: ' + (err.message || err));
    }
  };

  const toggleScopeItem = (id: string) => {
    setSelectedScopeIds((prev) =>
      prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]
    );
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Client API Keys"
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={openCreateModal}
            icon={<Plus className="w-4 h-4" />}
            className="w-full sm:w-auto justify-center"
          >
            Buat API Key
          </Button>
        }
      />

      {isError ? (
        <QueryError
          message={error instanceof Error ? error.message : 'Daftar kunci API tidak tersedia.'}
          onRetry={() => void refetch()}
        />
      ) : isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(3)].map((_, i) => (
            <Card key={i} className="p-5 animate-pulse space-y-4">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-bg-surface-2" />
                <div className="space-y-2 flex-1">
                  <div className="h-4 bg-bg-surface-2 rounded w-24" />
                  <div className="h-3 bg-bg-surface-2 rounded w-32" />
                </div>
              </div>
              <div className="space-y-2 pt-3 border-t border-border/40">
                <div className="h-3 bg-bg-surface-2 rounded w-full" />
                <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
              </div>
              <div className="pt-3 border-t border-border flex justify-between">
                <div className="h-8 bg-bg-surface-2 rounded w-20" />
                <div className="h-8 bg-bg-surface-2 rounded w-20" />
              </div>
            </Card>
          ))}
        </div>
      ) : keys.length === 0 ? (
        <Card className="py-12 px-6 text-center">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center mx-auto shadow-inner">
              <KeyRound className="w-6 h-6" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Belum Ada Kunci API Klien</h3>
              <p className="text-xs text-text-muted mt-1 leading-relaxed">
                Buat kunci API pertama untuk menghubungkan editor atau aplikasi Anda (Cursor, Cline, Open WebUI, LibreChat, atau skrip personal) ke Route-X Gateway.
              </p>
            </div>
            <Button
              variant="primary"
              size="sm"
              onClick={openCreateModal}
              icon={<Plus className="w-4 h-4 text-black" />}
            >
              Buat Kunci API Pertama
            </Button>
          </div>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {keys.map((k) => (
            <Card key={k.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-accent/10 border border-accent/20 text-accent flex items-center justify-center">
                    <KeyRound className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{k.name}</h4>
                    <span className="text-[11px] text-accent font-mono">{k.masked_key}</span>
                  </div>
                </div>
                <Badge variant={k.enabled ? 'success' : 'neutral'}>
                  {k.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Batas RPM / TPM</span>
                  <span className="font-mono text-white">{(k as any).rate_limit_rpm ?? k.rpm_limit ?? '∞'} RPM / {(k as any).rate_limit_tpm ?? k.tpm_limit ?? '∞'} TPM</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Terakhir Digunakan</span>
                  <span className="font-mono text-text-secondary">
                    {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : 'Belum pernah'}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40 items-center">
                  <span className="text-text-muted">Allowed Scope</span>
                  <span className="font-mono text-xs">
                    {(k.allowed_models && k.allowed_models.length > 0) ? (
                      <span className="text-amber-400 font-medium">
                        {k.allowed_models.length} Model Dibatasi
                        {k.allowed_providers && k.allowed_providers.length > 0 ? ` (${k.allowed_providers.length} Provider)` : ''}
                      </span>
                    ) : (
                      <span className="text-accent font-medium">Semua Model (Bebas)</span>
                    )}
                  </span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center gap-2 flex-wrap justify-between">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleOpenAllowed(k)}
                className="w-full sm:w-auto"
              >
                Allowed Scope
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleRotate(k.id)}
                icon={<RotateCw className="w-3.5 h-3.5" />}
              >
                Rotasi
              </Button>
              <Button
                variant="danger"
                size="sm"
                onClick={() => handleRevoke(k.id)}
                icon={<Trash2 className="w-3.5 h-3.5" />}
              >
                Cabut
              </Button>
            </div>
          </Card>
        ))}
        </div>
      )}

      {/* Modal Generate Key Result */}
      {createdRawKey && (
        <Modal
          isOpen={true}
          onClose={() => setCreatedRawKey(null)}
          title="Kunci API Berhasil Dibuat"
          footer={
            <Button
              variant="primary"
              size="md"
              onClick={() => setCreatedRawKey(null)}
              className="w-full sm:w-auto"
            >
              Saya Sudah Menyimpan Kunci Ini
            </Button>
          }
        >
          <div className="space-y-4">
            <div className="p-3 bg-bg-surface-2 rounded-inner border border-accent/30 flex items-center justify-between">
              <span className="font-mono text-xs text-accent break-all select-all font-semibold">
                {createdRawKey}
              </span>
              <Tooltip content={copied ? "Tersalin ke clipboard!" : "Salin Kunci API"} position="left">
                <button
                  type="button"
                  onClick={() => copyToClipboard(createdRawKey)}
                  aria-label="Salin kunci API ke clipboard"
                  className="p-2 text-text-muted hover:text-accent rounded-nav ml-2 flex-shrink-0 cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
                >
                  {copied ? <Check className="w-4 h-4 text-accent" aria-hidden="true" /> : <Copy className="w-4 h-4" aria-hidden="true" />}
                </button>
              </Tooltip>
            </div>
            <p className="text-xs text-text-muted">
              Pastikan Anda telah menyalin dan menyimpan token di atas di tempat yang aman (seperti environment variable).
            </p>
          </div>
        </Modal>
      )}

      {/* Modal Create Key */}
      <Modal
        isOpen={isCreateOpen}
        onClose={() => {
          setIsCreateOpen(false);
          setShowAdvanced(false);
        }}
        title="Buat Kunci API Klien Baru"
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setIsCreateOpen(false);
                setShowAdvanced(false);
              }}
            >
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-api-key-form">
              Hasilkan Kunci API
            </Button>
          </>
        }
      >
        <form id="create-api-key-form" noValidate onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label htmlFor="api-key-name" className="block text-xs font-medium text-text-secondary mb-1.5">
              Nama Kunci *
            </label>
            <input
              id="api-key-name"
              name="name"
              type="text"
              required
              autoFocus
              placeholder="Contoh: Cursor IDE, Backend Production, OpenWebUI"
              value={newKey.name}
              onChange={(e) => setNewKey({ ...newKey, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white placeholder:text-text-muted focus:outline-none focus:border-accent"
            />
          </div>

          <div className="pt-2 border-t border-border/60">
            <button
              type="button"
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="inline-flex items-center gap-1.5 text-xs text-text-muted hover:text-accent font-medium transition-colors cursor-pointer"
            >
              {showAdvanced ? <ChevronUp className="w-3.5 h-3.5 text-accent" /> : <ChevronDown className="w-3.5 h-3.5 text-accent" />}
              <span>{showAdvanced ? 'Sembunyikan Pengaturan Lanjutan' : '⚙️ Pengaturan Lanjutan (Batas Anggaran & Kecepatan)'}</span>
            </button>

            {showAdvanced && (
              <div className="space-y-4 mt-3 p-3.5 rounded-xl border border-border/80 bg-bg-surface-2/40">
                <div>
                  <label htmlFor="api-key-budget" className="block text-xs font-medium text-text-secondary mb-1.5">
                    Pagu Anggaran Bulanan (USD, Opsional)
                  </label>
                  <div className="relative">
                    <span className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted font-mono text-xs">$</span>
                    <input
                      id="api-key-budget"
                      type="number"
                      step="0.01"
                      min="0"
                      placeholder="Contoh: 10.00 (Kosongkan jika tanpa batas anggaran)"
                      value={monthlyBudgetUsd}
                      onChange={(e) => setMonthlyBudgetUsd(e.target.value)}
                      className="w-full pl-7 pr-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent text-xs"
                    />
                  </div>
                  <div className="flex items-center gap-1.5 mt-2 flex-wrap">
                    <span className="text-[10px] text-text-muted font-medium">Pilihan Cepat:</span>
                    {['10.00', '25.00', '50.00', '100.00'].map((amt) => (
                      <button
                        key={amt}
                        type="button"
                        onClick={() => setMonthlyBudgetUsd(amt)}
                        className={`min-h-[36px] px-2.5 py-1 rounded-md text-xs font-mono border transition-colors cursor-pointer flex items-center justify-center ${
                          monthlyBudgetUsd === amt
                            ? 'bg-accent/15 border-accent text-accent font-semibold'
                            : 'bg-bg-surface-2 border-border text-text-secondary hover:text-white hover:border-text-muted'
                        }`}
                      >
                        ${parseFloat(amt)}
                      </button>
                    ))}
                    {monthlyBudgetUsd && (
                      <button
                        type="button"
                        onClick={() => setMonthlyBudgetUsd('')}
                        className="min-h-[36px] px-2 py-1 text-xs text-text-muted hover:text-rose-400 ml-auto cursor-pointer flex items-center"
                      >
                        Hapus
                      </button>
                    )}
                  </div>
                  <p className="text-[11px] text-text-muted mt-1.5 leading-relaxed">
                    Otomatis membatasi pengeluaran per bulan untuk kunci ini tanpa perlu repot membuka menu Anggaran.
                  </p>
                </div>

                <div className="grid grid-cols-2 gap-3 pt-2 border-t border-border/40">
                  <div>
                    <label htmlFor="api-key-rpm" className="block text-xs font-medium text-text-secondary mb-1">
                      Batas Pesan / Menit (RPM)
                    </label>
                    <input
                      id="api-key-rpm"
                      name="rpm_limit"
                      type="number"
                      min={0}
                      value={newKey.rpm_limit}
                      onChange={(e) => setNewKey({ ...newKey, rpm_limit: Math.max(0, parseInt(e.target.value) || 0) })}
                      className="w-full px-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent"
                      placeholder="0 = Tanpa batas"
                    />
                  </div>
                  <div>
                    <label htmlFor="api-key-tpm" className="block text-xs font-medium text-text-secondary mb-1">
                      Batas Token / Menit (TPM)
                    </label>
                    <input
                      id="api-key-tpm"
                      name="tpm_limit"
                      type="number"
                      min={0}
                      value={newKey.tpm_limit}
                      onChange={(e) => setNewKey({ ...newKey, tpm_limit: Math.max(0, parseInt(e.target.value) || 0) })}
                      className="w-full px-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent"
                      placeholder="0 = Tanpa batas"
                    />
                  </div>
                </div>
              </div>
            )}
          </div>
        </form>
      </Modal>

      {/* Modal Edit Allowed */}
      <Modal
        isOpen={isAllowedOpen}
        onClose={() => setIsAllowedOpen(false)}
        title={`Scope Hak Akses: ${selectedKey?.name || 'Kunci API'}`}
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsAllowedOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="edit-allowed-form" isLoading={isLoadingAllowed}>
              Simpan Perubahan Scope
            </Button>
          </>
        }
      >
        <form id="edit-allowed-form" noValidate onSubmit={handleSaveAllowed} className="space-y-4 text-xs">
          {/* Search and Quick Actions */}
          <div className="flex items-center justify-between gap-2">
            <div className="relative flex-1">
              <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
              <input
                type="text"
                placeholder="Cari provider atau model (contoh: tknharbor, deepseek, claude)..."
                value={scopeSearch}
                onChange={(e) => setScopeSearch(e.target.value)}
                className="w-full pl-9 pr-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-xs text-white focus:outline-none focus:border-accent"
              />
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              <button
                type="button"
                onClick={() => {
                  const visibleIds = filteredScopeOptions.map((o) => o.id);
                  setSelectedScopeIds((prev) => Array.from(new Set([...prev, ...visibleIds])));
                }}
                className="px-2.5 py-1 text-[11px] rounded bg-bg-surface-2 hover:bg-bg-surface-3 text-text-secondary hover:text-white border border-border cursor-pointer transition-colors"
              >
                Pilih Semua
              </button>
              <button
                type="button"
                onClick={() => setSelectedScopeIds([])}
                className="px-2.5 py-1 text-[11px] rounded bg-bg-surface-2 hover:bg-bg-surface-3 text-accent hover:text-accent/80 border border-border cursor-pointer transition-colors"
              >
                Bebaskan (Semua)
              </button>
            </div>
          </div>

          {/* Status Hint */}
          <div className={`p-2.5 rounded-lg border text-xs flex items-center justify-between ${
            selectedScopeIds.length === 0
              ? 'bg-accent/5 border-accent/20 text-text-secondary'
              : 'bg-amber-500/5 border-amber-500/20 text-amber-200/90'
          }`}>
            <span>
              {selectedScopeIds.length === 0 ? (
                <>✨ <strong>Mode Bebas:</strong> Kunci ini diizinkan memanggil semua model yang aktif di gateway.</>
              ) : (
                <>🔒 <strong>Mode Terbatas:</strong> Hanya <strong>{selectedScopeIds.length}</strong> model yang dicentang yang boleh diakses.</>
              )}
            </span>
            {selectedScopeIds.length > 0 && (
              <span className="font-mono text-[10px] text-amber-300 shrink-0">
                {selectedScopeIds.length} / {scopeOptions.length} model
              </span>
            )}
          </div>

          {/* Unified Model List (Provider / Model) */}
          <div className="max-h-72 overflow-y-auto space-y-1.5 pr-1 border border-border/50 rounded-lg p-2 bg-bg-surface-1">
            {filteredScopeOptions.length === 0 ? (
              <div className="py-8 text-center text-text-muted text-xs">
                Tidak ada model atau provider yang cocok dengan "{scopeSearch}".
              </div>
            ) : (
              filteredScopeOptions.map((opt) => {
                const isChecked = selectedScopeIds.includes(opt.id);
                return (
                  <div
                    key={opt.id}
                    onClick={() => toggleScopeItem(opt.id)}
                    className={`flex items-center justify-between p-2.5 rounded-lg border cursor-pointer transition-all ${
                      isChecked
                        ? 'bg-accent/10 border-accent/40 text-white shadow-sm'
                        : 'bg-bg-surface-2/60 border-border/60 text-text-secondary hover:border-border hover:bg-bg-surface-2'
                    }`}
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      <Checkbox
                        checked={isChecked}
                        onChange={() => toggleScopeItem(opt.id)}
                        onClick={(e) => e.stopPropagation()}
                      />
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5 flex-wrap">
                          <span className="font-semibold text-accent text-xs">
                            {opt.providerDisplayName}
                          </span>
                          <span className="text-text-muted font-normal text-xs">/</span>
                          <span className="font-medium text-white text-xs truncate">
                            {opt.modelDisplayName}
                          </span>
                        </div>
                        <div className="flex items-center gap-2 mt-0.5 text-[10px] text-text-muted font-mono truncate">
                          <span>model: {opt.modelSlug}</span>
                          {opt.providerName && (
                            <>
                              <span>•</span>
                              <span>provider: {opt.providerName}</span>
                            </>
                          )}
                          {opt.contextWindow && (
                            <>
                              <span>•</span>
                              <span>{Math.round(opt.contextWindow / 1000)}k ctx</span>
                            </>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-1.5 shrink-0 ml-2">
                      {opt.family && (
                        <span className="px-1.5 py-0.5 rounded text-[10px] bg-bg-surface border border-border text-text-muted font-mono">
                          {opt.family}
                        </span>
                      )}
                      {opt.capabilities && opt.capabilities.length > 0 && (
                        <span className="px-1.5 py-0.5 rounded text-[10px] bg-accent/10 text-accent border border-accent/20 font-mono">
                          {opt.capabilities[0]}
                        </span>
                      )}
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </form>
      </Modal>
    </div>

  );
};
