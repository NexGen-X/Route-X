import React from 'react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';
import { Coins, RotateCcw, Power, Trash2 } from 'lucide-react';
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
  const isDanger = pct >= threshold;

  return (
    <Card className="p-5 flex flex-col justify-between" data-testid={`budget-card-${budget.id}`}>
      <div>
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-full bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center shrink-0">
              <Coins className="w-5 h-5" />
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
          <Badge variant={isDanger ? 'error' : 'success'}>
            {pct}%
          </Badge>
        </div>

        <div className="mt-4">
          <div className="flex justify-between text-xs mb-1.5">
            <span className="text-text-muted">Terpakai</span>
            <span className="font-mono text-white font-semibold">
              {formatUSD(budget.spent_usd, 4)} / {formatUSD(budget.max_spend_usd, 2)}
            </span>
          </div>
          <div className="w-full h-2 bg-bg-surface-2 rounded-full overflow-hidden">
            <div
              className={`h-full transition-all duration-500 rounded-full ${
                isDanger ? 'bg-status-error' : 'bg-accent'
              }`}
              style={{ width: `${Math.min(pct, 100)}%` }}
            />
          </div>
        </div>

        <div className="mt-4 space-y-1 text-xs text-text-muted">
          <div className="flex justify-between py-1 border-b border-border/40">
            <span>Ambang Peringatan</span>
            <span className="font-mono text-text-primary">{threshold}%</span>
          </div>
          <div className="flex justify-between py-1">
            <span>Aksi Pelanggaran</span>
            <span className="font-mono text-accent uppercase">{budget.action || budget.action_on_exceed || 'block'}</span>
          </div>
        </div>
      </div>

      <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
        <button
          type="button"
          onClick={() => void onToggle(budget)}
          className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold border transition-colors cursor-pointer ${
            budget.enabled
              ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20 hover:bg-emerald-500/20'
              : 'bg-bg-surface-2 text-text-muted border-border hover:text-white'
          }`}
          title={budget.enabled ? 'Klik untuk menonaktifkan anggaran' : 'Klik untuk mengaktifkan anggaran'}
          aria-label={budget.enabled ? 'Nonaktifkan anggaran' : 'Aktifkan anggaran'}
        >
          <Power className="w-3 h-3" />
          <span>{budget.enabled ? 'Aktif' : 'Nonaktif'}</span>
        </button>
        <div className="flex items-center gap-1.5">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void onReset(budget.id)}
            icon={<RotateCcw className="w-3.5 h-3.5" />}
          >
            Reset Periode
          </Button>
          <Tooltip content="Hapus Anggaran" position="top">
            <button
              type="button"
              onClick={() => void onDelete(budget.id)}
              className="p-1.5 rounded-md text-text-muted hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
              aria-label="Hapus anggaran"
            >
              <Trash2 className="w-4 h-4" />
            </button>
          </Tooltip>
        </div>
      </div>
    </Card>
  );
};
