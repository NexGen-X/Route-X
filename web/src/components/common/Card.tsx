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
    <div className={cn('bg-bg-surface border border-border rounded-card overflow-hidden shadow-sm transition-all', className)}>
      {(title || action) && (
        <div className="px-6 py-4.5 border-b border-border/70 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
          <div className="min-w-0">
            {title && <h3 className="text-base font-semibold text-text-primary tracking-tight">{title}</h3>}
          </div>
          {action && <div className="shrink-0 max-w-full overflow-x-auto">{action}</div>}
        </div>
      )}
      <div className="p-6">{children}</div>
    </div>
  );
};
