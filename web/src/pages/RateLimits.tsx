import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RateLimit } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Gauge, Plus, Trash2 } from 'lucide-react';

export const RateLimits: React.FC = () => {
  const [limits, setLimits] = useState<RateLimit[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newLimit, setNewLimit] = useState({
    name: '',
    scope: 'api_key',
    limit_type: 'requests',
    max_value: 60,
    window_seconds: 60,
  });

  const loadLimits = async () => {
    try {
      const res = await api.rateLimits.list();
      setLimits(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadLimits();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.rateLimits.create(newLimit);
      setIsCreateOpen(false);
      loadLimits();
    } catch (err) {
      alert('Gagal membuat limit: ' + err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Hapus aturan batas laju ini?')) return;
    try {
      await api.rateLimits.delete(id);
      loadLimits();
    } catch (err) {
      alert('Gagal menghapus: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Rate Limits</h2>
          <p className="text-xs text-text-secondary mt-1">
            Pembatasan laju permintaan terdistribusi via Redis (RPM, TPM, concurrent requests) per IP, API key, atau tier.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Rate Limit
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {limits.map((l) => (
          <Card key={l.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center">
                    <Gauge className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{l.name}</h4>
                    <span className="text-[11px] text-text-muted font-mono">{l.scope}</span>
                  </div>
                </div>
                <Badge variant={l.enabled ? 'success' : 'neutral'}>
                  {l.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Batas Maksimal</span>
                  <span className="font-mono text-white font-semibold">
                    {l.max_value.toLocaleString()} {l.limit_type}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Jendela Waktu</span>
                  <span className="font-mono text-accent">{l.window_seconds} detik</span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex justify-end">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleDelete(l.id)}
                icon={<Trash2 className="w-3.5 h-3.5" />}
              >
                Hapus
              </Button>
            </div>
          </Card>
        ))}
      </div>

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Konfigurasi Rate Limit Baru"
        subtitle="Terapkan pembatasan kuota laju pada tingkat Redis terdistribusi"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Aturan</label>
            <input
              type="text"
              required
              placeholder="Standard Key 60 RPM"
              value={newLimit.name}
              onChange={(e) => setNewLimit({ ...newLimit, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Cakupan (Scope)</label>
              <select
                value={newLimit.scope}
                onChange={(e) => setNewLimit({ ...newLimit, scope: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              >
                <option value="api_key">Per Kunci API</option>
                <option value="ip">Per Alamat IP Klien</option>
                <option value="model">Per Model Inferensi</option>
              </select>
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Tipe Batasan</label>
              <select
                value={newLimit.limit_type}
                onChange={(e) => setNewLimit({ ...newLimit, limit_type: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              >
                <option value="requests">Jumlah Requests</option>
                <option value="tokens">Jumlah Tokens</option>
              </select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Batas Kuota</label>
              <input
                type="number"
                required
                value={newLimit.max_value}
                onChange={(e) => setNewLimit({ ...newLimit, max_value: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Jendela Detik</label>
              <input
                type="number"
                required
                value={newLimit.window_seconds}
                onChange={(e) => setNewLimit({ ...newLimit, window_seconds: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Rate Limit
          </Button>
        </form>
      </Modal>
    </div>
  );
};
