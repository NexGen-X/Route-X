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
    'relative inline-flex items-center justify-center gap-2 font-medium select-none rounded-md transition-all duration-150 disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none focus:outline-none focus-visible:ring-1 focus-visible:ring-indigo-500/40 cursor-pointer';

  const variants = {
    primary:
      'bg-indigo-600 hover:bg-indigo-500 text-white font-medium shadow-sm shadow-indigo-600/25 border border-indigo-400/20 active:scale-[0.98] transition-all',
    secondary:
      'bg-zinc-900 hover:bg-zinc-800 text-zinc-200 border border-zinc-800 hover:border-zinc-700 shadow-sm',
    danger:
      'bg-rose-950/40 text-rose-300 hover:bg-rose-900/60 border border-rose-800/60 hover:border-rose-700 shadow-sm',
    ghost:
      'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/60 border border-transparent',
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
