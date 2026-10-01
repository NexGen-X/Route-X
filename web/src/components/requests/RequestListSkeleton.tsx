import React from 'react';

export interface RequestListSkeletonProps {
  count?: number;
}

export const RequestListSkeleton: React.FC<RequestListSkeletonProps> = ({ count = 6 }) => {
  return (
    <div className="flex flex-col gap-3">
      {Array.from({ length: count }).map((_, i) => (
        <div
          key={i}
          className="p-4 bg-bg-surface/40 backdrop-blur-md rounded-2xl border border-border/10 flex flex-col sm:flex-row gap-4 sm:items-center animate-[shimmer_1.5s_infinite] bg-gradient-to-r from-bg-surface-2 via-bg-surface-3 to-bg-surface-2 bg-[length:400%_100%]"
        >
          <div className="flex-1 space-y-2 w-full sm:min-w-[200px]">
            <div className="flex items-center gap-2">
              <div className="w-4 h-4 bg-bg-surface-2 rounded-full shrink-0" />
              <div className="w-24 h-4 bg-bg-surface-2 rounded" />
            </div>
            <div className="w-32 h-3 bg-bg-surface-2 rounded ml-6" />
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 sm:flex sm:flex-row gap-4 sm:gap-6 flex-1 w-full">
            <div className="flex-1 space-y-2">
              <div className="w-24 sm:w-28 h-4 bg-bg-surface-2 rounded" />
              <div className="w-16 sm:w-20 h-3 bg-bg-surface-2 rounded" />
            </div>
            <div className="flex-1 space-y-2">
              <div className="w-16 h-4 bg-bg-surface-2 rounded" />
              <div className="w-24 h-3 bg-bg-surface-2 rounded" />
            </div>
            <div className="flex-1 space-y-2 col-span-2 sm:col-span-1">
              <div className="w-12 h-4 bg-bg-surface-2 rounded" />
              <div className="w-20 h-3 bg-bg-surface-2 rounded" />
            </div>
          </div>
          <div className="absolute top-4 right-4 sm:static sm:w-[80px] flex justify-end">
             <div className="w-8 h-8 rounded-full bg-bg-surface-2" />
          </div>
        </div>
      ))}
    </div>
  );
};
