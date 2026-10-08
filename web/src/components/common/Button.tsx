import React from 'react';
import { Loader2 } from 'lucide-react';
import { cn } from '../../utils/cn';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  size?: 'sm' | 'md' | 'lg';
  isLoading?: boolean;
  icon?: React.ReactNode;
}

export const Button: React.FC<ButtonProps> = ({
  children,
  variant = 'secondary',
  size = 'md',
  isLoading = false,
  icon,
  className = '',
  disabled,
  type = 'button',
  ...props
}) => {
  const base =
    'relative inline-flex items-center justify-center gap-2 font-medium select-none rounded-nav overflow-hidden transition-all duration-200 active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none disabled:active:scale-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg-base cursor-pointer';

  const variants = {
    primary:
      'bg-gradient-to-r from-blue-600 via-indigo-600 to-blue-600 bg-[length:200%_auto] hover:bg-[position:right_center] text-white font-semibold shadow-sm shadow-blue-500/25 hover:shadow-glow-accent border border-blue-400/30 hover:border-blue-300/50',
    secondary:
      'bg-bg-surface-2/90 hover:bg-bg-surface-3 text-text-primary hover:text-white border border-white/[0.08] hover:border-white/[0.18] shadow-sm hover:shadow-glow-subtle/20',
    danger:
      'bg-status-error/10 text-status-error hover:text-white hover:bg-status-error border border-status-error/25 hover:border-status-error/50 shadow-sm shadow-rose-950/20',
    ghost:
      'text-text-secondary hover:text-text-primary hover:bg-bg-surface-2/80 border border-transparent',
  };

  const sizes = {
    sm: 'text-xs px-3 py-1.5 min-h-[32px]',
    md: 'text-sm px-4 py-2 min-h-[44px] sm:min-h-[38px]',
    lg: 'text-base px-5 py-2.5 min-h-[48px] sm:min-h-[44px]',
  };

  return (
    <button
      type={type}
      aria-busy={isLoading}
      className={cn(base, variants[variant], sizes[size], className)}
      disabled={disabled || isLoading}
      {...props}
    >
      {isLoading ? (
        <Loader2 className="w-4 h-4 animate-spin shrink-0 text-current" aria-hidden="true" />
      ) : icon ? (
        <span className="inline-flex shrink-0 items-center justify-center" aria-hidden="true">
          {icon}
        </span>
      ) : null}
      {children}
    </button>
  );
};
