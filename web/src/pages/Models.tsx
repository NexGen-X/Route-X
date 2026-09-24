import React, { useEffect, useState, useMemo } from 'react';
import { api } from '../api/client';
import type { Model, ProviderModel, Price } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Drawer } from '../components/common/Drawer';
import { PageHeader } from '../components/common/PageHeader';
import { Select } from '../components/common/Select';
import { Tooltip } from '../components/common/Tooltip';
import { Cpu, Plus, DollarSign, Trash2, Search, X, Table, LayoutGrid, ChevronLeft, ChevronRight, Layers } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { QueryError } from '../components/common/QueryError';

export const getModelFamily = (m: Model): string => {
  if (m.family && m.family.trim()) {
    const f = m.family.toLowerCase().trim();
    if (f.includes('claude')) return 'Claude';
    if (f.includes('gpt') || f.includes('o1') || f.includes('o3') || f.includes('openai')) return 'GPT';
    if (f.includes('deepseek')) return 'DeepSeek';
    if (f.includes('gemini')) return 'Gemini';
    if (f.includes('qwen')) return 'Qwen';
    if (f.includes('glm')) return 'GLM';
    if (f.includes('kimi') || f.includes('moonshot')) return 'Kimi';
    if (f.includes('grok')) return 'Grok';
    if (f.includes('mimo')) return 'Mimo';
    return m.family;
  }
  const id = (m.model_id || '').toLowerCase();
  if (id.startsWith('claude')) return 'Claude';
  if (id.startsWith('gpt') || id.startsWith('o1') || id.startsWith('o3') || id.startsWith('text-embedding')) return 'GPT';
  if (id.startsWith('deepseek')) return 'DeepSeek';
  if (id.startsWith('gemini')) return 'Gemini';
  if (id.startsWith('qwen')) return 'Qwen';
  if (id.startsWith('glm')) return 'GLM';
  if (id.startsWith('kimi') || id.startsWith('moonshot')) return 'Kimi';
  if (id.startsWith('grok')) return 'Grok';
  if (id.startsWith('mimo')) return 'Mimo';
  return 'Other';
};

export const Models: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [models, setModels] = useState<Model[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  // Tampilan & Navigasi
  const [viewMode, setViewMode] = useState<'table' | 'grid'>(() => {
    const saved = localStorage.getItem('routex_models_view_mode');
    return saved === 'grid' ? 'grid' : 'table';
  });
  const [selectedFamily, setSelectedFamily] = useState<string>('all');
  const [currentPage, setCurrentPage] = useState<number>(1);
  const [pageSize, setPageSize] = useState<number>(12);

  const handleViewChange = (mode: 'table' | 'grid') => {
    setViewMode(mode);
    localStorage.setItem('routex_models_view_mode', mode);
  };

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
    setIsLoading(true);
    setLoadError(null);
    try {
      const mRes = await api.models.list();
      setModels(mRes.items || []);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadModels();
  }, []);

  // Hitung jumlah model per rumpun untuk filter chips
  const familyCounts = useMemo(() => {
    const counts: Record<string, number> = { all: models.length };
    models.forEach((m) => {
      const fam = getModelFamily(m);
      counts[fam] = (counts[fam] || 0) + 1;
    });
    return counts;
  }, [models]);

  const availableFamilies = useMemo(() => {
    const priority = ['Claude', 'GPT', 'DeepSeek', 'Gemini', 'Qwen', 'GLM', 'Kimi', 'Grok', 'Mimo'];
    const present = Object.keys(familyCounts).filter((k) => k !== 'all' && familyCounts[k] > 0);
    present.sort((a, b) => {
      const idxA = priority.indexOf(a);
      const idxB = priority.indexOf(b);
      if (idxA !== -1 && idxB !== -1) return idxA - idxB;
      if (idxA !== -1) return -1;
      if (idxB !== -1) return 1;
      return a.localeCompare(b);
    });
    return ['all', ...present];
  }, [familyCounts]);

  const filteredModels = useMemo(() => {
    let result = models;
    if (selectedFamily !== 'all') {
      result = result.filter((m) => getModelFamily(m).toLowerCase() === selectedFamily.toLowerCase());
    }
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase().trim();
      result = result.filter((m) => {
        return (
          m.model_id.toLowerCase().includes(q) ||
          m.display_name.toLowerCase().includes(q) ||
          (m.family && m.family.toLowerCase().includes(q)) ||
          getModelFamily(m).toLowerCase().includes(q) ||
          (m.providers &&
            m.providers.some(
              (p) =>
                p.provider_name.toLowerCase().includes(q) ||
                p.display_name.toLowerCase().includes(q) ||
                p.upstream_model_name.toLowerCase().includes(q)
            ))
        );
      });
    }
    return result;
  }, [models, selectedFamily, searchQuery]);

  useEffect(() => {
    setCurrentPage(1);
  }, [searchQuery, selectedFamily]);

  const totalPages = Math.max(1, Math.ceil(filteredModels.length / pageSize));
  const paginatedModels = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filteredModels.slice(start, start + pageSize);
  }, [filteredModels, currentPage, pageSize]);

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
    const confirmed = await confirmModal({
      title: 'Hapus Model Kanonik',
      message: `Hapus model kanonik "${name}"? Ini akan memutuskan semua provider yang tertaut ke model ini.`,
      confirmText: 'Hapus Model',
      danger: true,
    });
    if (!confirmed) return;
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
        description="Katalog model AI kanonik, pemetaan ke penyedia upstream, serta konfigurasi tarif biaya token."
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

      {loadError && (
        <div className="mb-4">
          <QueryError message={loadError} onRetry={() => void loadModels()} />
        </div>
      )}

      {/* Toolbar Pencarian & View Mode */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 bg-bg-surface-1 p-3 rounded-xl border border-border">
        <div className="relative flex-1">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
          <input
            type="text"
            placeholder="Cari model berdasarkan nama, slug, provider, atau family (mis. deepseek, claude, tknharbor)..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-9 pr-9 py-2 bg-bg-surface-2 border border-border rounded-lg text-xs text-white placeholder:text-text-muted focus:outline-none focus:border-accent"
          />
          {searchQuery && (
            <button
              type="button"
              onClick={() => setSearchQuery('')}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 p-1 text-text-muted hover:text-white rounded-md cursor-pointer transition-colors"
              aria-label="Hapus pencarian"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>

        <div className="flex items-center justify-between sm:justify-end gap-3 shrink-0">
          <span className="text-xs font-mono text-text-muted">
            <strong className="text-white">{filteredModels.length}</strong> dari {models.length} model
          </span>

          {/* View Switcher */}
          <div className="flex items-center bg-bg-surface-2 p-0.5 rounded-lg border border-border">
            <button
              type="button"
              onClick={() => handleViewChange('table')}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md cursor-pointer transition-all ${
                viewMode === 'table'
                  ? 'bg-accent text-white shadow-sm font-semibold'
                  : 'text-text-muted hover:text-white'
              }`}
              title="Tampilan Tabel Ringkas"
            >
              <Table className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Tabel</span>
            </button>
            <button
              type="button"
              onClick={() => handleViewChange('grid')}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md cursor-pointer transition-all ${
                viewMode === 'grid'
                  ? 'bg-accent text-white shadow-sm font-semibold'
                  : 'text-text-muted hover:text-white'
              }`}
              title="Tampilan Kartu Grid"
            >
              <LayoutGrid className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Kartu</span>
            </button>
          </div>
        </div>
      </div>

      {/* Filter Rumpun / Family Chips Bar */}
      {availableFamilies.length > 2 && (
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs no-scrollbar">
          <div className="flex items-center gap-1 text-text-muted mr-1 shrink-0">
            <Layers className="w-3.5 h-3.5" />
            <span className="text-[11px] font-semibold uppercase tracking-wider">Keluarga:</span>
          </div>
          {availableFamilies.map((fam) => {
            const isSelected = selectedFamily.toLowerCase() === fam.toLowerCase();
            const label = fam === 'all' ? 'Semua' : fam;
            const count = familyCounts[fam] ?? 0;
            return (
              <button
                key={fam}
                type="button"
                onClick={() => setSelectedFamily(fam)}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium shrink-0 cursor-pointer transition-colors ${
                  isSelected
                    ? 'bg-accent text-white shadow-sm'
                    : 'bg-bg-surface-1 border border-border text-text-muted hover:text-white hover:bg-bg-surface-2'
                }`}
              >
                <span>{label}</span>
                <span
                  className={`text-[10px] px-1.5 py-0.2 rounded-full font-mono ${
                    isSelected ? 'bg-white/20 text-white' : 'bg-bg-surface-2 text-text-muted'
                  }`}
                >
                  {count}
                </span>
              </button>
            );
          })}
        </div>
      )}

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(6)].map((_, idx) => (
            <Card key={idx} className="p-5 animate-pulse space-y-4">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-bg-surface-2" />
                <div className="space-y-1.5 flex-1">
                  <div className="h-4 bg-bg-surface-2 rounded w-2/3" />
                  <div className="h-3 bg-bg-surface-2 rounded w-1/3" />
                </div>
              </div>
              <div className="space-y-2 pt-2">
                <div className="h-3 bg-bg-surface-2 rounded w-full" />
                <div className="h-3 bg-bg-surface-2 rounded w-4/5" />
              </div>
            </Card>
          ))}
        </div>
      ) : models.length === 0 ? (
        <Card className="p-10 text-center rounded-box bg-bg-surface-1 border border-border/60">
          <div className="w-12 h-12 rounded-2xl bg-purple-500/10 border border-purple-500/20 text-purple-400 flex items-center justify-center mx-auto mb-3">
            <Cpu className="w-6 h-6" />
          </div>
          <h3 className="text-base font-bold text-white mb-1">Belum Ada Model Kanonik Terdaftar</h3>
          <p className="text-xs text-text-secondary max-w-md mx-auto mb-5 leading-relaxed">
            Daftarkan model kanonik AI pertama Anda (misal <code className="font-mono text-accent">gpt-5</code> atau <code className="font-mono text-accent">claude-sonnet-5</code>) untuk mulai merutekan permintaan ke provider upstream.
          </p>
          <Button
            variant="primary"
            size="sm"
            onClick={() => setIsCreateOpen(true)}
            icon={<Plus className="w-4 h-4" />}
            className="h-9 px-4 text-xs font-semibold mx-auto"
          >
            Daftarkan Model Sekarang
          </Button>
        </Card>
      ) : filteredModels.length === 0 ? (
        <Card className="p-10 text-center rounded-box bg-bg-surface-1 border border-border/60">
          <div className="w-12 h-12 rounded-2xl bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center mx-auto mb-3">
            <Search className="w-6 h-6" />
          </div>
          <h3 className="text-base font-bold text-white mb-1">Model Tidak Ditemukan</h3>
          <p className="text-xs text-text-secondary max-w-md mx-auto mb-4 leading-relaxed">
            Tidak ada model yang cocok dengan kriteria filter saat ini
            {searchQuery && (
              <> (kata kunci: <span className="font-mono text-accent">"{searchQuery}"</span>)</>
            )}
            {selectedFamily !== 'all' && (
              <> (keluarga: <span className="font-mono text-accent">{selectedFamily}</span>)</>
            )}.
          </p>
          <div className="flex items-center justify-center gap-2">
            {searchQuery && (
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setSearchQuery('')}
              >
                Reset Kata Kunci
              </Button>
            )}
            {selectedFamily !== 'all' && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSelectedFamily('all')}
              >
                Tampilkan Semua Rumpun
              </Button>
            )}
          </div>
        </Card>
      ) : (
        <div className="space-y-4">
          {/* Mode Tampilan: Table vs Grid */}
          {viewMode === 'table' ? (
            <div className="bg-bg-surface-1 border border-border rounded-xl overflow-hidden shadow-sm">
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs">
                  <thead>
                    <tr className="border-b border-border text-text-muted font-semibold text-xs bg-bg-surface-2/40">
                      <th className="py-3 px-4">Model & ID</th>
                      <th className="py-3 px-4">Keluarga</th>
                      <th className="py-3 px-4">Penyedia Upstream</th>
                      <th className="py-3 px-4">Konteks / Output</th>
                      <th className="py-3 px-4">Kemampuan</th>
                      <th className="py-3 px-4 text-center">Status</th>
                      <th className="py-3 px-4 text-right">Aksi</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border/60">
                    {paginatedModels.map((m) => (
                      <tr key={m.id} className="hover:bg-bg-surface-2/40 transition-colors">
                        <td className="py-3 px-4">
                          <div className="flex items-center gap-2.5">
                            <div className="w-8 h-8 rounded-lg bg-purple-500/10 border border-purple-500/20 text-purple-400 flex items-center justify-center shrink-0">
                              <Cpu className="w-4 h-4" />
                            </div>
                            <div className="min-w-0">
                              <div className="font-semibold text-white truncate max-w-[200px]">{m.display_name}</div>
                              <div className="text-[11px] text-accent font-mono truncate max-w-[200px]">{m.model_id}</div>
                            </div>
                          </div>
                        </td>
                        <td className="py-3 px-4 whitespace-nowrap">
                          <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-bg-surface-2 border border-border text-text-secondary">
                            {getModelFamily(m)}
                          </span>
                        </td>
                        <td className="py-3 px-4">
                          <div className="flex flex-wrap gap-1 max-w-xs">
                            {m.providers && m.providers.length > 0 ? (
                              m.providers.map((p) => (
                                <Tooltip key={p.provider_id} content={`Upstream: ${p.upstream_model_name}`}>
                                  <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-purple-500/10 border border-purple-500/20 text-purple-300 text-[10px] font-medium">
                                    <span className="w-1.5 h-1.5 rounded-full bg-purple-400" />
                                    <span>{p.display_name || p.provider_name}</span>
                                  </span>
                                </Tooltip>
                              ))
                            ) : (
                              <span className="text-[11px] text-text-muted italic">Belum terhubung</span>
                            )}
                          </div>
                        </td>
                        <td className="py-3 px-4 font-mono text-[11px] whitespace-nowrap">
                          <div className="text-white">
                            {m.context_window != null ? `${m.context_window.toLocaleString()} ctx` : '-'}
                          </div>
                          <div className="text-text-muted">
                            {m.max_output_tokens != null ? `${m.max_output_tokens.toLocaleString()} out` : '-'}
                          </div>
                        </td>
                        <td className="py-3 px-4">
                          <div className="flex flex-wrap gap-1 max-w-[150px]">
                            {m.capabilities && m.capabilities.length > 0 ? (
                              m.capabilities.map((c) => (
                                <span key={c} className="px-1.5 py-0.5 text-[9px] font-mono rounded bg-bg-surface-2 text-text-muted border border-border/50">
                                  {c}
                                </span>
                              ))
                            ) : (
                              <span className="text-[11px] text-text-muted">-</span>
                            )}
                          </div>
                        </td>
                        <td className="py-3 px-4 text-center whitespace-nowrap">
                          <Badge variant={m.enabled ? 'success' : 'neutral'}>
                            {m.enabled ? 'active' : 'disabled'}
                          </Badge>
                        </td>
                        <td className="py-3 px-4 text-right whitespace-nowrap">
                          <div className="flex items-center justify-end gap-1.5">
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => handleOpenPricing(m)}
                              icon={<DollarSign className="w-3.5 h-3.5" />}
                              className="h-7 px-2.5 text-xs"
                            >
                              Harga
                            </Button>
                            <Tooltip content="Hapus Model" position="left">
                              <button
                                type="button"
                                onClick={() => handleDeleteModel(m.id, m.display_name)}
                                className="p-1.5 h-7 w-7 text-text-muted hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors flex items-center justify-center focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 cursor-pointer"
                                aria-label={`Hapus model ${m.display_name}`}
                              >
                                <Trash2 className="w-3.5 h-3.5" />
                              </button>
                            </Tooltip>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {paginatedModels.map((m) => (
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
                        <Tooltip content="Hapus Model" position="left">
                          <button
                            type="button"
                            onClick={() => handleDeleteModel(m.id, m.display_name)}
                            className="p-1.5 h-8 w-8 text-text-muted hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors flex items-center justify-center focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 cursor-pointer"
                            aria-label={`Hapus model ${m.display_name}`}
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </Tooltip>
                      </div>
                    </div>

                    <div className="mt-4 space-y-2 text-xs">
                      <div className="flex justify-between py-1 border-b border-border/40">
                        <span className="text-text-muted">Keluarga Model</span>
                        <span className="font-semibold text-text-secondary">{getModelFamily(m)}</span>
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
                      <div className="text-xs font-semibold text-text-muted mb-2 flex items-center justify-between">
                        <span>Penyedia Upstream ({m.providers?.length || 0})</span>
                      </div>
                      <div className="flex flex-wrap gap-1.5">
                        {m.providers && m.providers.length > 0 ? (
                          m.providers.map((p) => (
                            <Tooltip key={p.provider_id} content={`Upstream model: ${p.upstream_model_name}`}>
                              <span
                                className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-purple-500/10 border border-purple-500/25 text-purple-300 text-[11px] font-medium"
                              >
                                <span className="w-1.5 h-1.5 rounded-full bg-purple-400" />
                                <span className="font-semibold text-white">{p.display_name || p.provider_name}</span>
                                {p.upstream_model_name !== m.model_id && (
                                  <span className="text-[10px] text-text-muted font-mono">({p.upstream_model_name})</span>
                                )}
                              </span>
                            </Tooltip>
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
                      Harga
                    </Button>
                  </div>
                </Card>
              ))}
            </div>
          )}

          {/* Kontrol Paginasi */}
          <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-3 px-1 text-xs text-text-muted border-t border-border/50">
            <div className="flex items-center gap-2">
              <span>
                Menampilkan <strong className="text-white font-mono">{Math.min((currentPage - 1) * pageSize + 1, filteredModels.length)}</strong> - <strong className="text-white font-mono">{Math.min(currentPage * pageSize, filteredModels.length)}</strong> dari <strong className="text-white font-mono">{filteredModels.length}</strong> model
                {filteredModels.length !== models.length && (
                  <span className="text-text-muted"> (difilter dari total {models.length})</span>
                )}
              </span>
            </div>

            <div className="flex items-center gap-3">
              <div className="flex items-center gap-1.5">
                <span className="text-[11px] text-text-muted shrink-0">Per halaman:</span>
                <div className="w-20 shrink-0">
                  <Select
                    variant="compact"
                    value={String(pageSize)}
                    onChange={(val) => {
                      setPageSize(Number(val));
                      setCurrentPage(1);
                    }}
                    options={[
                      { value: '12', label: '12' },
                      { value: '24', label: '24' },
                      { value: '48', label: '48' },
                    ]}
                    aria-label="Jumlah model per halaman"
                  />
                </div>
              </div>

              <div className="flex items-center gap-1">
                <button
                  type="button"
                  disabled={currentPage <= 1}
                  onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                  className="p-1.5 rounded-lg border border-border bg-bg-surface-2 text-text-muted hover:text-white disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
                  aria-label="Halaman sebelumnya"
                >
                  <ChevronLeft className="w-4 h-4" />
                </button>

                <span className="px-2.5 py-1 text-xs font-mono text-white bg-bg-surface-2 border border-border rounded-lg">
                  {currentPage} / {totalPages}
                </span>

                <button
                  type="button"
                  disabled={currentPage >= totalPages}
                  onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                  className="p-1.5 rounded-lg border border-border bg-bg-surface-2 text-text-muted hover:text-white disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
                  aria-label="Halaman berikutnya"
                >
                  <ChevronRight className="w-4 h-4" />
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Modal Create Model */}
      <Drawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title="Registrasi Model Kanonik"
        subtitle="Tambahkan entri model baru ke dalam katalog Route-X"
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateOpen(false)}>
              Batal
            </Button>
            <Button variant="primary" type="submit" form="create-model-form">
              Simpan Model
            </Button>
          </>
        }
      >
        <form id="create-model-form" noValidate onSubmit={handleCreateModel} className="space-y-4 text-xs">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">ID Model Kanonik *</label>
            <input
              type="text"
              required
              placeholder="gpt-4o"
              value={newModel.model_id}
              onChange={(e) => setNewModel({ ...newModel, model_id: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Tampilan *</label>
            <input
              type="text"
              required
              placeholder="OpenAI GPT-4o Omni"
              value={newModel.display_name}
              onChange={(e) => setNewModel({ ...newModel, display_name: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Keluarga (Family)</label>
            <input
              type="text"
              placeholder="gpt-4"
              value={newModel.family}
              onChange={(e) => setNewModel({ ...newModel, family: e.target.value })}
              className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Context Window</label>
              <input
                type="number"
                value={newModel.context_window}
                onChange={(e) => setNewModel({ ...newModel, context_window: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Max Output Tokens</label>
              <input
                type="number"
                value={newModel.max_output_tokens}
                onChange={(e) => setNewModel({ ...newModel, max_output_tokens: parseInt(e.target.value) || 0 })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>
          </div>
        </form>
      </Drawer>

      {/* Modal Pricing */}
      <Drawer
        isOpen={isPricingOpen}
        onClose={() => setIsPricingOpen(false)}
        title={`Konfigurasi Harga: ${selectedModel?.display_name || selectedModel?.model_id || 'Model'}`}
        subtitle="Atur tarif token per 1M (USD) untuk model ini pada provider upstream terkait"
        footer={
          mappings.length > 0 ? (
            <>
              <Button variant="ghost" onClick={() => setIsPricingOpen(false)}>
                Batal
              </Button>
              <Button variant="primary" type="submit" form="pricing-form" isLoading={isSavingPrice}>
                Simpan Harga
              </Button>
            </>
          ) : (
            <Button variant="ghost" onClick={() => setIsPricingOpen(false)}>
              Tutup
            </Button>
          )
        }
      >
        {isPricingLoading ? (
          <div className="py-8 text-center text-xs text-text-muted">Memuat pemetaan model...</div>
        ) : mappings.length === 0 ? (
          <div className="py-6 text-center text-xs text-text-muted space-y-2">
            <p>Model ini belum memiliki pemetaan ke upstream provider.</p>
            <p className="text-[11px]">Hubungkan provider terlebih dahulu sebelum mengatur struktur harga.</p>
          </div>
        ) : (
          <form id="pricing-form" noValidate onSubmit={handleSavePrice} className="space-y-4 text-xs">
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
                  description: `Provider: ${provName} | Model Upstream: ${mp.upstream_model_name || '-'}`,
                };
              })}
            />

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5">
                  Input / 1M Token (USD) *
                </label>
                <input
                  type="text"
                  inputMode="decimal"
                  required
                  placeholder="2.50"
                  value={pricingForm.input_per_1m_usd}
                  onChange={(e) => setPricingForm({ ...pricingForm, input_per_1m_usd: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5">
                  Output / 1M Token (USD) *
                </label>
                <input
                  type="text"
                  inputMode="decimal"
                  required
                  placeholder="10.00"
                  value={pricingForm.output_per_1m_usd}
                  onChange={(e) => setPricingForm({ ...pricingForm, output_per_1m_usd: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">
                Cached Input / 1M Token (USD) (Opsional)
              </label>
              <input
                type="text"
                inputMode="decimal"
                placeholder="1.25"
                value={pricingForm.cached_input_per_1m_usd}
                onChange={(e) => setPricingForm({ ...pricingForm, cached_input_per_1m_usd: e.target.value })}
                className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
              />
            </div>

            {pricingHistory.length > 0 && (
              <div className="pt-2">
                <span className="block text-xs font-medium text-text-secondary mb-1.5">
                  Riwayat Penetapan Harga
                </span>
                <div className="max-h-32 overflow-y-auto space-y-1 rounded-lg border border-border p-2.5 bg-bg-surface-2/40">
                  {pricingHistory.map((h, i) => (
                    <div key={`${h.effective_from}-${i}`} className="flex justify-between text-[11px] font-mono text-text-secondary">
                      <span>In: ${h.input_per_1m_usd} | Out: ${h.output_per_1m_usd}</span>
                      <span className="text-text-muted">{h.effective_from ? new Date(h.effective_from).toLocaleDateString() : '-'}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </form>
        )}
      </Drawer>
    </div>
  );
};
