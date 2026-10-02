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
            className={`px-3 py-1.5 rounded-inner text-xs font-medium transition-all ${
              activeTab === tab.id
                ? 'bg-accent/10 text-accent border border-accent/30 shadow-sm font-semibold'
                : 'bg-bg-surface-2 text-text-secondary border border-border hover:text-white'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Search Input Bar */}
      <div className="relative w-full md:w-72">
        <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-muted pointer-events-none" />
        <input
          type="text"
          placeholder="Cari ID request, model, IP..."
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && onSearchSubmit?.()}
          className="w-full pl-9 pr-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-xs text-text-primary focus:outline-none focus:border-accent"
        />
      </div>
    </div>
  );
};
