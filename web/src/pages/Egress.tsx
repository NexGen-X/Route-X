import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { EgressPool } from '../types';
import { Card } from '../components/common/Card';
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
            Tambahkan proxy keluar (seperti BrightData, Smartproxy, atau VPS) untuk menyembunyikan IP gateway atau bypass pemblokiran wilayah AI.
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
