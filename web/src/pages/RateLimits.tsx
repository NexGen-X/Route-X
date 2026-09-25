import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { RateLimit, APIKey } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Tooltip } from '../components/common/Tooltip';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { Gauge, Plus, Trash2, Sparkles } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export const RateLimits: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [limits, setLimits] = useState<RateLimit[]>([]);
  const [apiKeys, setApiKeys] = useState<APIKey[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newLimit, setNewLimit] = useState({
    scope: 'api_key',
    scope_id: '',
    requests_per_minute: 60,
    tokens_per_minute: 100000,
    requests_per_second: 10,
  });

  const loadLimits = async () => {
    setIsLoading(true);
    try {
      const [lRes, kRes] = await Promise.all([
        api.rateLimits.list(),
        api.apiKeys.list().catch(() => ({ items: [] as APIKey[] })),
      ]);
      setLimits(lRes.items || []);
      setApiKeys(kRes.items || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadLimits();
  }, []);

  const getScopeDisplay = (scope: string, scopeId?: string) => {
    if (scope === 'global' || scopeId === 'global' || !scopeId) {
      return { label: 'Global Gateway', isRaw: false };
    }
    if (scope === 'api_key') {
      const key = apiKeys.find((k) => k.id === scopeId);
      return { label: key ? key.name : `${scopeId.slice(0, 8)}...`, isRaw: !key };
    }
    if (scope === 'ip') {
      return { label: `IP: ${scopeId}`, isRaw: false };
    }
    return { label: scopeId, isRaw: true };
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    const num = (v: number) => (Number.isFinite(v) ? v : 0);
    const rpm = num(newLimit.requests_per_minute);
    const tpm = num(newLimit.tokens_per_minute);
    const rps = num(newLimit.requests_per_second);
    if (rpm < 0 || tpm < 0 || rps < 0) {
      toast.error('RPM/TPM/RPS tidak boleh negatif.');
      return;
    }
    if (rpm === 0 && tpm === 0 && rps === 0) {
      toast.error('Isi minimal satu batas (RPM/TPM/RPS) lebih dari 0.');
      return;
    }
    const finalScopeId = newLimit.scope === 'global' ? 'global' : newLimit.scope_id.trim();
    if (!finalScopeId) {
      toast.error('Pilih atau masukkan target scope.');
      return;
    }
    try {
      await api.rateLimits.create({
        ...newLimit,
        scope_id: finalScopeId,
        requests_per_minute: rpm,
        tokens_per_minute: tpm,
        requests_per_second: rps,
      });
      setIsCreateOpen(false);
      toast.success('Aturan batas laju berhasil dibuat');
      loadLimits();
    } catch (err) {
      toast.error('Gagal membuat limit: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDelete = async (id: string) => {
    const confirmed = await confirmModal({
      title: 'Hapus Aturan Rate Limit',
      message: 'Apakah Anda yakin ingin menghapus aturan pembatasan laju ini? Kebijakan fallback default akan berlaku.',
      danger: true,
      confirmText: 'Ya, Hapus Aturan',
    });
    if (!confirmed) return;

    try {
      await api.rateLimits.delete(id);
      toast.success('Aturan batas laju berhasil dihapus');
      loadLimits();
    } catch (err) {
      toast.error('Gagal menghapus: ' + (err instanceof Error ? err.message : String(err)));
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
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
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
              <Gauge className="w-6 h-6" />
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
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4 text-black" />}
            >
              Tambah Rate Limit Pertama
            </Button>
          </div>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {limits.map((l) => (
            <Card key={l.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center">
                    <Gauge className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white uppercase">{l.scope}</h4>
                    <Tooltip content={`Scope ID: ${l.scope_id}`} position="bottom">
                      <span className="text-[11px] text-text-muted font-mono cursor-default">
                        {getScopeDisplay(l.scope, l.scope_id).label}
                      </span>
                    </Tooltip>
                  </div>
                </div>
                <Badge variant={l.enabled ? 'success' : 'neutral'}>
                  {l.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Requests Per Minute (RPM)</span>
                  <span className="font-mono text-white font-semibold">
                    {l.requests_per_minute ? `${l.requests_per_minute.toLocaleString()} req/m` : '-'}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Tokens Per Minute (TPM)</span>
                  <span className="font-mono text-accent">
                    {l.tokens_per_minute ? `${l.tokens_per_minute.toLocaleString()} tok/m` : '-'}
                  </span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Requests Per Second (RPS)</span>
                  <span className="font-mono text-text-secondary">
                    {l.requests_per_second ? `${l.requests_per_second} req/s` : '-'}
                  </span>
                </div>
                <div className="flex justify-between py-1">
                  <span className="text-text-muted">Batas Harian</span>
                  <span className="font-mono text-text-secondary">
                    {l.daily_request_limit ? `${l.daily_request_limit.toLocaleString()} req/hari` : '-'}
                  </span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex justify-end">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleDelete(l.id)}
                icon={<Trash2 className="w-3.5 h-3.5" />}
              >
                Hapus
              </Button>
            </div>
          </Card>
        ))}
        </div>
      )}

      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Konfigurasi Rate Limit Baru"
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-rate-limit-form">
              Simpan Rate Limit
            </Button>
          </>
        }
      >
        <form id="create-rate-limit-form" noValidate onSubmit={handleCreate} className="space-y-4 text-xs">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Select
              label="Cakupan (Scope)"
              value={newLimit.scope}
              onChange={(val) => setNewLimit({ ...newLimit, scope: val, scope_id: val === 'global' ? 'global' : '' })}
              options={[
                { value: 'api_key', label: 'Per Kunci API', description: 'Limit berbasis token API klien individual' },
                { value: 'ip', label: 'Per Alamat IP Klien', description: 'Limit berbasis IP publik pemanggil' },
                { value: 'global', label: 'Global Gateway', description: 'Limit total seluruh gateway' },
              ]}
            />
            {newLimit.scope === 'global' ? (
              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5">Scope ID / Target</label>
                <div className="px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-text-muted font-mono">
                  Seluruh Gateway (Global)
                </div>
              </div>
            ) : newLimit.scope === 'api_key' ? (
              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5">Target Kunci API *</label>
                {apiKeys.length > 0 ? (
                  <Select
                    value={newLimit.scope_id}
                    onChange={(val) => setNewLimit({ ...newLimit, scope_id: val })}
                    options={[
                      { value: '', label: '-- Pilih Kunci API --' },
                      ...apiKeys.map((k) => ({
                        value: k.id,
                        label: `${k.name} (${k.masked_key || k.id.slice(0, 8)})`,
                      })),
                    ]}
                  />
                ) : (
                  <input
                    type="text"
                    required
                    placeholder="Masukkan UUID Kunci API..."
                    value={newLimit.scope_id}
                    onChange={(e) => setNewLimit({ ...newLimit, scope_id: e.target.value })}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                  />
                )}
              </div>
            ) : (
              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5">Alamat IP Target *</label>
                <input
                  type="text"
                  required
                  placeholder="Contoh: 192.168.1.1 atau 0.0.0.0/0"
                  value={newLimit.scope_id}
                  onChange={(e) => setNewLimit({ ...newLimit, scope_id: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                />
              </div>
            )}
          </div>

          {/* Quick Rate Limit Presets */}
          <div className="p-3 rounded-xl bg-accent/5 border border-accent/20 space-y-2">
            <span className="text-[11px] font-semibold text-accent flex items-center gap-1.5">
              <Sparkles className="w-3.5 h-3.5" />
              Template Proteksi Cepat (Rekomendasi Pemakaian)
            </span>
            <div className="grid grid-cols-3 gap-2">
              <button
                type="button"
                onClick={() =>
                  setNewLimit((prev) => ({
                    ...prev,
                    requests_per_minute: 60,
                    tokens_per_minute: 100000,
                    requests_per_second: 5,
                  }))
                }
                className="px-2 py-1.5 rounded-lg bg-bg-surface-2 border border-border hover:border-emerald-500/50 hover:bg-emerald-500/10 text-center transition-all cursor-pointer group"
              >
                <span className="block text-xs font-bold text-white group-hover:text-emerald-300">60 RPM</span>
                <span className="block text-[10px] text-text-muted mt-0.5">Dev / Sandbox</span>
              </button>
              <button
                type="button"
                onClick={() =>
                  setNewLimit((prev) => ({
                    ...prev,
                    requests_per_minute: 300,
                    tokens_per_minute: 500000,
                    requests_per_second: 15,
                  }))
                }
                className="px-2 py-1.5 rounded-lg bg-bg-surface-2 border border-border hover:border-sky-500/50 hover:bg-sky-500/10 text-center transition-all cursor-pointer group"
              >
                <span className="block text-xs font-bold text-white group-hover:text-sky-300">300 RPM</span>
                <span className="block text-[10px] text-text-muted mt-0.5">Standar IDE</span>
              </button>
              <button
                type="button"
                onClick={() =>
                  setNewLimit((prev) => ({
                    ...prev,
                    requests_per_minute: 1200,
                    tokens_per_minute: 2000000,
                    requests_per_second: 50,
                  }))
                }
                className="px-2 py-1.5 rounded-lg bg-bg-surface-2 border border-border hover:border-purple-500/50 hover:bg-purple-500/10 text-center transition-all cursor-pointer group"
              >
                <span className="block text-xs font-bold text-white group-hover:text-purple-300">1200 RPM</span>
                <span className="block text-[10px] text-text-muted mt-0.5">High Load</span>
              </button>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Req / Menit (RPM)</label>
              <input
                type="number"
                min={0}
                value={newLimit.requests_per_minute}
                onChange={(e) => setNewLimit({ ...newLimit, requests_per_minute: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Token / Menit (TPM)</label>
              <input
                type="number"
                min={0}
                value={newLimit.tokens_per_minute}
                onChange={(e) => setNewLimit({ ...newLimit, tokens_per_minute: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Req / Detik (RPS)</label>
              <input
                type="number"
                min={0}
                value={newLimit.requests_per_second}
                onChange={(e) => setNewLimit({ ...newLimit, requests_per_second: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
          </div>
        </form>
      </Modal>
    </div>
  );
};
