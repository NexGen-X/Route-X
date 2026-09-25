import React, { useState, useEffect } from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { Eye, EyeOff } from 'lucide-react';
import { api } from '../../api/client';
import { useToast } from '../../context/ToastContext';

export interface CreateEgressFormData {
  name: string;
  kind: string;
  proxy_url: string;
  weight: number;
  region: string;
}

export interface CreateEgressDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

const INITIAL_FORM: CreateEgressFormData = {
  name: '',
  kind: 'socks5',
  proxy_url: '',
  weight: 100,
  region: 'auto',
};

export const CreateEgressDrawer: React.FC<CreateEgressDrawerProps> = ({
  isOpen,
  onClose,
  onSuccess,
}) => {
  const { toast } = useToast();
  const [formData, setFormData] = useState<CreateEgressFormData>(INITIAL_FORM);
  const [showProxyUrl, setShowProxyUrl] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setFormData(INITIAL_FORM);
      setShowProxyUrl(false);
    }
  }, [isOpen]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    try {
      await api.egress.create(formData);
      toast.success('Egress proxy pool baru berhasil ditambahkan.', 'Egress Dibuat');
      setFormData(INITIAL_FORM);
      onSuccess();
      onClose();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      toast.error('Gagal membuat egress pool: ' + message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    setFormData(INITIAL_FORM);
    setShowProxyUrl(false);
    onClose();
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={handleClose}
      title="Tambah Egress Proxy Pool"
      footer={
        <>
          <Button variant="ghost" onClick={handleClose} disabled={isSubmitting}>
            Batal
          </Button>
          <Button variant="primary" type="submit" form="create-egress-form" isLoading={isSubmitting}>
            Simpan Egress Pool
          </Button>
        </>
      }
    >
      <form id="create-egress-form" noValidate onSubmit={handleSubmit} className="space-y-4 text-xs">
        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Pool *</label>
          <input
            type="text"
            required
            placeholder="residential-sg-1 atau my-proxy"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
          />
        </div>
        <Select
          label="Protokol Proxy"
          value={formData.kind}
          onChange={(val) => setFormData({ ...formData, kind: val })}
          options={[
            { value: 'socks5', label: 'SOCKS5 Proxy', description: 'Protokol raw socket tcp/udp dengan stealth transport' },
            { value: 'http', label: 'HTTP Proxy', description: 'Standar http proxy forwarder' },
            { value: 'https', label: 'HTTPS Proxy', description: 'Http proxy terenkripsi TLS' },
          ]}
        />
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <label className="text-xs font-medium text-text-secondary">Proxy URL Lengkap *</label>
            <button
              type="button"
              onClick={() => setShowProxyUrl(!showProxyUrl)}
              className="text-[11px] text-text-muted hover:text-white flex items-center gap-1 focus:outline-none cursor-pointer"
            >
              {showProxyUrl ? (
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
            type={showProxyUrl ? 'text' : 'password'}
            required
            placeholder="socks5://user:pass@host:1080"
            value={formData.proxy_url}
            onChange={(e) => setFormData({ ...formData, proxy_url: e.target.value })}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Bobot Alokasi</label>
            <input
              type="number"
              min="1"
              max="1000"
              value={formData.weight}
              onChange={(e) => setFormData({ ...formData, weight: parseInt(e.target.value) || 100 })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Wilayah (Region)</label>
            <input
              type="text"
              placeholder="auto / ap-southeast-1"
              value={formData.region}
              onChange={(e) => setFormData({ ...formData, region: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
        </div>
      </form>
    </Drawer>
  );
};
