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
  Key,
  Eye,
  EyeOff,
  Laptop,
  Server,
  Lock,
  Shield,
  Activity,
} from 'lucide-react';
import { api } from '../api/client';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Badge } from '../components/common/Badge';
import { Modal } from '../components/common/Modal';
import { Select } from '../components/common/Select';
import { Checkbox } from '../components/common/Checkbox';
import type { CLITool, CLIDetectedResponse, Model, RoutingRule, APIKey } from '../types';
import { useToast } from '../context/ToastContext';
import { copyTextToClipboard } from '../utils/clipboard';
import { QueryError } from '../components/common/QueryError';

export const CLIIntegrations: React.FC = () => {
  const { toast } = useToast();
  const [data, setData] = useState<CLIDetectedResponse | null>(null);
  const [models, setModels] = useState<Model[]>([]);
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [registeredKeys, setRegisteredKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('all');

  // Tab utama hierarkis: default ke 'installed' agar tidak visual clutter
  const [mainTab, setMainTab] = useState<'installed' | 'all'>('installed');

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
  const feedbackTimersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    return () => {
      feedbackTimersRef.current.forEach((t) => clearTimeout(t));
      feedbackTimersRef.current = [];
    };
  }, []);

  // Live Protocol Diagnostics status: menguji langsung endpoint inferensi asli
  const [diagStatus, setDiagStatus] = useState<
    Record<string, { testing: boolean; latency?: number; ok?: boolean; message?: string }>
  >({});

  // Cek apakah user membuka dashboard dari mesin lokal (localhost) atau remote cloud
  const isLocalGateway = useMemo(() => {
    if (typeof window === 'undefined') return false;
    const h = window.location.hostname.toLowerCase();
    return h === 'localhost' || h === '127.0.0.1' || h === '::1';
  }, []);

  // Pilihan konteks lingkungan: Server (host) vs Laptop (remote client)
  const [envContext, setEnvContext] = useState<'server' | 'remote'>(isLocalGateway ? 'server' : 'remote');

  const publicBaseURL = useMemo(() => {
    if (typeof window !== 'undefined') {
      return window.location.origin;
    }
    return 'https://id-tech.cloud';
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

  // Modal skrip ekspor global
  const [isExportModalOpen, setIsExportModalOpen] = useState(false);
  const [exportScriptContent, setExportScriptContent] = useState('');
  const [exportScriptLoading, setExportScriptLoading] = useState(false);
  const [includeKeyInModal, setIncludeKeyInModal] = useState(true);

  const loadData = async () => {
    try {
      setRefreshing(true);
      const [cliRes, modelsRes, rulesRes, apiKeysRes] = await Promise.all([
        api.cli.detected(),
        api.models.list(),
        api.routing.list(),
        api.apiKeys.list().catch(() => ({ items: [] })),
      ]);

      setData(cliRes);
      setModels(modelsRes.items || []);
      setRules(rulesRes.items || []);
      setRegisteredKeys(apiKeysRes.items || []);

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
    if (toolId === 'agy') return; // Antigravity CLI terlindungi (read-only)

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
    if (toolId === 'agy') return; // Antigravity CLI terlindungi (read-only)

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
    if (toolId === 'agy') {
      toast.info('Antigravity CLI dilindungi oleh aturan sistem dan tidak dapat dimodifikasi.');
      return;
    }

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
          [toolId]: 'Konfigurasi berhasil disimpan dan disinkronkan!',
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

  // Uji Sambungan Nyata (Live Protocol Diagnostics)
  const handleTestConnection = async (tool: CLITool) => {
    const toolId = tool.id;
    setDiagStatus((prev) => ({ ...prev, [toolId]: { testing: true } }));
    const start = performance.now();
    const cfg = toolConfigs[toolId];
    const targetModel = cfg?.target || tool.active_target || models[0]?.model_id || 'Atria-Dawn-Preview';
    const effectiveKey = cfg?.apiKey || userApiKey || '';

    try {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      };
      if (effectiveKey) {
        headers['Authorization'] = `Bearer ${effectiveKey}`;
        headers['x-api-key'] = effectiveKey;
      }

      let res: Response;
      if (toolId === 'claude') {
        // Uji ke endpoint Anthropic Messages API asli
        headers['anthropic-version'] = '2023-06-01';
        res = await fetch('/v1/messages', {
          method: 'POST',
          headers,
          body: JSON.stringify({
            model: targetModel,
            max_tokens: 5,
            messages: [{ role: 'user', content: 'ping' }],
          }),
        });
      } else {
        // Uji ke endpoint OpenAI Chat Completions asli
        res = await fetch('/v1/chat/completions', {
          method: 'POST',
          headers,
          body: JSON.stringify({
            model: targetModel,
            max_tokens: 5,
            messages: [{ role: 'user', content: 'ping' }],
          }),
        });
      }

      const latency = Math.round(performance.now() - start);
      if (res.ok) {
        setDiagStatus((prev) => ({
          ...prev,
          [toolId]: {
            testing: false,
            latency,
            ok: true,
            message: `200 OK • Terhubung (${latency}ms)`,
          },
        }));
      } else {
        let errDetail = `HTTP ${res.status}`;
        try {
          const errJson = await res.json();
          if (errJson?.error?.message) errDetail = errJson.error.message;
          else if (errJson?.message) errDetail = errJson.message;
        } catch {}
        setDiagStatus((prev) => ({
          ...prev,
          [toolId]: {
            testing: false,
            latency,
            ok: false,
            message: errDetail,
          },
        }));
      }
    } catch (err: any) {
      setDiagStatus((prev) => ({
        ...prev,
        [toolId]: {
          testing: false,
          latency: 0,
          ok: false,
          message: err.message || 'Koneksi gagal',
        },
      }));
    }
  };

  // Susun snippet 3 variabel lengkap (Base URL, Target Model & Kunci API)
  const getToolCompleteSnippet = (tool: CLITool) => {
    const cfg = toolConfigs[tool.id];
    const target = cfg?.target || tool.active_target || models[0]?.model_id || '';
    const isClaude = tool.id === 'claude';
    const gwURL = isClaude ? publicBaseURL : `${publicBaseURL}/v1`;
    const key = cfg?.apiKey || userApiKey || '<KUNCI_API_ROUTEX_ANDA>';

    const lines: string[] = [];

    // 1. Base URL
    for (const [k] of Object.entries(tool.env_vars || {})) {
      if (k.includes('BASE') || k.includes('HOST') || k.includes('ENDPOINT') || k.includes('PATH')) {
        lines.push(`export ${k}='${gwURL}'`);
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
      toast.success('Disalin ke clipboard.');
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
      '# Kunci API Route-X untuk autentikasi CLI',
      `export OPENAI_API_KEY='${keyToUse}'`,
      `export ANTHROPIC_API_KEY='${keyToUse}'`,
      `export OPENCODE_API_KEY='${keyToUse}'`,
      '',
    ].join('\n');

    return header + exportScriptContent;
  }, [exportScriptContent, includeKeyInModal, userApiKey]);

  const categories = ['all', 'Coding Agent', 'Terminal Assistant', 'Local LLM', 'Git Automation', 'Workflow & Prompts', 'DevOps & SRE'];

  const installedCount = useMemo(() => (data?.tools || []).filter((t) => t.installed).length, [data]);
  const totalCount = useMemo(() => (data?.tools || []).length, [data]);

  const filteredTools = (data?.tools || []).filter((tool) => {
    const q = searchQuery.toLowerCase();
    const matchesSearch =
      (tool.name || '').toLowerCase().includes(q) ||
      (tool.id || '').toLowerCase().includes(q) ||
      (tool.description || '').toLowerCase().includes(q);

    const matchesCategory =
      selectedCategory === 'all' || (tool.category || '').toLowerCase() === selectedCategory.toLowerCase();

    const matchesTab = mainTab === 'all' || tool.installed;

    return matchesSearch && matchesCategory && matchesTab;
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
      {/* Top Banner: Pemisah Konteks Server Host vs Laptop Remote Client */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Kolom 1: Status Terdeteksi & Toggle Lingkungan */}
        <Card className="p-5 flex flex-col justify-between border-l-4 border-l-accent">
          <div>
            <div className="flex items-center justify-between mb-3">
              <span className="text-xs font-semibold text-text-muted uppercase tracking-wider">
                Konektivitas CLI
              </span>
              <div className="flex items-center gap-1 bg-[#121212] p-0.5 rounded border border-[#262626]">
                <button
                  type="button"
                  onClick={() => setEnvContext('server')}
                  className={`text-[11px] px-2 py-0.5 rounded font-medium flex items-center gap-1 transition-all ${
                    envContext === 'server'
                      ? 'bg-accent text-black font-bold shadow'
                      : 'text-text-muted hover:text-white'
                  }`}
                  title="CLI berjalan di server ini"
                >
                  <Server className="w-3 h-3" /> Server Host
                </button>
                <button
                  type="button"
                  onClick={() => setEnvContext('remote')}
                  className={`text-[11px] px-2 py-0.5 rounded font-medium flex items-center gap-1 transition-all ${
                    envContext === 'remote'
                      ? 'bg-accent text-black font-bold shadow'
                      : 'text-text-muted hover:text-white'
                  }`}
                  title="CLI berjalan di laptop atau komputer pribadi Anda"
                >
                  <Laptop className="w-3 h-3" /> Laptop/PC
                </button>
              </div>
            </div>

            <div className="flex items-baseline gap-2 mb-1">
              <h3 className="text-2xl font-extrabold text-white">
                {installedCount}
              </h3>
              <span className="text-sm text-text-muted">
                dari {totalCount} Perkakas Terpasang
              </span>
            </div>

            <p className="text-xs text-text-secondary mt-2">
              {envContext === 'server' ? (
                <span>
                  🟢 <strong>Mode Server Host:</strong> Tombol <em>"Terapkan ke CLI"</em> langsung memperbarui file konfigurasi di server ini (<code>~/.config/opencode</code>, <code>~/.claude</code>, dll.).
                </span>
              ) : (
                <span>
                  🌐 <strong>Mode Remote Client:</strong> Untuk terminal laptop/komputer Anda, gunakan tombol <em>"Salin Snippet"</em> menuju endpoint publik <code>{publicBaseURL}</code>.
                </span>
              )}
            </p>
          </div>

          <div className="mt-4 pt-3 border-t border-[#262626] flex items-center justify-between text-xs font-mono">
            <span className="text-text-muted">Gateway URL:</span>
            <code className="text-accent bg-accent/10 px-2 py-0.5 rounded text-[11px]">
              {publicBaseURL}/v1
            </code>
          </div>
        </Card>

        {/* Kolom 2: Skrip Lingkungan Terpadu & Panduan Cepat */}
        <Card className="p-5 flex flex-col justify-between lg:col-span-2">
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-semibold text-text-muted flex items-center gap-1.5 uppercase tracking-wider">
                <FileCode className="w-3.5 h-3.5 text-accent" />
                {envContext === 'server' ? 'Auto-Loader Shell Host' : 'Ekspor Cepat Terminal Laptop'}
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleOpenExportModal}
                className="text-xs h-7 gap-1 text-text-muted hover:text-white"
              >
                <ExternalLink className="w-3.5 h-3.5" />
                Lihat Skrip Penuh
              </Button>
            </div>

            {envContext === 'server' ? (
              <div className="space-y-2">
                <p className="text-xs text-text-secondary">
                  Host server ini sudah dilengkapi <strong>Auto-Loader</strong> di <code>~/.bashrc</code> dan <code>/etc/profile.d/routex.sh</code>. Setiap kali Anda membuka terminal baru, seluruh konfigurasi CLI otomatis aktif:
                </p>
                <div className="flex items-center gap-2 bg-[#121212] border border-[#262626] p-2.5 rounded-lg">
                  <code className="text-xs font-mono text-accent flex-1 select-all">
                    source ~/.routex/cli-env.sh
                  </code>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-xs gap-1 text-text-muted hover:text-white"
                    onClick={() => handleCopy('source ~/.routex/cli-env.sh', 'global-source')}
                  >
                    {copiedKey === 'global-source' ? (
                      <Check className="w-3.5 h-3.5 text-accent" />
                    ) : (
                      <Copy className="w-3.5 h-3.5" />
                    )}
                    {copiedKey === 'global-source' ? 'Tersalin' : 'Salin'}
                  </Button>
                </div>
              </div>
            ) : (
              <div className="space-y-2">
                <p className="text-xs text-text-secondary">
                  Jalankan perintah ini di terminal komputer/laptop Anda untuk mengarahkan semua CLI ke Route-X Gateway:
                </p>
                <div className="flex items-center gap-2 bg-[#121212] border border-[#262626] p-2 rounded-lg">
                  <code className="text-[11px] font-mono text-accent flex-1 truncate select-all">
                    export ANTHROPIC_BASE_URL="{publicBaseURL}" OPENAI_API_BASE="{publicBaseURL}/v1" ROUTEX_API_KEY="{userApiKey || '<KUNCI_API>'}"
                  </code>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-xs gap-1 text-text-muted hover:text-white"
                    onClick={() => {
                      const snippet = `export ANTHROPIC_BASE_URL="${publicBaseURL}"\nexport OPENAI_API_BASE="${publicBaseURL}/v1"\nexport OPENCODE_API_BASE="${publicBaseURL}/v1"\nexport ROUTEX_API_KEY="${userApiKey || '<KUNCI_API_ROUTEX>'}"`;
                      handleCopy(snippet, 'laptop-snippet');
                    }}
                  >
                    {copiedKey === 'laptop-snippet' ? (
                      <Check className="w-3.5 h-3.5 text-accent" />
                    ) : (
                      <Copy className="w-3.5 h-3.5" />
                    )}
                    {copiedKey === 'laptop-snippet' ? 'Tersalin' : 'Salin'}
                  </Button>
                </div>
              </div>
            )}
          </div>

          <div className="mt-3 pt-3 border-t border-[#262626] flex items-center justify-between text-[11px] text-text-muted">
            <span>
              💡 <em>Stabilitas:</em> Route-X otomatis melakukan verifikasi protokol dan menjaga file konfigurasi klien.
            </span>
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

      {/* Global API Key Helper Toolbar dengan Pilihan Cepat */}
      <div className="bg-[#161616] p-4 rounded-lg border border-[#262626] space-y-3">
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-accent/10 text-accent flex-shrink-0">
              <Key className="w-4 h-4" />
            </div>
            <div>
              <h4 className="text-xs font-bold text-white flex items-center gap-2">
                Kunci API Route-X untuk CLI
                <Badge variant={userApiKey ? 'lime' : 'neutral'}>
                  {userApiKey ? 'Kunci Terpasang' : 'Belum Diisi'}
                </Badge>
              </h4>
              <p className="text-[11px] text-text-muted">
                Kunci ini digunakan saat melakukan uji sambungan dan disematkan ke snippet shell agar terbebas dari error 401.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 w-full md:w-auto">
            <div className="relative flex-1 md:w-80">
              <input
                type={showGlobalKey ? 'text' : 'password'}
                placeholder="sk_live_..."
                value={userApiKey}
                onChange={(e) => {
                  const val = e.target.value.trim();
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
                Hapus
              </Button>
            )}
          </div>
        </div>

        {/* Pilihan Cepat Kunci Terdaftar di Database */}
        {registeredKeys.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-[#222] text-[11px]">
            <span className="text-text-muted flex items-center gap-1 font-medium">
              <Key className="w-3 h-3 text-accent" /> Kunci Terdaftar di Sistem:
            </span>
            {registeredKeys.map((k) => (
              <button
                key={k.id}
                type="button"
                onClick={() => {
                  if (k.raw_key) {
                    setUserApiKey(k.raw_key);
                    try {
                      localStorage.setItem('routex_cli_apikey', k.raw_key);
                    } catch {}
                    toast.success(`Kunci "${k.name}" dipilih dan diterapkan.`);
                  } else {
                    toast.info(`Kunci "${k.name}" berstatus aktif (${k.masked_key}). Jika Anda memiliki salinan raw key, tempelkan di kotak input.`);
                  }
                }}
                className="px-2.5 py-0.5 rounded text-[11px] font-mono bg-[#1E1E1E] hover:bg-[#282828] text-text-secondary hover:text-white border border-[#333] transition-colors flex items-center gap-1"
                title={`Gunakan kunci ${k.name}`}
              >
                <span className="text-white font-medium">{k.name}</span>
                <span className="text-text-muted">({k.masked_key})</span>
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Tab Filter Hierarkis & Pencarian (Penyederhanaan Visual) */}
      <div className="flex flex-col sm:flex-row gap-3 items-center justify-between bg-[#141414] p-3 rounded-lg border border-[#262626]">
        {/* Tab Utama: Terpasang vs Semua */}
        <div className="flex items-center gap-2 w-full sm:w-auto">
          <div className="flex p-1 bg-[#1A1A1A] rounded-lg border border-[#262626]">
            <button
              type="button"
              onClick={() => setMainTab('installed')}
              className={`text-xs px-3 py-1.5 rounded-md font-semibold flex items-center gap-1.5 transition-all ${
                mainTab === 'installed'
                  ? 'bg-accent text-black shadow'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <CheckCircle2 className="w-3.5 h-3.5" />
              CLI Terpasang
              <span className={`px-1.5 py-0.2 rounded-full text-[10px] ${
                mainTab === 'installed' ? 'bg-black/20 text-black font-bold' : 'bg-[#262626] text-text-muted'
              }`}>
                {installedCount}
              </span>
            </button>

            <button
              type="button"
              onClick={() => setMainTab('all')}
              className={`text-xs px-3 py-1.5 rounded-md font-semibold flex items-center gap-1.5 transition-all ${
                mainTab === 'all'
                  ? 'bg-accent text-black shadow'
                  : 'text-text-muted hover:text-white'
              }`}
            >
              <Terminal className="w-3.5 h-3.5" />
              Katalog Lengkap
              <span className={`px-1.5 py-0.2 rounded-full text-[10px] ${
                mainTab === 'all' ? 'bg-black/20 text-black font-bold' : 'bg-[#262626] text-text-muted'
              }`}>
                {totalCount}
              </span>
            </button>
          </div>
        </div>

        {/* Search & Category Filter */}
        <div className="flex items-center gap-3 w-full sm:w-auto">
          <div className="relative w-full sm:w-64">
            <Search className="w-4 h-4 absolute left-3 top-2.5 text-text-muted" />
            <input
              type="text"
              placeholder="Cari (misal: claude, opencode)..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-[#1C1C1C] border border-[#333] rounded-md pl-9 pr-3 py-1.5 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none"
            />
          </div>

          <div className="flex items-center gap-1.5 overflow-x-auto">
            <Filter className="w-3.5 h-3.5 text-text-muted flex-shrink-0" />
            <select
              value={selectedCategory}
              onChange={(e) => setSelectedCategory(e.target.value)}
              className="bg-[#1C1C1C] border border-[#333] rounded px-2 py-1.5 text-xs text-white focus:border-accent focus:outline-none"
            >
              <option value="all">Semua Kategori</option>
              {categories.filter(c => c !== 'all').map((cat) => (
                <option key={cat} value={cat}>{cat}</option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* Grid Kartu Perkakas CLI */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {filteredTools.map((tool) => {
          const isAgy = tool.id === 'agy';
          const cfg = toolConfigs[tool.id] || {
            mode: tool.active_mode || 'model_only',
            target: tool.active_target || models[0]?.model_id || '',
            apiKey: '',
          };
          const isApplying = applyingTool === tool.id;
          const successMsg = applySuccess[tool.id];
          const completeSnippet = getToolCompleteSnippet(tool);
          const diag = diagStatus[tool.id];

          return (
            <Card
              key={tool.id}
              className={`p-5 flex flex-col justify-between transition-all duration-200 ${
                isAgy
                  ? 'border-amber-500/30 bg-[#161410] shadow-sm'
                  : tool.installed
                  ? 'border-[#333] hover:border-accent/40 bg-[#161616]'
                  : 'border-[#222] opacity-75 bg-[#121212]'
              }`}
            >
              <div>
                {/* Header Kartu */}
                <div className="flex items-start justify-between gap-2 mb-2">
                  <div className="flex items-center gap-2.5">
                    <div
                      className={`p-2 rounded-lg ${
                        isAgy
                          ? 'bg-amber-500/10 text-amber-400'
                          : tool.installed
                          ? 'bg-accent/10 text-accent'
                          : 'bg-[#222] text-text-muted'
                      }`}
                    >
                      {isAgy ? <Shield className="w-5 h-5" /> : <Terminal className="w-5 h-5" />}
                    </div>
                    <div>
                      <h4 className="text-base font-bold text-white flex items-center gap-2">
                        {tool.name}
                      </h4>
                      <span className="text-[11px] font-mono text-text-muted">{tool.category}</span>
                    </div>
                  </div>

                  {/* Status Badge */}
                  {isAgy ? (
                    <Badge variant="neutral" className="gap-1 bg-amber-500/10 text-amber-400 border-amber-500/30 font-semibold">
                      <Lock className="w-3 h-3" />
                      Dilindungi Sistem
                    </Badge>
                  ) : tool.installed ? (
                    <Badge variant="lime" className="gap-1 font-semibold">
                      <CheckCircle2 className="w-3 h-3" />
                      Terpasang
                    </Badge>
                  ) : (
                    <Badge variant="neutral" className="gap-1 text-text-muted">
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
                      📍 <span className="text-text-muted">Biner:</span> {tool.path}
                    </div>
                    {tool.version && (
                      <div className="truncate text-accent">
                        ⚡ <span className="text-text-muted">Versi:</span> {tool.version}
                      </div>
                    )}
                  </div>
                )}

                {/* Pengamanan Khusus Antigravity CLI */}
                {isAgy ? (
                  <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 text-xs text-amber-400 space-y-1.5 mb-3">
                    <div className="flex items-center gap-1.5 font-bold">
                      <Lock className="w-3.5 h-3.5" /> Antigravity CLI (Read-Only)
                    </div>
                    <p className="text-[11px] leading-relaxed text-amber-300/80">
                      Antigravity CLI dikelola langsung oleh sistem inti DeepMind. Konfigurasi terkunci untuk memastikan stabilitas sesi pair-programming Anda.
                    </p>
                  </div>
                ) : (
                  /* Kolom Konfigurasi 3-Mode */
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
                          <div className="p-2.5 rounded bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                            Belum ada model terdaftar di katalog.
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
                          <div className="p-2.5 rounded bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                            Belum ada aturan routing terdaftar di database.
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
                            <div className="p-2 rounded bg-[#121212] border border-[#262626] text-[10px] font-mono space-y-1">
                              <div className="text-text-muted flex items-center justify-between">
                                <span>Alur Cascade:</span>
                                <span className="text-accent font-semibold">Auto-Failover</span>
                              </div>
                              <div className="flex items-center gap-1.5 overflow-x-auto py-0.5">
                                <span className="px-1.5 py-0.5 rounded bg-[#1C1C1C] border border-[#333] text-white whitespace-nowrap">
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
                          <div className="p-2.5 rounded bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-400">
                            Belum ada aturan combo bertingkat.
                          </div>
                        )
                      )}
                    </div>

                    {/* Kunci API Khusus Tool (Opsional) */}
                    <div>
                      <div className="flex items-center justify-between mb-1">
                        <label className="text-[11px] font-medium text-text-muted flex items-center gap-1">
                          <Key className="w-3 h-3 text-accent" />
                          Kunci API Khusus:
                        </label>
                        {cfg.apiKey ? (
                          <span className="text-[10px] text-accent font-mono font-semibold">Kustom</span>
                        ) : userApiKey ? (
                          <span className="text-[10px] text-emerald-400 font-mono">Memakai Global</span>
                        ) : (
                          <span className="text-[10px] text-text-muted font-mono">Opsional</span>
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
                )}
              </div>

              {/* Action Buttons & Real Live Diagnostics */}
              <div className="mt-4 pt-3 border-t border-[#262626] space-y-2">
                {successMsg && (
                  <div className="p-2 rounded bg-accent/10 border border-accent/30 text-[11px] font-mono text-accent flex items-center gap-1.5">
                    <Check className="w-3.5 h-3.5 flex-shrink-0" />
                    <span>{successMsg}</span>
                  </div>
                )}

                {/* Hasil Live Diagnostics */}
                {diag && (
                  <div
                    className={`p-2 rounded text-[11px] font-mono flex items-center justify-between gap-1.5 border ${
                      diag.testing
                        ? 'bg-[#1A1A1A] border-[#333] text-text-muted'
                        : diag.ok
                        ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400'
                        : 'bg-red-500/10 border-red-500/30 text-red-400'
                    }`}
                  >
                    <div className="flex items-center gap-1.5 truncate">
                      {diag.testing ? (
                        <RefreshCw className="w-3 h-3 animate-spin text-accent" />
                      ) : diag.ok ? (
                        <CheckCircle2 className="w-3.5 h-3.5 flex-shrink-0 text-emerald-400" />
                      ) : (
                        <XCircle className="w-3.5 h-3.5 flex-shrink-0 text-red-400" />
                      )}
                      <span className="truncate">{diag.testing ? 'Menguji sambungan inferensi...' : diag.message}</span>
                    </div>
                    {diag.latency !== undefined && !diag.testing && (
                      <span className="text-[10px] font-bold opacity-80">{diag.latency}ms</span>
                    )}
                  </div>
                )}

                <div className="flex items-center gap-2">
                  {/* Tombol Terapkan ke CLI */}
                  {isAgy ? (
                    <Button
                      variant="secondary"
                      size="sm"
                      disabled
                      className="flex-1 text-xs min-h-[36px] opacity-60 cursor-not-allowed font-medium"
                      title="Antigravity CLI dilindungi oleh sistem"
                    >
                      <Lock className="w-3 h-3 mr-1.5 text-amber-400" />
                      Terkunci (Sistem)
                    </Button>
                  ) : (
                    <Button
                      variant="primary"
                      size="sm"
                      className="flex-1 text-xs min-h-[36px] font-semibold cursor-pointer"
                      disabled={isApplying}
                      onClick={() => handleApply(tool.id)}
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
                  )}

                  {/* Tombol Uji Sambungan Inferensi Real */}
                  <button
                    type="button"
                    onClick={() => handleTestConnection(tool)}
                    disabled={diag?.testing}
                    title="Uji sambungan inferensi nyata (test token) ke endpoint Route-X"
                    className="min-h-[36px] px-2.5 rounded-inner bg-[#1A1A1A] hover:bg-[#222] border border-[#333] text-xs text-text-secondary hover:text-white transition-all flex items-center gap-1 font-mono cursor-pointer"
                  >
                    {diag?.testing ? (
                      <RefreshCw className="w-3.5 h-3.5 animate-spin text-accent" />
                    ) : (
                      <>
                        <Activity className="w-3.5 h-3.5 text-accent" />
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
                        <span className="hidden sm:inline">Salin</span>
                      </>
                    )}
                  </Button>
                </div>
              </div>
            </Card>
          );
        })}
      </div>

      {/* Tampilan Kosong Jika Tidak Ada Perkakas yang Cocok */}
      {filteredTools.length === 0 && (
        <div className="text-center py-16 bg-[#141414] rounded-lg border border-[#262626] space-y-3">
          <Terminal className="w-10 h-10 text-text-muted mx-auto opacity-50" />
          <h4 className="text-base font-semibold text-white">
            {mainTab === 'installed'
              ? 'Belum ada perkakas CLI yang terdeteksi terpasang di host'
              : 'Tidak ada perkakas CLI yang cocok dengan pencarian'}
          </h4>
          <p className="text-xs text-text-muted max-w-md mx-auto">
            {mainTab === 'installed'
              ? 'Periksa tab "Katalog Lengkap" untuk melihat seluruh perkakas AI CLI yang didukung Route-X.'
              : 'Coba ubah kata kunci pencarian atau ganti filter kategori.'}
          </p>
          {mainTab === 'installed' && (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setMainTab('all')}
              className="text-xs gap-1.5"
            >
              <Terminal className="w-3.5 h-3.5" />
              Buka Katalog Lengkap ({totalCount})
            </Button>
          )}
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
            Berkas ini tersimpan di <code>~/.routex/cli-env.sh</code> dan otomatis disinkronkan setiap kali Anda menekan tombol "Terapkan ke CLI".
          </p>

          {userApiKey && (
            <div className="flex items-center justify-between bg-[#1A1A1A] px-3 py-2 rounded border border-[#262626] text-xs">
              <span className="text-text-muted">Sertakan Kunci API Anda ke skrip ekspor:</span>
              <Checkbox
                checked={includeKeyInModal}
                onChange={(e) => setIncludeKeyInModal(e.target.checked)}
                label={<span className="text-white font-mono">Inject OPENAI_API_KEY & ANTHROPIC_API_KEY</span>}
              />
            </div>
          )}

          <div className="relative">
            <pre className="bg-[#121212] p-4 rounded-lg border border-[#262626] text-xs font-mono text-white overflow-x-auto max-h-96">
              {exportScriptLoading ? 'Memuat skrip...' : modalRenderedScript}
            </pre>
          </div>

          <div className="bg-[#1A1A1A] p-3 rounded border border-[#2B2B2B] text-xs text-text-muted">
            <strong className="text-white block mb-1">Tips Integrasi Shell:</strong>
            Untuk memuat otomatis setiap membuka sesi terminal baru di mesin Anda:
            <code className="block mt-1 bg-[#121212] p-1.5 rounded font-mono text-accent">
              echo 'source ~/.routex/cli-env.sh' &gt;&gt; ~/.bashrc
            </code>
          </div>
        </div>
      </Modal>
    </div>
  );
};
