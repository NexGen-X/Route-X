import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Model, Provider } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import { Cpu, Plus, DollarSign, Layers } from 'lucide-react';

export const Models: React.FC = () => {
  const [models, setModels] = useState<Model[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [selectedModel, setSelectedModel] = useState<Model | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [isPriceOpen, setIsPriceOpen] = useState(false);
  const [targetMappingId, setTargetMappingId] = useState<string>('');

  const [newModel, setNewModel] = useState({
    model_id: '',
    display_name: '',
    family: 'gpt-4',
    context_window: 128000,
    max_output_tokens: 4096,
  });

  const [pricingForm, setPricingForm] = useState({
    input_usd: '2.50',
    output_usd: '10.00',
    cached_input_usd: '1.25',
  });

  const loadModels = async () => {
    try {
      const [mRes, pRes] = await Promise.all([api.models.list(), api.providers.list()]);
      setModels(mRes.items || []);
      setProviders(pRes.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadModels();
  }, []);

  const handleCreateModel = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.models.create({
        ...newModel,
        capabilities: ['chat', 'streaming'],
      });
      setIsCreateOpen(false);
      loadModels();
    } catch (err) {
      alert('Gagal membuat model: ' + err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Model Registry & Pricing</h2>
          <p className="text-xs text-text-secondary mt-1">
            Katalog model kanonik, pemetaan alias, dan struktur harga per 1 juta token dengan presisi skala 8 desimal.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onClick={() => setIsCreateOpen(true)}
          icon={<Plus className="w-4 h-4" />}
        >
          Daftarkan Model
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {models.map((m) => (
          <Card key={m.id} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-purple-500/10 border border-purple-500/20 text-purple-400 flex items-center justify-center">
                    <Cpu className="w-5 h-5" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">{m.display_name}</h4>
                    <span className="text-[11px] text-accent font-mono">{m.model_id}</span>
                  </div>
                </div>
                <Badge variant={m.enabled ? 'success' : 'neutral'}>
                  {m.enabled ? 'active' : 'disabled'}
                </Badge>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Keluarga Model</span>
                  <span className="font-semibold text-text-secondary">{m.family || '-'}</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Context Window</span>
                  <span className="font-mono text-white">{(m.context_window ?? 0).toLocaleString()} tokens</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Max Output</span>
                  <span className="font-mono text-white">{(m.max_output_tokens ?? 0).toLocaleString()} tokens</span>
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
              <div className="flex gap-1">
                {m.capabilities?.map((c) => (
                  <span key={c} className="px-2 py-0.5 text-[10px] font-mono rounded bg-bg-surface-2 text-text-muted">
                    {c}
                  </span>
                ))}
              </div>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  setSelectedModel(m);
                  setIsPriceOpen(true);
                }}
                icon={<DollarSign className="w-3.5 h-3.5" />}
              >
                Pricing
              </Button>
            </div>
          </Card>
        ))}
      </div>

      {/* Modal Create Model */}
      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Registrasi Model Kanonik"
        subtitle="Tambahkan entri model baru ke dalam katalog Route-X"
      >
        <form onSubmit={handleCreateModel} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">ID Model Kanonik</label>
            <input
              type="text"
              required
              placeholder="gpt-4o"
              value={newModel.model_id}
              onChange={(e) => setNewModel({ ...newModel, model_id: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Nama Tampilan</label>
            <input
              type="text"
              required
              placeholder="OpenAI GPT-4o Omni"
              value={newModel.display_name}
              onChange={(e) => setNewModel({ ...newModel, display_name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div>
            <label className="block font-semibold text-text-secondary uppercase mb-1">Keluarga (Family)</label>
            <input
              type="text"
              placeholder="gpt-4"
              value={newModel.family}
              onChange={(e) => setNewModel({ ...newModel, family: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Context Window</label>
              <input
                type="number"
                value={newModel.context_window}
                onChange={(e) => setNewModel({ ...newModel, context_window: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">Max Output Tokens</label>
              <input
                type="number"
                value={newModel.max_output_tokens}
                onChange={(e) => setNewModel({ ...newModel, max_output_tokens: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>
          </div>
          <Button type="submit" variant="primary" size="md" className="w-full mt-2">
            Simpan Model
          </Button>
        </form>
      </Modal>
    </div>
  );
};
