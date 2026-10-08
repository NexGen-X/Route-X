import React from 'react';
import { Search, Filter, Clock, Server, CheckCircle2, AlertCircle, X } from 'lucide-react';
import type { RequestTabFilter } from './utils';

export interface RequestFilterBarProps {
  activeTab: RequestTabFilter;
  onTabChange: (tab: RequestTabFilter) => void;
  search: string;
  onSearchChange: (value: string) => void;
  onSearchSubmit?: () => void;
  providerFilter?: string;
  onProviderFilterChange?: (provider: string) => void;
  statusFilter?: string;
  onStatusFilterChange?: (status: string) => void;
  timeRangeFilter?: string;
  onTimeRangeFilterChange?: (range: string) => void;
  onResetFilters?: () => void;
}

const TABS: Array<{ id: RequestTabFilter; label: string }> = [
  { id: 'all', label: 'All Requests' },
  { id: 'success', label: 'Success (2xx)' },
  { id: 'errors', label: 'Errors (4xx/5xx)' },
];

const PROVIDER_OPTIONS = [
  { value: '', label: 'Semua Provider' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'google', label: 'Google Gemini' },
  { value: 'groq', label: 'Groq Cloud' },
  { value: 'mistral', label: 'Mistral AI' },
  { value: 'ollama', label: 'Ollama Local' },
];

const STATUS_OPTIONS = [
  { value: '', label: 'Semua Status' },
  { value: '200', label: '200 OK' },
  { value: '400', label: '400 Bad Request' },
  { value: '401', label: '401 Unauthorized' },
  { value: '429', label: '429 Rate Limited' },
  { value: '500', label: '500 Server Error' },
  { value: '502', label: '502 Bad Gateway' },
  { value: '504', label: '504 Timeout' },
];

const TIME_RANGES = [
  { value: '', label: 'Semua Waktu' },
  { value: '15m', label: '15 Menit Terakhir' },
  { value: '1h', label: '1 Jam Terakhir' },
  { value: '24h', label: '24 Jam Terakhir' },
  { value: '7d', label: '7 Hari Terakhir' },
];

export const RequestFilterBar: React.FC<RequestFilterBarProps> = ({
  activeTab,
  onTabChange,
  search,
  onSearchChange,
  onSearchSubmit,
  providerFilter = '',
  onProviderFilterChange,
  statusFilter = '',
  onStatusFilterChange,
  timeRangeFilter = '',
  onTimeRangeFilterChange,
  onResetFilters,
}) => {
  const hasActiveFilters = Boolean(
    search ||
    providerFilter ||
    statusFilter ||
    timeRangeFilter ||
    activeTab !== 'all'
  );

  return (
    <div className="space-y-3 pb-4">
      {/* Baris Atas: Tabs Filter Utama & Pencarian Cepat */}
      <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3">
        {/* Filter Tabs */}
        <div className="flex flex-wrap items-center gap-2" role="tablist" aria-label="Filter status request">
          {TABS.map((tab) => {
            const isActive = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={isActive}
                aria-label={`Filter ${tab.label}`}
                onClick={() => onTabChange(tab.id)}
                className={`min-h-[44px] sm:min-h-[36px] px-3.5 py-1.5 rounded-chip text-xs font-semibold transition-all inline-flex items-center gap-1.5 cursor-pointer ${
                  isActive
                    ? 'bg-accent/15 text-blue-400 border border-accent/40 shadow-sm shadow-blue-500/10'
                    : 'bg-bg-surface-2 text-text-secondary border border-border hover:border-border-hover hover:text-white'
                }`}
              >
                {tab.id === 'success' && <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" />}
                {tab.id === 'errors' && <AlertCircle className="w-3.5 h-3.5 text-rose-400" aria-hidden="true" />}
                <span>{tab.label}</span>
              </button>
            );
          })}
        </div>

        {/* Search Input Bar */}
        <div className="relative w-full md:w-80">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-muted pointer-events-none" aria-hidden="true" />
          <input
            type="text"
            placeholder="Cari ID request, model, IP..."
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && onSearchSubmit?.()}
            aria-label="Cari request berdasarkan ID, model, atau IP"
            className="w-full min-h-[44px] sm:min-h-[36px] pl-9 pr-8 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-text-primary placeholder:text-text-muted/60 focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent/40 transition-colors"
          />
          {search && (
            <button
              type="button"
              onClick={() => {
                onSearchChange('');
                onSearchSubmit?.();
              }}
              aria-label="Hapus kata kunci pencarian"
              className="absolute right-2.5 top-1/2 -translate-y-1/2 p-1 text-text-muted hover:text-white rounded-md transition-colors"
            >
              <X className="w-3.5 h-3.5" aria-hidden="true" />
            </button>
          )}
        </div>
      </div>

      {/* Baris Bawah: Filter Tajam (Provider, Status Code, Rentang Waktu) */}
      <div className="flex flex-wrap items-center gap-2 pt-1 border-t border-border/40 text-xs">
        <div className="flex items-center gap-1.5 text-text-muted font-medium text-[11px] mr-1">
          <Filter className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
          <span>Filter Lanjutan:</span>
        </div>

        {/* Dropdown Provider */}
        {onProviderFilterChange && (
          <div className="relative inline-flex items-center">
            <Server className="w-3.5 h-3.5 absolute left-2.5 text-text-muted pointer-events-none" aria-hidden="true" />
            <select
              value={providerFilter}
              onChange={(e) => onProviderFilterChange(e.target.value)}
              aria-label="Filter berdasarkan upstream provider"
              className="min-h-[44px] sm:min-h-[32px] pl-7 pr-7 py-1 bg-bg-surface-2 border border-border rounded-inner text-[11px] font-mono text-text-primary focus:outline-none focus:border-accent appearance-none cursor-pointer hover:border-border-hover transition-colors"
            >
              {PROVIDER_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value} className="bg-bg-surface text-text-primary">
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Dropdown Status Code */}
        {onStatusFilterChange && (
          <div className="relative inline-flex items-center">
            <select
              value={statusFilter}
              onChange={(e) => onStatusFilterChange(e.target.value)}
              aria-label="Filter berdasarkan kode status HTTP"
              className="min-h-[44px] sm:min-h-[32px] px-3 py-1 bg-bg-surface-2 border border-border rounded-inner text-[11px] font-mono text-text-primary focus:outline-none focus:border-accent appearance-none cursor-pointer hover:border-border-hover transition-colors"
            >
              {STATUS_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value} className="bg-bg-surface text-text-primary">
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Dropdown Rentang Waktu */}
        {onTimeRangeFilterChange && (
          <div className="relative inline-flex items-center">
            <Clock className="w-3.5 h-3.5 absolute left-2.5 text-text-muted pointer-events-none" aria-hidden="true" />
            <select
              value={timeRangeFilter}
              onChange={(e) => onTimeRangeFilterChange(e.target.value)}
              aria-label="Filter berdasarkan rentang waktu"
              className="min-h-[44px] sm:min-h-[32px] pl-7 pr-7 py-1 bg-bg-surface-2 border border-border rounded-inner text-[11px] font-mono text-text-primary focus:outline-none focus:border-accent appearance-none cursor-pointer hover:border-border-hover transition-colors"
            >
              {TIME_RANGES.map((opt) => (
                <option key={opt.value} value={opt.value} className="bg-bg-surface text-text-primary">
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Reset Filter Button bila ada filter yang aktif */}
        {hasActiveFilters && onResetFilters && (
          <button
            type="button"
            onClick={onResetFilters}
            aria-label="Reset semua filter permintaan"
            className="min-h-[44px] sm:min-h-[32px] px-2.5 py-1 text-[11px] text-text-muted hover:text-rose-400 hover:bg-rose-500/10 rounded-inner transition-colors ml-auto inline-flex items-center gap-1 cursor-pointer"
          >
            <X className="w-3 h-3" aria-hidden="true" />
            <span>Reset Filter</span>
          </button>
        )}
      </div>
    </div>
  );
};
