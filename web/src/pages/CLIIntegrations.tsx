import React, { useState, useEffect, useRef, useMemo } from 'react';
import {
  Terminal,
  CheckCircle2,
  XCircle,
  Copy,
  Check,
  RefreshCw,
  FileCode,
  ExternalLink,
  Search,
  Filter,
  Zap,
  Key,
  Eye,
  EyeOff,
  Laptop,
  Server,
} from 'lucide-react';
import { api } from '../api/client';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Badge } from '../components/common/Badge';
import { Modal } from '../components/common/Modal';
import { Select } from '../components/common/Select';
import type { CLITool, CLIDetectedResponse, Model, RoutingRule } from '../types';
import { useToast } from '../context/ToastContext';
import { copyTextToClipboard } from '../utils/clipboard';
import { QueryError } from '../components/common/QueryError';

export const CLIIntegrations: React.FC = () => {
  const { toast } = useToast();
  const [data, setData] = useState<CLIDetectedResponse | null>(null);
  const [models, setModels] = useState<Model[]>([]);
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [onlyInstalled, setOnlyInstalled] = useState(false);

  // Kunci API global pengguna untuk menghasilkan skrip terminal siap pakai
  const [userApiKey, setUserApiKey] = useState<string>(() => {
    try {
      return localStorage.getItem('routex_cli_apikey') || '';
    } catch {
      return '';
    }
  });
  const [showGlobalKey, setShowGlobalKey] = useState(false);

  // Status form lokal per tool { [toolId]: { mode, target, apiKey } }
  const [toolConfigs, setToolConfigs] = useState<
    Record<string, { mode: 'model_only' | 'routing' | 'combo'; target: string; apiKey: string }>
  >({});
  const [showKeyMap, setShowKeyMap] = useState<Record<string, boolean>>({});
  const [applyingTool, setApplyingTool] = useState<string | null>(null);
  const [applySuccess, setApplySuccess] = useState<Record<string, string>>({});
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  // Timer indikator salin/sukses; dibatalkan saat unmount agar tidak ada setState basi.
  const feedbackTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  // Batalkan timer umpan balik yang tersisa saat unmount.
  useEffect(() => {
    return () => {
      feedbackTimersRef.current.forEach((t) => clearTimeout(t));
      feedbackTimersRef.current = [];
    };
  }, []);

  const [pingStatus, setPingStatus] = useState<
    Record<string, { testing: boolean; latency?: number; ok?: boolean }>
  >({});

  // Cek apakah user membuka dashboard dari mesin lokal (localhost) atau remote cloud
  const isLocalGateway = useMemo(() => {
    if (typeof window === 'undefined') return false;
    const h = window.location.hostname.toLowerCase();
    return h === 'localhost' || h === '127.0.0.1' || h === '::1';
  }, []);

  // Temukan aturan combo dinamis dari database (aturan dengan tag [combo:...] pada deskripsinya)
  const dynamicComboOptions = useMemo(() => {
    const comboRules = rules.filter((r) => (r.description || '').includes('[combo:'));
    if (comboRules.length === 0) {
      return [];
    }
    return comboRules.map((r) => {
      const m = (r.description || '').match(/\[combo:alias=([^\]]+)\]/);
      const alias = m ? m[1].trim() : r.name;
      const cleanDesc = (r.description || '')
        .replace(/\[combo:[^\]]+\]/g, '')
        .replace(/\[routing\]/g, '')
        .trim();
      return {
        value: alias,
        label: `${r.name} (${alias})`,
        description: cleanDesc || 'Smart Tiered Cascade Rule',
      };
    });
  }, [rules]);

  const handlePing = async (toolId: string) => {
    setPingStatus((prev) => ({ ...prev, [toolId]: { testing: true } }));
    const start = performance.now();
    try {
      // gateway_url adalah basis /v1 gateway; healthz menandai ujung hidupnya.
      const base = (data?.gateway_url || '').replace(/\/v1\/?$/, '');
      const res = await fetch(`${base}/healthz`);
      const latency = Math.round(performance.now() - start);
      setPingStatus((prev) => ({
        ...prev,
        [toolId]: { testing: false, latency, ok: res.ok },
      }));
    } catch {
      setPingStatus((prev) => ({
        ...prev,
        [toolId]: { testing: false, latency: 0, ok: false },
      }));
    }
  };

  // Modal skrip ekspor global
  const [isExportModalOpen, setIsExportModalOpen] = useState(false);
  const [exportScriptContent, setExportScriptContent] = useState('');
  const [exportScriptLoading, setExportScriptLoading] = useState(false);
  const [includeKeyInModal, setIncludeKeyInModal] = useState(true);

  const loadData = async () => {
    try {
      setRefreshing(true);
      const [cliRes, modelsRes, rulesRes] = await Promise.all([
        api.cli.detected(),
        api.models.list(),
        api.routing.list(),
      ]);

      setData(cliRes);
      setModels(modelsRes.items || []);
      setRules(rulesRes.items || []);

      // Inisialisasi formulir lokal untuk tiap tool
      const initialConfigs: Record<
        string,
        { mode: 'model_only' | 'routing' | 'combo'; target: string; apiKey: string }
      > = {};
      cliRes.tools.forEach((t: CLITool) => {
        const mode = t.active_mode || 'model_only';
        const fallback =
          mode === 'routing'
            ? rulesRes.items[0]?.name || ''
            : mode === 'combo'
            ? dynamicComboOptions[0]?.value || ''
            : modelsRes.items[0]?.model_id || '';
        initialConfigs[t.id] = {
          mode,
          target: t.active_target || fallback,
          apiKey: '',
        };
      });
      setToolConfigs(initialConfigs);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleModeChange = (toolId: string, mode: 'model_only' | 'routing' | 'combo') => {
    setToolConfigs((prev) => {
      const current = prev[toolId];
      let newTarget = current?.target || '';

      if (mode === 'model_only') {
        newTarget = models[0]?.model_id || '';
      } else if (mode === 'routing') {
        newTarget = rules[0]?.name || '';
      } else if (mode === 'combo') {
        newTarget = dynamicComboOptions[0]?.value || '';
      }

      return {
        ...prev,
        [toolId]: {
          ...current,
          mode,
          target: newTarget,
        },
      };
    });
  };

  const handleTargetChange = (toolId: string, target: string) => {
    setToolConfigs((prev) => ({
      ...prev,
      [toolId]: {
        ...prev[toolId],
        target,
      },
    }));
  };

  const handleApiKeyChange = (toolId: string, apiKey: string) => {
    setToolConfigs((prev) => ({
      ...prev,
      [toolId]: {
        ...prev[toolId],
        apiKey,
      },
    }));
  };

  const toggleShowKey = (toolId: string) => {
    setShowKeyMap((prev) => ({ ...prev, [toolId]: !prev[toolId] }));
  };

  const handleApply = async (toolId: string) => {
    const config = toolConfigs[toolId];
    if (!config) return;

    try {
      setApplyingTool(toolId);
      const effectiveKey = config.apiKey || userApiKey || undefined;
      const res = await api.cli.configure({
        tool_id: toolId,
        mode: config.mode,
        target: config.target,
        api_key: effectiveKey,
      });

      if (res.success) {
        setApplySuccess((prev) => ({
          ...prev,
          [toolId]: isLocalGateway
            ? 'Konfigurasi berhasil diterapkan ke host lokal!'
            : 'Preferensi berhasil disimpan di gateway!',
        }));
        feedbackTimersRef.current.push(
          setTimeout(() => {
            setApplySuccess((prev) => {
              const next = { ...prev };
              delete next[toolId];
              return next;
            });
          }, 4000)
        );

        // Perbarui data lokal
        setData((prev) => {
          if (!prev) return prev;
          return {
            ...prev,
            tools: prev.tools.map((t) => (t.id === toolId ? res.tool : t)),
          };
        });
      }
    } catch (err: any) {
      toast.error(`Gagal menerapkan konfigurasi: ${err.message || 'Error tidak diketahui'}`, 'Gagal Konfigurasi');
    } finally {
      setApplyingTool(null);
    }
  };

  // Susun snippet 3 variabel lengkap (Base URL, Target Model & Kunci API)
  const getToolCompleteSnippet = (tool: CLITool) => {
    const cfg = toolConfigs[tool.id];
    const target = cfg?.target || tool.active_target || models[0]?.model_id || '';
    const gwURL = data?.gateway_url || 'http://localhost:8080/v1';
    const key = cfg?.apiKey || userApiKey || '<KUNCI_API_ROUTEX_ANDA>';

    const lines: string[] = [];

    // 1. Base URL
    for (const [k, v] of Object.entries(tool.env_vars || {})) {
      if (k.includes('BASE') || k.includes('HOST') || k.includes('ENDPOINT') || k.includes('PATH')) {
        lines.push(`export ${k}='${v || gwURL}'`);
      }
    }

    // 2. Target Model / Rule
    for (const [k] of Object.entries(tool.env_vars || {})) {
      if (k.includes('MODEL') || k.includes('DEFAULT')) {
        lines.push(`export ${k}='${target}'`);
      }
    }

    // 3. API Key
    const keyVar = tool.env_var_api_key || 'OPENAI_API_KEY';
    lines.push(`export ${keyVar}='${key}'`);

    return lines.join('\n');
  };

  const handleCopy = async (text: string, key: string) => {
    if (!text) {
      toast.error('Tidak ada teks untuk disalin.');
      return;
    }
    try {
      await copyTextToClipboard(text);
      setCopiedKey(key);
      toast.success('Variabel lingkungan disalin ke clipboard.');
      feedbackTimersRef.current.push(setTimeout(() => setCopiedKey(null), 2000));
    } catch (err) {
      toast.error('Gagal menyalin: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleOpenExportModal = async () => {
    setIsExportModalOpen(true);
    setExportScriptLoading(true);
    try {
      const res = await api.cli.exportScript();
      setExportScriptContent(res.content);
    } catch (err: any) {
      setExportScriptContent(`# Gagal memuat berkas: ${err.message}`);
    } finally {
      setExportScriptLoading(false);
    }
  };

  const modalRenderedScript = useMemo(() => {
    if (!exportScriptContent) return '';
    if (!includeKeyInModal) return exportScriptContent;

    const keyToUse = userApiKey || '<KUNCI_API_ROUTEX_ANDA>';
    const header = [
      '# Kunci API Route-X untuk autentikasi CLI (Tambahan Klien)',
      `export OPENAI_API_KEY='${keyToUse}'`,
      `export ANTHROPIC_API_KEY='${keyToUse}'`,
      '',
    ].join('\n');

    return header + exportScriptContent;
  }, [exportScriptContent, includeKeyInModal, userApiKey]);

  const categories = ['all', 'Coding Agent', 'Terminal Assistant', 'Local LLM', 'Git Automation', 'Workflow & Prompts', 'DevOps & SRE'];

  const filteredTools = (data?.tools || []).filter((tool) => {
    const q = searchQuery.toLowerCase();
    const matchesSearch =
      (tool.name || '').toLowerCase().includes(q) ||
      (tool.id || '').toLowerCase().includes(q) ||
      (tool.description || '').toLowerCase().includes(q);

    const matchesCategory =
      selectedCategory === 'all' || (tool.category || '').toLowerCase() === selectedCategory.toLowerCase();

    const matchesInstalled = !onlyInstalled || tool.installed;

    return matchesSearch && matchesCategory && matchesInstalled;
  });

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-text-muted">
        <RefreshCw className="w-8 h-8 animate-spin text-accent mb-4" />
        <p className="text-sm font-mono">Memindai perkakas AI CLI di sistem host...</p>
      </div>
    );
  }

  if (loadError && !data) {
    return <QueryError message={loadError} onRetry={() => void loadData()} />;
  }

  return (
    <div className="space-y-6">
      {/* Top Banner & Global Stats */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card className="p-5 flex flex-col justify-between border-l-4 border-l-accent">
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium text-text-muted flex items-center gap-1.5">
                {isLocalGateway ? (
                  <Laptop className="w-3.5 h-3.5 text-accent" />
                ) : (
                  <Server className="w-3.5 h-3.5 text-blue-400" />
                )}
                Lingkungan Gateway
              </span>
              <Badge variant={isLocalGateway ? 'lime' : 'info'}>
                {isLocalGateway ? 'Lokal (Host)' : 'Remote (Cloud)'}
              </Badge>
            </div>
            <h3 className="text-xl font-bold text-white mb-1">
              {data?.total_detected} / {data?.total_available} CLI Terdeteksi
            </h3>
            <p className="text-xs text-text-muted">
              {isLocalGateway
                ? 'Biner perkakas terdeteksi otomatis di sistem lokal ini. Klik "Terapkan ke CLI" untuk menyinkronkan langsung ke file konfigurasi.'
                : 'Route-X berjalan di server remote. Gunakan tombol "Salin Snippet" pada masing-masing tool untuk mengekspor variabel ke terminal laptop Anda.'}
            </p>
          </div>
          <div className="mt-4 pt-4 border-t border-[#262626] flex items-center justify-between">
            <span className="text-xs font-mono text-text-muted">Base URL:</span>
            <code className="text-xs font-mono text-accent bg-accent/10 px-2 py-0.5 rounded">
              {data?.gateway_url || 'http://localhost:8080/v1'}
            </code>
          </div>
        </Card>

        <Card className="p-5 flex flex-col justify-between lg:col-span-2">
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium text-text-muted flex items-center gap-1.5">
                <FileCode className="w-3.5 h-3.5 text-accent" />
                Skrip Lingkungan Terpadu
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleOpenExportModal}
                className="text-xs h-7 gap-1"
              >
                <ExternalLink className="w-3.5 h-3.5" />
                Lihat Skrip Lengkap
              </Button>
            </div>
            <p className="text-xs text-text-secondary mb-3">
              Muat semua variabel lingkungan CLI yang dikonfigurasi sekaligus di terminal dengan satu perintah:
            </p>
            <div className="flex items-center gap-2 bg-[#121212] border border-[#262626] p-2.5 rounded-lg">
              <code className="text-xs font-mono text-accent flex-1 select-all">
                source ~/.routex/cli-env.sh
              </code>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs gap-1"
                onClick={() => handleCopy('source ~/.routex/cli-env.sh', 'global-source')}
              >
                {copiedKey === 'global-source' ? (
                  <Check className="w-3.5 h-3.5 text-accent" />
                ) : (
                  <Copy className="w-3.5 h-3.5 text-text-muted" />
                )}
                {copiedKey === 'global-source' ? 'Tersalin' : 'Salin'}
              </Button>
            </div>
          </div>
          <div className="mt-3 flex items-center justify-between text-[11px] text-text-muted">
            <span>Tambahkan ke <code>~/.bashrc</code> atau <code>~/.zshrc</code> untuk memuat otomatis setiap sesi shell.</span>
            <Button
              variant="secondary"
              size="sm"
              onClick={loadData}
              disabled={refreshing}
              className="gap-1.5 h-7 text-xs"
            >
              <RefreshCw className={`w-3 h-3 ${refreshing ? 'animate-spin' : ''}`} />
              Pindai Ulang
            </Button>
          </div>
        </Card>
      </div>

      {/* Global API Key Helper Toolbar */}
      <div className="bg-[#161616] p-4 rounded-lg border border-[#262626] flex flex-col md:flex-row items-start md:items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-accent/10 text-accent flex-shrink-0">
            <Key className="w-4 h-4" />
          </div>
          <div>
            <h4 className="text-xs font-bold text-white flex items-center gap-2">
              Kunci API Route-X untuk Terminal CLI
              <Badge variant={userApiKey ? 'lime' : 'neutral'}>
                {userApiKey ? 'Tersedia' : 'Opsional'}
              </Badge>
            </h4>
            <p className="text-[11px] text-text-muted">
              Masukkan API Key Anda di sini agar tombol salin otomatis menghasilkan export lengkap tanpa error 401. Kunci hanya tersimpan di peramban lokal Anda.
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 w-full md:w-auto">
          <div className="relative flex-1 md:w-72">
            <input
              type={showGlobalKey ? 'text' : 'password'}
              placeholder="sk_live_..."
              value={userApiKey}
              onChange={(e) => {
                const val = e.target.value;
                setUserApiKey(val);
                try {
                  localStorage.setItem('routex_cli_apikey', val);
                } catch {}
              }}
              className="w-full bg-[#121212] border border-[#333] rounded-md px-3 py-1.5 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none font-mono pr-8"
            />
            <button
              type="button"
              onClick={() => setShowGlobalKey(!showGlobalKey)}
              className="absolute right-2 top-2 text-text-muted hover:text-white"
              title={showGlobalKey ? 'Sembunyikan' : 'Tampilkan'}
            >
              {showGlobalKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
            </button>
          </div>
          {userApiKey && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setUserApiKey('');
                try {
                  localStorage.removeItem('routex_cli_apikey');
                } catch {}
              }}
              className="text-xs min-h-[36px] text-text-muted hover:text-red-400"
              title="Kosongkan kunci tersimpan"
            >
              Reset
            </Button>
          )}
        </div>
      </div>

      {/* Filter & Search Bar */}
      <div className="flex flex-col sm:flex-row gap-3 items-center justify-between bg-[#141414] p-3 rounded-lg border border-[#262626]">
        <div className="relative w-full sm:w-80">
          <Search className="w-4 h-4 absolute left-3 top-2.5 text-text-muted" />
          <input
            type="text"
            placeholder="Cari perkakas CLI (misal: agy, claude, aider)..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full bg-[#1C1C1C] border border-[#333] rounded-md pl-9 pr-3 py-1.5 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none"
          />
        </div>

        <div className="flex items-center gap-2 w-full sm:w-auto overflow-x-auto">
          <Filter className="w-3.5 h-3.5 text-text-muted" />
          <div className="flex gap-1.5">
            {categories.map((cat) => (
              <button
                key={cat}
                onClick={() => setSelectedCategory(cat)}
                className={`px-2.5 py-1 rounded text-xs transition-colors whitespace-nowrap ${
                  selectedCategory === cat
                    ? 'bg-accent text-black font-semibold'
                    : 'bg-[#1C1C1C] text-text-muted hover:text-white border border-[#262626]'
                }`}
              >
                {cat === 'all' ? 'Semua Kategori' : cat}
              </button>
            ))}
          </div>

          <label className="flex items-center gap-1.5 text-xs text-text-secondary ml-2 cursor-pointer whitespace-nowrap">
            <input
              type="checkbox"
              checked={onlyInstalled}
              onChange={(e) => setOnlyInstalled(e.target.checked)}
              className="rounded border-[#333] bg-[#1C1C1C] text-accent focus:ring-0 cursor-pointer"
            />
            Terinstall Saja
          </label>
        </div>
      </div>

      {/* CLI Tools Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {filteredTools.map((tool) => {
          const cfg = toolConfigs[tool.id] || {
            mode: tool.active_mode || 'model_only',
            target: tool.active_target || models[0]?.model_id || '',
            apiKey: '',
          };
          const isApplying = applyingTool === tool.id;
          const successMsg = applySuccess[tool.id];
          const completeSnippet = getToolCompleteSnippet(tool);

          return (
            <Card
              key={tool.id}
              className={`p-5 flex flex-col justify-between transition-all duration-200 ${
                tool.installed
                  ? 'border-[#333] hover:border-accent/40 bg-[#161616]'
                  : 'border-[#222] opacity-80 bg-[#121212]'
              }`}
            >
              <div>
                {/* Header Kartu */}
                <div className="flex items-start justify-between gap-2 mb-2">
                  <div className="flex items-center gap-2.5">
                    <div
                      className={`p-2 rounded-lg ${
                        tool.installed ? 'bg-accent/10 text-accent' : 'bg-[#222] text-text-muted'
                      }`}
                    >
                      <Terminal className="w-5 h-5" />
                    </div>
                    <div>
                      <h4 className="text-base font-bold text-white flex items-center gap-2">
                        {tool.name}
                      </h4>
                      <span className="text-[11px] font-mono text-text-muted">{tool.category}</span>
                    </div>
                  </div>

                  {tool.installed ? (
                    <Badge variant="lime" className="gap-1">
                      <CheckCircle2 className="w-3 h-3" />
                      Terdeteksi
                    </Badge>
                  ) : (
                    <Badge variant="neutral" className="gap-1">
                      <XCircle className="w-3 h-3" />
                      Belum Ada
                    </Badge>
                  )}
                </div>

                {/* Deskripsi & Lokasi Biner */}
                <p className="text-xs text-text-secondary line-clamp-2 mb-3 min-h-[32px]">
                  {tool.description}
                </p>

                {tool.installed && (
                  <div className="mb-4 bg-[#1C1C1C] p-2 rounded border border-[#262626] text-[11px] font-mono text-text-muted space-y-0.5">
                    <div className="truncate text-white">
                      📍 <span className="text-text-muted">Path:</span> {tool.path}
                    </div>
                    {tool.version && (
                      <div className="truncate text-accent">
                        ⚡ <span className="text-text-muted">Versi:</span> {tool.version}
                      </div>
                    )}
                  </div>
                )}

                {/* Kolom Konfigurasi 3-Mode (Model Only, Routing, Combo Routing) */}
                <div className="space-y-3 pt-3 border-t border-[#262626]">
                  <div>
                    <label className="text-xs font-medium text-text-secondary block mb-1.5">
                      Mode Konfigurasi:
                    </label>
                    <div className="grid grid-cols-3 gap-1 p-1 bg-[#121212] rounded-lg border border-[#262626]">
                      <button
                        type="button"
                        onClick={() => handleModeChange(tool.id, 'model_only')}
                        className={`text-[11px] py-1 px-1.5 rounded font-medium transition-all ${
                          cfg.mode === 'model_only'
                            ? 'bg-accent text-black font-bold shadow'
                            : 'text-text-muted hover:text-white'
                        }`}
                        title="Direct passthrough ke satu model tanpa failover"
                      >
                        1. Model
                      </button>

                      <button
                        type="button"
                        onClick={() => handleModeChange(tool.id, 'routing')}
                        className={`text-[11px] py-1 px-1.5 rounded font-medium transition-all ${
                          cfg.mode === 'routing'
                            ? 'bg-accent text-black font-bold shadow'
                            : 'text-text-muted hover:text-white'
                        }`}
                        title="Routing failover dinamis ke aturan rute"
                      >
                        2. Routing
                      </button>

                      <button
                        type="button"
                        onClick={() => handleModeChange(tool.id, 'combo')}
                        className={`text-[11px] py-1 px-1.5 rounded font-medium transition-all ${
                          cfg.mode === 'combo'
                            ? 'bg-accent text-black font-bold shadow'
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
                      {cfg.mode === 'model_only' && 'Target Model:'}
                      {cfg.mode === 'routing' && 'Target Rule Routing:'}
                      {cfg.mode === 'combo' && 'Smart Combo Cascade:'}
                    </label>

                    {cfg.mode === 'model_only' && (
                      models.length > 0 ? (
                        <Select
                          value={cfg.target}
                          onChange={(val) => handleTargetChange(tool.id, val)}
                          options={models.map((m) => ({
                            value: m.model_id,
                            label: `${m.display_name || m.model_id} (${m.family || 'universal'})`,
                          }))}
                        />
                      ) : (
                        <div className="p-2.5 rounded-inner bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                          Belum ada model kanonik terdaftar di database. Silakan daftarkan model di menu{' '}
                          <a href="#/models" className="underline font-semibold text-white">Models</a>.
                        </div>
                      )
                    )}

                    {cfg.mode === 'routing' && (
                      rules.length > 0 ? (
                        <Select
                          value={cfg.target}
                          onChange={(val) => handleTargetChange(tool.id, val)}
                          options={rules.map((r) => ({
                            value: r.name,
                            label: `${r.name} (${r.strategy})`,
                          }))}
                        />
                      ) : (
                        <div className="p-2.5 rounded-inner bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                          Belum ada aturan routing terdaftar di database. Buat aturan di menu{' '}
                          <a href="#/routing-rules" className="underline font-semibold text-white">Routing Rules</a>.
                        </div>
                      )
                    )}

                    {cfg.mode === 'combo' && (
                      dynamicComboOptions.length > 0 ? (
                        <div className="space-y-2">
                          <Select
                            value={cfg.target}
                            onChange={(val) => handleTargetChange(tool.id, val)}
                            options={dynamicComboOptions}
                          />

                          {/* Interactive Combo Routing Flow Diagram */}
                          <div className="p-2 rounded-inner bg-[#121212] border border-border/60 text-[10px] font-mono space-y-1">
                            <div className="text-text-muted flex items-center justify-between">
                              <span>Alur Eksekusi Cascade:</span>
                              <span className="text-accent font-semibold">Auto-Failover</span>
                            </div>
                            <div className="flex items-center gap-1.5 overflow-x-auto py-0.5">
                              <span className="px-1.5 py-0.5 rounded bg-bg-surface-2 border border-border text-white whitespace-nowrap">
                                {tool.name}
                              </span>
                              <span className="text-text-muted">➔</span>
                              <span className="px-1.5 py-0.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 whitespace-nowrap">
                                Tier 1 (Cepat/Lokal)
                              </span>
                              <span className="text-text-muted">➔</span>
                              <span className="px-1.5 py-0.5 rounded bg-purple-500/10 border border-purple-500/30 text-purple-400 whitespace-nowrap">
                                Tier 2 (Flagship)
                              </span>
                            </div>
                          </div>
                        </div>
                      ) : (
                        <div className="p-2.5 rounded-inner bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                          Belum ada aturan combo bertingkat. Buat aturan bertingkat di menu{' '}
                          <a href="#/routing-rules" className="underline font-semibold text-white">Routing Rules</a>{' '}
                          dengan tag deskripsi [combo:alias=nama].
                        </div>
                      )
                    )}
                  </div>

                  {/* Kunci API Khusus Tool (Opsional) */}
                  <div>
                    <div className="flex items-center justify-between mb-1">
                      <label className="text-[11px] font-medium text-text-muted flex items-center gap-1">
                        <Key className="w-3 h-3 text-accent" />
                        Kunci API Khusus Tool:
                      </label>
                      {cfg.apiKey ? (
                        <span className="text-[10px] text-accent font-mono font-semibold">Kustom</span>
                      ) : userApiKey ? (
                        <span className="text-[10px] text-emerald-400 font-mono">Memakai Global</span>
                      ) : (
                        <span className="text-[10px] text-text-muted font-mono">Placeholder</span>
                      )}
                    </div>
                    <div className="relative">
                      <input
                        type={showKeyMap[tool.id] ? 'text' : 'password'}
                        placeholder={
                          userApiKey
                            ? 'Memakai kunci global'
                            : `Isi ${tool.env_var_api_key || 'API Key'}...`
                        }
                        value={cfg.apiKey}
                        onChange={(e) => handleApiKeyChange(tool.id, e.target.value)}
                        className="w-full bg-[#121212] border border-[#262626] rounded px-2.5 py-1 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none font-mono pr-8"
                      />
                      <button
                        type="button"
                        onClick={() => toggleShowKey(tool.id)}
                        className="absolute right-2 top-1.5 text-text-muted hover:text-white"
                        title={showKeyMap[tool.id] ? 'Sembunyikan' : 'Tampilkan'}
                      >
                        {showKeyMap[tool.id] ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              {/* Tombol Terapkan, Uji Latensi Ping & Snippet Shell */}
              <div className="mt-4 pt-3 border-t border-[#262626] space-y-2">
                {successMsg && (
                  <div className="p-2 rounded bg-accent/10 border border-accent/30 text-[11px] font-mono text-accent flex items-center gap-1.5">
                    <Check className="w-3.5 h-3.5 flex-shrink-0" />
                    <span>{successMsg}</span>
                  </div>
                )}

                <div className="flex items-center gap-2">
                  <Button
                    variant="primary"
                    size="sm"
                    className="flex-1 text-xs min-h-[36px] font-semibold cursor-pointer"
                    disabled={isApplying}
                    onClick={() => handleApply(tool.id)}
                    title={
                      isLocalGateway
                        ? 'Simpan dan terapkan langsung ke berkas konfigurasi lokal host'
                        : 'Simpan preferensi konfigurasi perkakas ini di server gateway'
                    }
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

                  <button
                    type="button"
                    onClick={() => handlePing(tool.id)}
                    disabled={pingStatus[tool.id]?.testing}
                    title="Uji latensi respon gateway dari browser ke Route-X"
                    className="min-h-[36px] px-2.5 rounded-inner bg-bg-surface-2 hover:bg-bg-surface-3 border border-border text-xs text-text-secondary hover:text-white transition-all flex items-center gap-1 font-mono cursor-pointer"
                  >
                    {pingStatus[tool.id]?.testing ? (
                      <RefreshCw className="w-3 h-3 animate-spin text-accent" />
                    ) : pingStatus[tool.id]?.latency !== undefined ? (
                      <span className={pingStatus[tool.id]?.ok ? 'text-emerald-400 font-semibold' : 'text-red-400 font-semibold'}>
                        {pingStatus[tool.id]?.latency}ms
                      </span>
                    ) : (
                      <>
                        <Zap className="w-3 h-3 text-text-muted hover:text-accent" />
                        <span className="hidden sm:inline text-[11px] text-text-muted">Ping</span>
                      </>
                    )}
                  </button>

                  <Button
                    variant="secondary"
                    size="sm"
                    className="min-h-[36px] px-2.5 text-xs gap-1 font-mono text-white cursor-pointer"
                    title="Salin 3 variabel terminal lengkap (Base URL, Target Model & Kunci API)"
                    onClick={() => handleCopy(completeSnippet, `snippet-${tool.id}`)}
                  >
                    {copiedKey === `snippet-${tool.id}` ? (
                      <>
                        <Check className="w-3.5 h-3.5 text-accent" />
                        <span className="hidden sm:inline">Tersalin</span>
                      </>
                    ) : (
                      <>
                        <Copy className="w-3.5 h-3.5 text-text-muted" />
                        <span className="hidden sm:inline">Salin Snippet</span>
                      </>
                    )}
                  </Button>
                </div>
              </div>
            </Card>
          );
        })}
      </div>

      {filteredTools.length === 0 && (
        <div className="text-center py-16 bg-[#141414] rounded-lg border border-[#262626]">
          <Terminal className="w-10 h-10 text-text-muted mx-auto mb-2 opacity-50" />
          <p className="text-sm font-medium text-white">Tidak ada perkakas CLI yang cocok</p>
          <p className="text-xs text-text-muted mt-1">
            Coba ubah kata kunci pencarian atau matikan filter "Terinstall Saja".
          </p>
        </div>
      )}

      {/* Modal Skrip Ekspor Gabungan */}
      <Modal
        isOpen={isExportModalOpen}
        onClose={() => setIsExportModalOpen(false)}
        title="Skrip Lingkungan Terpadu CLI Route-X"
        maxWidth="lg"
        footer={
          <div className="flex items-center justify-between w-full">
            <Button
              variant="secondary"
              onClick={() => handleCopy(modalRenderedScript, 'modal-full-script')}
              icon={
                copiedKey === 'modal-full-script' ? (
                  <Check className="w-3.5 h-3.5 text-accent" />
                ) : (
                  <Copy className="w-3.5 h-3.5" />
                )
              }
            >
              {copiedKey === 'modal-full-script' ? 'Tersalin' : 'Salin Skrip'}
            </Button>
            <Button variant="ghost" onClick={() => setIsExportModalOpen(false)}>
              Tutup
            </Button>
          </div>
        }
      >
        <div className="space-y-4">
          <p className="text-xs text-text-secondary">
            Berkas ini tersimpan di <code>~/.routex/cli-env.sh</code> dan diperbarui secara otomatis setiap kali Anda menekan tombol "Terapkan ke CLI".
          </p>

          {userApiKey && (
            <div className="flex items-center justify-between bg-[#1A1A1A] px-3 py-2 rounded border border-[#262626] text-xs">
              <span className="text-text-muted">Sertakan Kunci API Anda ke skrip ekspor:</span>
              <label className="flex items-center gap-1.5 cursor-pointer text-white font-mono">
                <input
                  type="checkbox"
                  checked={includeKeyInModal}
                  onChange={(e) => setIncludeKeyInModal(e.target.checked)}
                  className="rounded border-[#333] bg-[#121212] text-accent focus:ring-0"
                />
                Inject OPENAI_API_KEY & ANTHROPIC_API_KEY
              </label>
            </div>
          )}

          <div className="relative">
            <pre className="bg-[#121212] p-4 rounded-lg border border-[#262626] text-xs font-mono text-white overflow-x-auto max-h-96">
              {exportScriptLoading ? 'Memuat skrip...' : modalRenderedScript}
            </pre>
          </div>

          <div className="bg-[#1A1A1A] p-3 rounded border border-[#2B2B2B] text-xs text-text-muted">
            <strong className="text-white block mb-1">Tips Integrasi Otomatis Shell:</strong>
            Jalankan perintah berikut di terminal Anda untuk selalu mengaktifkan Route-X saat membuka shell baru:
            <code className="block mt-1 bg-[#121212] p-1.5 rounded font-mono text-accent">
              echo 'source ~/.routex/cli-env.sh' &gt;&gt; ~/.bashrc
            </code>
          </div>
        </div>
      </Modal>
    </div>
  );
};
