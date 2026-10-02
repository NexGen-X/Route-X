import React from 'react';
import { cn } from '../../utils/cn';

interface CardProps {
  children: React.ReactNode;
  className?: string;
  title?: string;
  subtitle?: string;
  action?: React.ReactNode;
}

export const Card: React.FC<CardProps> = ({
  children,
  className = '',
  title,
  subtitle,
  action,
}) => {
  return (
    <div className={cn('bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] hover:border-white/[0.12] rounded-xl overflow-hidden shadow-sm transition-all duration-200 ease-out', className)}>
      {(title || action) && (
        <div className="px-5 py-3.5 border-b border-white/[0.06] flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
          <div className="min-w-0">
            {title && <h3 className="text-sm font-semibold text-white tracking-tight">{title}</h3>}
            {subtitle && <p className="text-xs text-zinc-400 mt-0.5">{subtitle}</p>}
          </div>
          {action && <div className="shrink-0 max-w-full overflow-x-auto">{action}</div>}
        </div>
      )}
      <div className="p-5">{children}</div>
    </div>
  );
};
