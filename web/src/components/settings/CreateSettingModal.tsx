import React from 'react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { Save } from 'lucide-react';
import type { CreateSettingModalProps } from './types';

export const CreateSettingModal: React.FC<CreateSettingModalProps> = ({
  isOpen,
  isCreating,
  newSetting,
  onClose,
  onChange,
  onSubmit,
}) => {
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Tambah Parameter Runtime"
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={onClose}>
            Batal
          </Button>
          <Button
            variant="primary"
            size="sm"
            type="submit"
            form="create-setting-form"
            isLoading={isCreating}
            icon={<Save className="w-3.5 h-3.5" />}
          >
            Simpan Parameter
          </Button>
        </div>
      }
    >
      <form id="create-setting-form" noValidate onSubmit={onSubmit} className="space-y-4 text-xs">
        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5">
            Kunci Parameter (Key) *
          </label>
          <input
            type="text"
            required
            placeholder="contoh: gateway:maintenance_mode atau timeout_ms"
            value={newSetting.key}
            onChange={(e) => onChange('key', e.target.value)}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
          />
          <span className="text-[10px] text-text-muted mt-1 block">
            Gunakan huruf kecil, angka, dan titik/titik dua sebagai pemisah namespace.
          </span>
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5">
            Deskripsi Singkat (Opsional)
          </label>
          <input
            type="text"
            placeholder="Keterangan fungsi atau tujuan parameter ini..."
            value={newSetting.description}
            onChange={(e) => onChange('description', e.target.value)}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5">
            Nilai Parameter (Value) *
          </label>
          <textarea
            rows={4}
            required
            placeholder="Masukkan teks biasa, angka, boolean, atau format JSON..."
            value={newSetting.value}
            onChange={(e) => onChange('value', e.target.value)}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
          />
        </div>
      </form>
    </Modal>
  );
};
