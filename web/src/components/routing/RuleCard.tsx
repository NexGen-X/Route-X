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

  return (
    <Card className="p-4 sm:p-5 flex flex-col justify-between">
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
                <Zap className="w-5 h-5" />
              ) : ruleMode.mode === 'combo_routing' ? (
                <Layers className="w-5 h-5" />
              ) : (
                <Shuffle className="w-5 h-5" />
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
          <Badge variant={rule.enabled ? 'success' : 'neutral'}>
            {rule.enabled ? 'active' : 'disabled'}
          </Badge>
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
                <Zap className="w-3 h-3" /> Direct 1:1 Passthrough
              </span>
              <span className="text-emerald-400 flex items-center gap-1 font-sans">
                <CheckCircle2 className="w-3 h-3" /> 1 Percobaan Langsung
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
                <ArrowRight className="w-3.5 h-3.5" />
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

        {/* 2. Mode COMBO ROUTING: Interactive Cascade Flow */}
        {ruleMode.mode === 'combo_routing' && (
          <div className="p-2.5 rounded-lg bg-bg-surface-2/70 border border-accent/20 space-y-2 mt-3.5">
            {ruleMode.alias && (
              <div className="flex items-center justify-between pb-1.5 border-b border-border/30">
                <span className="text-[10px] text-text-muted flex items-center gap-1 font-sans">
                  <Terminal className="w-3 h-3 text-accent" /> Virtual Endpoint:
                </span>
                <code className="text-[11px] font-mono font-bold text-accent bg-surface px-1.5 py-0.5 rounded border border-accent/30">
                  {ruleMode.alias}
                </code>
              </div>
            )}
            <div className="flex items-center gap-1.5 flex-wrap text-[11px]">
              {(() => {
                // Rantai model dibaca dari kolom jsonb; aturan produksi lama
                // yang belum dikonversi jatuh ke tag description.
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
                      {i > 0 && <ArrowRight className="w-3 h-3 text-text-muted" />}
                      <span
                        className={`px-2 py-0.5 rounded border font-semibold ${
                          i === 0
                            ? 'bg-accent/15 border-accent/30 text-accent'
                            : 'bg-purple-500/15 border-purple-500/30 text-purple-300'
                        }`}
                      >
                        {`T${i + 1}: ${mObj?.display_name || modelId}`}
                      </span>
                    </React.Fragment>
                  );
                });
              })()}
            </div>
            <div className="flex items-center gap-2 pt-1 text-[10px] text-text-muted font-sans">
              <span className="flex items-center gap-1 text-emerald-400">
                <Zap className="w-2.5 h-2.5" /> Context Bypass
              </span>
              <span>•</span>
              <span className="flex items-center gap-1 text-sky-400">
                <ShieldCheck className="w-2.5 h-2.5" /> 429/5xx Failover
              </span>
            </div>
          </div>
        )}

        {/* 3. Mode ROUTING (Multi-Provider Failover) Detail Table */}
        {ruleMode.mode === 'routing' && (
          <div className="mt-3.5 space-y-2 text-xs">
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Target Model</span>
              <span className="font-mono text-white truncate max-w-[180px]">
                {modelDisplayName}
              </span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Strategi</span>
              <span className="font-semibold text-white uppercase font-mono">{rule.strategy}</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
              <span className="text-text-muted">Maksimal Percobaan</span>
              <span className="font-mono text-white">{rule.max_attempts} percobaan</span>
            </div>
            <div className="flex justify-between py-1 border-b border-border/40">
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

      {/* Action Buttons: Responsive & Balanced Layout */}
      <div className="mt-4 pt-3 border-t border-border/60 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
        <div className="grid grid-cols-2 sm:flex sm:items-center gap-1.5">
          <Button
            variant="secondary"
            size="sm"
            className="w-full sm:w-auto justify-center"
            onClick={() => onEdit(rule)}
            icon={<Edit2 className="w-3.5 h-3.5" />}
          >
            Edit Aturan
          </Button>
          <Button
            variant="secondary"
            size="sm"
            className="w-full sm:w-auto justify-center text-purple-300 border-purple-500/30 hover:bg-purple-500/10"
            onClick={() => onConfigureProviders(rule)}
            icon={<Server className="w-3.5 h-3.5" />}
          >
            Providers
          </Button>
        </div>
        <div className="grid grid-cols-2 sm:flex sm:items-center gap-1.5">
          <Button
            variant={rule.enabled ? 'danger' : 'secondary'}
            size="sm"
            className="w-full sm:w-auto justify-center"
            onClick={() => onToggle(rule)}
          >
            {rule.enabled ? 'Nonaktifkan' : 'Aktifkan'}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            className="w-full sm:w-auto justify-center text-status-error/80 border-status-error/20 hover:bg-status-error/10 hover:text-status-error"
            onClick={() => onDelete(rule.id)}
            icon={<Trash2 className="w-3.5 h-3.5" />}
          >
            Hapus
          </Button>
        </div>
      </div>
    </Card>
  );
};
