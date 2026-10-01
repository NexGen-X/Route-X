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
      <form id="create-setting-form" noValidate onSubmit={onSubmit} className="space-y-5 text-sm">
        <div className="space-y-2 group">
          <label className="block text-xs font-semibold text-text-muted uppercase tracking-wider ml-1">
            Kunci Parameter (Key) *
          </label>
          <div className="relative">
            <div className="absolute inset-0 bg-gradient-to-r from-accent/20 to-accent/0 rounded-xl opacity-0 group-focus-within:opacity-100 transition-opacity duration-500 blur-md pointer-events-none" />
            <input
              type="text"
              required
              placeholder="contoh: gateway:maintenance_mode atau timeout_ms"
              value={newSetting.key}
              onChange={(e) => onChange('key', e.target.value)}
              className="relative w-full px-4 py-3 bg-bg-surface-2/50 backdrop-blur-xl border border-white/5 rounded-xl text-white font-mono text-xs focus:outline-none focus:border-accent/50 focus:bg-bg-surface-2/80 transition-all shadow-inner"
            />
          </div>
          <span className="text-[10px] text-text-muted mt-1.5 block ml-1 leading-relaxed">
            Gunakan huruf kecil, angka, dan titik/titik dua sebagai pemisah namespace.
          </span>
        </div>

        <div className="space-y-2 group">
          <label className="block text-xs font-semibold text-text-muted uppercase tracking-wider ml-1">
            Deskripsi Singkat (Opsional)
          </label>
          <div className="relative">
            <div className="absolute inset-0 bg-gradient-to-r from-accent/20 to-accent/0 rounded-xl opacity-0 group-focus-within:opacity-100 transition-opacity duration-500 blur-md pointer-events-none" />
            <input
              type="text"
              placeholder="Keterangan fungsi atau tujuan parameter ini..."
              value={newSetting.description}
              onChange={(e) => onChange('description', e.target.value)}
              className="relative w-full px-4 py-3 bg-bg-surface-2/50 backdrop-blur-xl border border-white/5 rounded-xl text-white text-xs focus:outline-none focus:border-accent/50 focus:bg-bg-surface-2/80 transition-all shadow-inner"
            />
          </div>
        </div>

        <div className="space-y-2 group">
          <label className="block text-xs font-semibold text-text-muted uppercase tracking-wider ml-1">
            Nilai Parameter (Value) *
          </label>
          <div className="relative">
            <div className="absolute inset-0 bg-gradient-to-r from-accent/20 to-accent/0 rounded-xl opacity-0 group-focus-within:opacity-100 transition-opacity duration-500 blur-md pointer-events-none" />
            <textarea
              rows={4}
              required
              placeholder="Masukkan teks biasa, angka, boolean, atau format JSON..."
              value={newSetting.value}
              onChange={(e) => onChange('value', e.target.value)}
              className="relative w-full px-4 py-3 bg-bg-surface-2/50 backdrop-blur-xl border border-white/5 rounded-xl text-white font-mono text-xs focus:outline-none focus:border-accent/50 focus:bg-bg-surface-2/80 transition-all shadow-inner resize-none"
            />
          </div>
        </div>
      </form>
    </Modal>
  );
};
