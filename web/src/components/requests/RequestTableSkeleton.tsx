import React from 'react';

export interface RequestTableSkeletonProps {
  rows?: number;
}

export const RequestTableSkeleton: React.FC<RequestTableSkeletonProps> = ({
  rows = 8,
}) => {
  return (
    <div className="bg-bg-surface border border-border rounded-card overflow-hidden shadow-sm">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs whitespace-nowrap">
          <thead>
            <tr className="border-b border-border bg-bg-surface-1 text-text-muted font-medium text-xs">
              <th className="py-2.5 px-4 font-semibold">Timestamp</th>
              <th className="py-2.5 px-4 font-semibold">Method</th>
              <th className="py-2.5 px-4 font-semibold">Path</th>
              <th className="py-2.5 px-4 font-semibold">Status</th>
              <th className="py-2.5 px-4 font-semibold">Model</th>
              <th className="py-2.5 px-4 font-semibold">Latency</th>
              <th className="py-2.5 px-4 font-semibold">Cost</th>
              <th className="py-2.5 px-4 text-right font-semibold">Aksi</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {Array.from({ length: rows }).map((_, i) => (
              <tr key={i} className="animate-pulse">
                <td className="py-2.5 px-4">
                  <div className="h-3.5 bg-bg-surface-2 rounded w-20 mb-1" />
                  <div className="h-2.5 bg-bg-surface-2/60 rounded w-16" />
                </td>
                <td className="py-2.5 px-4">
                  <div className="h-4 bg-bg-surface-2 rounded w-10" />
                </td>
                <td className="py-2.5 px-4">
                  <div className="h-3.5 bg-bg-surface-2 rounded w-36 mb-1" />
                  <div className="h-2.5 bg-bg-surface-2/60 rounded w-20" />
                </td>
                <td className="py-2.5 px-4">
                  <div className="h-5 bg-bg-surface-2 rounded-inner w-12" />
                </td>
                <td className="py-2.5 px-4">
                  <div className="h-3.5 bg-bg-surface-2 rounded w-28 mb-1" />
                  <div className="h-2.5 bg-bg-surface-2/60 rounded w-20" />
                </td>
                <td className="py-2.5 px-4">
                  <div className="h-3.5 bg-bg-surface-2 rounded w-16 mb-1" />
                  <div className="h-2.5 bg-bg-surface-2/60 rounded w-12" />
                </td>
                <td className="py-2.5 px-4">
                  <div className="h-3.5 bg-bg-surface-2 rounded w-16 mb-1" />
                  <div className="h-2.5 bg-bg-surface-2/60 rounded w-12" />
                </td>
                <td className="py-2.5 px-4 text-right">
                  <div className="h-3.5 bg-bg-surface-2 rounded w-10 ml-auto" />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};
