import React from 'react';
import { Search } from 'lucide-react';
import type { RequestTabFilter } from './utils';

export interface RequestFilterBarProps {
  activeTab: RequestTabFilter;
  onTabChange: (tab: RequestTabFilter) => void;
  search: string;
  onSearchChange: (value: string) => void;
  onSearchSubmit?: () => void;
}

const TABS: Array<{ id: RequestTabFilter; label: string }> = [
  { id: 'all', label: 'All Requests' },
  { id: 'success', label: 'Success (2xx)' },
  { id: 'errors', label: 'Errors (4xx/5xx)' },
];

export const RequestFilterBar: React.FC<RequestFilterBarProps> = ({
  activeTab,
  onTabChange,
  search,
  onSearchChange,
  onSearchSubmit,
}) => {
  return (
    <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
      {/* Filter Tabs */}
      <div className="flex flex-wrap items-center gap-1.5">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => onTabChange(tab.id)}
            className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-all duration-150 cursor-pointer ${
              activeTab === tab.id
                ? 'bg-white/10 text-white border border-white/[0.08] shadow-sm font-semibold'
                : 'bg-white/[0.02] text-zinc-400 border border-white/[0.06] hover:text-white hover:bg-white/[0.04]'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Search Input Bar */}
      <div className="relative w-full md:w-72">
        <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500 pointer-events-none" />
        <input
          type="text"
          placeholder="Cari ID request, model, IP..."
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && onSearchSubmit?.()}
          className="w-full pl-8 pr-3 py-1.5 bg-white/[0.02] border border-white/[0.06] rounded-lg text-xs text-white placeholder:text-zinc-500 focus:outline-none focus:border-white/[0.15] transition-colors"
        />
      </div>
    </div>
  );
};
