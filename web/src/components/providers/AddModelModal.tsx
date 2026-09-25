import React, { useState } from 'react';
import type { Provider } from '../../types';
import type { api as apiClient } from '../../api/client';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { Plus } from 'lucide-react';

export interface AddModelModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedProvider: Provider | null;
  loadProviderDetails: (providerId: string) => Promise<void>;
  toast: {
    success: (msg: string) => void;
    error: (msg: string) => void;
    info?: (msg: string) => void;
  };
  api: typeof apiClient;
}

export const AddModelModal: React.FC<AddModelModalProps> = ({
  isOpen,
  onClose,
  selectedProvider,
  loadProviderDetails,
  toast,
  api,
}) => {
  const [newModelName, setNewModelName] = useState('');
  const [isSavingModel, setIsSavingModel] = useState(false);

  const handleAddModelManual = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;
    const trimmed = newModelName.trim();
    if (!trimmed) {
      toast.error('Nama model wajib diisi');
      return;
    }

    setIsSavingModel(true);
    try {
      await api.providers.addModel(selectedProvider.id, trimmed);
      toast.success(`Model "${trimmed}" berhasil didaftarkan`);
      setNewModelName('');
      onClose();
      await loadProviderDetails(selectedProvider.id);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal menambahkan model: ' + errMsg);
    } finally {
      setIsSavingModel(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={`Tambah Model Manual — ${selectedProvider?.display_name || selectedProvider?.name || ''}`}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Batal
          </Button>
          <Button
            type="submit"
            variant="primary"
            form="add-model-manual-form"
            isLoading={isSavingModel}
            icon={<Plus className="w-4 h-4" />}
          >
            Simpan Model
          </Button>
        </>
      }
    >
      <form id="add-model-manual-form" noValidate onSubmit={handleAddModelManual} className="space-y-4 text-xs">
        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5">ID Model *</label>
          <input
            type="text"
            name="model_name"
            required
            placeholder="claude-3-7-sonnet, gpt-4o, atau deepseek/deepseek-chat"
            value={newModelName}
            onChange={(e) => setNewModelName(e.target.value)}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
          />
          <p className="text-[11px] text-text-muted mt-1">Sesuai dokumentasi provider.</p>
        </div>
      </form>
    </Modal>
  );
};
