import React, { useEffect, useState, useCallback } from 'react';
import { api } from '../api/client';
import type { RateLimit, APIKey, Model } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { Gauge, Plus } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import {
  RateLimitCard,
  CreateRateLimitDrawer,
  getScopeDisplay,
  getErrorMessage,
  type RateLimitFormData,
} from '../components/budgets';

export const RateLimits: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [limits, setLimits] = useState<RateLimit[]>([]);
  const [apiKeys, setApiKeys] = useState<APIKey[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [initialValues, setInitialValues] = useState<Partial<RateLimitFormData> | undefined>(undefined);

  const loadLimits = useCallback(async () => {
    setIsLoading(true);
    try {
      const [lRes, kRes, mRes] = await Promise.all([
        api.rateLimits.list(),
        api.apiKeys.list().catch(() => ({ items: [] as APIKey[] })),
        api.models.list().catch(() => ({ items: [] as Model[] })),
      ]);
      setLimits(lRes.items || []);
      setApiKeys(kRes.items || []);
      setModels(mRes.items || []);
      setLoadError(null);
    } catch (err: unknown) {
      setLoadError(getErrorMessage(err));
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadLimits();
  }, [loadLimits]);

  const handleCreate = async (formData: RateLimitFormData) => {
    const num = (v: number) => (Number.isFinite(v) ? v : 0);
    const rpm = num(formData.requests_per_minute);
    const tpm = num(formData.tokens_per_minute);
    const rps = num(formData.requests_per_second);
    if (rpm < 0 || tpm < 0 || rps < 0) {
      toast.error('RPM/TPM/RPS tidak boleh negatif.');
      return;
    }
    if (rpm === 0 && tpm === 0 && rps === 0) {
      toast.error('Isi minimal satu batas (RPM/TPM/RPS) lebih dari 0.');
      return;
    }
    const finalScopeId = formData.scope === 'global' ? 'global' : formData.scope_id.trim();
    if (!finalScopeId) {
      toast.error('Pilih atau masukkan target scope.');
      return;
    }
    try {
      await api.rateLimits.create({
        ...formData,
        scope_id: finalScopeId,
        requests_per_minute: rpm,
        tokens_per_minute: tpm,
        requests_per_second: rps,
      });
      setIsCreateOpen(false);
      setInitialValues(undefined);
      toast.success('Aturan batas laju berhasil dibuat');
      void loadLimits();
    } catch (err: unknown) {
      toast.error('Gagal membuat limit: ' + getErrorMessage(err));
    }
  };

  const handleDelete = async (id: string) => {
    const confirmed = await confirmModal({
      title: 'Hapus Aturan Rate Limit',
      message: 'Apakah Anda yakin ingin menghapus aturan pembatasan laju ini? Kebijakan fallback default akan berlaku.',
      danger: true,
      confirmText: 'Ya, Hapus Aturan',
      cancelText: 'Batal',
    });
    if (!confirmed) return;

    try {
      await api.rateLimits.delete(id);
      toast.success('Aturan batas laju berhasil dihapus');
      void loadLimits();
    } catch (err: unknown) {
      toast.error('Gagal menghapus: ' + getErrorMessage(err));
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Rate Limits"
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={() => {
              setInitialValues(undefined);
              setIsCreateOpen(true);
            }}
            icon={<Plus className="w-4 h-4" aria-hidden="true" />}
            className="w-full sm:w-auto justify-center"
          >
            Tambah Rate Limit
          </Button>
        }
      />

      {loadError && <QueryError message={loadError} onRetry={() => void loadLimits()} />}

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
      ) : limits.length === 0 && !loadError ? (
        <Card className="py-12 px-6 text-center">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center mx-auto shadow-inner">
              <Gauge className="w-6 h-6" aria-hidden="true" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Belum Ada Aturan Rate Limit</h3>
              <p className="text-xs text-text-muted mt-1 leading-relaxed">
                Tetapkan batasan laju kuota inferensi (RPM, TPM, atau RPS) per Kunci API, Alamat IP Klien, atau Model untuk mencegah lonjakan beban yang tidak diinginkan.
              </p>
            </div>
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                setInitialValues(undefined);
                setIsCreateOpen(true);
              }}
              icon={<Plus className="w-4 h-4 text-black" aria-hidden="true" />}
            >
              Tambah Rate Limit Pertama
            </Button>
          </div>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {limits.map((l) => (
            <RateLimitCard
              key={l.id}
              limit={l}
              scopeDisplay={getScopeDisplay(l.scope, l.scope_id, apiKeys, models)}
              onDelete={handleDelete}
            />
          ))}
        </div>
      )}

      {/* Drawer Tambah Rate Limit */}
      <CreateRateLimitDrawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        onSubmit={handleCreate}
        apiKeys={apiKeys}
        initialValues={initialValues}
      />
    </div>
  );
};
