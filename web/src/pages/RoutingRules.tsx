import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RoutingRule, CircuitBreakerStatus, Model, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { Plus, Trash2, ZapOff, RotateCcw, RefreshCw, Zap, Shuffle, Layers, Edit2, Globe, Terminal, ShieldCheck, ArrowRight } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export interface FallbackTierItem {
  id: string;
  model_id: string;
  provider_ids: string[];
}

export const RoutingRules: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [breakers, setBreakers] = useState<CircuitBreakerStatus[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isBreakersLoading, setIsBreakersLoading] = useState(false);
  const [isProvidersDrawerOpen, setIsProvidersDrawerOpen] = useState(false);
  const [selectedRule, setSelectedRule] = useState<any>(null);
  const [ruleProviders, setRuleProviders] = useState<string[]>([]);
  const [ruleWeights, setRuleWeights] = useState<Record<string, number>>({});

  // Edit Drawer States
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<RoutingRule | null>(null);
  const [editMode, setEditMode] = useState<'model_only' | 'routing' | 'combo_routing'>('model_only');
  const [editTargetModelId, setEditTargetModelId] = useState('');
  const [editTargetProviderId, setEditTargetProviderId] = useState('');
  const [editVirtualAlias, setEditVirtualAlias] = useState('');
  const [editOriginalAlias, setEditOriginalAlias] = useState('');
  const [editTier1ModelId, setEditTier1ModelId] = useState('');
  const [editTier1Providers, setEditTier1Providers] = useState<string[]>([]);
  const [editFallbackTiers, setEditFallbackTiers] = useState<FallbackTierItem[]>([
    { id: '1', model_id: '', provider_ids: [] },
  ]);
  const [editRuleProviders, setEditRuleProviders] = useState<string[]>([]);
  const [editRuleWeights, setEditRuleWeights] = useState<Record<string, number>>({});
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

  // 3-Mode Form States (Create)
  const [mode, setMode] = useState<'model_only' | 'routing' | 'combo_routing'>('model_only');
  const [targetModelId, setTargetModelId] = useState('');
  const [targetProviderId, setTargetProviderId] = useState('');
  const [virtualAlias, setVirtualAlias] = useState('');
  const [tier1ModelId, setTier1ModelId] = useState('');
  const [tier1Providers, setTier1Providers] = useState<string[]>([]);
  const [fallbackTiers, setFallbackTiers] = useState<FallbackTierItem[]>([
    { id: '1', model_id: '', provider_ids: [] },
  ]);
  const [createRuleProviders, setCreateRuleProviders] = useState<string[]>([]);
  const [createRuleWeights, setCreateRuleWeights] = useState<Record<string, number>>({});

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

  const loadRules = async () => {
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
    }
  };

  const getRuleMode = (r: RoutingRule): { mode: string; label: string; badgeVariant: 'info' | 'lime' | 'warn'; detail: string; alias?: string | null; pipelineCount?: number } => {
    const desc = r.description || '';
    // Tag struktur [combo:pipeline=...] / [combo:tier2=...] / [model_only] menentukan mode
    if (desc.includes('[combo:pipeline=') || desc.includes('[combo:tier2=')) {
      const aliasMatch = desc.match(/\[combo:alias=([^\]]+)\]/);
      const alias = aliasMatch ? aliasMatch[1] : null;

      let pipelineCount = 2;
      const pipeMatch = desc.match(/\[combo:pipeline=(\[.*?\])\]/);
      if (pipeMatch) {
        try {
          const parsed = JSON.parse(pipeMatch[1]);
          pipelineCount = parsed.length + 1;
        } catch (e) {}
      }
      return {
        mode: 'combo_routing',
        label: 'COMBO PIPELINE',
        badgeVariant: 'lime',
        detail: alias ? `Virtual Endpoint: ${alias} (${pipelineCount}-Tier Cascade)` : `${pipelineCount}-Tier Dynamic Cascade`,
        alias,
        pipelineCount,
      };
    }
    if (r.max_attempts === 1 || desc.includes('[model_only]')) {
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
    setTier1ModelId('');
    setTier1Providers([]);
    setFallbackTiers([{ id: '1', model_id: '', provider_ids: [] }]);
    setCreateRuleProviders([]);
    setCreateRuleWeights({});
    setIsCreateOpen(true);
  };

  const toggleCreateProvider = (pid: string) => {
    setCreateRuleProviders((prev) =>
      prev.includes(pid) ? prev.filter((p) => p !== pid) : [...prev, pid]
    );
  };

  const updateCreateWeight = (pid: string, w: number) => {
    setCreateRuleWeights((prev) => ({ ...prev, [pid]: w }));
  };

  const handleSelectMatchingProviders = () => {
    if (!targetModelId) {
      setCreateRuleProviders(providers.map((p) => p.id));
      return;
    }
    const selModel = models.find((m) => m.id === targetModelId);
    const matchingIds = providers
      .filter((p) => selModel?.providers?.some((mp) => mp.provider_id === p.id))
      .map((p) => p.id);
    setCreateRuleProviders(matchingIds.length > 0 ? matchingIds : providers.map((p) => p.id));
  };

  const handleClearCreateProviders = () => {
    setCreateRuleProviders([]);
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
          description: newRule.description?.trim()
            ? `[model_only] ${newRule.description.trim()}`
            : `[model_only] Direct 1:1 passthrough ke model ${modelName}${selProv ? ` via ${selProv.name}` : ''}`,
        };
      } else if (mode === 'combo_routing') {
        if (!tier1ModelId) {
          toast.error('Pilih Model Tier 1 (Utama) terlebih dahulu.');
          return;
        }
        if (fallbackTiers.length === 0 || fallbackTiers.some((t) => !t.model_id)) {
          toast.error('Harap pilih model untuk setiap tier fallback.');
          return;
        }
        const allTierModelIds = [tier1ModelId, ...fallbackTiers.map((t) => t.model_id)];
        const uniqueSet = new Set(allTierModelIds);
        if (uniqueSet.size !== allTierModelIds.length) {
          toast.error('Model antar tier tidak boleh sama — model duplikat membuat loop fallback.');
          return;
        }
        const selT1 = models.find((m) => m.id === tier1ModelId);
        if (!selT1) {
          toast.error('Model Tier 1 tidak valid di registry.');
          return;
        }

        // Daftarkan virtual alias bila diisi (Opsi A)
        const cleanAlias = virtualAlias.trim().toLowerCase();
        if (cleanAlias) {
          try {
            await api.models.addAlias(selT1.id, cleanAlias);
          } catch (aliasErr) {
            console.warn('Notice saat mendaftarkan alias model:', aliasErr);
          }
        }

        // Susun N-Tier Pipeline (Opsi B)
        const pipelineData = fallbackTiers.map((t, idx) => {
          const m = models.find((mod) => mod.id === t.model_id);
          return {
            tier: idx + 2,
            model: m?.model_id || t.model_id,
            providers: t.provider_ids.length > 0 ? t.provider_ids : undefined,
          };
        });

        const aliasTag = cleanAlias ? `[combo:alias=${cleanAlias}] ` : '';
        const pipeTag = `[combo:pipeline=${JSON.stringify(pipelineData)}]`;
        const legacyTier2 = pipelineData[0]?.model || '';
        const legacyTag = legacyTier2 ? `[combo:tier2=${legacyTier2}] ` : '';

        const cascadeNames = [
          `Tier 1 (${selT1.display_name})`,
          ...pipelineData.map((p) => `Tier ${p.tier} (${models.find((m) => m.model_id === p.model)?.display_name || p.model})`),
        ].join(' -> ');

        payload = {
          ...payload,
          name:
            newRule.name ||
            (cleanAlias
              ? `pipeline-${cleanAlias}`
              : `combo-${selT1.model_id.replace(/[^a-zA-Z0-9_-]/g, '-')}-cascade`),
          strategy: 'priority',
          max_attempts: (fallbackTiers.length + 1) * 2,
          match_model_id: selT1.id,
          provider_ids: tier1Providers.length > 0 ? tier1Providers : undefined,
          description: newRule.description?.trim()
            ? `${aliasTag}${legacyTag}${pipeTag} ${newRule.description.trim()}`
            : `${aliasTag}${legacyTag}${pipeTag} Smart Tiered Cascade: ${cascadeNames}`,
        };
      } else {
        const selModel = targetModelId ? models.find((m) => m.id === targetModelId) : null;
        payload = {
          ...payload,
          name:
            newRule.name ||
            (selModel
              ? `failover-${selModel.model_id.replace(/[^a-zA-Z0-9_-]/g, '-')}`
              : `routing-global-${newRule.strategy}`),
          strategy: newRule.strategy,
          max_attempts: newRule.max_attempts,
          match_model_id: targetModelId || undefined,
          provider_ids: createRuleProviders.length > 0 ? createRuleProviders : undefined,
          weights: Object.keys(createRuleWeights).length > 0 ? createRuleWeights : undefined,
          description: newRule.description?.trim()
            ? `[routing] ${newRule.description.trim()}`
            : `[routing] Failover multi-provider (${newRule.strategy}) untuk ${selModel?.display_name || 'semua model'}`,
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
      setTier1ModelId('');
      setTier1Providers([]);
      setFallbackTiers([{ id: '1', model_id: '', provider_ids: [] }]);
      setCreateRuleProviders([]);
      setCreateRuleWeights({});
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
    setEditMode(mMode);

    let cleanDesc = r.description || '';
    cleanDesc = cleanDesc
      .replace(/\[combo:alias=[^\]]+\]\s*/g, '')
      .replace(/\[combo:pipeline=\[.*?\]\]\s*/g, '')
      .replace(/\[combo:tier2=[^\]]+\]\s*/g, '')
      .replace(/^\[model_only\]\s*/, '')
      .replace(/^\[routing\]\s*/, '')
      .trim();

    const aliasMatch = (r.description || '').match(/\[combo:alias=([^\]]+)\]/);
    const currAlias = aliasMatch ? aliasMatch[1] : '';
    setEditVirtualAlias(currAlias);
    setEditOriginalAlias(currAlias);

    setEditRuleForm({
      name: r.name || '',
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
    const pWeights: Record<string, number> = {};
    if (r.providers) {
      r.providers.forEach((p: any) => {
        if (p.weight) pWeights[p.provider_id] = p.weight;
      });
    }
    setEditRuleProviders(pIds);
    setEditRuleWeights(pWeights);

    setEditTargetModelId(r.match_model_id || '');
    setEditTargetProviderId(pIds.length === 1 ? pIds[0] : '');

    if (mMode === 'combo_routing') {
      setEditTier1ModelId(r.match_model_id || '');
      setEditTier1Providers(pIds);

      const pipeMatch = (r.description || '').match(/\[combo:pipeline=(\[.*?\])\]/);
      if (pipeMatch) {
        try {
          const parsed = JSON.parse(pipeMatch[1]);
          const loadedTiers = parsed.map((p: any) => {
            const found = models.find((m) => m.model_id === p.model || m.id === p.model);
            return {
              id: Math.random().toString(),
              model_id: found ? found.id : p.model,
              provider_ids: p.providers || [],
            };
          });
          setEditFallbackTiers(loadedTiers.length > 0 ? loadedTiers : [{ id: '1', model_id: '', provider_ids: [] }]);
        } catch (e) {
          setEditFallbackTiers([{ id: '1', model_id: '', provider_ids: [] }]);
        }
      } else {
        const match = (r.description || '').match(/\[combo:tier2=([^\]]+)\]/);
        if (match) {
          const tier2Key = match[1];
          const found = models.find((m) => m.model_id === tier2Key || m.id === tier2Key);
          setEditFallbackTiers([
            { id: '1', model_id: found ? found.id : tier2Key, provider_ids: [] },
          ]);
        } else {
          setEditFallbackTiers([{ id: '1', model_id: '', provider_ids: [] }]);
        }
      }
    } else {
      setEditTier1ModelId(r.match_model_id || '');
      setEditTier1Providers([]);
      setEditFallbackTiers([{ id: '1', model_id: '', provider_ids: [] }]);
    }

    setIsEditOpen(true);
  };

  const toggleEditProvider = (pid: string) => {
    setEditRuleProviders((prev) =>
      prev.includes(pid) ? prev.filter((p) => p !== pid) : [...prev, pid]
    );
  };

  const updateEditWeight = (pid: string, w: number) => {
    setEditRuleWeights((prev) => ({ ...prev, [pid]: w }));
  };

  const handleSelectMatchingEditProviders = () => {
    if (!editTargetModelId) {
      setEditRuleProviders(providers.map((p) => p.id));
      return;
    }
    const selModel = models.find((m) => m.id === editTargetModelId);
    const matchingIds = providers
      .filter((p) => selModel?.providers?.some((mp) => mp.provider_id === p.id))
      .map((p) => p.id);
    setEditRuleProviders(matchingIds.length > 0 ? matchingIds : providers.map((p) => p.id));
  };

  const handleClearEditProviders = () => {
    setEditRuleProviders([]);
  };

  const handleEditSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingRule) return;

    try {
      let payload: any = {
        priority: editRuleForm.priority,
        failure_threshold: editRuleForm.failure_threshold,
        open_duration_ms: editRuleForm.open_duration_ms,
        half_open_probes: editRuleForm.half_open_probes,
        backoff_ms: editRuleForm.backoff_ms,
      };

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
          description: customDesc
            ? `[model_only] ${customDesc}`
            : `[model_only] Direct 1:1 passthrough ke model ${modelName}${selProv ? ` via ${selProv.name}` : ''}`,
        };
      } else if (editMode === 'combo_routing') {
        if (!editTier1ModelId) {
          toast.error('Pilih Model Tier 1 (Utama) terlebih dahulu.');
          return;
        }
        if (editFallbackTiers.length === 0 || editFallbackTiers.some((t) => !t.model_id)) {
          toast.error('Harap pilih model untuk setiap tier fallback.');
          return;
        }
        const allTierModelIds = [editTier1ModelId, ...editFallbackTiers.map((t) => t.model_id)];
        const uniqueSet = new Set(allTierModelIds);
        if (uniqueSet.size !== allTierModelIds.length) {
          toast.error('Model antar tier tidak boleh sama — model duplikat membuat loop fallback.');
          return;
        }
        const selT1 = models.find((m) => m.id === editTier1ModelId);
        if (!selT1) {
          toast.error('Model Tier 1 tidak valid di registry.');
          return;
        }

        // Daftarkan virtual alias bila diisi/berubah (Opsi A)
        const cleanAlias = editVirtualAlias.trim().toLowerCase();
        if (cleanAlias && cleanAlias !== editOriginalAlias) {
          try {
            await api.models.addAlias(selT1.id, cleanAlias);
          } catch (aliasErr) {
            console.warn('Notice saat mendaftarkan alias model:', aliasErr);
          }
        }

        // Susun N-Tier Pipeline (Opsi B)
        const pipelineData = editFallbackTiers.map((t, idx) => {
          const m = models.find((mod) => mod.id === t.model_id);
          return {
            tier: idx + 2,
            model: m?.model_id || t.model_id,
            providers: t.provider_ids.length > 0 ? t.provider_ids : undefined,
          };
        });

        const aliasTag = cleanAlias ? `[combo:alias=${cleanAlias}] ` : '';
        const pipeTag = `[combo:pipeline=${JSON.stringify(pipelineData)}]`;
        const legacyTier2 = pipelineData[0]?.model || '';
        const legacyTag = legacyTier2 ? `[combo:tier2=${legacyTier2}] ` : '';

        const cascadeNames = [
          `Tier 1 (${selT1.display_name})`,
          ...pipelineData.map((p) => `Tier ${p.tier} (${models.find((m) => m.model_id === p.model)?.display_name || p.model})`),
        ].join(' -> ');

        payload = {
          ...payload,
          name:
            editRuleForm.name ||
            (cleanAlias
              ? `pipeline-${cleanAlias}`
              : `combo-${selT1.model_id.replace(/[^a-zA-Z0-9_-]/g, '-')}-cascade`),
          strategy: 'priority',
          max_attempts: (editFallbackTiers.length + 1) * 2,
          match_model_id: selT1.id,
          provider_ids: editTier1Providers.length > 0 ? editTier1Providers : [],
          weights: {},
          description: customDesc
            ? `${aliasTag}${legacyTag}${pipeTag} ${customDesc}`
            : `${aliasTag}${legacyTag}${pipeTag} Smart Tiered Cascade: ${cascadeNames}`,
        };
      } else {
        const selModel = editTargetModelId ? models.find((m) => m.id === editTargetModelId) : null;
        payload = {
          ...payload,
          name:
            editRuleForm.name ||
            (selModel
              ? `failover-${selModel.model_id.replace(/[^a-zA-Z0-9_-]/g, '-')}`
              : `routing-global-${editRuleForm.strategy}`),
          strategy: editRuleForm.strategy,
          max_attempts: editRuleForm.max_attempts,
          match_model_id: editTargetModelId ? editTargetModelId : '',
          provider_ids: editRuleProviders,
          weights: editRuleWeights,
          description: customDesc
            ? `[routing] ${customDesc}`
            : `[routing] Failover multi-provider (${editRuleForm.strategy}) untuk ${selModel?.display_name || 'semua model'}`,
        };
      }

      await api.routing.update(editingRule.id, payload);
      setIsEditOpen(false);
      setEditingRule(null);
      toast.success(`Aturan perutean "${payload.name || editingRule.name}" berhasil diperbarui`);
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

  return (
    <div className="space-y-6">
      <PageHeader
        title="Routing Rules Engine"
        description="Strategi pemilihan upstream (Priority, Lowest Cost, Lowest Latency, Weighted, Round-Robin) dan failover otomatis."
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

      {rulesError && <QueryError message={rulesError} onRetry={() => void loadRules()} />}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {rules.map((r) => {
          const ruleMode = getRuleMode(r);
          const matchedModel = r.match_model_id
            ? models.find((m) => m.id === r.match_model_id || m.model_id === r.match_model_id)
            : null;
          const cleanCustomDesc = r.description
            ? r.description
                .replace(/\[combo:alias=[^\]]+\]\s*/g, '')
                .replace(/\[combo:pipeline=\[.*?\]\]\s*/g, '')
                .replace(/\[combo:tier2=[^\]]+\]\s*/g, '')
                .replace(/^\[(model_only|routing)\]\s*/, '')
                .trim()
            : '';

          return (
            <Card key={r.id} className="p-5 flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className={`w-9 h-9 rounded-full flex items-center justify-center ${
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
                    <div>
                      <h4 className="text-sm font-bold text-white">{r.name}</h4>
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

                {cleanCustomDesc && (
                  <p className="mt-3 text-xs text-text-secondary leading-relaxed bg-bg-surface-2/50 p-2 rounded border border-border/40">
                    {cleanCustomDesc}
                  </p>
                )}

                {ruleMode.mode === 'combo_routing' && (
                  <div className="p-2.5 rounded-lg bg-bg-surface-2/70 border border-accent/20 space-y-2 mt-3">
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
                      <span className="px-2 py-0.5 rounded bg-accent/15 border border-accent/30 text-accent font-semibold flex items-center gap-1">
                        T1: {matchedModel?.display_name || 'Tier 1'}
                      </span>
                      {(() => {
                        const pipeMatch = r.description?.match(/\[combo:pipeline=(\[.*?\])\]/);
                        if (pipeMatch) {
                          try {
                            const parsed = JSON.parse(pipeMatch[1]);
                            return parsed.map((p: any) => {
                              const mObj = models.find(m => m.model_id === p.model || m.id === p.model);
                              return (
                                <React.Fragment key={p.tier}>
                                  <ArrowRight className="w-3 h-3 text-text-muted" />
                                  <span className="px-2 py-0.5 rounded bg-purple-500/15 border border-purple-500/30 text-purple-300 font-semibold">
                                    T{p.tier}: {mObj?.display_name || p.model}
                                    {p.providers?.length ? ` (${p.providers.length} prov)` : ''}
                                  </span>
                                </React.Fragment>
                              );
                            });
                          } catch (e) {}
                        }
                        const t2Match = r.description?.match(/\[combo:tier2=([^\]]+)\]/);
                        if (t2Match) {
                          const mObj = models.find(m => m.model_id === t2Match[1] || m.id === t2Match[1]);
                          return (
                            <>
                              <ArrowRight className="w-3 h-3 text-text-muted" />
                              <span className="px-2 py-0.5 rounded bg-purple-500/15 border border-purple-500/30 text-purple-300 font-semibold">
                                T2: {mObj?.display_name || t2Match[1]}
                              </span>
                            </>
                          );
                        }
                        return null;
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

                <div className="mt-4 space-y-2 text-xs">
                  <div className="p-2 rounded bg-surface/70 border border-border/50 font-mono text-[11px] text-accent">
                    {ruleMode.detail}
                  </div>
                  <div className="flex justify-between py-1 border-b border-border/40">
                    <span className="text-text-muted">Target Model</span>
                    <span className="font-mono text-white truncate max-w-[180px]" title={matchedModel?.display_name || r.match_model_id || 'Semua Model'}>
                      {matchedModel ? `${matchedModel.display_name} (${matchedModel.model_id})` : r.match_model_id || 'Semua Model (Catch-All)'}
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
                  <div className="pt-2">
                    <div className="text-[10px] uppercase font-bold text-text-muted mb-1.5 flex items-center justify-between">
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
              </div>

              <div className="mt-5 pt-3 border-t border-border flex items-center gap-2 flex-wrap justify-between">
                <div className="flex items-center gap-1.5">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleOpenEdit(r)}
                    icon={<Edit2 className="w-3.5 h-3.5" />}
                  >
                    Edit Aturan
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleOpenProviders(r)}
                    title="Ubah pemetaan provider & bobot secara cepat"
                  >
                    Providers
                  </Button>
                </div>
                <div className="flex items-center gap-1.5">
                  <Button
                    variant={r.enabled ? 'danger' : 'secondary'}
                    size="sm"
                    onClick={() => handleToggle(r)}
                  >
                    {r.enabled ? 'Nonaktifkan' : 'Aktifkan'}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
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

      {/* Integrated Circuit Breakers Section */}
      <div className="pt-6 border-t border-border/40">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-lg font-bold text-white tracking-tight flex items-center gap-2">
              <ZapOff className="w-4 h-4 text-status-warning" />
              Status Circuit Breakers Terdistribusi
            </h3>
            <p className="text-xs text-text-secondary mt-0.5">
              Isolasi otomatis provider yang mengalami lonjakan kegagalan dan pemulihan darurat.
            </p>
          </div>
          <Button variant="secondary" size="sm" onClick={loadBreakers} isLoading={isBreakersLoading}>
            <RefreshCw className="w-3.5 h-3.5" />
          </Button>
        </div>

        {breakersError && <QueryError message={breakersError} onRetry={() => void loadBreakers()} />}

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {breakers.length === 0 ? (
            <div className="col-span-3 p-6 text-center rounded-box bg-bg-surface-1 border border-border/40 text-xs text-text-muted">
              Seluruh sirkuit provider dalam kondisi normal (Closed). Tidak ada pemutus sirkuit yang terbuka.
            </div>
          ) : (
            breakers.map((b, idx) => (
              <Card key={idx} className="p-4 flex flex-col justify-between">
                <div>
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-2.5">
                      <div className={`w-8 h-8 rounded-full flex items-center justify-center ${
                        b.state === 'open' ? 'bg-status-error/10 text-status-error' :
                        b.state === 'half-open' ? 'bg-status-warning/10 text-status-warning' :
                        'bg-status-success/10 text-status-success'
                      }`}>
                        <ZapOff className="w-4 h-4" />
                      </div>
                      <div>
                        <h4 className="text-xs font-bold text-white font-mono">{b.model || 'Default'}</h4>
                        <span className="text-[11px] text-text-muted font-mono">{b.provider_id}</span>
                      </div>
                    </div>
                    <Badge variant={b.state === 'open' ? 'error' : b.state === 'half-open' ? 'warn' : 'success'}>
                      {b.state}
                    </Badge>
                  </div>
                  <div className="mt-3 text-xs space-y-1">
                    <div className="flex justify-between text-text-muted">
                      <span>Kegagalan Konsekutif:</span>
                      <span className="font-mono text-white">{b.failure_count ?? 0}</span>
                    </div>
                  </div>
                </div>
                {b.state !== 'closed' && (
                  <div className="mt-4 pt-2 border-t border-border">
                    <Button
                      variant="secondary"
                      size="sm"
                      className="w-full"
                      onClick={() => handleResetBreaker(b.provider_id, b.model || '')}
                      icon={<RotateCcw className="w-3.5 h-3.5" />}
                    >
                      Reset Sirkuit
                    </Button>
                  </div>
                )}
              </Card>
            ))
          )}
        </div>
      </div>

      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Buat Aturan Perutean Cerdas (Routing Rule)"
        subtitle="Konfigurasikan jalur eksekusi model: Passthrough langsung 1:1, Failover multi-provider, atau Cascade hemat biaya."
        maxWidth="3xl"
      >
        <div className="space-y-5 text-xs">
          {/* 3-Mode Selector Tabs */}
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1.5">
              Pilih Mode Routing
            </label>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 p-1.5 bg-bg-surface-2 border border-border rounded-xl">
              <button
                type="button"
                onClick={() => setMode('model_only')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  mode === 'model_only'
                    ? 'bg-sky-500/15 border border-sky-400/60 text-sky-300 shadow-sm shadow-sky-500/10'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-1 border border-transparent'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Zap className={`w-4 h-4 ${mode === 'model_only' ? 'text-sky-400' : 'text-text-muted'}`} />
                  <span className="font-bold">1. Model Only</span>
                </div>
                <span className={`text-[10px] font-normal ${mode === 'model_only' ? 'text-sky-400/80' : 'opacity-70'}`}>Direct 1:1 Passthrough</span>
              </button>
              <button
                type="button"
                onClick={() => setMode('routing')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  mode === 'routing'
                    ? 'bg-purple-500/15 border border-purple-400/60 text-purple-300 shadow-sm shadow-purple-500/10'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-1 border border-transparent'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Shuffle className={`w-4 h-4 ${mode === 'routing' ? 'text-purple-400' : 'text-text-muted'}`} />
                  <span className="font-bold">2. Routing</span>
                </div>
                <span className={`text-[10px] font-normal ${mode === 'routing' ? 'text-purple-400/80' : 'opacity-70'}`}>Multi-Provider Failover</span>
              </button>
              <button
                type="button"
                onClick={() => setMode('combo_routing')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  mode === 'combo_routing'
                    ? 'bg-accent/15 border border-accent/60 text-accent shadow-sm shadow-accent/10'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-1 border border-transparent'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Layers className={`w-4 h-4 ${mode === 'combo_routing' ? 'text-accent' : 'text-text-muted'}`} />
                  <span className="font-bold">3. Combo Routing</span>
                </div>
                <span className={`text-[10px] font-normal ${mode === 'combo_routing' ? 'text-accent/80' : 'opacity-70'}`}>Tier 1 &rarr; Tier 2 Cascade</span>
              </button>
            </div>
          </div>

          <form onSubmit={handleCreate} className="space-y-4">
            {mode === 'model_only' && (
              <div className="p-3.5 bg-sky-500/10 border border-sky-500/25 rounded-xl text-sky-300 text-[11px] leading-relaxed flex items-start gap-2.5">
                <Zap className="w-4 h-4 text-sky-400 shrink-0 mt-0.5" />
                <div>
                  <strong>Mode Model Only:</strong> Passthrough langsung 1-to-1 ke model dan provider tertentu tanpa overhead failover. Latensi paling instan (&lt; 2ms saat cache hit).
                </div>
              </div>
            )}

            {mode === 'routing' && (
              <div className="p-3.5 bg-purple-500/10 border border-purple-500/25 rounded-xl text-purple-300 text-[11px] leading-relaxed flex items-start gap-2.5">
                <Shuffle className="w-4 h-4 text-purple-400 shrink-0 mt-0.5" />
                <div>
                  <strong>Mode Multi-Provider Routing:</strong> Mendistribusikan lalu lintas atau failover antar beberapa provider upstream (Priority, Lowest Latency, Lowest Cost, Weighted, Round Robin).
                </div>
              </div>
            )}

            {mode === 'combo_routing' && (
              <div className="p-3.5 bg-accent/10 border border-accent/25 rounded-xl text-accent text-[11px] leading-relaxed flex items-start gap-2.5">
                <Layers className="w-4 h-4 text-accent shrink-0 mt-0.5" />
                <div>
                  <strong>Mode Combo Routing:</strong> Rantai bertingkat cerdas. Permintaan pertama dialokasikan ke <strong>Tier 1 (Lokal/Hemat)</strong>. Bila kuota habis (429) atau upstream error, otomatis dialihkan ke <strong>Tier 2 (Flagship Fallback)</strong>!
                </div>
              </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Nama Aturan
                </label>
                <input
                  type="text"
                  placeholder={
                    mode === 'model_only'
                      ? 'direct-gpt4o'
                      : mode === 'combo_routing'
                      ? 'combo-local-to-flagship'
                      : 'failover-deepseek'
                  }
                  value={newRule.name}
                  onChange={(e) => setNewRule({ ...newRule, name: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                />
                <span className="text-[10px] text-text-muted mt-0.5 block">
                  Kosongkan untuk nama otomatis berdasarkan mode &amp; model.
                </span>
              </div>
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Deskripsi (Opsional)
                </label>
                <input
                  type="text"
                  placeholder="Catatan tujuan atau spesifikasi aturan..."
                  value={newRule.description}
                  onChange={(e) => setNewRule({ ...newRule, description: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
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

            {/* Mode 2: Routing Form Fields */}
            {mode === 'routing' && (
              <div className="space-y-4 pt-1 border-t border-border/40">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <Select
                    label="Target Model"
                    value={targetModelId}
                    onChange={(val) => setTargetModelId(val)}
                    placeholder="-- Semua Model (Catch-All) --"
                    options={[
                      { value: '', label: '-- Semua Model (Catch-All) --' },
                      ...models.map((m) => {
                        const provNames = m.providers?.map((p) => p.display_name || p.provider_name).join(', ');
                        return {
                          value: m.id,
                          label: `${m.display_name} (${m.model_id})`,
                          description: provNames ? `Tersedia di: ${provNames}` : (m.family ? `Keluarga: ${m.family}` : undefined),
                        };
                      }),
                    ]}
                  />
                  <Select
                    label="Strategi Routing"
                    value={newRule.strategy}
                    onChange={(val) => setNewRule({ ...newRule, strategy: val })}
                    options={[
                      { value: 'priority', label: 'Priority (Urutan Tertinggi)', description: 'Mengarahkan ke upstream prioritas tertinggi' },
                      { value: 'lowest_cost', label: 'Lowest Cost (Biaya Termurah)', description: 'Mengarahkan ke provider dengan tarif token terendah' },
                      { value: 'lowest_latency', label: 'Lowest Latency (Latensi Terendah)', description: 'Mengarahkan ke provider dengan respon tergesit' },
                      { value: 'weighted', label: 'Weighted (Bobot Proporsional)', description: 'Distribusi beban berbasis rasio bobot upstream' },
                      { value: 'round_robin', label: 'Round Robin (Beban Berimbang)', description: 'Distribusi bergilir seimbang antar semua upstream' },
                    ]}
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block font-semibold text-text-secondary uppercase mb-1">Maksimal Percobaan</label>
                    <input
                      type="number"
                      min="1"
                      max="10"
                      value={newRule.max_attempts}
                      onChange={(e) => setNewRule({ ...newRule, max_attempts: parseInt(e.target.value) || 3 })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                    />
                  </div>
                  <div>
                    <label className="block font-semibold text-text-secondary uppercase mb-1">Jeda Backoff (ms)</label>
                    <input
                      type="number"
                      min="0"
                      value={newRule.backoff_ms}
                      onChange={(e) => setNewRule({ ...newRule, backoff_ms: parseInt(e.target.value) || 200 })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                    />
                  </div>
                </div>

                {/* Provider Selection & Weights directly in Create */}
                <div className="p-3 bg-bg-surface-2 border border-border rounded-xl space-y-3">
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-2 border-b border-border/60">
                    <div>
                      <h4 className="font-semibold text-white">Pilih Upstream Providers &amp; Bobot</h4>
                      <p className="text-[11px] text-text-muted mt-0.5">
                        Centang provider untuk membatasi failover. Kosongkan jika ingin berlaku ke semua provider pendukung.
                      </p>
                    </div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        onClick={handleSelectMatchingProviders}
                      >
                        {targetModelId ? 'Pilih Yang Mendukung' : 'Pilih Semua'}
                      </Button>
                      {createRuleProviders.length > 0 && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={handleClearCreateProviders}
                        >
                          Bersihkan
                        </Button>
                      )}
                    </div>
                  </div>

                  <div className="space-y-2 max-h-60 overflow-y-auto pr-1">
                    {providers.map((p) => {
                      const targetM = models.find((m) => m.id === targetModelId);
                      const matchedUpstream = targetM?.providers?.find((mp) => mp.provider_id === p.id);
                      const isChecked = createRuleProviders.includes(p.id);

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
                            <label className="flex items-center gap-2 font-semibold text-white cursor-pointer min-w-0">
                              <input
                                type="checkbox"
                                checked={isChecked}
                                onChange={() => toggleCreateProvider(p.id)}
                                className="rounded border-border bg-bg-surface-2 text-purple-400 focus:ring-purple-400 flex-shrink-0 cursor-pointer"
                              />
                              <span className="truncate">{p.display_name || p.name}</span>
                              <span className="text-[10px] text-text-muted font-mono">({p.kind})</span>
                            </label>

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
                                value={createRuleWeights[p.id] || 1}
                                onChange={(e) => updateCreateWeight(p.id, parseInt(e.target.value) || 1)}
                                className="w-20 px-2 py-0.5 bg-bg-surface-2 border border-border rounded text-white text-xs font-mono focus:outline-none focus:border-purple-400 focus:ring-1 focus:ring-purple-400"
                              />
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}

            {/* Mode 3: Combo Pipeline Form Fields (Opsi A, B, C) */}
            {mode === 'combo_routing' && (
              <div className="space-y-4 pt-1 border-t border-border/40">
                {/* Opsi A: Virtual Model Endpoint / Alias */}
                <div className="bg-bg-surface-2/60 p-3.5 rounded-lg border border-border/50">
                  <div className="flex items-center gap-2 mb-1.5">
                    <Globe className="w-4 h-4 text-accent" />
                    <label className="text-xs font-semibold text-white uppercase tracking-wider">
                      Virtual Model Endpoint (Opsi A - Pipeline Gateway)
                    </label>
                    <Badge variant="lime">Opsional</Badge>
                  </div>
                  <p className="text-[11px] text-text-muted mb-2 leading-relaxed">
                    Nama model virtual yang dapat dipanggil langsung oleh aplikasi klien (Open WebUI, Python OpenAI SDK, curl) tanpa perlu mengetahui model asli di baliknya.
                  </p>
                  <input
                    type="text"
                    value={virtualAlias}
                    onChange={(e) => setVirtualAlias(e.target.value.toLowerCase().replace(/[^a-z0-9_.-]/g, '-'))}
                    placeholder="misal: smart-combo, prod-gateway, cost-saver"
                    className="w-full px-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                  />
                  {virtualAlias && (
                    <div className="mt-2 text-[10px] text-accent/90 font-mono flex items-center gap-1.5">
                      <Terminal className="w-3 h-3" />
                      <span>Endpoint siap dipanggil: <code className="bg-surface px-1 py-0.5 rounded text-white font-bold">{virtualAlias}</code></span>
                    </div>
                  )}
                </div>

                {/* Tier 1: Model Utama + Provider Selector */}
                <div className="bg-bg-surface-2/60 p-3.5 rounded-lg border border-border/50 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="w-5 h-5 rounded-full bg-accent text-bg-base font-bold text-xs flex items-center justify-center">1</span>
                      <span className="text-xs font-bold text-white uppercase tracking-wider">Tier 1: Model Utama (Primary)</span>
                    </div>
                    <Badge variant="lime">Prioritas 1</Badge>
                  </div>
                  <Select
                    label="Pilih Model Utama"
                    required
                    value={tier1ModelId}
                    onChange={(val) => {
                      setTier1ModelId(val);
                      setTier1Providers([]);
                    }}
                    placeholder="-- Pilih Model Tier 1 --"
                    options={models.map((m) => ({
                      value: m.id,
                      label: `${m.display_name} (${m.model_id})`,
                      description: m.providers?.length ? `Tersedia di ${m.providers.length} provider` : undefined,
                    }))}
                  />
                  {tier1ModelId && (
                    <div>
                      <label className="block text-[11px] font-semibold text-text-secondary mb-1">
                        Penyaringan Provider Tier 1 (Opsional)
                      </label>
                      {(() => {
                        const m = models.find((mod) => mod.id === tier1ModelId);
                        const provs = m?.providers || [];
                        if (provs.length === 0) {
                          return <span className="text-[10px] text-text-muted">Tidak ada mapping provider untuk model ini.</span>;
                        }
                        return (
                          <div className="flex flex-wrap gap-1.5 mt-1">
                            {provs.map((mp) => {
                              const active = tier1Providers.includes(mp.provider_id);
                              return (
                                <button
                                  key={mp.provider_id}
                                  type="button"
                                  onClick={() => {
                                    setTier1Providers((prev) =>
                                      prev.includes(mp.provider_id)
                                        ? prev.filter((id) => id !== mp.provider_id)
                                        : [...prev, mp.provider_id]
                                    );
                                  }}
                                  className={`text-[10px] px-2 py-1 rounded border transition-colors cursor-pointer ${
                                    active
                                      ? 'bg-accent/20 border-accent text-accent font-semibold'
                                      : 'bg-surface border-border text-text-secondary hover:border-border-hover'
                                  }`}
                                >
                                  {mp.display_name || mp.provider_name} {active && '✓'}
                                </button>
                              );
                            })}
                            {tier1Providers.length > 0 && (
                              <button
                                type="button"
                                onClick={() => setTier1Providers([])}
                                className="text-[10px] px-1.5 py-1 text-text-muted hover:text-white cursor-pointer"
                              >
                                Reset (Semua)
                              </button>
                            )}
                          </div>
                        );
                      })()}
                      <span className="text-[10px] text-text-muted mt-1 block">
                        {tier1Providers.length === 0
                          ? 'Semua provider model ini diizinkan melayani Tier 1.'
                          : `${tier1Providers.length} provider dipilih untuk Tier 1.`}
                      </span>
                    </div>
                  )}
                </div>

                {/* Opsi B: N-Tier Dynamic Cascade Fallback Tiers */}
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <label className="text-xs font-bold text-white uppercase tracking-wider flex items-center gap-1.5">
                      <Layers className="w-3.5 h-3.5 text-accent" />
                      Fallback Tiers (Opsi B - N-Tier Cascade)
                    </label>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => {
                        setFallbackTiers((prev) => [
                          ...prev,
                          { id: Math.random().toString(), model_id: '', provider_ids: [] },
                        ]);
                      }}
                      className="text-xs py-1"
                    >
                      <Plus className="w-3.5 h-3.5 mr-1" /> Tambah Tier Fallback
                    </Button>
                  </div>

                  {fallbackTiers.map((tier, idx) => {
                    const tierNum = idx + 2;
                    const selModel = models.find((m) => m.id === tier.model_id);
                    const provs = selModel?.providers || [];

                    return (
                      <div key={tier.id} className="bg-bg-surface-2/60 p-3.5 rounded-lg border border-border/50 space-y-3 relative">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <span className="w-5 h-5 rounded-full bg-purple-500/20 text-purple-400 font-bold text-xs flex items-center justify-center border border-purple-500/30">
                              {tierNum}
                            </span>
                            <span className="text-xs font-bold text-white uppercase tracking-wider">
                              Tier {tierNum} Fallback
                            </span>
                          </div>
                          {fallbackTiers.length > 1 && (
                            <button
                              type="button"
                              onClick={() => {
                                setFallbackTiers((prev) => prev.filter((t) => t.id !== tier.id));
                              }}
                              className="p-1 rounded text-red-400/70 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                              title={`Hapus Tier ${tierNum}`}
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                        </div>

                        <Select
                          label={`Model Tier ${tierNum}`}
                          required
                          value={tier.model_id}
                          onChange={(val) => {
                            setFallbackTiers((prev) =>
                              prev.map((t) => (t.id === tier.id ? { ...t, model_id: val, provider_ids: [] } : t))
                            );
                          }}
                          placeholder={`-- Pilih Model Tier ${tierNum} --`}
                          options={models
                            .filter((m) => m.id !== tier1ModelId && !fallbackTiers.some((ot) => ot.id !== tier.id && ot.model_id === m.id))
                            .map((m) => ({
                              value: m.id,
                              label: `${m.display_name} (${m.model_id})`,
                              description: m.providers?.length ? `Tersedia di ${m.providers.length} provider` : undefined,
                            }))}
                        />

                        {tier.model_id && (
                          <div>
                            <label className="block text-[11px] font-semibold text-text-secondary mb-1">
                              Penyaringan Provider Tier {tierNum} (Opsional)
                            </label>
                            {provs.length === 0 ? (
                              <span className="text-[10px] text-text-muted">Tidak ada mapping provider untuk model ini.</span>
                            ) : (
                              <div className="flex flex-wrap gap-1.5 mt-1">
                                {provs.map((mp) => {
                                  const active = tier.provider_ids.includes(mp.provider_id);
                                  return (
                                    <button
                                      key={mp.provider_id}
                                      type="button"
                                      onClick={() => {
                                        setFallbackTiers((prev) =>
                                          prev.map((t) => {
                                            if (t.id !== tier.id) return t;
                                            const newPids = active
                                              ? t.provider_ids.filter((p) => p !== mp.provider_id)
                                              : [...t.provider_ids, mp.provider_id];
                                            return { ...t, provider_ids: newPids };
                                          })
                                        );
                                      }}
                                      className={`text-[10px] px-2 py-1 rounded border transition-colors cursor-pointer ${
                                        active
                                          ? 'bg-purple-500/20 border-purple-500 text-purple-300 font-semibold'
                                          : 'bg-surface border-border text-text-secondary hover:border-border-hover'
                                      }`}
                                    >
                                      {mp.display_name || mp.provider_name} {active && '✓'}
                                    </button>
                                  );
                                })}
                                {tier.provider_ids.length > 0 && (
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setFallbackTiers((prev) =>
                                        prev.map((t) => (t.id === tier.id ? { ...t, provider_ids: [] } : t))
                                      );
                                    }}
                                    className="text-[10px] px-1.5 py-1 text-text-muted hover:text-white cursor-pointer"
                                  >
                                    Reset (Semua)
                                  </button>
                                )}
                              </div>
                            )}
                            <span className="text-[10px] text-text-muted mt-1 block">
                              {tier.provider_ids.length === 0
                                ? `Semua provider diizinkan melayani Tier ${tierNum}.`
                                : `${tier.provider_ids.length} provider dipilih untuk Tier ${tierNum}.`}
                            </span>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>

                {/* Opsi C: Proteksi & Otomasi Cerdas */}
                <div className="p-3 bg-accent/5 border border-accent/20 rounded-lg space-y-2">
                  <div className="flex items-center gap-2 text-accent font-semibold text-xs">
                    <ShieldCheck className="w-4 h-4" />
                    <span>Proteksi &amp; Otomasi Cerdas (Opsi C Aktif)</span>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-[11px] text-text-secondary">
                    <div className="flex items-start gap-1.5">
                      <span className="text-accent font-bold">⚡</span>
                      <span><strong>Smart Context Window Bypass:</strong> Jika token prompt melebihi limit Tier 1, gateway otomatis melompati ke tier yang muat tanpa error 400.</span>
                    </div>
                    <div className="flex items-start gap-1.5">
                      <span className="text-accent font-bold">🛡️</span>
                      <span><strong>Multi-Error Failover:</strong> Kegagalan kuota/rate-limit (429), server error (5xx), atau timeout langsung memicu fallback mulus ke tier berikutnya.</span>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Circuit Breaker & Evaluation Priority */}
            <div className="grid grid-cols-2 gap-3 pt-2 border-t border-border/40">
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Prioritas Evaluasi
                </label>
                <input
                  type="number"
                  value={newRule.priority}
                  onChange={(e) => setNewRule({ ...newRule, priority: parseInt(e.target.value) || 100 })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                />
                <span className="text-[10px] text-text-muted mt-0.5 block">
                  Angka lebih kecil dievaluasi lebih awal (bawaan: 100).
                </span>
              </div>
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Ambang Circuit Breaker
                </label>
                <input
                  type="number"
                  value={newRule.failure_threshold}
                  onChange={(e) =>
                    setNewRule({ ...newRule, failure_threshold: parseInt(e.target.value) || 5 })
                  }
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                />
                <span className="text-[10px] text-text-muted mt-0.5 block">
                  Jumlah kegagalan berturut sebelum isolasi otomatis.
                </span>
              </div>
            </div>

            <Button type="submit" variant="primary" size="md" className="w-full mt-4">
              Simpan Aturan Routing ({mode === 'model_only' ? 'Model Only' : mode === 'combo_routing' ? 'Combo Routing' : 'Multi-Provider'})
            </Button>
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
        subtitle="Perbarui jalur eksekusi, strategi failover, provider pendukung, atau parameter sirkuit."
        maxWidth="3xl"
      >
        <div className="space-y-5 text-xs">
          {/* 3-Mode Selector Tabs */}
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1.5">
              Pilih Mode Routing
            </label>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 p-1.5 bg-bg-surface-2 border border-border rounded-xl">
              <button
                type="button"
                onClick={() => setEditMode('model_only')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  editMode === 'model_only'
                    ? 'bg-sky-500/15 border border-sky-400/60 text-sky-300 shadow-sm shadow-sky-500/10'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-1 border border-transparent'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Zap className={`w-4 h-4 ${editMode === 'model_only' ? 'text-sky-400' : 'text-text-muted'}`} />
                  <span className="font-bold">1. Model Only</span>
                </div>
                <span className={`text-[10px] font-normal ${editMode === 'model_only' ? 'text-sky-400/80' : 'opacity-70'}`}>Direct 1:1 Passthrough</span>
              </button>
              <button
                type="button"
                onClick={() => setEditMode('routing')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  editMode === 'routing'
                    ? 'bg-purple-500/15 border border-purple-400/60 text-purple-300 shadow-sm shadow-purple-500/10'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-1 border border-transparent'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Shuffle className={`w-4 h-4 ${editMode === 'routing' ? 'text-purple-400' : 'text-text-muted'}`} />
                  <span className="font-bold">2. Routing</span>
                </div>
                <span className={`text-[10px] font-normal ${editMode === 'routing' ? 'text-purple-400/80' : 'opacity-70'}`}>Multi-Provider Failover</span>
              </button>
              <button
                type="button"
                onClick={() => setEditMode('combo_routing')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  editMode === 'combo_routing'
                    ? 'bg-accent/15 border border-accent/60 text-accent shadow-sm shadow-accent/10'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-1 border border-transparent'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Layers className={`w-4 h-4 ${editMode === 'combo_routing' ? 'text-accent' : 'text-text-muted'}`} />
                  <span className="font-bold">3. Combo Pipeline</span>
                </div>
                <span className={`text-[10px] font-normal ${editMode === 'combo_routing' ? 'text-accent/80' : 'opacity-70'}`}>N-Tier &amp; Smart Gateway</span>
              </button>
            </div>
          </div>

          <form onSubmit={handleEditSave} className="space-y-4">
            {editMode === 'model_only' && (
              <div className="p-3.5 bg-sky-500/10 border border-sky-500/25 rounded-xl text-sky-300 text-[11px] leading-relaxed flex items-start gap-2.5">
                <Zap className="w-4 h-4 text-sky-400 shrink-0 mt-0.5" />
                <div>
                  <strong>Mode Model Only:</strong> Passthrough langsung 1-to-1 ke model dan provider tertentu tanpa overhead failover. Latensi paling instan (&lt; 2ms saat cache hit).
                </div>
              </div>
            )}

            {editMode === 'routing' && (
              <div className="p-3.5 bg-purple-500/10 border border-purple-500/25 rounded-xl text-purple-300 text-[11px] leading-relaxed flex items-start gap-2.5">
                <Shuffle className="w-4 h-4 text-purple-400 shrink-0 mt-0.5" />
                <div>
                  <strong>Mode Multi-Provider Routing:</strong> Mendistribusikan lalu lintas atau failover antar beberapa provider upstream (Priority, Lowest Latency, Lowest Cost, Weighted, Round Robin).
                </div>
              </div>
            )}

            {editMode === 'combo_routing' && (
              <div className="p-3.5 bg-accent/10 border border-accent/25 rounded-xl text-accent text-[11px] leading-relaxed flex items-start gap-2.5">
                <Layers className="w-4 h-4 text-accent shrink-0 mt-0.5" />
                <div>
                  <strong>Mode Combo Pipeline (Opsi A, B, &amp; C):</strong> Rantai bertingkat dinamis (N-Tier) dengan multi-provider per tier, virtual model alias gateway, auto context bypass, dan multi-error failover (429, 5xx, timeout).
                </div>
              </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Nama Aturan
                </label>
                <input
                  type="text"
                  placeholder={
                    editMode === 'model_only'
                      ? 'direct-gpt4o'
                      : editMode === 'combo_routing'
                      ? 'combo-local-to-flagship'
                      : 'failover-deepseek'
                  }
                  value={editRuleForm.name}
                  onChange={(e) => setEditRuleForm({ ...editRuleForm, name: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                />
              </div>
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Deskripsi (Opsional)
                </label>
                <input
                  type="text"
                  placeholder="Catatan tujuan atau spesifikasi aturan..."
                  value={editRuleForm.description}
                  onChange={(e) => setEditRuleForm({ ...editRuleForm, description: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
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

            {/* Mode 2: Routing Form Fields */}
            {editMode === 'routing' && (
              <div className="space-y-4 pt-1 border-t border-border/40">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <Select
                    label="Target Model"
                    value={editTargetModelId}
                    onChange={(val) => setEditTargetModelId(val)}
                    placeholder="-- Semua Model (Catch-All) --"
                    options={[
                      { value: '', label: '-- Semua Model (Catch-All) --' },
                      ...models.map((m) => {
                        const provNames = m.providers?.map((p) => p.display_name || p.provider_name).join(', ');
                        return {
                          value: m.id,
                          label: `${m.display_name} (${m.model_id})`,
                          description: provNames ? `Tersedia di: ${provNames}` : (m.family ? `Keluarga: ${m.family}` : undefined),
                        };
                      }),
                    ]}
                  />
                  <Select
                    label="Strategi Routing"
                    value={editRuleForm.strategy}
                    onChange={(val) => setEditRuleForm({ ...editRuleForm, strategy: val })}
                    options={[
                      { value: 'priority', label: 'Priority (Urutan Tertinggi)', description: 'Mengarahkan ke upstream prioritas tertinggi' },
                      { value: 'lowest_cost', label: 'Lowest Cost (Biaya Termurah)', description: 'Mengarahkan ke provider dengan tarif token terendah' },
                      { value: 'lowest_latency', label: 'Lowest Latency (Latensi Terendah)', description: 'Mengarahkan ke provider dengan respon tergesit' },
                      { value: 'weighted', label: 'Weighted (Bobot Proporsional)', description: 'Distribusi beban berbasis rasio bobot upstream' },
                      { value: 'round_robin', label: 'Round Robin (Beban Berimbang)', description: 'Distribusi bergilir seimbang antar semua upstream' },
                    ]}
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block font-semibold text-text-secondary uppercase mb-1">Maksimal Percobaan</label>
                    <input
                      type="number"
                      min="1"
                      max="10"
                      value={editRuleForm.max_attempts}
                      onChange={(e) => setEditRuleForm({ ...editRuleForm, max_attempts: parseInt(e.target.value) || 3 })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                    />
                  </div>
                  <div>
                    <label className="block font-semibold text-text-secondary uppercase mb-1">Jeda Backoff (ms)</label>
                    <input
                      type="number"
                      min="0"
                      value={editRuleForm.backoff_ms}
                      onChange={(e) => setEditRuleForm({ ...editRuleForm, backoff_ms: parseInt(e.target.value) || 200 })}
                      className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                    />
                  </div>
                </div>

                {/* Provider Selection & Weights directly in Edit */}
                <div className="p-3 bg-bg-surface-2 border border-border rounded-xl space-y-3">
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-2 border-b border-border/60">
                    <div>
                      <h4 className="font-semibold text-white">Pilih Upstream Providers &amp; Bobot</h4>
                      <p className="text-[11px] text-text-muted mt-0.5">
                        Centang provider untuk membatasi failover. Kosongkan jika ingin berlaku ke semua provider pendukung.
                      </p>
                    </div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        onClick={handleSelectMatchingEditProviders}
                      >
                        {editTargetModelId ? 'Pilih Yang Mendukung' : 'Pilih Semua'}
                      </Button>
                      {editRuleProviders.length > 0 && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={handleClearEditProviders}
                        >
                          Bersihkan
                        </Button>
                      )}
                    </div>
                  </div>

                  <div className="space-y-2 max-h-60 overflow-y-auto pr-1">
                    {providers.map((p) => {
                      const targetM = models.find((m) => m.id === editTargetModelId);
                      const matchedUpstream = targetM?.providers?.find((mp) => mp.provider_id === p.id);
                      const isChecked = editRuleProviders.includes(p.id);

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
                            <label className="flex items-center gap-2 font-semibold text-white cursor-pointer min-w-0">
                              <input
                                type="checkbox"
                                checked={isChecked}
                                onChange={() => toggleEditProvider(p.id)}
                                className="rounded border-border bg-bg-surface-2 text-purple-400 focus:ring-purple-400 flex-shrink-0 cursor-pointer"
                              />
                              <span className="truncate">{p.display_name || p.name}</span>
                              <span className="text-[10px] text-text-muted font-mono">({p.kind})</span>
                            </label>

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
                                value={editRuleWeights[p.id] || 1}
                                onChange={(e) => updateEditWeight(p.id, parseInt(e.target.value) || 1)}
                                className="w-20 px-2 py-0.5 bg-bg-surface-2 border border-border rounded text-white text-xs font-mono focus:outline-none focus:border-purple-400 focus:ring-1 focus:ring-purple-400"
                              />
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            )}

            {/* Mode 3: Combo Pipeline Form Fields (Opsi A, B, C) */}
            {editMode === 'combo_routing' && (
              <div className="space-y-4 pt-1 border-t border-border/40">
                {/* Opsi A: Virtual Model Endpoint / Alias */}
                <div className="bg-bg-surface-2/60 p-3.5 rounded-lg border border-border/50">
                  <div className="flex items-center gap-2 mb-1.5">
                    <Globe className="w-4 h-4 text-accent" />
                    <label className="text-xs font-semibold text-white uppercase tracking-wider">
                      Virtual Model Endpoint (Opsi A - Pipeline Gateway)
                    </label>
                    <Badge variant="lime">Opsional</Badge>
                  </div>
                  <p className="text-[11px] text-text-muted mb-2 leading-relaxed">
                    Nama model virtual yang dapat dipanggil langsung oleh aplikasi klien (Open WebUI, Python OpenAI SDK, curl) tanpa perlu mengetahui model asli di baliknya.
                  </p>
                  <input
                    type="text"
                    value={editVirtualAlias}
                    onChange={(e) => setEditVirtualAlias(e.target.value.toLowerCase().replace(/[^a-z0-9_.-]/g, '-'))}
                    placeholder="misal: smart-combo, prod-gateway, cost-saver"
                    className="w-full px-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono text-xs focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                  />
                  {editVirtualAlias && (
                    <div className="mt-2 text-[10px] text-accent/90 font-mono flex items-center gap-1.5">
                      <Terminal className="w-3 h-3" />
                      <span>Endpoint siap dipanggil: <code className="bg-surface px-1 py-0.5 rounded text-white font-bold">{editVirtualAlias}</code></span>
                    </div>
                  )}
                </div>

                {/* Tier 1: Model Utama + Provider Selector */}
                <div className="bg-bg-surface-2/60 p-3.5 rounded-lg border border-border/50 space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="w-5 h-5 rounded-full bg-accent text-bg-base font-bold text-xs flex items-center justify-center">1</span>
                      <span className="text-xs font-bold text-white uppercase tracking-wider">Tier 1: Model Utama (Primary)</span>
                    </div>
                    <Badge variant="lime">Prioritas 1</Badge>
                  </div>
                  <Select
                    label="Pilih Model Utama"
                    required
                    value={editTier1ModelId}
                    onChange={(val) => {
                      setEditTier1ModelId(val);
                      setEditTier1Providers([]);
                    }}
                    placeholder="-- Pilih Model Tier 1 --"
                    options={models.map((m) => ({
                      value: m.id,
                      label: `${m.display_name} (${m.model_id})`,
                      description: m.providers?.length ? `Tersedia di ${m.providers.length} provider` : undefined,
                    }))}
                  />
                  {editTier1ModelId && (
                    <div>
                      <label className="block text-[11px] font-semibold text-text-secondary mb-1">
                        Penyaringan Provider Tier 1 (Opsional)
                      </label>
                      {(() => {
                        const m = models.find((mod) => mod.id === editTier1ModelId);
                        const provs = m?.providers || [];
                        if (provs.length === 0) {
                          return <span className="text-[10px] text-text-muted">Tidak ada mapping provider untuk model ini.</span>;
                        }
                        return (
                          <div className="flex flex-wrap gap-1.5 mt-1">
                            {provs.map((mp) => {
                              const active = editTier1Providers.includes(mp.provider_id);
                              return (
                                <button
                                  key={mp.provider_id}
                                  type="button"
                                  onClick={() => {
                                    setEditTier1Providers((prev) =>
                                      prev.includes(mp.provider_id)
                                        ? prev.filter((id) => id !== mp.provider_id)
                                        : [...prev, mp.provider_id]
                                    );
                                  }}
                                  className={`text-[10px] px-2 py-1 rounded border transition-colors cursor-pointer ${
                                    active
                                      ? 'bg-accent/20 border-accent text-accent font-semibold'
                                      : 'bg-surface border-border text-text-secondary hover:border-border-hover'
                                  }`}
                                >
                                  {mp.display_name || mp.provider_name} {active && '✓'}
                                </button>
                              );
                            })}
                            {editTier1Providers.length > 0 && (
                              <button
                                type="button"
                                onClick={() => setEditTier1Providers([])}
                                className="text-[10px] px-1.5 py-1 text-text-muted hover:text-white cursor-pointer"
                              >
                                Reset (Semua)
                              </button>
                            )}
                          </div>
                        );
                      })()}
                      <span className="text-[10px] text-text-muted mt-1 block">
                        {editTier1Providers.length === 0
                          ? 'Semua provider model ini diizinkan melayani Tier 1.'
                          : `${editTier1Providers.length} provider dipilih untuk Tier 1.`}
                      </span>
                    </div>
                  )}
                </div>

                {/* Opsi B: N-Tier Dynamic Cascade Fallback Tiers */}
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <label className="text-xs font-bold text-white uppercase tracking-wider flex items-center gap-1.5">
                      <Layers className="w-3.5 h-3.5 text-accent" />
                      Fallback Tiers (Opsi B - N-Tier Cascade)
                    </label>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => {
                        setEditFallbackTiers((prev) => [
                          ...prev,
                          { id: Math.random().toString(), model_id: '', provider_ids: [] },
                        ]);
                      }}
                      className="text-xs py-1"
                    >
                      <Plus className="w-3.5 h-3.5 mr-1" /> Tambah Tier Fallback
                    </Button>
                  </div>

                  {editFallbackTiers.map((tier, idx) => {
                    const tierNum = idx + 2;
                    const selModel = models.find((m) => m.id === tier.model_id);
                    const provs = selModel?.providers || [];

                    return (
                      <div key={tier.id} className="bg-bg-surface-2/60 p-3.5 rounded-lg border border-border/50 space-y-3 relative">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <span className="w-5 h-5 rounded-full bg-purple-500/20 text-purple-400 font-bold text-xs flex items-center justify-center border border-purple-500/30">
                              {tierNum}
                            </span>
                            <span className="text-xs font-bold text-white uppercase tracking-wider">
                              Tier {tierNum} Fallback
                            </span>
                          </div>
                          {editFallbackTiers.length > 1 && (
                            <button
                              type="button"
                              onClick={() => {
                                setEditFallbackTiers((prev) => prev.filter((t) => t.id !== tier.id));
                              }}
                              className="p-1 rounded text-red-400/70 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                              title={`Hapus Tier ${tierNum}`}
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                        </div>

                        <Select
                          label={`Model Tier ${tierNum}`}
                          required
                          value={tier.model_id}
                          onChange={(val) => {
                            setEditFallbackTiers((prev) =>
                              prev.map((t) => (t.id === tier.id ? { ...t, model_id: val, provider_ids: [] } : t))
                            );
                          }}
                          placeholder={`-- Pilih Model Tier ${tierNum} --`}
                          options={models
                            .filter((m) => m.id !== editTier1ModelId && !editFallbackTiers.some((ot) => ot.id !== tier.id && ot.model_id === m.id))
                            .map((m) => ({
                              value: m.id,
                              label: `${m.display_name} (${m.model_id})`,
                              description: m.providers?.length ? `Tersedia di ${m.providers.length} provider` : undefined,
                            }))}
                        />

                        {tier.model_id && (
                          <div>
                            <label className="block text-[11px] font-semibold text-text-secondary mb-1">
                              Penyaringan Provider Tier {tierNum} (Opsional)
                            </label>
                            {provs.length === 0 ? (
                              <span className="text-[10px] text-text-muted">Tidak ada mapping provider untuk model ini.</span>
                            ) : (
                              <div className="flex flex-wrap gap-1.5 mt-1">
                                {provs.map((mp) => {
                                  const active = tier.provider_ids.includes(mp.provider_id);
                                  return (
                                    <button
                                      key={mp.provider_id}
                                      type="button"
                                      onClick={() => {
                                        setEditFallbackTiers((prev) =>
                                          prev.map((t) => {
                                            if (t.id !== tier.id) return t;
                                            const newPids = active
                                              ? t.provider_ids.filter((p) => p !== mp.provider_id)
                                              : [...t.provider_ids, mp.provider_id];
                                            return { ...t, provider_ids: newPids };
                                          })
                                        );
                                      }}
                                      className={`text-[10px] px-2 py-1 rounded border transition-colors cursor-pointer ${
                                        active
                                          ? 'bg-purple-500/20 border-purple-500 text-purple-300 font-semibold'
                                          : 'bg-surface border-border text-text-secondary hover:border-border-hover'
                                      }`}
                                    >
                                      {mp.display_name || mp.provider_name} {active && '✓'}
                                    </button>
                                  );
                                })}
                                {tier.provider_ids.length > 0 && (
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setEditFallbackTiers((prev) =>
                                        prev.map((t) => (t.id === tier.id ? { ...t, provider_ids: [] } : t))
                                      );
                                    }}
                                    className="text-[10px] px-1.5 py-1 text-text-muted hover:text-white cursor-pointer"
                                  >
                                    Reset (Semua)
                                  </button>
                                )}
                              </div>
                            )}
                            <span className="text-[10px] text-text-muted mt-1 block">
                              {tier.provider_ids.length === 0
                                ? `Semua provider diizinkan melayani Tier ${tierNum}.`
                                : `${tier.provider_ids.length} provider dipilih untuk Tier ${tierNum}.`}
                            </span>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>

                {/* Opsi C: Proteksi & Otomasi Cerdas */}
                <div className="p-3 bg-accent/5 border border-accent/20 rounded-lg space-y-2">
                  <div className="flex items-center gap-2 text-accent font-semibold text-xs">
                    <ShieldCheck className="w-4 h-4" />
                    <span>Proteksi &amp; Otomasi Cerdas (Opsi C Aktif)</span>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-[11px] text-text-secondary">
                    <div className="flex items-start gap-1.5">
                      <span className="text-accent font-bold">⚡</span>
                      <span><strong>Smart Context Window Bypass:</strong> Jika token prompt melebihi limit Tier 1, gateway otomatis melompati ke tier yang muat tanpa error 400.</span>
                    </div>
                    <div className="flex items-start gap-1.5">
                      <span className="text-accent font-bold">🛡️</span>
                      <span><strong>Multi-Error Failover:</strong> Kegagalan kuota/rate-limit (429), server error (5xx), atau timeout langsung memicu fallback mulus ke tier berikutnya.</span>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Circuit Breaker & Evaluation Priority */}
            <div className="grid grid-cols-2 gap-3 pt-2 border-t border-border/40">
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Prioritas Evaluasi
                </label>
                <input
                  type="number"
                  value={editRuleForm.priority}
                  onChange={(e) => setEditRuleForm({ ...editRuleForm, priority: parseInt(e.target.value) || 100 })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                />
                <span className="text-[10px] text-text-muted mt-0.5 block">
                  Angka lebih kecil dievaluasi lebih awal (bawaan: 100).
                </span>
              </div>
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Ambang Circuit Breaker
                </label>
                <input
                  type="number"
                  value={editRuleForm.failure_threshold}
                  onChange={(e) =>
                    setEditRuleForm({ ...editRuleForm, failure_threshold: parseInt(e.target.value) || 5 })
                  }
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                />
                <span className="text-[10px] text-text-muted mt-0.5 block">
                  Jumlah kegagalan berturut sebelum isolasi otomatis.
                </span>
              </div>
            </div>

            <Button type="submit" variant="primary" size="md" className="w-full mt-4">
              Simpan Perubahan Aturan ({editMode === 'model_only' ? 'Model Only' : editMode === 'combo_routing' ? 'Combo Routing' : 'Multi-Provider'})
            </Button>
          </form>
        </div>
      </Drawer>
    
      <Drawer
        isOpen={isProvidersDrawerOpen}
        onClose={() => setIsProvidersDrawerOpen(false)}
        title="Edit Providers & Weights"
        subtitle={
          selectedRule?.match_model_id
            ? `Aturan ini terhubung ke model: ${models.find(m => m.id === selectedRule.match_model_id || m.model_id === selectedRule.match_model_id)?.display_name || selectedRule.match_model_id}`
            : "Pilih provider yang akan digunakan dalam aturan routing ini dan atur bobot (untuk mode weighted)."
        }
      >
        <form onSubmit={handleSaveProviders} className="space-y-4 text-xs">
          <div className="space-y-3">
            {providers.map(p => {
              const targetM = models.find(m => m.id === selectedRule?.match_model_id || m.model_id === selectedRule?.match_model_id);
              const matchedUpstream = targetM?.providers?.find(mp => mp.provider_id === p.id);
              return (
                <div key={p.id} className="p-3 bg-bg-surface-2 border border-border rounded-lg flex flex-col gap-2">
                  <div className="flex items-center justify-between gap-2">
                    <label className="flex items-center gap-2 font-semibold text-white cursor-pointer min-w-0">
                      <input
                        type="checkbox"
                        checked={ruleProviders.includes(p.id)}
                        onChange={() => toggleProvider(p.id)}
                        className="rounded border-border bg-bg-surface-2 text-accent focus:ring-accent flex-shrink-0 cursor-pointer"
                      />
                      <span className="truncate">{p.display_name || p.name}</span>
                      <span className="text-[10px] text-text-muted font-mono">({p.kind})</span>
                    </label>
                    {targetM ? (
                      matchedUpstream ? (
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 flex-shrink-0">
                          ✓ {matchedUpstream.upstream_model_name}
                        </span>
                      ) : (
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-bg-surface-1 text-text-muted border border-border/60 flex-shrink-0">
                          Model tidak tersedia
                        </span>
                      )
                    ) : null}
                  </div>
                  {ruleProviders.includes(p.id) && (
                    <div className="pl-6 flex items-center gap-2 mt-1">
                      <label className="text-text-secondary">Bobot (Weight):</label>
                      <input
                        type="number"
                        min="1"
                        value={ruleWeights[p.id] || 1}
                        onChange={(e) => updateWeight(p.id, parseInt(e.target.value) || 1)}
                        className="w-24 px-2 py-1 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
                      />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-3">
            Simpan Providers
          </Button>
        </form>
      </Drawer>
</div>
  );
};
