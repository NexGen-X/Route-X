import React, { useState, useEffect, useRef } from 'react';
import {
  Search,
  LayoutDashboard,
  Terminal,
  Activity,
  Server,
  Cpu,
  GitFork,
  Code2,
  KeyRound,
  Coins,
  Sliders,
  HeartPulse,
  Copy,
  Trash2,
  Globe,
  ArrowRight,
} from 'lucide-react';
import { api } from '../../api/client';
import { useToast } from '../../context/ToastContext';

interface CommandPaletteProps {
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
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  // Daftar aksi dan navigasi terdaftar
  const items: PaletteItem[] = [
    // Navigasi
    {
      id: 'nav-dashboard',
      title: 'Dashboard Overview',
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
      title: 'Requests Inspector',
      subtitle: 'Penelusuran audit log request, payload prompt, dan failover',
      category: 'Navigasi',
      icon: Terminal,
      action: () => {
        onNavigate('/requests');
        onClose();
      },
    },
    {
      id: 'nav-cli',
      title: 'CLI Integrations & 3-Mode Config',
      subtitle: 'Konfigurasi Antigravity, Claude, OpenCode, dan perkakas AI terminal',
      category: 'Navigasi',
      icon: Code2,
      badge: '20 Tools',
      action: () => {
        onNavigate('/cli-integrations');
        onClose();
      },
    },
    {
      id: 'nav-routing',
      title: 'Routing & Failover Rules',
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
      title: 'Upstream Providers',
      subtitle: 'Koneksi ke OpenAI, Anthropic, Google, Ollama, dan penyedia AI',
      category: 'Navigasi',
      icon: Server,
      action: () => {
        onNavigate('/upstreams/providers');
        onClose();
      },
    },
    {
      id: 'nav-models',
      title: 'Models & Pricing Catalog',
      subtitle: 'Katalog model kanonik dan struktur penetapan harga moneter USD',
      category: 'Navigasi',
      icon: Cpu,
      action: () => {
        onNavigate('/upstreams/models');
        onClose();
      },
    },
    {
      id: 'nav-apikeys',
      title: 'API Keys Management',
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
      title: 'Observability & Metrics',
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
      title: 'Budgets & Rate Limits',
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
      title: 'Settings & Domain HTTPS',
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
      title: 'System Diagnostics & Health',
      subtitle: 'Metrik memori Go, koneksi PostgreSQL pool, dan Redis cache stats',
      category: 'Navigasi',
      icon: HeartPulse,
      action: () => {
        onNavigate('/system/diagnostics');
        onClose();
      },
    },

    // Aksi Cepat
    {
      id: 'act-copy-url',
      title: 'Salin Base URL API Gateway',
      subtitle: 'Salin endpoint /v1 untuk di-paste ke SDK atau konfigurasi tool',
      category: 'Aksi Cepat',
      icon: Copy,
      action: () => {
        const gwURL = `${window.location.protocol}//${window.location.host}/v1`;
        navigator.clipboard.writeText(gwURL);
        toast.success(`Base URL API berhasil disalin ke clipboard: ${gwURL}`);
        onClose();
      },
    },
    {
      id: 'act-flush-cache',
      title: 'Bersihkan Cache Respons Redis',
      subtitle: 'Hapus seluruh entri cache jawaban inferensi dari Redis',
      category: 'Aksi Cepat',
      icon: Trash2,
      action: async () => {
        try {
          const res = await api.system.flushCache();
          toast.success(`Cache berhasil dibersihkan: ${res.message}`);
        } catch (err) {
          toast.error('Gagal membersihkan cache: ' + (err instanceof Error ? err.message : String(err)));
        }
        onClose();
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
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('antigravity');
        onClose();
      },
    },
    {
      id: 'cli-claude',
      title: 'Konfigurasi: Claude Code CLI',
      subtitle: 'Anthropic Claude terminal assistant',
      category: 'Perkakas CLI',
      icon: Code2,
      action: () => {
        onNavigate('/cli-integrations');
        onSelectCLI?.('claude');
        onClose();
      },
    },
    {
      id: 'cli-opencode',
      title: 'Konfigurasi: OpenCode CLI',
      subtitle: 'Open-source autonomous terminal coding agent',
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
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-20 px-4">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity"
        onClick={onClose}
      />

      {/* Palette Modal */}
      <div className="relative w-full max-w-xl bg-bg-surface-1 border border-[#262626] rounded-2xl shadow-2xl shadow-black/80 overflow-hidden z-10 animate-in fade-in zoom-in-95 duration-150">
        {/* Search Input Bar */}
        <div className="flex items-center gap-3 px-4 py-3.5 border-b border-border bg-bg-surface-2/60">
          <Search className="w-5 h-5 text-accent shrink-0" />
          <input
            ref={inputRef}
            type="text"
            placeholder="Ketik perintah, nama halaman, atau perkakas CLI (mis. 'Antigravity', 'Cache')..."
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIndex(0);
            }}
            className="flex-1 bg-transparent border-none text-sm text-white placeholder:text-text-muted focus:outline-none font-sans"
          />
          <kbd className="hidden sm:inline-block px-1.5 py-0.5 text-[10px] font-mono text-text-muted bg-bg-surface-1 border border-border rounded">
            ESC
          </kbd>
        </div>

        {/* Results List */}
        <div ref={listRef} className="max-h-80 overflow-y-auto p-2 space-y-1">
          {filteredItems.length === 0 ? (
            <div className="py-8 text-center text-xs text-text-muted">
              Tidak ada perintah atau perkakas yang cocok dengan "{query}".
            </div>
          ) : (
            filteredItems.map((item, idx) => {
              const Icon = item.icon;
              const isSelected = idx === selectedIndex;
              return (
                <button
                  key={item.id}
                  onClick={item.action}
                  onMouseEnter={() => setSelectedIndex(idx)}
                  className={`w-full text-left px-3.5 py-2.5 rounded-xl flex items-center justify-between transition-colors ${
                    isSelected
                      ? 'bg-accent/15 text-white border border-accent/30'
                      : 'text-text-secondary hover:bg-bg-surface-2 hover:text-white border border-transparent'
                  }`}
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <div
                      className={`w-8 h-8 rounded-lg flex items-center justify-center shrink-0 ${
                        isSelected ? 'bg-accent text-black font-bold' : 'bg-bg-surface-2 text-accent'
                      }`}
                    >
                      <Icon className="w-4 h-4" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-xs font-semibold text-white truncate">{item.title}</span>
                        {item.badge && (
                          <span className="text-[10px] px-1.5 py-0.2 bg-accent/20 text-accent rounded font-mono font-semibold">
                            {item.badge}
                          </span>
                        )}
                      </div>
                      {item.subtitle && (
                        <p className="text-[11px] text-text-muted truncate mt-0.5">{item.subtitle}</p>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0 ml-2">
                    <span className="text-[10px] text-text-muted uppercase font-mono px-1.5 py-0.5 rounded bg-bg-surface-2 hidden sm:inline-block">
                      {item.category}
                    </span>
                    {isSelected && <ArrowRight className="w-3.5 h-3.5 text-accent" />}
                  </div>
                </button>
              );
            })
          )}
        </div>

        {/* Footer info */}
        <div className="px-4 py-2.5 bg-bg-surface-2/80 border-t border-border flex items-center justify-between text-[11px] text-text-muted">
          <div className="flex items-center gap-3">
            <span>
              Gunakan <kbd className="px-1 py-0.5 bg-bg-surface-1 border border-border rounded text-[10px]">↑</kbd>{' '}
              <kbd className="px-1 py-0.5 bg-bg-surface-1 border border-border rounded text-[10px]">↓</kbd> untuk memilih
            </span>
            <span>
              <kbd className="px-1 py-0.5 bg-bg-surface-1 border border-border rounded text-[10px]">↵</kbd> untuk mengeksekusi
            </span>
          </div>
          <span className="font-mono text-accent font-semibold">Route-X FastNav</span>
        </div>
      </div>
    </div>
  );
};
