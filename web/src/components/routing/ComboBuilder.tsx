import React, { useId } from 'react';
import { Plus, Trash2, Zap, Scale, Activity, Coins, Layers, ArrowRight, ShieldCheck } from 'lucide-react';
import { Select } from '../common/Select';
import { Button } from '../common/Button';
import {
  LABEL_STRATEGI,
  STRATEGI_COMBO,
  ATTEMPTS_MIN,
  ATTEMPTS_MAX,
  MODELS_MAX,
  validasiPipeline,
  validasiAlias,
  type ComboPipeline,
  type StrategiCombo,
} from '../../lib/rulePipeline';
import type { Model } from '../../types';

export interface ComboBuilderProps {
  value: ComboPipeline;
  onChange: (p: ComboPipeline) => void;
  alias: string;
  onAliasChange: (a: string) => void;
  models: Model[];
  errors?: string[];
  disabled?: boolean;
  idPrefix?: string;
}

const OPSI_STRATEGI = STRATEGI_COMBO.map((s) => ({
  value: s,
  label: LABEL_STRATEGI[s].label,
  description: LABEL_STRATEGI[s].ringkas,
}));


export const ComboBuilder: React.FC<ComboBuilderProps> = ({
  value,
  onChange,
  alias,
  onAliasChange,
  models,
  errors,
  disabled = false,
  idPrefix,
}) => {
  const uid = useId();
  const prefix = idPrefix || uid;

  const errorsInline = errors ?? [
    ...validasiPipeline(value),
    ...(validasiAlias(alias, value) ? [validasiAlias(alias, value) as string] : []),
  ];
  const tampilkanError = errorsInline.length > 0;

  const modelOptions = models
    .filter((m) => m.enabled)
    .map((m) => ({ value: m.model_id, label: m.display_name, description: m.model_id }));

  const dipakaiDiLain = (index: number, modelId: string) =>
    value.models.some((m, i) => i !== index && m === modelId);

  const ubahStrategi = (s: string) => {
    onChange({ ...value, strategy: s as StrategiCombo });
  };

  const ubahModel = (index: number, modelId: string) => {
    const nextModels = value.models.map((m, i) => (i === index ? modelId : m));
    onChange({ ...value, models: nextModels });
  };

  const tambahModel = () => {
    if (value.models.length >= MODELS_MAX || disabled) return;
    onChange({ ...value, models: [...value.models, ''] });
  };

  const hapusModel = (index: number) => {
    onChange({ ...value, models: value.models.filter((_, i) => i !== index) });
  };

  const ubahAttempts = (e: React.ChangeEvent<HTMLInputElement>) => {
    const n = parseInt(e.target.value, 10);
    onChange({ ...value, attempts: Number.isNaN(n) ? ATTEMPTS_MIN : n });
  };

  const idStrategi = `${prefix}-strategi`;
  const idAttempts = `${prefix}-attempts`;
  const idAlias = `${prefix}-alias`;

  // Hitung bobot probabilitas visual
  const validModelsCount = value.models.filter((m) => Boolean(m.trim())).length;
  const equalPercent = validModelsCount > 0 ? (100 / validModelsCount).toFixed(1) : '0';

  return (
    <div className="space-y-4" data-testid="combo-builder">
      {/* 1. Pemilihan Strategi Perutean */}
      <div>
        <Select
          id={idStrategi}
          label="Strategi"
          options={OPSI_STRATEGI}
          value={value.strategy}
          onChange={ubahStrategi}
          disabled={disabled}
        />
        <p className="mt-1 text-[11px] text-text-muted">
          {LABEL_STRATEGI[value.strategy].ringkas}
        </p>
      </div>

      {/* 2. Visualisasi Bobot Probabilitas & Distribusi Trafik */}
      {value.models.length > 0 && (
        <div className="p-3 rounded-xl bg-bg-surface-2/60 border border-border space-y-2">
          <div className="flex items-center justify-between text-xs">
            <span className="font-semibold text-white flex items-center gap-1.5">
              {value.strategy === 'round_robin' ? (
                <Scale className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
              ) : value.strategy === 'lowest_latency' ? (
                <Activity className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" />
              ) : value.strategy === 'lowest_cost' ? (
                <Coins className="w-3.5 h-3.5 text-cyan-400" aria-hidden="true" />
              ) : (
                <Layers className="w-3.5 h-3.5 text-blue-400" aria-hidden="true" />
              )}
              <span>Visualisasi Distribusi &amp; Probabilitas</span>
            </span>
            <span className="text-[10px] font-mono text-text-muted">
              Anggaran: {value.attempts} attempt{value.attempts > 1 ? 's' : ''}
            </span>
          </div>

          {/* Bar Segmentasi Proporsi */}
          {value.strategy === 'round_robin' ? (
            <div className="space-y-1.5">
              <div className="h-2 w-full rounded-full bg-bg-surface overflow-hidden flex">
                {value.models.map((_, i) => (
                  <div
                    key={i}
                    style={{ width: `${100 / value.models.length}%` }}
                    className={`h-full ${
                      i === 0
                        ? 'bg-blue-500'
                        : i === 1
                        ? 'bg-purple-500'
                        : i === 2
                        ? 'bg-emerald-500'
                        : i === 3
                        ? 'bg-amber-500'
                        : 'bg-cyan-500'
                    }`}
                  />
                ))}
              </div>
              <div className="flex items-center justify-between text-[10px] font-mono text-text-muted">
                <span>Pembagian Rata Beban:</span>
                <span className="text-accent font-semibold">~{equalPercent}% per model</span>
              </div>
            </div>
          ) : value.strategy === 'lowest_latency' ? (
            <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-[11px] text-emerald-300 flex items-center gap-2">
              <Activity className="w-3.5 h-3.5 text-emerald-400 shrink-0" aria-hidden="true" />
              <span>Prioritas inferensi dinamis otomatis dialirkan ke model dengan latensi terukur terendah saat request tiba.</span>
            </div>
          ) : value.strategy === 'lowest_cost' ? (
            <div className="p-2 rounded-lg bg-cyan-500/10 border border-cyan-500/20 text-[11px] text-cyan-300 flex items-center gap-2">
              <Coins className="w-3.5 h-3.5 text-cyan-400 shrink-0" aria-hidden="true" />
              <span>Gateway otomatis mengutamakan model dengan tarif per 1M token terhemat dari katalog harga.</span>
            </div>
          ) : (
            /* Priority Cascade */
            <div className="space-y-1.5">
              <div className="flex items-center gap-1.5 overflow-x-auto pb-1 text-[11px]">
                {value.models.map((mId, i) => {
                  const mObj = models.find((m) => m.model_id === mId);
                  const name = mObj?.display_name || mId || `Pilih Model ${i + 1}`;
                  return (
                    <React.Fragment key={i}>
                      {i > 0 && <ArrowRight className="w-3 h-3 text-text-muted shrink-0" aria-hidden="true" />}
                      <span className={`px-2 py-0.5 rounded text-[10px] font-mono font-medium truncate max-w-[140px] ${
                        i === 0
                          ? 'bg-blue-500/20 text-blue-300 border border-blue-500/40'
                          : 'bg-bg-surface text-text-secondary border border-border'
                      }`}>
                        T{i + 1}: {name}
                      </span>
                    </React.Fragment>
                  );
                })}
              </div>
              <div className="flex items-center justify-between text-[10px] text-text-muted font-sans">
                <span className="flex items-center gap-1 text-blue-300">
                  <ShieldCheck className="w-3 h-3" aria-hidden="true" /> Tier 1 Utama
                </span>
                <span className="font-mono">Failover bertahap jika timeout / 429</span>
              </div>
            </div>
          )}
        </div>
      )}

      {/* 3. Daftar Model dalam Resep Combo */}
      <div>
        <div className="flex items-center justify-between mb-1.5">
          <span className="text-xs font-semibold text-text-secondary uppercase">
            Model ({value.models.length}/{MODELS_MAX})
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            icon={<Plus className="w-3.5 h-3.5" aria-hidden="true" />}
            onClick={tambahModel}
            disabled={disabled || value.models.length >= MODELS_MAX}
            aria-label="Tambah Model"
            className="min-h-[36px] font-semibold"
          >
            Tambah Model
          </Button>
        </div>

        <div className="space-y-2">
          {value.models.map((modelId, index) => {
            const options = modelOptions.map((o) => ({
              ...o,
              disabled: dipakaiDiLain(index, o.value),
            }));
            return (
              <div key={index} className="flex items-center gap-2">
                <span className="flex-shrink-0 w-6 text-center text-xs font-mono font-bold text-accent bg-accent/10 py-1.5 rounded border border-accent/20">
                  {index + 1}
                </span>
                <div className="flex-1">
                  <Select
                    id={`${prefix}-model-${index}`}
                    options={options}
                    value={modelId}
                    onChange={(v) => ubahModel(index, v)}
                    placeholder="-- Pilih Model --"
                    disabled={disabled}
                    searchable
                    aria-label={`Model urutan ${index + 1}`}
                  />
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  icon={<Trash2 className="w-3.5 h-3.5" aria-hidden="true" />}
                  onClick={() => hapusModel(index)}
                  disabled={disabled}
                  aria-label={`Hapus model urutan ${index + 1}`}
                  className="min-h-[38px] min-w-[38px] flex items-center justify-center text-text-muted hover:text-red-400 hover:bg-red-500/10"
                />
              </div>
            );
          })}
          {value.models.length === 0 && (
            <p className="text-[11px] text-text-muted italic p-3 rounded-lg border border-dashed border-border text-center">
              Belum ada model — tambahkan minimal satu model untuk resep combo.
            </p>
          )}
        </div>
        <p className="mt-1 text-[11px] text-text-muted">
          Urutan adalah preferensi fallback. Strategi Prioritas dapat menggeser urutan
          ini karena mengurutkan per prioritas provider.
        </p>
      </div>

      {/* 4. Anggaran Total Percobaan (Attempts) */}
      <div>
        <label
          htmlFor={idAttempts}
          className="block text-xs font-semibold text-text-secondary uppercase mb-1"
        >
          Anggaran Attempts
        </label>
        <input
          id={idAttempts}
          type="number"
          min={ATTEMPTS_MIN}
          max={ATTEMPTS_MAX}
          step={1}
          value={value.attempts}
          onChange={ubahAttempts}
          disabled={disabled}
          className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-white focus:outline-none focus:ring-1 focus:ring-accent font-mono"
        />
        <p className="mt-1 text-[11px] text-text-muted">
          Anggaran TOTAL percobaan lintas seluruh model, bukan per model. Contoh:
          anggaran {value.attempts} dengan {value.models.length} model berarti maksimal{' '}
          {value.attempts} request upstream, lalu berhenti.
        </p>
      </div>

      {/* 5. Virtual Endpoint (Alias) */}
      <div>
        <label
          htmlFor={idAlias}
          className="block text-xs font-semibold text-text-secondary uppercase mb-1"
        >
          Virtual Endpoint (Alias)
        </label>
        <input
          id={idAlias}
          type="text"
          value={alias}
          onChange={(e) => onAliasChange(e.target.value)}
          disabled={disabled}
          placeholder="contoh: murah-cerdas"
          className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-white focus:outline-none focus:ring-1 focus:ring-accent font-mono"
        />
        <p className="mt-1 text-[11px] text-text-muted flex items-center gap-1">
          <Zap className="w-3 h-3 flex-shrink-0 text-accent" aria-hidden="true" />
          Opsional. Klien memanggil pengenal ini alih-alih nama model. Wajib ada resep
          pipeline yang sah.
        </p>
      </div>

      {/* 6. Pesan Error Validasi */}
      {tampilkanError && (
        <div
          role="alert"
          className="p-2.5 rounded-lg border border-status-error/40 bg-status-error/5 text-xs text-status-error space-y-1"
          data-testid="combo-builder-errors"
        >
          {errorsInline.map((e, i) => (
            <p key={i}>• {e}</p>
          ))}
        </div>
      )}
    </div>
  );
};
