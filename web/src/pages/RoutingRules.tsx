import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RoutingRule, CircuitBreakerStatus, Model, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { GitFork, Plus, Trash2, ZapOff, RotateCcw, RefreshCw, Zap, Shuffle, Layers } from 'lucide-react';

export const RoutingRules: React.FC = () => {
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [breakers, setBreakers] = useState<CircuitBreakerStatus[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isBreakersLoading, setIsBreakersLoading] = useState(false);

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
        api.models.list().catch(() => ({ items: [] })),
        api.providers.list().catch(() => ({ items: [] })),
      ]);
      setRules(resRules.items || []);
      setModels(resModels.items || []);
      setProviders(resProv.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  const getRuleMode = (r: RoutingRule): { mode: string; label: string; badgeVariant: 'info' | 'lime' | 'warn'; detail: string } => {
    const desc = r.description || '';
    if (desc.includes('[combo:tier2=') || desc.toLowerCase().includes('combo')) {
      const match = desc.match(/\[combo:tier2=([^\]]+)\]/);
      const tier2 = match ? match[1] : 'Flagship';
      return { mode: 'combo_routing', label: 'COMBO ROUTING', badgeVariant: 'lime', detail: `Tier 2 Fallback: ${tier2}` };
    }
    if (r.max_attempts === 1 || desc.includes('[model_only]') || desc.toLowerCase().includes('model only')) {
      return { mode: 'model_only', label: 'MODEL ONLY', badgeVariant: 'info', detail: 'Direct 1:1 Passthrough' };
    }
    return { mode: 'routing', label: 'ROUTING', badgeVariant: 'warn', detail: `${r.max_attempts} percobaan failover` };
  };

  const loadBreakers = async () => {
    setIsBreakersLoading(true);
    try {
      const res = await api.breakers.list();
      setBreakers(res.items || []);
    } catch (err) {
      console.error(err);
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
      loadBreakers();
    } catch (err) {
      alert('Gagal mereset circuit breaker: ' + err);
    }
  };

  const handleToggle = async (r: RoutingRule) => {
    try {
      await api.routing.toggle(r.id, !r.enabled);
      loadRules();
    } catch (err) {
      alert('Gagal toggle aturan: ' + err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Hapus aturan perutean ini?')) return;
    try {
      await api.routing.delete(id);
      loadRules();
    } catch (err) {
      alert('Gagal menghapus: ' + err);
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
        const selModel = models.find((m) => m.id === targetModelId);
        const selProv = providers.find((p) => p.id === targetProviderId);
        const modelName = selModel?.model_id || targetModelId;
        payload = {
          ...payload,
          name: newRule.name || `direct-${modelName.replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: 'priority',
          max_attempts: 1,
          match_model_id: targetModelId || undefined,
          provider_ids: targetProviderId ? [targetProviderId] : undefined,
          description: `[model_only] Direct 1:1 passthrough ke model ${modelName}${selProv ? ` via ${selProv.name}` : ''}`,
        };
      } else if (mode === 'combo_routing') {
        const tier1M = models.find((m) => m.model_id === tier1ModelName || m.display_name === tier1ModelName);
        payload = {
          ...payload,
          name: newRule.name || `combo-${tier1ModelName.replace(/[^a-zA-Z0-9_-]/g, '-')}-to-${tier2ModelName.replace(/[^a-zA-Z0-9_-]/g, '-')}`,
          strategy: 'priority',
          max_attempts: 2,
          match_model_id: tier1M?.id || undefined,
          description: `[combo:tier2=${tier2ModelName}] Smart Tiered Cascade: Tier 1 (${tier1ModelName}) -> Tier 2 (${tier2ModelName})`,
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
      loadRules();
    } catch (err: any) {
      alert('Gagal membuat aturan: ' + (err.message || err));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Routing Rules Engine</h2>
          <p className="text-xs text-text-secondary mt-1">
            Strategi pemilihan upstream (Priority, Lowest Cost, Lowest Latency, Weighted, Round-Robin) dan failover otomatis.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Aturan
        </Button>
      </div>

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
                </div>
              </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
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
                <div>
                  <label className="block font-semibold text-text-secondary uppercase mb-1">Target Model</label>
                  <select
                    value={targetModelId}
                    onChange={(e) => setTargetModelId(e.target.value)}
                    required
                    className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
                  >
                    <option value="">-- Pilih Model --</option>
                    {models.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.display_name} ({m.model_id})
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block font-semibold text-text-secondary uppercase mb-1">Target Provider</label>
                  <select
                    value={targetProviderId}
                    onChange={(e) => setTargetProviderId(e.target.value)}
                    className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
                  >
                    <option value="">-- Bawaan (Semua Provider Model) --</option>
                    {providers.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.display_name || p.name} ({p.kind})
                      </option>
                    ))}
                  </select>
                </div>
              </>
            )}

            {mode === 'routing' && (
              <>
                <div>
                  <label className="block font-semibold text-text-secondary uppercase mb-1">Target Model</label>
                  <select
                    value={targetModelId}
                    onChange={(e) => setTargetModelId(e.target.value)}
                    className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
                  >
                    <option value="">-- Semua Model --</option>
                    {models.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.display_name} ({m.model_id})
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block font-semibold text-text-secondary uppercase mb-1">Strategi Routing</label>
                  <select
                    value={newRule.strategy}
                    onChange={(e) => setNewRule({ ...newRule, strategy: e.target.value })}
                    className="w-full px-3 py-2 bg-surface-dark border border-border rounded-nav text-white"
                  >
                    <option value="priority">Priority (Urutan Tertinggi)</option>
                    <option value="lowest_cost">Lowest Cost (Biaya Termurah)</option>
                    <option value="lowest_latency">Lowest Latency (Latensi Terendah)</option>
                    <option value="weighted">Weighted (Bobot Proporsional)</option>
                    <option value="round_robin">Round Robin (Beban Berimbang)</option>
                  </select>
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
    </div>
  );
};
