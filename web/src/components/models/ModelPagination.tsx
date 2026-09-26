import React from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Select } from '../common/Select';
import type { ModelPaginationProps } from './types';

export const ModelPagination: React.FC<ModelPaginationProps> = ({
  currentPage,
  pageSize,
  totalItems,
  totalPages,
  onPageChange,
  onPageSizeChange,
  totalUnfilteredItems,
}) => {
  if (totalItems === 0) return null;

  const startItem = Math.min((currentPage - 1) * pageSize + 1, totalItems);
  const endItem = Math.min(currentPage * pageSize, totalItems);

  return (
    <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-3 px-1 text-xs text-text-muted border-t border-border/50" data-testid="model-pagination">
      <div className="flex items-center gap-2">
        <span>
          Menampilkan <strong className="text-white font-mono">{startItem}</strong> - <strong className="text-white font-mono">{endItem}</strong> dari <strong className="text-white font-mono">{totalItems}</strong> model
          {totalUnfilteredItems !== undefined && totalUnfilteredItems !== totalItems && (
            <span className="text-text-muted"> (difilter dari total {totalUnfilteredItems})</span>
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
                onPageSizeChange(Number(val));
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
            onClick={() => onPageChange(Math.max(1, currentPage - 1))}
            className="p-1.5 rounded-lg border border-border bg-bg-surface-2 text-text-muted hover:text-white disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            aria-label="Halaman sebelumnya"
          >
            <ChevronLeft className="w-4 h-4" />
          </button>

          <span className="px-2.5 py-1 text-xs font-mono text-white bg-bg-surface-2 border border-border rounded-lg" aria-label={`Halaman saat ini ${currentPage} dari ${totalPages}`}>
            {currentPage} / {totalPages}
          </span>

          <button
            type="button"
            disabled={currentPage >= totalPages}
            onClick={() => onPageChange(Math.min(totalPages, currentPage + 1))}
            className="p-1.5 rounded-lg border border-border bg-bg-surface-2 text-text-muted hover:text-white disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            aria-label="Halaman berikutnya"
          >
            <ChevronRight className="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>
  );
};
