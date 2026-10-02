import React, { useState, useEffect } from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { useToast } from '../../context/ToastContext';
import { MODEL_FAMILIES } from './utils';
import type { CreateModelDrawerProps, CreateModelFormData } from './types';

const INITIAL_FORM_DATA: CreateModelFormData = {
  model_id: '',
  display_name: '',
  family: 'gpt-4',
  context_window: 128000,
  max_output_tokens: 4096,
};

export const CreateModelDrawer: React.FC<CreateModelDrawerProps> = ({
  isOpen,
  onClose,
  onSubmit,
  isSubmitting = false,
  existingModelIds = [],
}) => {
  const { toast } = useToast();
  const [formData, setFormData] = useState<CreateModelFormData>(INITIAL_FORM_DATA);

  useEffect(() => {
    if (isOpen) {
      setFormData(INITIAL_FORM_DATA);
    }
  }, [isOpen]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const modelId = formData.model_id.trim();
    const displayName = formData.display_name.trim();

    if (!modelId) {
      toast.error('ID model wajib diisi.');
      return;
    }

    if (existingModelIds.some((id) => id.toLowerCase() === modelId.toLowerCase())) {
      toast.error(`Model "${modelId}" sudah terdaftar.`);
      return;
    }

    if (formData.context_window <= 0 || formData.max_output_tokens <= 0) {
      toast.error('Context window dan max output tokens wajib lebih dari 0.');
      return;
    }

    await onSubmit({
      ...formData,
      model_id: modelId,
      display_name: displayName || modelId,
    });
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title="Registrasi Model Kanonik"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={isSubmitting}>
            Batal
          </Button>
          <Button variant="primary" type="submit" form="create-model-form" isLoading={isSubmitting}>
            Simpan Model
          </Button>
        </>
      }
    >
      <form id="create-model-form" noValidate onSubmit={handleSubmit} className="space-y-4 text-xs">
        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="model_id">
            ID Model Kanonik *
          </label>
          <input
            id="model_id"
            name="model_id"
            type="text"
            required
            placeholder="gpt-4o"
            value={formData.model_id}
            onChange={(e) => setFormData({ ...formData, model_id: e.target.value })}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="display_name">
            Nama Tampilan *
          </label>
          <input
            id="display_name"
            name="display_name"
            type="text"
            required
            placeholder="OpenAI GPT-4o Omni"
            value={formData.display_name}
            onChange={(e) => setFormData({ ...formData, display_name: e.target.value })}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="family">
            Keluarga (Family)
          </label>
          <input
            id="family"
            name="family"
            type="text"
            placeholder="gpt-4"
            value={formData.family}
            onChange={(e) => setFormData({ ...formData, family: e.target.value })}
            className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent mb-2"
          />
          {/* Family Presets Pills */}
          <div className="flex flex-wrap gap-1.5 pt-0.5">
            <span className="text-[11px] text-text-muted mr-1 self-center">Preset:</span>
            {MODEL_FAMILIES.map((preset) => (
              <button
                key={preset}
                type="button"
                onClick={() => setFormData({ ...formData, family: preset.toLowerCase() })}
                className="px-2 py-0.5 rounded text-[10px] font-mono bg-bg-surface-2 border border-border text-text-secondary hover:text-white hover:border-accent transition-colors cursor-pointer"
              >
                {preset}
              </button>
            ))}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="context_window">
              Context Window
            </label>
            <input
              id="context_window"
              name="context_window"
              type="number"
              value={formData.context_window}
              onChange={(e) => setFormData({ ...formData, context_window: parseInt(e.target.value, 10) || 0 })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5" htmlFor="max_output_tokens">
              Max Output Tokens
            </label>
            <input
              id="max_output_tokens"
              name="max_output_tokens"
              type="number"
              value={formData.max_output_tokens}
              onChange={(e) => setFormData({ ...formData, max_output_tokens: parseInt(e.target.value, 10) || 0 })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
        </div>
      </form>
    </Drawer>
  );
};
