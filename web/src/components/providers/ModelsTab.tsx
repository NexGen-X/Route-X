import React, { useEffect, useState, useMemo, useRef } from 'react';
import type { Provider, ProviderModel } from '../../types';
import type { ModelTestResult } from './types';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';
import {
  Plus,
  RefreshCw,
  Trash2,
  CheckCircle2,
  DownloadCloud,
  Copy,
  Check,
  Bot,
  Search,
  FlaskConical,
  AlertTriangle,
} from 'lucide-react';

export interface ModelsTabProps {
  models: ProviderModel[];
  selectedProvider: Provider;
  syncingId: string | null;
  testingModelId: string | null;
  modelTestResults: Record<string, ModelTestResult>;
  handleSyncModels: (provider: Provider) => void | Promise<void>;
  handleTestModel: (model: ProviderModel) => void | Promise<void>;
  handleDeleteModel: (model: ProviderModel) => void | Promise<void>;
  onOpenAddModelModal: () => void;
  copyWithFeedback: (text: string, message: string, onCopied?: () => void) => Promise<void> | void;
}

export const ModelsTab: React.FC<ModelsTabProps> = ({
  models,
  selectedProvider,
  syncingId,
  testingModelId,
  modelTestResults,
  handleSyncModels,
  handleTestModel,
  handleDeleteModel,
  onOpenAddModelModal,
  copyWithFeedback,
}) => {
  const [modelSearch, setModelSearch] = useState('');
  const [copiedModelId, setCopiedModelId] = useState<string | null>(null);
  const copyTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    return () => {
      copyTimersRef.current.forEach((t) => clearTimeout(t));
      copyTimersRef.current = [];
    };
  }, []);

  const getModelBadges = (modelName: string, providerKind: string) => {
    const lower = modelName.toLowerCase();
    let brand = 'Upstream';
    let context = 'Standard';
    if (lower.includes('claude')) { brand = 'Anthropic'; context = '200K'; }
    else if (lower.includes('gpt-4') || lower.includes('o1') || lower.includes('o3')) { brand = 'OpenAI'; context = '128K'; }
    else if (lower.includes('deepseek')) { brand = 'DeepSeek'; context = lower.includes('r1') ? '128K' : '64K'; }
    else if (lower.includes('gemini')) { brand = 'Google'; context = '1M'; }
    else if (lower.includes('llama')) { brand = 'Meta'; context = '128K'; }
    else if (lower.includes('mistral') || lower.includes('codestral')) { brand = 'Mistral'; context = '32K'; }
    else if (providerKind === 'ollama') { brand = 'Local'; context = 'Local Host'; }
    return { brand, context };
  };

  const filteredModels = useMemo(() => {
    if (!modelSearch.trim()) return models;
    const term = modelSearch.toLowerCase();
    return models.filter((m) => m.upstream_model_name.toLowerCase().includes(term));
  }, [models, modelSearch]);

  return (
    <div className="space-y-2.5 sm:space-y-3">
      <div className="flex items-center gap-1.5 sm:gap-2">
        <div className="relative flex-1 min-w-0">
          <Search className="w-3.5 h-3.5 text-text-muted absolute left-2.5 top-2" aria-hidden="true" />
          <input
            type="text"
            aria-label="Cari model upstream"
            placeholder="Cari model..."
            value={modelSearch}
            onChange={(e) => setModelSearch(e.target.value)}
            className="w-full pl-8 pr-2 py-1.5 text-xs bg-bg-surface-2 border border-border rounded-lg text-white font-mono placeholder:text-text-muted outline-none focus:border-accent"
          />
        </div>
        <Tooltip content="Tarik model dari upstream" position="top">
          <button
            type="button"
            onClick={() => handleSyncModels(selectedProvider)}
            disabled={syncingId === selectedProvider.id}
            aria-label="Tarik daftar model dari upstream"
            className="p-1.5 sm:p-2 rounded-lg bg-bg-surface-2 text-text-primary border border-border hover:bg-border/60 hover:text-white transition-colors cursor-pointer disabled:opacity-50 flex-shrink-0 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
          >
            {syncingId === selectedProvider.id ? <RefreshCw className="w-4 h-4 animate-spin" aria-hidden="true" /> : <DownloadCloud className="w-4 h-4" aria-hidden="true" />}
          </button>
        </Tooltip>
        <Tooltip content="Tambah model manual" position="top">
          <button
            type="button"
            onClick={onOpenAddModelModal}
            aria-label="Tambah model manual"
            className="p-1.5 sm:p-2 rounded-lg bg-accent text-black hover:bg-accent-hover transition-colors cursor-pointer flex-shrink-0 focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
          >
            <Plus className="w-4 h-4" aria-hidden="true" />
          </button>
        </Tooltip>
      </div>

      {filteredModels.length > 0 ? (
        <div className="space-y-2.5 min-h-0 max-h-[45dvh] sm:max-h-[40vh] overflow-y-auto overscroll-contain pr-0.5 scrollbar-thin">
          {filteredModels.map((model) => {
            const { brand, context } = getModelBadges(model.upstream_model_name, selectedProvider.kind);
            const isTesting = testingModelId === model.id;
            const testResult = modelTestResults[model.id];

            return (
              <div key={model.id} className="px-2.5 py-2 sm:p-3 rounded-xl border border-border bg-bg-surface-2/40 hover:border-border/80 transition-all space-y-1.5">
                <div className="flex items-center gap-1.5 min-w-0">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 flex-shrink-0" />
                  <span className="font-mono text-xs sm:text-sm font-bold text-white truncate min-w-0 flex-1">{model.upstream_model_name}</span>
                  <span className="hidden md:inline px-1.5 py-0.2 text-[10px] rounded bg-purple-500/10 text-purple-300 border border-purple-500/20 font-mono flex-shrink-0">{brand}</span>
                  <span className="hidden sm:inline px-1.5 py-0.2 text-[10px] rounded bg-blue-500/10 text-blue-300 border border-blue-500/20 font-mono flex-shrink-0">{context}</span>
                  <button
                    type="button"
                    onClick={() => void copyWithFeedback(model.upstream_model_name, `Model ID "${model.upstream_model_name}" disalin`, () => {
                      setCopiedModelId(model.id);
                      copyTimersRef.current.push(setTimeout(() => setCopiedModelId(null), 2000));
                    })}
                    aria-label={`Salin ID model ${model.upstream_model_name}`}
                    className="p-1.5 rounded-md text-text-muted hover:text-white hover:bg-bg-surface transition-colors cursor-pointer flex-shrink-0"
                  >
                    {copiedModelId === model.id ? <Check className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" /> : <Copy className="w-3.5 h-3.5" aria-hidden="true" />}
                  </button>
                  <button
                    type="button"
                    onClick={() => handleTestModel(model)}
                    disabled={isTesting}
                    aria-label={`Uji inferensi model ${model.upstream_model_name}`}
                    className="p-1.5 rounded-md text-text-muted hover:text-white hover:bg-bg-surface transition-colors cursor-pointer disabled:opacity-50 flex-shrink-0"
                  >
                    {isTesting ? <RefreshCw className="w-3.5 h-3.5 animate-spin text-accent" aria-hidden="true" /> : <FlaskConical className="w-3.5 h-3.5 text-accent" aria-hidden="true" />}
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDeleteModel(model)}
                    aria-label={`Hapus model ${model.upstream_model_name}`}
                    className="p-1.5 rounded-md text-text-muted hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer flex-shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
                  </button>
                </div>

                {testResult && (
                  <div className={`px-2 py-1.5 rounded-lg border text-[11px] flex items-center justify-between gap-2 animate-in fade-in duration-150 ${testResult.ok ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300' : 'bg-red-500/10 border-red-500/30 text-red-300'}`}>
                    <span className="flex items-center gap-1.5 min-w-0 font-bold truncate">
                      {testResult.ok ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" aria-hidden="true" /> : <AlertTriangle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" aria-hidden="true" />}
                      <span className="truncate">{testResult.ok ? 'Uji ok' : 'Uji gagal'}{testResult.latency_ms > 0 && ` · ${testResult.latency_ms} ms`}</span>
                    </span>
                    <span className="opacity-70 font-mono flex-shrink-0 text-[10px]">{testResult.timestamp}</span>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <div className="p-5 rounded-xl border border-dashed border-border text-center space-y-2">
          <Bot className="w-8 h-8 text-text-muted mx-auto" aria-hidden="true" />
          <p className="text-xs text-text-secondary">{modelSearch ? 'Tidak ada model yang cocok dengan kata kunci.' : 'Belum ada model upstream yang terhubung.'}</p>
          <div className="pt-2 flex justify-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => handleSyncModels(selectedProvider)} icon={<DownloadCloud className="w-3.5 h-3.5" aria-hidden="true" />}>Tarik Model Upstream</Button>
            <Button variant="primary" size="sm" onClick={onOpenAddModelModal} icon={<Plus className="w-3.5 h-3.5" aria-hidden="true" />}>Tambah Manual</Button>
          </div>
        </div>
      )}
    </div>
  );
};
