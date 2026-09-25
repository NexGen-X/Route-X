import React, { useState, useEffect, useRef, useMemo } from 'react';
import { Terminal, RefreshCw } from 'lucide-react';
import { api } from '../api/client';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import type { CLITool, CLIDetectedResponse, Model, RoutingRule, APIKey } from '../types';
import { useToast } from '../context/ToastContext';
import { copyTextToClipboard } from '../utils/clipboard';
import { QueryError } from '../components/common/QueryError';
import type { ToolConfig, DiagStatus } from '../components/cli/types';
import { extractDynamicComboOptions } from '../components/cli/types';
import { CLIEnvBanner } from '../components/cli/CLIEnvBanner';
import { CLIApiKeyToolbar } from '../components/cli/CLIApiKeyToolbar';
import { CLIFilterToolbar } from '../components/cli/CLIFilterToolbar';
import { CLIToolCard } from '../components/cli/CLIToolCard';
import { CLIExportModal } from '../components/cli/CLIExportModal';

const CATEGORIES = [
  'all',
  'Coding Agent',
  'Terminal Assistant',
  'Local LLM',
  'Git Automation',
  'Workflow & Prompts',
  'DevOps & SRE',
];

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
  const [mainTab, setMainTab] = useState<'installed' | 'all'>('installed');

  // Kunci API global pengguna untuk menghasilkan skrip terminal siap pakai
  const [userApiKey, setUserApiKey] = useState<string>(() => {
    try {
      const legacy = localStorage.getItem('routex_cli_apikey');
      if (legacy !== null) {
        sessionStorage.setItem('routex_cli_apikey', legacy);
        localStorage.removeItem('routex_cli_apikey');
      }
      return sessionStorage.getItem('routex_cli_apikey') || '';
    } catch {
      return '';
    }
  });
  const [showGlobalKey, setShowGlobalKey] = useState(false);

  // Status form lokal per tool { [toolId]: { mode, target, apiKey } }
  const [toolConfigs, setToolConfigs] = useState<Record<string, ToolConfig>>({});
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
  const [diagStatus, setDiagStatus] = useState<Record<string, DiagStatus>>({});

  // Cek apakah user membuka dashboard dari mesin lokal (localhost) atau remote cloud
  const isLocalGateway = useMemo(() => {
    if (typeof window === 'undefined') return false;
    const h = window.location.hostname.toLowerCase();
    return h === 'localhost' || h === '127.0.0.1' || h === '::1';
  }, []);

  // Pilihan konteks lingkungan: Server (host) vs Laptop (remote client)
  const [envContext, setEnvContext] = useState<'server' | 'remote'>(
    isLocalGateway ? 'server' : 'remote',
  );

  const publicBaseURL = useMemo(() => {
    if (typeof window !== 'undefined') {
      return window.location.origin;
    }
    return 'http://localhost:8080';
  }, []);

  // Temukan aturan combo dinamis dari database
  const dynamicComboOptions = useMemo(() => extractDynamicComboOptions(rules), [rules]);

  // Modal skrip ekspor global
  const [isExportModalOpen, setIsExportModalOpen] = useState(false);

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

      const comboOpts = extractDynamicComboOptions(rulesRes.items || []);
      const initialConfigs: Record<string, ToolConfig> = {};
      cliRes.tools.forEach((t: CLITool) => {
        const mode = t.active_mode || 'model_only';
        const fallback =
          mode === 'routing'
            ? rulesRes.items[0]?.name || ''
            : mode === 'combo'
            ? comboOpts[0]?.value || ''
            : modelsRes.items[0]?.model_id || '';
        initialConfigs[t.id] = {
          mode,
          target: t.active_target || fallback,
          apiKey: '',
        };
      });
      setToolConfigs(initialConfigs);
      setLoadError(null);
    } catch (err: unknown) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    void loadData();
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
          }, 4000),
        );

        setData((prev) => {
          if (!prev) return prev;
          return {
            ...prev,
            tools: prev.tools.map((t) => (t.id === toolId ? res.tool : t)),
          };
        });
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`Gagal menerapkan konfigurasi: ${msg || 'Error tidak diketahui'}`, 'Gagal Konfigurasi');
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
    const targetModel =
      cfg?.target || tool.active_target || models[0]?.model_id || 'Atria-Dawn-Preview';
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
    } catch (err: unknown) {
      setDiagStatus((prev) => ({
        ...prev,
        [toolId]: {
          testing: false,
          latency: 0,
          ok: false,
          message: err instanceof Error ? err.message : 'Koneksi gagal',
        },
      }));
    }
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
    } catch (err: unknown) {
      toast.error('Gagal menyalin: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const installedCount = useMemo(
    () => (data?.tools || []).filter((t) => t.installed).length,
    [data],
  );
  const totalCount = useMemo(() => (data?.tools || []).length, [data]);

  const filteredTools = useMemo(() => {
    return (data?.tools || []).filter((tool) => {
      const q = searchQuery.toLowerCase();
      const matchesSearch =
        (tool.name || '').toLowerCase().includes(q) ||
        (tool.id || '').toLowerCase().includes(q) ||
        (tool.description || '').toLowerCase().includes(q);

      const matchesCategory =
        selectedCategory === 'all' ||
        (tool.category || '').toLowerCase() === selectedCategory.toLowerCase();

      const matchesTab = mainTab === 'all' || tool.installed;

      return matchesSearch && matchesCategory && matchesTab;
    });
  }, [data, searchQuery, selectedCategory, mainTab]);

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
      <PageHeader
        title="Integrasi CLI & Editor AI"
        actions={
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void loadData()}
            disabled={refreshing}
            className="gap-1.5 h-8 text-xs"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${refreshing ? 'animate-spin' : ''}`} />
            Pindai Ulang
          </Button>
        }
      />

      {/* Top Banner: Pemisah Konteks Server Host vs Laptop Remote Client */}
      <CLIEnvBanner
        envContext={envContext}
        setEnvContext={setEnvContext}
        installedCount={installedCount}
        totalCount={totalCount}
        publicBaseURL={publicBaseURL}
        userApiKey={userApiKey}
        copiedKey={copiedKey}
        onCopy={handleCopy}
        onOpenExportModal={() => setIsExportModalOpen(true)}
        onRefresh={() => void loadData()}
        refreshing={refreshing}
      />

      {/* Global API Key Helper Toolbar dengan Pilihan Cepat */}
      <CLIApiKeyToolbar
        userApiKey={userApiKey}
        setUserApiKey={setUserApiKey}
        showGlobalKey={showGlobalKey}
        setShowGlobalKey={setShowGlobalKey}
        registeredKeys={registeredKeys}
      />

      {/* Tab Filter Hierarkis & Pencarian */}
      <CLIFilterToolbar
        mainTab={mainTab}
        setMainTab={setMainTab}
        installedCount={installedCount}
        totalCount={totalCount}
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        selectedCategory={selectedCategory}
        setSelectedCategory={setSelectedCategory}
        categories={CATEGORIES}
      />

      {/* Grid Kartu Perkakas CLI */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {filteredTools.map((tool) => (
          <CLIToolCard
            key={tool.id}
            tool={tool}
            config={
              toolConfigs[tool.id] || {
                mode: tool.active_mode || 'model_only',
                target: tool.active_target || models[0]?.model_id || '',
                apiKey: '',
              }
            }
            models={models}
            rules={rules}
            dynamicComboOptions={dynamicComboOptions}
            userApiKey={userApiKey}
            publicBaseURL={publicBaseURL}
            isApplying={applyingTool === tool.id}
            successMsg={applySuccess[tool.id]}
            diag={diagStatus[tool.id]}
            copiedKey={copiedKey}
            onModeChange={handleModeChange}
            onTargetChange={handleTargetChange}
            onApiKeyChange={handleApiKeyChange}
            onApply={handleApply}
            onTestConnection={handleTestConnection}
            onCopy={handleCopy}
          />
        ))}
      </div>

      {/* Tampilan Kosong Jika Tidak Ada Perkakas yang Cocok */}
      {filteredTools.length === 0 && (
        <div className="text-center py-16 bg-bg-surface rounded-box border border-border space-y-3">
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
      <CLIExportModal
        isOpen={isExportModalOpen}
        onClose={() => setIsExportModalOpen(false)}
        userApiKey={userApiKey}
        copiedKey={copiedKey}
        onCopy={handleCopy}
      />
    </div>
  );
};
