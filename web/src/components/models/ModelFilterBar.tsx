import React from 'react';
import { Search, X, Table, LayoutGrid, Layers } from 'lucide-react';
import type { ModelFilterBarProps } from './types';

export const ModelFilterBar: React.FC<ModelFilterBarProps> = ({
  searchQuery,
  onSearchChange,
  viewMode,
  onViewModeChange,
  selectedFamily,
  onSelectFamily,
  availableFamilies,
  familyCounts,
  totalCount,
  filteredCount,
}) => {
  return (
    <div className="space-y-3">
      {/* Toolbar Pencarian & View Mode */}
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 bg-bg-surface-1 p-3 rounded-xl border border-border">
        <div className="relative flex-1">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
          <input
            type="text"
            aria-label="Cari model"
            placeholder="Cari model berdasarkan nama, slug, provider, atau family (mis. deepseek, claude, tknharbor)..."
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            className="w-full pl-9 pr-9 py-2 bg-bg-surface-2 border border-border rounded-lg text-xs text-white placeholder:text-text-muted focus:outline-none focus:border-accent"
          />
          {searchQuery && (
            <button
              type="button"
              onClick={() => onSearchChange('')}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 p-1 text-text-muted hover:text-white rounded-md cursor-pointer transition-colors"
              aria-label="Hapus pencarian"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>

        <div className="flex items-center justify-between sm:justify-end gap-3 shrink-0">
          <span data-testid="models-counter" className="text-xs font-mono text-text-muted">
            <strong className="text-white">{filteredCount}</strong> dari {totalCount} model
          </span>

          {/* View Switcher */}
          <div className="flex items-center bg-bg-surface-2 p-0.5 rounded-lg border border-border" role="group" aria-label="Pilihan tampilan model">
            <button
              type="button"
              onClick={() => onViewModeChange('table')}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md cursor-pointer transition-all ${
                viewMode === 'table'
                  ? 'bg-accent text-white shadow-sm font-semibold'
                  : 'text-text-muted hover:text-white'
              }`}
              title="Tampilan Tabel Ringkas"
              aria-label="Tampilan Tabel"
              aria-pressed={viewMode === 'table'}
            >
              <Table className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Tabel</span>
            </button>
            <button
              type="button"
              onClick={() => onViewModeChange('grid')}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md cursor-pointer transition-all ${
                viewMode === 'grid'
                  ? 'bg-accent text-white shadow-sm font-semibold'
                  : 'text-text-muted hover:text-white'
              }`}
              title="Tampilan Kartu Grid"
              aria-label="Tampilan Grid"
              aria-pressed={viewMode === 'grid'}
            >
              <LayoutGrid className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Kartu</span>
            </button>
          </div>
        </div>
      </div>

      {/* Filter Rumpun / Family Chips Bar */}
      {availableFamilies.length > 2 && (
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs no-scrollbar" role="region" aria-label="Filter keluarga model">
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
                data-testid={`family-chip-${fam.toLowerCase()}`}
                onClick={() => onSelectFamily(fam)}
                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium shrink-0 cursor-pointer transition-colors ${
                  isSelected
                    ? 'bg-accent text-white shadow-sm'
                    : 'bg-bg-surface-1 border border-border text-text-muted hover:text-white hover:bg-bg-surface-2'
                }`}
                aria-pressed={isSelected}
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
    </div>
  );
};
