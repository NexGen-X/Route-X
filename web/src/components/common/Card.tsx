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
        'bg-bg-surface/85 backdrop-blur-md border border-white/[0.08] rounded-card overflow-hidden shadow-surface-elevated hover:border-white/[0.14] transition-all duration-200',
        className
      )}
      {...props}
    >
      {(title || action || subtitle) && (
        <div
          className={cn(
            'px-5 py-3.5 border-b border-white/[0.06] bg-bg-surface-1/40 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4',
            headerClassName
          )}
        >
          <div className="min-w-0">
            {title && (
              <h3 className="text-sm font-semibold text-text-primary tracking-tight">{title}</h3>
            )}
            {subtitle && (
              <p className="text-xs text-text-muted mt-0.5 leading-relaxed">{subtitle}</p>
            )}
          </div>
          {action && <div className="shrink-0 max-w-full overflow-x-auto">{action}</div>}
        </div>
      )}
      <div className={cn('p-5', bodyClassName)}>{children}</div>
    </div>
  );
};
