import React from 'react';
import { Cpu, DollarSign, Trash2 } from 'lucide-react';
import type { ModelTableViewProps } from './types';
import { getModelFamily } from './utils';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';

const formatTokensK = (val?: number | null): string => {
  if (val == null) return '-';
  if (val >= 1000) return `${Math.round(val / 1000)}k`;
  return String(val);
};

export const ModelTableView: React.FC<ModelTableViewProps> = ({
  models,
  onOpenPricing,
  onDeleteModel,
}) => {
  return (
    <div className="bg-bg-surface-1 border border-border rounded-xl overflow-hidden shadow-sm">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-xs" data-testid="models-table">
          <thead>
            <tr className="border-b border-border text-text-muted font-semibold text-xs bg-bg-surface-2/40">
              <th className="py-3 px-4">Model & ID</th>
              <th className="py-3 px-4">Keluarga</th>
              <th className="py-3 px-4">Penyedia Upstream</th>
              <th className="py-3 px-4">Konteks / Output</th>
              <th className="py-3 px-4">Kemampuan</th>
              <th className="py-3 px-4 text-center">Status</th>
              <th className="py-3 px-4 text-right">Aksi</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {models.map((m) => (
              <tr key={m.id} className="hover:bg-bg-surface-2/40 transition-colors" data-testid={`model-row-${m.model_id}`}>
                <td className="py-3 px-4">
                  <div className="flex items-center gap-2.5">
                    <div className="w-8 h-8 rounded-lg bg-purple-500/10 border border-purple-500/20 text-purple-400 flex items-center justify-center shrink-0">
                      <Cpu className="w-4 h-4" />
                    </div>
                    <div className="min-w-0">
                      <div className="font-semibold text-white truncate max-w-[200px]">{m.display_name}</div>
                      <div className="text-[11px] text-accent font-mono truncate max-w-[200px]">{m.model_id}</div>
                    </div>
                  </div>
                </td>
                <td className="py-3 px-4 whitespace-nowrap">
                  <span className="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-bg-surface-2 border border-border text-text-secondary">
                    {getModelFamily(m)}
                  </span>
                </td>
                <td className="py-3 px-4">
                  <div className="flex flex-wrap gap-1 max-w-xs">
                    {m.providers && m.providers.length > 0 ? (
                      m.providers.map((p) => (
                        <Tooltip key={p.provider_id} content={`Upstream: ${p.upstream_model_name}`}>
                          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-purple-500/10 border border-purple-500/20 text-purple-300 text-[10px] font-medium">
                            <span className="w-1.5 h-1.5 rounded-full bg-purple-400" />
                            <span>{p.display_name || p.provider_name}</span>
                          </span>
                        </Tooltip>
                      ))
                    ) : (
                      <span className="text-[11px] text-text-muted italic">Belum terhubung</span>
                    )}
                  </div>
                </td>
                <td className="py-3 px-4 font-mono text-[11px] whitespace-nowrap">
                  <span className="text-white font-medium">{formatTokensK(m.context_window)}</span>
                  <span className="text-text-muted mx-1">/</span>
                  <span className="text-text-muted">{formatTokensK(m.max_output_tokens)}</span>
                </td>
                <td className="py-3 px-4">
                  <div className="flex items-center gap-1">
                    {m.capabilities && m.capabilities.length > 0 ? (
                      <>
                        {m.capabilities.slice(0, 2).map((c) => (
                          <span
                            key={c}
                            className="px-1.5 py-0.5 text-[10px] font-mono rounded bg-bg-surface-2 text-text-secondary border border-border/50"
                          >
                            {c}
                          </span>
                        ))}
                        {m.capabilities.length > 2 && (
                          <Tooltip
                            content={m.capabilities.slice(2).join(', ')}
                            position="top"
                          >
                            <span className="px-1 py-0.5 text-[9px] font-mono rounded bg-bg-surface-3 text-text-muted border border-border/40 cursor-help">
                              +{m.capabilities.length - 2}
                            </span>
                          </Tooltip>
                        )}
                      </>
                    ) : (
                      <span className="text-[11px] text-text-muted">-</span>
                    )}
                  </div>
                </td>
                <td className="py-3 px-4 text-center whitespace-nowrap">
                  <Badge variant={m.enabled ? 'success' : 'neutral'}>
                    {m.enabled ? 'active' : 'disabled'}
                  </Badge>
                </td>
                <td className="py-3 px-4 text-right whitespace-nowrap">
                  <div className="flex items-center justify-end gap-1.5">
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => onOpenPricing(m)}
                      icon={<DollarSign className="w-3.5 h-3.5" />}
                      className="h-7 px-2.5 text-xs"
                      aria-label={`Konfigurasi harga untuk ${m.display_name}`}
                    >
                      Harga
                    </Button>
                    <Tooltip content="Hapus Model" position="left">
                      <button
                        type="button"
                        onClick={() => onDeleteModel(m.id, m.display_name)}
                        className="p-1.5 h-7 w-7 text-text-muted hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors flex items-center justify-center focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500 cursor-pointer"
                        aria-label={`Hapus model ${m.display_name}`}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </Tooltip>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};
