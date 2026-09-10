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
    <div className={cn('bg-bg-surface border border-border rounded-card overflow-hidden', className)}>
      {(title || subtitle || action) && (
        <div className="px-5 py-4 border-b border-border flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
          <div className="min-w-0">
            {title && <h3 className="text-base font-semibold text-text-primary">{title}</h3>}
            {subtitle && <p className="text-xs text-text-secondary mt-0.5">{subtitle}</p>}
          </div>
          {action && <div className="shrink-0 max-w-full overflow-x-auto">{action}</div>}
        </div>
      )}
      <div className="p-5">{children}</div>
    </div>
  );
};
