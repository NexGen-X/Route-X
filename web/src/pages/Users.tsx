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
        /* Loading skeleton — tabel minimalis (Item 4.2) */
        <Card className="overflow-hidden">
          <div className="animate-pulse divide-y divide-border">
            {[...Array(3)].map((_, idx) => (
              <div key={idx} className="flex items-center gap-4 px-4 py-3">
                <div className="w-8 h-8 bg-bg-surface-2 rounded-full flex-shrink-0" />
                <div className="flex-1 space-y-1.5 min-w-0">
                  <div className="h-3.5 bg-bg-surface-2 rounded w-1/3" />
                  <div className="h-3 bg-bg-surface-2 rounded w-1/2" />
                </div>
                <div className="h-5 bg-bg-surface-2 rounded w-14 flex-shrink-0" />
                <div className="h-5 bg-bg-surface-2 rounded w-16 flex-shrink-0" />
                <div className="h-7 bg-bg-surface-2 rounded w-20 flex-shrink-0" />
              </div>
            ))}
          </div>
        </Card>
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
        /* Tabel akun enterprise minimalis — Item 4.2 */
        <Card className="overflow-hidden p-0">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border bg-bg-surface-2/60">
                  <th className="px-4 py-3 text-left text-[11px] font-semibold text-text-muted uppercase tracking-wide">
                    Pengguna
                  </th>
                  <th className="px-3 py-3 text-left text-[11px] font-semibold text-text-muted uppercase tracking-wide hidden sm:table-cell">
                    Role
                  </th>
                  <th className="px-3 py-3 text-left text-[11px] font-semibold text-text-muted uppercase tracking-wide">
                    Status
                  </th>
                  <th className="px-3 py-3 text-left text-[11px] font-semibold text-text-muted uppercase tracking-wide hidden md:table-cell">
                    Keterangan
                  </th>
                  <th className="px-4 py-3 text-right text-[11px] font-semibold text-text-muted uppercase tracking-wide">
                    Aksi
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {users.map((u) => (
                  <tr key={u.id} className="hover:bg-bg-surface-2/40 transition-colors">
                    {/* Avatar + Nama + Email */}
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="w-8 h-8 rounded-full bg-accent/15 border border-accent/20 flex items-center justify-center flex-shrink-0">
                          <span className="text-xs font-bold text-accent">
                            {(u.display_name || u.email).charAt(0).toUpperCase()}
                          </span>
                        </div>
                        <div className="min-w-0">
                          <p className="font-semibold text-white truncate">{u.display_name}</p>
                          <p className="text-[11px] text-text-muted font-mono truncate">{u.email}</p>
                        </div>
                      </div>
                    </td>
                    {/* Role */}
                    <td className="px-3 py-3 hidden sm:table-cell">
                      <Badge variant="lime" className="text-[10px] font-mono px-2 py-0.5 flex items-center gap-1 w-fit">
                        <Shield className="w-3 h-3" /> Admin
                      </Badge>
                    </td>
                    {/* Status */}
                    <td className="px-3 py-3">
                      <Badge variant={statusVariant(u.status)}>{u.status}</Badge>
                    </td>
                    {/* Keterangan */}
                    <td className="px-3 py-3 hidden md:table-cell">
                      {u.must_change_password ? (
                        <span className="text-[11px] text-amber-300">Wajib ganti password</span>
                      ) : (
                        <span className="text-[11px] text-text-muted">—</span>
                      )}
                    </td>
                    {/* Aksi */}
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1.5">
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => void openDetail(u.id)}
                          icon={<UserCheck className="w-3.5 h-3.5" />}
                          className="text-xs"
                        >
                          Kelola
                        </Button>
                        {u.status === 'active' ? (
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => void handleStatus(u, 'disabled')}
                            className="text-xs hidden sm:inline-flex"
                          >
                            Nonaktifkan
                          </Button>
                        ) : (
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => void handleStatus(u, 'active')}
                            className="text-xs hidden sm:inline-flex"
                          >
                            Aktifkan
                          </Button>
                        )}
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => void handleDelete(u)}
                          icon={<Trash2 className="w-3.5 h-3.5 text-status-error/80" />}
                          className="text-xs text-status-error/80 border-status-error/20 hover:bg-status-error/10 hover:text-status-error"
                          title="Hapus pengguna"
                          aria-label={`Hapus pengguna ${u.email}`}
                        />
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
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
            <label htmlFor="new-user-email" className="block text-xs font-medium text-text-secondary mb-1.5">Email *</label>
            <input
              id="new-user-email"
              type="email"
              required
              value={newUser.email}
              onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label htmlFor="new-user-name" className="block text-xs font-medium text-text-secondary mb-1.5">Nama Tampilan *</label>
            <input
              id="new-user-name"
              type="text"
              required
              value={newUser.name}
              onChange={(e) => setNewUser({ ...newUser, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label htmlFor="new-user-password" className="block text-xs font-medium text-text-secondary mb-1.5">Password Awal *</label>
            <div className="relative">
              <input
                id="new-user-password"
                type={showNewUserPassword ? 'text' : 'password'}
                required
                value={newUser.password}
                onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
                className="w-full px-3 py-2 pr-10 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
              <button
                type="button"
                onClick={() => setShowNewUserPassword(!showNewUserPassword)}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-muted hover:text-white p-1 cursor-pointer"
                aria-label={showNewUserPassword ? 'Sembunyikan password' : 'Lihat password'}
              >
                {showNewUserPassword ? <EyeOff className="w-4 h-4" aria-hidden="true" /> : <Eye className="w-4 h-4" aria-hidden="true" />}
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
