import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { User, Role } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Users as UsersIcon, Plus, Key, Shield, LogOut } from 'lucide-react';

export const Users: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newUser, setNewUser] = useState({
    name: '',
    email: '',
    password: '',
    role_ids: [] as string[],
  });

  const loadUsers = async () => {
    try {
      const [uRes, rRes] = await Promise.all([api.users.list(), api.roles.list()]);
      setUsers(uRes.items || []);
      setRoles(rRes.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadUsers();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.users.create({
        ...newUser,
        must_change_password: true,
      });
      setIsCreateOpen(false);
      loadUsers();
    } catch (err) {
      alert('Gagal membuat user: ' + err);
    }
  };

  const handleForceReset = async (u: User) => {
    if (!confirm(`Paksa reset password untuk ${u.email}?`)) return;
    try {
      const res = await api.users.forceResetPassword(u.id);
      alert(`Password sementara: ${res.temp_password}`);
      loadUsers();
    } catch (err) {
      alert('Gagal reset password: ' + err);
    }
  };

  const handleRevokeSessions = async (u: User) => {
    if (!confirm(`Cabut seluruh sesi login aktif untuk ${u.email}?`)) return;
    try {
      await api.users.revokeSessions(u.id);
      alert('Seluruh sesi berhasil dicabut.');
    } catch (err) {
      alert('Gagal mencabut sesi: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Users & RBAC</h2>
          <p className="text-xs text-text-secondary mt-1">
            Manajemen akun pengguna konsol dan penetapan peran (Super Admin, Admin, Operator, Viewer).
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Tambah Pengguna
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {users.map((u) => (
          <Card key={u.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-accent/20 border border-accent/40 text-accent font-bold flex items-center justify-center text-xs">
                    {u.display_name.charAt(0).toUpperCase()}
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{u.display_name}</h4>
                    <span className="text-[11px] text-text-muted font-mono">{u.email}</span>
                  </div>
                </div>
                <Badge variant={u.status === 'active' ? 'success' : 'error'}>
                  {u.status}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Peran (Roles)</span>
                  <div className="flex gap-1">
                    {u.roles?.map((r) => (
                      <Badge key={r} variant="lime" size="sm">
                        {r}
                      </Badge>
                    )) || '-'}
                  </div>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Login Terakhir</span>
                  <span className="font-mono text-text-secondary">
                    {u.last_login_at ? new Date(u.last_login_at).toLocaleDateString() : 'Belum pernah'}
                  </span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleForceReset(u)}
                icon={<Key className="w-3.5 h-3.5" />}
              >
                Reset Sandi
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleRevokeSessions(u)}
                icon={<LogOut className="w-3.5 h-3.5" />}
              >
                Cabut Sesi
              </Button>
            </div>
          </Card>
        ))}
      </div>

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Daftarkan Pengguna Baru"
        subtitle="Kredensial login untuk tim administrasi Route-X"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Lengkap</label>
            <input
              type="text"
              required
              placeholder="Ahmad Fauzi"
              value={newUser.name}
              onChange={(e) => setNewUser({ ...newUser, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Alamat Email</label>
            <input
              type="email"
              required
              placeholder="fauzi@routex.internal"
              value={newUser.email}
              onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Kata Sandi Awal</label>
            <input
              type="password"
              required
              placeholder="Minimal 12 karakter"
              value={newUser.password}
              onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Pilih Peran (Role)</label>
            <div className="space-y-1.5 mt-2">
              {roles.map((r) => (
                <label key={r.id} className="flex items-center gap-2 text-text-primary cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newUser.role_ids.includes(r.id)}
                    onChange={(e) => {
                      if (e.target.checked) {
                        setNewUser({ ...newUser, role_ids: [...newUser.role_ids, r.id] });
                      } else {
                        setNewUser({ ...newUser, role_ids: newUser.role_ids.filter((id) => id !== r.id) });
                      }
                    }}
                    className="rounded bg-bg-surface-2 border-border text-accent focus:ring-0"
                  />
                  <span>{r.name}</span>
                </label>
              ))}
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Buat Akun Pengguna
          </Button>
        </form>
      </Modal>
    </div>
  );
};
