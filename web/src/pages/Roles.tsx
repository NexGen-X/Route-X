import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Role } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { Checkbox } from '../components/common/Checkbox';
import { PageHeader } from '../components/common/PageHeader';
import { Tooltip } from '../components/common/Tooltip';
import { ShieldCheck, Plus, Trash2, Save, SlidersHorizontal, Lock, Crown } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

interface PermItem { key: string; description: string }

export const RolesPage: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [roles, setRoles] = useState<Role[]>([]);
  const [perms, setPerms] = useState<PermItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newRole, setNewRole] = useState({ name: '', description: '', rank: 100 });
  const [editRoleId, setEditRoleId] = useState<string | null>(null);
  const [editPerms, setEditPerms] = useState<string[]>([]);
  const [editLoading, setEditLoading] = useState(false);
  const [permSearch, setPermSearch] = useState('');

  const loadAll = async () => {
    setIsLoading(true);
    try {
      const [rRes, pRes] = await Promise.all([api.roles.list(), api.roles.permissions()]);
      setRoles(rRes.items || []);
      setPerms(pRes.items || []);
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
    const target = roles.find((r) => r.id === roleId);
    if (target && (target.name.toLowerCase().replace(/[\s_-]/g, '') === 'superadmin' || target.rank === 0)) {
      toast.info('Peran Super Admin memiliki akses penuh permanen dan tidak memerlukan penyesuaian izin.');
      return;
    }
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

      {loadError && (
        <div className="mb-4">
          <QueryError message={loadError} onRetry={() => void loadAll()} />
        </div>
      )}

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(3)].map((_, idx) => (
            <Card key={idx} className="p-5 animate-pulse space-y-4">
              <div className="flex justify-between">
                <div className="h-4 bg-bg-surface-2 rounded w-1/2" />
                <div className="h-5 bg-bg-surface-2 rounded w-12" />
              </div>
              <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
              <div className="pt-3 border-t border-border flex gap-2">
                <div className="h-8 bg-bg-surface-2 rounded w-16" />
                <div className="h-8 bg-bg-surface-2 rounded w-16" />
              </div>
            </Card>
          ))}
        </div>
      ) : roles.length === 0 ? (
        <Card className="p-10 text-center rounded-box bg-bg-surface-1 border border-border/60">
          <div className="w-12 h-12 rounded-2xl bg-sky-500/10 border border-sky-500/20 text-sky-400 flex items-center justify-center mx-auto mb-3">
            <ShieldCheck className="w-6 h-6" />
          </div>
          <h3 className="text-base font-bold text-white mb-1">Belum Ada Peran Didefinisikan</h3>
          <p className="text-xs text-text-secondary max-w-md mx-auto mb-5 leading-relaxed">
            Buat peran kustom untuk menetapkan izin spesifik seperti pemantauan observabilitas, pengelolaan kunci API, atau konfigurasi model.
          </p>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="h-9 px-4 text-xs font-semibold mx-auto"
          >
            Buat Peran Pertama
          </Button>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {roles.map((r) => {
            const isSuperAdmin = r.name.toLowerCase().replace(/[\s_-]/g, '') === 'superadmin' || r.rank === 0;

            return (
              <Card key={r.id} className="p-5 flex flex-col justify-between">
                <div>
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-1.5 min-w-0">
                      <h4 className="text-sm font-bold text-white truncate">{r.name}</h4>
                      {isSuperAdmin && (
                        <Tooltip content="Peran Tertinggi / Root">
                          <span className="inline-flex items-center">
                            <Crown className="w-3.5 h-3.5 text-amber-400 shrink-0" />
                          </span>
                        </Tooltip>
                      )}
                    </div>
                    <div className="flex gap-1 shrink-0">
                      {isSuperAdmin ? (
                        <Badge variant="success">root / penuh</Badge>
                      ) : (
                        <>
                          {r.is_system && <Badge variant="info">sistem</Badge>}
                          <Badge variant="neutral">rank {r.rank}</Badge>
                        </>
                      )}
                    </div>
                  </div>
                  <p className="mt-1.5 text-[11px] text-text-muted leading-relaxed">
                    {isSuperAdmin
                      ? 'Peran sistem tertinggi dengan hak akses mutlak dan tidak terbatas ke seluruh API gateway.'
                      : r.description || 'Tanpa deskripsi.'}
                  </p>
                </div>

                <div className="mt-4 pt-3 border-t border-border flex items-center justify-between gap-2">
                  {isSuperAdmin ? (
                    <div className="flex items-center justify-between w-full py-1.5 px-3 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
                      <span className="flex items-center gap-1.5 text-xs font-semibold">
                        <ShieldCheck className="w-4 h-4 text-emerald-400" />
                        Akses Penuh Permanen
                      </span>
                      <span className="text-[10px] text-emerald-400/80 font-mono font-bold">Anti-Lockout</span>
                    </div>
                  ) : (
                    <>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => void openEdit(r.id)}
                        icon={<SlidersHorizontal className="w-3.5 h-3.5 text-accent" />}
                        className="cursor-pointer"
                      >
                        Edit Izin
                      </Button>

                      {r.is_system ? (
                        <Tooltip content="Peran sistem dilindungi dari penghapusan" position="left">
                          <div
                            className="flex items-center gap-1 text-[11px] text-text-muted px-2.5 py-1 rounded-nav bg-bg-surface-2 border border-border/40 cursor-default select-none"
                          >
                            <Lock className="w-3 h-3 text-text-muted" />
                            <span>Terkunci</span>
                          </div>
                        </Tooltip>
                      ) : (
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => void handleDelete(r)}
                          icon={<Trash2 className="w-3.5 h-3.5 text-status-error" />}
                          className="hover:border-status-error/40 hover:text-status-error cursor-pointer"
                        >
                          Hapus
                        </Button>
                      )}
                    </>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      )}

      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Buat Peran Kustom"
        subtitle="Rank 0 paling berkuasa. Pakai 100 ke atas untuk peran biasa."
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-role-form">
              Buat Peran
            </Button>
          </>
        }
      >
        <form id="create-role-form" noValidate onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Peran *</label>
            <input
              type="text"
              required
              value={newRole.name}
              onChange={(e) => setNewRole({ ...newRole, name: e.target.value })}
              placeholder="Operator"
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Deskripsi</label>
            <input
              type="text"
              value={newRole.description}
              onChange={(e) => setNewRole({ ...newRole, description: e.target.value })}
              placeholder="Boleh baca semua, tulis terbatas"
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Rank</label>
            <input
              type="number"
              min={0}
              value={newRole.rank}
              onChange={(e) => {
                const v = parseInt(e.target.value, 10);
                setNewRole({ ...newRole, rank: Number.isNaN(v) ? 100 : v });
              }}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
        </form>
      </Drawer>

      <Drawer
        isOpen={editRoleId !== null}
        onClose={() => setEditRoleId(null)}
        title={`Izin: ${editRole?.name || ''}`}
        subtitle="Centang izin yang dipegang peran ini"
        maxWidth="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setEditRoleId(null)}>
              Batal
            </Button>
            <Button variant="primary" onClick={() => void handleSavePerms()} icon={<Save className="w-3.5 h-3.5" />}>
              Simpan Izin ({editPerms.length})
            </Button>
          </>
        }
      >
        {editLoading ? (
          <p className="text-xs text-text-muted py-6 text-center">Memuat izin...</p>
        ) : (
          <div className="space-y-3 text-xs">
            <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2">
              <input
                type="text"
                placeholder="Cari izin (misal: models, keys)..."
                value={permSearch}
                onChange={(e) => setPermSearch(e.target.value)}
                className="flex-1 px-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-white text-xs focus:outline-none focus:border-accent"
              />
              <div className="flex items-center gap-1.5 self-end sm:self-auto">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    const matching = perms.filter((p) =>
                      p.key.toLowerCase().includes(permSearch.toLowerCase()) ||
                      p.description.toLowerCase().includes(permSearch.toLowerCase())
                    ).map((p) => p.key);
                    setEditPerms((prev) => Array.from(new Set([...prev, ...matching])));
                  }}
                  className="h-7 px-2 text-[11px]"
                >
                  Pilih Semua
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    const matching = new Set(
                      perms.filter((p) =>
                        p.key.toLowerCase().includes(permSearch.toLowerCase()) ||
                        p.description.toLowerCase().includes(permSearch.toLowerCase())
                      ).map((p) => p.key)
                    );
                    setEditPerms((prev) => prev.filter((k) => !matching.has(k)));
                  }}
                  className="h-7 px-2 text-[11px]"
                >
                  Lepas Semua
                </Button>
              </div>
            </div>

            <div className="space-y-4 max-h-[65vh] overflow-y-auto pr-1">
              {[
                {
                  id: 'gateway',
                  title: 'Core Gateway & Routing',
                  description: 'Aturan perutean, pembatasan laju, pencekalan IP, dan filter konten',
                  matches: (key: string) =>
                    ['routing:', 'ratelimits:', 'bans:', 'filters:', 'requests:'].some((prefix) => key.startsWith(prefix)),
                },
                {
                  id: 'upstreams',
                  title: 'Models & Providers Upstream',
                  description: 'Katalog model AI kanonis, provider upstream, dan rotasi kredensial',
                  matches: (key: string) =>
                    ['models:', 'providers:', 'credentials:'].some((prefix) => key.startsWith(prefix)),
                },
                {
                  id: 'identity',
                  title: 'Identitas, Peran & Kunci API',
                  description: 'Manajemen pengguna admin, matriks peran RBAC, dan kunci API klien',
                  matches: (key: string) =>
                    ['users:', 'roles:', 'apikeys:'].some((prefix) => key.startsWith(prefix)),
                },
                {
                  id: 'billing',
                  title: 'Biaya, Anggaran & Pemakaian',
                  description: 'Pagu pengeluaran USD skala 8 desimal dan pemantauan biaya komputasi',
                  matches: (key: string) =>
                    ['budgets:', 'usage:'].some((prefix) => key.startsWith(prefix)),
                },
                {
                  id: 'system',
                  title: 'Operasional, Webhook & Diagnostik',
                  description: 'Webhook, integrasi pihak ketiga, audit log, diagnostik kesehatan, dan setelan sistem',
                  matches: (key: string) =>
                    ['webhooks:', 'integrations:', 'audit:', 'health:', 'settings:'].some((prefix) => key.startsWith(prefix)),
                },
              ].map((group) => {
                const groupPerms = perms.filter(
                  (p) =>
                    group.matches(p.key) &&
                    (p.key.toLowerCase().includes(permSearch.toLowerCase()) ||
                      p.description.toLowerCase().includes(permSearch.toLowerCase()))
                );
                if (groupPerms.length === 0) return null;

                const selectedCount = groupPerms.filter((p) => editPerms.includes(p.key)).length;
                const isAllSelected = selectedCount === groupPerms.length;

                return (
                  <div key={group.id} className="rounded-xl bg-bg-surface-1 border border-border/60 p-3.5 space-y-3">
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-2 border-b border-border/40">
                      <div>
                        <div className="flex items-center gap-2">
                          <h4 className="text-xs font-bold text-white">{group.title}</h4>
                          <span
                            className={`px-2 py-0.5 rounded-full text-[10px] font-mono font-semibold ${
                              isAllSelected
                                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                                : selectedCount > 0
                                ? 'bg-accent/10 text-accent border border-accent/20'
                                : 'bg-bg-surface-2 text-text-muted border border-border'
                            }`}
                          >
                            {selectedCount} / {groupPerms.length} dipilih
                          </span>
                        </div>
                        <p className="text-[10px] text-text-muted mt-0.5">{group.description}</p>
                      </div>

                      <div className="flex items-center gap-1.5 self-end sm:self-auto">
                        <button
                          type="button"
                          onClick={() => {
                            const keys = groupPerms.map((p) => p.key);
                            if (isAllSelected) {
                              setEditPerms((prev) => prev.filter((k) => !keys.includes(k)));
                            } else {
                              setEditPerms((prev) => Array.from(new Set([...prev, ...keys])));
                            }
                          }}
                          className="text-[11px] font-medium text-accent hover:underline px-1 py-0.5 cursor-pointer"
                        >
                          {isAllSelected ? 'Lepas Grup' : 'Pilih Grup'}
                        </button>
                      </div>
                    </div>

                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      {groupPerms.map((p) => {
                        const checked = editPerms.includes(p.key);
                        return (
                          <div
                            key={p.key}
                            onClick={() => togglePerm(p.key)}
                            className={`flex items-start gap-2.5 p-2.5 rounded-lg border cursor-pointer transition-all ${
                              checked
                                ? 'bg-accent/5 border-accent/40 text-white shadow-sm'
                                : 'bg-bg-surface-2/60 border-border/60 text-text-secondary hover:border-border'
                            }`}
                          >
                            <Checkbox
                              checked={checked}
                              onChange={() => togglePerm(p.key)}
                              onClick={(e) => e.stopPropagation()}
                            />
                            <div className="min-w-0">
                              <span className="block font-mono text-[11px] font-semibold text-white truncate">
                                {p.key}
                              </span>
                              <span className="block text-[10px] text-text-muted mt-0.5 leading-tight">
                                {p.description}
                              </span>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                );
              })}

              {/* Izin Lainnya (Fallback) */}
              {(() => {
                const otherPerms = perms.filter(
                  (p) =>
                    !['routing:', 'ratelimits:', 'bans:', 'filters:', 'requests:', 'models:', 'providers:', 'credentials:', 'users:', 'roles:', 'apikeys:', 'budgets:', 'usage:', 'webhooks:', 'integrations:', 'audit:', 'health:', 'settings:'].some(
                      (prefix) => p.key.startsWith(prefix)
                    ) &&
                    (p.key.toLowerCase().includes(permSearch.toLowerCase()) ||
                      p.description.toLowerCase().includes(permSearch.toLowerCase()))
                );
                if (otherPerms.length === 0) return null;
                return (
                  <div className="rounded-xl bg-bg-surface-1 border border-border/60 p-3.5 space-y-3">
                    <h4 className="text-xs font-bold text-white">Izin Lainnya</h4>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      {otherPerms.map((p) => (
                        <div
                          key={p.key}
                          onClick={() => togglePerm(p.key)}
                          className="flex items-start gap-2.5 p-2.5 rounded-lg bg-bg-surface-2 border border-border cursor-pointer hover:border-border/80"
                        >
                          <Checkbox
                            checked={editPerms.includes(p.key)}
                            onChange={() => togglePerm(p.key)}
                            onClick={(e) => e.stopPropagation()}
                          />
                          <div>
                            <span className="block font-mono text-[11px] text-white font-semibold">{p.key}</span>
                            <span className="block text-[10px] text-text-muted mt-0.5">{p.description}</span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                );
              })()}

              {perms.length === 0 && <p className="text-text-muted text-center py-6">Katalog izin kosong.</p>}
            </div>
          </div>
        )}
      </Drawer>
    </div>
  );
};
