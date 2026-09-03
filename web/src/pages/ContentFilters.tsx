import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ContentFilter } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { ShieldAlert, Plus, Trash2 } from 'lucide-react';

export const ContentFilters: React.FC = () => {
  const [filters, setFilters] = useState<ContentFilter[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newFilter, setNewFilter] = useState({
    name: '',
    kind: 'blocked_pattern',
    applies_to: 'request',
    action: 'block',
    pattern: '',
    pattern_type: 'substring',
    priority: 100,
  });

  const loadFilters = async () => {
    try {
      const res = await api.filters.list();
      setFilters(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadFilters();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.filters.create(newFilter);
      setIsCreateOpen(false);
      loadFilters();
    } catch (err) {
      alert('Gagal membuat filter: ' + err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Hapus penyaring konten ini?')) return;
    try {
      await api.filters.delete(id);
      loadFilters();
    } catch (err) {
      alert('Gagal menghapus: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Content Filters & Security</h2>
          <p className="text-xs text-text-secondary mt-1">
            Penyaringan konten masukan/keluaran berbasis pola teks, ekspresi reguler, atau batas ukuran payload.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Penyaring
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filters.map((f) => (
          <Card key={f.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-rose-500/10 border border-rose-500/20 text-rose-400 flex items-center justify-center">
                    <ShieldAlert className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{f.name}</h4>
                    <span className="text-[11px] text-accent font-mono">{f.kind}</span>
                  </div>
                </div>
                <Badge variant={f.action === 'block' ? 'error' : 'warn'}>
                  {f.action.toUpperCase()}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Target Alur</span>
                  <span className="font-semibold text-text-primary capitalize">{f.applies_to}</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Pola Pencocokan</span>
                  <span className="font-mono text-text-secondary truncate max-w-[170px]" title={f.pattern}>
                    {f.pattern || '-'}
                  </span>
                </div>
                <div className="flex justify-between py-1">
                  <span className="text-text-muted">Tipe Pola</span>
                  <span className="font-mono text-accent">{f.pattern_type || '-'}</span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex justify-end">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleDelete(f.id)}
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
        title="Buat Penyaring Konten Baru"
        subtitle="Cegah kebocoran data sensitif atau permintaan berisiko tinggi"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Aturan</label>
            <input
              type="text"
              required
              placeholder="block-secret-keys"
              value={newFilter.name}
              onChange={(e) => setNewFilter({ ...newFilter, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Jenis Penyaring</label>
              <select
                value={newFilter.kind}
                onChange={(e) => setNewFilter({ ...newFilter, kind: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              >
                <option value="blocked_pattern">Blocked Pattern</option>
                <option value="allowed_pattern">Allowed Pattern</option>
              </select>
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Aksi</label>
              <select
                value={newFilter.action}
                onChange={(e) => setNewFilter({ ...newFilter, action: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              >
                <option value="block">Blokir (400 Bad Request)</option>
                <option value="warn">Peringatkan Saja (Warn)</option>
              </select>
            </div>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Pola Teks / Kata Kunci</label>
            <input
              type="text"
              required
              placeholder="sk-proj-[a-zA-Z0-9]+"
              value={newFilter.pattern}
              onChange={(e) => setNewFilter({ ...newFilter, pattern: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Tipe Pola</label>
            <select
              value={newFilter.pattern_type}
              onChange={(e) => setNewFilter({ ...newFilter, pattern_type: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            >
              <option value="substring">Substring (Pencocokan Kata)</option>
              <option value="regex">Regex (Ekspresi Reguler)</option>
            </select>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Penyaring
          </Button>
        </form>
      </Modal>
    </div>
  );
};
