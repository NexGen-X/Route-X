import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RoutingRule, CircuitBreakerStatus, Model, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
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

  // 3-Mode Form States (Fitur 3)
  const [mode, setMode] = useState<'model_only' | 'routing' | 'combo_routing'>('model_only');
  const [targetModelId, setTargetModelId] = useState('');
  const [targetProviderId, setTargetProviderId] = useState('');
  const [tier1ModelName, setTier1ModelName] = useState('');
  const [tier2ModelName, setTier2ModelName] = useState('');

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
          description: `[model_only] Direct 1:1 passthrough ke model ${modelName}${selProv ? ` via ${selProv.name}` : ''}`,
        };
      } else if (mode === 'combo_routing') {
        const t1 = tier1ModelName.trim();
        const t2 = tier2ModelName.trim();
        if (!t1 || !t2) {
          toast.error('Isi nama model Tier 1 dan Tier 2 dulu.');
          return;
        }
        if (t1.toLowerCase() === t2.toLowerCase()) {
          toast.error('Tier 1 dan Tier 2 tidak boleh sama — self-fallback membuat loop.');
          return;
        }
        const tier1M = models.find((m) => m.model_id === t1 || m.display_name === t1);
        if (!tier1M) {
          toast.error(`Model Tier 1 "${t1}" tidak dikenal di registry — pilih nama yang terdaftar agar aturan tidak menjadi catch-all.`);
          return;
        }
        payload = {
          ...payload,
          name: newRule.name || `combo-${t1.replace(/[^a-zA-Z0-9_-]/g, '-')}-to-${t2.replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: 'priority',
          max_attempts: 2,
          match_model_id: tier1M.id,
          description: `[combo:tier2=${t2}] Smart Tiered Cascade: Tier 1 (${t1}) -> Tier 2 (${t2})`,
        };
      } else {
        payload = {
          ...payload,
          name: newRule.name,
          strategy: newRule.strategy,
          max_attempts: newRule.max_attempts,
          match_model_id: targetModelId || undefined,
          provider_ids: targetProviderId ? [targetProviderId] : undefined,
          description: newRule.description || `[routing] Failover multi-provider untuk model`,
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
            onClick={() => setIsCreateOpen(true)}
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

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Buat Aturan Routing Baru"
        subtitle="Pilih salah satu dari 3 mode routing untuk mengatur jalur inferensi"
      >
        <div className="space-y-4 text-xs">
          {/* 3-Mode Selector Tabs */}
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1.5">Pilih Mode Routing</label>
            <div className="grid grid-cols-3 gap-2 p-1 bg-surface-dark border border-border rounded-lg">
              <button
                type="button"
                onClick={() => setMode('model_only')}
                className={`py-2 px-2 text-center rounded-md text-xs font-semibold transition-all flex flex-col items-center gap-1 ${
                  mode === 'model_only' ? 'bg-accent text-white shadow' : 'text-text-muted hover:text-white'
                }`}
              >
                <Zap className="w-4 h-4" />
                <span>1. Model Only</span>
              </button>
              <button
                type="button"
                onClick={() => setMode('routing')}
                className={`py-2 px-2 text-center rounded-md text-xs font-semibold transition-all flex flex-col items-center gap-1 ${
                  mode === 'routing' ? 'bg-accent text-white shadow' : 'text-text-muted hover:text-white'
                }`}
              >
                <Shuffle className="w-4 h-4" />
                <span>2. Routing</span>
              </button>
              <button
                type="button"
                onClick={() => setMode('combo_routing')}
                className={`py-2 px-2 text-center rounded-md text-xs font-semibold transition-all flex flex-col items-center gap-1 ${
                  mode === 'combo_routing' ? 'bg-accent text-white shadow' : 'text-text-muted hover:text-white'
                }`}
              >
                <Layers className="w-4 h-4" />
                <span>3. Combo Routing</span>
              </button>
            </div>
          </div>

          <form onSubmit={handleCreate} className="space-y-4">
            {mode === 'model_only' && (
              <div className="p-3 bg-accent/10 border border-accent/20 rounded-lg text-accent text-[11px] leading-relaxed">
                ⚡ <strong>Mode Model Only:</strong> Passthrough langsung 1-to-1 ke model dan provider tertentu tanpa overhead failover. Latensi paling instan (&lt; 2ms saat cache hit).
              </div>
            )}

            {mode === 'routing' && (
              <div className="p-3 bg-purple-500/10 border border-purple-500/20 rounded-lg text-purple-300 text-[11px] leading-relaxed">
                🔀 <strong>Mode Routing:</strong> Redundansi satu model dengan failover multi-provider (Priority, Lowest Latency, Lowest Cost, Weighted, Round Robin).
              </div>
            )}

            {mode === 'combo_routing' && (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-lg text-emerald-300 text-[11px] leading-relaxed">
                🔗 <strong>Mode Combo Routing:</strong> Rantai bertingkat cerdas antar-model. Permintaan pertama dialokasikan ke <strong>Tier 1 (Lokal/Hemat)</strong>. Bila kuota habis (429) atau server down, otomatis dialihkan ke <strong>Tier 2 (Flagship Fallback)</strong>!
              </div>
            )}

            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Aturan</label>
              <input
                type="text"
                required
                placeholder={
                  mode === 'model_only'
                    ? 'direct-gpt4o'
                    : mode === 'combo_routing'
                    ? 'combo-local-to-flagship'
                    : 'gpt-failover-rule'
                }
                value={newRule.name}
                onChange={(e) => setNewRule({ ...newRule, name: e.target.value })}
                className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
              />
            </div>

            {mode === 'model_only' && (
              <>
                <Select
                  label="Target Model"
                  required
                  value={targetModelId}
                  onChange={(val) => {
                    setTargetModelId(val);
                    setTargetProviderId('');
                  }}
                  placeholder="-- Pilih Model --"
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
                  label="Target Provider"
                  value={targetProviderId}
                  onChange={(val) => setTargetProviderId(val)}
                  placeholder="-- Bawaan (Semua Provider Model) --"
                  options={[
                    { value: '', label: '-- Bawaan (Semua Provider Model) --' },
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
              </>
            )}

            {mode === 'routing' && (
              <>
                <Select
                  label="Target Model"
                  value={targetModelId}
                  onChange={(val) => setTargetModelId(val)}
                  placeholder="-- Semua Model --"
                  options={[
                    { value: '', label: '-- Semua Model --' },
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
              </>
            )}

            {mode === 'combo_routing' && (
              <>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block font-semibold text-text-secondary uppercase mb-1">
                      Tier 1 Model (Lokal / Hemat)
                    </label>
                    <input
                      type="text"
                      required
                      placeholder="qwen2.5-coder atau llama-3.2"
                      value={tier1ModelName}
                      onChange={(e) => setTier1ModelName(e.target.value)}
                      className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white font-mono"
                    />
                    <span className="text-[10px] text-text-muted mt-0.5 block">
                      Diprioritaskan pertama kali untuk menghemat biaya.
                    </span>
                  </div>
                  <div>
                    <label className="block font-semibold text-text-secondary uppercase mb-1">
                      Tier 2 Model (Flagship Fallback)
                    </label>
                    <input
                      type="text"
                      required
                      placeholder="gpt-4o atau claude-3-5-sonnet"
                      value={tier2ModelName}
                      onChange={(e) => setTier2ModelName(e.target.value)}
                      className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white font-mono"
                    />
                    <span className="text-[10px] text-text-muted mt-0.5 block">
                      Cadangan otomatis jika Tier 1 gagal atau rate limited.
                    </span>
                  </div>
                </div>
              </>
            )}

            <div className="grid grid-cols-2 gap-3 pt-1 border-t border-border/40">
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">Prioritas Evaluasi</label>
                <input
                  type="number"
                  value={newRule.priority}
                  onChange={(e) => setNewRule({ ...newRule, priority: parseInt(e.target.value) || 100 })}
                  className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
                />
              </div>
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">Ambang Circuit Breaker</label>
                <input
                  type="number"
                  value={newRule.failure_threshold}
                  onChange={(e) => setNewRule({ ...newRule, failure_threshold: parseInt(e.target.value) || 5 })}
                  className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
                />
              </div>
            </div>

            <Button type="submit" variant="primary" size="md" className="w-full mt-3">
              Simpan Aturan Routing ({mode === 'model_only' ? 'Model Only' : mode === 'combo_routing' ? 'Combo Routing' : 'Routing'})
            </Button>
          </form>
        </div>
      </Modal>
    
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
