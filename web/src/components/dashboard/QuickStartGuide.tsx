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
      <div className="h-10 px-3.5 bg-zinc-900/40 border border-zinc-800/80 rounded-lg flex items-center justify-between text-xs transition-colors shadow-sm">
        <div className="flex items-center gap-2 truncate">
          <Sparkles className="w-3.5 h-3.5 text-zinc-400 shrink-0" aria-hidden="true" />
          <span className="text-zinc-200 font-medium truncate">
            Panduan Cepat Memulai Route-X
          </span>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <button
            type="button"
            onClick={toggleCollapse}
            aria-label="Buka Panduan"
            className="inline-flex items-center gap-1 px-2.5 py-1 text-xs text-zinc-400 hover:text-white bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 rounded-md transition-colors cursor-pointer"
          >
            <span>Buka Panduan</span>
            <ChevronDown className="w-3.5 h-3.5" aria-hidden="true" />
          </button>
          <button
            type="button"
            onClick={dismissOnboarding}
            aria-label="tutup panduan"
            className="text-zinc-500 hover:text-white p-1 rounded-md hover:bg-zinc-800 transition-colors cursor-pointer"
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
      <div className="bg-zinc-900/40 border border-zinc-800/80 rounded-lg p-4 sm:p-5 shadow-sm relative transition-colors">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2.5">
            <div className="w-6 h-6 rounded bg-zinc-800 border border-zinc-700/60 flex items-center justify-center text-zinc-300">
              <Sparkles className="w-3.5 h-3.5" aria-hidden="true" />
            </div>
            <h3 className="text-sm font-medium text-white tracking-tight">
              Panduan Cepat Memulai Route-X
            </h3>
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={toggleCollapse}
              aria-label="Ciutkan Panduan"
              className="inline-flex items-center gap-1 px-2 py-1 text-xs text-zinc-400 hover:text-white rounded-md hover:bg-zinc-800 transition-colors cursor-pointer"
              title="Ciutkan panduan untuk mengosongkan ruang layar"
            >
              <span>Ciutkan Panduan</span>
              <ChevronUp className="w-3.5 h-3.5" aria-hidden="true" />
            </button>
            <button
              type="button"
              onClick={dismissOnboarding}
              aria-label="tutup panduan"
              className="text-zinc-500 hover:text-white p-1 rounded-md hover:bg-zinc-800 transition-colors cursor-pointer"
              title="Tutup panduan secara permanen"
            >
              <X className="w-4 h-4" aria-hidden="true" />
            </button>
          </div>
        </div>

        {/* 3 Step Cards — Lean Zinc Cards */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mt-4">
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
            className="p-3.5 rounded-lg bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 hover:bg-zinc-800/50 cursor-pointer transition-colors space-y-1.5 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 font-medium">
                Langkah 1
              </span>
              <Server className="w-3.5 h-3.5 text-zinc-500 group-hover:text-zinc-300 transition-colors" aria-hidden="true" />
            </div>
            <h4 className="text-xs font-medium text-white group-hover:text-zinc-200 transition-colors">
              Tambah Penyedia AI Upstream
            </h4>
            <p className="text-[11px] text-zinc-400 leading-relaxed">
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
            className="p-3.5 rounded-lg bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 hover:bg-zinc-800/50 cursor-pointer transition-colors space-y-1.5 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 font-medium">
                Langkah 2
              </span>
              <KeyRound className="w-3.5 h-3.5 text-zinc-500 group-hover:text-zinc-300 transition-colors" aria-hidden="true" />
            </div>
            <h4 className="text-xs font-medium text-white group-hover:text-zinc-200 transition-colors">
              Terbitkan Kunci API Klien
            </h4>
            <p className="text-[11px] text-zinc-400 leading-relaxed">
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
            className="p-3.5 rounded-lg bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 hover:bg-zinc-800/50 cursor-pointer transition-colors space-y-1.5 group"
          >
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 font-medium">
                Langkah 3
              </span>
              <Terminal className="w-3.5 h-3.5 text-zinc-500 group-hover:text-zinc-300 transition-colors" aria-hidden="true" />
            </div>
            <h4 className="text-xs font-medium text-white group-hover:text-zinc-200 transition-colors">
              Sambungkan Editor atau CLI
            </h4>
            <p className="text-[11px] text-zinc-400 leading-relaxed">
              Integrasikan Cursor, Claude Code, Cline, Antigravity, atau Aider dalam 1-klik.
            </p>
          </div>
        </div>

        {/* Tab Kode Instan */}
        <div className="mt-4 pt-3.5 border-t border-zinc-800/80">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 mb-2.5">
            <div className="flex items-center gap-2">
              <Code2 className="w-3.5 h-3.5 text-zinc-400" aria-hidden="true" />
              <span className="text-xs font-medium text-zinc-200">
                Tab Kode Instan (Uji Konektivitas Gateway)
              </span>
            </div>

            <div className="flex items-center gap-1 overflow-x-auto p-0.5 rounded-md bg-zinc-900 border border-zinc-800 scrollbar-none">
              {(Object.keys(CODE_SNIPPETS) as CodeTab[]).map((tab) => (
                <button
                  key={tab}
                  type="button"
                  onClick={() => setActiveCodeTab(tab)}
                  className={`px-2 py-0.5 text-xs font-mono rounded transition-colors cursor-pointer ${
                    activeCodeTab === tab
                      ? 'bg-zinc-800 text-white font-medium'
                      : 'text-zinc-400 hover:text-white'
                  }`}
                >
                  {CODE_SNIPPETS[tab].label}
                </button>
              ))}
            </div>
          </div>

          <div className="relative rounded-md bg-zinc-950 border border-zinc-800 overflow-hidden font-mono text-xs">
            <div className="flex items-center justify-between px-3 py-1.5 border-b border-zinc-800 bg-zinc-900/60 text-[11px] text-zinc-400">
              <span>{CODE_SNIPPETS[activeCodeTab].label} Integration</span>
              <button
                type="button"
                onClick={() => handleCopyCodeSnippet(CODE_SNIPPETS[activeCodeTab].code)}
                className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
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
            <pre className="p-3 overflow-x-auto text-zinc-300 leading-relaxed text-[11px] scrollbar-thin">
              {CODE_SNIPPETS[activeCodeTab].code}
            </pre>
          </div>
        </div>

        {/* Strip Resep Cepat 1-Klik */}
        <div className="mt-4 pt-3.5 border-t border-zinc-800/80">
          <div className="flex items-center justify-between mb-2.5">
            <span className="text-xs font-medium text-zinc-400 tracking-wide flex items-center gap-1.5 font-mono">
              <Sparkles className="w-3.5 h-3.5 text-zinc-500" aria-hidden="true" />
              Resep Cepat 1-Klik (Siap Pakai untuk Pemula)
            </span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
            <button
              type="button"
              onClick={() => setSelectedRecipe('coding')}
              className="p-3 rounded-lg bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 hover:bg-zinc-800/50 transition-colors text-left group cursor-pointer"
            >
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium text-zinc-200 group-hover:text-white transition-colors">
                  🛠️ Coding Asisten
                </span>
                <span className="text-[10px] px-1.5 py-0.2 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 font-mono">
                  Cursor / Claude
                </span>
              </div>
              <p className="text-[11px] text-zinc-400 leading-relaxed">
                Panduan konfigurasi instan untuk menghubungkan Cursor, Claude Code, atau Cline.
              </p>
            </button>

            <button
              type="button"
              onClick={() => setSelectedRecipe('budget')}
              className="p-3 rounded-lg bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 hover:bg-zinc-800/50 transition-colors text-left group cursor-pointer"
            >
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium text-zinc-200 group-hover:text-white transition-colors">
                  💰 Hemat Biaya 90%
                </span>
                <span className="text-[10px] px-1.5 py-0.2 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 font-mono">
                  DeepSeek / Groq
                </span>
              </div>
              <p className="text-[11px] text-zinc-400 leading-relaxed">
                Alihkan prompt harian ke model hemat dengan fallback cerdas ke model flagship.
              </p>
            </button>

            <button
              type="button"
              onClick={() => setSelectedRecipe('uptime')}
              className="p-3 rounded-lg bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 hover:bg-zinc-800/50 transition-colors text-left group cursor-pointer"
            >
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium text-zinc-200 group-hover:text-white transition-colors">
                  🛡️ Anti-Downtime
                </span>
                <span className="text-[10px] px-1.5 py-0.2 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 font-mono">
                  Auto Failover
                </span>
              </div>
              <p className="text-[11px] text-zinc-400 leading-relaxed">
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
            <Button variant="ghost" size="sm" onClick={() => setSelectedRecipe(null)}>
              Tutup
            </Button>
            <Button
              variant="primary"
              size="sm"
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
              <div className="p-3 rounded-lg bg-zinc-900 border border-zinc-800 text-zinc-300 leading-relaxed">
                Route-X mendukung <strong>Dual-Protocol</strong> native: OpenAI API (
                <code className="font-mono text-white">/v1/chat/completions</code>) dan Anthropic Claude (
                <code className="font-mono text-white">/v1/messages</code>) secara bersamaan!
              </div>

              <div className="space-y-2">
                <h4 className="font-medium text-white">1. Parameter Sambungan Gateway</h4>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 font-mono text-[11px]">
                  <div className="p-2.5 rounded-md bg-zinc-900 border border-zinc-800">
                    <span className="text-zinc-500 block text-[10px]">OpenAI Endpoint:</span>
                    <span className="text-zinc-200">http://localhost:8080/v1</span>
                  </div>
                  <div className="p-2.5 rounded-md bg-zinc-900 border border-zinc-800">
                    <span className="text-zinc-500 block text-[10px]">Anthropic Endpoint:</span>
                    <span className="text-zinc-200">http://localhost:8080</span>
                  </div>
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <h4 className="font-medium text-white">2. Ekspor Variabel Shell Seketika</h4>
                  <button
                    type="button"
                    onClick={() =>
                      handleCopyRecipeText(
                        `export OPENAI_BASE_URL="http://localhost:8080/v1"\nexport ANTHROPIC_BASE_URL="http://localhost:8080"\nexport OPENAI_API_KEY="your_routex_api_key"\nexport ANTHROPIC_API_KEY="your_routex_api_key"`
                      )
                    }
                    className="inline-flex items-center gap-1 text-[11px] text-zinc-300 hover:text-white font-mono cursor-pointer"
                  >
                    {recipeCopied ? (
                      <Check className="w-3 h-3 text-emerald-400" aria-hidden="true" />
                    ) : (
                      <Copy className="w-3 h-3" aria-hidden="true" />
                    )}
                    <span>{recipeCopied ? 'Tersalin!' : 'Salin Snippet'}</span>
                  </button>
                </div>
                <pre className="p-3 rounded-md bg-zinc-900 border border-zinc-800 font-mono text-[11px] text-zinc-300 overflow-x-auto leading-relaxed">
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
              <div className="p-3 rounded-lg bg-zinc-900 border border-zinc-800 text-zinc-300 leading-relaxed">
                Hemat anggaran token hingga <strong>90%</strong> dengan mengarahkan percakapan rutin ke model
                ultra-hemat (DeepSeek V3 / Groq LLaMA 3.3) dan hanya beralih ke model flagship saat diperlukan.
              </div>

              <div className="space-y-2">
                <h4 className="font-medium text-white">Langkah Mudah Penerapan:</h4>
                <ol className="list-decimal list-inside space-y-1.5 text-zinc-400">
                  <li>
                    Buka menu <strong className="text-zinc-200">Penyedia AI</strong> dan tambahkan API Key DeepSeek atau Groq.
                  </li>
                  <li>
                    Buka menu <strong className="text-zinc-200">Perutean Cerdas</strong>, pilih mode Combo Cascade.
                  </li>
                  <li>
                    Setel Tier 1 ke DeepSeek V3, dan Tier 2 ke Claude 3.5 Sonnet / GPT-4o.
                  </li>
                </ol>
              </div>
            </>
          )}

          {selectedRecipe === 'uptime' && (
            <>
              <div className="p-3 rounded-lg bg-zinc-900 border border-zinc-800 text-zinc-300 leading-relaxed">
                Jaminan keandalan tinggi: Jika OpenAI mengalami lonjakan error atau limit kuota, Route-X otomatis mengalihkan request ke Anthropic atau Google dalam &lt; 200 ms.
              </div>

              <div className="space-y-2">
                <h4 className="font-medium text-white">Langkah Pengaktifan:</h4>
                <ol className="list-decimal list-inside space-y-1.5 text-zinc-400">
                  <li>
                    Pastikan minimal 2 Penyedia AI upstream terhubung.
                  </li>
                  <li>
                    Buka menu <strong className="text-zinc-200">Perutean Cerdas</strong>, pilih mode Failover.
                  </li>
                  <li>
                    Centang kedua provider untuk mengaktifkan recovery otomatis.
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
