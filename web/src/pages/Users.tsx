import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { User } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Users, Plus, Trash2, KeyRound, LogOut, Pencil, UserCheck, Eye, EyeOff, Shield } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export const UsersPage: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [users, setUsers] = useState<User[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [detailUserId, setDetailUserId] = useState<string | null>(null);
  const [detail, setDetail] = useState<{ roles: string[]; permissions: string[] } | null>(null);
  const [newUser, setNewUser] = useState({ email: '', name: '', password: '' });
  const [resetPw, setResetPw] = useState<{ userId: string; temp: string } | null>(null);
  const [renameName, setRenameName] = useState('');
  const [showNewUserPassword, setShowNewUserPassword] = useState(false);
  const [showResetPassword, setShowResetPassword] = useState(false);

  const loadAll = async () => {
    setIsLoading(true);
    try {
      const uRes = await api.users.list();
      setUsers(uRes.items || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadAll();
  }, []);

  const openDetail = async (id: string) => {
    setDetailUserId(id);
    setDetail(null);
    const u = users.find((x) => x.id === id);
    if (u) setRenameName(u.display_name || '');
    try {
      const res = await api.users.get(id);
      setDetail({ roles: res.roles || ['Admin'], permissions: res.permissions || ['*'] });
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
        must_change_password: true,
      });
      setIsCreateOpen(false);
      setNewUser({ email: '', name: '', password: '' });
      toast.success('Pengguna admin baru berhasil dibuat. Ia wajib ganti password saat login pertama.');
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
      title: 'Hapus Pengguna Admin?',
      message: `Hapus ${u.email} secara permanen? Akun sendiri dan admin aktif terakhir dilindungi backend.`,
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

  const handleForceReset = async (u: User) => {
    if (!resetPw || resetPw.userId !== u.id || !resetPw.temp.trim()) {
      toast.error('Kata sandi sementara wajib diisi.');
      return;
    }
    if (resetPw.temp.trim().length < 8) {
      toast.error('Password sementara minimal 8 karakter.');
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
      <PageHeader
        title={<span className="flex items-center gap-2"><Users className="w-6 h-6 text-accent" /> Pengguna Admin</span>}
        actions={
          <Button variant="primary" size="sm" onClick={() => setIsCreateOpen(true)} icon={<Plus className="w-4 h-4" />} className="w-full sm:w-auto justify-center">
            Tambah Pengguna
          </Button>
        }
      />

      {loadError && (
        <div className="mb-4">
          <QueryError message={loadError} onRetry={() => void loadAll()} />
        </div>
      )}

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(3)].map((_, idx) => (
            <Card key={idx} className="p-5 animate-pulse space-y-4">
              <div className="space-y-1.5">
                <div className="h-4 bg-bg-surface-2 rounded w-2/3" />
                <div className="h-3 bg-bg-surface-2 rounded w-1/2" />
              </div>
              <div className="h-6 bg-bg-surface-2 rounded w-1/3" />
              <div className="pt-3 border-t border-border flex gap-2">
                <div className="h-8 bg-bg-surface-2 rounded w-16" />
                <div className="h-8 bg-bg-surface-2 rounded w-20" />
              </div>
            </Card>
          ))}
        </div>
      ) : users.length === 0 ? (
        <Card className="p-10 text-center rounded-box bg-bg-surface-1 border border-border/60">
          <div className="w-12 h-12 rounded-2xl bg-sky-500/10 border border-sky-500/20 text-sky-400 flex items-center justify-center mx-auto mb-3">
            <Users className="w-6 h-6" />
          </div>
          <h3 className="text-base font-bold text-white mb-1">Belum Ada Pengguna Terdaftar</h3>
          <p className="text-xs text-text-secondary max-w-md mx-auto mb-5 leading-relaxed">
            Tambahkan akun administrator baru untuk mengelola Route-X AI Gateway.
          </p>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="h-9 px-4 text-xs font-semibold mx-auto"
          >
            Tambah Pengguna Pertama
          </Button>
        </Card>
      ) : (
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
                <div className="mt-3 flex flex-wrap gap-1 items-center">
                  <Badge variant="lime" className="text-[10px] font-mono px-2 py-0.5 flex items-center gap-1">
                    <Shield className="w-3 h-3" /> Admin
                  </Badge>
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
      )}

      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Tambah Pengguna Admin"
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-user-form">
              Simpan Pengguna
            </Button>
          </>
        }
      >
        <form id="create-user-form" noValidate onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Email *</label>
            <input
              type="email"
              required
              value={newUser.email}
              onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Tampilan *</label>
            <input
              type="text"
              required
              value={newUser.name}
              onChange={(e) => setNewUser({ ...newUser, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Password Awal *</label>
            <div className="relative">
              <input
                type={showNewUserPassword ? 'text' : 'password'}
                required
                value={newUser.password}
                onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
                className="w-full px-3 py-2 pr-10 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
              <button
                type="button"
                onClick={() => setShowNewUserPassword(!showNewUserPassword)}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-muted hover:text-white p-1"
                aria-label={showNewUserPassword ? 'Sembunyikan password' : 'Lihat password'}
              >
                {showNewUserPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>
        </form>
      </Drawer>

      <Drawer
        isOpen={detailUserId !== null}
        onClose={() => { setDetailUserId(null); setDetail(null); }}
        title="Kelola Pengguna Admin"
        maxWidth="lg"
        footer={
          <Button variant="ghost" onClick={() => { setDetailUserId(null); setDetail(null); }}>
            Tutup
          </Button>
        }
      >
        {!detail ? (
          <p className="text-xs text-text-muted py-6 text-center">Memuat detail...</p>
        ) : (
          <div className="space-y-5 text-xs">
            <div className="flex items-center justify-between p-3 bg-bg-surface-2 border border-border rounded-nav">
              <div className="flex items-center gap-2">
                <Shield className="w-4 h-4 text-accent" />
                <span className="text-white font-semibold">Tingkat Akses</span>
              </div>
              <Badge variant="lime" className="font-mono">Administrator Penuh</Badge>
            </div>

            <div className="pt-3 border-t border-border">
              <div className="flex items-center gap-1.5 text-text-secondary text-xs font-medium mb-2">
                <Pencil className="w-3.5 h-3.5 text-accent" />
                <span>Ubah Nama Tampilan:</span>
              </div>
              <div className="flex gap-2">
                <input
                  type="text"
                  placeholder="Nama tampilan baru"
                  value={renameName}
                  onChange={(e) => setRenameName(e.target.value)}
                  className="flex-1 px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
                />
                <Button
                  variant="secondary"
                  onClick={() => {
                    const name = renameName.trim();
                    if (!name || !detailUserId) {
                      toast.error('Nama tampilan baru wajib diisi.');
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

            <div className="pt-3 border-t border-border">
              <span className="block text-xs font-medium text-text-secondary mb-2">Reset Password Paksa</span>
              <div className="flex gap-2">
                <div className="relative flex-1">
                  <input
                    type={showResetPassword ? 'text' : 'password'}
                    placeholder="Password sementara"
                    value={resetPw?.userId === detailUserId ? resetPw.temp : ''}
                    onChange={(e) => setResetPw({ userId: detailUserId as string, temp: e.target.value })}
                    className="w-full px-3 py-2 pr-10 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                  />
                  <button
                    type="button"
                    onClick={() => setShowResetPassword(!showResetPassword)}
                    className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-muted hover:text-white p-1"
                    aria-label={showResetPassword ? 'Sembunyikan password' : 'Lihat password'}
                  >
                    {showResetPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
                <Button variant="secondary" onClick={() => { const u = users.find((x) => x.id === detailUserId); if (u) void handleForceReset(u); }} icon={<KeyRound className="w-3.5 h-3.5" />}>
                  Reset
                </Button>
              </div>
              <p className="mt-1.5 text-[11px] text-text-muted">Mengirim password baru dan mencabut seluruh sesi login pengguna.</p>
            </div>

            <div className="pt-3 border-t border-border flex items-center justify-between">
              <span className="text-text-secondary">Cabut seluruh sesi login pengguna ini.</span>
              <Button variant="secondary" onClick={() => { const u = users.find((x) => x.id === detailUserId); if (u) void handleRevokeSessions(u); }} icon={<LogOut className="w-3.5 h-3.5" />}>
                Cabut Sesi
              </Button>
            </div>
          </div>
        )}
      </Drawer>
    </div>
  );
};
