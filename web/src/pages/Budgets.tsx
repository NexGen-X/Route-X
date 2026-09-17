import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Budget, APIKey, Model } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { Coins, Plus, RotateCcw, Power, Sparkles } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { formatUSD, percentageOfDecimal } from '../utils/money';

export const Budgets: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [budgets, setBudgets] = useState<Budget[]>([]);
  const [apiKeys, setApiKeys] = useState<APIKey[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newBudget, setNewBudget] = useState({
    name: '',
    scope: 'global',
    scope_id: '',
    period: 'monthly',
    max_spend_usd: '100.00',
    alert_threshold: 80,
    action: 'block',
  });

  const loadBudgets = async () => {
    setIsLoading(true);
    try {
      const [bRes, kRes, mRes] = await Promise.all([
        api.budgets.list(),
        api.apiKeys.list().catch(() => ({ items: [] as APIKey[] })),
        api.models.list().catch(() => ({ items: [] as Model[] })),
      ]);
      setBudgets(bRes.items || []);
      setApiKeys(kRes.items || []);
      setModels(mRes.items || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadBudgets();
  }, []);

  const getScopeDisplay = (scope: string, scopeId?: string) => {
    if (!scopeId || scope === 'global') return { label: 'Global Gateway', isRaw: false };
    if (scope === 'api_key') {
      const key = apiKeys.find((k) => k.id === scopeId);
      return { label: key ? key.name : `${scopeId.slice(0, 8)}...`, isRaw: !key };
    }
    if (scope === 'model') {
      const m = models.find((mod) => mod.model_id === scopeId);
      return { label: m ? (m.display_name || m.model_id) : scopeId, isRaw: false };
    }
    return { label: scopeId, isRaw: true };
  };

  const handleReset = async (id: string) => {
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
      loadBudgets();
    } catch (err: any) {
      toast.error('Gagal reset anggaran: ' + (err.message || err));
    }
  };

  const handleToggle = async (b: Budget) => {
    try {
      await api.budgets.toggle(b.id, !b.enabled);
      toast.success(`Anggaran "${b.name}" ${!b.enabled ? 'diaktifkan' : 'dinonaktifkan'}.`);
      loadBudgets();
    } catch (err: any) {
      toast.error('Gagal mengubah status anggaran: ' + (err.message || err));
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    const limit = Number(newBudget.max_spend_usd);
    if (!Number.isFinite(limit) || limit <= 0) {
      toast.error('Batas maksimal wajib angka lebih dari 0 USD.');
      return;
    }
    if (!Number.isInteger(newBudget.alert_threshold) || newBudget.alert_threshold < 1 || newBudget.alert_threshold > 100) {
      toast.error('Ambang peringatan wajib 1-100 persen.');
      return;
    }
    if (newBudget.scope !== 'global' && !newBudget.scope_id.trim()) {
      toast.error(`Pilih target ${newBudget.scope === 'api_key' ? 'Kunci API' : 'Model'}.`);
      return;
    }
    try {
      await api.budgets.create({
        name: newBudget.name.trim(),
        scope: newBudget.scope,
        scope_id: newBudget.scope === 'global' ? undefined : (newBudget.scope_id.trim() || undefined),
        period: newBudget.period,
        limit_usd: newBudget.max_spend_usd,
        alert_threshold_pct: newBudget.alert_threshold,
        action_on_exceed: newBudget.action,
      });
      setIsCreateOpen(false);
      toast.success('Alokasi anggaran baru berhasil disimpan.', 'Anggaran Dibuat');
      loadBudgets();
    } catch (err: any) {
      toast.error('Gagal membuat anggaran: ' + (err.message || err));
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Budgets & Cost Control"
        description="Kontrol batas pengeluaran biaya AI (Global, per Kunci API, atau per Model) dengan peringatan dini dan pemblokiran otomatis."
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="w-full sm:w-auto justify-center"
          >
            Alokasikan Anggaran
          </Button>
        }
      />

      {loadError && <QueryError message={loadError} onRetry={() => void loadBudgets()} />}

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
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
        <Card className="py-12 px-6 text-center">
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
                onClick={() => setIsCreateOpen(true)}
                icon={<Plus className="w-4 h-4 text-black" />}
              >
                Alokasikan Anggaran
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  setNewBudget({
                    name: 'Pagu Dev Santai',
                    scope: 'global',
                    scope_id: '',
                    period: 'monthly',
                    max_spend_usd: '10.00',
                    alert_threshold: 80,
                    action: 'block',
                  });
                  setIsCreateOpen(true);
                }}
                icon={<Sparkles className="w-3.5 h-3.5 text-accent" />}
              >
                Template Dev $10/bln
              </Button>
            </div>
          </div>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {budgets.map((b) => {
          const pct = percentageOfDecimal(b.spent_usd || '0', b.max_spend_usd || '0');
          const threshold = b.alert_threshold ?? b.alert_threshold_pct ?? 80;
          const isDanger = pct >= threshold;

          return (
            <Card key={b.id} className="p-5 flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center">
                      <Coins className="w-5 h-5" />
                    </div>
                    <div>
                      <h4 className="text-sm font-bold text-white">{b.name}</h4>
                      <div className="flex items-center gap-1.5 text-[11px] text-text-muted font-mono mt-0.5">
                        <span className="text-accent font-semibold">{getScopeDisplay(b.scope, b.scope_id).label}</span>
                        <span>•</span>
                        <span>{b.period}</span>
                      </div>
                    </div>
                  </div>
                  <Badge variant={isDanger ? 'error' : 'success'}>
                    {pct}%
                  </Badge>
                </div>

                <div className="mt-4">
                  <div className="flex justify-between text-xs mb-1.5">
                    <span className="text-text-muted">Terpakai</span>
                    <span className="font-mono text-white font-semibold">
                      {formatUSD(b.spent_usd, 4)} / {formatUSD(b.max_spend_usd, 2)}
                    </span>
                  </div>
                  <div className="w-full h-2 bg-bg-surface-2 rounded-full overflow-hidden">
                    <div
                      className={`h-full transition-all duration-500 rounded-full ${
                        isDanger ? 'bg-status-error' : 'bg-accent'
                      }`}
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                </div>

                <div className="mt-4 space-y-1 text-xs text-text-muted">
                  <div className="flex justify-between py-1 border-b border-border/40">
                    <span>Ambang Peringatan</span>
                    <span className="font-mono text-text-primary">{threshold}%</span>
                  </div>
                  <div className="flex justify-between py-1">
                    <span>Aksi Pelanggaran</span>
                    <span className="font-mono text-accent uppercase">{b.action}</span>
                  </div>
                </div>
              </div>

              <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
                <button
                  type="button"
                  onClick={() => handleToggle(b)}
                  className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold border transition-colors cursor-pointer ${
                    b.enabled
                      ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20 hover:bg-emerald-500/20'
                      : 'bg-bg-surface-2 text-text-muted border-border hover:text-white'
                  }`}
                  title={b.enabled ? 'Klik untuk menonaktifkan anggaran' : 'Klik untuk mengaktifkan anggaran'}
                >
                  <Power className="w-3 h-3" />
                  <span>{b.enabled ? 'Aktif' : 'Nonaktif'}</span>
                </button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => handleReset(b.id)}
                  icon={<RotateCcw className="w-3.5 h-3.5" />}
                >
                  Reset Periode
                </Button>
              </div>
            </Card>
          );
        })}
        </div>
      )}

      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Alokasi Anggaran Moneter Baru"
        subtitle="Batas biaya inferensi dihitung dari pemakaian token upstream"
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-budget-form">
              Simpan Anggaran
            </Button>
          </>
        }
      >
        <form id="create-budget-form" noValidate onSubmit={handleCreate} className="space-y-4 text-xs">
          {/* Quick Presets Khusus Developer / Pemakaian Pribadi */}
          <div className="p-3 rounded-xl bg-accent/5 border border-accent/20 space-y-2">
            <span className="text-[11px] font-semibold text-accent flex items-center gap-1.5">
              <Sparkles className="w-3.5 h-3.5" />
              Template Pagu Cepat (Pemakaian Pribadi & Koding)
            </span>
            <div className="grid grid-cols-3 gap-2">
              <button
                type="button"
                onClick={() =>
                  setNewBudget((prev) => ({
                    ...prev,
                    name: 'Pagu Dev Santai',
                    max_spend_usd: '10.00',
                    alert_threshold: 80,
                    period: 'monthly',
                    scope: 'global',
                    scope_id: '',
                  }))
                }
                className="px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-emerald-500/50 hover:bg-emerald-500/10 text-center transition-all cursor-pointer group"
              >
                <span className="block text-xs font-bold text-white group-hover:text-emerald-300">$10 / bln</span>
                <span className="block text-[10px] text-text-muted mt-0.5">Dev Hemat</span>
              </button>
              <button
                type="button"
                onClick={() =>
                  setNewBudget((prev) => ({
                    ...prev,
                    name: 'Pagu Koding Rutin',
                    max_spend_usd: '30.00',
                    alert_threshold: 85,
                    period: 'monthly',
                    scope: 'global',
                    scope_id: '',
                  }))
                }
                className="px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-sky-500/50 hover:bg-sky-500/10 text-center transition-all cursor-pointer group"
              >
                <span className="block text-xs font-bold text-white group-hover:text-sky-300">$30 / bln</span>
                <span className="block text-[10px] text-text-muted mt-0.5">Koding Harian</span>
              </button>
              <button
                type="button"
                onClick={() =>
                  setNewBudget((prev) => ({
                    ...prev,
                    name: 'Pagu Power Developer',
                    max_spend_usd: '100.00',
                    alert_threshold: 90,
                    period: 'monthly',
                    scope: 'global',
                    scope_id: '',
                  }))
                }
                className="px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-purple-500/50 hover:bg-purple-500/10 text-center transition-all cursor-pointer group"
              >
                <span className="block text-xs font-bold text-white group-hover:text-purple-300">$100 / bln</span>
                <span className="block text-[10px] text-text-muted mt-0.5">Power User</span>
              </button>
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Anggaran *</label>
            <input
              type="text"
              required
              placeholder="Budget Bulanan Tim Internal"
              value={newBudget.name}
              onChange={(e) => setNewBudget({ ...newBudget, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Select
              label="Cakupan (Scope)"
              value={newBudget.scope}
              onChange={(val) => setNewBudget({ ...newBudget, scope: val, scope_id: '' })}
              options={[
                { value: 'global', label: 'Global Gateway', description: 'Berlaku untuk total pemakaian seluruh gateway' },
                { value: 'api_key', label: 'Per API Key', description: 'Membatasi pengeluaran kunci API tertentu' },
                { value: 'model', label: 'Per Model', description: 'Membatasi pengeluaran kuota model tertentu' },
              ]}
            />
            <Select
              label="Periode"
              value={newBudget.period}
              onChange={(val) => setNewBudget({ ...newBudget, period: val })}
              options={[
                { value: 'daily', label: 'Harian', description: 'Reset pagu setiap 24 jam' },
                { value: 'weekly', label: 'Mingguan', description: 'Reset pagu setiap awal pekan' },
                { value: 'monthly', label: 'Bulanan', description: 'Reset pagu setiap tanggal 1 bulan' },
              ]}
            />
          </div>

          {newBudget.scope === 'api_key' && (
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">
                Target Kunci API *
              </label>
              {apiKeys.length > 0 ? (
                <Select
                  value={newBudget.scope_id}
                  onChange={(val) => setNewBudget({ ...newBudget, scope_id: val })}
                  options={[
                    { value: '', label: '-- Pilih Kunci API --' },
                    ...apiKeys.map((k) => ({
                      value: k.id,
                      label: `${k.name} (${k.masked_key || k.id.slice(0, 8)})`,
                    })),
                  ]}
                />
              ) : (
                <input
                  type="text"
                  required
                  placeholder="Masukkan UUID Kunci API..."
                  value={newBudget.scope_id}
                  onChange={(e) => setNewBudget({ ...newBudget, scope_id: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                />
              )}
            </div>
          )}

          {newBudget.scope === 'model' && (
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">
                Target Model *
              </label>
              {models.length > 0 ? (
                <Select
                  value={newBudget.scope_id}
                  onChange={(val) => setNewBudget({ ...newBudget, scope_id: val })}
                  options={[
                    { value: '', label: '-- Pilih Model --' },
                    ...models.map((m) => ({
                      value: m.model_id,
                      label: `${m.display_name || m.model_id} (${m.model_id})`,
                    })),
                  ]}
                />
              ) : (
                <input
                  type="text"
                  required
                  placeholder="Masukkan Slug Model (contoh: gpt-4o)..."
                  value={newBudget.scope_id}
                  onChange={(e) => setNewBudget({ ...newBudget, scope_id: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                />
              )}
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Batas Maksimal (USD) *</label>
              <input
                type="number"
                step="0.01"
                min="0.01"
                required
                value={newBudget.max_spend_usd}
                onChange={(e) => setNewBudget({ ...newBudget, max_spend_usd: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Ambang Peringatan (%) *</label>
              <input
                type="number"
                min={1}
                max={100}
                required
                value={newBudget.alert_threshold}
                onChange={(e) => {
                  const v = parseInt(e.target.value, 10);
                  setNewBudget({ ...newBudget, alert_threshold: Number.isNaN(v) ? 80 : v });
                }}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
          </div>
        </form>
      </Drawer>
    </div>
  );
};
