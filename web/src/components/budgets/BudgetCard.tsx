import React from 'react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';
import { Coins, RotateCcw, Power, Trash2, CalendarClock } from 'lucide-react';
import { formatUSD, percentageOfDecimal } from '../../utils/money';
import type { BudgetCardProps } from './types';

export const BudgetCard: React.FC<BudgetCardProps> = ({
  budget,
  scopeDisplay,
  onToggle,
  onReset,
  onDelete,
}) => {
  const pct = percentageOfDecimal(budget.spent_usd || '0', budget.max_spend_usd || '0');
  const threshold = budget.alert_threshold ?? budget.alert_threshold_pct ?? 80;

  /**
   * Gradien dinamis kuota visual:
   * - <70%: hijau (from-emerald-500 to-teal-400)
   * - 70-90%: amber (from-amber-500 to-yellow-400)
   * - >90%: merah (from-rose-500 to-red-600)
   */
  const getProgressGradient = (percent: number) => {
    if (percent > 90) return 'bg-gradient-to-r from-rose-500 to-red-600 shadow-sm shadow-rose-500/20';
    if (percent >= 70) return 'bg-gradient-to-r from-amber-500 to-yellow-400 shadow-sm shadow-amber-500/20';
    return 'bg-gradient-to-r from-emerald-500 to-teal-400 shadow-sm shadow-emerald-500/20';
  };

  const getResetLabel = (period: string) => {
    switch (period?.toLowerCase()) {
      case 'daily':
        return 'Reset Harian';
      case 'weekly':
        return 'Reset Mingguan';
      case 'monthly':
      default:
        return 'Reset Bulanan';
    }
  };

  return (
    <Card className="p-5 flex flex-col justify-between" data-testid={`budget-card-${budget.id}`}>
      <div>
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-full bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center shrink-0">
              <Coins className="w-5 h-5" aria-hidden="true" />
            </div>
            <div>
              <h4 className="text-sm font-bold text-white leading-tight">{budget.name}</h4>
              <div className="flex items-center gap-1.5 text-[11px] text-text-muted font-mono mt-0.5">
                <span className="text-accent font-semibold">{scopeDisplay.label}</span>
                <span>•</span>
                <span>{budget.period}</span>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-1.5">
            {/* Reset Frequency Badge */}
            <span
              className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-mono font-medium bg-bg-surface-3/80 text-text-secondary border border-border/60"
              title={`Frekuensi siklus reset: ${budget.period}`}
            >
              <CalendarClock className="w-3 h-3 text-accent" aria-hidden="true" />
              <span>{getResetLabel(budget.period)}</span>
            </span>
            <Badge variant={pct > 90 ? 'error' : pct >= 70 ? 'warn' : 'success'}>
              {pct}%
            </Badge>
          </div>
        </div>

        {/* Progress Bar Kuota Visual dengan Gradien Dinamis */}
        <div className="mt-4">
          <div className="flex justify-between text-xs mb-1.5">
            <span className="text-text-muted">Terpakai</span>
            <span className="font-mono text-white font-semibold">
              {formatUSD(budget.spent_usd, 4)} / {formatUSD(budget.max_spend_usd, 2)}
            </span>
          </div>
          <div className="w-full h-2.5 bg-bg-surface-2 rounded-full overflow-hidden border border-border/40 p-0.5">
            <div
              className={`h-full transition-all duration-500 rounded-full ${getProgressGradient(pct)}`}
              style={{ width: `${Math.min(pct, 100)}%` }}
            />
          </div>
        </div>

        <div className="mt-3.5 flex items-center justify-between text-xs text-text-muted font-mono pt-2 border-t border-border/40">
          <div className="flex items-center gap-1.5">
            <span>Ambang:</span>
            <span className="text-text-primary font-medium">{threshold}%</span>
          </div>
          <span className="text-border">•</span>
          <div className="flex items-center gap-1.5">
            <span>Aksi:</span>
            <span className="text-accent font-semibold uppercase">{budget.action || budget.action_on_exceed || 'block'}</span>
          </div>
        </div>
      </div>

      <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
        <button
          type="button"
          onClick={() => void onToggle(budget)}
          className={`min-h-[44px] sm:min-h-[32px] inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold border transition-colors cursor-pointer ${
            budget.enabled
              ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20 hover:bg-emerald-500/20'
              : 'bg-bg-surface-2 text-text-muted border-border hover:text-white'
          }`}
          title={budget.enabled ? 'Klik untuk menonaktifkan anggaran' : 'Klik untuk mengaktifkan anggaran'}
          aria-label={budget.enabled ? 'Nonaktifkan anggaran' : 'Aktifkan anggaran'}
        >
          <Power className="w-3 h-3" aria-hidden="true" />
          <span>{budget.enabled ? 'Aktif' : 'Nonaktif'}</span>
        </button>
        <div className="flex items-center gap-1.5">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void onReset(budget.id)}
            icon={<RotateCcw className="w-3 h-3 text-text-muted" aria-hidden="true" />}
            className="text-xs text-text-secondary hover:text-white"
          >
            Reset Periode
          </Button>
          <Tooltip content="Hapus Anggaran" position="top">
            <button
              type="button"
              onClick={() => void onDelete(budget.id)}
              className="min-h-[44px] min-w-[44px] sm:min-h-[32px] sm:min-w-[32px] flex items-center justify-center rounded-md text-text-muted hover:text-rose-400 hover:bg-rose-500/10 transition-colors cursor-pointer"
              aria-label="Hapus anggaran"
            >
              <Trash2 className="w-4 h-4" aria-hidden="true" />
            </button>
          </Tooltip>
        </div>
      </div>
    </Card>
  );
};
