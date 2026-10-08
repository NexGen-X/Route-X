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
  Code2,
} from 'lucide-react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { copyTextToClipboard } from '../../utils/clipboard';

export interface QuickStartGuideProps {
  onNavigate: (path: string) => void;
  onDismiss?: () => void;
}

type CodeTab = 'curl' | 'python' | 'typescript' | 'shell';

const CODE_SNIPPETS: Record<CodeTab, { label: string; language: string; code: string }> = {
  curl: {
    label: 'cURL',
    language: 'bash',
    code: `curl http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer rx_live_secret_key" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello Route-X AI Gateway!"}]
  }'`,
  },
  python: {
    label: 'Python SDK',
    language: 'python',
    code: `from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="rx_live_secret_key"
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello Route-X AI Gateway!"}]
)
print(response.choices[0].message.content)`,
  },
  typescript: {
    label: 'TypeScript',
    language: 'typescript',
    code: `import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:8080/v1',
  apiKey: 'rx_live_secret_key',
});

const response = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Hello Route-X AI Gateway!' }],
});
console.log(response.choices[0].message.content);`,
  },
  shell: {
    label: 'Shell Env',
    language: 'bash',
    code: `export OPENAI_BASE_URL="http://localhost:8080/v1"
export ANTHROPIC_BASE_URL="http://localhost:8080"
export OPENAI_API_KEY="rx_live_secret_key"
export ANTHROPIC_API_KEY="rx_live_secret_key"`,
  },
};

export const QuickStartGuide: React.FC<QuickStartGuideProps> = ({ onNavigate, onDismiss }) => {
  const [showOnboarding, setShowOnboarding] = useState<boolean>(() => {
    try {
      return (
        localStorage.getItem('routex-quickstart-dismissed') !== '1' &&
        localStorage.getItem('routex_dismiss_onboarding') !== 'true'
      );
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

  const [activeCodeTab, setActiveCodeTab] = useState<CodeTab>('curl');
  const [codeCopied, setCodeCopied] = useState(false);
  const codeCopyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [selectedRecipe, setSelectedRecipe] = useState<'coding' | 'budget' | 'uptime' | null>(null);
  const [recipeCopied, setRecipeCopied] = useState(false);
  const recipeCopyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleCopyCodeSnippet = (text: string) => {
    copyTextToClipboard(text);
    setCodeCopied(true);
    if (codeCopyTimerRef.current) clearTimeout(codeCopyTimerRef.current);
    codeCopyTimerRef.current = setTimeout(() => setCodeCopied(false), 2000);
  };

  const handleCopyRecipeText = (text: string) => {
    copyTextToClipboard(text);
    setRecipeCopied(true);
    if (recipeCopyTimerRef.current) clearTimeout(recipeCopyTimerRef.current);
    recipeCopyTimerRef.current = setTimeout(() => setRecipeCopied(false), 2000);
  };

  const dismissOnboarding = () => {
    setShowOnboarding(false);
    try {
      localStorage.setItem('routex-quickstart-dismissed', '1');
      localStorage.setItem('routex_dismiss_onboarding', 'true');
    } catch {
      // Abaikan kegagalan localStorage
    }
    onDismiss?.();
  };

  const toggleCollapse = () => {
    setIsCollapsed((prev) => {
      const next = !prev;
      try {
        localStorage.setItem('routex_quickstart_collapsed', String(next));
      } catch {
        // Abaikan
      }
      return next;
    });
  };

  if (!showOnboarding) {
    return null;
  }

  // Tampilan ciut (Collapsed Compact Bar)
  if (isCollapsed) {
    return (
      <div className="h-12 px-4 bg-bg-surface border border-border/80 rounded-card flex items-center justify-between text-xs transition-all shadow-sm">
        <div className="flex items-center gap-2 truncate">
          <Sparkles className="w-4 h-4 text-blue-400 shrink-0" aria-hidden="true" />
          <span className="text-white font-medium truncate">
            Panduan Cepat Memulai Route-X (3 Langkah Integrasi)
          </span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <button
            type="button"
            onClick={toggleCollapse}
            aria-label="Buka Panduan"
            className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-text-secondary hover:text-white bg-bg-surface-2 hover:bg-bg-surface-3 border border-border/80 rounded-lg transition-colors cursor-pointer"
          >
            <span>Buka Panduan</span>
            <ChevronDown className="w-3.5 h-3.5" aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={dismissOnboarding}
            aria-label="tutup panduan"
            className="text-text-muted hover:text-white p-2 rounded-lg hover:bg-bg-surface-2 transition-colors cursor-pointer min-w-[36px] min-h-[36px] flex items-center justify-center"
            title="Tutup panduan secara permanen"
          >
            <X className="w-4 h-4" aria-hidden="true" />
          </button>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="bg-bg-surface border border-border/80 rounded-card p-5 sm:p-6 shadow-sm relative overflow-hidden transition-all">
        {/* Ambient subtle glow background */}
        <div
          className="pointer-events-none absolute -top-24 right-10 w-96 h-40 bg-blue-500/10 rounded-full blur-3xl"
          aria-hidden="true"
        />

        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2.5">
            <div className="p-1.5 rounded-lg bg-blue-500/10 text-blue-400 border border-blue-500/20">
              <Sparkles className="w-4 h-4" aria-hidden="true" />
            </div>
            <h3 className="text-sm font-semibold text-white tracking-tight">
              Panduan Cepat Memulai Route-X
            </h3>
          </div>
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={toggleCollapse}
              aria-label="Ciutkan Panduan"
              className="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs text-text-muted hover:text-white rounded-lg hover:bg-bg-surface-2 transition-colors cursor-pointer"
              title="Ciutkan panduan untuk mengosongkan ruang layar"
            >
              <span>Ciutkan Panduan</span>
              <ChevronUp className="w-3.5 h-3.5" aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={dismissOnboarding}
              aria-label="tutup panduan"
              className="text-text-muted hover:text-white p-2 rounded-lg hover:bg-bg-surface-2 transition-colors cursor-pointer min-w-[36px] min-h-[36px] flex items-center justify-center"
              title="Tutup panduan secara permanen"
            >
              <X className="w-4 h-4" aria-hidden="true" />
            </button>
          </div>
        </div>

        {/* 3 Step Cards */}
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
            className="p-4 rounded-xl bg-bg-surface-2/60 border border-border/80 hover:border-blue-500/40 hover:bg-bg-surface-2 cursor-pointer transition-all space-y-2 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-mono px-2 py-0.5 rounded-md bg-bg-surface-3 text-text-secondary border border-border/60 font-semibold">
                Langkah 1
              </span>
              <Server className="w-4 h-4 text-text-muted group-hover:text-blue-400 transition-colors" aria-hidden="true" />
            </div>
            <h4 className="text-xs font-semibold text-white group-hover:text-blue-400 transition-colors">
              Tambah Penyedia AI Upstream
            </h4>
            <p className="text-[11px] text-text-muted leading-relaxed">
              Daftarkan OpenAI, Anthropic, Gemini, Groq, atau Ollama dengan API Key Anda.
            </p>
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
            className="p-4 rounded-xl bg-bg-surface-2/60 border border-border/80 hover:border-blue-500/40 hover:bg-bg-surface-2 cursor-pointer transition-all space-y-2 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-mono px-2 py-0.5 rounded-md bg-bg-surface-3 text-text-secondary border border-border/60 font-semibold">
                Langkah 2
              </span>
              <KeyRound className="w-4 h-4 text-text-muted group-hover:text-blue-400 transition-colors" aria-hidden="true" />
            </div>
            <h4 className="text-xs font-semibold text-white group-hover:text-blue-400 transition-colors">
              Terbitkan Kunci API Klien
            </h4>
            <p className="text-[11px] text-text-muted leading-relaxed">
              Buat Bearer token aman untuk mengautentikasi aplikasi, script, dan CLI Anda.
            </p>
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
            className="p-4 rounded-xl bg-bg-surface-2/60 border border-border/80 hover:border-blue-500/40 hover:bg-bg-surface-2 cursor-pointer transition-all space-y-2 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-mono px-2 py-0.5 rounded-md bg-bg-surface-3 text-text-secondary border border-border/60 font-semibold">
                Langkah 3
              </span>
              <Terminal className="w-4 h-4 text-text-muted group-hover:text-blue-400 transition-colors" aria-hidden="true" />
            </div>
            <h4 className="text-xs font-semibold text-white group-hover:text-blue-400 transition-colors">
              Sambungkan Editor atau CLI
            </h4>
            <p className="text-[11px] text-text-muted leading-relaxed">
              Integrasikan Cursor, Claude Code, Cline, Antigravity, atau Aider dalam 1-klik.
            </p>
          </div>
        </div>

        {/* Tab Kode Instan (Instant Code Snippets) */}
        <div className="mt-5 pt-4 border-t border-border/60">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 mb-3">
            <div className="flex items-center gap-2">
              <Code2 className="w-4 h-4 text-blue-400" aria-hidden="true" />
              <span className="text-xs font-semibold text-white tracking-wide">
                Tab Kode Instan (Uji Konektivitas Gateway)
              </span>
            </div>

            <div className="flex items-center gap-1.5 overflow-x-auto p-1 rounded-xl bg-bg-surface-2/70 border border-border/70 scrollbar-none">
              {(Object.keys(CODE_SNIPPETS) as CodeTab[]).map((tab) => (
                <button
                  key={tab}
                  type="button"
                  onClick={() => setActiveCodeTab(tab)}
                  className={`px-2.5 py-1 text-xs font-mono rounded-lg transition-all cursor-pointer ${
                    activeCodeTab === tab
                      ? 'bg-blue-600 text-white font-medium shadow-sm'
                      : 'text-text-muted hover:text-white hover:bg-bg-surface-3/50'
                  }`}
                >
                  {CODE_SNIPPETS[tab].label}
                </button>
              ))}
            </div>
          </div>

          <div className="relative rounded-xl bg-bg-surface-2/80 border border-border/80 overflow-hidden font-mono text-xs">
            <div className="flex items-center justify-between px-3.5 py-2 border-b border-border/60 bg-bg-surface-3/30 text-[11px] text-text-muted">
              <span>{CODE_SNIPPETS[activeCodeTab].label} Integration</span>
              <button
                type="button"
                onClick={() => handleCopyCodeSnippet(CODE_SNIPPETS[activeCodeTab].code)}
                className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] text-blue-400 hover:text-blue-300 hover:bg-blue-500/10 transition-colors cursor-pointer"
              >
                {codeCopied ? (
                  <>
                    <Check className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" />
                    <span className="text-emerald-400">Tersalin!</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3.5 h-3.5" aria-hidden="true" />
                    <span>Salin Kode</span>
                  </>
                )}
              </button>
            </div>
            <pre className="p-3.5 overflow-x-auto text-text-primary leading-relaxed text-[11px] scrollbar-thin">
              {CODE_SNIPPETS[activeCodeTab].code}
            </pre>
          </div>
        </div>

        {/* Strip Resep Cepat 1-Klik untuk Pemula */}
        <div className="mt-5 pt-4 border-t border-border/60">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-semibold text-text-secondary uppercase tracking-wider flex items-center gap-1.5 font-mono">
              <Sparkles className="w-3.5 h-3.5 text-blue-400" aria-hidden="true" />
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
              className="p-3.5 rounded-xl bg-bg-surface-2/60 border border-border/80 hover:border-sky-500/40 hover:bg-sky-500/5 transition-all text-left group cursor-pointer"
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
              className="p-3.5 rounded-xl bg-bg-surface-2/60 border border-border/80 hover:border-emerald-500/40 hover:bg-emerald-500/5 transition-all text-left group cursor-pointer"
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
              className="p-3.5 rounded-xl bg-bg-surface-2/60 border border-border/80 hover:border-purple-500/40 hover:bg-purple-500/5 transition-all text-left group cursor-pointer"
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
                  selectedRecipe === 'coding' ? '/cli-integrations' : '/gateway/routing';
                setSelectedRecipe(null);
                onNavigate(target);
              }}
              icon={<ChevronRight className="w-4 h-4" aria-hidden="true" />}
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
                    <span className="text-blue-400">http://localhost:8080/v1</span>
                  </div>
                  <div className="p-2.5 rounded-lg bg-bg-surface-2 border border-border">
                    <span className="text-text-muted block text-[10px]">Anthropic Endpoint:</span>
                    <span className="text-blue-400">http://localhost:8080</span>
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
                    className="inline-flex items-center gap-1 text-[11px] text-blue-400 hover:underline font-mono cursor-pointer"
                  >
                    {recipeCopied ? (
                      <Check className="w-3 h-3 text-emerald-400" aria-hidden="true" />
                    ) : (
                      <Copy className="w-3 h-3" aria-hidden="true" />
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
                    <strong className="text-blue-400">Combo Cascade</strong>.
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
