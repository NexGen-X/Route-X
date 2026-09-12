import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Budget } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { Coins, Plus, RotateCcw } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { formatUSD, percentageOfDecimal } from '../utils/money';

export const Budgets: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [budgets, setBudgets] = useState<Budget[]>([]);
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
    try {
      const res = await api.budgets.list();
      setBudgets(res.items || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    }
  };

  useEffect(() => {
    loadBudgets();
  }, []);

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
        description="Alokasi batas pengeluaran moneter USD skala 8 desimal dengan peringatan ambang batas dan pemblokiran otomatis."
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

      {budgets.length === 0 && !loadError ? (
        <Card className="py-12 px-6 text-center">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center mx-auto shadow-inner">
              <Coins className="w-6 h-6" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Belum Ada Anggaran yang Dialokasikan</h3>
              <p className="text-xs text-text-muted mt-1 leading-relaxed">
                Tetapkan pagu pengeluaran inferensi USD skala 8 desimal (harian, mingguan, atau bulanan) untuk mencegah lonjakan biaya upstream tanpa terduga.
              </p>
            </div>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4 text-black" />}
            >
              Alokasikan Anggaran Pertama
            </Button>
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
                      <span className="text-[11px] text-text-muted font-mono">{b.scope} • {b.period}</span>
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

              <div className="mt-5 pt-3 border-t border-border flex justify-end">
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
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Anggaran</label>
            <input
              type="text"
              required
              placeholder="Budget Bulanan Tim Internal"
              value={newBudget.name}
              onChange={(e) => setNewBudget({ ...newBudget, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Select
              label="Cakupan (Scope)"
              value={newBudget.scope}
              onChange={(val) => setNewBudget({ ...newBudget, scope: val })}
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

          {newBudget.scope !== 'global' && (
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">
                Target {newBudget.scope === 'api_key' ? 'ID Kunci API' : 'ID Model'}
              </label>
              <input
                type="text"
                required
                placeholder={newBudget.scope === 'api_key' ? 'Masukkan UUID Kunci API...' : 'Masukkan Canonical Slug Model (contoh: gpt-4o)...'}
                value={newBudget.scope_id}
                onChange={(e) => setNewBudget({ ...newBudget, scope_id: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Batas Maksimal (USD)</label>
              <input
                type="number"
                step="0.01"
                min="0.01"
                required
                value={newBudget.max_spend_usd}
                onChange={(e) => setNewBudget({ ...newBudget, max_spend_usd: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Ambang Peringatan (%)</label>
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
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Anggaran
          </Button>
        </form>
      </Drawer>
    </div>
  );
};
