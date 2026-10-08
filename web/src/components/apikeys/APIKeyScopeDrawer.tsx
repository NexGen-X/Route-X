import React from 'react';
import { Layers, Search, CheckSquare, Square } from 'lucide-react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Checkbox } from '../common/Checkbox';
import type { ScopeOption } from './types';
import type { APIKey } from '../../types';

export interface APIKeyScopeDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  selectedKey: APIKey | null;
  scopeOptions: ScopeOption[];
  filteredScopeOptions: ScopeOption[];
  selectedScopeIds: string[];
  scopeSearch: string;
  setScopeSearch: (val: string) => void;
  onToggleScopeItem: (id: string) => void;
  onSelectAll: () => void;
  onDeselectAll: () => void;
  onSave: (e: React.FormEvent) => void;
  isLoading?: boolean;
}

export const APIKeyScopeDrawer: React.FC<APIKeyScopeDrawerProps> = ({
  isOpen,
  onClose,
  selectedKey,
  scopeOptions,
  filteredScopeOptions,
  selectedScopeIds,
  scopeSearch,
  setScopeSearch,
  onToggleScopeItem,
  onSelectAll,
  onDeselectAll,
  onSave,
  isLoading = false,
}) => {
  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title={
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-lg bg-accent/15 text-accent flex items-center justify-center">
            <Layers className="w-4 h-4" aria-hidden="true" />
          </div>
          <span>Allowed Scope Model & Provider</span>
        </div>
      }
      subtitle={
        selectedKey
          ? `Batasi model atau upstream provider yang diizinkan untuk ${selectedKey.name}. Biarkan kosong untuk mengizinkan seluruh model.`
          : undefined
      }
      maxWidth="xl"
      footer={
        <div className="flex items-center justify-between w-full">
          <span className="text-xs text-text-muted">
            {selectedScopeIds.length === 0
              ? 'Akses Bebas (Seluruh model diizinkan)'
              : `${selectedScopeIds.length} target dipilih`}
          </span>
          <div className="flex items-center gap-2">
            <Button variant="ghost" onClick={onClose} className="min-h-[44px]">
              Batal
            </Button>
            <Button
              variant="primary"
              onClick={onSave}
              isLoading={isLoading}
              className="min-h-[44px] px-5 font-semibold"
            >
              Simpan Hak Akses
            </Button>
          </div>
        </div>
      }
    >
      <div className="space-y-4">
        {/* Search & Bulk Select Controls */}
        <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3">
          <div className="relative flex-1">
            <Search className="w-4 h-4 absolute left-3 top-3 text-text-muted" aria-hidden="true" />
            <input
              type="text"
              placeholder="Cari model atau penyedia AI..."
              value={scopeSearch}
              onChange={(e) => setScopeSearch(e.target.value)}
              className="w-full bg-bg-surface-2 border border-border rounded-nav pl-9 pr-3 py-2 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none"
            />
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={onSelectAll}
              icon={<CheckSquare className="w-3.5 h-3.5" aria-hidden="true" />}
              className="text-xs flex-1 sm:flex-initial"
            >
              Pilih Semua
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={onDeselectAll}
              icon={<Square className="w-3.5 h-3.5" aria-hidden="true" />}
              className="text-xs flex-1 sm:flex-initial"
            >
              Reset
            </Button>
          </div>
        </div>

        {/* Scope Options List */}
        <div className="border border-border rounded-xl divide-y divide-border/60 max-h-[500px] overflow-y-auto bg-bg-surface-2/40">
          {filteredScopeOptions.length === 0 ? (
            <div className="p-8 text-center text-xs text-text-muted">
              Tidak ada model atau provider yang cocok dengan kriteria pencarian.
            </div>
          ) : (
            filteredScopeOptions.map((opt) => {
              const checked = selectedScopeIds.includes(opt.id);
              return (
                <label
                  key={opt.id}
                  className="flex items-center justify-between p-3.5 hover:bg-bg-surface-2/80 transition-colors cursor-pointer group"
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <Checkbox
                      checked={checked}
                      onChange={() => onToggleScopeItem(opt.id)}
                      aria-label={`Pilih model ${opt.modelDisplayName} dari provider ${opt.providerDisplayName}`}
                    />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-xs text-white group-hover:text-accent transition-colors truncate">
                          {opt.modelDisplayName}
                        </span>
                        {opt.family && (
                          <span className="px-1.5 py-0.2 rounded text-[10px] font-mono bg-bg-surface-3 text-text-muted border border-border">
                            {opt.family}
                          </span>
                        )}
                      </div>
                      <span className="text-[11px] text-text-muted font-mono block truncate">
                        Provider: {opt.providerDisplayName} ({opt.modelSlug})
                      </span>
                    </div>
                  </div>
                </label>
              );
            })
          )}
        </div>
      </div>
    </Drawer>
  );
};
