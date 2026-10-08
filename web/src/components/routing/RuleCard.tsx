import React from 'react';
import type { RoutingRule, Model, Provider } from '../../types';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import {
  Zap,
  Layers,
  Shuffle,
  Edit2,
  Server,
  Trash2,
  CheckCircle2,
  ArrowRight,
  Terminal,
  ShieldCheck,
  Scale,
  Coins,
  RefreshCw,
  ArrowDownNarrowWide,
} from 'lucide-react';
import { getRuleMode } from './types';
import { parsePipeline, parsePipelineDariTag } from '../../lib/rulePipeline';

export interface RuleCardProps {
  rule: RoutingRule;
  models: Model[];
  providers: Provider[];
  onEdit: (rule: RoutingRule) => void;
  onConfigureProviders: (rule: RoutingRule) => void;
  onToggle: (rule: RoutingRule) => void | Promise<void>;
  onDelete: (id: string) => void | Promise<void>;
}

export const RuleCard: React.FC<RuleCardProps> = ({
  rule,
  models,
  providers,
  onEdit,
  onConfigureProviders,
  onToggle,
  onDelete,
}) => {
  const ruleMode = getRuleMode(rule, models);
  const matchedModel = rule.match_model_id
    ? models.find((m) => m.id === rule.match_model_id || m.model_id === rule.match_model_id)
    : null;

  const cleanCustomDesc = rule.description
    ? rule.description
        .replace(/\[combo:alias=[^\]]+\]\s*/g, '')
        .replace(/\[combo:pipeline=\[[\s\S]*?\}\]\s*/g, '')
        .replace(/\[combo:tier2=[^\]]+\]\s*/g, '')
        .replace(/^\[(model_only|routing)\]\s*/, '')
        .trim()
    : '';

  const isAutoDesc =
    cleanCustomDesc.startsWith('Direct 1:1 passthrough') ||
    cleanCustomDesc.startsWith('Smart Tiered Cascade:') ||
    cleanCustomDesc.startsWith('Failover multi-provider') ||
    cleanCustomDesc.startsWith('Combo ');

  const modelDisplayName = matchedModel
    ? matchedModel.display_name && matchedModel.display_name !== matchedModel.model_id
      ? `${matchedModel.display_name} (${matchedModel.model_id})`
      : matchedModel.model_id
    : rule.match_model_id || 'Semua Model (Catch-All)';

  // Helper untuk visualisasi strategi routing
  const renderStrategyVisual = (strategy: string) => {
    const s = strategy.toLowerCase();
    if (s === 'priority') {
      return (
        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-semibold bg-blue-500/10 text-blue-400 border border-blue-500/25">
          <ArrowDownNarrowWide className="w-3 h-3 text-blue-400" aria-hidden="true" />
          <span>Priority Failover</span>
        </span>
      );
    }
    if (s === 'weighted') {
      return (
        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-semibold bg-amber-500/10 text-amber-400 border border-amber-500/25">
          <Scale className="w-3 h-3 text-amber-400" aria-hidden="true" />
          <span>Weighted Probabilistic</span>
        </span>
      );
    }
    if (s.includes('latency')) {
      return (
        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/25">
          <Zap className="w-3 h-3 text-emerald-400" aria-hidden="true" />
          <span>Lowest-Latency First</span>
        </span>
      );
    }
    if (s.includes('cost')) {
      return (
        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-semibold bg-cyan-500/10 text-cyan-400 border border-cyan-500/25">
          <Coins className="w-3 h-3 text-cyan-400" aria-hidden="true" />
          <span>Lowest-Cost First</span>
        </span>
      );
    }
    return (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-semibold bg-purple-500/10 text-purple-400 border border-purple-500/25">
        <RefreshCw className="w-3 h-3 text-purple-400" aria-hidden="true" />
        <span>Round Robin</span>
      </span>
    );
  };

  return (
    <Card className="p-4 sm:p-5 flex flex-col justify-between hover:border-border-hover transition-all">
      <div>
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-3 min-w-0">
            <div
              className={`w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0 ${
                ruleMode.mode === 'model_only'
                  ? 'bg-sky-500/10 border border-sky-500/20 text-sky-400'
                  : ruleMode.mode === 'combo_routing'
                  ? 'bg-accent/10 border border-accent/20 text-accent'
                  : 'bg-purple-500/10 border border-purple-500/20 text-purple-400'
              }`}
            >
              {ruleMode.mode === 'model_only' ? (
                <Zap className="w-5 h-5" aria-hidden="true" />
              ) : ruleMode.mode === 'combo_routing' ? (
                <Layers className="w-5 h-5" aria-hidden="true" />
              ) : (
                <Shuffle className="w-5 h-5" aria-hidden="true" />
              )}
            </div>
            <div className="min-w-0">
              <h4 className="text-sm font-bold text-white truncate">{rule.name}</h4>
              <div className="flex items-center gap-2 mt-1">
                <Badge variant={ruleMode.badgeVariant}>{ruleMode.label}</Badge>
                <span className="text-[10px] text-text-muted font-mono">Prio: {rule.priority}</span>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <button
              type="button"
              role="switch"
              aria-checked={rule.enabled}
              onClick={() => void onToggle(rule)}
              title={rule.enabled ? 'Nonaktifkan aturan' : 'Aktifkan aturan'}
              aria-label={rule.enabled ? 'Nonaktifkan aturan' : 'Aktifkan aturan'}
              className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border border-border/80 transition-colors duration-200 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent ${
                rule.enabled ? 'bg-emerald-500/90' : 'bg-bg-surface-3'
              }`}
            >
              <span
                className={`pointer-events-none inline-block h-3.5 w-3.5 mt-[2px] ml-[2px] transform rounded-full bg-white shadow transition duration-200 ${
                  rule.enabled ? 'translate-x-4' : 'translate-x-0'
                }`}
              />
            </button>
          </div>
        </div>

        {cleanCustomDesc && !isAutoDesc && (
          <p className="mt-3 text-xs text-text-secondary leading-relaxed bg-bg-surface-2/50 p-2 rounded border border-border/40">
            {cleanCustomDesc}
          </p>
        )}

        {/* 1. Mode MODEL ONLY: Visual Passthrough Streamlined */}
        {ruleMode.mode === 'model_only' && (
          <div className="mt-3.5 p-3 rounded-lg bg-bg-surface-2/60 border border-sky-500/20 space-y-2.5">
            <div className="flex items-center justify-between text-[10px] text-text-muted font-mono">
              <span className="flex items-center gap-1 text-sky-400 font-semibold">
                <Zap className="w-3 h-3" aria-hidden="true" /> Direct 1:1 Passthrough
              </span>
              <span className="text-emerald-400 flex items-center gap-1 font-sans">
                <CheckCircle2 className="w-3 h-3" aria-hidden="true" /> 1 Percobaan Langsung
              </span>
            </div>
            <div className="flex items-center justify-between gap-2 p-2.5 rounded bg-surface border border-border/50 text-xs">
              <div className="min-w-0 flex-1">
                <span className="text-[10px] text-text-muted block font-sans">Target Model</span>
                <span className="font-mono font-bold text-white truncate block">
                  {modelDisplayName}
                </span>
              </div>
              <div className="p-1 rounded bg-sky-500/10 text-sky-400 flex-shrink-0">
                <ArrowRight className="w-3.5 h-3.5" aria-hidden="true" />
              </div>
              <div className="min-w-0 flex-1 text-right">
                <span className="text-[10px] text-text-muted block font-sans">Upstream Provider</span>
                <span className="font-medium text-purple-300 truncate block">
                  {rule.providers && rule.providers.length > 0 ? (
                    providers.find((prov) => prov.id === rule.providers?.[0]?.provider_id)?.display_name ||
                    providers.find((prov) => prov.id === rule.providers?.[0]?.provider_id)?.name ||
                    'Provider Terpilih'
                  ) : (
                    'Semua Provider (Bawaan)'
                  )}
                </span>
              </div>
            </div>
          </div>
        )}

        {/* 2. Mode COMBO ROUTING: Interactive Cascade Flow & Fallback Chain Pills */}
        {ruleMode.mode === 'combo_routing' && (
          <div className="p-3 rounded-lg bg-bg-surface-1 border border-border/70 space-y-2.5 mt-3.5">
            {ruleMode.alias && (
              <div className="flex items-center justify-between pb-1.5 border-b border-border/40">
                <span className="text-[10px] text-text-muted flex items-center gap-1 font-sans">
                  <Terminal className="w-3 h-3 text-text-secondary" aria-hidden="true" /> Virtual Endpoint:
                </span>
                <code className="text-[11px] font-mono font-semibold text-text-primary bg-bg-surface-2 px-1.5 py-0.5 rounded border border-border/60">
                  {ruleMode.alias}
                </code>
              </div>
            )}

            {/* Fallback chain tag pills */}
            <div className="space-y-1">
              <span className="text-[10px] text-text-muted uppercase tracking-wider block font-medium">
                Rantai Fallback Model
              </span>
              <div className="flex items-center gap-1.5 flex-wrap text-[11px]">
                {(() => {
                  const modelUtama = models.find((m) => m.id === rule.match_model_id);
                  const pipeline =
                    parsePipeline(rule) ?? parsePipelineDariTag(rule.description, modelUtama?.model_id);

                  if (!pipeline) {
                    return (
                      <span className="text-[10px] text-text-muted italic">Resep combo tidak terbaca</span>
                    );
                  }
                  return pipeline.models.map((modelId, i) => {
                    const mObj = models.find((m) => m.model_id === modelId);
                    return (
                      <React.Fragment key={`${modelId}-${i}`}>
                        {i > 0 && <ArrowRight className="w-3 h-3 text-text-muted/60 shrink-0" aria-hidden="true" />}
                        <span className="px-2 py-0.5 rounded border border-border/70 bg-bg-surface-2 font-mono text-[11px] text-text-primary font-medium hover:border-accent/40 transition-colors shadow-xs">
                          {`T${i + 1}: ${mObj?.display_name || modelId}`}
                        </span>
                      </React.Fragment>
                    );
                  });
                })()}
              </div>
            </div>

            <div className="flex items-center justify-between pt-1 text-[10px] text-text-muted font-sans border-t border-border/30">
              <div className="flex items-center gap-2">
                <span className="flex items-center gap-1 text-emerald-400">
                  <Zap className="w-2.5 h-2.5" aria-hidden="true" /> Context Bypass
                </span>
                <span>•</span>
                <span className="flex items-center gap-1 text-sky-400">
                  <ShieldCheck className="w-2.5 h-2.5" aria-hidden="true" /> 429/5xx Failover
                </span>
              </div>
              <span className="font-mono text-text-secondary">
                {rule.max_attempts} attempts
              </span>
            </div>
          </div>
        )}

        {/* 3. Mode ROUTING (Multi-Provider Failover) Detail Table */}
        {ruleMode.mode === 'routing' && (
          <div className="mt-3.5 space-y-1.5 text-xs">
            <div className="flex justify-between py-1">
              <span className="text-text-muted">Target Model</span>
              <span className="font-mono text-white truncate max-w-[180px]">
                {modelDisplayName}
              </span>
            </div>
            <div className="flex items-center justify-between py-1">
              <span className="text-text-muted">Strategi Perutean</span>
              {renderStrategyVisual(rule.strategy)}
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">Maksimal Percobaan</span>
              <span className="font-mono text-white">{rule.max_attempts} percobaan</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-text-muted">Jeda Backoff</span>
              <span className="font-mono text-white">{rule.backoff_ms} ms</span>
            </div>
            <div className="pt-1.5">
              <div className="text-xs font-semibold text-text-muted mb-1 flex items-center justify-between">
                <span>Provider Terpilih ({rule.providers?.length || 0})</span>
              </div>
              <div className="flex flex-wrap gap-1">
                {rule.providers && rule.providers.length > 0 ? (
                  rule.providers.map((rp) => {
                    const p = providers.find((prov) => prov.id === rp.provider_id);
                    const pName = p ? p.display_name || p.name : rp.provider_id.slice(0, 8);
                    return (
                      <span
                        key={rp.provider_id}
                        className="px-2 py-0.5 rounded text-[10px] font-medium bg-purple-500/10 text-purple-300 border border-purple-500/20"
                      >
                        {pName}
                        {rp.weight ? ` (w:${rp.weight})` : ''}
                      </span>
                    );
                  })
                ) : (
                  <span className="text-[10px] text-text-muted italic">Semua provider (Bawaan)</span>
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Footer Kartu Aksi: Tombol Primer Edit & Opsi Sekunder Ergonomis */}
      <div className="mt-4 pt-3 border-t border-border flex items-center justify-between gap-2">
        <Button
          variant="primary"
          size="sm"
          onClick={() => onEdit(rule)}
          icon={<Edit2 className="w-3.5 h-3.5" aria-hidden="true" />}
          className="text-xs min-h-[38px] sm:min-h-[34px] font-semibold"
          aria-label={`Edit aturan ${rule.name}`}
        >
          Edit Aturan
        </Button>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => onConfigureProviders(rule)}
            icon={<Server className="w-3.5 h-3.5" aria-hidden="true" />}
            className="text-xs min-h-[38px] sm:min-h-[34px]"
            title="Kelola Upstream Provider"
            aria-label={`Kelola provider untuk aturan ${rule.name}`}
          >
            Providers
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => void onDelete(rule.id)}
            icon={<Trash2 className="w-3.5 h-3.5" aria-hidden="true" />}
            className="text-xs min-h-[38px] sm:min-h-[34px]"
            title="Hapus Aturan"
            aria-label={`Hapus aturan ${rule.name}`}
          >
            Hapus
          </Button>
        </div>
      </div>
    </Card>
  );
};
