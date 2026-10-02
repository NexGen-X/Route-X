import React from 'react';
import { cn } from '../../utils/cn';

interface BadgeProps {
  children: React.ReactNode;
  variant?: 'lime' | 'success' | 'error' | 'warn' | 'neutral' | 'info';
  size?: 'sm' | 'md';
  className?: string;
}

export const Badge: React.FC<BadgeProps> = ({
  children,
  variant = 'neutral',
  size = 'sm',
  className = '',
}) => {
  const variants = {
    lime: 'bg-accent/10 text-accent border border-accent/20',
    success: 'bg-status-success/10 text-status-success border border-status-success/20',
    error: 'bg-status-error/10 text-status-error border border-status-error/20',
    warn: 'bg-status-warn/10 text-status-warn border border-status-warn/20',
    info: 'bg-status-info/10 text-status-info border border-status-info/20',
    neutral: 'bg-bg-surface-2 text-text-secondary border border-border',
  };

  const sizes = {
    sm: 'text-[11px] px-2 py-0.5',
    md: 'text-xs px-2.5 py-1',
  };

  return (
    <span
      className={cn('inline-flex items-center gap-1 font-medium rounded-chip tracking-wide', variants[variant], sizes[size], className)}
    >
      {children}
    </span>
  );
};
