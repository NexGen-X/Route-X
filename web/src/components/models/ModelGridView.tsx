import React from 'react';
import { Cpu, DollarSign, Trash2 } from 'lucide-react';
import type { ModelGridViewProps } from './types';
import { getModelFamily } from './utils';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';

export const ModelGridView: React.FC<ModelGridViewProps> = ({
  models,
  onOpenPricing,
  onDeleteModel,
}) => {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4" data-testid="models-grid">
      {models.map((m) => (
        <Card key={m.id} className="p-5 flex flex-col justify-between" data-testid={`model-card-${m.model_id}`}>
          <div>
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-full bg-purple-500/10 border border-purple-500/20 text-purple-400 flex items-center justify-center">
                  <Cpu className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-bold text-white">{m.display_name}</h4>
                  <span className="text-[11px] text-accent font-mono">{m.model_id}</span>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Badge variant={m.enabled ? 'success' : 'neutral'}>
                  {m.enabled ? 'active' : 'disabled'}
                </Badge>
                <Tooltip content="Hapus Model" position="left">
                  <button
                    type="button"
                    onClick={() => onDeleteModel(m.id, m.display_name)}
                    className="p-1.5 h-8 w-8 text-text-muted hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors flex items-center justify-center focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 cursor-pointer"
                    aria-label={`Hapus model ${m.display_name}`}
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </Tooltip>
              </div>
            </div>

            <div className="mt-4 space-y-2 text-xs">
              <div className="flex justify-between py-1 border-b border-border/40">
                <span className="text-text-muted">Keluarga Model</span>
                <span className="font-semibold text-text-secondary">{getModelFamily(m)}</span>
              </div>
              <div className="flex justify-between py-1 border-b border-border/40">
                <span className="text-text-muted">Context Window</span>
                <span className="font-mono text-white">
                  {m.context_window != null ? `${m.context_window.toLocaleString()} tokens` : '-'}
                </span>
              </div>
              <div className="flex justify-between py-1 border-b border-border/40">
                <span className="text-text-muted">Max Output</span>
                <span className="font-mono text-white">
                  {m.max_output_tokens != null ? `${m.max_output_tokens.toLocaleString()} tokens` : '-'}
                </span>
              </div>
            </div>
            <div className="mt-4 pt-3 border-t border-border/40">
              <div className="text-xs font-semibold text-text-muted mb-2 flex items-center justify-between">
                <span>Penyedia Upstream ({m.providers?.length || 0})</span>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {m.providers && m.providers.length > 0 ? (
                  m.providers.map((p) => (
                    <Tooltip key={p.provider_id} content={`Upstream model: ${p.upstream_model_name}`}>
                      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-purple-500/10 border border-purple-500/25 text-purple-300 text-[11px] font-medium">
                        <span className="w-1.5 h-1.5 rounded-full bg-purple-400" />
                        <span className="font-semibold text-white">{p.display_name || p.provider_name}</span>
                        {p.upstream_model_name !== m.model_id && (
                          <span className="text-[10px] text-text-muted font-mono">({p.upstream_model_name})</span>
                        )}
                      </span>
                    </Tooltip>
                  ))
                ) : (
                  <span className="text-[11px] text-text-muted italic">Belum terhubung ke provider</span>
                )}
              </div>
            </div>
          </div>

          <div className="mt-5 pt-3 border-t border-border flex items-center justify-between">
            <div className="flex flex-wrap gap-1">
              {m.capabilities?.map((c) => (
                <span key={c} className="px-2 py-0.5 text-[10px] font-mono rounded bg-bg-surface-2 text-text-muted">
                  {c}
                </span>
              ))}
            </div>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onOpenPricing(m)}
              icon={<DollarSign className="w-3.5 h-3.5" />}
              aria-label={`Konfigurasi harga untuk ${m.display_name}`}
            >
              Harga
            </Button>
          </div>
        </Card>
      ))}
    </div>
  );
};
