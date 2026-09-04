import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RoutingRule, CircuitBreakerStatus } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { GitFork, Plus, Trash2, ZapOff, RotateCcw, RefreshCw } from 'lucide-react';

export const RoutingRules: React.FC = () => {
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [breakers, setBreakers] = useState<CircuitBreakerStatus[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isBreakersLoading, setIsBreakersLoading] = useState(false);
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
      const res = await api.routing.list();
      setRules(res.items || []);
    } catch (err) {
      console.error(err);
    }
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
      await api.routing.create({
        ...newRule,
        match_capabilities: ['chat'],
      });
      setIsCreateOpen(false);
      loadRules();
    } catch (err) {
      alert('Gagal membuat aturan: ' + err);
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
        {rules.map((r) => (
          <Card key={r.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-accent/10 border border-accent/20 text-accent flex items-center justify-center">
                    <GitFork className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{r.name}</h4>
                    <span className="text-[11px] text-accent font-mono">Prioritas: {r.priority}</span>
                  </div>
                </div>
                <Badge variant={r.enabled ? 'success' : 'neutral'}>
                  {r.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Strategi</span>
                  <span className="font-semibold text-accent uppercase font-mono">{r.strategy}</span>
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
        ))}
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
        subtitle="Tentukan strategi alokasi provider dan parameter ketahanan"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Aturan</label>
            <input
              type="text"
              required
              placeholder="gpt-priority-failover"
              value={newRule.name}
              onChange={(e) => setNewRule({ ...newRule, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Strategi Routing</label>
            <select
              value={newRule.strategy}
              onChange={(e) => setNewRule({ ...newRule, strategy: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
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
              <label className="block font-semibold text-text-secondary uppercase mb-1">Prioritas Evaluasi</label>
              <input
                type="number"
                value={newRule.priority}
                onChange={(e) => setNewRule({ ...newRule, priority: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Maksimal Percobaan</label>
              <input
                type="number"
                value={newRule.max_attempts}
                onChange={(e) => setNewRule({ ...newRule, max_attempts: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Ambang Kegagalan Breaker</label>
              <input
                type="number"
                value={newRule.failure_threshold}
                onChange={(e) => setNewRule({ ...newRule, failure_threshold: parseInt(e.target.value) || 5 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Durasi Buka Sirkuit (ms)</label>
              <input
                type="number"
                value={newRule.open_duration_ms}
                onChange={(e) => setNewRule({ ...newRule, open_duration_ms: parseInt(e.target.value) || 30000 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Aturan
          </Button>
        </form>
      </Modal>
    </div>
  );
};
