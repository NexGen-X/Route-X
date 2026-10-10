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
        'flex flex-col gap-2.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4 pb-1',
        className
      )}
    >
      <div className="min-w-0">
        <h2 className="text-base sm:text-lg font-semibold tracking-tight text-zinc-100 flex items-center gap-2">
          {title}
        </h2>
        {description && (
          <p className="text-xs text-zinc-400 mt-0.5 leading-relaxed font-normal">
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex flex-wrap sm:flex-nowrap items-center gap-2 shrink-0">
          {actions}
        </div>
      )}
    </div>
  );
};
