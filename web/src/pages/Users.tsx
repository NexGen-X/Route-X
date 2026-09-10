import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { User, Role } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Select } from '../components/common/Select';
import { Users, Plus, Trash2, KeyRound, LogOut, Pencil, UserCheck } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export const UsersPage: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [users, setUsers] = useState<User[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [detailUserId, setDetailUserId] = useState<string | null>(null);
  const [detail, setDetail] = useState<{ roles: Role[]; permissions: string[] } | null>(null);
  const [newUser, setNewUser] = useState({ email: '', name: '', password: '', role_ids: [] as string[] });
  const [resetPw, setResetPw] = useState<{ userId: string; temp: string } | null>(null);

  const loadAll = async () => {
    try {
      const [uRes, rRes] = await Promise.all([api.users.list(), api.roles.list()]);
      setUsers(uRes.items || []);
      setRoles(rRes.items || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    }
  };

  useEffect(() => {
    void loadAll();
  }, []);

  const openDetail = async (id: string) => {
    setDetailUserId(id);
    setDetail(null);
    try {
      const res = await api.users.get(id);
      setDetail({ roles: res.roles || [], permissions: res.permissions || [] });
    } catch (err) {
      toast.error('Gagal memuat detail pengguna: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.users.create({
        email: newUser.email.trim(),
        name: newUser.name.trim(),
        password: newUser.password,
        role_ids: newUser.role_ids,
        must_change_password: true,
      });
      setIsCreateOpen(false);
      setNewUser({ email: '', name: '', password: '', role_ids: [] });
      toast.success('Pengguna baru berhasil dibuat. Ia wajib ganti password saat login pertama.');
      void loadAll();
    } catch (err) {
      toast.error('Gagal membuat pengguna: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleStatus = async (u: User, status: 'active' | 'disabled' | 'locked') => {
    const ok = await confirmModal({
      title: status === 'active' ? 'Aktifkan Pengguna?' : 'Nonaktifkan Pengguna?',
      message: `${u.email} akan diubah statusnya menjadi ${status}.`,
      confirmText: 'Ya, Lanjutkan',
      danger: status !== 'active',
    });
    if (!ok) return;
    try {
      await api.users.update(u.id, { status });
      toast.success(`Status ${u.email} menjadi ${status}.`);
      void loadAll();
    } catch (err) {
      toast.error('Gagal mengubah status: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDelete = async (u: User) => {
    const ok = await confirmModal({
      title: 'Hapus Pengguna?',
      message: `Hapus ${u.email} secara permanen? Akun sendiri dan pemegang terakhir Super Admin dilindungi backend.`,
      confirmText: 'Ya, Hapus',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.users.delete(u.id);
      toast.success(`${u.email} dihapus.`);
      void loadAll();
    } catch (err) {
      toast.error('Gagal menghapus: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleGrant = async (userId: string, roleId: string) => {
    if (!roleId) return;
    try {
      await api.users.grantRole(userId, roleId);
      toast.success('Peran diberikan.');
      const res = await api.users.get(userId);
      setDetail({ roles: res.roles || [], permissions: res.permissions || [] });
      void loadAll();
    } catch (err) {
      toast.error('Gagal memberi peran: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleRevokeRole = async (userId: string, roleId: string, roleName: string) => {
    const ok = await confirmModal({
      title: 'Cabut Peran?',
      message: `Cabut peran ${roleName} dari pengguna ini?`,
      confirmText: 'Ya, Cabut',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.users.revokeRole(userId, roleId);
      toast.success('Peran dicabut.');
      const res = await api.users.get(userId);
      setDetail({ roles: res.roles || [], permissions: res.permissions || [] });
      void loadAll();
    } catch (err) {
      toast.error('Gagal mencabut peran: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleForceReset = async (u: User) => {
    if (!resetPw || resetPw.userId !== u.id || !resetPw.temp.trim()) {
      toast.error('Isi password sementara dulu.');
      return;
    }
    const ok = await confirmModal({
      title: 'Reset Password Paksa?',
      message: `Password ${u.email} diganti dan seluruh sesinya dicabut.`,
      confirmText: 'Ya, Reset',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.users.forceResetPassword(u.id, resetPw.temp);
      toast.success('Password direset, sesi dicabut. Sampaikan password sementara lewat jalur aman.');
      setResetPw(null);
    } catch (err) {
      toast.error('Gagal reset: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleRevokeSessions = async (u: User) => {
    const ok = await confirmModal({
      title: 'Cabut Semua Sesi?',
      message: `Seluruh sesi login ${u.email} dicabut, termasuk milik Anda bila itu akun Anda.`,
      confirmText: 'Ya, Cabut',
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await api.users.revokeSessions(u.id);
      toast.success(`${res.count} sesi dicabut.`);
    } catch (err) {
      toast.error('Gagal mencabut sesi: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const statusVariant = (s: string) => (s === 'active' ? 'success' : s === 'locked' ? 'error' : 'neutral');

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white flex items-center gap-2">
            <Users className="w-6 h-6 text-accent" /> Pengguna Admin
          </h2>
          <p className="text-xs text-text-secondary mt-1">
            Akun konsol, peran, status, reset password paksa, dan pencabutan sesi. Akun sendiri dan pemegang
            terakhir Super Admin dilindungi backend.
          </p>
        </div>
        <Button variant="primary" size="sm" onClick={() => setIsCreateOpen(true)} icon={<Plus className="w-4 h-4" />}>
          Tambah Pengguna
        </Button>
      </div>

      {loadError && <QueryError message={loadError} onRetry={() => void loadAll()} />}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {users.map((u) => (
          <Card key={u.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <h4 className="text-sm font-bold text-white truncate">{u.display_name}</h4>
                  <span className="text-[11px] text-text-muted font-mono truncate block">{u.email}</span>
                </div>
                <Badge variant={statusVariant(u.status)}>{u.status}</Badge>
              </div>
              <div className="mt-3 flex flex-wrap gap-1">
                {(u.roles || []).map((r) => (
                  <span key={r} className="px-2 py-0.5 text-[10px] font-mono rounded bg-bg-surface-2 text-text-secondary">
                    {r}
                  </span>
                ))}
                {(!u.roles || u.roles.length === 0) && (
                  <span className="text-[10px] text-text-muted">tanpa peran</span>
                )}
              </div>
              {u.must_change_password && (
                <p className="mt-2 text-[11px] text-amber-300">Wajib ganti password saat login berikutnya.</p>
              )}
            </div>
            <div className="mt-4 pt-3 border-t border-border flex flex-wrap gap-2">
              <Button variant="secondary" size="sm" onClick={() => void openDetail(u.id)} icon={<UserCheck className="w-3.5 h-3.5" />}>
                Kelola
              </Button>
              {u.status === 'active' ? (
                <Button variant="secondary" size="sm" onClick={() => void handleStatus(u, 'disabled')}>
                  Nonaktifkan
                </Button>
              ) : (
                <Button variant="secondary" size="sm" onClick={() => void handleStatus(u, 'active')}>
                  Aktifkan
                </Button>
              )}
              <Button variant="secondary" size="sm" onClick={() => void handleDelete(u)} icon={<Trash2 className="w-3.5 h-3.5" />}>
                Hapus
              </Button>
            </div>
          </Card>
        ))}
      </div>

      <Modal isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} title="Tambah Pengguna" subtitle="Akun baru wajib ganti password saat login pertama">
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Email</label>
            <input type="email" required value={newUser.email} onChange={(e) => setNewUser({ ...newUser, email: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white" />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Tampilan</label>
            <input type="text" required value={newUser.name} onChange={(e) => setNewUser({ ...newUser, name: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white" />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Password Awal</label>
            <input type="password" required value={newUser.password} onChange={(e) => setNewUser({ ...newUser, password: e.target.value })} className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono" />
          </div>
          <Select
            label="Peran Awal"
            value={newUser.role_ids[0] || ''}
            onChange={(val) => setNewUser({ ...newUser, role_ids: val ? [val] : [] })}
            options={roles.map((r) => ({ value: r.id, label: r.name, description: `rank ${r.rank}${r.is_system ? ' · sistem' : ''}` }))}
          />
          <div className="pt-3 flex justify-end gap-2 border-t border-border">
            <Button type="button" variant="secondary" onClick={() => setIsCreateOpen(false)}>Batal</Button>
            <Button type="submit" variant="primary">Simpan Pengguna</Button>
          </div>
        </form>
      </Modal>

      <Modal isOpen={detailUserId !== null} onClose={() => { setDetailUserId(null); setDetail(null); }} title="Kelola Pengguna" subtitle="Peran, password, dan sesi" maxWidth="lg">
        {!detail ? (
          <p className="text-xs text-text-muted py-6 text-center">Memuat detail...</p>
        ) : (
          <div className="space-y-5 text-xs">
            <div>
              <span className="block font-semibold text-text-secondary uppercase mb-2">Peran Dimiliki</span>
              <div className="space-y-2">
                {detail.roles.map((r) => (
                  <div key={r.id} className="flex items-center justify-between px-3 py-2 bg-bg-surface-2 border border-border rounded-nav">
                    <span className="text-white font-semibold">{r.name}</span>
                    <Button variant="secondary" size="sm" onClick={() => void handleRevokeRole(detailUserId as string, r.id, r.name)}>
                      Cabut
                    </Button>
                  </div>
                ))}
                {detail.roles.length === 0 && <p className="text-text-muted">Belum memegang peran.</p>}
              </div>
              <div className="mt-3 flex gap-2">
                <Select
                  label="Beri Peran"
                  value=""
                  onChange={(val) => void handleGrant(detailUserId as string, val)}
                  options={roles.filter((r) => !detail.roles.some((dr) => dr.id === r.id)).map((r) => ({ value: r.id, label: r.name, description: `rank ${r.rank}` }))}
                />
              </div>
            </div>
            <div className="pt-3 border-t border-border">
              <span className="block font-semibold text-text-secondary uppercase mb-2">Reset Password Paksa</span>
              <div className="flex gap-2">
                <input type="password" placeholder="Password sementara" value={resetPw?.userId === detailUserId ? resetPw.temp : ''} onChange={(e) => setResetPw({ userId: detailUserId as string, temp: e.target.value })} className="flex-1 px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono" />
                <Button variant="secondary" onClick={() => { const u = users.find((x) => x.id === detailUserId); if (u) void handleForceReset(u); }} icon={<KeyRound className="w-3.5 h-3.5" />}>
                  Reset
                </Button>
              </div>
              <p className="mt-1 text-[11px] text-text-muted">Mengirim password baru dan mencabut seluruh sesi pengguna.</p>
            </div>
            <div className="pt-3 border-t border-border flex items-center justify-between">
              <span className="text-text-secondary">Cabut seluruh sesi login pengguna ini.</span>
              <Button variant="secondary" onClick={() => { const u = users.find((x) => x.id === detailUserId); if (u) void handleRevokeSessions(u); }} icon={<LogOut className="w-3.5 h-3.5" />}>
                Cabut Sesi
              </Button>
            </div>
            <div className="pt-3 border-t border-border">
              <span className="block font-semibold text-text-secondary uppercase mb-1">Izin Efektif ({detail.permissions.length})</span>
              <p className="font-mono text-[11px] text-text-secondary break-all">{detail.permissions.join(', ') || '-'}</p>
            </div>
            <div className="flex items-center gap-2 text-text-muted">
              <Pencil className="w-3.5 h-3.5" />
              <span>Ubah nama tampilan pengguna:</span>
            </div>
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="Nama tampilan baru"
                id="rename-user-input"
                className="flex-1 px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
              <Button
                variant="secondary"
                onClick={() => {
                  const el = document.getElementById('rename-user-input') as HTMLInputElement | null;
                  const name = el?.value.trim() || '';
                  if (!name || !detailUserId) {
                    toast.error('Isi nama tampilan baru dulu.');
                    return;
                  }
                  void api.users.update(detailUserId, { name }).then(() => {
                    toast.success('Nama tampilan diperbarui.');
                    void loadAll();
                  }).catch((err) => {
                    toast.error('Gagal mengubah nama: ' + (err instanceof Error ? err.message : String(err)));
                  });
                }}
              >
                Simpan Nama
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
};
