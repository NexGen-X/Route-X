import React from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { useToast } from '../../context/ToastContext';
import { isValidUSD } from './utils';
import type { ModelPricingDrawerProps } from './types';

export const ModelPricingDrawer: React.FC<ModelPricingDrawerProps> = ({
  isOpen,
  onClose,
  model,
  mappings,
  selectedMappingId,
  onSelectMappingId,
  pricingHistory,
  pricingForm,
  onPricingFormChange,
  onSavePrice,
  isLoading = false,
  isSaving = false,
}) => {
  const { toast } = useToast();

  const handleFormSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedMappingId) return;

    if (!isValidUSD(pricingForm.input_per_1m_usd) || !isValidUSD(pricingForm.output_per_1m_usd)) {
      toast.error('Harga input/output wajib angka >= 0 (contoh: 2.50).');
      return;
    }
    if (pricingForm.cached_input_per_1m_usd.trim() !== '' && !isValidUSD(pricingForm.cached_input_per_1m_usd)) {
      toast.error('Harga cached input wajib angka >= 0 atau dikosongkan.');
      return;
    }

    await onSavePrice();
  };

  const modelTitle = model?.display_name || model?.model_id || 'Model';

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title={`Konfigurasi Harga: ${modelTitle}`}
      footer={
        mappings.length > 0 ? (
          <>
            <Button variant="ghost" onClick={onClose} disabled={isSaving}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="pricing-form" isLoading={isSaving}>
              Simpan Harga
            </Button>
          </>
        ) : (
          <Button variant="ghost" onClick={onClose}>
            Tutup
          </Button>
        )
      }
    >
      {isLoading ? (
        <div className="py-8 text-center text-xs text-text-muted" data-testid="pricing-loading">
          Memuat pemetaan model...
        </div>
      ) : mappings.length === 0 ? (
        <div className="py-6 text-center text-xs text-text-muted space-y-2" data-testid="pricing-empty-mappings">
          <p>Model ini belum memiliki pemetaan ke upstream provider.</p>
          <p className="text-[11px]">Hubungkan provider terlebih dahulu sebelum mengatur struktur harga.</p>
        </div>
      ) : (
        <form id="pricing-form" noValidate onSubmit={handleFormSubmit} className="space-y-4 text-xs">
          <Select
            label="Pilih Pemetaan Provider"
            value={selectedMappingId}
            onChange={(val) => {
              onSelectMappingId(val);
            }}
            options={mappings.filter((mp) => mp.id).map((mp) => {
              const prov = model?.providers?.find((p) => p.provider_id === mp.provider_id);
              const provName = prov ? (prov.display_name || prov.provider_name) : (mp.provider_id || '').slice(0, 8);
              return {
                value: mp.id,
                label: `${provName} → ${mp.upstream_model_name || '?'}`,
                description: `Provider: ${provName} | Model Upstream: ${mp.upstream_model_name || '-'}`,
              };
            })}
          />

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="input_per_1m_usd">
                Input / 1M Token (USD) *
              </label>
              <input
                id="input_per_1m_usd"
                name="input_per_1m_usd"
                type="text"
                inputMode="decimal"
                required
                placeholder="2.50"
                value={pricingForm.input_per_1m_usd}
                onChange={(e) => onPricingFormChange({ ...pricingForm, input_per_1m_usd: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="output_per_1m_usd">
                Output / 1M Token (USD) *
              </label>
              <input
                id="output_per_1m_usd"
                name="output_per_1m_usd"
                type="text"
                inputMode="decimal"
                required
                placeholder="10.00"
                value={pricingForm.output_per_1m_usd}
                onChange={(e) => onPricingFormChange({ ...pricingForm, output_per_1m_usd: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="cached_input_per_1m_usd">
              Cached Input / 1M Token (USD) (Opsional)
            </label>
            <input
              id="cached_input_per_1m_usd"
              name="cached_input_per_1m_usd"
              type="text"
              inputMode="decimal"
              placeholder="1.25"
              value={pricingForm.cached_input_per_1m_usd}
              onChange={(e) => onPricingFormChange({ ...pricingForm, cached_input_per_1m_usd: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>

          {pricingHistory.length > 0 && (
            <div className="pt-2" data-testid="pricing-history-section">
              <span className="block text-xs font-medium text-text-secondary mb-1.5">
                Riwayat Penetapan Harga
              </span>
              <div className="max-h-32 overflow-y-auto space-y-1 rounded-lg border border-border p-2.5 bg-bg-surface-2/40">
                {pricingHistory.map((h, i) => (
                  <div key={`${h.effective_from}-${i}`} className="flex justify-between text-[11px] font-mono text-text-secondary">
                    <span>In: ${h.input_per_1m_usd} | Out: ${h.output_per_1m_usd}</span>
                    <span className="text-text-muted">{h.effective_from ? new Date(h.effective_from).toLocaleDateString() : '-'}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </form>
      )}
    </Drawer>
  );
};
