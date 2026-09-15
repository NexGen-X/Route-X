import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { EgressPool, EgressProbeResult } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { Select } from '../components/common/Select';
import { Checkbox } from '../components/common/Checkbox';
import { PageHeader } from '../components/common/PageHeader';
import {
  Network,
  Plus,
  Trash2,
  Play,
  CheckCircle2,
  AlertCircle,
  Edit2,
  RefreshCw,
  Activity,
  Zap,
  Eye,
  EyeOff,
} from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export const Egress: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [pools, setPools] = useState<EgressPool[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [editingPool, setEditingPool] = useState<EgressPool | null>(null);
  const [showCreateProxyUrl, setShowCreateProxyUrl] = useState(false);
  const [showEditProxyUrl, setShowEditProxyUrl] = useState(false);

  const [newPool, setNewPool] = useState({
    name: '',
    kind: 'socks5',
    proxy_url: '',
    weight: 100,
    region: 'auto',
  });

  const [editForm, setEditForm] = useState({
    name: '',
    kind: 'socks5',
    proxy_url: '',
    weight: 100,
    region: 'auto',
    enabled: true,
  });

  // State hasil uji koneksi live probe
  const [testingId, setTestingId] = useState<string | null>(null);
  const [probeFeedback, setProbeFeedback] = useState<{
    poolId: string;
    result: EgressProbeResult;
  } | null>(null);

  const loadPools = async () => {
    setIsLoading(true);
    try {
      const res = await api.egress.list();
      setPools(res.items || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
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
      setNewPool({ name: '', kind: 'socks5', proxy_url: '', weight: 100, region: 'auto' });
      toast.success('Egress proxy pool baru berhasil ditambahkan.', 'Egress Dibuat');
      loadPools();
    } catch (err: any) {
      toast.error('Gagal membuat egress pool: ' + (err.message || err));
    }
  };

  const handleOpenEdit = (p: EgressPool) => {
    setEditingPool(p);
    setEditForm({
      name: p.name,
      kind: p.kind || 'socks5',
      proxy_url: '',
      weight: p.weight || 100,
      region: p.region || 'auto',
      enabled: p.enabled,
    });
  };

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingPool) return;

    try {
      const payload: Partial<EgressPool> & { proxy_url?: string } = {
        name: editForm.name,
        kind: editForm.kind,
        weight: editForm.weight,
        region: editForm.region,
        enabled: editForm.enabled,
      };
      if (editForm.proxy_url.trim()) {
        payload.proxy_url = editForm.proxy_url.trim();
      }

      await api.egress.update(editingPool.id, payload);
      setEditingPool(null);
      toast.success('Egress proxy pool berhasil diperbarui.', 'Perubahan Disimpan');
      loadPools();
    } catch (err: any) {
      toast.error('Gagal memperbarui egress pool: ' + (err.message || err));
    }
  };

  const handleDelete = async (id: string, name: string) => {
    const ok = await confirmModal({
      title: 'Hapus Egress Pool?',
      message: `Hapus egress pool "${name}"? Upstream provider terkait akan beralih ke koneksi langsung secara otomatis.`,
      confirmText: 'Ya, Hapus',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;

    try {
      await api.egress.delete(id);
      toast.success(`Egress pool "${name}" berhasil dihapus.`, 'Egress Dihapus');
      loadPools();
    } catch (err: any) {
      toast.error('Gagal menghapus egress pool: ' + (err.message || err));
    }
  };

  const handleTestProbe = async (id: string) => {
    setTestingId(id);
    setProbeFeedback(null);
    try {
      const res = await api.egress.test(id);
      setProbeFeedback({ poolId: id, result: res });
      toast.success(`Uji koneksi egress sukses: ${res.latency_ms} ms (Exit IP: ${res.exit_ip || 'n/a'})`, 'Egress Sehat');
      loadPools();
    } catch (err: any) {
      setProbeFeedback({ poolId: id, result: { status: 'unhealthy', latency_ms: 0, checked_at: new Date().toISOString(), message: err.message || String(err) } });
      toast.error('Uji koneksi egress gagal: ' + (err.message || err));
    } finally {
      setTestingId(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Halaman */}
      <PageHeader
        title={
          <span className="flex items-center gap-2.5">
            Egress Proxy Pools
            <span className="text-xs font-semibold text-accent bg-accent/10 px-2 py-0.5 rounded-full border border-accent/20">
              Stealth Routing
            </span>
          </span>
        }
        description={
          <span>
            Manajemen proxy keluar (Xray / SOCKS5 / HTTP / HTTPS) untuk merutekan panggilan upstream AI.
            Semua URL proxy dan kredensial disimpan terenkripsi secara aman dengan <strong>AES-256-GCM</strong>.
          </span>
        }
        actions={
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={loadPools} isLoading={isLoading}>
              <RefreshCw className="w-3.5 h-3.5" />
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4" />}
            >
              Tambah Pool
            </Button>
          </div>
        }
      />

      {loadError && <QueryError message={loadError} onRetry={() => void loadPools()} />}

      {/* Notifikasi Hasil Test Probe */}
      {probeFeedback && (
        <div
          role={probeFeedback.result.status === 'healthy' ? 'status' : 'alert'}
          aria-live="polite"
          className={`p-4 rounded-xl text-xs border flex items-start gap-3 transition-all ${
            probeFeedback.result.status === 'healthy'
              ? 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
              : 'bg-red-500/10 text-red-300 border-red-500/30'
          }`}
        >
          {probeFeedback.result.status === 'healthy' ? (
            <CheckCircle2 className="w-5 h-5 shrink-0 text-emerald-400 mt-0.5" />
          ) : (
            <AlertCircle className="w-5 h-5 shrink-0 text-red-400 mt-0.5" />
          )}
          <div className="flex-1 space-y-1">
            <div className="flex items-center justify-between">
              <span className="font-bold uppercase tracking-wider">
                {probeFeedback.result.status === 'healthy' ? 'Uji Koneksi Berhasil' : 'Uji Koneksi Gagal'}
              </span>
              {probeFeedback.result.latency_ms !== undefined && (
                <span className="font-mono text-[11px] bg-bg-surface-2 px-2 py-0.5 rounded border border-border text-emerald-400">
                  {probeFeedback.result.latency_ms} ms
                </span>
              )}
            </div>
            <p>{probeFeedback.result.message}</p>
            {probeFeedback.result.exit_ip && (
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pt-1 text-[11px] text-text-secondary font-mono">
                <span>Exit IP: <strong className="text-white">{probeFeedback.result.exit_ip}</strong></span>
                {probeFeedback.result.country && (
                  <span>Wilayah: <strong className="text-accent">{probeFeedback.result.country}</strong></span>
                )}
                {probeFeedback.result.datacenter && (
                  <span>Edge PoP: <strong className="text-white">{probeFeedback.result.datacenter}</strong></span>
                )}
              </div>
            )}
          </div>
          <button
            onClick={() => setProbeFeedback(null)}
            className="text-text-muted hover:text-white text-xs font-bold px-1"
          >
            ✕
          </button>
        </div>
      )}

      {/* Grid Kartu Egress Pool */}
      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <Card key={i} className="p-5 space-y-4 animate-pulse">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-bg-surface-2" />
                <div className="space-y-1.5 flex-1">
                  <div className="h-4 bg-bg-surface-2 rounded w-1/2" />
                  <div className="h-3 bg-bg-surface-2 rounded w-1/3" />
                </div>
              </div>
              <div className="space-y-2">
                <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
                <div className="h-3 bg-bg-surface-2 rounded w-1/2" />
              </div>
            </Card>
          ))}
        </div>
      ) : pools.length === 0 ? (
        <Card className="p-12 text-center">
          <div className="w-12 h-12 rounded-full bg-accent/10 text-accent flex items-center justify-center mx-auto mb-4">
            <Network className="w-6 h-6" />
          </div>
          <h3 className="text-base font-bold text-white">Belum Ada Egress Proxy Pool</h3>
          <p className="text-xs text-text-secondary mt-1 max-w-md mx-auto">
            Tambahkan proxy keluar (seperti SOCKS5 Xray lokal, BrightData, Smartproxy, atau VPS) untuk menyembunyikan IP gateway atau bypass pemblokiran wilayah AI.
          </p>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="mt-4"
          >
            Tambah Egress Pool Sekarang
          </Button>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {pools.map((p) => {
            const isTesting = testingId === p.id;
            const isXrayLocal = (p.name || '').includes('Xray');

            return (
              <Card key={p.id} className="p-5 flex flex-col justify-between border-border hover:border-accent/30 transition-all">
                <div>
                  {/* Header Kartu */}
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div
                        className={`w-10 h-10 rounded-xl flex items-center justify-center border ${
                          isXrayLocal
                            ? 'bg-amber-500/10 border-amber-500/20 text-amber-400'
                            : 'bg-blue-500/10 border-blue-500/20 text-blue-400'
                        }`}
                      >
                        {isXrayLocal ? <Zap className="w-5 h-5" /> : <Network className="w-5 h-5" />}
                      </div>
                      <div>
                        <h4 className="text-sm font-bold text-white flex items-center gap-1.5">
                          {p.name || 'tanpa nama'}
                        </h4>
                        <span className="text-[11px] text-text-muted font-mono uppercase">
                          {(p.kind || 'PROXY')} • {p.region || 'global'}
                        </span>
                      </div>
                    </div>
                    <Badge variant={p.enabled ? 'success' : 'neutral'}>
                      {p.enabled ? 'active' : 'disabled'}
                    </Badge>
                  </div>

                  {/* Detail Info & Status Kesehatan */}
                  <div className="mt-4 space-y-2 text-xs">
                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Status Koneksi</span>
                      {p.last_health_status === 'healthy' ? (
                        <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-400">
                          <Activity className="w-3 h-3 text-emerald-400" />
                          Terhubung ({p.last_latency_ms || 0} ms)
                        </span>
                      ) : p.last_health_status === 'unhealthy' ? (
                        <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-red-400">
                          <AlertCircle className="w-3 h-3 text-red-400" />
                          Tidak Terhubung
                        </span>
                      ) : (
                        <span className="text-text-muted text-[11px]">Belum Diuji</span>
                      )}
                    </div>

                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Target Proxy</span>
                      <span className="font-mono text-accent text-[11px] truncate max-w-[190px]">
                        {p.masked_hint || '[ENCRYPTED AT REST]'}
                      </span>
                    </div>

                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Bobot Alokasi</span>
                      <span className="font-mono text-white">{p.weight}</span>
                    </div>
                  </div>
                </div>

                <div className="mt-5 pt-3 border-t border-border flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleTestProbe(p.id)}
                    isLoading={isTesting}
                    icon={<Play className="w-3.5 h-3.5 text-emerald-400" />}
                    className="w-full sm:w-auto justify-center text-xs text-white"
                  >
                    Uji Ping
                  </Button>

                  <div className="grid grid-cols-2 sm:flex items-center gap-2 w-full sm:w-auto">
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleOpenEdit(p)}
                      icon={<Edit2 className="w-3.5 h-3.5" />}
                      className="w-full sm:w-auto justify-center text-xs"
                    >
                      Edit
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => handleDelete(p.id, p.name)}
                      icon={<Trash2 className="w-3.5 h-3.5" />}
                      className="w-full sm:w-auto justify-center text-xs"
                    >
                      Hapus
                    </Button>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {/* Modal Tambah Egress Pool */}
      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Tambah Egress Proxy Pool"
        subtitle="URL proxy akan dienkripsi secara aman dengan AES-256-GCM"
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-egress-form">
              Simpan Egress Pool
            </Button>
          </>
        }
      >
        <form id="create-egress-form" noValidate onSubmit={handleCreate} className="space-y-4 text-xs">
          <Select
            label="Preset / Sumber Proxy"
            value=""
            placeholder="Pilih Preset atau Kustom..."
            onChange={(val) => {
              if (val === 'custom') return;
              if (val === 'xray_socks') {
                setNewPool({
                  ...newPool,
                  name: '⚡ Xray SOCKS5 Bridge (Local)',
                  kind: 'socks5',
                  proxy_url: 'socks5://xray:10808',
                  region: 'local',
                });
              } else if (val === 'xray_http') {
                setNewPool({
                  ...newPool,
                  name: '⚡ Xray HTTP Bridge (Local)',
                  kind: 'http',
                  proxy_url: 'http://xray:10809',
                  region: 'local',
                });
              } else if (val === 'xray_tunnel') {
                setNewPool({
                  ...newPool,
                  name: '⚡ Xray Stealth Tunnel (Multiplexed)',
                  kind: 'socks5',
                  proxy_url: 'socks5://xray:10808',
                  region: 'auto',
                });
              }
            }}
            options={[
              { value: 'custom', label: 'Kustom / Manual (Masukkan Proxy Luar)', description: 'Konfigurasi IP/domain proxy eksternal' },
              { value: 'xray_socks', label: '⚡ Xray SOCKS5 Internal Bridge', description: 'socks5://xray:10808' },
              { value: 'xray_http', label: '⚡ Xray HTTP Internal Bridge', description: 'http://xray:10809' },
              { value: 'xray_tunnel', label: '⚡ Xray Stealth Tunnel', description: 'Auto Multiplexed Stealth Routing' },
            ]}
          />
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Pool *</label>
            <input
              type="text"
              required
              placeholder="residential-sg-1 atau xray-tunnel"
              value={newPool.name}
              onChange={(e) => setNewPool({ ...newPool, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <Select
            label="Protokol Proxy"
            value={newPool.kind}
            onChange={(val) => setNewPool({ ...newPool, kind: val })}
            options={[
              { value: 'socks5', label: 'SOCKS5 Proxy (Termasuk Xray / Sing-box)', description: 'Protokol raw socket tcp/udp dengan stealth transport' },
              { value: 'http', label: 'HTTP Proxy', description: 'Standar http proxy forwarder' },
              { value: 'https', label: 'HTTPS Proxy', description: 'Http proxy terenkripsi TLS' },
            ]}
          />
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-xs font-medium text-text-secondary">Proxy URL Lengkap *</label>
              <button
                type="button"
                onClick={() => setShowCreateProxyUrl(!showCreateProxyUrl)}
                className="text-[11px] text-text-muted hover:text-white flex items-center gap-1 focus:outline-none cursor-pointer"
              >
                {showCreateProxyUrl ? (
                  <>
                    <EyeOff className="w-3 h-3" />
                    <span>Sembunyikan</span>
                  </>
                ) : (
                  <>
                    <Eye className="w-3 h-3" />
                    <span>Tampilkan</span>
                  </>
                )}
              </button>
            </div>
            <input
              type={showCreateProxyUrl ? 'text' : 'password'}
              required
              placeholder="socks5://user:pass@host:1080 atau socks5://xray:10808"
              value={newPool.proxy_url}
              onChange={(e) => setNewPool({ ...newPool, proxy_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
            <span className="text-[11px] text-text-muted block mt-1">
              Untuk container Xray internal, gunakan: <code className="text-accent font-mono">socks5://xray:10808</code>
            </span>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Bobot Alokasi</label>
              <input
                type="number"
                min="1"
                max="1000"
                value={newPool.weight}
                onChange={(e) => setNewPool({ ...newPool, weight: parseInt(e.target.value) || 100 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Wilayah (Region)</label>
              <input
                type="text"
                placeholder="auto / ap-southeast-1"
                value={newPool.region}
                onChange={(e) => setNewPool({ ...newPool, region: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
              />
            </div>
          </div>
        </form>
      </Drawer>

      {/* Modal Edit Egress Pool */}
      <Drawer
        isOpen={!!editingPool}
        onClose={() => setEditingPool(null)}
        title="Edit Egress Proxy Pool"
        subtitle="Perbarui konfigurasi proxy keluar atau lakukan rotasi kredensial"
        footer={
          <>
            <Button variant="ghost" onClick={() => setEditingPool(null)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="edit-egress-form">
              Simpan Perubahan
            </Button>
          </>
        }
      >
        <form id="edit-egress-form" noValidate onSubmit={handleUpdate} className="space-y-4 text-xs">
          <Select
            label="Preset / Sumber Proxy"
            value=""
            placeholder="Pilih Preset Baru..."
            onChange={(val) => {
              if (val === 'custom') return;
              if (val === 'xray_socks') {
                setEditForm({
                  ...editForm,
                  name: '⚡ Xray SOCKS5 Bridge (Local)',
                  kind: 'socks5',
                  proxy_url: 'socks5://xray:10808',
                  region: 'local',
                });
              } else if (val === 'xray_http') {
                setEditForm({
                  ...editForm,
                  name: '⚡ Xray HTTP Bridge (Local)',
                  kind: 'http',
                  proxy_url: 'http://xray:10809',
                  region: 'local',
                });
              }
            }}
            options={[
              { value: 'custom', label: 'Pertahankan / Masukkan URL Manual', description: 'Gunakan URL proxy kustom saat ini' },
              { value: 'xray_socks', label: '⚡ Xray SOCKS5 Internal Bridge', description: 'socks5://xray:10808' },
              { value: 'xray_http', label: '⚡ Xray HTTP Internal Bridge', description: 'http://xray:10809' },
            ]}
          />
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Pool *</label>
            <input
              type="text"
              required
              value={editForm.name}
              onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <Select
            label="Protokol Proxy"
            value={editForm.kind}
            onChange={(val) => setEditForm({ ...editForm, kind: val })}
            options={[
              { value: 'socks5', label: 'SOCKS5 Proxy', description: 'Protokol raw socket tcp/udp dengan stealth transport' },
              { value: 'http', label: 'HTTP Proxy', description: 'Standar http proxy forwarder' },
              { value: 'https', label: 'HTTPS Proxy', description: 'Http proxy terenkripsi TLS' },
            ]}
          />
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-xs font-medium text-text-secondary">
                Ganti Proxy URL (Rotasi Sandi)
              </label>
              <button
                type="button"
                onClick={() => setShowEditProxyUrl(!showEditProxyUrl)}
                className="text-[11px] text-text-muted hover:text-white flex items-center gap-1 focus:outline-none cursor-pointer"
              >
                {showEditProxyUrl ? (
                  <>
                    <EyeOff className="w-3 h-3" />
                    <span>Sembunyikan</span>
                  </>
                ) : (
                  <>
                    <Eye className="w-3 h-3" />
                    <span>Tampilkan</span>
                  </>
                )}
              </button>
            </div>
            <input
              type={showEditProxyUrl ? 'text' : 'password'}
              placeholder="Kosongkan jika tidak ingin mengubah URL proxy saat ini"
              value={editForm.proxy_url}
              onChange={(e) => setEditForm({ ...editForm, proxy_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono placeholder:text-text-muted focus:outline-none focus:border-accent"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Bobot Alokasi</label>
              <input
                type="number"
                min="1"
                max="1000"
                value={editForm.weight}
                onChange={(e) => setEditForm({ ...editForm, weight: parseInt(e.target.value) || 100 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Wilayah (Region)</label>
              <input
                type="text"
                value={editForm.region}
                onChange={(e) => setEditForm({ ...editForm, region: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
              />
            </div>
          </div>
          <div className="pt-2">
            <Checkbox
              id="editPoolEnabled"
              checked={editForm.enabled}
              onChange={(e) => setEditForm({ ...editForm, enabled: e.target.checked })}
              label="Aktifkan pool ini untuk menerima lalu lintas keluar"
            />
          </div>
        </form>
      </Drawer>
    </div>
  );
};
