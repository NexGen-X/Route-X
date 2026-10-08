import React from 'react';
import { cn } from '../../utils/cn';

export interface PageHeaderProps {
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
}

export const PageHeader: React.FC<PageHeaderProps> = ({
  title,
  description,
  actions,
  className = '',
}) => {
  return (
    <div
      className={cn(
        'flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-6 pb-2',
        className
      )}
    >
      <div className="min-w-0 space-y-1">
        <h2 className="text-lg sm:text-xl font-bold tracking-tight text-white flex items-center gap-2.5">
          {title}
        </h2>
        {description && (
          <p className="text-xs sm:text-sm text-text-muted mt-0.5 leading-relaxed font-normal">
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex flex-wrap sm:flex-nowrap items-center gap-2 sm:gap-2.5 shrink-0">
          {actions}
        </div>
      )}
    </div>
  );
};
