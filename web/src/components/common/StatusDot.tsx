import React from 'react';
import { cn } from '../../utils/cn';

export type StatusDotVariant =
  | 'healthy'
  | 'degraded'
  | 'unhealthy'
  | 'neutral'
  | 'active'
  | 'disabled';

export interface StatusDotProps {
  status: StatusDotVariant;
  label?: string;
  latencyMs?: number;
  className?: string;
}

const STATUS_COLORS: Record<StatusDotVariant, string> = {
  healthy: 'bg-emerald-400',
  active: 'bg-emerald-400',
  degraded: 'bg-amber-400',
  unhealthy: 'bg-rose-400',
  disabled: 'bg-zinc-500',
  neutral: 'bg-zinc-400',
};

const DEFAULT_LABELS: Partial<Record<StatusDotVariant, string>> = {
  healthy: 'Sehat',
  active: 'Aktif',
  degraded: 'Terdegradasi',
  unhealthy: 'Terganggu',
  disabled: 'Nonaktif',
  neutral: 'Netral',
};

/**
 * Komponen StatusDot bergaya Linear/Vercel-grade untuk menggantikan badge tebal.
 * Menampilkan bulatan halus 6px dengan warna status semantik, label ringkas, dan latensi monospaced.
 */
export const StatusDot: React.FC<StatusDotProps> = ({
  status,
  label,
  latencyMs,
  className = '',
}) => {
  const dotColorClass = STATUS_COLORS[status] || STATUS_COLORS.neutral;
  const displayLabel = label ?? DEFAULT_LABELS[status];

  return (
    <div
      className={cn(
        'inline-flex items-center gap-1.5 text-xs text-text-secondary font-mono select-none',
        className
      )}
    >
      <span className={cn('w-1.5 h-1.5 rounded-full shrink-0', dotColorClass)} />
      {displayLabel && (
        <span className="text-xs text-text-secondary font-medium tracking-tight">
          {displayLabel}
        </span>
      )}
      {typeof latencyMs === 'number' && latencyMs >= 0 && (
        <span className="text-[11px] text-text-muted font-mono">{latencyMs}ms</span>
      )}
    </div>
  );
};
