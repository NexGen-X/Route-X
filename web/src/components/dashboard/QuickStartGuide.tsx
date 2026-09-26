import React, { useState, useRef } from 'react';
import {
  Sparkles,
  X,
  Server,
  KeyRound,
  Terminal,
  Copy,
  Check,
  ChevronRight,
  ChevronDown,
  ChevronUp,
} from 'lucide-react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { copyTextToClipboard } from '../../utils/clipboard';

export interface QuickStartGuideProps {
  onNavigate: (path: string) => void;
}

export const QuickStartGuide: React.FC<QuickStartGuideProps> = ({ onNavigate }) => {
  const [showOnboarding, setShowOnboarding] = useState<boolean>(() => {
    try {
      return localStorage.getItem('routex_dismiss_onboarding') !== 'true';
    } catch {
      return true;
    }
  });

  const [isCollapsed, setIsCollapsed] = useState<boolean>(() => {
    try {
      return localStorage.getItem('routex_quickstart_collapsed') === 'true';
    } catch {
      return false;
    }
  });

  const [selectedRecipe, setSelectedRecipe] = useState<'coding' | 'budget' | 'uptime' | null>(null);
  const [recipeCopied, setRecipeCopied] = useState(false);
  const recipeCopyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleCopyRecipeText = (text: string) => {
    copyTextToClipboard(text);
    setRecipeCopied(true);
    if (recipeCopyTimerRef.current) clearTimeout(recipeCopyTimerRef.current);
    recipeCopyTimerRef.current = setTimeout(() => setRecipeCopied(false), 2000);
  };

  const dismissOnboarding = () => {
    setShowOnboarding(false);
    try {
      localStorage.setItem('routex_dismiss_onboarding', 'true');
    } catch {
      // Abaikan kegagalan localStorage pada restricted context
    }
  };

  const toggleCollapse = () => {
    setIsCollapsed((prev) => {
      const next = !prev;
      try {
        localStorage.setItem('routex_quickstart_collapsed', String(next));
      } catch {
        // Abaikan kegagalan localStorage pada restricted context
      }
      return next;
    });
  };

  if (!showOnboarding) {
    return null;
  }

  // Tampilan ciut (Collapsed Compact Bar setinggi 44px)
  if (isCollapsed) {
    return (
      <div className="h-11 px-4 bg-bg-surface border border-border rounded-card flex items-center justify-between text-xs transition-all shadow-sm">
        <div className="flex items-center gap-2 truncate">
          <span className="text-white font-medium truncate">
            🚀 Panduan Mulai Cepat (3 Langkah Integrasi)
          </span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <button
            type="button"
            onClick={toggleCollapse}
            aria-label="Buka Panduan"
            className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-text-secondary hover:text-white bg-bg-surface-2 hover:bg-bg-surface-3 border border-border rounded-nav transition-colors cursor-pointer"
          >
            <span>Buka Panduan</span>
            <ChevronDown className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={dismissOnboarding}
            aria-label="Tutup panduan"
            className="text-text-muted hover:text-white p-1 rounded-nav hover:bg-bg-surface-2 transition-colors cursor-pointer"
            title="Tutup panduan secara permanen"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="bg-bg-surface border border-border rounded-card p-5 sm:p-6 shadow-sm relative overflow-hidden transition-all">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <Sparkles className="w-4 h-4 text-accent" />
            <h3 className="text-sm font-semibold text-white tracking-tight">
              Panduan Cepat Memulai Route-X
            </h3>
          </div>
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={toggleCollapse}
              aria-label="Ciutkan Panduan"
              className="inline-flex items-center gap-1 px-2.5 py-1 text-xs text-text-muted hover:text-white rounded-nav hover:bg-bg-surface-2 transition-colors cursor-pointer"
              title="Ciutkan panduan untuk mengosongkan ruang layar"
            >
              <span>Ciutkan Panduan</span>
              <ChevronUp className="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              onClick={dismissOnboarding}
              aria-label="Tutup panduan"
              className="text-text-muted hover:text-white p-1 rounded-nav hover:bg-bg-surface-2 transition-colors cursor-pointer"
              title="Tutup panduan secara permanen"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-3.5 mt-5">
          {/* Step 1 */}
          <div
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onNavigate('/upstreams/providers');
              }
            }}
            onClick={() => onNavigate('/upstreams/providers')}
            className="p-4 rounded-inner bg-bg-surface-2 border border-border hover:border-accent/40 cursor-pointer transition-all space-y-2 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-bg-surface-3 text-text-secondary border border-border font-semibold">
                Langkah 1
              </span>
              <Server className="w-4 h-4 text-text-muted group-hover:text-accent transition-colors" />
            </div>
            <h4 className="text-xs font-semibold text-white group-hover:text-accent transition-colors">
              Tambah Penyedia AI Upstream
            </h4>
          </div>

          {/* Step 2 */}
          <div
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onNavigate('/access/api-keys');
              }
            }}
            onClick={() => onNavigate('/access/api-keys')}
            className="p-4 rounded-inner bg-bg-surface-2 border border-border hover:border-accent/40 cursor-pointer transition-all space-y-2 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-bg-surface-3 text-text-secondary border border-border font-semibold">
                Langkah 2
              </span>
              <KeyRound className="w-4 h-4 text-text-muted group-hover:text-accent transition-colors" />
            </div>
            <h4 className="text-xs font-semibold text-white group-hover:text-accent transition-colors">
              Terbitkan Kunci API Klien
            </h4>
          </div>

          {/* Step 3 */}
          <div
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onNavigate('/cli-integrations');
              }
            }}
            onClick={() => onNavigate('/cli-integrations')}
            className="p-4 rounded-inner bg-bg-surface-2 border border-border hover:border-accent/40 cursor-pointer transition-all space-y-2 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-bg-surface-3 text-text-secondary border border-border font-semibold">
                Langkah 3
              </span>
              <Terminal className="w-4 h-4 text-text-muted group-hover:text-accent transition-colors" />
            </div>
            <h4 className="text-xs font-semibold text-white group-hover:text-accent transition-colors">
              Sambungkan Editor atau CLI
            </h4>
          </div>
        </div>

        {/* Strip Resep Cepat 1-Klik untuk Pemula */}
        <div className="mt-5 pt-4 border-t border-border">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-1.5 font-mono">
              <Sparkles className="w-3.5 h-3.5 text-accent" />
              Resep Cepat 1-Klik (Siap Pakai untuk Pemula)
            </span>
            <span className="text-[11px] text-text-muted hidden sm:inline">
              Pilih skenario penggunaan Anda
            </span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <button
              type="button"
              onClick={() => setSelectedRecipe('coding')}
              className="p-3.5 rounded-inner bg-bg-surface-2 border border-border hover:border-sky-500/40 hover:bg-sky-500/5 transition-all text-left group cursor-pointer"
            >
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs font-semibold text-white group-hover:text-sky-300 transition-colors flex items-center gap-1.5">
                  🛠️ Coding Asisten
                </span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-sky-500/10 text-sky-300 border border-sky-500/20 font-mono">
                  Cursor / Claude
                </span>
              </div>
              <p className="text-[11px] text-text-muted leading-relaxed">
                Panduan &amp; konfigurasi instan untuk menghubungkan Cursor, Claude Code, atau Cline.
              </p>
            </button>

            <button
              type="button"
              onClick={() => setSelectedRecipe('budget')}
              className="p-3.5 rounded-inner bg-bg-surface-2 border border-border hover:border-emerald-500/40 hover:bg-emerald-500/5 transition-all text-left group cursor-pointer"
            >
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs font-semibold text-white group-hover:text-emerald-300 transition-colors flex items-center gap-1.5">
                  💰 Hemat Biaya 90%
                </span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-300 border border-emerald-500/20 font-mono">
                  DeepSeek / Groq
                </span>
              </div>
              <p className="text-[11px] text-text-muted leading-relaxed">
                Alihkan prompt harian ke model hemat dengan fallback cerdas ke model flagship.
              </p>
            </button>

            <button
              type="button"
              onClick={() => setSelectedRecipe('uptime')}
              className="p-3.5 rounded-inner bg-bg-surface-2 border border-border hover:border-purple-500/40 hover:bg-purple-500/5 transition-all text-left group cursor-pointer"
            >
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs font-semibold text-white group-hover:text-purple-300 transition-colors flex items-center gap-1.5">
                  🛡️ Anti-Downtime
                </span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-purple-500/10 text-purple-300 border border-purple-500/20 font-mono">
                  Auto Failover
                </span>
              </div>
              <p className="text-[11px] text-text-muted leading-relaxed">
                Kombinasi multi-upstream: jika OpenAI sibuk/error, otomatis beralih ke Claude/Gemini.
              </p>
            </button>
          </div>
        </div>
      </div>

      {/* Modal Panduan Resep 1-Klik */}
      <Modal
        isOpen={!!selectedRecipe}
        onClose={() => setSelectedRecipe(null)}
        title={
          selectedRecipe === 'coding'
            ? '🛠️ Resep: Hubungkan Coding Assistant (Cursor / Claude Code / Cline)'
            : selectedRecipe === 'budget'
            ? '💰 Resep: Hemat Biaya Inferensi Hingga 90%'
            : '🛡️ Resep: Anti-Downtime dengan Auto-Failover Multi-Provider'
        }
        maxWidth="2xl"
        footer={
          <div className="flex items-center justify-between w-full">
            <Button variant="ghost" onClick={() => setSelectedRecipe(null)}>
              Tutup
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                const target =
                  selectedRecipe === 'coding' ? '/cli-integrations' : '/upstreams/routing';
                setSelectedRecipe(null);
                onNavigate(target);
              }}
              icon={<ChevronRight className="w-4 h-4" />}
            >
              {selectedRecipe === 'coding' ? 'Buka Pengaturan CLI Lengkap' : 'Atur Routing Cerdas'}
            </Button>
          </div>
        }
      >
        <div className="space-y-4 text-xs">
          {selectedRecipe === 'coding' && (
            <>
              <div className="p-3.5 rounded-xl bg-sky-500/10 border border-sky-500/20 text-sky-200 leading-relaxed">
                Route-X mendukung <strong>Dual-Protocol</strong> native: OpenAI API (
                <code className="font-mono text-white">/v1/chat/completions</code>) dan Anthropic Claude (
                <code className="font-mono text-white">/v1/messages</code>) secara bersamaan!
              </div>

              <div className="space-y-2">
                <h4 className="font-semibold text-white">1. Parameter Sambungan Gateway</h4>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 font-mono text-[11px]">
                  <div className="p-2.5 rounded-lg bg-bg-surface-2 border border-border">
                    <span className="text-text-muted block text-[10px]">OpenAI Endpoint:</span>
                    <span className="text-accent">http://localhost:8080/v1</span>
                  </div>
                  <div className="p-2.5 rounded-lg bg-bg-surface-2 border border-border">
                    <span className="text-text-muted block text-[10px]">Anthropic Endpoint:</span>
                    <span className="text-accent">http://localhost:8080</span>
                  </div>
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <h4 className="font-semibold text-white">2. Ekspor Variabel Shell Seketika</h4>
                  <button
                    type="button"
                    onClick={() =>
                      handleCopyRecipeText(
                        `export OPENAI_BASE_URL="http://localhost:8080/v1"\nexport ANTHROPIC_BASE_URL="http://localhost:8080"\nexport OPENAI_API_KEY="your_routex_api_key"\nexport ANTHROPIC_API_KEY="your_routex_api_key"`
                      )
                    }
                    className="inline-flex items-center gap-1 text-[11px] text-accent hover:underline font-mono cursor-pointer"
                  >
                    {recipeCopied ? (
                      <Check className="w-3 h-3 text-emerald-400" />
                    ) : (
                      <Copy className="w-3 h-3" />
                    )}
                    <span>{recipeCopied ? 'Tersalin!' : 'Salin Snippet'}</span>
                  </button>
                </div>
                <pre className="p-3 rounded-xl bg-bg-surface-2 border border-border font-mono text-[11px] text-text-primary overflow-x-auto leading-relaxed">
{`export OPENAI_BASE_URL="http://localhost:8080/v1"
export ANTHROPIC_BASE_URL="http://localhost:8080"
export OPENAI_API_KEY="your_routex_api_key"
export ANTHROPIC_API_KEY="your_routex_api_key"`}
                </pre>
              </div>
            </>
          )}

          {selectedRecipe === 'budget' && (
            <>
              <div className="p-3.5 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-200 leading-relaxed">
                Hemat anggaran token hingga <strong>90%</strong> dengan mengarahkan percakapan rutin ke model
                ultra-hemat (DeepSeek V3 / Groq LLaMA 3.3) dan hanya beralih ke model flagship saat diperlukan.
              </div>

              <div className="space-y-2">
                <h4 className="font-semibold text-white">Langkah Mudah Penerapan:</h4>
                <ol className="list-decimal list-inside space-y-1.5 text-text-secondary">
                  <li>
                    Buka menu <strong className="text-white">Penyedia AI</strong> dan tambahkan API Key DeepSeek atau Groq.
                  </li>
                  <li>
                    Buka menu <strong className="text-white">Perutean Cerdas</strong>, pilih mode{' '}
                    <strong className="text-accent">Combo Cascade</strong>.
                  </li>
                  <li>
                    Setel <em>Tier 1 (Utama)</em> ke DeepSeek V3, dan <em>Tier 2 (Cadangan)</em> ke Claude 3.5
                    Sonnet / GPT-4o.
                  </li>
                </ol>
              </div>
            </>
          )}

          {selectedRecipe === 'uptime' && (
            <>
              <div className="p-3.5 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-200 leading-relaxed">
                Jaminan keandalan tinggi (<em>High Availability</em>): Jika OpenAI mengalami lonjakan error (503 / 500)
                atau batas kuota (429), Route-X otomatis mengalihkan request ke Anthropic atau Google dalam &lt; 200
                milidetik.
              </div>

              <div className="space-y-2">
                <h4 className="font-semibold text-white">Langkah Pengaktifan:</h4>
                <ol className="list-decimal list-inside space-y-1.5 text-text-secondary">
                  <li>
                    Pastikan minimal 2 Penyedia AI upstream terhubung (misal: OpenAI + Anthropic).
                  </li>
                  <li>
                    Buka menu <strong className="text-white">Perutean Cerdas</strong>, pilih mode{' '}
                    <strong className="text-purple-300">Failover (Priority)</strong>.
                  </li>
                  <li>
                    Centang kedua provider; Route-X akan otomatis menangani pemulihan saat provider utama
                    sibuk.
                  </li>
                </ol>
              </div>
            </>
          )}
        </div>
      </Modal>
    </>
  );
};
