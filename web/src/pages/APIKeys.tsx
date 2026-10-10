import React, { useState, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { KeyRound, Plus, RefreshCw, Search } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { getErrorMessage } from '../utils/error';
import type { Model, APIKey } from '../types';
import {
  APIKeyCard,
  CreateAPIKeyDrawer,
  APIKeyRevealModal,
  APIKeyScopeDrawer,
  type ScopeOption,
} from '../components/apikeys';

export const APIKeys: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const queryClient = useQueryClient();

  const {
    data: keysData,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = useQuery({
    queryKey: ['apiKeys'],
    queryFn: () => api.apiKeys.list(),
  });
  const keys = keysData?.items || [];

  const { data: modelsData } = useQuery({
    queryKey: ['models'],
    queryFn: () => api.models.list(),
  });
  const models = (modelsData?.items || []) as Model[];

  // State untuk drawer & modal
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isSubmittingCreate, setIsSubmittingCreate] = useState(false);
  const [createdRawKey, setCreatedRawKey] = useState<string | null>(null);

  // Filter pencarian
  const [searchQuery, setSearchQuery] = useState('');

  // Scope Drawer State
  const [isAllowedOpen, setIsAllowedOpen] = useState(false);
  const [selectedKey, setSelectedKey] = useState<APIKey | null>(null);
  const [selectedScopeIds, setSelectedScopeIds] = useState<string[]>([]);
  const [scopeSearch, setScopeSearch] = useState('');
  const [isLoadingAllowed, setIsLoadingAllowed] = useState(false);

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

  const filteredKeys = useMemo(() => {
    if (!searchQuery.trim()) return keys;
    const q = searchQuery.toLowerCase().trim();
    return keys.filter(
      (k) =>
        k.name.toLowerCase().includes(q) ||
        k.masked_key.toLowerCase().includes(q)
    );
  }, [keys, searchQuery]);

  const handleCreateSubmit = async (formData: {
    name: string;
    rpm_limit: number;
    tpm_limit: number;
    monthlyBudgetUsd: string;
    expiresAt?: string;
  }) => {
    if (!formData.name) {
      toast.error('Nama kunci wajib diisi.');
      return;
    }
    if (formData.rpm_limit < 0 || formData.tpm_limit < 0) {
      toast.error('Batas RPM/TPM tidak boleh negatif (0 = tanpa batas).');
      return;
    }

    setIsSubmittingCreate(true);
    try {
      const res = await api.apiKeys.create({
        name: formData.name,
        rate_limit_rpm: formData.rpm_limit || undefined,
        rate_limit_tpm: formData.tpm_limit || undefined,
        scopes: ['inference'],
        model_ids: [],
        provider_ids: [],
        expires_at: formData.expiresAt,
      });

      const keyId = res.id;
      const budgetNum = Number(formData.monthlyBudgetUsd);
      let budgetWarning: string | null = null;
      if (keyId && Number.isFinite(budgetNum) && budgetNum > 0) {
        try {
          await api.budgets.create({
            name: `Anggaran ${formData.name}`,
            scope: 'api_key',
            scope_id: keyId,
            period: 'monthly',
            limit_usd: budgetNum.toFixed(2),
            alert_threshold_pct: 80,
            action_on_exceed: 'block',
          });
        } catch (bErr: unknown) {
          budgetWarning = getErrorMessage(bErr);
        }
      }

      setIsCreateOpen(false);
      setCreatedRawKey(res.raw_key || null);

      if (budgetWarning) {
        toast.warn(
          `Kunci API dibuat, namun alokasi anggaran otomatis gagal: ${budgetWarning}`,
          'Kunci Dibuat Sebagian'
        );
      } else {
        toast.success(
          budgetNum > 0
            ? `Kunci API dan alokasi anggaran $${budgetNum.toFixed(2)}/bulan berhasil dibuat.`
            : 'Kunci API baru berhasil dibuat.',
          'API Key Dibuat'
        );
      }
      void queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
      void queryClient.invalidateQueries({ queryKey: ['budgets'] });
    } catch (err: unknown) {
      toast.error('Gagal membuat API key: ' + getErrorMessage(err));
    } finally {
      setIsSubmittingCreate(false);
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
      void queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: unknown) {
      toast.error('Gagal rotasi key: ' + getErrorMessage(err));
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
      void queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: unknown) {
      toast.error('Gagal mencabut key: ' + getErrorMessage(err));
    }
  };

  const handleOpenAllowed = async (k: APIKey) => {
    setSelectedKey(k);
    setIsAllowedOpen(true);
    setScopeSearch('');

    const initialModels: string[] = k.allowed_models || k.model_ids || [];
    const initialProviders: string[] = k.allowed_providers || k.provider_ids || [];

    const resolveSelected = (modelsList: string[], providersList: string[]) => {
      if (modelsList.length === 0) return [];
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
      // Pertahankan initial value jika endpoint gagal
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
      void queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: unknown) {
      toast.error('Gagal menyimpan allowed scope: ' + getErrorMessage(err));
    }
  };

  const toggleScopeItem = (id: string) => {
    setSelectedScopeIds((prev) =>
      prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]
    );
  };

  const handleSelectAllScopes = () => {
    setSelectedScopeIds(filteredScopeOptions.map((opt) => opt.id));
  };

  const handleDeselectAllScopes = () => {
    setSelectedScopeIds([]);
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Client API Keys"
        actions={
          <div className="flex items-center gap-2 w-full sm:w-auto">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void refetch()}
              isLoading={isFetching}
              icon={<RefreshCw className="w-3.5 h-3.5" aria-hidden="true" />}
              aria-label="Segarkan daftar kunci API"
              className="min-h-[44px] sm:min-h-0"
            />
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4" aria-hidden="true" />}
              className="w-full sm:w-auto justify-center min-h-[44px] sm:min-h-0 font-semibold"
            >
              Buat API Key
            </Button>
          </div>
        }
      />

      {/* Filter & Toolbar */}
      {keys.length > 0 && (
        <div className="flex flex-col sm:flex-row items-center justify-between gap-3 bg-bg-surface p-3 rounded-card border border-border">
          <div className="relative w-full sm:w-80">
            <Search className="w-4 h-4 absolute left-3 top-3 text-text-muted" aria-hidden="true" />
            <input
              type="text"
              placeholder="Cari berdasarkan nama atau prefix kunci..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-bg-surface-2 border border-border rounded-nav pl-9 pr-3 py-2 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none"
            />
          </div>
          <div className="text-xs text-text-muted">
            Menampilkan <span className="text-white font-medium">{filteredKeys.length}</span> dari{' '}
            <span className="text-white font-medium">{keys.length}</span> kunci
          </div>
        </div>
      )}

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
                <div className="w-10 h-10 rounded-xl bg-bg-surface-2" />
                <div className="space-y-2 flex-1">
                  <div className="h-4 bg-bg-surface-2 rounded w-28" />
                  <div className="h-3 bg-bg-surface-2 rounded w-36" />
                </div>
              </div>
              <div className="space-y-2 pt-3 border-t border-border/40">
                <div className="h-3 bg-bg-surface-2 rounded w-full" />
                <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
              </div>
              <div className="pt-3 border-t border-border flex justify-between">
                <div className="h-8 bg-bg-surface-2 rounded w-24" />
                <div className="h-8 bg-bg-surface-2 rounded w-20" />
              </div>
            </Card>
          ))}
        </div>
      ) : keys.length === 0 ? (
        <Card className="py-12 px-6 text-center border-border bg-bg-surface">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center mx-auto shadow-inner">
              <KeyRound className="w-6 h-6" aria-hidden="true" />
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
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4" aria-hidden="true" />}
              className="min-h-[44px] px-5 font-semibold mx-auto"
            >
              Buat Kunci API Pertama
            </Button>
          </div>
        </Card>
      ) : filteredKeys.length === 0 ? (
        <Card className="p-8 text-center text-xs text-text-muted">
          Tidak ada kunci API yang cocok dengan &quot;{searchQuery}&quot;.
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredKeys.map((k) => (
            <APIKeyCard
              key={k.id}
              apiKey={k}
              onOpenAllowed={handleOpenAllowed}
              onRotate={handleRotate}
              onRevoke={handleRevoke}
            />
          ))}
        </div>
      )}

      {/* Drawer Pembuatan Kunci Baru */}
      <CreateAPIKeyDrawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        onSubmit={handleCreateSubmit}
        isSubmitting={isSubmittingCreate}
      />

      {/* Modal Penampilan Kunci Mentah */}
      <APIKeyRevealModal
        rawKey={createdRawKey}
        onClose={() => setCreatedRawKey(null)}
      />

      {/* Drawer Hak Akses Scope (Model & Provider) */}
      <APIKeyScopeDrawer
        isOpen={isAllowedOpen}
        onClose={() => setIsAllowedOpen(false)}
        selectedKey={selectedKey}
        scopeOptions={scopeOptions}
        filteredScopeOptions={filteredScopeOptions}
        selectedScopeIds={selectedScopeIds}
        scopeSearch={scopeSearch}
        setScopeSearch={setScopeSearch}
        onToggleScopeItem={toggleScopeItem}
        onSelectAll={handleSelectAllScopes}
        onDeselectAll={handleDeselectAllScopes}
        onSave={handleSaveAllowed}
        isLoading={isLoadingAllowed}
      />
    </div>
  );
};
