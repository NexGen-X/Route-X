import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { EgressPool } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Network, Plus, Trash2 } from 'lucide-react';

export const Egress: React.FC = () => {
  const [pools, setPools] = useState<EgressPool[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newPool, setNewPool] = useState({
    name: '',
    kind: 'http',
    proxy_url: '',
    weight: 100,
    region: 'ap-southeast-1',
  });

  const loadPools = async () => {
    try {
      const res = await api.egress.list();
      setPools(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadPools();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.egress.create(newPool);
      setIsCreateOpen(false);
      setNewPool({ name: '', kind: 'http', proxy_url: '', weight: 100, region: 'ap-southeast-1' });
      loadPools();
    } catch (err) {
      alert('Gagal membuat egress pool: ' + err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Hapus egress pool ini?')) return;
    try {
      await api.egress.delete(id);
      loadPools();
    } catch (err) {
      alert('Gagal menghapus: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Egress Proxy Pools</h2>
          <p className="text-xs text-text-secondary mt-1">
            Manajemen proxy keluar (HTTP/HTTPS/SOCKS5) untuk routing lalu lintas ke upstream provider.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Pool
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {pools.map((p) => (
          <Card key={p.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center">
                    <Network className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{p.name}</h4>
                    <span className="text-[11px] text-text-muted font-mono">{(p.kind || 'PROXY').toUpperCase()} • {p.region || 'global'}</span>
                  </div>
                </div>
                <Badge variant={p.enabled ? 'success' : 'neutral'}>
                  {p.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Bobot Alokasi</span>
                  <span className="font-mono text-white">{p.weight}</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Proxy URL</span>
                  <span className="font-mono text-accent">[ENCRYPTED AT REST]</span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex justify-end">
              <Button
                variant="danger"
                size="sm"
                onClick={() => handleDelete(p.id)}
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
        title="Tambah Egress Proxy Pool"
        subtitle="URL proxy akan dienkripsi secara aman dengan AES-256-GCM"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Pool</label>
            <input
              type="text"
              required
              placeholder="residential-sg-1"
              value={newPool.name}
              onChange={(e) => setNewPool({ ...newPool, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Protokol Proxy</label>
            <select
              value={newPool.kind}
              onChange={(e) => setNewPool({ ...newPool, kind: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            >
              <option value="http">HTTP Proxy</option>
              <option value="https">HTTPS Proxy</option>
              <option value="socks5">SOCKS5 Proxy</option>
            </select>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Proxy URL Lengkap</label>
            <input
              type="text"
              required
              placeholder="http://user:pass@proxy.example.com:8080"
              value={newPool.proxy_url}
              onChange={(e) => setNewPool({ ...newPool, proxy_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Wilayah (Region)</label>
            <input
              type="text"
              value={newPool.region}
              onChange={(e) => setNewPool({ ...newPool, region: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Egress Pool
          </Button>
        </form>
      </Modal>
    </div>
  );
};
