import React, { useState } from 'react';
import type { Model, Provider, RoutingRule } from '../../types';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { ComboBuilder } from './ComboBuilder';
import { useToast } from '../../context/ToastContext';
import { api } from '../../api/client';
import {
  Zap,
  Layers,
  RotateCcw,
  ChevronDown,
  ChevronUp,
} from 'lucide-react';
import {
  LABEL_STRATEGI,
  pipelineKosong,
  bangunPipeline,
  validasiPipeline,
  validasiAlias,
  type ComboPipeline,
} from '../../lib/rulePipeline';

export interface CreateRuleDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  models: Model[];
  providers: Provider[];
  onSuccess: () => void;
}

interface NewRuleState {
  name: string;
  description: string;
  priority: number;
  strategy: string;
  max_attempts: number;
  backoff_ms: number;
  failure_threshold: number;
  open_duration_ms: number;
  half_open_probes: number;
}

const INITIAL_RULE_STATE: NewRuleState = {
  name: '',
  description: '',
  priority: 100,
  strategy: 'priority',
  max_attempts: 3,
  backoff_ms: 200,
  failure_threshold: 5,
  open_duration_ms: 30000,
  half_open_probes: 2,
};

export const CreateRuleDrawer: React.FC<CreateRuleDrawerProps> = ({
  isOpen,
  onClose,
  models,
  providers,
  onSuccess,
}) => {
  const { toast } = useToast();
  const [mode, setMode] = useState<'model_only' | 'combo_routing'>('model_only');
  const [targetModelId, setTargetModelId] = useState('');
  const [targetProviderId, setTargetProviderId] = useState('');
  const [virtualAlias, setVirtualAlias] = useState('');
  const [createPipeline, setCreatePipeline] = useState<ComboPipeline>(pipelineKosong());
  const [newRule, setNewRule] = useState<NewRuleState>(INITIAL_RULE_STATE);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const resetForm = () => {
    setNewRule(INITIAL_RULE_STATE);
    setMode('model_only');
    setTargetModelId('');
    setTargetProviderId('');
    setVirtualAlias('');
    setCreatePipeline(pipelineKosong());
    setShowAdvanced(false);
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    try {
      let payload: Partial<RoutingRule> = {
        priority: newRule.priority,
        failure_threshold: newRule.failure_threshold,
        open_duration_ms: newRule.open_duration_ms,
        half_open_probes: newRule.half_open_probes,
        backoff_ms: newRule.backoff_ms,
      };

      if (mode === 'model_only') {
        if (!targetModelId) {
          toast.error('Pilih model target dulu — tanpa model, aturan menjadi catch-all semua model.');
          setIsSubmitting(false);
          return;
        }
        const selModel = models.find((m) => m.id === targetModelId);
        const selProv = providers.find((p) => p.id === targetProviderId);
        const modelName = selModel?.model_id || targetModelId;
        payload = {
          ...payload,
          name: newRule.name || `direct-${modelName.replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: 'priority',
          max_attempts: 1,
          match_model_id: targetModelId,
          provider_ids: targetProviderId ? [targetProviderId] : undefined,
          pipeline: null,
          virtual_alias: '',
          description: newRule.description?.trim()
            ? `[model_only] ${newRule.description.trim()}`
            : `[model_only] Direct 1:1 passthrough ke model ${modelName}${selProv ? ` via ${selProv.name}` : ''}`,
        };
      } else {
        // mode === 'combo_routing' (Failover & Combo Cascade terpadu)
        const bersih = bangunPipeline(createPipeline);
        const errors = [
          ...validasiPipeline(createPipeline),
          validasiAlias(virtualAlias, createPipeline),
        ].filter((errItem): errItem is string => typeof errItem === 'string');
        if (errors.length > 0) {
          toast.error(errors[0]);
          setIsSubmitting(false);
          return;
        }
        const modelUtama = models.find((m) => m.model_id === bersih.models[0]);
        if (!modelUtama) {
          toast.error(`Model "${bersih.models[0]}" tidak terdaftar di registry.`);
          setIsSubmitting(false);
          return;
        }

        const cleanAlias = virtualAlias.trim().toLowerCase();
        const namaModel = bersih.models.map(
          (id) => models.find((m) => m.model_id === id)?.display_name || id,
        );

        payload = {
          ...payload,
          name:
            newRule.name ||
            cleanAlias ||
            `combo-${bersih.models[0].replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: bersih.strategy,
          max_attempts: bersih.attempts,
          match_model_id: modelUtama.id,
          description: newRule.description?.trim()
            ? newRule.description.trim()
            : `Combo ${LABEL_STRATEGI[bersih.strategy].label}: ${namaModel.join(' -> ')}`,
          pipeline: bersih,
          virtual_alias: cleanAlias,
        };
      }

      await api.routing.create(payload);
      resetForm();
      toast.success('Aturan perutean cerdas berhasil diterapkan');
      onSuccess();
      onClose();
    } catch (err: unknown) {
      toast.error('Gagal membuat aturan: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={handleClose}
      title="Buat Aturan Perutean Cerdas (Routing Rule)"
      maxWidth="2xl"
      footer={
        <div className="flex items-center justify-between gap-3 w-full">
          <Button type="button" variant="ghost" size="md" onClick={handleClose}>
            Batal
          </Button>
          <Button
            type="submit"
            form="create-routing-rule-form"
            variant="primary"
            size="md"
            isLoading={isSubmitting}
            className="flex-1 sm:flex-initial"
          >
            Simpan Aturan {mode === 'combo_routing' ? '(Failover & Combo)' : ''}
          </Button>
        </div>
      }
    >
      <div className="space-y-5 text-xs">
        {/* Mode Selector */}
        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5">
            Pilih Mode Routing
          </label>
          <div className="grid grid-cols-2 gap-1.5 p-1 bg-bg-surface-2 border border-border rounded-xl">
            <button
              type="button"
              onClick={() => setMode('model_only')}
              className={`py-2 px-2 text-center rounded-lg text-xs font-semibold transition-all flex flex-col sm:flex-row items-center justify-center gap-1 sm:gap-1.5 cursor-pointer ${
                mode === 'model_only'
                  ? 'bg-sky-500/15 border border-sky-400/60 text-sky-300 shadow-sm shadow-sky-500/10'
                  : 'text-text-muted hover:text-white hover:bg-bg-surface border border-transparent'
              }`}
            >
              <Zap
                className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${
                  mode === 'model_only' ? 'text-sky-400' : 'text-text-muted'
                }`}
              />
              <span className="text-[11px] sm:text-xs">Model Only</span>
            </button>
            <button
              type="button"
              onClick={() => setMode('combo_routing')}
              className={`py-2 px-2 text-center rounded-lg text-xs font-semibold transition-all flex flex-col sm:flex-row items-center justify-center gap-1 sm:gap-1.5 cursor-pointer ${
                mode === 'combo_routing'
                  ? 'bg-accent/15 border border-accent/60 text-accent shadow-sm shadow-accent/10'
                  : 'text-text-muted hover:text-white hover:bg-bg-surface border border-transparent'
              }`}
            >
              <Layers
                className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${
                  mode === 'combo_routing' ? 'text-accent' : 'text-text-muted'
                }`}
              />
              <span className="text-[11px] sm:text-xs">Failover &amp; Combo Cascade</span>
            </button>
          </div>
        </div>

        <form id="create-routing-rule-form" noValidate onSubmit={handleCreate} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1">
                Nama Aturan
              </label>
              <input
                type="text"
                placeholder={mode === 'model_only' ? 'direct-gpt4o' : 'failover-smart-combo'}
                value={newRule.name}
                onChange={(e) => setNewRule({ ...newRule, name: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white text-xs placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
              />
              <span className="text-[10px] text-text-muted mt-0.5 block">
                Kosongkan untuk nama otomatis berdasarkan mode &amp; model.
              </span>
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1">
                Deskripsi (Opsional)
              </label>
              <input
                type="text"
                placeholder="Catatan tujuan atau spesifikasi aturan..."
                value={newRule.description}
                onChange={(e) => setNewRule({ ...newRule, description: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white text-xs placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
              />
            </div>
          </div>

          {/* Mode 1: Model Only Form Fields */}
          {mode === 'model_only' && (
            <div className="space-y-3 pt-1 border-t border-border/40">
              <Select
                label="Target Model"
                required
                value={targetModelId}
                onChange={(val) => {
                  setTargetModelId(val);
                  setTargetProviderId('');
                }}
                placeholder="-- Pilih Model Target --"
                options={models.map((m) => {
                  const provNames = m.providers
                    ?.map((p) => p.display_name || p.provider_name)
                    .join(', ');
                  return {
                    value: m.id,
                    label: `${m.display_name} (${m.model_id})`,
                    description: provNames
                      ? `Tersedia di: ${provNames}`
                      : m.family
                      ? `Keluarga: ${m.family}`
                      : undefined,
                  };
                })}
              />
              <Select
                label="Target Provider (Opsional)"
                value={targetProviderId}
                onChange={(val) => setTargetProviderId(val)}
                placeholder="-- Bawaan (Semua Provider Penyedia Model) --"
                options={[
                  { value: '', label: '-- Bawaan (Semua Provider Penyedia Model) --' },
                  ...providers.map((p) => {
                    const selModel = models.find((m) => m.id === targetModelId);
                    const matched = selModel?.providers?.find((mp) => mp.provider_id === p.id);
                    return {
                      value: p.id,
                      label: `${p.display_name || p.name} (${p.kind})${
                        matched ? ' ✓ Menyediakan Model' : ''
                      }`,
                      description: matched
                        ? `Model upstream: ${matched.upstream_model_name}`
                        : undefined,
                    };
                  }),
                ]}
              />
            </div>
          )}

          {/* Mode 2: Failover & Combo Cascade Form Fields */}
          {mode === 'combo_routing' && (
            <div className="space-y-4 pt-1 border-t border-border/40">
              <ComboBuilder
                idPrefix="create-combo"
                value={createPipeline}
                onChange={setCreatePipeline}
                alias={virtualAlias}
                onAliasChange={setVirtualAlias}
                models={models}
              />
            </div>
          )}

          {/* Pengaturan Lanjutan (Retry, Prioritas & Circuit Breaker) - Collapsible Accordion */}
          <div className="pt-2 border-t border-border/40">
            <button
              type="button"
              onClick={() => setShowAdvanced((prev) => !prev)}
              className="flex items-center justify-between w-full py-1.5 px-2 text-xs font-semibold text-text-secondary hover:text-white rounded-lg hover:bg-bg-surface-2 transition-colors cursor-pointer"
            >
              <span className="flex items-center gap-1.5">
                <RotateCcw className="w-3.5 h-3.5 text-text-muted" />
                ⚙️ Pengaturan Lanjutan (Retry, Prioritas &amp; Circuit Breaker)
              </span>
              {showAdvanced ? (
                <ChevronUp className="w-4 h-4 text-text-muted" />
              ) : (
                <ChevronDown className="w-4 h-4 text-text-muted" />
              )}
            </button>

            {showAdvanced && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">
                    Maksimal Percobaan
                  </label>
                  <input
                    type="number"
                    min="1"
                    max="10"
                    value={newRule.max_attempts}
                    onChange={(e) =>
                      setNewRule({ ...newRule, max_attempts: parseInt(e.target.value) || 3 })
                    }
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">
                    Jeda Backoff (ms)
                  </label>
                  <input
                    type="number"
                    min="0"
                    value={newRule.backoff_ms}
                    onChange={(e) =>
                      setNewRule({ ...newRule, backoff_ms: parseInt(e.target.value) || 200 })
                    }
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">
                    Prioritas Evaluasi
                  </label>
                  <input
                    type="number"
                    value={newRule.priority}
                    onChange={(e) =>
                      setNewRule({ ...newRule, priority: parseInt(e.target.value) || 100 })
                    }
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">
                    Ambang Circuit Breaker
                  </label>
                  <input
                    type="number"
                    value={newRule.failure_threshold}
                    onChange={(e) =>
                      setNewRule({ ...newRule, failure_threshold: parseInt(e.target.value) || 5 })
                    }
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                  />
                </div>
              </div>
            )}
          </div>
        </form>
      </div>
    </Drawer>
  );
};
