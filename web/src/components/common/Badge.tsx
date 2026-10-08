import React from 'react';
import { cn } from '../../utils/cn';

export type BadgeVariant =
  | 'blue'
  | 'emerald'
  | 'purple'
  | 'amber'
  | 'rose'
  | 'neutral'
  | 'lime'
  | 'success'
  | 'error'
  | 'warn'
  | 'info';

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  children: React.ReactNode;
  variant?: BadgeVariant;
  size?: 'sm' | 'md';
  dot?: boolean;
  className?: string;
}

const VARIANT_STYLES: Record<string, { pill: string; dot: string }> = {
  blue: {
    pill: 'bg-blue-500/10 text-blue-400 border-blue-500/25 shadow-[0_0_12px_-3px_rgba(59,130,246,0.25)]',
    dot: 'bg-blue-400 shadow-[0_0_8px_rgba(59,130,246,0.8)]',
  },
  emerald: {
    pill: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/25 shadow-[0_0_12px_-3px_rgba(16,185,129,0.25)]',
    dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(16,185,129,0.8)]',
  },
  purple: {
    pill: 'bg-purple-500/10 text-purple-400 border-purple-500/25 shadow-[0_0_12px_-3px_rgba(168,85,247,0.25)]',
    dot: 'bg-purple-400 shadow-[0_0_8px_rgba(168,85,247,0.8)]',
  },
  amber: {
    pill: 'bg-amber-500/10 text-amber-400 border-amber-500/25 shadow-[0_0_12px_-3px_rgba(245,158,11,0.25)]',
    dot: 'bg-amber-400 shadow-[0_0_8px_rgba(245,158,11,0.8)]',
  },
  rose: {
    pill: 'bg-rose-500/10 text-rose-400 border-rose-500/25 shadow-[0_0_12px_-3px_rgba(244,63,94,0.25)]',
    dot: 'bg-rose-400 shadow-[0_0_8px_rgba(244,63,94,0.8)]',
  },
  lime: {
    pill: 'bg-lime-500/10 text-lime-400 border-lime-500/25 shadow-[0_0_12px_-3px_rgba(163,230,53,0.25)]',
    dot: 'bg-lime-400 shadow-[0_0_8px_rgba(163,230,53,0.8)]',
  },
  neutral: {
    pill: 'bg-bg-surface-2 text-text-secondary border-white/[0.08] shadow-sm',
    dot: 'bg-zinc-400',
  },
};

const VARIANT_ALIASES: Record<string, string> = {
  info: 'blue',
  success: 'emerald',
  warn: 'amber',
  error: 'rose',
};

export const Badge: React.FC<BadgeProps> = ({
  children,
  variant = 'neutral',
  size = 'sm',
  dot = false,
  className = '',
  ...props
}) => {
  const resolvedVariantKey = VARIANT_ALIASES[variant] || variant;
  const config = VARIANT_STYLES[resolvedVariantKey] || VARIANT_STYLES.neutral;

  const sizes = {
    sm: 'text-[11px] px-2.5 py-0.5 min-h-[22px]',
    md: 'text-xs px-3 py-1 min-h-[26px]',
  };

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 font-medium rounded-chip tracking-wide border select-none transition-colors duration-150',
        config.pill,
        sizes[size],
        className
      )}
      {...props}
    >
      {dot && (
        <span
          className={cn('w-1.5 h-1.5 rounded-full shrink-0 animate-pulse-glow', config.dot)}
          aria-hidden="true"
        />
      )}
      {children}
    </span>
  );
};
