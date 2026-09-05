import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { EgressPool, EgressProbeResult } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
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
} from 'lucide-react';

export const Egress: React.FC = () => {
  const [pools, setPools] = useState<EgressPool[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [editingPool, setEditingPool] = useState<EgressPool | null>(null);

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
    } catch (err) {
      console.error('Gagal memuat egress pool:', err);
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
      loadPools();
    } catch (err) {
      alert('Gagal membuat egress pool: ' + err);
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
      loadPools();
    } catch (err) {
      alert('Gagal memperbarui egress pool: ' + err);
    }
  };

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`Hapus egress pool "${name}"? Upstream provider terkait akan beralih ke koneksi langsung.`)) return;
    try {
      await api.egress.delete(id);
      loadPools();
    } catch (err) {
      alert('Gagal menghapus egress pool: ' + err);
    }
  };

  const handleTestProbe = async (id: string) => {
    setTestingId(id);
    setProbeFeedback(null);
    try {
      const res = await api.egress.test(id);
      setProbeFeedback({ poolId: id, result: res });
      // Refresh status pool
      loadPools();
    } catch (err: any) {
      setProbeFeedback({
        poolId: id,
        result: {
          status: 'unhealthy',
          message: 'Uji koneksi gagal dieksekusi: ' + (err.message || err),
          checked_at: new Date().toISOString(),
        },
      });
    } finally {
      setTestingId(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Halaman */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white flex items-center gap-2.5">
            Egress Proxy Pools
            <span className="text-xs font-semibold text-accent bg-accent/10 px-2 py-0.5 rounded-full border border-accent/20">
              Stealth Routing
            </span>
          </h2>
          <p className="text-xs text-text-secondary mt-1 max-w-2xl">
            Manajemen proxy keluar (Xray / SOCKS5 / HTTP / HTTPS) untuk merutekan panggilan upstream AI.
            Semua URL proxy dan kredensial disimpan terenkripsi secara aman dengan <strong>AES-256-GCM</strong>.
          </p>
        </div>
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
      </div>

      {/* Notifikasi Hasil Test Probe */}
      {probeFeedback && (
        <div
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
            <div className="font-bold text-white flex items-center gap-2">
              {probeFeedback.result.status === 'healthy' ? 'Koneksi Proxy Sukses' : 'Koneksi Proxy Gagal'}
              {probeFeedback.result.latency_ms !== undefined && (
                <span className="font-mono text-[11px] bg-bg-surface-2 px-2 py-0.5 rounded border border-border text-emerald-400">
                  {probeFeedback.result.latency_ms} ms
                </span>
              )}
            </div>
            <p>{probeFeedback.result.message}</p>
            {probeFeedback.result.exit_ip && (
              <div className="flex items-center gap-3 pt-1 text-[11px] text-text-secondary font-mono">
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
      {pools.length === 0 && !isLoading ? (
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
            const isXrayLocal = p.name.includes('Xray');

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
                          {p.name}
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
                      <span className="font-mono text-accent text-[11px] truncate max-w-[190px]" title={p.masked_hint || '[ENCRYPTED AT REST]'}>
                        {p.masked_hint || '[ENCRYPTED AT REST]'}
                      </span>
                    </div>

                    <div className="flex justify-between py-1 border-b border-border/40">
                      <span className="text-text-muted">Bobot Alokasi</span>
                      <span className="font-mono text-white">{p.weight}</span>
                    </div>
                  </div>
                </div>

                {/* Tombol Aksi */}
                <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => handleTestProbe(p.id)}
                    isLoading={isTesting}
                    icon={<Play className="w-3.5 h-3.5 text-emerald-400" />}
                    className="text-xs text-white"
                  >
                    Uji Ping
                  </Button>

                  <div className="flex items-center gap-2">
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleOpenEdit(p)}
                      icon={<Edit2 className="w-3.5 h-3.5" />}
                    >
                      Edit
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => handleDelete(p.id, p.name)}
                      icon={<Trash2 className="w-3.5 h-3.5" />}
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
      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Tambah Egress Proxy Pool"
        subtitle="URL proxy akan dienkripsi secara aman dengan AES-256-GCM"
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Preset / Sumber Proxy</label>
            <select
              onChange={(e) => {
                const val = e.target.value;
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
              className="w-full px-3 py-2 bg-bg-surface-2 border border-accent/40 rounded-nav text-white text-xs font-medium focus:outline-none focus:border-accent"
            >
              <option value="custom">Kustom / Manual (Masukkan Proxy Luar)</option>
              <option value="xray_socks">⚡ Xray SOCKS5 Internal Bridge (socks5://xray:10808)</option>
              <option value="xray_http">⚡ Xray HTTP Internal Bridge (http://xray:10809)</option>
              <option value="xray_tunnel">⚡ Xray Stealth Tunnel (Auto Multiplexed)</option>
            </select>
            <span className="text-[11px] text-text-muted block mt-1">
              Pilih preset untuk mengisi otomatis URL dan konfigurasi Xray tanpa perlu mengetik manual.
            </span>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Pool</label>
            <input
              type="text"
              required
              placeholder="residential-sg-1 atau xray-tunnel"
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
              <option value="socks5">SOCKS5 Proxy (Termasuk Xray / Sing-box)</option>
              <option value="http">HTTP Proxy</option>
              <option value="https">HTTPS Proxy</option>
            </select>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Proxy URL Lengkap</label>
            <input
              type="text"
              required
              placeholder="socks5://user:pass@host:1080 atau socks5://xray:10808"
              value={newPool.proxy_url}
              onChange={(e) => setNewPool({ ...newPool, proxy_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
            <span className="text-[11px] text-text-muted block mt-1">
              Untuk container Xray internal, gunakan: <code>socks5://xray:10808</code>
            </span>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Bobot Alokasi</label>
              <input
                type="number"
                min="1"
                max="1000"
                value={newPool.weight}
                onChange={(e) => setNewPool({ ...newPool, weight: parseInt(e.target.value) || 100 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Wilayah (Region)</label>
              <input
                type="text"
                placeholder="auto / ap-southeast-1"
                value={newPool.region}
                onChange={(e) => setNewPool({ ...newPool, region: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Egress Pool
          </Button>
        </form>
      </Modal>

      {/* Modal Edit Egress Pool */}
      <Modal
        isOpen={!!editingPool}
        onClose={() => setEditingPool(null)}
        title="Edit Egress Proxy Pool"
        subtitle="Perbarui konfigurasi proxy keluar atau lakukan rotasi kredensial"
      >
        <form onSubmit={handleUpdate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Preset / Sumber Proxy</label>
            <select
              onChange={(e) => {
                const val = e.target.value;
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
              className="w-full px-3 py-2 bg-bg-surface-2 border border-accent/40 rounded-nav text-white text-xs font-medium focus:outline-none focus:border-accent"
            >
              <option value="custom">Pertahankan / Masukkan URL Manual</option>
              <option value="xray_socks">⚡ Xray SOCKS5 Internal Bridge (socks5://xray:10808)</option>
              <option value="xray_http">⚡ Xray HTTP Internal Bridge (http://xray:10809)</option>
            </select>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Pool</label>
            <input
              type="text"
              required
              value={editForm.name}
              onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Protokol Proxy</label>
            <select
              value={editForm.kind}
              onChange={(e) => setEditForm({ ...editForm, kind: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            >
              <option value="socks5">SOCKS5 Proxy</option>
              <option value="http">HTTP Proxy</option>
              <option value="https">HTTPS Proxy</option>
            </select>
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">
              Ganti Proxy URL (Rotasi Sandi)
            </label>
            <input
              type="text"
              placeholder="Kosongkan jika tidak ingin mengubah URL proxy saat ini"
              value={editForm.proxy_url}
              onChange={(e) => setEditForm({ ...editForm, proxy_url: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono placeholder:text-text-muted"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Bobot Alokasi</label>
              <input
                type="number"
                min="1"
                max="1000"
                value={editForm.weight}
                onChange={(e) => setEditForm({ ...editForm, weight: parseInt(e.target.value) || 100 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Wilayah (Region)</label>
              <input
                type="text"
                value={editForm.region}
                onChange={(e) => setEditForm({ ...editForm, region: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
              />
            </div>
          </div>
          <div className="flex items-center gap-2 pt-2">
            <input
              type="checkbox"
              id="editPoolEnabled"
              checked={editForm.enabled}
              onChange={(e) => setEditForm({ ...editForm, enabled: e.target.checked })}
              className="rounded border-border bg-bg-surface-2 text-accent focus:ring-accent"
            />
            <label htmlFor="editPoolEnabled" className="text-white font-semibold cursor-pointer">
              Aktifkan pool ini untuk menerima lalu lintas keluar
            </label>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Perubahan
          </Button>
        </form>
      </Modal>
    </div>
  );
};
