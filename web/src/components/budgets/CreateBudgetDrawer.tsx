import React, { useEffect, useState } from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { Sparkles } from 'lucide-react';
import type { BudgetFormData, CreateBudgetDrawerProps } from './types';

const INITIAL_FORM_STATE: BudgetFormData = {
  name: '',
  scope: 'global',
  scope_id: '',
  period: 'monthly',
  max_spend_usd: '100.00',
  alert_threshold: 80,
  action: 'block',
};

export const CreateBudgetDrawer: React.FC<CreateBudgetDrawerProps> = ({
  isOpen,
  onClose,
  onSubmit,
  apiKeys,
  models,
  initialValues,
}) => {
  const [formData, setFormData] = useState<BudgetFormData>(INITIAL_FORM_STATE);

  useEffect(() => {
    if (isOpen) {
      setFormData({
        ...INITIAL_FORM_STATE,
        ...initialValues,
      });
    }
  }, [isOpen, initialValues]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    void onSubmit(formData);
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title="Alokasi Anggaran Moneter Baru"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Batal
          </Button>
          <Button variant="primary" type="submit" form="create-budget-form">
            Simpan Anggaran
          </Button>
        </>
      }
    >
      <form id="create-budget-form" noValidate onSubmit={handleSubmit} className="space-y-4 text-xs">
        {/* Quick Presets Khusus Developer / Pemakaian Pribadi */}
        <div className="p-3 rounded-xl bg-accent/5 border border-accent/20 space-y-2">
          <span className="text-[11px] font-semibold text-accent flex items-center gap-1.5">
            <Sparkles className="w-3.5 h-3.5" />
            Preset 1-Klik Anggaran
          </span>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  name: 'Batas $10/Bulan',
                  max_spend_usd: '10.00',
                  alert_threshold: 80,
                  period: 'monthly',
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-emerald-500/50 hover:bg-emerald-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-emerald-300">$10/bln</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Dev Hemat</span>
            </button>
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  name: 'Batas $50/Bulan',
                  max_spend_usd: '50.00',
                  alert_threshold: 85,
                  period: 'monthly',
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-sky-500/50 hover:bg-sky-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-sky-300">$50/bln</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Tim Standar</span>
            </button>
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  name: 'Batas $100/Bulan',
                  max_spend_usd: '100.00',
                  alert_threshold: 90,
                  period: 'monthly',
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-purple-500/50 hover:bg-purple-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-purple-300">$100/bln</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Power Dev</span>
            </button>
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  name: 'Batas Harian $5',
                  max_spend_usd: '5.00',
                  alert_threshold: 80,
                  period: 'daily',
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-amber-500/50 hover:bg-amber-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-amber-300">$5/hari</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Uji Harian</span>
            </button>
          </div>
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Anggaran *</label>
          <input
            type="text"
            required
            placeholder="Budget Bulanan Tim Internal"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
          />
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <Select
            label="Cakupan (Scope)"
            value={formData.scope}
            onChange={(val) => setFormData({ ...formData, scope: val, scope_id: '' })}
            options={[
              { value: 'global', label: 'Global Gateway', description: 'Berlaku untuk total pemakaian seluruh gateway' },
              { value: 'api_key', label: 'Per API Key', description: 'Membatasi pengeluaran kunci API tertentu' },
              { value: 'model', label: 'Per Model', description: 'Membatasi pengeluaran kuota model tertentu' },
            ]}
          />
          <Select
            label="Periode"
            value={formData.period}
            onChange={(val) => setFormData({ ...formData, period: val })}
            options={[
              { value: 'daily', label: 'Harian', description: 'Reset pagu setiap 24 jam' },
              { value: 'weekly', label: 'Mingguan', description: 'Reset pagu setiap awal pekan' },
              { value: 'monthly', label: 'Bulanan', description: 'Reset pagu setiap tanggal 1 bulan' },
            ]}
          />
        </div>

        {formData.scope === 'api_key' && (
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Target Kunci API *
            </label>
            {apiKeys.length > 0 ? (
              <Select
                value={formData.scope_id}
                onChange={(val) => setFormData({ ...formData, scope_id: val })}
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
                value={formData.scope_id}
                onChange={(e) => setFormData({ ...formData, scope_id: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            )}
          </div>
        )}

        {formData.scope === 'model' && (
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Target Model *
            </label>
            {models.length > 0 ? (
              <Select
                value={formData.scope_id}
                onChange={(val) => setFormData({ ...formData, scope_id: val })}
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
                value={formData.scope_id}
                onChange={(e) => setFormData({ ...formData, scope_id: e.target.value })}
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
              value={formData.max_spend_usd}
              onChange={(e) => setFormData({ ...formData, max_spend_usd: e.target.value })}
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
              value={formData.alert_threshold}
              onChange={(e) => {
                const v = parseInt(e.target.value, 10);
                setFormData({ ...formData, alert_threshold: Number.isNaN(v) ? 80 : v });
              }}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
        </div>

        <div>
          <Select
            label="Tindakan Pelanggaran"
            value={formData.action}
            onChange={(val) => setFormData({ ...formData, action: val })}
            options={[
              { value: 'block', label: 'Blokir Permintaan (429 Too Many Requests)', description: 'Tolak request baru jika limit tercapai' },
              { value: 'warn', label: 'Peringatan Saja (Catat Log)', description: 'Izinkan inferensi tapi tandai pelanggaran di audit log' },
            ]}
          />
        </div>
      </form>
    </Drawer>
  );
};
