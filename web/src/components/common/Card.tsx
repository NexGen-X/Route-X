import React from 'react';
import { cn } from '../../utils/cn';

export interface CardProps extends Omit<React.HTMLAttributes<HTMLDivElement>, 'title'> {
  children: React.ReactNode;
  className?: string;
  title?: React.ReactNode;
  subtitle?: React.ReactNode;
  action?: React.ReactNode;
  headerClassName?: string;
  bodyClassName?: string;
}

export const Card: React.FC<CardProps> = ({
  children,
  className = '',
  title,
  subtitle,
  action,
  headerClassName,
  bodyClassName,
  ...props
}) => {
  return (
    <div
      className={cn(
        'bg-zinc-900/40 border border-zinc-800/80 rounded-lg overflow-hidden shadow-sm transition-colors',
        className
      )}
      {...props}
    >
      {(title || action || subtitle) && (
        <div
          className={cn(
            'px-4 py-3 border-b border-zinc-800/80 bg-zinc-900/20 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4',
            headerClassName
          )}
        >
          <div className="min-w-0">
            {title && (
              <h3 className="text-sm font-medium text-text-primary tracking-tight">{title}</h3>
            )}
            {subtitle && (
              <p className="text-xs text-text-muted mt-0.5 leading-relaxed">{subtitle}</p>
            )}
          </div>
          {action && <div className="shrink-0 max-w-full overflow-x-auto">{action}</div>}
        </div>
      )}
      <div className={cn('p-4 sm:p-5', bodyClassName)}>{children}</div>
    </div>
  );
};
