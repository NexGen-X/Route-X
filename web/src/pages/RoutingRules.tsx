import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RoutingRule, CircuitBreakerStatus, Model, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { GitFork, Plus, Trash2, ZapOff, RotateCcw, RefreshCw, Zap, Shuffle, Layers } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

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

  const [rulesError, setRulesError] = useState<string | null>(null);
  const [breakersError, setBreakersError] = useState<string | null>(null);

  // 3-Mode Form States
  const [mode, setMode] = useState<'model_only' | 'routing' | 'combo_routing'>('model_only');
  const [targetModelId, setTargetModelId] = useState('');
  const [targetProviderId, setTargetProviderId] = useState('');
  const [tier1ModelId, setTier1ModelId] = useState('');
  const [tier2ModelId, setTier2ModelId] = useState('');
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

  const getRuleMode = (r: RoutingRule): { mode: string; label: string; badgeVariant: 'info' | 'lime' | 'warn'; detail: string } => {
    const desc = r.description || '';
    // Hanya tag struktur [combo:tier2=...] / [model_only] yang menentukan mode —
    // kata bebas di deskripsi custom tidak boleh mengubah badge.
    if (desc.includes('[combo:tier2=')) {
      const match = desc.match(/\[combo:tier2=([^\]]+)\]/);
      const tier2 = match ? match[1] : 'Flagship';
      return { mode: 'combo_routing', label: 'COMBO ROUTING', badgeVariant: 'lime', detail: `Tier 2 Fallback: ${tier2}` };
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
    setTier1ModelId('');
    setTier2ModelId('');
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
          description:
            newRule.description ||
            `[model_only] Direct 1:1 passthrough ke model ${modelName}${selProv ? ` via ${selProv.name}` : ''}`,
        };
      } else if (mode === 'combo_routing') {
        if (!tier1ModelId || !tier2ModelId) {
          toast.error('Pilih model Tier 1 dan Tier 2 terlebih dahulu.');
          return;
        }
        if (tier1ModelId === tier2ModelId) {
          toast.error('Tier 1 dan Tier 2 tidak boleh sama — self-fallback membuat loop.');
          return;
        }
        const selT1 = models.find((m) => m.id === tier1ModelId);
        const selT2 = models.find((m) => m.id === tier2ModelId);
        if (!selT1 || !selT2) {
          toast.error('Model Tier 1 atau Tier 2 tidak valid di registry.');
          return;
        }
        payload = {
          ...payload,
          name:
            newRule.name ||
            `combo-${selT1.model_id.replace(/[^a-zA-Z0-9_-]/g, '-')}-to-${selT2.model_id.replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: 'priority',
          max_attempts: 2,
          match_model_id: selT1.id,
          description:
            newRule.description ||
            `[combo:tier2=${selT2.model_id}] Smart Tiered Cascade: Tier 1 (${selT1.display_name}) -> Tier 2 (${selT2.display_name})`,
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
          description:
            newRule.description ||
            `[routing] Failover multi-provider (${newRule.strategy}) untuk ${selModel?.display_name || 'semua model'}`,
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
      setCreateRuleProviders([]);
      setCreateRuleWeights({});
      toast.success('Aturan perutean cerdas berhasil diterapkan');
      loadRules();
    } catch (err: any) {
      toast.error('Gagal membuat aturan: ' + (err.message || err));
    }
  };

  const handleOpenProviders = (r: any) => {
    setSelectedRule(r);
    setRuleProviders(r.provider_ids || []);
    setRuleWeights(r.weights || {});
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
          return (
            <Card key={r.id} className="p-5 flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-accent/10 border border-accent/20 text-accent flex items-center justify-center">
                      <GitFork className="w-5 h-5" />
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

                <div className="mt-4 space-y-2 text-xs">
                  <div className="p-2 rounded bg-surface/70 border border-border/50 font-mono text-[11px] text-accent">
                    {ruleMode.detail}
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
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleOpenProviders(r)}
              >
                Edit Providers
              </Button>
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
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 p-1.5 bg-surface-dark border border-border rounded-xl">
              <button
                type="button"
                onClick={() => setMode('model_only')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  mode === 'model_only'
                    ? 'bg-accent text-white shadow-md shadow-accent/20 border border-accent/40'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-2'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Zap className="w-4 h-4" />
                  <span className="font-bold">1. Model Only</span>
                </div>
                <span className="text-[10px] font-normal opacity-80">Direct 1:1 Passthrough</span>
              </button>
              <button
                type="button"
                onClick={() => setMode('routing')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  mode === 'routing'
                    ? 'bg-accent text-white shadow-md shadow-accent/20 border border-accent/40'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-2'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Shuffle className="w-4 h-4" />
                  <span className="font-bold">2. Routing</span>
                </div>
                <span className="text-[10px] font-normal opacity-80">Multi-Provider Failover</span>
              </button>
              <button
                type="button"
                onClick={() => setMode('combo_routing')}
                className={`py-2.5 px-3 text-center rounded-lg text-xs font-semibold transition-all flex flex-col items-center gap-1.5 cursor-pointer ${
                  mode === 'combo_routing'
                    ? 'bg-accent text-white shadow-md shadow-accent/20 border border-accent/40'
                    : 'text-text-muted hover:text-white hover:bg-bg-surface-2'
                }`}
              >
                <div className="flex items-center gap-1.5">
                  <Layers className="w-4 h-4" />
                  <span className="font-bold">3. Combo Routing</span>
                </div>
                <span className="text-[10px] font-normal opacity-80">Tier 1 &rarr; Tier 2 Cascade</span>
              </button>
            </div>
          </div>

          <form onSubmit={handleCreate} className="space-y-4">
            {mode === 'model_only' && (
              <div className="p-3.5 bg-accent/10 border border-accent/20 rounded-xl text-accent text-[11px] leading-relaxed">
                ⚡ <strong>Mode Model Only:</strong> Passthrough langsung 1-to-1 ke model dan provider tertentu tanpa overhead failover. Latensi paling instan (&lt; 2ms saat cache hit).
              </div>
            )}

            {mode === 'routing' && (
              <div className="p-3.5 bg-purple-500/10 border border-purple-500/20 rounded-xl text-purple-300 text-[11px] leading-relaxed">
                🔀 <strong>Mode Multi-Provider Routing:</strong> Mendistribusikan lalu lintas atau failover antar beberapa provider upstream (Priority, Lowest Latency, Lowest Cost, Weighted, Round Robin).
              </div>
            )}

            {mode === 'combo_routing' && (
              <div className="p-3.5 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-emerald-300 text-[11px] leading-relaxed">
                🔗 <strong>Mode Combo Routing:</strong> Rantai bertingkat cerdas. Permintaan pertama dialokasikan ke <strong>Tier 1 (Lokal/Hemat)</strong>. Bila kuota habis (429) atau upstream error, otomatis dialihkan ke <strong>Tier 2 (Flagship Fallback)</strong>!
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
                  className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
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
                  className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
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
                      className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
                    />
                  </div>
                  <div>
                    <label className="block font-semibold text-text-secondary uppercase mb-1">Jeda Backoff (ms)</label>
                    <input
                      type="number"
                      min="0"
                      value={newRule.backoff_ms}
                      onChange={(e) => setNewRule({ ...newRule, backoff_ms: parseInt(e.target.value) || 200 })}
                      className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
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
                              ? 'bg-accent/10 border-accent/40'
                              : 'bg-bg-surface-1 border-border/80 hover:border-border'
                          }`}
                        >
                          <div className="flex items-center justify-between gap-2">
                            <label className="flex items-center gap-2 font-semibold text-white cursor-pointer min-w-0">
                              <input
                                type="checkbox"
                                checked={isChecked}
                                onChange={() => toggleCreateProvider(p.id)}
                                className="rounded border-border bg-surface-dark text-accent focus:ring-accent flex-shrink-0"
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
                                <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-amber-500/10 text-amber-400 border border-amber-500/20 shrink-0">
                                  Tidak Menyediakan
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
                                className="w-20 px-2 py-0.5 bg-surface-dark border border-border rounded text-white text-xs"
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

            {/* Mode 3: Combo Routing Form Fields */}
            {mode === 'combo_routing' && (
              <div className="space-y-4 pt-1 border-t border-border/40">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div>
                    <Select
                      label="Tier 1 Model (Lokal / Hemat)"
                      required
                      value={tier1ModelId}
                      onChange={(val) => {
                        setTier1ModelId(val);
                        if (val === tier2ModelId) {
                          setTier2ModelId('');
                        }
                      }}
                      placeholder="-- Pilih Model Tier 1 (Utama) --"
                      options={models.map((m) => {
                        const provNames = m.providers?.map((p) => p.display_name || p.provider_name).join(', ');
                        return {
                          value: m.id,
                          label: `${m.display_name} (${m.model_id})`,
                          description: provNames ? `Tersedia di: ${provNames}` : undefined,
                        };
                      })}
                    />
                    <span className="text-[10px] text-text-muted mt-1 block">
                      Diprioritaskan pertama kali untuk efisiensi latensi dan biaya token.
                    </span>
                  </div>

                  <div>
                    <Select
                      label="Tier 2 Model (Flagship Fallback)"
                      required
                      value={tier2ModelId}
                      onChange={(val) => setTier2ModelId(val)}
                      placeholder="-- Pilih Model Tier 2 (Cadangan) --"
                      options={models
                        .filter((m) => m.id !== tier1ModelId)
                        .map((m) => {
                          const provNames = m.providers?.map((p) => p.display_name || p.provider_name).join(', ');
                          return {
                            value: m.id,
                            label: `${m.display_name} (${m.model_id})`,
                            description: provNames ? `Tersedia di: ${provNames}` : undefined,
                          };
                        })}
                    />
                    <span className="text-[10px] text-text-muted mt-1 block">
                      Cadangan otomatis tanpa interupsi jika Tier 1 mengalami down / 429.
                    </span>
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
                  className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
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
                  className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
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
                        className="rounded border-border bg-surface-dark text-accent focus:ring-accent flex-shrink-0"
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
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-amber-500/10 text-amber-400 border border-amber-500/20 flex-shrink-0">
                          Model tidak terdaftar
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
                        className="w-24 px-2 py-1 bg-surface-dark border border-border rounded-nav text-white"
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
