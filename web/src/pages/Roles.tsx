import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Role } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { PageHeader } from '../components/common/PageHeader';
import { ShieldCheck, Plus, Trash2, Save } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

interface PermItem { key: string; description: string }

export const RolesPage: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [roles, setRoles] = useState<Role[]>([]);
  const [perms, setPerms] = useState<PermItem[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newRole, setNewRole] = useState({ name: '', description: '', rank: 100 });
  const [editRoleId, setEditRoleId] = useState<string | null>(null);
  const [editPerms, setEditPerms] = useState<string[]>([]);
  const [editLoading, setEditLoading] = useState(false);

  const loadAll = async () => {
    try {
      const [rRes, pRes] = await Promise.all([api.roles.list(), api.roles.permissions()]);
      setRoles(rRes.items || []);
      setPerms(pRes.items || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    }
  };

  useEffect(() => {
    void loadAll();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.roles.create({
        name: newRole.name.trim(),
        description: newRole.description.trim() || undefined,
        rank: newRole.rank,
      });
      setIsCreateOpen(false);
      setNewRole({ name: '', description: '', rank: 100 });
      toast.success('Peran kustom dibuat. Atur izinnya lewat tombol Izin.');
      void loadAll();
    } catch (err) {
      toast.error('Gagal membuat peran: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const openEdit = async (roleId: string) => {
    setEditRoleId(roleId);
    setEditPerms([]);
    setEditLoading(true);
    try {
      const detail = await api.roles.get(roleId);
      const keys = (detail.permissions || []).map((p: any) => (typeof p === 'string' ? p : p.key));
      setEditPerms(keys);
    } catch (err) {
      toast.error('Gagal memuat izin peran: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setEditLoading(false);
    }
  };

  const togglePerm = (key: string) => {
    setEditPerms((prev) => (prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]));
  };

  const handleSavePerms = async () => {
    if (!editRoleId) return;
    try {
      await api.roles.setPermissions(editRoleId, editPerms);
      toast.success('Izin peran disimpan.');
      setEditRoleId(null);
      void loadAll();
    } catch (err) {
      toast.error('Gagal menyimpan izin: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDelete = async (r: Role) => {
    const ok = await confirmModal({
      title: 'Hapus Peran?',
      message: `Hapus peran kustom "${r.name}"? SEMUA pengguna pemegang peran ini akan langsung kehilangan izinnya. Peran sistem tidak bisa dihapus.`,
      confirmText: 'Ya, Hapus',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.roles.delete(r.id);
      toast.success(`Peran ${r.name} dihapus.`);
      void loadAll();
    } catch (err) {
      toast.error('Gagal menghapus: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const editRole = roles.find((r) => r.id === editRoleId) || null;

  return (
    <div className="space-y-6">
      <PageHeader
        title={<span className="flex items-center gap-2"><ShieldCheck className="w-6 h-6 text-accent" /> Peran & Izin</span>}
        description="Peran kustom, matriks izin, dan rank kewenangan. Rank kecil berarti berkuasa. Peran sistem tidak bisa dihapus."
        actions={
          <Button variant="primary" size="sm" onClick={() => setIsCreateOpen(true)} icon={<Plus className="w-4 h-4" />} className="w-full sm:w-auto justify-center">
            Buat Peran
          </Button>
        }
      />

      {loadError && <QueryError message={loadError} onRetry={() => void loadAll()} />}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {roles.map((r) => (
          <Card key={r.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between gap-2">
                <h4 className="text-sm font-bold text-white">{r.name}</h4>
                <div className="flex gap-1 shrink-0">
                  {r.is_system && <Badge variant="info">sistem</Badge>}
                  <Badge variant="neutral">rank {r.rank}</Badge>
                </div>
              </div>
              <p className="mt-1 text-[11px] text-text-muted">{r.description || 'Tanpa deskripsi.'}</p>
            </div>
            <div className="mt-4 pt-3 border-t border-border flex gap-2">
              <Button variant="secondary" size="sm" onClick={() => void openEdit(r.id)}>
                Izin
              </Button>
              {!r.is_system && (
                <Button variant="secondary" size="sm" onClick={() => void handleDelete(r)} icon={<Trash2 className="w-3.5 h-3.5" />}>
                  Hapus
                </Button>
              )}
            </div>
          </Card>
        ))}
      </div>

      <Modal isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} title="Buat Peran Kustom" subtitle="Rank 0 paling berkuasa. Pakai 100 ke atas untuk peran biasa.">
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Peran</label>
            <input type="text" required value={newRole.name} onChange={(e) => setNewRole({ ...newRole, name: e.target.value })} placeholder="Operator" className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white" />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Deskripsi</label>
            <input type="text" value={newRole.description} onChange={(e) => setNewRole({ ...newRole, description: e.target.value })} placeholder="Boleh baca semua, tulis terbatas" className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white" />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Rank</label>
            <input type="number" min={0} value={newRole.rank} onChange={(e) => {
              const v = parseInt(e.target.value, 10);
              setNewRole({ ...newRole, rank: Number.isNaN(v) ? 100 : v });
            }} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono" />
          </div>
          <div className="pt-3 flex justify-end gap-2 border-t border-border">
            <Button type="button" variant="secondary" onClick={() => setIsCreateOpen(false)}>Batal</Button>
            <Button type="submit" variant="primary">Buat Peran</Button>
          </div>
        </form>
      </Modal>

      <Modal isOpen={editRoleId !== null} onClose={() => setEditRoleId(null)} title={`Izin: ${editRole?.name || ''}`} subtitle="Centang izin yang dipegang peran ini" maxWidth="lg">
        {editLoading ? (
          <p className="text-xs text-text-muted py-6 text-center">Memuat izin...</p>
        ) : (
          <div className="space-y-2 text-xs max-h-96 overflow-y-auto">
            {perms.map((p) => (
              <label key={p.key} className="flex items-start gap-3 px-3 py-2 bg-bg-surface-2 border border-border rounded-nav cursor-pointer hover:border-accent/40">
                <input type="checkbox" checked={editPerms.includes(p.key)} onChange={() => togglePerm(p.key)} className="mt-0.5" />
                <span>
                  <span className="block font-mono text-white">{p.key}</span>
                  <span className="block text-[11px] text-text-muted">{p.description}</span>
                </span>
              </label>
            ))}
            {perms.length === 0 && <p className="text-text-muted">Katalog izin kosong.</p>}
            <div className="pt-3 flex justify-end gap-2 border-t border-border sticky bottom-0 bg-bg-surface py-2">
              <Button variant="secondary" onClick={() => setEditRoleId(null)}>Batal</Button>
              <Button variant="primary" onClick={() => void handleSavePerms()} icon={<Save className="w-3.5 h-3.5" />}>
                Simpan Izin ({editPerms.length})
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
};
