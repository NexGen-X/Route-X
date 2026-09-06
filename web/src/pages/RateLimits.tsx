import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RateLimit } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Select } from '../components/common/Select';
import { Gauge, Plus, Trash2 } from 'lucide-react';
import { useToast } from '../context/ToastContext';

export const RateLimits: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [limits, setLimits] = useState<RateLimit[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newLimit, setNewLimit] = useState({
    scope: 'api_key',
    scope_id: 'default',
    requests_per_minute: 60,
    tokens_per_minute: 100000,
    requests_per_second: 10,
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
      toast.success('Aturan batas laju berhasil dibuat');
      loadLimits();
    } catch (err) {
      toast.error('Gagal membuat limit: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDelete = async (id: string) => {
    const confirmed = await confirmModal({
      title: 'Hapus Aturan Rate Limit',
      message: 'Apakah Anda yakin ingin menghapus aturan pembatasan laju ini? Kebijakan fallback default akan berlaku.',
      danger: true,
      confirmText: 'Ya, Hapus Aturan',
    });
    if (!confirmed) return;

    try {
      await api.rateLimits.delete(id);
      toast.success('Aturan batas laju berhasil dihapus');
      loadLimits();
    } catch (err) {
      toast.error('Gagal menghapus: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Rate Limits</h2>
          <p className="text-xs text-text-secondary mt-1">
            Pembatasan laju permintaan terdistribusi via Redis (RPM, TPM, RPS) per IP, API key, atau global.
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

      {limits.length === 0 ? (
        <Card className="py-12 px-6 text-center">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center mx-auto shadow-inner">
              <Gauge className="w-6 h-6" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Belum Ada Aturan Rate Limit</h3>
              <p className="text-xs text-text-muted mt-1 leading-relaxed">
                Tetapkan batasan laju kuota terdistribusi berbasis Redis (RPM, TPM, atau RPS) per Kunci API, Alamat IP Klien, atau Model untuk mencegah kelebihan beban.
              </p>
            </div>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4 text-black" />}
            >
              Tambah Rate Limit Pertama
            </Button>
          </div>
        </Card>
      ) : (
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
                    <h4 className="text-sm font-bold text-white uppercase">{l.scope}</h4>
                    <span className="text-[11px] text-text-muted font-mono">{l.scope_id || 'Semua'}</span>
                  </div>
                </div>
                <Badge variant={l.enabled ? 'success' : 'neutral'}>
                  {l.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Requests Per Minute (RPM)</span>
                  <span className="font-mono text-white font-semibold">
                    {l.requests_per_minute ? `${l.requests_per_minute.toLocaleString()} req/m` : '-'}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Tokens Per Minute (TPM)</span>
                  <span className="font-mono text-accent">
                    {l.tokens_per_minute ? `${l.tokens_per_minute.toLocaleString()} tok/m` : '-'}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Requests Per Second (RPS)</span>
                  <span className="font-mono text-text-secondary">
                    {l.requests_per_second ? `${l.requests_per_second} req/s` : '-'}
                  </span>
                </div>
                <div className="flex justify-between py-1">
                  <span className="text-text-muted">Batas Harian</span>
                  <span className="font-mono text-text-secondary">
                    {l.daily_request_limit ? `${l.daily_request_limit.toLocaleString()} req/hari` : '-'}
                  </span>
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
      )}

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Konfigurasi Rate Limit Baru"
        subtitle="Terapkan pembatasan kuota laju pada tingkat Redis terdistribusi"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Select
              label="Cakupan (Scope)"
              value={newLimit.scope}
              onChange={(val) => setNewLimit({ ...newLimit, scope: val })}
              options={[
                { value: 'api_key', label: 'Per Kunci API', description: 'Limit berbasis token API klien individual' },
                { value: 'ip', label: 'Per Alamat IP Klien', description: 'Limit berbasis IP publik pemanggil' },
                { value: 'global', label: 'Global Gateway', description: 'Limit total seluruh gateway' },
              ]}
            />
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Scope ID / Target</label>
              <input
                type="text"
                required
                placeholder="ID atau IP (default: default)"
                value={newLimit.scope_id}
                onChange={(e) => setNewLimit({ ...newLimit, scope_id: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Req / Menit (RPM)</label>
              <input
                type="number"
                value={newLimit.requests_per_minute}
                onChange={(e) => setNewLimit({ ...newLimit, requests_per_minute: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Token / Menit (TPM)</label>
              <input
                type="number"
                value={newLimit.tokens_per_minute}
                onChange={(e) => setNewLimit({ ...newLimit, tokens_per_minute: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Req / Detik (RPS)</label>
              <input
                type="number"
                value={newLimit.requests_per_second}
                onChange={(e) => setNewLimit({ ...newLimit, requests_per_second: parseInt(e.target.value) || 0 })}
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
