import React, { useState, useEffect } from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import { Checkbox } from '../common/Checkbox';
import { Eye, EyeOff } from 'lucide-react';
import { api } from '../../api/client';
import { useToast } from '../../context/ToastContext';
import type { EgressPool } from '../../types';

export interface EditEgressFormData {
  name: string;
  kind: string;
  proxy_url: string;
  weight: number;
  region: string;
  enabled: boolean;
}

export interface EditEgressDrawerProps {
  pool: EgressPool | null;
  isOpen?: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

const INITIAL_EDIT_FORM: EditEgressFormData = {
  name: '',
  kind: 'socks5',
  proxy_url: '',
  weight: 100,
  region: 'auto',
  enabled: true,
};

export const EditEgressDrawer: React.FC<EditEgressDrawerProps> = ({
  pool,
  isOpen,
  onClose,
  onSuccess,
}) => {
  const { toast } = useToast();
  const isDrawerOpen = isOpen !== undefined ? isOpen : Boolean(pool);
  const [editForm, setEditForm] = useState<EditEgressFormData>(INITIAL_EDIT_FORM);
  const [showProxyUrl, setShowProxyUrl] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (pool) {
      setEditForm({
        name: pool.name,
        kind: pool.kind || 'socks5',
        proxy_url: '',
        weight: pool.weight ?? 100,
        region: pool.region || 'auto',
        enabled: pool.enabled,
      });
      setShowProxyUrl(false);
    }
  }, [pool]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!pool) return;

    setIsSubmitting(true);
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

      await api.egress.update(pool.id, payload);
      toast.success('Egress proxy pool berhasil diperbarui.', 'Perubahan Disimpan');
      onSuccess();
      onClose();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      toast.error('Gagal memperbarui egress pool: ' + message);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Drawer
      isOpen={isDrawerOpen}
      onClose={onClose}
      title="Edit Egress Proxy Pool"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={isSubmitting}>
            Batal
          </Button>
          <Button variant="primary" type="submit" form="edit-egress-form" isLoading={isSubmitting}>
            Simpan Perubahan
          </Button>
        </>
      }
    >
      <form id="edit-egress-form" noValidate onSubmit={handleSubmit} className="space-y-4 text-xs">
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
  );
};
