import React, { useEffect, useState } from 'react';
import type { RoutingRule, Model, Provider } from '../../types';
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
  parsePipeline,
  parsePipelineDariTag,
  bangunPipeline,
  bangunPayloadUpdate,
  validasiPipeline,
  validasiAlias,
  type ComboPipeline,
  type StrategiCombo,
} from '../../lib/rulePipeline';
import { getRuleMode } from './types';

export interface EditRuleDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  rule: RoutingRule | null;
  models: Model[];
  providers: Provider[];
  onSuccess: () => void;
}

interface EditRuleFormState {
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

export const EditRuleDrawer: React.FC<EditRuleDrawerProps> = ({
  isOpen,
  onClose,
  rule,
  models,
  providers,
  onSuccess,
}) => {
  const { toast } = useToast();
  const [editMode, setEditMode] = useState<'model_only' | 'combo_routing'>('model_only');
  const [editTargetModelId, setEditTargetModelId] = useState('');
  const [editTargetProviderId, setEditTargetProviderId] = useState('');
  const [editVirtualAlias, setEditVirtualAlias] = useState('');
  const [editPipeline, setEditPipeline] = useState<ComboPipeline>(pipelineKosong());
  const [showEditAdvanced, setShowEditAdvanced] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const [editRuleForm, setEditRuleForm] = useState<EditRuleFormState>({
    name: '',
    description: '',
    priority: 100,
    strategy: 'priority',
    max_attempts: 3,
    backoff_ms: 200,
    failure_threshold: 5,
    open_duration_ms: 30000,
    half_open_probes: 2,
  });

  useEffect(() => {
    if (!rule || !isOpen) return;

    const detected = getRuleMode(rule, models);
    const mMode = detected.mode;

    let cleanDesc = rule.description || '';
    cleanDesc = cleanDesc
      .replace(/\[combo:alias=[^\]]+\]\s*/g, '')
      .replace(/\[combo:pipeline=\[[\s\S]*?\}\]\s*/g, '')
      .replace(/\[combo:tier2=[^\]]+\]\s*/g, '')
      .replace(/^\[model_only\]\s*/, '')
      .replace(/^\[routing\]\s*/, '')
      .trim();

    const aliasMatch = (rule.description || '').match(/\[combo:alias=([^\]]+)\]/);
    const currAlias =
      (rule.virtual_alias && rule.virtual_alias.trim()) || (aliasMatch ? aliasMatch[1] : '');
    setEditVirtualAlias(currAlias);

    setEditRuleForm({
      name: rule.name || currAlias || '',
      description: cleanDesc,
      priority: rule.priority ?? 100,
      strategy: rule.strategy || 'priority',
      max_attempts: rule.max_attempts ?? 3,
      backoff_ms: rule.backoff_ms ?? 200,
      failure_threshold: rule.failure_threshold ?? 5,
      open_duration_ms: rule.open_duration_ms ?? 30000,
      half_open_probes: rule.half_open_probes ?? 2,
    });

    const pIds = rule.providers ? rule.providers.map((p) => p.provider_id) : [];

    setEditTargetModelId(rule.match_model_id || '');
    setEditTargetProviderId(pIds.length === 1 ? pIds[0] : '');

    if (mMode === 'combo_routing') {
      setEditMode('combo_routing');
      const modelUtama = models.find((m) => m.id === rule.match_model_id);
      const pipeline =
        parsePipeline(rule) ?? parsePipelineDariTag(rule.description, modelUtama?.model_id);
      setEditPipeline(pipeline ?? pipelineKosong());
    } else if (mMode === 'routing') {
      // Jika aturan lama bertipe 'routing' (tanpa jsonb pipeline), lakukan transisi mulus:
      // buat pipeline 1 model dari rule.match_model_id, set editMode ke 'combo_routing'
      setEditMode('combo_routing');
      const modelUtama = models.find(
        (m) => m.id === rule.match_model_id || m.model_id === rule.match_model_id,
      );
      const pipelineModels = modelUtama?.model_id
        ? [modelUtama.model_id]
        : rule.match_model_id
        ? [rule.match_model_id]
        : [];
      const validStrategy: StrategiCombo = (
        ['priority', 'round_robin', 'lowest_latency', 'lowest_cost'] as StrategiCombo[]
      ).includes(rule.strategy as StrategiCombo)
        ? (rule.strategy as StrategiCombo)
        : 'priority';
      setEditPipeline({
        strategy: validStrategy,
        attempts: Math.max(1, rule.max_attempts || 3),
        models: pipelineModels,
      });
      if (!currAlias && rule.name) {
        setEditVirtualAlias(rule.name);
      }
    } else {
      setEditMode('model_only');
      setEditPipeline(pipelineKosong());
    }
  }, [rule, isOpen, models]);

  const handleEditSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!rule) return;
    setIsSubmitting(true);

    try {
      // Mulai dari keadaan tersimpan agar field yang TIDAK diedit formulir ini tetap
      // utuh — ini perbaikan inti bug K2: handler lama menimpa seluruh payload per
      // mode, sehingga operator yang hanya mengganti nama aturan combo kehilangan
      // provider_ids/weights. Setiap mode hanya menimpa field yang dikelolanya.
      const existingProviders = (rule.providers || []).map((p) => p.provider_id);
      const existingWeights: Record<string, number> = {};
      (rule.providers || []).forEach((p) => {
        if (p.weight) existingWeights[p.provider_id] = p.weight;
      });

      let payload: Partial<RoutingRule> = bangunPayloadUpdate(
        {
          name: rule.name || '',
          description: rule.description || '',
          priority: rule.priority ?? 100,
          strategy: rule.strategy || 'priority',
          max_attempts: rule.max_attempts ?? 3,
          backoff_ms: rule.backoff_ms ?? 200,
          failure_threshold: rule.failure_threshold ?? 5,
          open_duration_ms: rule.open_duration_ms ?? 30000,
          half_open_probes: rule.half_open_probes ?? 2,
          match_model_id: rule.match_model_id || '',
          provider_ids: existingProviders,
          weights: existingWeights,
          pipeline: rule.pipeline ?? null,
          virtual_alias: rule.virtual_alias ?? '',
        },
        {
          priority: editRuleForm.priority,
          failure_threshold: editRuleForm.failure_threshold,
          open_duration_ms: editRuleForm.open_duration_ms,
          half_open_probes: editRuleForm.half_open_probes,
          backoff_ms: editRuleForm.backoff_ms,
        },
      );

      const customDesc = editRuleForm.description?.trim();

      if (editMode === 'model_only') {
        if (!editTargetModelId) {
          toast.error('Pilih model target dulu — tanpa model, aturan menjadi catch-all semua model.');
          setIsSubmitting(false);
          return;
        }
        const selModel = models.find((m) => m.id === editTargetModelId);
        const selProv = providers.find((p) => p.id === editTargetProviderId);
        const modelName = selModel?.model_id || editTargetModelId;
        payload = {
          ...payload,
          name: editRuleForm.name || `direct-${modelName.replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: 'priority',
          max_attempts: 1,
          match_model_id: editTargetModelId,
          provider_ids: editTargetProviderId ? [editTargetProviderId] : [],
          weights: {},
          pipeline: null,
          virtual_alias: '',
          description: customDesc
            ? `[model_only] ${customDesc}`
            : `[model_only] Direct 1:1 passthrough ke model ${modelName}${
                selProv ? ` via ${selProv.name}` : ''
              }`,
        };
      } else {
        // editMode === 'combo_routing' (Failover & Combo Cascade terpadu)
        const bersih = bangunPipeline(editPipeline);
        const errors = [
          ...validasiPipeline(editPipeline),
          validasiAlias(editVirtualAlias, editPipeline),
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

        const cleanAlias = editVirtualAlias.trim().toLowerCase();
        const namaModel = bersih.models.map(
          (id) => models.find((m) => m.model_id === id)?.display_name || id,
        );

        payload = {
          ...payload,
          name:
            editRuleForm.name ||
            cleanAlias ||
            `combo-${bersih.models[0].replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: bersih.strategy,
          max_attempts: bersih.attempts,
          match_model_id: modelUtama.id,
          description: customDesc
            ? customDesc
            : `Combo ${LABEL_STRATEGI[bersih.strategy].label}: ${namaModel.join(' -> ')}`,
          pipeline: bersih,
          virtual_alias: cleanAlias,
        };
      }

      await api.routing.update(rule.id, payload);
      const namaUpdate = (payload.name as string) || rule.name;
      toast.success(`Aturan perutean "${namaUpdate}" berhasil diperbarui`);
      onSuccess();
      onClose();
    } catch (err: unknown) {
      toast.error('Gagal memperbarui aturan: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title={`Edit Aturan: ${rule?.name || ''}`}
      maxWidth="2xl"
      footer={
        <div className="flex items-center justify-between gap-3 w-full">
          <Button type="button" variant="ghost" size="md" onClick={onClose}>
            Batal
          </Button>
          <Button
            type="submit"
            form="edit-routing-rule-form"
            variant="primary"
            size="md"
            isLoading={isSubmitting}
            className="flex-1 sm:flex-initial"
          >
            Simpan Perubahan {editMode === 'combo_routing' ? '(Failover & Combo)' : ''}
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
              onClick={() => setEditMode('model_only')}
              className={`py-2 px-2 text-center rounded-lg text-xs font-semibold transition-all flex flex-col sm:flex-row items-center justify-center gap-1 sm:gap-1.5 cursor-pointer ${
                editMode === 'model_only'
                  ? 'bg-sky-500/15 border border-sky-400/60 text-sky-300 shadow-sm shadow-sky-500/10'
                  : 'text-text-muted hover:text-white hover:bg-bg-surface border border-transparent'
              }`}
            >
              <Zap
                className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${
                  editMode === 'model_only' ? 'text-sky-400' : 'text-text-muted'
                }`}
              />
              <span className="text-[11px] sm:text-xs">Model Only</span>
            </button>
            <button
              type="button"
              onClick={() => setEditMode('combo_routing')}
              className={`py-2 px-2 text-center rounded-lg text-xs font-semibold transition-all flex flex-col sm:flex-row items-center justify-center gap-1 sm:gap-1.5 cursor-pointer ${
                editMode === 'combo_routing'
                  ? 'bg-accent/15 border border-accent/60 text-accent shadow-sm shadow-accent/10'
                  : 'text-text-muted hover:text-white hover:bg-bg-surface border border-transparent'
              }`}
            >
              <Layers
                className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${
                  editMode === 'combo_routing' ? 'text-accent' : 'text-text-muted'
                }`}
              />
              <span className="text-[11px] sm:text-xs">Failover &amp; Combo Cascade</span>
            </button>
          </div>
        </div>

        <form id="edit-routing-rule-form" noValidate onSubmit={handleEditSave} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1">
                Nama Aturan
              </label>
              <input
                type="text"
                placeholder={editMode === 'model_only' ? 'direct-gpt4o' : 'failover-smart-combo'}
                value={editRuleForm.name}
                onChange={(e) => setEditRuleForm({ ...editRuleForm, name: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white text-xs placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1">
                Deskripsi (Opsional)
              </label>
              <input
                type="text"
                placeholder="Catatan tujuan atau spesifikasi aturan..."
                value={editRuleForm.description}
                onChange={(e) =>
                  setEditRuleForm({ ...editRuleForm, description: e.target.value })
                }
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white text-xs placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
              />
            </div>
          </div>

          {/* Mode 1: Model Only Form Fields */}
          {editMode === 'model_only' && (
            <div className="space-y-3 pt-1 border-t border-border/40">
              <Select
                label="Target Model"
                required
                value={editTargetModelId}
                onChange={(val) => {
                  setEditTargetModelId(val);
                  setEditTargetProviderId('');
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
                value={editTargetProviderId}
                onChange={(val) => setEditTargetProviderId(val)}
                placeholder="-- Bawaan (Semua Provider Penyedia Model) --"
                options={[
                  { value: '', label: '-- Bawaan (Semua Provider Penyedia Model) --' },
                  ...providers.map((p) => {
                    const selModel = models.find((m) => m.id === editTargetModelId);
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
          {editMode === 'combo_routing' && (
            <div className="space-y-4 pt-1 border-t border-border/40">
              <ComboBuilder
                idPrefix="edit-combo"
                value={editPipeline}
                onChange={setEditPipeline}
                alias={editVirtualAlias}
                onAliasChange={setEditVirtualAlias}
                models={models}
              />
            </div>
          )}

          {/* Pengaturan Lanjutan (Retry, Prioritas & Circuit Breaker) - Collapsible Accordion */}
          <div className="pt-2 border-t border-border/40">
            <button
              type="button"
              onClick={() => setShowEditAdvanced((prev) => !prev)}
              className="flex items-center justify-between w-full py-1.5 px-2 text-xs font-semibold text-text-secondary hover:text-white rounded-lg hover:bg-bg-surface-2 transition-colors cursor-pointer"
            >
              <span className="flex items-center gap-1.5">
                <RotateCcw className="w-3.5 h-3.5 text-text-muted" />
                ⚙️ Pengaturan Lanjutan (Retry, Prioritas &amp; Circuit Breaker)
              </span>
              {showEditAdvanced ? (
                <ChevronUp className="w-4 h-4 text-text-muted" />
              ) : (
                <ChevronDown className="w-4 h-4 text-text-muted" />
              )}
            </button>

            {showEditAdvanced && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">
                    Maksimal Percobaan
                  </label>
                  <input
                    type="number"
                    min="1"
                    max="10"
                    value={editRuleForm.max_attempts}
                    onChange={(e) =>
                      setEditRuleForm({
                        ...editRuleForm,
                        max_attempts: parseInt(e.target.value) || 3,
                      })
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
                    value={editRuleForm.backoff_ms}
                    onChange={(e) =>
                      setEditRuleForm({
                        ...editRuleForm,
                        backoff_ms: parseInt(e.target.value) || 200,
                      })
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
                    value={editRuleForm.priority}
                    onChange={(e) =>
                      setEditRuleForm({
                        ...editRuleForm,
                        priority: parseInt(e.target.value) || 100,
                      })
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
                    value={editRuleForm.failure_threshold}
                    onChange={(e) =>
                      setEditRuleForm({
                        ...editRuleForm,
                        failure_threshold: parseInt(e.target.value) || 5,
                      })
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
