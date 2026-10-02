import React, { useEffect, useState, useCallback } from 'react';
import { api } from '../api/client';
import type { Budget, APIKey, Model, RateLimit } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { Coins, Plus, Sparkles, Gauge } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import {
  type BudgetTab,
  type BudgetFormData,
  type RateLimitFormData,
  getScopeDisplay,
  getErrorMessage,
  BudgetCard,
  RateLimitCard,
  CreateBudgetDrawer,
  CreateRateLimitDrawer,
} from '../components/budgets';

export interface BudgetsProps {
  initialTab?: BudgetTab;
}

export const Budgets: React.FC<BudgetsProps> = ({ initialTab = 'budgets' }) => {
  const { toast, confirmModal } = useToast();
  const [activeTab, setActiveTab] = useState<BudgetTab>(initialTab);
  const [budgets, setBudgets] = useState<Budget[]>([]);
  const [limits, setLimits] = useState<RateLimit[]>([]);
  const [apiKeys, setApiKeys] = useState<APIKey[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  // Drawer States
  const [isCreateBudgetOpen, setIsCreateBudgetOpen] = useState(false);
  const [budgetInitialValues, setBudgetInitialValues] = useState<Partial<BudgetFormData> | undefined>(undefined);

  const [isCreateLimitOpen, setIsCreateLimitOpen] = useState(false);
  const [limitInitialValues, setLimitInitialValues] = useState<Partial<RateLimitFormData> | undefined>(undefined);

  useEffect(() => {
    if (initialTab) {
      setActiveTab(initialTab);
    }
  }, [initialTab]);

  const loadData = useCallback(async () => {
    setIsLoading(true);
    try {
      const [bRes, lRes, kRes, mRes] = await Promise.all([
        api.budgets.list(),
        api.rateLimits.list().catch(() => ({ items: [] as RateLimit[] })),
        api.apiKeys.list().catch(() => ({ items: [] as APIKey[] })),
        api.models.list().catch(() => ({ items: [] as Model[] })),
      ]);
      setBudgets(bRes.items || []);
      setLimits(lRes.items || []);
      setApiKeys(kRes.items || []);
      setModels(mRes.items || []);
      setLoadError(null);
    } catch (err: unknown) {
      setLoadError(getErrorMessage(err));
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  // Handler Anggaran
  const handleResetBudget = async (id: string) => {
    const ok = await confirmModal({
      title: 'Reset Pemakaian Anggaran?',
      message: 'Reset pemakaian anggaran periode ini kembali ke nol? Tindakan ini akan membuka kembali akses jika sebelumnya terblokir.',
      confirmText: 'Ya, Reset',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;

    try {
      await api.budgets.reset(id);
      toast.success('Pemakaian anggaran periode ini berhasil direset ke nol.', 'Anggaran Direset');
      void loadData();
    } catch (err: unknown) {
      toast.error('Gagal reset anggaran: ' + getErrorMessage(err));
    }
  };

  const handleToggleBudget = async (b: Budget) => {
    try {
      await api.budgets.toggle(b.id, !b.enabled);
      toast.success(`Anggaran "${b.name}" ${!b.enabled ? 'diaktifkan' : 'dinonaktifkan'}.`);
      void loadData();
    } catch (err: unknown) {
      toast.error('Gagal mengubah status anggaran: ' + getErrorMessage(err));
    }
  };

  const handleDeleteBudget = async (id: string) => {
    const confirmed = await confirmModal({
      title: 'Hapus Alokasi Anggaran?',
      message: 'Apakah Anda yakin ingin menghapus alokasi anggaran ini? Batas pengeluaran untuk target ini tidak akan berlaku lagi.',
      confirmText: 'Ya, Hapus Anggaran',
      cancelText: 'Batal',
      danger: true,
    });
    if (!confirmed) return;

    try {
      await api.budgets.delete(id);
      toast.success('Alokasi anggaran berhasil dihapus.', 'Anggaran Dihapus');
      void loadData();
    } catch (err: unknown) {
      toast.error('Gagal menghapus anggaran: ' + getErrorMessage(err));
    }
  };

  const handleCreateBudget = async (formData: BudgetFormData) => {
    const limit = Number(formData.max_spend_usd);
    if (!Number.isFinite(limit) || limit <= 0) {
      toast.error('Batas maksimal wajib angka lebih dari 0 USD.');
      return;
    }
    if (!Number.isInteger(formData.alert_threshold) || formData.alert_threshold < 1 || formData.alert_threshold > 100) {
      toast.error('Ambang peringatan wajib 1-100 persen.');
      return;
    }
    if (formData.scope !== 'global' && !formData.scope_id.trim()) {
      toast.error(`Pilih target ${formData.scope === 'api_key' ? 'Kunci API' : 'Model'}.`);
      return;
    }
    try {
      await api.budgets.create({
        name: formData.name.trim(),
        scope: formData.scope,
        scope_id: formData.scope === 'global' ? undefined : (formData.scope_id.trim() || undefined),
        period: formData.period,
        limit_usd: formData.max_spend_usd,
        alert_threshold_pct: formData.alert_threshold,
        action_on_exceed: formData.action,
      });
      setIsCreateBudgetOpen(false);
      setBudgetInitialValues(undefined);
      toast.success('Alokasi anggaran baru berhasil disimpan.', 'Anggaran Dibuat');
      void loadData();
    } catch (err: unknown) {
      toast.error('Gagal membuat anggaran: ' + getErrorMessage(err));
    }
  };

  // Handler Rate Limits
  const handleCreateLimit = async (formData: RateLimitFormData) => {
    const num = (v: number) => (Number.isFinite(v) ? v : 0);
    const rpm = num(formData.requests_per_minute);
    const tpm = num(formData.tokens_per_minute);
    const rps = num(formData.requests_per_second);
    if (rpm < 0 || tpm < 0 || rps < 0) {
      toast.error('RPM/TPM/RPS tidak boleh negatif.');
      return;
    }
    if (rpm === 0 && tpm === 0 && rps === 0) {
      toast.error('Isi minimal satu batas (RPM/TPM/RPS) lebih dari 0.');
      return;
    }
    const finalScopeId = formData.scope === 'global' ? 'global' : formData.scope_id.trim();
    if (!finalScopeId) {
      toast.error('Pilih atau masukkan target scope.');
      return;
    }
    try {
      await api.rateLimits.create({
        ...formData,
        scope_id: finalScopeId,
        requests_per_minute: rpm,
        tokens_per_minute: tpm,
        requests_per_second: rps,
      });
      setIsCreateLimitOpen(false);
      setLimitInitialValues(undefined);
      toast.success('Aturan batas laju berhasil dibuat');
      void loadData();
    } catch (err: unknown) {
      toast.error('Gagal membuat limit: ' + getErrorMessage(err));
    }
  };

  const handleDeleteLimit = async (id: string) => {
    const confirmed = await confirmModal({
      title: 'Hapus Aturan Rate Limit',
      message: 'Apakah Anda yakin ingin menghapus aturan pembatasan laju ini? Kebijakan fallback default akan berlaku.',
      danger: true,
      confirmText: 'Ya, Hapus Aturan',
      cancelText: 'Batal',
    });
    if (!confirmed) return;

    try {
      await api.rateLimits.delete(id);
      toast.success('Aturan batas laju berhasil dihapus');
      void loadData();
    } catch (err: unknown) {
      toast.error('Gagal menghapus: ' + getErrorMessage(err));
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={activeTab === 'budgets' ? 'Batas Anggaran (Budgets & Cost Control)' : 'Batas Laju Trafik (Rate Limits)'}
        actions={
          activeTab === 'budgets' ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                setBudgetInitialValues(undefined);
                setIsCreateBudgetOpen(true);
              }}
              icon={<Plus className="w-4 h-4" />}
              className="w-full sm:w-auto justify-center"
            >
              Alokasikan Anggaran
            </Button>
          ) : (
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                setLimitInitialValues(undefined);
                setIsCreateLimitOpen(true);
              }}
              icon={<Plus className="w-4 h-4" />}
              className="w-full sm:w-auto justify-center"
            >
              Tambah Rate Limit
            </Button>
          )
        }
      />

      {/* Tab Switcher Pills */}
      <div className="flex flex-wrap border-b border-border/80 gap-1" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'budgets'}
          onClick={() => setActiveTab('budgets')}
          className={`px-4 py-2.5 text-xs font-semibold rounded-t-lg transition-colors flex items-center gap-2 cursor-pointer ${
            activeTab === 'budgets'
              ? 'border-b-2 border-accent text-white bg-bg-surface-2'
              : 'text-text-muted hover:text-white hover:bg-bg-surface-2/40'
          }`}
        >
          <Coins className="w-4 h-4 text-accent" />
          <span>Batas Anggaran (USD)</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'limits'}
          onClick={() => setActiveTab('limits')}
          className={`px-4 py-2.5 text-xs font-semibold rounded-t-lg transition-colors flex items-center gap-2 cursor-pointer ${
            activeTab === 'limits'
              ? 'border-b-2 border-accent text-white bg-bg-surface-2'
              : 'text-text-muted hover:text-white hover:bg-bg-surface-2/40'
          }`}
        >
          <Gauge className="w-4 h-4 text-accent" />
          <span>Batas Laju Trafik (RPM / TPM)</span>
        </button>
      </div>

      {loadError && <QueryError message={loadError} onRetry={() => void loadData()} />}

      {/* ===================== TAB 1: BATAS ANGGARAN (USD) ===================== */}
      {activeTab === 'budgets' && (
        <>
          {isLoading ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4" data-testid="budgets-loading-skeleton">
              {[1, 2, 3].map((i) => (
                <Card key={i} className="p-5 space-y-4 animate-pulse">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-bg-surface-2" />
                    <div className="space-y-1.5 flex-1">
                      <div className="h-4 bg-bg-surface-2 rounded w-1/2" />
                      <div className="h-3 bg-bg-surface-2 rounded w-1/3" />
                    </div>
                  </div>
                  <div className="h-2 bg-bg-surface-2 rounded-full" />
                  <div className="space-y-2">
                    <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
                    <div className="h-3 bg-bg-surface-2 rounded w-1/2" />
                  </div>
                </Card>
              ))}
            </div>
          ) : budgets.length === 0 && !loadError ? (
            <Card className="py-12 px-6 text-center" data-testid="budgets-empty-state">
              <div className="max-w-md mx-auto space-y-4">
                <div className="w-12 h-12 rounded-2xl bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center mx-auto shadow-inner">
                  <Coins className="w-6 h-6" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-white">Belum Ada Anggaran yang Dialokasikan</h3>
                  <p className="text-xs text-text-muted mt-1 leading-relaxed">
                    Tetapkan pagu pengeluaran inferensi USD (harian, mingguan, atau bulanan) untuk mencegah lonjakan biaya upstream tanpa terduga.
                  </p>
                </div>
                <div className="flex flex-col sm:flex-row items-center justify-center gap-2 pt-2">
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => {
                      setBudgetInitialValues(undefined);
                      setIsCreateBudgetOpen(true);
                    }}
                    icon={<Plus className="w-4 h-4 text-black" />}
                  >
                    Alokasikan Anggaran
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => {
                      setBudgetInitialValues({
                        name: 'Pagu Dev Santai',
                        scope: 'global',
                        scope_id: '',
                        period: 'monthly',
                        max_spend_usd: '10.00',
                        alert_threshold: 80,
                        action: 'block',
                      });
                      setIsCreateBudgetOpen(true);
                    }}
                    icon={<Sparkles className="w-3.5 h-3.5 text-accent" />}
                  >
                    Template Dev $10/bln
                  </Button>
                </div>
              </div>
            </Card>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4" data-testid="budgets-grid">
              {budgets.map((b) => (
                <BudgetCard
                  key={b.id}
                  budget={b}
                  scopeDisplay={getScopeDisplay(b.scope, b.scope_id, apiKeys, models)}
                  onToggle={handleToggleBudget}
                  onReset={handleResetBudget}
                  onDelete={handleDeleteBudget}
                />
              ))}
            </div>
          )}
        </>
      )}

      {/* ===================== TAB 2: BATAS LAJU TRAFIK (RATE LIMITS) ===================== */}
      {activeTab === 'limits' && (
        <>
          {isLoading ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4" data-testid="limits-loading-skeleton">
              {[1, 2, 3].map((i) => (
                <Card key={i} className="p-5 space-y-4 animate-pulse">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-bg-surface-2" />
                    <div className="space-y-1.5 flex-1">
                      <div className="h-4 bg-bg-surface-2 rounded w-1/2" />
                      <div className="h-3 bg-bg-surface-2 rounded w-1/3" />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
                    <div className="h-3 bg-bg-surface-2 rounded w-1/2" />
                  </div>
                </Card>
              ))}
            </div>
          ) : limits.length === 0 && !loadError ? (
            <Card className="py-12 px-6 text-center" data-testid="limits-empty-state">
              <div className="max-w-md mx-auto space-y-4">
                <div className="w-12 h-12 rounded-2xl bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center mx-auto shadow-inner">
                  <Gauge className="w-6 h-6" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-white">Belum Ada Aturan Rate Limit</h3>
                  <p className="text-xs text-text-muted mt-1 leading-relaxed">
                    Atur batas laju permintaan per menit (RPM) atau token per menit (TPM) terdistribusi via Redis sliding-window untuk melindungi stabilitas gateway.
                  </p>
                </div>
                <div className="pt-2">
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => {
                      setLimitInitialValues(undefined);
                      setIsCreateLimitOpen(true);
                    }}
                    icon={<Plus className="w-4 h-4 text-black" />}
                  >
                    Tambah Rate Limit
                  </Button>
                </div>
              </div>
            </Card>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4" data-testid="limits-grid">
              {limits.map((l) => (
                <RateLimitCard
                  key={l.id}
                  limit={l}
                  scopeDisplay={getScopeDisplay(l.scope, l.scope_id, apiKeys, models)}
                  onDelete={handleDeleteLimit}
                />
              ))}
            </div>
          )}
        </>
      )}

      {/* Drawers */}
      <CreateBudgetDrawer
        isOpen={isCreateBudgetOpen}
        onClose={() => {
          setIsCreateBudgetOpen(false);
          setBudgetInitialValues(undefined);
        }}
        onSubmit={handleCreateBudget}
        apiKeys={apiKeys}
        models={models}
        initialValues={budgetInitialValues}
      />

      <CreateRateLimitDrawer
        isOpen={isCreateLimitOpen}
        onClose={() => {
          setIsCreateLimitOpen(false);
          setLimitInitialValues(undefined);
        }}
        onSubmit={handleCreateLimit}
        apiKeys={apiKeys}
        initialValues={limitInitialValues}
      />
    </div>
  );
};
