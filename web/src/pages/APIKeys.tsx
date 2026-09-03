import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { APIKey } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { KeyRound, Plus, RotateCw, Trash2, Copy, Check } from 'lucide-react';

export const APIKeys: React.FC = () => {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newKey, setNewKey] = useState({
    name: '',
    rpm_limit: 120,
    tpm_limit: 100000,
  });
  const [createdRawKey, setCreatedRawKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const loadKeys = async () => {
    try {
      const res = await api.apiKeys.list();
      setKeys(res.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadKeys();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const res = await api.apiKeys.create({
        ...newKey,
        scopes: ['chat:completions', 'embeddings'],
        allowed_models: [],
        allowed_providers: [],
      });
      setIsCreateOpen(false);
      setCreatedRawKey(res.raw_key || null);
      loadKeys();
    } catch (err) {
      alert('Gagal membuat API key: ' + err);
    }
  };

  const handleRotate = async (id: string) => {
    if (!confirm('Putar (rotate) kunci API ini? Kunci lama akan langsung tidak berlaku.')) return;
    try {
      const res = await api.apiKeys.rotate(id);
      setCreatedRawKey(res.raw_key || null);
      loadKeys();
    } catch (err) {
      alert('Gagal rotasi key: ' + err);
    }
  };

  const handleRevoke = async (id: string) => {
    if (!confirm('Cabut (revoke) kunci API ini secara permanen?')) return;
    try {
      await api.apiKeys.revoke(id);
      loadKeys();
    } catch (err) {
      alert('Gagal mencabut key: ' + err);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Client API Keys</h2>
          <p className="text-xs text-text-secondary mt-1">
            Kunci akses klien dengan hashing HMAC-SHA256 ber-pepper server dan batas kuota mandiri.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Buat API Key
        </Button>
      </div>

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
                  <span className="font-mono text-white">{k.rpm_limit ?? '∞'} RPM / {k.tpm_limit ?? '∞'} TPM</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Terakhir Digunakan</span>
                  <span className="font-mono text-text-secondary">
                    {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : 'Belum pernah'}
                  </span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
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
                className="p-2 text-text-muted hover:text-accent rounded-nav ml-2 flex-shrink-0"
              >
                {copied ? <Check className="w-4 h-4 text-accent" /> : <Copy className="w-4 h-4" />}
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
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Kunci</label>
            <input
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
              <label className="block font-semibold text-text-secondary uppercase mb-1">Batas RPM</label>
              <input
                type="number"
                value={newKey.rpm_limit}
                onChange={(e) => setNewKey({ ...newKey, rpm_limit: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Batas TPM</label>
              <input
                type="number"
                value={newKey.tpm_limit}
                onChange={(e) => setNewKey({ ...newKey, tpm_limit: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Hasilkan Kunci API
          </Button>
        </form>
      </Modal>
    </div>
  );
};
