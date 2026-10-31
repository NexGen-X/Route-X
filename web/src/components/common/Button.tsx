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
    'relative inline-flex items-center justify-center gap-2 font-medium select-none rounded-full transition-all duration-150 disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/50 cursor-pointer';

  const variants = {
    primary:
      'bg-accent hover:bg-accent-hover text-white font-medium shadow-sm border border-white/10 active:scale-[0.98] transition-all',
    secondary:
      'bg-bg-surface-1 hover:bg-bg-surface-2 text-text-primary border border-border hover:border-border-hover shadow-sm',
    danger:
      'bg-rose-950/40 text-rose-300 hover:bg-rose-900/60 border border-rose-800/60 hover:border-rose-700 shadow-sm',
    ghost:
      'text-text-secondary hover:text-white hover:bg-bg-surface-2 border border-transparent',
  };

  const sizes = {
    sm: 'text-xs px-3 py-1.5 min-h-[36px] sm:min-h-[30px]',
    md: 'text-xs sm:text-sm px-3.5 py-1.5 min-h-[40px] sm:min-h-[34px]',
    lg: 'text-sm px-4 py-2 min-h-[44px] sm:min-h-[38px]',
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
        <Loader2 className="w-3.5 h-3.5 animate-spin shrink-0 text-current" aria-hidden="true" />
      ) : icon ? (
        <span className="inline-flex shrink-0 items-center justify-center" aria-hidden="true">
          {icon}
        </span>
      ) : null}
      {children}
    </button>
  );
};
