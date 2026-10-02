import React, { useEffect, useState, useMemo, useCallback } from 'react';
import { api } from '../api/client';
import type { Model, ProviderModel, Price } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { QueryError } from '../components/common/QueryError';
import { Cpu, Plus, Search } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import {
  ModelFilterBar,
  ModelTableView,
  ModelGridView,
  ModelPagination,
  CreateModelDrawer,
  ModelPricingDrawer,
  getModelFamily,
  getErrorMessage,
  calculateFamilyCounts,
  getAvailableFamilies,
  filterModels,
  type ViewMode,
  type CreateModelFormData,
  type PricingFormData,
} from '../components/models';

export { getModelFamily };

export const Models: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [models, setModels] = useState<Model[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState<boolean>(false);
  const [isCreating, setIsCreating] = useState<boolean>(false);
  const [searchQuery, setSearchQuery] = useState<string>('');

  // Tampilan & Navigasi
  const [viewMode, setViewMode] = useState<ViewMode>(() => {
    const saved = localStorage.getItem('routex_models_view_mode');
    return saved === 'grid' ? 'grid' : 'table';
  });
  const [selectedFamily, setSelectedFamily] = useState<string>('all');
  const [currentPage, setCurrentPage] = useState<number>(1);
  const [pageSize, setPageSize] = useState<number>(12);

  const handleViewChange = (mode: ViewMode) => {
    setViewMode(mode);
    localStorage.setItem('routex_models_view_mode', mode);
  };

  // State untuk Modal Pricing
  const [isPricingOpen, setIsPricingOpen] = useState<boolean>(false);
  const [selectedModel, setSelectedModel] = useState<Model | null>(null);
  const [mappings, setMappings] = useState<ProviderModel[]>([]);
  const [selectedMappingId, setSelectedMappingId] = useState<string>('');
  const [pricingHistory, setPricingHistory] = useState<Price[]>([]);
  const [pricingForm, setPricingForm] = useState<PricingFormData>({
    input_per_1m_usd: '',
    output_per_1m_usd: '',
    cached_input_per_1m_usd: '',
  });
  const [isPricingLoading, setIsPricingLoading] = useState<boolean>(false);
  const [isSavingPrice, setIsSavingPrice] = useState<boolean>(false);

  const loadModels = useCallback(async () => {
    setIsLoading(true);
    setLoadError(null);
    try {
      const mRes = await api.models.list();
      setModels(mRes.items || []);
    } catch (err: unknown) {
      setLoadError(getErrorMessage(err));
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadModels();
  }, [loadModels]);

  // Hitung jumlah model per rumpun untuk filter chips
  const familyCounts = useMemo(() => calculateFamilyCounts(models), [models]);
  const availableFamilies = useMemo(() => getAvailableFamilies(familyCounts), [familyCounts]);
  const filteredModels = useMemo(
    () => filterModels(models, searchQuery, selectedFamily),
    [models, searchQuery, selectedFamily]
  );

  useEffect(() => {
    setCurrentPage(1);
  }, [searchQuery, selectedFamily]);

  const totalPages = Math.max(1, Math.ceil(filteredModels.length / pageSize));
  const paginatedModels = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filteredModels.slice(start, start + pageSize);
  }, [filteredModels, currentPage, pageSize]);

  const handleCreateModel = async (formData: CreateModelFormData) => {
    setIsCreating(true);
    try {
      await api.models.create({
        ...formData,
        // CHECK models_capabilities_known hanya mengizinkan: text, vision,
        // reasoning, tools, embeddings. Nilai 'chat'/'streaming' memicu 500.
        capabilities: ['text'],
      });
      setIsCreateOpen(false);
      toast.success('Model kanonik baru berhasil didaftarkan');
      await loadModels();
    } catch (err: unknown) {
      toast.error('Gagal membuat model: ' + getErrorMessage(err));
    } finally {
      setIsCreating(false);
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
      await loadModels();
    } catch (err: unknown) {
      toast.error('Gagal menghapus model: ' + getErrorMessage(err));
    }
  };

  const loadPricingForMapping = useCallback(async (mappingId: string) => {
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
    } catch (err: unknown) {
      console.error('Gagal memuat riwayat harga:', err);
    }
  }, []);

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
    } catch (err: unknown) {
      console.error('Gagal memuat detail pemetaan model:', err);
    } finally {
      setIsPricingLoading(false);
    }
  };

  const handleSavePrice = async () => {
    if (!selectedMappingId) return;
    setIsSavingPrice(true);
    try {
      await api.models.setPrice(selectedMappingId, {
        input_per_1m_usd: pricingForm.input_per_1m_usd,
        output_per_1m_usd: pricingForm.output_per_1m_usd,
        cached_input_per_1m_usd: pricingForm.cached_input_per_1m_usd || undefined,
      });
      toast.success('Harga model berhasil disimpan');
      await loadPricingForMapping(selectedMappingId);
    } catch (err: unknown) {
      toast.error('Gagal menyimpan harga model: ' + getErrorMessage(err));
    } finally {
      setIsSavingPrice(false);
    }
  };

  const existingModelIds = useMemo(() => models.map((m) => m.model_id), [models]);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Model Registry & Pricing"
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
      <ModelFilterBar
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        viewMode={viewMode}
        onViewModeChange={handleViewChange}
        selectedFamily={selectedFamily}
        onSelectFamily={setSelectedFamily}
        availableFamilies={availableFamilies}
        familyCounts={familyCounts}
        totalCount={models.length}
        filteredCount={filteredModels.length}
      />

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4" data-testid="models-loading-skeleton">
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
        <Card className="p-10 text-center rounded-box bg-bg-surface-1 border border-border/60" data-testid="models-empty-state">
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
        <Card className="p-10 text-center rounded-box bg-bg-surface-1 border border-border/60" data-testid="models-filter-empty-state">
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
            <ModelTableView
              models={paginatedModels}
              onOpenPricing={handleOpenPricing}
              onDeleteModel={handleDeleteModel}
            />
          ) : (
            <ModelGridView
              models={paginatedModels}
              onOpenPricing={handleOpenPricing}
              onDeleteModel={handleDeleteModel}
            />
          )}

          {/* Kontrol Paginasi */}
          <ModelPagination
            currentPage={currentPage}
            pageSize={pageSize}
            totalItems={filteredModels.length}
            totalPages={totalPages}
            onPageChange={setCurrentPage}
            onPageSizeChange={(newSize) => {
              setPageSize(newSize);
              setCurrentPage(1);
            }}
            totalUnfilteredItems={models.length}
          />
        </div>
      )}

      {/* Drawer Create Model */}
      <CreateModelDrawer
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        onSubmit={handleCreateModel}
        isSubmitting={isCreating}
        existingModelIds={existingModelIds}
      />

      {/* Drawer Pricing */}
      <ModelPricingDrawer
        isOpen={isPricingOpen}
        onClose={() => setIsPricingOpen(false)}
        model={selectedModel}
        mappings={mappings}
        selectedMappingId={selectedMappingId}
        onSelectMappingId={(mappingId) => {
          setSelectedMappingId(mappingId);
          void loadPricingForMapping(mappingId);
        }}
        pricingHistory={pricingHistory}
        pricingForm={pricingForm}
        onPricingFormChange={setPricingForm}
        onSavePrice={handleSavePrice}
        isLoading={isPricingLoading}
        isSaving={isSavingPrice}
      />
    </div>
  );
};
