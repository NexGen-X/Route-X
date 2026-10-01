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
    <div className={cn('bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-card overflow-hidden shadow-sm transition-all', className)}>
      {(title || action) && (
        <div className="px-5 py-3.5 border-b border-border/50 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4 hover:-translate-y-0.5 hover:shadow-xl">
          <div className="min-w-0 hover:-translate-y-0.5 hover:shadow-xl">
            {title && <h3 className="text-sm font-semibold text-text-primary tracking-tight hover:-translate-y-0.5 hover:shadow-xl">{title}</h3>}
          </div>
          {action && <div className="shrink-0 max-w-full overflow-x-auto hover:-translate-y-0.5 hover:shadow-xl">{action}</div>}
        </div>
      )}
      <div className="p-5 hover:-translate-y-0.5 hover:shadow-xl">{children}</div>
    </div>
  );
};
