import React, { useState, useEffect, useRef } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
// import type { APIKey } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { PageHeader } from '../components/common/PageHeader';
import { KeyRound, Plus, RotateCw, Trash2, Copy, Check } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';
import { copyTextToClipboard } from '../utils/clipboard';

export const APIKeys: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const queryClient = useQueryClient();
  const { data: keysData, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['apiKeys'],
    queryFn: () => api.apiKeys.list(),
  });
  const keys = keysData?.items || [];
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newKey, setNewKey] = useState({
    name: '',
    rpm_limit: 120,
    tpm_limit: 100000,
  });
  const [createdRawKey, setCreatedRawKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [isAllowedOpen, setIsAllowedOpen] = useState(false);
  const [selectedKey, setSelectedKey] = useState<any>(null);
  const [allowedModels, setAllowedModels] = useState<string>('');
  const [allowedProviders, setAllowedProviders] = useState<string>('');

  // Timer indikator salin; dibatalkan saat unmount agar tidak ada setState basi.
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copyTimerRef.current) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);



  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newKey.name.trim()) {
      toast.error('Nama kunci wajib diisi.');
      return;
    }
    if (newKey.rpm_limit < 0 || newKey.tpm_limit < 0) {
      toast.error('Batas RPM/TPM tidak boleh negatif (0 = tanpa batas).');
      return;
    }
    try {
      const res = await api.apiKeys.create({
        name: newKey.name.trim(),
        rate_limit_rpm: newKey.rpm_limit || undefined,
        rate_limit_tpm: newKey.tpm_limit || undefined,
        scopes: ['inference'],
        model_ids: [],
        provider_ids: [],
      });
      setIsCreateOpen(false);
      setCreatedRawKey(res.raw_key || null);
      toast.success('Kunci API baru berhasil dibuat.', 'API Key Dibuat');
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: any) {
      toast.error('Gagal membuat API key: ' + (err.message || err));
    }
  };

  const handleRotate = async (id: string) => {
    const ok = await confirmModal({
      title: 'Putar Kunci API?',
      message: 'Kunci lama akan langsung tidak berlaku dan kunci baru akan diterbitkan seketika.',
      confirmText: 'Putar Kunci',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;

    try {
      const res = await api.apiKeys.rotate(id);
      setCreatedRawKey(res.raw_key || null);
      toast.success('Kunci API berhasil diputar.', 'Kunci Diperbarui');
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: any) {
      toast.error('Gagal rotasi key: ' + (err.message || err));
    }
  };

  const handleRevoke = async (id: string) => {
    const ok = await confirmModal({
      title: 'Cabut Kunci API Permanen?',
      message: 'Cabut kunci API ini secara permanen? Seluruh klien dan skrip yang menggunakannya akan langsung ditolak.',
      confirmText: 'Ya, Cabut Kunci',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;

    try {
      await api.apiKeys.revoke(id);
      toast.success('Kunci API berhasil dicabut secara permanen.', 'Kunci Dicabut');
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: any) {
      toast.error('Gagal mencabut key: ' + (err.message || err));
    }
  };

  const copyToClipboard = async (text: string) => {
    try {
      await copyTextToClipboard(text);
      setCopied(true);
      toast.success('Kunci API disalin ke clipboard.');
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
      copyTimerRef.current = setTimeout(() => {
        copyTimerRef.current = null;
        setCopied(false);
      }, 2000);
    } catch (err) {
      toast.error('Gagal menyalin kunci API: ' + (err instanceof Error ? err.message : String(err)));
    }
  };


  const handleOpenAllowed = (k: any) => {
    setSelectedKey(k);
    setAllowedModels((k.model_ids || []).join(', '));
    setAllowedProviders((k.provider_ids || []).join(', '));
    setIsAllowedOpen(true);
  };

  const handleSaveAllowed = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const m_ids = allowedModels.split(',').map(s => s.trim()).filter(Boolean);
      const p_ids = allowedProviders.split(',').map(s => s.trim()).filter(Boolean);
      await api.apiKeys.setAllowed(selectedKey.id, { model_ids: m_ids, provider_ids: p_ids });
      toast.success('Allowed Models & Providers berhasil diperbarui.');
      setIsAllowedOpen(false);
      queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
    } catch (err: any) {
      toast.error('Gagal menyimpan allowed list: ' + (err.message || err));
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Client API Keys"
        description="Kunci akses klien dengan hashing HMAC-SHA256 ber-pepper server dan batas kuota mandiri."
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="w-full sm:w-auto justify-center"
          >
            Buat API Key
          </Button>
        }
      />

      {isError ? (
        <QueryError
          message={error instanceof Error ? error.message : 'Daftar kunci API tidak tersedia.'}
          onRetry={() => void refetch()}
        />
      ) : isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(3)].map((_, i) => (
            <Card key={i} className="p-5 animate-pulse space-y-4">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-bg-surface-2" />
                <div className="space-y-2 flex-1">
                  <div className="h-4 bg-bg-surface-2 rounded w-24" />
                  <div className="h-3 bg-bg-surface-2 rounded w-32" />
                </div>
              </div>
              <div className="space-y-2 pt-3 border-t border-border/40">
                <div className="h-3 bg-bg-surface-2 rounded w-full" />
                <div className="h-3 bg-bg-surface-2 rounded w-3/4" />
              </div>
              <div className="pt-3 border-t border-border flex justify-between">
                <div className="h-8 bg-bg-surface-2 rounded w-20" />
                <div className="h-8 bg-bg-surface-2 rounded w-20" />
              </div>
            </Card>
          ))}
        </div>
      ) : keys.length === 0 ? (
        <Card className="py-12 px-6 text-center">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center mx-auto shadow-inner">
              <KeyRound className="w-6 h-6" />
            </div>
            <div>
              <h3 className="text-base font-bold text-white">Belum Ada Kunci API Klien</h3>
              <p className="text-xs text-text-muted mt-1 leading-relaxed">
                Buat kunci API pertama untuk menghubungkan editor atau aplikasi Anda (Cursor, Cline, Open WebUI, LibreChat, atau skrip personal) ke Route-X Gateway.
              </p>
            </div>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreateOpen(true)}
              icon={<Plus className="w-4 h-4 text-black" />}
            >
              Buat Kunci API Pertama
            </Button>
          </div>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {keys.map((k) => (
            <Card key={k.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-accent/10 border border-accent/20 text-accent flex items-center justify-center">
                    <KeyRound className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{k.name}</h4>
                    <span className="text-[11px] text-accent font-mono">{k.masked_key}</span>
                  </div>
                </div>
                <Badge variant={k.enabled ? 'success' : 'neutral'}>
                  {k.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Batas RPM / TPM</span>
                  <span className="font-mono text-white">{(k as any).rate_limit_rpm ?? k.rpm_limit ?? '∞'} RPM / {(k as any).rate_limit_tpm ?? k.tpm_limit ?? '∞'} TPM</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Terakhir Digunakan</span>
                  <span className="font-mono text-text-secondary">
                    {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : 'Belum pernah'}
                  </span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center gap-2 flex-wrap justify-between">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleOpenAllowed(k)}
              >
                Allowed Models/IPs
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleRotate(k.id)}
                icon={<RotateCw className="w-3.5 h-3.5" />}
              >
                Rotasi
              </Button>
              <Button
                variant="danger"
                size="sm"
                onClick={() => handleRevoke(k.id)}
                icon={<Trash2 className="w-3.5 h-3.5" />}
              >
                Cabut
              </Button>
            </div>
          </Card>
        ))}
        </div>
      )}

      {/* Modal Generate Key Result */}
      {createdRawKey && (
        <Modal
          isOpen={true}
          onClose={() => setCreatedRawKey(null)}
          title="Kunci API Berhasil Dibuat"
          subtitle="Simpan kunci ini sekarang. Kunci tidak akan pernah ditampilkan lagi!"
        >
          <div className="space-y-4">
            <div className="p-3 bg-bg-surface-2 rounded-inner border border-accent/30 flex items-center justify-between">
              <span className="font-mono text-xs text-accent break-all select-all font-semibold">
                {createdRawKey}
              </span>
              <button
                onClick={() => copyToClipboard(createdRawKey)}
                aria-label="Salin kunci API ke clipboard"
                title="Salin kunci API ke clipboard"
                className="p-2 text-text-muted hover:text-accent rounded-nav ml-2 flex-shrink-0 cursor-pointer"
              >
                {copied ? <Check className="w-4 h-4 text-accent" aria-hidden="true" /> : <Copy className="w-4 h-4" aria-hidden="true" />}
              </button>
            </div>
            <Button
              variant="primary"
              size="md"
              onClick={() => setCreatedRawKey(null)}
              className="w-full"
            >
              Saya Sudah Menyimpan Kunci Ini
            </Button>
          </div>
        </Modal>
      )}

      {/* Modal Create Key */}
      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Buat Kunci API Klien Baru"
        subtitle="Hasilkan token otentikasi format sk_live_..."
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label htmlFor="api-key-name" className="block font-semibold text-text-secondary uppercase mb-1">Nama Kunci</label>
            <input
              id="api-key-name"
              name="name"
              type="text"
              required
              placeholder="Backend Production Key"
              value={newKey.name}
              onChange={(e) => setNewKey({ ...newKey, name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label htmlFor="api-key-rpm" className="block font-semibold text-text-secondary uppercase mb-1">Batas RPM (0 = tanpa batas)</label>
              <input
                id="api-key-rpm"
                name="rpm_limit"
                type="number"
                min={0}
                value={newKey.rpm_limit}
                onChange={(e) => setNewKey({ ...newKey, rpm_limit: Math.max(0, parseInt(e.target.value) || 0) })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label htmlFor="api-key-tpm" className="block font-semibold text-text-secondary uppercase mb-1">Batas TPM (0 = tanpa batas)</label>
              <input
                id="api-key-tpm"
                name="tpm_limit"
                type="number"
                min={0}
                value={newKey.tpm_limit}
                onChange={(e) => setNewKey({ ...newKey, tpm_limit: Math.max(0, parseInt(e.target.value) || 0) })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Hasilkan Kunci API
          </Button>
        </form>
      </Modal>

      {/* Modal Edit Allowed */}
      <Modal
        isOpen={isAllowedOpen}
        onClose={() => setIsAllowedOpen(false)}
        title="Edit Allowed Models & Providers"
        subtitle="Batasi kunci ini hanya untuk model atau provider tertentu (pisahkan dengan koma)"
      >
        <form onSubmit={handleSaveAllowed} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Allowed Models (ID)</label>
            <input
              type="text"
              placeholder="gpt-4o, claude-3"
              value={allowedModels}
              onChange={(e) => setAllowedModels(e.target.value)}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Allowed Providers (ID)</label>
            <input
              type="text"
              placeholder="openai, anthropic"
              value={allowedProviders}
              onChange={(e) => setAllowedProviders(e.target.value)}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Perubahan
          </Button>
        </form>
      </Modal>
    </div>

  );
};
