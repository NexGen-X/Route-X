import React from 'react';
import { CheckCircle2, Terminal, Search, Filter } from 'lucide-react';
import { Select } from '../common/Select';

export interface CLIFilterToolbarProps {
  mainTab: 'installed' | 'all';
  setMainTab: (tab: 'installed' | 'all') => void;
  installedCount: number;
  totalCount: number;
  searchQuery: string;
  setSearchQuery: (val: string) => void;
  selectedCategory: string;
  setSelectedCategory: (cat: string) => void;
  categories: string[];
}

export const CLIFilterToolbar: React.FC<CLIFilterToolbarProps> = ({
  mainTab,
  setMainTab,
  installedCount,
  totalCount,
  searchQuery,
  setSearchQuery,
  selectedCategory,
  setSelectedCategory,
  categories,
}) => {
  return (
    <div className="flex flex-col sm:flex-row gap-3 items-center justify-between bg-bg-surface p-3.5 rounded-box border border-border">
      {/* Tab Utama: Terpasang vs Semua */}
      <div className="flex items-center gap-2 w-full sm:w-auto">
        <div className="flex p-1 bg-bg-surface-2 rounded-inner border border-border">
          <button
            type="button"
            onClick={() => setMainTab('installed')}
            className={`text-xs px-3 py-1.5 rounded-md font-semibold flex items-center gap-1.5 transition-all ${
              mainTab === 'installed'
                ? 'bg-accent text-black shadow'
                : 'text-text-muted hover:text-white'
            }`}
          >
            <CheckCircle2 className="w-3.5 h-3.5" />
            CLI Terpasang
            <span
              className={`px-1.5 py-0.2 rounded-full text-[10px] ${
                mainTab === 'installed'
                  ? 'bg-black/20 text-black font-bold'
                  : 'bg-bg-surface-3 text-text-muted'
              }`}
            >
              {installedCount}
            </span>
          </button>

          <button
            type="button"
            onClick={() => setMainTab('all')}
            className={`text-xs px-3 py-1.5 rounded-md font-semibold flex items-center gap-1.5 transition-all ${
              mainTab === 'all'
                ? 'bg-accent text-black shadow'
                : 'text-text-muted hover:text-white'
            }`}
          >
            <Terminal className="w-3.5 h-3.5" />
            Katalog Lengkap
            <span
              className={`px-1.5 py-0.2 rounded-full text-[10px] ${
                mainTab === 'all'
                  ? 'bg-black/20 text-black font-bold'
                  : 'bg-bg-surface-3 text-text-muted'
              }`}
            >
              {totalCount}
            </span>
          </button>
        </div>
      </div>

      {/* Search & Category Filter */}
      <div className="flex items-center gap-3 w-full sm:w-auto">
        <div className="relative w-full sm:w-64">
          <Search className="w-4 h-4 absolute left-3 top-2.5 text-text-muted" />
          <input
            type="text"
            placeholder="Cari (misal: claude, opencode)..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full bg-bg-surface-2 border border-border rounded-md pl-9 pr-3 py-1.5 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none"
          />
        </div>

        <div className="flex items-center gap-1.5 min-w-[170px] sm:min-w-[190px]">
          <Filter className="w-3.5 h-3.5 text-text-muted flex-shrink-0" />
          <div className="flex-1">
            <Select
              variant="compact"
              value={selectedCategory}
              onChange={(val) => setSelectedCategory(val)}
              options={[
                { value: 'all', label: 'Semua Kategori' },
                ...categories
                  .filter((c) => c !== 'all')
                  .map((cat) => ({
                    value: cat,
                    label: cat,
                  })),
              ]}
              aria-label="Filter kategori alat CLI"
            />
          </div>
        </div>
      </div>
    </div>
  );
};
