import React, { useEffect, useState } from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { Sparkles } from 'lucide-react';
import type { CreateRateLimitDrawerProps, RateLimitFormData } from './types';

const INITIAL_LIMIT_STATE: RateLimitFormData = {
  scope: 'api_key',
  scope_id: '',
  requests_per_minute: 60,
  tokens_per_minute: 100000,
  requests_per_second: 10,
};

export const CreateRateLimitDrawer: React.FC<CreateRateLimitDrawerProps> = ({
  isOpen,
  onClose,
  onSubmit,
  apiKeys,
  initialValues,
}) => {
  const [formData, setFormData] = useState<RateLimitFormData>(INITIAL_LIMIT_STATE);

  useEffect(() => {
    if (isOpen) {
      setFormData({
        ...INITIAL_LIMIT_STATE,
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
      title="Tambah Aturan Rate Limit Baru"
      footer={
        <>
          <Button variant="ghost" type="button" onClick={onClose}>
            Batal
          </Button>
          <Button variant="primary" type="submit" form="create-rate-limit-form">
            Simpan Limit
          </Button>
        </>
      }
    >
      <form id="create-rate-limit-form" onSubmit={handleSubmit} className="space-y-4 text-xs">
        {/* Presets 1-Klik Rate Limit */}
        <div className="p-3 rounded-xl bg-accent/5 border border-accent/20 space-y-2">
          <span className="text-[11px] font-semibold text-accent flex items-center gap-1.5">
            <Sparkles className="w-3.5 h-3.5" />
            Preset 1-Klik Kecepatan
          </span>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  requests_per_minute: 30,
                  tokens_per_minute: 50000,
                  requests_per_second: 5,
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-emerald-500/50 hover:bg-emerald-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-emerald-300">30 RPM</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Ringan</span>
            </button>
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  requests_per_minute: 60,
                  tokens_per_minute: 100000,
                  requests_per_second: 10,
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-sky-500/50 hover:bg-sky-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-sky-300">60 RPM</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Standar</span>
            </button>
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  requests_per_minute: 120,
                  tokens_per_minute: 250000,
                  requests_per_second: 20,
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-purple-500/50 hover:bg-purple-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-purple-300">120 RPM</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Tinggi</span>
            </button>
            <button
              type="button"
              onClick={() =>
                setFormData((prev) => ({
                  ...prev,
                  requests_per_minute: 300,
                  tokens_per_minute: 500000,
                  requests_per_second: 10,
                }))
              }
              className="min-h-[44px] px-2 py-2 rounded-lg bg-bg-surface-2 border border-border hover:border-amber-500/50 hover:bg-amber-500/10 text-center transition-all cursor-pointer group"
            >
              <span className="block text-xs font-bold text-white group-hover:text-amber-300">10 RPS</span>
              <span className="block text-[10px] text-text-muted mt-0.5">Spike Guard</span>
            </button>
          </div>
        </div>

        <div>
          <Select
            label="Cakupan Pembatasan (Scope) *"
            value={formData.scope}
            onChange={(val) => setFormData({ ...formData, scope: val, scope_id: '' })}
            options={[
              { value: 'api_key', label: 'Per Kunci API (Disarankan)', description: 'Terapkan limit ke Kunci API tertentu' },
              { value: 'ip', label: 'Per Alamat IP Klien', description: 'Terapkan limit ke alamat IPv4/IPv6 asal' },
              { value: 'global', label: 'Global Gateway', description: 'Terapkan ke seluruh gateway secara agregat' },
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

        {formData.scope === 'ip' && (
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Alamat IP Target *
            </label>
            <input
              type="text"
              required
              placeholder="misal: 192.168.1.50 atau 2001:db8::1"
              value={formData.scope_id}
              onChange={(e) => setFormData({ ...formData, scope_id: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Req / Menit (RPM)
            </label>
            <input
              type="number"
              min="0"
              value={formData.requests_per_minute}
              onChange={(e) =>
                setFormData({ ...formData, requests_per_minute: parseInt(e.target.value, 10) || 0 })
              }
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Token / Menit (TPM)
            </label>
            <input
              type="number"
              min="0"
              value={formData.tokens_per_minute}
              onChange={(e) =>
                setFormData({ ...formData, tokens_per_minute: parseInt(e.target.value, 10) || 0 })
              }
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Req / Detik (Burst RPS)
            </label>
            <input
              type="number"
              min="0"
              value={formData.requests_per_second}
              onChange={(e) =>
                setFormData({ ...formData, requests_per_second: parseInt(e.target.value, 10) || 0 })
              }
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
        </div>
      </form>
    </Drawer>
  );
};
