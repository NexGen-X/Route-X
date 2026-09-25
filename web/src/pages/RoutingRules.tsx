import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RoutingRule, CircuitBreakerStatus, Model, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { Checkbox } from '../components/common/Checkbox';
import { Plus, Trash2, ZapOff, RotateCcw, RefreshCw, Zap, Shuffle, Layers, Edit2, Terminal, ShieldCheck, ArrowRight, ChevronDown, ChevronUp, Server, CheckCircle2 } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { ComboBuilder } from '../components/routing/ComboBuilder';
import {
  LABEL_STRATEGI,
  pipelineKosong,
  modeAturan,
  parsePipeline,
  parsePipelineDariTag,
  bangunPipeline,
  bangunPayloadUpdate,
  validasiPipeline,
  validasiAlias,
  type ComboPipeline,
  type StrategiCombo,
} from '../lib/rulePipeline';

export const RoutingRules: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [breakers, setBreakers] = useState<CircuitBreakerStatus[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [isRulesLoading, setIsRulesLoading] = useState(true);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isBreakersLoading, setIsBreakersLoading] = useState(false);
  const [isProvidersDrawerOpen, setIsProvidersDrawerOpen] = useState(false);
  const [selectedRule, setSelectedRule] = useState<any>(null);
  const [ruleProviders, setRuleProviders] = useState<string[]>([]);
  const [ruleWeights, setRuleWeights] = useState<Record<string, number>>({});

  // Edit Drawer States
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<RoutingRule | null>(null);
  const [editMode, setEditMode] = useState<'model_only' | 'combo_routing'>('model_only');
  const [editTargetModelId, setEditTargetModelId] = useState('');
  const [editTargetProviderId, setEditTargetProviderId] = useState('');
  const [editVirtualAlias, setEditVirtualAlias] = useState('');
  // Resep combo untuk form edit. Dimuat dari kolom pipeline aturan; bila aturan
  // produksi lama belum dikonversi, jatuh ke tag [combo:...] di description.
  const [editPipeline, setEditPipeline] = useState<ComboPipeline>(pipelineKosong());
  const [editRuleForm, setEditRuleForm] = useState({
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

  const [rulesError, setRulesError] = useState<string | null>(null);
  const [breakersError, setBreakersError] = useState<string | null>(null);

  // 2-Mode Form States (Create)
  const [mode, setMode] = useState<'model_only' | 'combo_routing'>('model_only');
  const [targetModelId, setTargetModelId] = useState('');
  const [targetProviderId, setTargetProviderId] = useState('');
  const [virtualAlias, setVirtualAlias] = useState('');
  // Resep combo untuk form create. pipelineKosong memaksa operator memilih model
  // sebelum menyimpan (models sengaja kosong, validasi menolak models < 1).
  const [createPipeline, setCreatePipeline] = useState<ComboPipeline>(pipelineKosong());

  const [newRule, setNewRule] = useState({
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

  const [showAdvanced, setShowAdvanced] = useState(false);
  const [showEditAdvanced, setShowEditAdvanced] = useState(false);

  const loadRules = async () => {
    setIsRulesLoading(true);
    try {
      const [resRules, resModels, resProv] = await Promise.all([
        api.routing.list(),
        api.models.list(),
        api.providers.list(),
      ]);
      setRules(resRules.items || []);
      setModels(resModels.items || []);
      setProviders(resProv.items || []);
      setRulesError(null);
    } catch (err) {
      setRulesError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsRulesLoading(false);
    }
  };

  const getRuleMode = (r: RoutingRule): { mode: string; label: string; badgeVariant: 'info' | 'lime' | 'warn'; detail: string; alias?: string | null; pipelineCount?: number } => {
    // Sumber kebenaran mode adalah kolom pipeline (migrasi 0014), BUKAN max_attempts.
    // Memakai max_attempts adalah bug K2: aturan combo dengan anggaran 1 salah dibaca
    // model_only, lalu simpan berikutnya menghapus provider_ids/weights aturan tersebut.
    // Tag [combo:...] di description adalah cadangan untuk aturan produksi lama
    // (mis. raute-x) yang belum dikonversi ke kolom jsonb.
    if (modeAturan(r) === 'combo') {
      const modelUtama = models.find((m) => m.id === r.match_model_id);
      const pipeline = parsePipeline(r) ?? parsePipelineDariTag(r.description, modelUtama?.model_id);
      const aliasTag = (r.description || '').match(/\[combo:alias=([^\]]+)\]/);
      const alias = r.virtual_alias ?? (aliasTag ? aliasTag[1] : null);
      const jumlah = pipeline?.models.length ?? 1;
      return {
        mode: 'combo_routing',
        label: 'COMBO PIPELINE',
        badgeVariant: 'lime',
        detail: alias
          ? `Virtual Endpoint: ${alias} · ${jumlah} model`
          : `${jumlah} model · ${pipeline ? LABEL_STRATEGI[pipeline.strategy].label : 'Combo'}`,
        alias,
        pipelineCount: jumlah,
      };
    }
    if (r.max_attempts === 1 || (r.description || '').includes('[model_only]')) {
      return { mode: 'model_only', label: 'MODEL ONLY', badgeVariant: 'info', detail: 'Direct 1:1 Passthrough' };
    }
    return { mode: 'routing', label: 'ROUTING', badgeVariant: 'warn', detail: `${r.max_attempts} percobaan failover` };
  };

  const loadBreakers = async () => {
    setIsBreakersLoading(true);
    try {
      const res = await api.breakers.list();
      setBreakers(res.items || []);
      setBreakersError(null);
    } catch (err) {
      setBreakersError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsBreakersLoading(false);
    }
  };

  useEffect(() => {
    loadRules();
    loadBreakers();
  }, []);

  const handleResetBreaker = async (providerId: string, model: string) => {
    try {
      await api.breakers.reset(providerId, model);
      toast.success('Circuit breaker berhasil direset');
      loadBreakers();
    } catch (err) {
      toast.error('Gagal mereset circuit breaker: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleToggle = async (r: RoutingRule) => {
    try {
      await api.routing.toggle(r.id, !r.enabled);
      toast.success(`Aturan ${!r.enabled ? 'diaktifkan' : 'dinonaktifkan'}`);
      loadRules();
    } catch (err) {
      toast.error('Gagal toggle aturan: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDelete = async (id: string) => {
    const confirmed = await confirmModal({
      title: 'Hapus Aturan Routing',
      message: 'Apakah Anda yakin ingin menghapus aturan perutean ini? Jalur fallback akan disesuaikan secara dinamis.',
      danger: true,
      confirmText: 'Ya, Hapus Aturan',
    });
    if (!confirmed) return;

    try {
      await api.routing.delete(id);
      toast.success('Aturan perutean berhasil dihapus');
      loadRules();
    } catch (err) {
      toast.error('Gagal menghapus: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleOpenCreateDrawer = () => {
    setNewRule({
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
    setMode('model_only');
    setTargetModelId('');
    setTargetProviderId('');
    setVirtualAlias('');
    setCreatePipeline(pipelineKosong());
    setIsCreateOpen(true);
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      let payload: any = {
        priority: newRule.priority,
        failure_threshold: newRule.failure_threshold,
        open_duration_ms: newRule.open_duration_ms,
        half_open_probes: newRule.half_open_probes,
        backoff_ms: newRule.backoff_ms,
      };

      if (mode === 'model_only') {
        if (!targetModelId) {
          toast.error('Pilih model target dulu — tanpa model, aturan menjadi catch-all semua model.');
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
        ].filter((e): e is string => typeof e === 'string');
        if (errors.length > 0) {
          toast.error(errors[0]);
          return;
        }
        const modelUtama = models.find((m) => m.model_id === bersih.models[0]);
        if (!modelUtama) {
          toast.error(`Model "${bersih.models[0]}" tidak terdaftar di registry.`);
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
      setIsCreateOpen(false);
      setNewRule({
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
      setVirtualAlias('');
      setCreatePipeline(pipelineKosong());
      toast.success('Aturan perutean cerdas berhasil diterapkan');
      loadRules();
    } catch (err: any) {
      toast.error('Gagal membuat aturan: ' + (err.message || err));
    }
  };

  const handleOpenEdit = (r: RoutingRule) => {
    const detected = getRuleMode(r);
    const mMode = detected.mode as 'model_only' | 'routing' | 'combo_routing';
    setEditingRule(r);

    let cleanDesc = r.description || '';
    cleanDesc = cleanDesc
      .replace(/\[combo:alias=[^\]]+\]\s*/g, '')
      .replace(/\[combo:pipeline=\[[\s\S]*?\}\]\s*/g, '')
      .replace(/\[combo:tier2=[^\]]+\]\s*/g, '')
      .replace(/^\[model_only\]\s*/, '')
      .replace(/^\[routing\]\s*/, '')
      .trim();

    const aliasMatch = (r.description || '').match(/\[combo:alias=([^\]]+)\]/);
    const currAlias = (r.virtual_alias && r.virtual_alias.trim()) || (aliasMatch ? aliasMatch[1] : '');
    setEditVirtualAlias(currAlias);

    setEditRuleForm({
      name: r.name || currAlias || '',
      description: cleanDesc,
      priority: r.priority ?? 100,
      strategy: r.strategy || 'priority',
      max_attempts: r.max_attempts ?? 3,
      backoff_ms: r.backoff_ms ?? 200,
      failure_threshold: r.failure_threshold ?? 5,
      open_duration_ms: r.open_duration_ms ?? 30000,
      half_open_probes: r.half_open_probes ?? 2,
    });

    const pIds = r.providers ? r.providers.map((p: any) => p.provider_id) : [];

    setEditTargetModelId(r.match_model_id || '');
    setEditTargetProviderId(pIds.length === 1 ? pIds[0] : '');

    if (mMode === 'combo_routing') {
      setEditMode('combo_routing');
      const modelUtama = models.find((m) => m.id === r.match_model_id);
      const pipeline = parsePipeline(r) ?? parsePipelineDariTag(r.description, modelUtama?.model_id);
      setEditPipeline(pipeline ?? pipelineKosong());
    } else if (mMode === 'routing') {
      // Jika aturan lama bertipe 'routing' (tanpa jsonb pipeline), lakukan transisi mulus:
      // buat pipeline 1 model dari r.match_model_id, set editMode ke 'combo_routing',
      // sehingga operator dapat langsung melihat dan mengeditnya di antarmuka failover & combo terpadu.
      setEditMode('combo_routing');
      const modelUtama = models.find((m) => m.id === r.match_model_id || m.model_id === r.match_model_id);
      const pipelineModels = modelUtama?.model_id ? [modelUtama.model_id] : (r.match_model_id ? [r.match_model_id] : []);
      const validStrategy: StrategiCombo = (
        ['priority', 'round_robin', 'lowest_latency', 'lowest_cost'] as StrategiCombo[]
      ).includes(r.strategy as StrategiCombo)
        ? (r.strategy as StrategiCombo)
        : 'priority';
      setEditPipeline({
        strategy: validStrategy,
        attempts: Math.max(1, r.max_attempts || 3),
        models: pipelineModels,
      });
      if (!currAlias && r.name) {
        setEditVirtualAlias(r.name);
      }
    } else {
      setEditMode('model_only');
      setEditPipeline(pipelineKosong());
    }

    setIsEditOpen(true);
  };

  const handleEditSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingRule) return;

    try {
      // Mulai dari keadaan tersimpan agar field yang TIDAK diedit formulir ini tetap
      // utuh — ini perbaikan inti bug K2: handler lama menimpa seluruh payload per
      // mode, sehingga operator yang hanya mengganti nama aturan combo kehilangan
      // provider_ids/weights. Setiap mode hanya menimpa field yang dikelolanya.
      const existingProviders = (editingRule.providers || []).map((p: any) => p.provider_id);
      const existingWeights: Record<string, number> = {};
      (editingRule.providers || []).forEach((p: any) => {
        if (p.weight) existingWeights[p.provider_id] = p.weight;
      });

      let payload: Record<string, unknown> = bangunPayloadUpdate(
        {
          name: editingRule.name || '',
          description: editingRule.description || '',
          priority: editingRule.priority ?? 100,
          strategy: editingRule.strategy || 'priority',
          max_attempts: editingRule.max_attempts ?? 3,
          backoff_ms: editingRule.backoff_ms ?? 200,
          failure_threshold: editingRule.failure_threshold ?? 5,
          open_duration_ms: editingRule.open_duration_ms ?? 30000,
          half_open_probes: editingRule.half_open_probes ?? 2,
          match_model_id: editingRule.match_model_id || '',
          provider_ids: existingProviders,
          weights: existingWeights,
          pipeline: editingRule.pipeline ?? null,
          virtual_alias: editingRule.virtual_alias ?? '',
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
          // Model Only tidak punya resep combo maupun alias — keduanya dikosongkan
          // eksplisit. Tanpa ini, beralih dari aturan combo ke model_only akan
          // menyisakan pipeline lama (backend menolak alias tanpa pipeline dengan 400
          // bila aliasnya tidak ikut dibersihkan).
          pipeline: null,
          virtual_alias: '',
          description: customDesc
            ? `[model_only] ${customDesc}`
            : `[model_only] Direct 1:1 passthrough ke model ${modelName}${selProv ? ` via ${selProv.name}` : ''}`,
        };
      } else {
        // editMode === 'combo_routing' (Failover & Combo Cascade terpadu)
        const bersih = bangunPipeline(editPipeline);
        const errors = [
          ...validasiPipeline(editPipeline),
          validasiAlias(editVirtualAlias, editPipeline),
        ].filter((e): e is string => typeof e === 'string');
        if (errors.length > 0) {
          toast.error(errors[0]);
          return;
        }
        const modelUtama = models.find((m) => m.model_id === bersih.models[0]);
        if (!modelUtama) {
          toast.error(`Model "${bersih.models[0]}" tidak terdaftar di registry.`);
          return;
        }

        const cleanAlias = editVirtualAlias.trim().toLowerCase();
        const namaModel = bersih.models.map(
          (id) => models.find((m) => m.model_id === id)?.display_name || id,
        );

        // Menyimpan form combo menulis resep ke kolom jsonb + virtual_alias. Ini
        // sekaligus mengonversi aturan produksi lama (tag description) ke jalur baru:
        // description bersih dari tag, dan alias berpindah dari tag ke kolom.
        // pendaftaran alias lewat api.models.addAlias dihentikan.
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

      await api.routing.update(editingRule.id, payload);
      setIsEditOpen(false);
      setEditingRule(null);
      const namaUpdate = (payload.name as string) || editingRule.name;
      toast.success(`Aturan perutean "${namaUpdate}" berhasil diperbarui`);
      loadRules();
    } catch (err: any) {
      toast.error('Gagal memperbarui aturan: ' + (err.message || err));
    }
  };

  const handleOpenProviders = (r: any) => {
    setSelectedRule(r);
    const pIds = r.providers ? r.providers.map((p: any) => p.provider_id) : (r.provider_ids || []);
    const pWeights: Record<string, number> = {};
    if (r.providers) {
      r.providers.forEach((p: any) => {
        if (p.weight) pWeights[p.provider_id] = p.weight;
      });
    } else if (r.weights) {
      Object.assign(pWeights, r.weights);
    }
    setRuleProviders(pIds);
    setRuleWeights(pWeights);
    setIsProvidersDrawerOpen(true);
  };

  const handleSaveProviders = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.routing.setProviders(selectedRule.id, { provider_ids: ruleProviders, weights: ruleWeights });
      toast.success('Providers & weights berhasil diperbarui.');
      setIsProvidersDrawerOpen(false);
      loadRules();
    } catch (err: any) {
      toast.error('Gagal menyimpan providers: ' + (err.message || err));
    }
  };

  const toggleProvider = (pid: string) => {
    setRuleProviders(prev => prev.includes(pid) ? prev.filter(p => p !== pid) : [...prev, pid]);
  };

  const updateWeight = (pid: string, w: number) => {
    setRuleWeights(prev => ({ ...prev, [pid]: w }));
  };

  const openBreakers = breakers.filter((b) => b.state === 'open');
  const halfOpenBreakers = breakers.filter((b) => b.state === 'half-open');
  const totalAbnormalBreakers = openBreakers.length + halfOpenBreakers.length;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Routing Rules Engine"
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={handleOpenCreateDrawer}
            icon={<Plus className="w-4 h-4" />}
            className="w-full sm:w-auto justify-center"
          >
            Tambah Aturan
          </Button>
        }
      />

      {rulesError && (
        <div className="mb-4">
          <QueryError message={rulesError} onRetry={() => void loadRules()} />
        </div>
      )}

      {isRulesLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(3)].map((_, idx) => (
            <Card key={idx} className="p-5 animate-pulse space-y-4">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-bg-surface-2" />
                <div className="space-y-1.5 flex-1">
                  <div className="h-4 bg-bg-surface-2 rounded w-2/3" />
                  <div className="h-3 bg-bg-surface-2 rounded w-1/3" />
                </div>
              </div>
              <div className="h-12 bg-bg-surface-2 rounded" />
            </Card>
          ))}
        </div>
      ) : rules.length === 0 ? (
        <Card className="p-10 text-center rounded-box bg-bg-surface-1 border border-border/60">
          <div className="w-12 h-12 rounded-2xl bg-sky-500/10 border border-sky-500/20 text-sky-400 flex items-center justify-center mx-auto mb-3">
            <Layers className="w-6 h-6" />
          </div>
          <h3 className="text-base font-bold text-white mb-1">Belum Ada Aturan Perutean Aktif</h3>
          <p className="text-xs text-text-secondary max-w-md mx-auto mb-5 leading-relaxed">
            Konfigurasikan aturan perutean pertama: Passthrough langsung 1:1 ke upstream, Failover multi-provider otomatis, atau Cascade hemat biaya.
          </p>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="h-9 px-4 text-xs font-semibold mx-auto"
          >
            Tambah Aturan Pertama
          </Button>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {rules.map((r) => {
          const ruleMode = getRuleMode(r);
          const matchedModel = r.match_model_id
            ? models.find((m) => m.id === r.match_model_id || m.model_id === r.match_model_id)
            : null;
          const cleanCustomDesc = r.description
            ? r.description
                .replace(/\[combo:alias=[^\]]+\]\s*/g, '')
      .replace(/\[combo:pipeline=\[[\s\S]*?\}\]\s*/g, '')
                .replace(/\[combo:tier2=[^\]]+\]\s*/g, '')
                .replace(/^\[(model_only|routing)\]\s*/, '')
                .trim()
            : '';
          const isAutoDesc =
            cleanCustomDesc.startsWith('Direct 1:1 passthrough') ||
            cleanCustomDesc.startsWith('Smart Tiered Cascade:') ||
            cleanCustomDesc.startsWith('Failover multi-provider') ||
            cleanCustomDesc.startsWith('Combo ');

          const modelDisplayName = matchedModel
            ? (matchedModel.display_name && matchedModel.display_name !== matchedModel.model_id
                ? `${matchedModel.display_name} (${matchedModel.model_id})`
                : matchedModel.model_id)
            : r.match_model_id || 'Semua Model (Catch-All)';

          return (
            <Card key={r.id} className="p-4 sm:p-5 flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className={`w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0 ${
                      ruleMode.mode === 'model_only'
                        ? 'bg-sky-500/10 border border-sky-500/20 text-sky-400'
                        : ruleMode.mode === 'combo_routing'
                        ? 'bg-accent/10 border border-accent/20 text-accent'
                        : 'bg-purple-500/10 border border-purple-500/20 text-purple-400'
                    }`}>
                      {ruleMode.mode === 'model_only' ? (
                        <Zap className="w-5 h-5" />
                      ) : ruleMode.mode === 'combo_routing' ? (
                        <Layers className="w-5 h-5" />
                      ) : (
                        <Shuffle className="w-5 h-5" />
                      )}
                    </div>
                    <div className="min-w-0">
                      <h4 className="text-sm font-bold text-white truncate">{r.name}</h4>
                      <div className="flex items-center gap-2 mt-1">
                        <Badge variant={ruleMode.badgeVariant}>
                          {ruleMode.label}
                        </Badge>
                        <span className="text-[10px] text-text-muted font-mono">Prio: {r.priority}</span>
                      </div>
                    </div>
                  </div>
                  <Badge variant={r.enabled ? 'success' : 'neutral'}>
                    {r.enabled ? 'active' : 'disabled'}
                  </Badge>
                </div>

                {cleanCustomDesc && !isAutoDesc && (
                  <p className="mt-3 text-xs text-text-secondary leading-relaxed bg-bg-surface-2/50 p-2 rounded border border-border/40">
                    {cleanCustomDesc}
                  </p>
                )}

                {/* 1. Mode MODEL ONLY: Visual Passthrough Streamlined */}
                {ruleMode.mode === 'model_only' && (
                  <div className="mt-3.5 p-3 rounded-lg bg-bg-surface-2/60 border border-sky-500/20 space-y-2.5">
                    <div className="flex items-center justify-between text-[10px] text-text-muted font-mono">
                      <span className="flex items-center gap-1 text-sky-400 font-semibold">
                        <Zap className="w-3 h-3" /> Direct 1:1 Passthrough
                      </span>
                      <span className="text-emerald-400 flex items-center gap-1 font-sans">
                        <CheckCircle2 className="w-3 h-3" /> 1 Percobaan Langsung
                      </span>
                    </div>
                    <div className="flex items-center justify-between gap-2 p-2.5 rounded bg-surface border border-border/50 text-xs">
                      <div className="min-w-0 flex-1">
                        <span className="text-[10px] text-text-muted block font-sans">Target Model</span>
                        <span className="font-mono font-bold text-white truncate block">
                          {modelDisplayName}
                        </span>
                      </div>
                      <div className="p-1 rounded bg-sky-500/10 text-sky-400 flex-shrink-0">
                        <ArrowRight className="w-3.5 h-3.5" />
                      </div>
                      <div className="min-w-0 flex-1 text-right">
                        <span className="text-[10px] text-text-muted block font-sans">Upstream Provider</span>
                        <span className="font-medium text-purple-300 truncate block">
                          {r.providers && r.providers.length > 0 ? (
                            providers.find((prov) => prov.id === r.providers?.[0]?.provider_id)?.display_name ||
                            providers.find((prov) => prov.id === r.providers?.[0]?.provider_id)?.name ||
                            'Provider Terpilih'
                          ) : (
                            'Semua Provider (Bawaan)'
                          )}
                        </span>
                      </div>
                    </div>
                  </div>
                )}

                {/* 2. Mode COMBO ROUTING: Interactive Cascade Flow */}
                {ruleMode.mode === 'combo_routing' && (
                  <div className="p-2.5 rounded-lg bg-bg-surface-2/70 border border-accent/20 space-y-2 mt-3.5">
                    {ruleMode.alias && (
                      <div className="flex items-center justify-between pb-1.5 border-b border-border/30">
                        <span className="text-[10px] text-text-muted flex items-center gap-1 font-sans">
                          <Terminal className="w-3 h-3 text-accent" /> Virtual Endpoint:
                        </span>
                        <code className="text-[11px] font-mono font-bold text-accent bg-surface px-1.5 py-0.5 rounded border border-accent/30">
                          {ruleMode.alias}
                        </code>
                      </div>
                    )}
                    <div className="flex items-center gap-1.5 flex-wrap text-[11px]">
                      {(() => {
                        // Rantai model dibaca dari kolom jsonb; aturan produksi lama
                        // yang belum dikonversi jatuh ke tag description.
                        const modelUtama = models.find((m) => m.id === r.match_model_id);
                        const pipeline = parsePipeline(r) ?? parsePipelineDariTag(r.description, modelUtama?.model_id);
                        if (!pipeline) {
                          return <span className="text-[10px] text-text-muted italic">Resep combo tidak terbaca</span>;
                        }
                        return pipeline.models.map((modelId, i) => {
                          const mObj = models.find((m) => m.model_id === modelId);
                          return (
                            <React.Fragment key={`${modelId}-${i}`}>
                              {i > 0 && <ArrowRight className="w-3 h-3 text-text-muted" />}
                              <span
                                className={`px-2 py-0.5 rounded border font-semibold ${
                                  i === 0
                                    ? 'bg-accent/15 border-accent/30 text-accent'
                                    : 'bg-purple-500/15 border-purple-500/30 text-purple-300'
                                }`}
                              >
                                {`T${i + 1}: ${mObj?.display_name || modelId}`}
                              </span>
                            </React.Fragment>
                          );
                        });
                      })()}
                    </div>
                    <div className="flex items-center gap-2 pt-1 text-[10px] text-text-muted font-sans">
                      <span className="flex items-center gap-1 text-emerald-400">
                        <Zap className="w-2.5 h-2.5" /> Context Bypass
                      </span>
                      <span>•</span>
                      <span className="flex items-center gap-1 text-sky-400">
                        <ShieldCheck className="w-2.5 h-2.5" /> 429/5xx Failover
                      </span>
                    </div>
                  </div>
                )}

                {/* 3. Mode ROUTING (Multi-Provider Failover) Detail Table */}
                {ruleMode.mode === 'routing' && (
                  <div className="mt-3.5 space-y-2 text-xs">
                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Target Model</span>
                      <span className="font-mono text-white truncate max-w-[180px]">
                        {modelDisplayName}
                      </span>
                    </div>
                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Strategi</span>
                      <span className="font-semibold text-white uppercase font-mono">{r.strategy}</span>
                    </div>
                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Maksimal Percobaan</span>
                      <span className="font-mono text-white">{r.max_attempts} percobaan</span>
                    </div>
                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Jeda Backoff</span>
                      <span className="font-mono text-white">{r.backoff_ms} ms</span>
                    </div>
                    <div className="pt-1.5">
                      <div className="text-xs font-semibold text-text-muted mb-1 flex items-center justify-between">
                        <span>Provider Terpilih ({r.providers?.length || 0})</span>
                      </div>
                      <div className="flex flex-wrap gap-1">
                        {r.providers && r.providers.length > 0 ? (
                          r.providers.map((rp) => {
                            const p = providers.find((prov) => prov.id === rp.provider_id);
                            const pName = p ? (p.display_name || p.name) : rp.provider_id.slice(0, 8);
                            return (
                              <span
                                key={rp.provider_id}
                                className="px-2 py-0.5 rounded text-[10px] font-medium bg-purple-500/10 text-purple-300 border border-purple-500/20"
                              >
                                {pName}{rp.weight ? ` (w:${rp.weight})` : ''}
                              </span>
                            );
                          })
                        ) : (
                          <span className="text-[10px] text-text-muted italic">Semua provider (Bawaan)</span>
                        )}
                      </div>
                    </div>
                  </div>
                )}
              </div>

              {/* Action Buttons: Responsive & Balanced Layout */}
              <div className="mt-4 pt-3 border-t border-border/60 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
                <div className="grid grid-cols-2 sm:flex sm:items-center gap-1.5">
                  <Button
                    variant="secondary"
                    size="sm"
                    className="w-full sm:w-auto justify-center"
                    onClick={() => handleOpenEdit(r)}
                    icon={<Edit2 className="w-3.5 h-3.5" />}
                  >
                    Edit Aturan
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    className="w-full sm:w-auto justify-center text-purple-300 border-purple-500/30 hover:bg-purple-500/10"
                    onClick={() => handleOpenProviders(r)}
                    icon={<Server className="w-3.5 h-3.5" />}
                  >
                    Providers
                  </Button>
                </div>
                <div className="grid grid-cols-2 sm:flex sm:items-center gap-1.5">
                  <Button
                    variant={r.enabled ? 'danger' : 'secondary'}
                    size="sm"
                    className="w-full sm:w-auto justify-center"
                    onClick={() => handleToggle(r)}
                  >
                    {r.enabled ? 'Nonaktifkan' : 'Aktifkan'}
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    className="w-full sm:w-auto justify-center text-status-error/80 border-status-error/20 hover:bg-status-error/10 hover:text-status-error"
                    onClick={() => handleDelete(r.id)}
                    icon={<Trash2 className="w-3.5 h-3.5" />}
                  >
                    Hapus
                  </Button>
                </div>
              </div>
            </Card>
          );
        })}
      </div>
      )}

      {/* Integrated Circuit Breakers Panel */}
      <div className="pt-6 border-t border-border/40">
        <div className="rounded-box bg-bg-surface-1 border border-border/60 p-4 sm:p-5 shadow-sm space-y-4">
          {/* Header Row: Responsive & Unbroken on Mobile */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div className="flex items-start gap-3">
              <div className={`w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 mt-0.5 ${
                totalAbnormalBreakers > 0
                  ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                  : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              }`}>
                {totalAbnormalBreakers > 0 ? (
                  <ZapOff className="w-4 h-4" />
                ) : (
                  <ShieldCheck className="w-4 h-4" />
                )}
              </div>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-base font-bold text-white tracking-tight">
                    Pemutus Sirkuit (Circuit Breakers)
                  </h3>
                  {totalAbnormalBreakers > 0 ? (
                    <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium bg-rose-500/10 text-rose-400 border border-rose-500/20">
                      <span className="w-1.5 h-1.5 rounded-full bg-rose-400 animate-ping" />
                      {totalAbnormalBreakers} Sirkuit Terganggu
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                      <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                      Semua Beroperasi Normal
                    </span>
                  )}
                </div>
                <p className="text-xs text-text-secondary mt-0.5">
                  Proteksi otomatis yang mengisolasi provider saat terjadi lonjakan kegagalan dan failover darurat.
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2 self-end sm:self-auto flex-shrink-0">
              <Button
                variant="secondary"
                size="sm"
                onClick={loadBreakers}
                isLoading={isBreakersLoading}
                className="h-8 px-3 text-xs"
              >
                <RefreshCw className={`w-3.5 h-3.5 mr-1.5 ${isBreakersLoading ? 'animate-spin' : ''}`} />
                Segarkan
              </Button>
            </div>
          </div>

          {breakersError && <QueryError message={breakersError} onRetry={() => void loadBreakers()} />}

          {/* Body: Healthy State vs Tripped State */}
          {breakers.length === 0 || totalAbnormalBreakers === 0 ? (
            <div className="p-4 sm:p-5 rounded-lg bg-bg-surface-2/60 border border-border/40 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
              <div className="flex items-start gap-3">
                <div className="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 flex-shrink-0 mt-0.5">
                  <CheckCircle2 className="w-4 h-4" />
                </div>
                <div>
                  <h4 className="text-xs font-bold text-white">
                    Seluruh Sirkuit Provider Normal (Closed)
                  </h4>
                  <p className="text-xs text-text-secondary mt-0.5 leading-relaxed max-w-xl">
                    Proteksi isolasi otomatis aktif. Tidak ada upstream yang mengalami lonjakan kegagalan beruntun atau terisolasi dari perutean model.
                  </p>
                </div>
              </div>

              {/* 3 Telemetry Chips */}
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 w-full md:w-auto flex-shrink-0 pt-2 md:pt-0 border-t md:border-t-0 border-border/40">
                <div className="px-3 py-1.5 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                  <span className="text-[10px] text-text-muted block font-sans">Provider Dipantau</span>
                  <span className="text-xs font-bold font-mono text-white">
                    {providers.length} Terhubung
                  </span>
                </div>
                <div className="px-3 py-1.5 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                  <span className="text-[10px] text-text-muted block font-sans">Sirkuit Terputus</span>
                  <span className="text-xs font-bold font-mono text-emerald-400">
                    0 (Aman)
                  </span>
                </div>
                <div className="col-span-2 sm:col-span-1 px-3 py-1.5 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                  <span className="text-[10px] text-text-muted block font-sans">Ambang Isolasi</span>
                  <span className="text-xs font-bold font-mono text-sky-400">
                    5x Gagal
                  </span>
                </div>
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 pt-1">
              {breakers.map((b, idx) => {
                const provObj = providers.find((p) => p.id === b.provider_id);
                const providerName = provObj?.display_name || provObj?.name || b.provider_id;
                const isTrip = b.state === 'open';
                const isHalf = b.state === 'half-open';

                return (
                  <div
                    key={idx}
                    className={`p-3.5 rounded-lg border flex flex-col justify-between transition-colors ${
                      isTrip
                        ? 'bg-rose-500/5 border-rose-500/30'
                        : isHalf
                        ? 'bg-amber-500/5 border-amber-500/30'
                        : 'bg-bg-surface-2 border-border/60'
                    }`}
                  >
                    <div>
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <span className="text-[10px] text-text-muted font-sans uppercase tracking-wider block">
                            Provider Upstream
                          </span>
                          <h4 className="text-xs font-bold text-white truncate">
                            {providerName}
                          </h4>
                          <span className="text-[11px] font-mono text-sky-300 block truncate">
                            {b.model || 'Semua Model Provider'}
                          </span>
                        </div>
                        <Badge variant={isTrip ? 'error' : isHalf ? 'warn' : 'success'}>
                          {isTrip ? 'Terisolasi (Open)' : isHalf ? 'Uji Coba (Half-Open)' : 'Normal'}
                        </Badge>
                      </div>

                      <div className="mt-3 p-2 rounded bg-bg-surface-1/80 border border-border/40 text-xs space-y-1 font-mono">
                        <div className="flex justify-between text-text-muted text-[11px]">
                          <span>Kegagalan Konsekutif:</span>
                          <span className={`font-bold ${isTrip ? 'text-rose-400' : 'text-white'}`}>
                            {b.failure_count ?? 0}
                          </span>
                        </div>
                        {b.next_probe_at && (
                          <div className="flex justify-between text-text-muted text-[11px]">
                            <span>Jadwal Uji Probe:</span>
                            <span className="text-amber-300">
                              {new Date(b.next_probe_at).toLocaleTimeString('id-ID', {
                                hour: '2-digit',
                                minute: '2-digit',
                                second: '2-digit',
                              })}
                            </span>
                          </div>
                        )}
                      </div>
                    </div>

                    {b.state !== 'closed' && (
                      <div className="mt-3 pt-2 border-t border-border/40">
                        <Button
                          variant="secondary"
                          size="sm"
                          className="w-full text-xs h-8 border-border/60 hover:border-accent text-white"
                          onClick={() => handleResetBreaker(b.provider_id, b.model || '')}
                        >
                          <RotateCcw className="w-3.5 h-3.5 mr-1.5 text-accent" />
                          Pulihkan Sirkuit Sekarang
                        </Button>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Buat Aturan Perutean Cerdas (Routing Rule)"
        maxWidth="2xl"
        footer={
          <div className="flex items-center justify-between gap-3 w-full">
            <Button
              type="button"
              variant="ghost"
              size="md"
              onClick={() => setIsCreateOpen(false)}
            >
              Batal
            </Button>
            <Button
              type="submit"
              form="create-routing-rule-form"
              variant="primary"
              size="md"
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
                <Zap className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${mode === 'model_only' ? 'text-sky-400' : 'text-text-muted'}`} />
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
                <Layers className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${mode === 'combo_routing' ? 'text-accent' : 'text-text-muted'}`} />
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
                  placeholder={
                    mode === 'model_only'
                      ? 'direct-gpt4o'
                      : 'failover-smart-combo'
                  }
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
                    const provNames = m.providers?.map((p) => p.display_name || p.provider_name).join(', ');
                    return {
                      value: m.id,
                      label: `${m.display_name} (${m.model_id})`,
                      description: provNames ? `Tersedia di: ${provNames}` : (m.family ? `Keluarga: ${m.family}` : undefined),
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
                        label: `${p.display_name || p.name} (${p.kind})${matched ? ' ✓ Menyediakan Model' : ''}`,
                        description: matched ? `Model upstream: ${matched.upstream_model_name}` : undefined,
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
    
      {/* Drawer Edit Routing Rule */}
      <Drawer
        isOpen={isEditOpen}
        onClose={() => {
          setIsEditOpen(false);
          setEditingRule(null);
        }}
        title={`Edit Aturan: ${editingRule?.name || ''}`}
        maxWidth="2xl"
        footer={
          <div className="flex items-center justify-between gap-3 w-full">
            <Button
              type="button"
              variant="ghost"
              size="md"
              onClick={() => {
                setIsEditOpen(false);
                setEditingRule(null);
              }}
            >
              Batal
            </Button>
            <Button
              type="submit"
              form="edit-routing-rule-form"
              variant="primary"
              size="md"
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
                <Zap className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${editMode === 'model_only' ? 'text-sky-400' : 'text-text-muted'}`} />
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
                <Layers className={`w-3.5 h-3.5 sm:w-4 sm:h-4 shrink-0 ${editMode === 'combo_routing' ? 'text-accent' : 'text-text-muted'}`} />
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
                  placeholder={
                    editMode === 'model_only'
                      ? 'direct-gpt4o'
                      : 'failover-smart-combo'
                  }
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
                  onChange={(e) => setEditRuleForm({ ...editRuleForm, description: e.target.value })}
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
                    const provNames = m.providers?.map((p) => p.display_name || p.provider_name).join(', ');
                    return {
                      value: m.id,
                      label: `${m.display_name} (${m.model_id})`,
                      description: provNames ? `Tersedia di: ${provNames}` : (m.family ? `Keluarga: ${m.family}` : undefined),
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
                        label: `${p.display_name || p.name} (${p.kind})${matched ? ' ✓ Menyediakan Model' : ''}`,
                        description: matched ? `Model upstream: ${matched.upstream_model_name}` : undefined,
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
                        setEditRuleForm({ ...editRuleForm, max_attempts: parseInt(e.target.value) || 3 })
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
                        setEditRuleForm({ ...editRuleForm, backoff_ms: parseInt(e.target.value) || 200 })
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
                        setEditRuleForm({ ...editRuleForm, priority: parseInt(e.target.value) || 100 })
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
                        setEditRuleForm({ ...editRuleForm, failure_threshold: parseInt(e.target.value) || 5 })
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
    
      <Drawer
        isOpen={isProvidersDrawerOpen}
        onClose={() => setIsProvidersDrawerOpen(false)}
        title="Edit Providers & Weights"
        footer={
          <div className="flex items-center justify-between gap-3 w-full">
            <Button
              type="button"
              variant="ghost"
              size="md"
              onClick={() => setIsProvidersDrawerOpen(false)}
            >
              Batal
            </Button>
            <Button
              type="submit"
              form="providers-routing-rule-form"
              variant="primary"
              size="md"
              className="flex-1 sm:flex-initial"
            >
              Simpan Providers
            </Button>
          </div>
        }
      >
        <form id="providers-routing-rule-form" noValidate onSubmit={handleSaveProviders} className="space-y-4 text-xs">
          <div className="space-y-2">
            {providers.map(p => {
              const targetM = models.find(m => m.id === selectedRule?.match_model_id || m.model_id === selectedRule?.match_model_id);
              const matchedUpstream = targetM?.providers?.find(mp => mp.provider_id === p.id);
              const isChecked = ruleProviders.includes(p.id);
              return (
                <div
                  key={p.id}
                  className={`p-2.5 rounded-lg border transition-all ${
                    isChecked
                      ? 'bg-purple-500/10 border-purple-400/40 shadow-sm shadow-purple-500/5'
                      : 'bg-bg-surface-1 border-border/70 hover:border-border'
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2 font-semibold text-white cursor-pointer min-w-0">
                      <Checkbox
                        checked={isChecked}
                        onChange={() => toggleProvider(p.id)}
                      />
                      <span className="truncate">{p.display_name || p.name}</span>
                      <span className="text-[10px] text-text-muted font-mono">({p.kind})</span>
                    </div>
                    {targetM ? (
                      matchedUpstream ? (
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shrink-0">
                          ✓ {matchedUpstream.upstream_model_name}
                        </span>
                      ) : (
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-bg-surface-2 text-text-muted border border-border/60 shrink-0">
                          Model tidak tersedia
                        </span>
                      )
                    ) : null}
                  </div>
                  {isChecked && (
                    <div className="pl-6 flex items-center gap-2 mt-2 pt-2 border-t border-border/40">
                      <label className="text-[11px] text-text-secondary">Bobot (Weight):</label>
                      <input
                        type="number"
                        min="1"
                        value={ruleWeights[p.id] || 1}
                        onChange={(e) => updateWeight(p.id, parseInt(e.target.value) || 1)}
                        className="w-20 px-2 py-0.5 bg-bg-surface-2 border border-border rounded text-white text-xs font-mono focus:outline-none focus:border-purple-400 focus:ring-1 focus:ring-purple-400"
                      />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </form>
      </Drawer>
</div>
  );
};
