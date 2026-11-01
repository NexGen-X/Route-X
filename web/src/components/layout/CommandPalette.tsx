import React, { useState, useEffect, useRef } from 'react';
import FocusTrap from 'focus-trap-react';
import {
  Search,
  LayoutDashboard,
  Terminal,
  Activity,
  Server,
  GitFork,
  Code2,
  KeyRound,
  Coins,
  Sliders,
  HeartPulse,
  Copy,
  Globe,
  ArrowRight,
  Sparkles,
  CornerDownLeft,
} from 'lucide-react';
import { useToast } from '../../context/ToastContext';
import { copyTextToClipboard } from '../../utils/clipboard';
import { muatJumlahToolCli } from '../../lib/cliCount';

export interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  onNavigate: (path: string) => void;
  onSelectCLI?: (toolId: string) => void;
}

interface PaletteItem {
  id: string;
  title: string;
  subtitle?: string;
  category: 'Navigasi' | 'Aksi Cepat' | 'Perkakas CLI';
  icon: React.ComponentType<{ className?: string }>;
  action: () => void;
  badge?: string;
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({
  isOpen,
  onClose,
  onNavigate,
  onSelectCLI,
}) => {
  const { toast } = useToast();
  const [query, setQuery] = useState('');
  // Badge jumlah tool pada perintah CLI dihitung dari registri backend
  // (cache bersama lib/cliCount) — bukan konstanta.
  const [jumlahToolCli, setJumlahToolCli] = useState<number | null>(null);
  useEffect(() => {
    let hidup = true;
    muatJumlahToolCli().then((n) => {
      if (hidup) setJumlahToolCli(n);
    });
    return () => {
      hidup = false;
    };
  }, []);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const focusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (isOpen) {
      const prevOverflow = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
      setQuery('');
      setSelectedIndex(0);
      if (focusTimerRef.current) clearTimeout(focusTimerRef.current);
      focusTimerRef.current = setTimeout(() => {
        focusTimerRef.current = null;
        inputRef.current?.focus();
      }, 50);
      return () => {
        document.body.style.overflow = prevOverflow;
      };
    } else if (focusTimerRef.current) {
      clearTimeout(focusTimerRef.current);
      focusTimerRef.current = null;
    }
  }, [isOpen]);

  useEffect(() => {
    return () => {
      if (focusTimerRef.current) {
        clearTimeout(focusTimerRef.current);
        focusTimerRef.current = null;
      }
    };
  }, []);

  // Daftar aksi dan navigasi terdaftar
  const items: PaletteItem[] = [
    // Navigasi
    {
      id: 'nav-dashboard',
      title: 'Ringkasan Dashboard',
      subtitle: 'Ringkasan performa sistem, volume token, dan latensi model',
      category: 'Navigasi',
      icon: LayoutDashboard,
      action: () => {
        onNavigate('/');
        onClose();
      },
    },
    {
      id: 'nav-requests',
      title: 'Permintaan',
      subtitle: 'Penelusuran log audit permintaan, payload prompt, dan failover',
      category: 'Navigasi',
      icon: Terminal,
      action: () => {
        onNavigate('/requests');
        onClose();
      },
    },
    {
      id: 'nav-cli',
      title: 'Integrasi CLI & Konfigurasi 3 Mode',
      subtitle: 'Konfigurasi Antigravity, Claude, OpenCode, dan perkakas AI terminal',
      category: 'Navigasi',
      icon: Code2,
      badge: jumlahToolCli !== null ? `${jumlahToolCli} Tools` : undefined,
      action: () => {
        onNavigate('/cli-integrations');
        onClose();
      },
    },
    {
      id: 'nav-routing',
      title: 'Aturan Perutean & Failover',
      subtitle: 'Pengaturan Model Only, Smart Routing, dan Combo Tier 1/Tier 2',
      category: 'Navigasi',
      icon: GitFork,
      action: () => {
        onNavigate('/gateway/routing');
        onClose();
      },
    },
    {
      id: 'nav-providers',
      title: 'Penyedia Upstream',
      subtitle: 'Koneksi ke OpenAI, Anthropic, Google, Ollama, dan penyedia AI',
      category: 'Navigasi',
      icon: Server,
      action: () => {
        onNavigate('/upstreams/providers');
        onClose();
      },
    },
    {
      id: 'nav-apikeys',
      title: 'Manajemen Kunci API',
      subtitle: 'Kredensial akses klien Bearer token dan rotasi kunci aman',
      category: 'Navigasi',
      icon: KeyRound,
      action: () => {
        onNavigate('/access/api-keys');
        onClose();
      },
    },
    {
      id: 'nav-observability',
      title: 'Observabilitas & Metrik',
      subtitle: 'Deret waktu latensi, throughput, dan telemetri sistem Prometheus',
      category: 'Navigasi',
      icon: Activity,
      action: () => {
        onNavigate('/observability');
        onClose();
      },
    },
    {
      id: 'nav-budgets',
      title: 'Anggaran & Batas Laju',
      subtitle: 'Alokasi plafon keuangan moneter dan pembatasan RPM/TPM',
      category: 'Navigasi',
      icon: Coins,
      action: () => {
        onNavigate('/gateway/budgets');
        onClose();
      },
    },
    {
      id: 'nav-settings',
      title: 'Pengaturan & Domain HTTPS',
      subtitle: 'Konfigurasi nama domain publik, SSL otomatis Caddy, dan setelan runtime',
      category: 'Navigasi',
      icon: Sliders,
      action: () => {
        onNavigate('/system/settings');
        onClose();
      },
    },
    {
      id: 'nav-diagnostics',
      title: 'Kesehatan Sistem (Observabilitas)',
      subtitle: 'Checklist uji live gateway dan supervisor pekerja latar',
      category: 'Navigasi',
      icon: HeartPulse,
      action: () => {
        onNavigate('/observability');
        onClose();
      },
    },

    // Aksi Cepat
    {
      id: 'act-copy-url',
      title: 'Salin Base URL API Gateway',
      subtitle: 'Salin endpoint /v1 untuk ditempel ke SDK atau konfigurasi tool',
      category: 'Aksi Cepat',
      icon: Copy,
      action: async () => {
        const gwURL = `${window.location.protocol}//${window.location.host}/v1`;
        try {
          await copyTextToClipboard(gwURL);
          toast.success(`Base URL API berhasil disalin ke clipboard: ${gwURL}`);
          onClose();
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : String(err);
          toast.error(`Gagal menyalin Base URL: ${msg}`);
        }
      },
    },
    {
      id: 'act-domain-https',
      title: 'Atur Domain Kustom & HTTPS',
      subtitle: 'Buka form pengaturan nama domain untuk Cloudflare / Let’s Encrypt',
      category: 'Aksi Cepat',
      icon: Globe,
      action: () => {
        onNavigate('/system/settings');
        onClose();
      },
    },

    // Pintasan Perkakas CLI Populer
    {
      id: 'cli-antigravity',
      title: 'Konfigurasi: Antigravity CLI (AGY)',
      subtitle: 'CLI resmi agentic Google DeepMind coding assistant',
      category: 'Perkakas CLI',
      icon: Code2,
      badge: 'CLI Agent',
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('antigravity');
        onClose();
      },
    },
    {
      id: 'cli-claude',
      title: 'Konfigurasi: Claude Code CLI',
      subtitle: 'Asisten terminal Anthropic Claude',
      category: 'Perkakas CLI',
      icon: Code2,
      badge: 'CLI Agent',
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('claude');
        onClose();
      },
    },
    {
      id: 'cli-opencode',
      title: 'Konfigurasi: OpenCode CLI',
      subtitle: 'Agen koding terminal otonom sumber terbuka',
      category: 'Perkakas CLI',
      icon: Code2,
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('opencode');
        onClose();
      },
    },
    {
      id: 'cli-cursor',
      title: 'Konfigurasi: Cursor AI IDE',
      subtitle: 'Editor kode AI dengan custom base URL Route-X',
      category: 'Perkakas CLI',
      icon: Code2,
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('cursor');
        onClose();
      },
    },
    {
      id: 'cli-continue',
      title: 'Konfigurasi: Continue.dev (VS Code)',
      subtitle: 'Ekstensi autopilot coding open-source untuk VS Code & JetBrains',
      category: 'Perkakas CLI',
      icon: Code2,
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('continue');
        onClose();
      },
    },
    {
      id: 'cli-aider',
      title: 'Konfigurasi: Aider AI Pair Programmer',
      subtitle: 'Terminal AI pair programming tool untuk git workflow',
      category: 'Perkakas CLI',
      icon: Code2,
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('aider');
        onClose();
      },
    },
  ];

  // Filter berdasarkan query pencarian
  const filteredItems = items.filter((it) => {
    if (!query.trim()) return true;
    const q = query.toLowerCase();
    return (
      it.title.toLowerCase().includes(q) ||
      (it.subtitle && it.subtitle.toLowerCase().includes(q)) ||
      it.category.toLowerCase().includes(q)
    );
  });

  // Scroll active item into view
  useEffect(() => {
    const selectedElem = listRef.current?.querySelector('[aria-selected="true"]');
    if (selectedElem) {
      selectedElem.scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIndex]);

  // Navigasi keyboard
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isOpen) return;

      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev + 1) % Math.max(1, filteredItems.length));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev - 1 + filteredItems.length) % Math.max(1, filteredItems.length));
      } else if (e.key === 'Enter') {
        e.preventDefault();
        if (filteredItems[selectedIndex]) {
          filteredItems[selectedIndex].action();
        }
      } else if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, selectedIndex, filteredItems]);

  if (!isOpen) return null;

  return (
    <FocusTrap
      active={isOpen}
      focusTrapOptions={{
        clickOutsideDeactivates: true,
        fallbackFocus: () => inputRef.current || document.body,
      }}
    >
      <div className="fixed inset-0 z-50 flex items-start justify-center pt-16 sm:pt-24 px-4">
        {/* Backdrop glassmorphism */}
        <div
          className="fixed inset-0 bg-black/70 backdrop-blur-sm transition-opacity"
          onClick={onClose}
          aria-hidden="true"
        />

        {/* Raycast/Spotlight Floating Palette Dialog */}
        <div
          role="dialog"
          aria-modal="true"
          aria-label="Palette Perintah Cepat"
          className="relative w-full max-w-2xl bg-bg-surface-1 border border-border rounded-xl shadow-2xl shadow-black/60 overflow-hidden z-10 animate-fade-in"
        >
          {/* Search Bar */}
          <div className="flex items-center gap-3 px-4 py-3.5 border-b border-border">
            <Search className="w-5 h-5 text-text-muted shrink-0" aria-hidden="true" />
            <input
              ref={inputRef}
              type="text"
              role="combobox"
              aria-expanded={isOpen}
              aria-controls="command-palette-results"
              aria-autocomplete="list"
              aria-label="Palette Perintah Cepat"
              placeholder="Cari perintah, navigasi rute, atau panduan CLI (mis. 'Antigravity', 'Cache')..."
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setSelectedIndex(0);
              }}
              className="flex-1 bg-transparent border-none text-sm text-text-primary placeholder:text-text-muted focus:outline-none font-sans"
            />
            <kbd
              className="hidden sm:inline-flex items-center px-1.5 py-0.5 text-[10px] font-mono text-text-muted bg-bg-surface-2 border border-border rounded shadow-xs"
              aria-hidden="true"
            >
              ESC
            </kbd>
          </div>

          {/* Results List */}
          <div
            id="command-palette-results"
            role="listbox"
            ref={listRef}
            className="max-h-[380px] overflow-y-auto p-2 space-y-1 scrollbar-thin"
          >
            {filteredItems.length === 0 ? (
              <div className="py-12 text-center text-xs text-text-muted space-y-2">
                <Sparkles className="w-5 h-5 mx-auto text-text-muted" aria-hidden="true" />
                <p>Tidak ada perintah atau perkakas yang cocok dengan "{query}".</p>
              </div>
            ) : (
              (() => {
                // Hasil dikelompokkan per kategori dengan judul grup kecil, mengikuti sampel 07.
                const order: string[] = [];
                const byCategory = new Map<string, { item: (typeof filteredItems)[number]; idx: number }[]>();
                filteredItems.forEach((item, idx) => {
                  if (!byCategory.has(item.category)) {
                    byCategory.set(item.category, []);
                    order.push(item.category);
                  }
                  byCategory.get(item.category)!.push({ item, idx });
                });
                return order.map((category) => (
                  <div key={category} role="group" aria-label={category}>
                    <p className="px-3 pt-2.5 pb-1 text-[10px] font-medium uppercase tracking-wider text-text-muted">
                      {category}
                    </p>
                    {byCategory.get(category)!.map(({ item, idx }) => {
                      const Icon = item.icon;
                      const isSelected = idx === selectedIndex;

                      return (
                  <button
                    key={item.id}
                    type="button"
                    role="option"
                    aria-selected={isSelected}
                    onClick={item.action}
                    onMouseEnter={() => setSelectedIndex(idx)}
                    className={`w-full text-left px-3 py-2 rounded-nav flex items-center justify-between transition-colors cursor-pointer ${
                      isSelected
                        ? 'bg-bg-surface-2 text-text-primary font-medium border border-transparent'
                        : 'text-text-secondary hover:bg-bg-surface-2/60 hover:text-text-primary border border-transparent'
                    }`}
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div
                        className={`w-7 h-7 rounded-nav flex items-center justify-center shrink-0 transition-colors ${
                          isSelected
                            ? 'bg-bg-surface-3 text-text-primary'
                            : 'bg-bg-surface-2 text-text-secondary'
                        }`}
                      >
                        <Icon className="w-3.5 h-3.5" aria-hidden="true" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-xs font-medium text-text-primary truncate">
                            {item.title}
                          </span>
                          {item.badge && (
                            <span className="text-[10px] px-1.5 py-0.2 bg-bg-surface-2 text-text-secondary border border-border rounded-full font-mono">
                              {item.badge}
                            </span>
                          )}
                        </div>
                        {item.subtitle && (
                          <p className="text-[11px] text-text-muted truncate mt-0.5">
                            {item.subtitle}
                          </p>
                        )}
                      </div>
                    </div>

                    <div className="flex items-center gap-2 shrink-0 ml-3">
                      <span className="text-[10px] text-text-muted uppercase font-mono px-2 py-0.5 rounded-md bg-bg-surface-2/80 border border-border/50 hidden sm:inline-block">
                        {item.category}
                      </span>
                      {isSelected ? (
                        <CornerDownLeft className="w-3.5 h-3.5 text-text-secondary" aria-hidden="true" />
                      ) : (
                        <ArrowRight className="w-3.5 h-3.5 text-text-muted/40 hidden group-hover:inline-block" aria-hidden="true" />
                      )}
                    </div>
                      </button>
                      );
                    })}
                  </div>
                ));
              })()
            )}
          </div>

          {/* Footer toolbar */}
          <div className="px-4 py-2.5 border-t border-border flex items-center justify-between text-[11px] text-text-muted font-mono">
            <div className="flex items-center gap-3">
              <span className="flex items-center gap-1">
                <kbd className="px-1 py-0.5 bg-bg-surface-1 border border-border/80 rounded text-[10px]" aria-hidden="true">↑</kbd>
                <kbd className="px-1 py-0.5 bg-bg-surface-1 border border-border/80 rounded text-[10px]" aria-hidden="true">↓</kbd>
                <span>Pilih</span>
              </span>
              <span className="text-border" aria-hidden="true">•</span>
              <span className="flex items-center gap-1">
                <kbd className="px-1.5 py-0.5 bg-bg-surface-1 border border-border/80 rounded text-[10px]" aria-hidden="true">↵</kbd>
                <span>Jalankan</span>
              </span>
              <span className="text-border" aria-hidden="true">•</span>
              <span className="flex items-center gap-1">
                <kbd className="px-1.5 py-0.5 bg-bg-surface-1 border border-border/80 rounded text-[10px]" aria-hidden="true">ESC</kbd>
                <span>Tutup</span>
              </span>
            </div>
            <span className="font-mono text-text-secondary font-medium text-[10px] tracking-wide">
              Route-X Spotlight
            </span>
          </div>
        </div>
      </div>
    </FocusTrap>
  );
};
