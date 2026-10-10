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
    pill: 'bg-zinc-900 text-zinc-300 border-zinc-800',
    dot: 'bg-zinc-400',
  },
  emerald: {
    pill: 'bg-emerald-950/40 text-emerald-400 border-emerald-800/60',
    dot: 'bg-emerald-500',
  },
  purple: {
    pill: 'bg-purple-950/40 text-purple-300 border-purple-800/60',
    dot: 'bg-purple-400',
  },
  amber: {
    pill: 'bg-amber-950/40 text-amber-400 border-amber-800/60',
    dot: 'bg-amber-500',
  },
  rose: {
    pill: 'bg-rose-950/40 text-rose-400 border-rose-800/60',
    dot: 'bg-rose-500',
  },
  lime: {
    pill: 'bg-zinc-900 text-zinc-200 border-zinc-800',
    dot: 'bg-emerald-400',
  },
  neutral: {
    pill: 'bg-zinc-900 text-zinc-400 border-zinc-800',
    dot: 'bg-zinc-500',
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
    sm: 'text-[11px] px-2 py-0.5 min-h-[20px]',
    md: 'text-xs px-2.5 py-0.5 min-h-[24px]',
  };

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 font-medium rounded-md tracking-wide border select-none transition-colors duration-150',
        config.pill,
        sizes[size],
        className
      )}
      {...props}
    >
      {dot && (
        <span
          className={cn('w-1.5 h-1.5 rounded-full shrink-0', config.dot)}
          aria-hidden="true"
        />
      )}
      {children}
    </span>
  );
};
