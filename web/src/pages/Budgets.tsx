import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Budget } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Coins, Plus, RotateCcw } from 'lucide-react';

export const Budgets: React.FC = () => {
  const [budgets, setBudgets] = useState<Budget[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newBudget, setNewBudget] = useState({
    name: '',
    scope: 'global',
    period: 'monthly',
    max_spend_usd: '100.00',
    alert_threshold: 80,
    action: 'block',
  });

  const loadBudgets = async () => {
    try {
      const res = await api.budgets.list();
      setBudgets(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadBudgets();
  }, []);

  const handleReset = async (id: string) => {
    if (!confirm('Reset pemakaian anggaran periode ini kembali ke nol?')) return;
    try {
      await api.budgets.reset(id);
      loadBudgets();
    } catch (err) {
      alert('Gagal reset anggaran: ' + err);
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.budgets.create({
        ...newBudget,
        limit_usd: newBudget.max_spend_usd,
        alert_threshold_pct: newBudget.alert_threshold,
        action_on_exceed: newBudget.action,
      });
      setIsCreateOpen(false);
      loadBudgets();
    } catch (err) {
      alert('Gagal membuat budget: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Budgets & Cost Control</h2>
          <p className="text-xs text-text-secondary mt-1">
            Alokasi batas pengeluaran moneter USD skala 8 desimal dengan peringatan ambang batas dan pemblokiran otomatis.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Alokasikan Anggaran
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {budgets.map((b) => {
          const spent = parseFloat(b.spent_usd || '0');
          const max = parseFloat(b.max_spend_usd || '1');
          const pct = Math.min(100, Math.round((spent / max) * 100));
          const isDanger = pct >= b.alert_threshold;

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
                      ${spent.toFixed(4)} / ${max.toFixed(2)}
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
                    <span className="font-mono text-text-primary">{b.alert_threshold}%</span>
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

      <Modal
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
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Cakupan (Scope)</label>
              <select
                value={newBudget.scope}
                onChange={(e) => setNewBudget({ ...newBudget, scope: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              >
                <option value="global">Global Gateway</option>
                <option value="api_key">Per API Key</option>
                <option value="model">Per Model</option>
              </select>
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Periode</label>
              <select
                value={newBudget.period}
                onChange={(e) => setNewBudget({ ...newBudget, period: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              >
                <option value="daily">Harian</option>
                <option value="weekly">Mingguan</option>
                <option value="monthly">Bulanan</option>
              </select>
            </div>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Batas Maksimal (USD)</label>
            <input
              type="number"
              step="0.01"
              required
              value={newBudget.max_spend_usd}
              onChange={(e) => setNewBudget({ ...newBudget, max_spend_usd: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Anggaran
          </Button>
        </form>
      </Modal>
    </div>
  );
};
