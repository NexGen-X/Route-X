import React, { useState } from 'react';
import {
  Terminal,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Key,
  Eye,
  EyeOff,
  Check,
  RefreshCw,
  Activity,
  Copy,
  ChevronDown,
  ChevronUp,
} from 'lucide-react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { statusKonfigurasiTool } from './statusTool';
import { Button } from '../common/Button';
import { Select } from '../common/Select';
import type { CLITool, Model, RoutingRule } from '../../types';
import type { ToolConfig, DiagStatus, ComboOption } from './types';
import { getToolCompleteSnippet } from './types';

export interface CLIToolCardProps {
  tool: CLITool;
  config: ToolConfig;
  models: Model[];
  rules: RoutingRule[];
  dynamicComboOptions: ComboOption[];
  // B-512: penanda sumber pendamping yang GAGAL dimuat di halaman
  // (B-225). Daftar kosong dari sumber yang gagal BUKAN katalog
  // kosong — klaim definitif "belum ada … terdaftar" digerbang
  // penanda ini supaya tidak bertabrakan dengan banner kegagalan
  // halaman di layar yang sama.
  modelsGagal?: boolean;
  rulesGagal?: boolean;
  userApiKey: string;
  publicBaseURL: string;
  isApplying: boolean;
  successMsg: string | undefined;
  warningMsg?: string | undefined;
  diag: DiagStatus | undefined;
  copiedKey: string | null;
  onModeChange: (toolId: string, mode: 'model_only' | 'routing' | 'combo') => void;
  onTargetChange: (toolId: string, target: string) => void;
  onApiKeyChange: (toolId: string, apiKey: string) => void;
  onApply: (toolId: string) => void;
  onTestConnection: (tool: CLITool) => void;
  onCopy: (text: string, key: string) => void;
}

export const CLIToolCard: React.FC<CLIToolCardProps> = ({
  tool,
  config,
  models,
  rules,
  dynamicComboOptions,
  modelsGagal = false,
  rulesGagal = false,
  userApiKey,
  publicBaseURL,
  isApplying,
  successMsg,
  warningMsg,
  diag,
  copiedKey,
  onModeChange,
  onTargetChange,
  onApiKeyChange,
  onApply,
  onTestConnection,
  onCopy,
}) => {
  const [showKey, setShowKey] = useState(false);
  const [isConfigOpen, setIsConfigOpen] = useState(false);
  const completeSnippet = getToolCompleteSnippet(tool, config, models, userApiKey, publicBaseURL);
  // Kejujuran katalog: tool yang secara arsitektur tidak dapat
  // diarahkan ke gateway ditampilkan apa adanya — tanpa tawaran
  // Configure, snippet, atau tombol Terapkan yang akan mengklaim
  // dukungan yang tidak ada.
  const tidakDidukung = tool.gateway_supported === false;

  const modeLabels: Record<'model_only' | 'routing' | 'combo', string> = {
    model_only: 'Model Langsung',
    routing: 'Smart Routing',
    combo: 'Combo Cascade',
  };

  return (
    <Card
      key={tool.id}
      className={`p-5 flex flex-col justify-between transition-all duration-200 rounded-card shadow-sm ${
        tool.installed
          ? 'border-border hover:border-border/80 bg-bg-surface'
          : 'border-border-subtle opacity-75 bg-bg-surface-2'
      }`}
    >
      <div className="space-y-3.5">
        {/* Header Kartu: Icon, Nama, Kategori & Status Badge */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div
              className={`w-10 h-10 rounded-inner flex items-center justify-center shrink-0 border ${
                tool.installed
                  ? 'bg-bg-surface-2 border-border text-text-primary'
                  : 'bg-bg-surface-3 border-border text-text-muted'
              }`}
            >
              <Terminal className="w-5 h-5 text-accent-text" />
            </div>
            <div className="truncate">
              <h4 className="text-base font-semibold text-white tracking-tight truncate">
                {tool.name}
              </h4>
              <span className="text-[11px] font-mono text-text-muted block truncate">
                {tool.category}
              </span>
            </div>
          </div>

          {/* Status Badge Minimalis */}
          <div className="shrink-0 pt-0.5">
            {tidakDidukung ? (
              <Badge variant="warn" className="gap-1 font-medium">
                <AlertTriangle className="w-3 h-3" />
                Tidak Dapat Diarahkan ke Gateway
              </Badge>
            ) : statusKonfigurasiTool(tool) === 'terkonfigurasi-routex' ? (
              <Badge variant="lime" className="gap-1 font-medium">
                <CheckCircle2 className="w-3 h-3" />
                Terarah ke Route-X
              </Badge>
            ) : statusKonfigurasiTool(tool) === 'belum-dikonfigurasi' ? (
              <Badge variant="warn" className="gap-1 font-medium">
                <AlertTriangle className="w-3 h-3" />
                Terpasang · Belum Dikonfigurasi
              </Badge>
            ) : (
              <Badge variant="neutral" className="gap-1 text-text-muted">
                <XCircle className="w-3 h-3" />
                Belum Ada
              </Badge>
            )}
          </div>
        </div>

        {/* Deskripsi Singkat */}
        <p className="text-xs text-text-secondary line-clamp-2 leading-relaxed">
          {tool.description}
        </p>

        {/* Alasan jujur untuk tool yang tidak dapat diarahkan ke gateway */}
        {tidakDidukung && (
          <p className="text-xs text-text-muted leading-relaxed">
            {tool.unsupported_reason ||
              'Tool ini tidak dapat diarahkan ke gateway Route-X.'}
          </p>
        )}

        {/* Box Command 1-Baris Siap Salin di Kartu Muka */}
        {!tidakDidukung && (
        <div className="bg-bg-surface-2 p-2.5 rounded-inner border border-border flex items-center justify-between text-xs font-mono">
          <span className="truncate pr-2 text-text-secondary select-all font-mono">
            {completeSnippet.split('\n')[0] || `export ${tool.env_var_api_key || 'API_KEY'}=...`}
          </span>
          <button
            type="button"
            onClick={() => onCopy(completeSnippet, `snippet-${tool.id}`)}
            className="p-1 rounded text-text-muted hover:text-white hover:bg-bg-surface-3 transition-colors cursor-pointer shrink-0"
            title="Salin perintah lengkap"
            aria-label={`Salin perintah untuk ${tool.name}`}
          >
            {copiedKey === `snippet-${tool.id}` ? (
              <Check className="w-3.5 h-3.5 text-accent-text" />
            ) : (
              <Copy className="w-3.5 h-3.5" />
            )}
          </button>
        </div>
        )}

        {/* Ringkasan Konfigurasi Aktif */}
        {!tidakDidukung && (
        <div className="flex items-center justify-between text-xs font-mono text-text-muted pt-0.5">
          <span className="truncate">
            Mode:{' '}
            <strong className="text-text-secondary font-medium">
              {modeLabels[config.mode]}
            </strong>
            {config.target ? ` (${config.target})` : ''}
          </span>
          <button
            type="button"
            onClick={() => setIsConfigOpen(!isConfigOpen)}
            className="inline-flex items-center gap-1 text-[11px] text-text-muted hover:text-white transition-colors cursor-pointer ml-2 shrink-0"
            aria-label={isConfigOpen ? "Ciutkan konfigurasi target" : "Buka konfigurasi target"}
          >
            <span>{isConfigOpen ? 'Tutup' : 'Ubah'}</span>
            {isConfigOpen ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
          </button>
        </div>
        )}

        {/* Collapsible Accordion: Konfigurasi Target & Mode */}
        {!tidakDidukung && isConfigOpen && (
          <div className="space-y-3 pt-3 border-t border-border animate-fade-in">
              <div>
                <label className="text-xs font-medium text-text-secondary block mb-1.5">
                  Mode Konfigurasi:
                </label>
                <div className="grid grid-cols-3 gap-1 p-1 bg-bg-surface-2 rounded-nav border border-border">
                  <button
                    type="button"
                    onClick={() => onModeChange(tool.id, 'model_only')}
                    className={`text-[11px] py-1 px-1.5 rounded font-medium transition-all cursor-pointer ${
                      config.mode === 'model_only'
                        ? 'bg-white text-black font-semibold shadow'
                        : 'text-text-muted hover:text-white'
                    }`}
                    title="Direct passthrough ke satu model tanpa failover"
                  >
                    1. Model
                  </button>

                  <button
                    type="button"
                    onClick={() => onModeChange(tool.id, 'routing')}
                    className={`text-[11px] py-1 px-1.5 rounded font-medium transition-all cursor-pointer ${
                      config.mode === 'routing'
                        ? 'bg-white text-black font-semibold shadow'
                        : 'text-text-muted hover:text-white'
                    }`}
                    title="Routing failover dinamis ke aturan rute"
                  >
                    2. Routing
                  </button>

                  <button
                    type="button"
                    onClick={() => onModeChange(tool.id, 'combo')}
                    className={`text-[11px] py-1 px-1.5 rounded font-medium transition-all cursor-pointer ${
                      config.mode === 'combo'
                        ? 'bg-white text-black font-semibold shadow'
                        : 'text-text-muted hover:text-white'
                    }`}
                    title="Smart Tiered Cascade (Hemat/Cepat -> Flagship Fallback)"
                  >
                    3. Combo
                  </button>
                </div>
              </div>

              {/* Target Selector Dropdown Sesuai Mode */}
              <div>
                <label className="text-xs font-medium text-text-secondary block mb-1.5">
                  {config.mode === 'model_only' && 'Target Model:'}
                  {config.mode === 'routing' && 'Target Rule Routing:'}
                  {config.mode === 'combo' && 'Smart Combo Cascade:'}
                </label>

                {config.mode === 'model_only' &&
                  (models.length > 0 ? (
                    <Select
                      value={config.target}
                      onChange={(val) => onTargetChange(tool.id, val)}
                      options={models.map((m) => ({
                        value: m.model_id,
                        label: `${m.display_name || m.model_id} (${m.family || 'universal'})`,
                      }))}
                    />
                  ) : (
                    <div className="p-2.5 rounded bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                      {modelsGagal
                        ? 'Daftar model gagal dimuat — katalog belum dapat dipastikan kosong.'
                        : 'Belum ada model terdaftar di katalog.'}
                    </div>
                  ))}

                {config.mode === 'routing' &&
                  (rules.length > 0 ? (
                    <Select
                      value={config.target}
                      onChange={(val) => onTargetChange(tool.id, val)}
                      options={rules.map((r) => ({
                        value: r.name,
                        label: `${r.name} (${r.strategy})`,
                      }))}
                    />
                  ) : (
                    <div className="p-2.5 rounded bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                      {rulesGagal
                        ? 'Daftar aturan routing gagal dimuat — belum dapat dipastikan kosong.'
                        : 'Belum ada aturan routing terdaftar di database.'}
                    </div>
                  ))}

                {config.mode === 'combo' &&
                  (dynamicComboOptions.length > 0 ? (
                    <div className="space-y-2">
                      <Select
                        value={config.target}
                        onChange={(val) => onTargetChange(tool.id, val)}
                        options={dynamicComboOptions}
                      />
                      <div className="p-2 rounded bg-bg-surface-2 border border-border text-[10px] font-mono space-y-1">
                        <div className="text-text-muted flex items-center justify-between">
                          <span>Alur Cascade:</span>
                          <span className="text-accent-text font-medium">Auto-Failover</span>
                        </div>
                        <div className="flex items-center gap-1.5 overflow-x-auto py-0.5">
                          <span className="px-1.5 py-0.5 rounded bg-bg-surface-3 border border-border text-white whitespace-nowrap">
                            {tool.name}
                          </span>
                          <span className="text-text-muted">➔</span>
                          <span className="px-1.5 py-0.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 whitespace-nowrap">
                            Tier 1 (Cepat)
                          </span>
                          <span className="text-text-muted">➔</span>
                          <span className="px-1.5 py-0.5 rounded bg-purple-500/10 border border-purple-500/30 text-purple-400 whitespace-nowrap">
                            Tier 2 (Flagship)
                          </span>
                        </div>
                      </div>
                    </div>
                  ) : (
                    <div className="p-2.5 rounded bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                      {rulesGagal
                        ? 'Daftar aturan routing gagal dimuat — combo belum dapat dipastikan kosong.'
                        : 'Belum ada aturan combo bertingkat.'}
                    </div>
                  ))}
              </div>

              {/* Kunci API Khusus Tool (Opsional) */}
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-[11px] font-medium text-text-muted flex items-center gap-1">
                    <Key className="w-3 h-3 text-accent-text" />
                    Kunci API Khusus:
                  </label>
                  {config.apiKey ? (
                    <span className="text-[10px] text-accent-text font-mono font-medium">Kustom</span>
                  ) : userApiKey ? (
                    <span className="text-[10px] text-emerald-400 font-mono">Memakai Global</span>
                  ) : (
                    <span className="text-[10px] text-text-muted font-mono">Opsional</span>
                  )}
                </div>
                <div className="relative">
                  <input
                    type={showKey ? 'text' : 'password'}
                    placeholder={
                      userApiKey
                        ? 'Memakai kunci global'
                        : `Isi ${tool.env_var_api_key || 'API Key'}...`
                    }
                    value={config.apiKey}
                    onChange={(e) => onApiKeyChange(tool.id, e.target.value)}
                    className="w-full bg-bg-surface-2 border border-border rounded px-2.5 py-1 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none font-mono pr-8"
                  />
                  <button
                    type="button"
                    onClick={() => setShowKey(!showKey)}
                    className="absolute right-2 top-1.5 text-text-muted hover:text-white cursor-pointer"
                    title={showKey ? 'Sembunyikan' : 'Tampilkan'}
                    aria-label={showKey ? "Sembunyikan kunci API" : "Tampilkan kunci API"}
                  >
                    {showKey ? (
                      <EyeOff className="w-3.5 h-3.5" aria-hidden="true" />
                    ) : (
                      <Eye className="w-3.5 h-3.5" aria-hidden="true" />
                    )}
                  </button>
                </div>
              </div>
          </div>
        )}
      </div>

      {/* Action Buttons & Real Live Diagnostics */}
      {!tidakDidukung && (
      <div className="mt-4 pt-3 border-t border-border space-y-2">
        {successMsg && (
          <div className="p-2 rounded bg-accent/10 border border-accent/30 text-[11px] font-mono text-accent-text flex items-center gap-1.5">
            <Check className="w-3.5 h-3.5 flex-shrink-0" />
            <span>{successMsg}</span>
          </div>
        )}

        {warningMsg && (
          <div className="p-2 rounded bg-amber-500/10 border border-amber-500/30 text-[11px] font-mono text-amber-300 flex items-center gap-1.5">
            <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0" />
            <span>{warningMsg}</span>
          </div>
        )}

        {/* Hasil Live Diagnostics */}
        {diag && (
          <div
            className={`p-2 rounded text-[11px] font-mono flex items-center justify-between gap-1.5 border ${
              diag.testing
                ? 'bg-bg-surface-2 border-border text-text-muted'
                : diag.ok
                ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400'
                : 'bg-red-500/10 border-red-500/30 text-red-400'
            }`}
          >
            <div className="flex items-center gap-1.5 truncate">
              {diag.testing ? (
                <RefreshCw className="w-3 h-3 animate-spin text-accent-text" />
              ) : diag.ok ? (
                <CheckCircle2 className="w-3.5 h-3.5 flex-shrink-0 text-emerald-400" />
              ) : (
                <XCircle className="w-3.5 h-3.5 flex-shrink-0 text-red-400" />
              )}
              <span className="truncate">
                {diag.testing ? 'Menguji sambungan inferensi...' : diag.message}
              </span>
            </div>
            {diag.latency !== undefined && !diag.testing && (
              <span className="text-[10px] font-bold opacity-80">{diag.latency}ms</span>
            )}
          </div>
        )}

        {!tidakDidukung && (
        <div className="flex items-center gap-2">
          {/* Tombol Terapkan ke CLI */}
          <Button
            variant="primary"
            size="sm"
            className="flex-1 text-xs min-h-[36px] font-semibold cursor-pointer"
            disabled={isApplying}
            onClick={() => onApply(tool.id)}
            title="Simpan preferensi dan sinkronkan langsung ke berkas konfigurasi host"
          >
            {isApplying ? (
              <>
                <RefreshCw className="w-3 h-3 animate-spin mr-1.5" />
                Menerapkan...
              </>
            ) : (
              'Terapkan ke CLI'
            )}
          </Button>

          {/* Tombol Uji Sambungan Inferensi Real */}
          <button
            type="button"
            onClick={() => onTestConnection(tool)}
            disabled={diag?.testing}
            title="Uji sambungan inferensi nyata (test token) ke endpoint Route-X"
            className="min-h-[36px] px-2.5 rounded-inner bg-bg-surface-2 hover:bg-bg-surface-3 border border-border text-xs text-text-secondary hover:text-white transition-all flex items-center gap-1 font-mono cursor-pointer"
          >
            {diag?.testing ? (
              <RefreshCw className="w-3.5 h-3.5 animate-spin text-accent-text" />
            ) : (
              <>
                <Activity className="w-3.5 h-3.5 text-accent-text" />
                <span className="text-[11px]">Uji</span>
              </>
            )}
          </button>

          {/* Tombol Salin Snippet Shell */}
          <Button
            variant="secondary"
            size="sm"
            className="min-h-[36px] px-2.5 text-xs gap-1 font-mono text-white cursor-pointer"
            title="Salin variabel lingkungan terminal lengkap"
            onClick={() => onCopy(completeSnippet, `snippet-${tool.id}`)}
          >
            {copiedKey === `snippet-${tool.id}` ? (
              <>
                <Check className="w-3.5 h-3.5 text-accent-text" />
                <span className="hidden sm:inline">Tersalin</span>
              </>
            ) : (
              <>
                <Copy className="w-3.5 h-3.5 text-text-muted" />
                <span className="hidden sm:inline">Salin</span>
              </>
            )}
          </Button>
        </div>
        )}
      </div>
      )}
    </Card>
  );
};
