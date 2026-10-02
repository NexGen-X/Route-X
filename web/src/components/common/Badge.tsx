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
    lime: 'bg-accent/10 text-accent border-accent/20',
    success: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20',
    error: 'bg-rose-500/10 text-rose-400 border-rose-500/20',
    warn: 'bg-amber-500/10 text-amber-400 border-amber-500/20',
    info: 'bg-sky-500/10 text-sky-400 border-sky-500/20',
    neutral: 'bg-white/[0.03] text-zinc-300 border-white/[0.08]',
  };

  const sizes = {
    sm: 'text-[11px] px-2.5 py-0.5',
    md: 'text-xs px-3 py-1',
  };

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 font-medium rounded-full tracking-wide border',
        variants[variant],
        sizes[size],
        className
      )}
    >
      {children}
    </span>
  );
};
