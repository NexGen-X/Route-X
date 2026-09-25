import React from 'react';

export interface RequestTableSkeletonProps {
  mobileCount?: number;
  desktopCount?: number;
}

export const RequestTableSkeleton: React.FC<RequestTableSkeletonProps> = ({
  mobileCount = 5,
  desktopCount = 6,
}) => {
  return (
    <div className="p-4 sm:p-0">
      {/* Mobile skeleton */}
      <div className="sm:hidden space-y-3">
        {Array.from({ length: mobileCount }).map((_, i) => (
          <div
            key={i}
            className="p-3.5 bg-bg-surface-2/40 border border-border/40 rounded-lg animate-pulse space-y-2"
          >
            <div className="flex justify-between items-center">
              <div className="h-5 bg-bg-surface-2 rounded w-28" />
              <div className="h-4 bg-bg-surface-2 rounded w-16" />
            </div>
            <div className="h-4 bg-bg-surface-2 rounded w-40" />
            <div className="h-3 bg-bg-surface-2 rounded w-24" />
          </div>
        ))}
      </div>

      {/* Desktop table skeleton */}
      <div className="hidden sm:block overflow-x-auto">
        <table className="w-full text-left text-xs">
          <thead>
            <tr className="border-b border-border text-text-muted font-medium text-xs bg-bg-surface-2/40">
              <th className="py-3 px-4 font-semibold">Status &amp; Request ID</th>
              <th className="py-3 px-4 font-semibold">Model &amp; Provider</th>
              <th className="py-3 px-4 font-semibold">Token &amp; Cost</th>
              <th className="py-3 px-4 font-semibold">Durasi &amp; TTFT</th>
              <th className="py-3 px-4 font-semibold">Waktu &amp; Klien</th>
              <th className="py-3 px-4 text-right">Aksi</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {Array.from({ length: desktopCount }).map((_, i) => (
              <tr key={i} className="animate-pulse">
                <td className="py-3 px-4">
                  <div className="h-4 bg-bg-surface-2 rounded w-28" />
                </td>
                <td className="py-3 px-4">
                  <div className="h-4 bg-bg-surface-2 rounded w-36" />
                </td>
                <td className="py-3 px-4">
                  <div className="h-4 bg-bg-surface-2 rounded w-20" />
                </td>
                <td className="py-3 px-4">
                  <div className="h-4 bg-bg-surface-2 rounded w-16" />
                </td>
                <td className="py-3 px-4">
                  <div className="h-4 bg-bg-surface-2 rounded w-24" />
                </td>
                <td className="py-3 px-4 text-right">
                  <div className="h-4 bg-bg-surface-2 rounded w-12 ml-auto" />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};
