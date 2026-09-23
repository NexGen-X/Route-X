import React, { useId } from 'react';
import { Plus, Trash2, Zap } from 'lucide-react';
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
  // Resep combo saat ini. Induk biasanya mengisi dari parsePipeline(rule) atau
  // pipelineKosong() untuk aturan baru.
  value: ComboPipeline;
  onChange: (p: ComboPipeline) => void;
  // Virtual endpoint. Opsional, tetapi bila terisi resep wajib sah (lihat validasiAlias).
  alias: string;
  onAliasChange: (a: string) => void;
  // Registry model untuk pilihan (hanya yang enabled yang ditawarkan).
  models: Model[];
  // Error validasi final dari induk. Bila tidak diberikan, component menghitung
  // sendiri lewat validasiPipeline + validasiAlias untuk feedback langsung.
  errors?: string[];
  disabled?: boolean;
  // Awalan id supaya dua ComboBuilder (create + edit) tidak bentrok label htmlFor.
  idPrefix?: string;
}

// Opsi strategi diturunkan dari LABEL_STRATEGI agar label/deskripsi hanya ditulis
// sekali di pustaka dan konsisten antara ComboBuilder, tabel aturan, dan backend.
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

  // Model yang sudah dipilih di entri lain tidak boleh dipilih lagi — mencegah
  // duplikat di UI sebelum validasi menolaknya. Constraint backend juga menolak
  // duplikat, tetapi umpan balik langsung lebih baik daripada error 400.
  const dipakaiDiLain = (index: number, modelId: string) =>
    value.models.some((m, i) => i !== index && m === modelId);

  const ubahStrategi = (s: string) => {
    onChange({ ...value, strategy: s as StrategiCombo });
  };

  const ubahModel = (index: number, modelId: string) => {
    const models = value.models.map((m, i) => (i === index ? modelId : m));
    onChange({ ...value, models });
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

  return (
    <div className="space-y-4" data-testid="combo-builder">
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

      <div>
        <div className="flex items-center justify-between mb-1">
          <span className="text-xs font-semibold text-text-secondary uppercase">
            Model ({value.models.length}/{MODELS_MAX})
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            icon={<Plus className="w-3.5 h-3.5" />}
            onClick={tambahModel}
            disabled={disabled || value.models.length >= MODELS_MAX}
            aria-label="Tambah Model"
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
                <span className="flex-shrink-0 w-6 text-center text-xs font-mono text-text-muted">
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
                  icon={<Trash2 className="w-3.5 h-3.5" />}
                  onClick={() => hapusModel(index)}
                  disabled={disabled}
                  aria-label={`Hapus model urutan ${index + 1}`}
                />
              </div>
            );
          })}
          {value.models.length === 0 && (
            <p className="text-[11px] text-text-muted italic">
              Belum ada model — tambahkan minimal satu model untuk resep combo.
            </p>
          )}
        </div>
        <p className="mt-1 text-[11px] text-text-muted">
          Urutan adalah preferensi fallback. Strategi Prioritas dapat menggeser urutan
          ini karena mengurutkan per prioritas provider.
        </p>
      </div>

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
          className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-white focus:outline-none focus:ring-1 focus:ring-accent"
        />
        <p className="mt-1 text-[11px] text-text-muted">
          Anggaran TOTAL percobaan lintas seluruh model, bukan per model. Contoh:
          anggaran {value.attempts} dengan {value.models.length} model berarti maksimal{' '}
          {value.attempts} request upstream, lalu berhenti.
        </p>
      </div>

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
          className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-white focus:outline-none focus:ring-1 focus:ring-accent"
        />
        <p className="mt-1 text-[11px] text-text-muted flex items-center gap-1">
          <Zap className="w-3 h-3 flex-shrink-0" />
          Opsional. Klien memanggil pengenal ini alih-alih nama model. Wajib ada resep
          pipeline yang sah.
        </p>
      </div>

      {tampilkanError && (
        <div
          role="alert"
          className="p-2 rounded-lg border border-status-error/40 bg-status-error/5 text-xs text-status-error space-y-1"
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
