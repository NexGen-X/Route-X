import React from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { useToast } from '../../context/ToastContext';
import { isValidUSD } from './utils';
import type { ModelPricingDrawerProps } from './types';
import { DollarSign, Clock, Sparkles, Database } from 'lucide-react';

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
            <Button variant="ghost" onClick={onClose} disabled={isSaving} className="min-h-[44px] sm:min-h-[38px]">
              Batal
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="pricing-form"
              isLoading={isSaving}
              icon={<DollarSign className="w-4 h-4 text-black" aria-hidden="true" />}
              className="min-h-[44px] sm:min-h-[38px] font-semibold"
            >
              Simpan Harga
            </Button>
          </>
        ) : (
          <Button variant="ghost" onClick={onClose} className="min-h-[44px] sm:min-h-[38px]">
            Tutup
          </Button>
        )
      }
    >
      {isLoading ? (
        <div className="py-8 text-center text-xs text-text-muted" data-testid="pricing-loading" role="status">
          Memuat pemetaan model...
        </div>
      ) : mappings.length === 0 ? (
        <div className="py-8 text-center text-xs text-text-muted space-y-2.5" data-testid="pricing-empty-mappings">
          <Database className="w-8 h-8 text-text-muted mx-auto" aria-hidden="true" />
          <p className="font-semibold text-white">Model ini belum memiliki pemetaan ke upstream provider.</p>
          <p className="text-[11px] max-w-sm mx-auto text-text-secondary">
            Hubungkan provider terlebih dahulu di halaman Upstream Providers sebelum mengatur struktur harga token.
          </p>
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

          {/* Pricing Input Grid */}
          <div className="p-3.5 rounded-xl border border-border bg-bg-surface-2/40 space-y-3">
            <span className="text-[11px] font-semibold text-text-secondary flex items-center gap-1.5 uppercase tracking-wide">
              <DollarSign className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
              Struktur Tarif per 1 Juta Token (USD)
            </span>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="input_per_1m_usd">
                  Input / 1M Token (USD) *
                </label>
                <div className="relative">
                  <span className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted font-mono text-xs">$</span>
                  <input
                    id="input_per_1m_usd"
                    name="input_per_1m_usd"
                    type="text"
                    inputMode="decimal"
                    required
                    placeholder="2.50"
                    value={pricingForm.input_per_1m_usd}
                    onChange={(e) => onPricingFormChange({ ...pricingForm, input_per_1m_usd: e.target.value })}
                    className="w-full pl-7 pr-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="output_per_1m_usd">
                  Output / 1M Token (USD) *
                </label>
                <div className="relative">
                  <span className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted font-mono text-xs">$</span>
                  <input
                    id="output_per_1m_usd"
                    name="output_per_1m_usd"
                    type="text"
                    inputMode="decimal"
                    required
                    placeholder="10.00"
                    value={pricingForm.output_per_1m_usd}
                    onChange={(e) => onPricingFormChange({ ...pricingForm, output_per_1m_usd: e.target.value })}
                    className="w-full pl-7 pr-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                  />
                </div>
              </div>
            </div>

            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="cached_input_per_1m_usd">
                Cached Input / 1M Token (USD) (Opsional)
              </label>
              <div className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted font-mono text-xs">$</span>
                <input
                  id="cached_input_per_1m_usd"
                  name="cached_input_per_1m_usd"
                  type="text"
                  inputMode="decimal"
                  placeholder="1.25"
                  value={pricingForm.cached_input_per_1m_usd}
                  onChange={(e) => onPricingFormChange({ ...pricingForm, cached_input_per_1m_usd: e.target.value })}
                  className="w-full pl-7 pr-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                />
              </div>
              <p className="text-[10px] text-text-muted mt-1 flex items-center gap-1">
                <Sparkles className="w-3 h-3 text-accent" aria-hidden="true" />
                Tarif diskon jika upstream provider mendukung prompt caching (mis. Anthropic, Gemini, DeepSeek).
              </p>
            </div>
          </div>

          {pricingHistory.length > 0 && (
            <div className="pt-2" data-testid="pricing-history-section">
              <span className="text-xs font-medium text-text-secondary mb-1.5 flex items-center gap-1.5">
                <Clock className="w-3.5 h-3.5 text-text-muted" aria-hidden="true" />
                Riwayat Penetapan Harga
              </span>
              <div className="max-h-36 overflow-y-auto space-y-1.5 rounded-xl border border-border p-2.5 bg-bg-surface-2/40 scrollbar-thin">
                {pricingHistory.map((h, i) => (
                  <div key={`${h.effective_from}-${i}`} className="flex justify-between items-center text-[11px] font-mono text-text-secondary p-1.5 rounded bg-bg-surface border border-border/50">
                    <span className="text-white font-medium">In: ${h.input_per_1m_usd} | Out: ${h.output_per_1m_usd}</span>
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
