import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { EgressPool } from '../types';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { Network, Plus, RefreshCw, Zap } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import {
  EgressProbeFeedback,
  EgressPoolCard,
  CreateEgressDrawer,
  EditEgressDrawer,
  CloudProvisionDrawer,
  type EgressProbeFeedbackData,
} from '../components/egress';

export const Egress: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [pools, setPools] = useState<EgressPool[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isProvisionOpen, setIsProvisionOpen] = useState(false);
  const [editingPool, setEditingPool] = useState<EgressPool | null>(null);

  // State hasil uji koneksi live probe
  const [testingId, setTestingId] = useState<string | null>(null);
  const [probeFeedback, setProbeFeedback] = useState<EgressProbeFeedbackData | null>(null);

  const loadPools = async () => {
    setIsLoading(true);
    try {
      const res = await api.egress.list();
      setPools(res.items || []);
      setLoadError(null);
    } catch (err: unknown) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadPools();
  }, []);

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
      void loadPools();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      toast.error('Gagal menghapus egress pool: ' + message);
    }
  };

  const handleTestProbe = async (id: string) => {
    setTestingId(id);
    setProbeFeedback(null);
    try {
      const res = await api.egress.test(id);
      setProbeFeedback({ poolId: id, result: res });
      toast.success(
        `Uji koneksi egress sukses: ${res.latency_ms} ms (Exit IP: ${res.exit_ip || 'n/a'})`,
        'Egress Sehat'
      );
      void loadPools();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      setProbeFeedback({
        poolId: id,
        result: {
          status: 'unhealthy',
          latency_ms: 0,
          checked_at: new Date().toISOString(),
          message,
        },
      });
      toast.error('Uji koneksi egress gagal: ' + message);
    } finally {
      setTestingId(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Halaman */}
      <PageHeader
        title="Egress Proxy Pools"
        actions={
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => void loadPools()} isLoading={isLoading}>
              <RefreshCw className="w-3.5 h-3.5" />
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setIsProvisionOpen(true)}
              icon={<Zap className="w-3.5 h-3.5 text-accent" />}
              className="border-accent/40 hover:border-accent"
            >
              Auto-Deploy Cloud Relay
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
      <EgressProbeFeedback feedback={probeFeedback} onDismiss={() => setProbeFeedback(null)} />

      {/* Grid Kartu Egress Pool */}
      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="p-6 space-y-4 animate-pulse bg-bg-surface/40 backdrop-blur-md shadow-lg rounded-2xl border border-white/5 transition-all">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-bg-surface-2/60" />
                <div className="space-y-1.5 flex-1">
                  <div className="h-4 bg-bg-surface-2/60 rounded w-1/2" />
                  <div className="h-3 bg-bg-surface-2/60 rounded w-1/3" />
                </div>
              </div>
              <div className="space-y-2 mt-2">
                <div className="h-3 bg-bg-surface-2/60 rounded w-3/4" />
                <div className="h-3 bg-bg-surface-2/60 rounded w-1/2" />
              </div>
            </div>
          ))}
        </div>
      ) : pools.length === 0 ? (
        <div className="p-16 text-center bg-bg-surface/40 backdrop-blur-md shadow-lg rounded-2xl border border-white/5 flex flex-col items-center justify-center transition-all">
          <div className="w-14 h-14 rounded-full bg-accent/10 text-accent flex items-center justify-center mx-auto mb-5 ring-1 ring-accent/30 shadow-[0_0_15px_rgba(var(--color-accent),0.2)]">
            <Network className="w-7 h-7" />
          </div>
          <h3 className="text-lg font-semibold text-white tracking-wide">Belum Ada Egress Proxy Pool</h3>
          <p className="text-sm text-text-secondary mt-2 max-w-md mx-auto leading-relaxed">
            Tambahkan proxy keluar (seperti BrightData, Smartproxy, atau VPS) untuk menyembunyikan IP gateway atau bypass pemblokiran wilayah AI.
          </p>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="mt-6 hover:shadow-lg hover:shadow-accent/20 transition-all duration-300"
          >
            Tambah Egress Pool Sekarang
          </Button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {pools.map((p) => (
            <EgressPoolCard
              key={p.id}
              pool={p}
              isTesting={testingId === p.id}
              onTestProbe={handleTestProbe}
              onEdit={setEditingPool}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}

      {/* Modal Tambah Egress Pool */}
      <CreateEgressDrawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        onSuccess={() => void loadPools()}
      />

      {/* Modal Edit Egress Pool */}
      <EditEgressDrawer
        pool={editingPool}
        onClose={() => setEditingPool(null)}
        onSuccess={() => void loadPools()}
      />

      {/* Drawer Auto-Deploy Cloud Relay (Cloudflare & Deno) */}
      <CloudProvisionDrawer
        isOpen={isProvisionOpen}
        onClose={() => setIsProvisionOpen(false)}
        onSuccess={() => void loadPools()}
      />
    </div>
  );
};
