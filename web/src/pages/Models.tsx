import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Model, ProviderModel, Price } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { Cpu, Plus, DollarSign, Trash2 } from 'lucide-react';
import { useToast } from '../context/ToastContext';

export const Models: React.FC = () => {
  const { toast } = useToast();
  const [models, setModels] = useState<Model[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);

  // State untuk Modal Pricing
  const [isPricingOpen, setIsPricingOpen] = useState(false);
  const [selectedModel, setSelectedModel] = useState<Model | null>(null);
  const [mappings, setMappings] = useState<ProviderModel[]>([]);
  const [selectedMappingId, setSelectedMappingId] = useState<string>('');
  const [pricingHistory, setPricingHistory] = useState<Price[]>([]);
  const [pricingForm, setPricingForm] = useState({
    input_per_1m_usd: '',
    output_per_1m_usd: '',
    cached_input_per_1m_usd: '',
  });
  const [isPricingLoading, setIsPricingLoading] = useState(false);
  const [isSavingPrice, setIsSavingPrice] = useState(false);

  const [newModel, setNewModel] = useState({
    model_id: '',
    display_name: '',
    family: 'gpt-4',
    context_window: 128000,
    max_output_tokens: 4096,
  });

  const loadModels = async () => {
    try {
      const mRes = await api.models.list();
      setModels(mRes.items || []);
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadModels();
  }, []);

  const handleCreateModel = async (e: React.FormEvent) => {
    e.preventDefault();
    const modelId = newModel.model_id.trim();
    const displayName = newModel.display_name.trim();
    if (!modelId) {
      toast.error('ID model wajib diisi.');
      return;
    }
    if (models.some((m) => m.model_id.toLowerCase() === modelId.toLowerCase())) {
      toast.error(`Model "${modelId}" sudah terdaftar.`);
      return;
    }
    if (newModel.context_window <= 0 || newModel.max_output_tokens <= 0) {
      toast.error('Context window dan max output tokens wajib lebih dari 0.');
      return;
    }
    try {
      await api.models.create({
        ...newModel,
        model_id: modelId,
        display_name: displayName || modelId,
        // CHECK models_capabilities_known hanya mengizinkan: text, vision,
        // reasoning, tools, embeddings. Nilai 'chat'/'streaming' memicu 500.
        capabilities: ['text'],
      });
      setIsCreateOpen(false);
      toast.success('Model kanonik baru berhasil didaftarkan');
      loadModels();
    } catch (err) {
      toast.error('Gagal membuat model: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleDeleteModel = async (id: string, name: string) => {
    if (!window.confirm(`Hapus model kanonik "${name}"? Ini akan memutuskan semua provider yang tertaut ke model ini.`)) return;
    try {
      await api.models.delete(id);
      toast.success(`Model "${name}" berhasil dihapus`);
      loadModels();
    } catch (err) {
      toast.error('Gagal menghapus model: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const loadPricingForMapping = async (mappingId: string) => {
    try {
      const res = await api.models.pricingHistory(mappingId);
      const items = res.items || [];
      setPricingHistory(items);
      if (items.length > 0) {
        const latest = items[0];
        setPricingForm({
          input_per_1m_usd: latest.input_per_1m_usd || '',
          output_per_1m_usd: latest.output_per_1m_usd || '',
          cached_input_per_1m_usd: latest.cached_input_per_1m_usd || '',
        });
      }
    } catch (err) {
      console.error('Gagal memuat riwayat harga:', err);
    }
  };

  const handleOpenPricing = async (m: Model) => {
    setSelectedModel(m);
    setIsPricingOpen(true);
    setIsPricingLoading(true);
    setMappings([]);
    setSelectedMappingId('');
    setPricingHistory([]);
    setPricingForm({
      input_per_1m_usd: '',
      output_per_1m_usd: '',
      cached_input_per_1m_usd: '',
    });

    try {
      const detail = await api.models.get(m.id);
      const mps = detail.mappings || [];
      setMappings(mps);
      if (mps.length > 0) {
        setSelectedMappingId(mps[0].id);
        await loadPricingForMapping(mps[0].id);
      }
    } catch (err) {
      console.error('Gagal memuat detail pemetaan model:', err);
    } finally {
      setIsPricingLoading(false);
    }
  };

  const handleSavePrice = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedMappingId) return;
    const validUSD = (v: string) => v.trim() !== '' && Number.isFinite(Number(v)) && Number(v) >= 0;
    if (!validUSD(pricingForm.input_per_1m_usd) || !validUSD(pricingForm.output_per_1m_usd)) {
      toast.error('Harga input/output wajib angka >= 0 (contoh: 2.50).');
      return;
    }
    if (pricingForm.cached_input_per_1m_usd.trim() !== '' && !validUSD(pricingForm.cached_input_per_1m_usd)) {
      toast.error('Harga cached input wajib angka >= 0 atau dikosongkan.');
      return;
    }
    setIsSavingPrice(true);
    try {
      await api.models.setPrice(selectedMappingId, {
        input_per_1m_usd: pricingForm.input_per_1m_usd,
        output_per_1m_usd: pricingForm.output_per_1m_usd,
        cached_input_per_1m_usd: pricingForm.cached_input_per_1m_usd || undefined,
      });
      toast.success('Harga model berhasil disimpan');
      await loadPricingForMapping(selectedMappingId);
    } catch (err) {
      toast.error('Gagal menyimpan harga model: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      setIsSavingPrice(false);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Model Registry & Pricing"
        description="Katalog model kanonik, pemetaan alias, dan struktur harga per 1 juta token dengan presisi skala 8 desimal."
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="w-full sm:w-auto justify-center"
          >
            Daftarkan Model
          </Button>
        }
      />

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
                <div className="flex items-center gap-2">
                  <Badge variant={m.enabled ? 'success' : 'neutral'}>
                    {m.enabled ? 'active' : 'disabled'}
                  </Badge>
                  <button
                    onClick={() => handleDeleteModel(m.id, m.display_name)}
                    className="p-1 text-text-muted hover:text-red-400 hover:bg-red-500/10 rounded-nav transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                    title="Hapus Model"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              <div className="mt-4 space-y-2 text-xs">
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Keluarga Model</span>
                  <span className="font-semibold text-text-secondary">{m.family || '-'}</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Context Window</span>
                  <span className="font-mono text-white">{m.context_window != null ? `${m.context_window.toLocaleString()} tokens` : '-'}</span>
                </div>
                <div className="flex justify-between py-1 border-b border-border/40">
                  <span className="text-text-muted">Max Output</span>
                  <span className="font-mono text-white">{m.max_output_tokens != null ? `${m.max_output_tokens.toLocaleString()} tokens` : '-'}</span>
                </div>
              </div>
              <div className="mt-4 pt-3 border-t border-border/40">
                <div className="text-[10px] font-bold text-text-muted uppercase tracking-wider mb-2 flex items-center justify-between">
                  <span>Penyedia Upstream ({m.providers?.length || 0})</span>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {m.providers && m.providers.length > 0 ? (
                    m.providers.map((p) => (
                      <span
                        key={p.provider_id}
                        className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-purple-500/10 border border-purple-500/25 text-purple-300 text-[11px] font-medium"
                        title={`Upstream model name: ${p.upstream_model_name}`}
                      >
                        <span className="w-1.5 h-1.5 rounded-full bg-purple-400" />
                        <span className="font-semibold text-white">{p.display_name || p.provider_name}</span>
                        {p.upstream_model_name !== m.model_id && (
                          <span className="text-[10px] text-text-muted font-mono">({p.upstream_model_name})</span>
                        )}
                      </span>
                    ))
                  ) : (
                    <span className="text-[11px] text-text-muted italic">Belum terhubung ke provider</span>
                  )}
                </div>
              </div>
            </div>

            <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
              <div className="flex flex-wrap gap-1">
                {m.capabilities?.map((c) => (
                  <span key={c} className="px-2 py-0.5 text-[10px] font-mono rounded bg-bg-surface-2 text-text-muted">
                    {c}
                  </span>
                ))}
              </div>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleOpenPricing(m)}
                icon={<DollarSign className="w-3.5 h-3.5" />}
              >
                Pricing
              </Button>
            </div>
          </Card>
        ))}
      </div>

      {/* Modal Create Model */}
      <Drawer
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
      </Drawer>

      {/* Modal Pricing */}
      <Drawer
        isOpen={isPricingOpen}
        onClose={() => setIsPricingOpen(false)}
        title={`Konfigurasi Harga: ${selectedModel?.display_name || selectedModel?.model_id || 'Model'}`}
        subtitle="Atur tarif token per 1M (USD) untuk model ini pada provider upstream terkait"
      >
        {isPricingLoading ? (
          <div className="py-8 text-center text-xs text-text-muted">Memuat pemetaan model...</div>
        ) : mappings.length === 0 ? (
          <div className="py-6 text-center text-xs text-text-muted space-y-2">
            <p>Model ini belum memiliki pemetaan ke upstream provider.</p>
            <p className="text-[11px]">Hubungkan provider terlebih dahulu sebelum mengatur struktur harga.</p>
          </div>
        ) : (
          <form onSubmit={handleSavePrice} className="space-y-4 text-xs">
            <Select
              label="Pilih Pemetaan Provider"
              value={selectedMappingId}
              onChange={(val) => {
                setSelectedMappingId(val);
                loadPricingForMapping(val);
              }}
              options={mappings.filter((mp) => mp.id).map((mp) => {
                const prov = selectedModel?.providers?.find((p) => p.provider_id === mp.provider_id);
                const provName = prov ? (prov.display_name || prov.provider_name) : (mp.provider_id || '').slice(0, 8);
                return {
                  value: mp.id,
                  label: `${provName} → ${mp.upstream_model_name || '?'}`,
                  description: `Provider: ${provName} | ID Mapping: ${(mp.id || '').slice(0, 8)}`,
                };
              })}
            />

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Input / 1M Token (USD) *
                </label>
                <input
                  type="text"
                  required
                  placeholder="2.50"
                  value={pricingForm.input_per_1m_usd}
                  onChange={(e) => setPricingForm({ ...pricingForm, input_per_1m_usd: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
                />
              </div>

              <div>
                <label className="block font-semibold text-text-secondary uppercase mb-1">
                  Output / 1M Token (USD) *
                </label>
                <input
                  type="text"
                  required
                  placeholder="10.00"
                  value={pricingForm.output_per_1m_usd}
                  onChange={(e) => setPricingForm({ ...pricingForm, output_per_1m_usd: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
                />
              </div>
            </div>

            <div>
              <label className="block font-semibold text-text-secondary uppercase mb-1">
                Cached Input / 1M Token (USD) (Opsional)
              </label>
              <input
                type="text"
                placeholder="1.25"
                value={pricingForm.cached_input_per_1m_usd}
                onChange={(e) => setPricingForm({ ...pricingForm, cached_input_per_1m_usd: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
            </div>

            {pricingHistory.length > 0 && (
              <div className="pt-2">
                <span className="block font-semibold text-text-secondary uppercase mb-1">
                  Riwayat Penetapan Harga
                </span>
                <div className="max-h-32 overflow-y-auto space-y-1 rounded border border-border p-2 bg-bg-base">
                  {pricingHistory.map((h, i) => (
                    <div key={`${h.effective_from}-${i}`} className="flex justify-between text-[11px] font-mono text-text-secondary">
                      <span>In: ${h.input_per_1m_usd} | Out: ${h.output_per_1m_usd}</span>
                      <span className="text-text-muted">{h.effective_from ? new Date(h.effective_from).toLocaleDateString() : '-'}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="pt-3 flex justify-end gap-2 border-t border-border">
              <Button type="button" variant="secondary" onClick={() => setIsPricingOpen(false)}>
                Batal
              </Button>
              <Button type="submit" variant="primary" isLoading={isSavingPrice}>
                Simpan Harga
              </Button>
            </div>
          </form>
        )}
      </Drawer>
    </div>
  );
};
