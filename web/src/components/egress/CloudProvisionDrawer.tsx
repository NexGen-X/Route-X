import React, { useState } from 'react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { Checkbox } from '../common/Checkbox';
import { api } from '../../api/client';
import { useToast } from '../../context/ToastContext';
import {
  Cloud,
  Zap,
  ExternalLink,
  CheckCircle2,
  AlertCircle,
  Copy,
  Check,
  Eye,
  EyeOff,
  Sparkles,
} from 'lucide-react';

interface CloudProvisionDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

type TabType = 'cloudflare' | 'deno';

export const CloudProvisionDrawer: React.FC<CloudProvisionDrawerProps> = ({
  isOpen,
  onClose,
  onSuccess,
}) => {
  const { toast } = useToast();
  const [activeTab, setActiveTab] = useState<TabType>('cloudflare');

  // Cloudflare state
  const [cfAccountId, setCfAccountId] = useState('');
  const [cfApiToken, setCfApiToken] = useState('');
  const [cfGatewayId, setCfGatewayId] = useState('routex-ai-gateway');
  const [cfPoolName, setCfPoolName] = useState('');
  const [cfCollectLogs, setCfCollectLogs] = useState(true);
  const [cfEnableCache, setCfEnableCache] = useState(true);
  const [showCfToken, setShowCfToken] = useState(false);

  // Deno state
  const [denoToken, setDenoToken] = useState('');
  const [denoProjectName, setDenoProjectName] = useState(() => `routex-relay-${Math.random().toString(36).substring(2, 7)}`);
  const [denoPoolName, setDenoPoolName] = useState('');
  const [showDenoToken, setShowDenoToken] = useState(false);

  // Execution state
  const [isLoading, setIsLoading] = useState(false);
  const [progressMsg, setProgressMsg] = useState('');
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [successResult, setSuccessResult] = useState<{
    provider: string;
    url: string;
    name: string;
    alreadyExist?: boolean;
  } | null>(null);
  const [isCopied, setIsCopied] = useState(false);

  const resetForm = () => {
    setIsLoading(false);
    setProgressMsg('');
    setErrorMsg(null);
    setSuccessResult(null);
    setIsCopied(false);
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text);
    setIsCopied(true);
    toast.success('URL proxy berhasil disalin ke clipboard.', 'Disalin');
    setTimeout(() => setIsCopied(false), 2000);
  };

  const handleProvisionCloudflare = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!cfAccountId.trim()) {
      setErrorMsg('Cloudflare Account ID wajib diisi.');
      return;
    }
    if (!cfApiToken.trim()) {
      setErrorMsg('Cloudflare API Token wajib diisi.');
      return;
    }

    setIsLoading(true);
    setErrorMsg(null);
    setProgressMsg('Menghubungi Cloudflare API v4...');

    try {
      setProgressMsg('Mendaftarkan AI Gateway di Cloudflare Edge...');
      const res = await api.egress.provisionCloudflare({
        account_id: cfAccountId.trim(),
        api_token: cfApiToken.trim(),
        gateway_id: cfGatewayId.trim() || 'routex-ai-gateway',
        name: cfPoolName.trim() || undefined,
        collect_logs: cfCollectLogs,
        enable_cache: cfEnableCache,
      });

      setProgressMsg('Menyimpan ke Egress Pools Route-X...');
      setSuccessResult({
        provider: 'Cloudflare AI Gateway',
        url: res.target_url,
        name: res.pool.name,
        alreadyExist: res.already_exist,
      });
      toast.success(
        res.already_exist
          ? 'AI Gateway yang sudah ada di Cloudflare berhasil dihubungkan.'
          : 'AI Gateway baru berhasil dibuat di Cloudflare.',
        'Cloudflare Terhubung'
      );
      onSuccess();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      setErrorMsg(message || 'Gagal membuat Cloudflare AI Gateway. Pastikan token dan account ID benar.');
    } finally {
      setIsLoading(false);
    }
  };

  const handleProvisionDeno = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!denoToken.trim()) {
      setErrorMsg('Deno Deploy Access Token wajib diisi.');
      return;
    }

    setIsLoading(true);
    setErrorMsg(null);
    setProgressMsg('Menghubungi Deno Deploy API...');

    try {
      setProgressMsg('Mengunggah skrip streaming proxy ke Deno Edge...');
      const res = await api.egress.provisionDeno({
        access_token: denoToken.trim(),
        project_name: denoProjectName.trim() || undefined,
        name: denoPoolName.trim() || undefined,
      });

      setProgressMsg('Menyimpan ke Egress Pools Route-X...');
      setSuccessResult({
        provider: 'Deno Deploy Streaming Relay',
        url: res.target_url,
        name: res.pool.name,
      });
      toast.success('Script streaming relay berhasil di-deploy ke Deno.', 'Deno Terhubung');
      onSuccess();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      setErrorMsg(message || 'Gagal mendeploy ke Deno Deploy. Pastikan access token valid.');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="2xl"
      title={
        <div className="flex items-center gap-2">
          <Zap className="w-5 h-5 text-accent" />
          <span>Auto-Deploy Cloud Proxy Relay</span>
        </div>
      }
    >
      {/* Jika Berhasil (Success View) */}
      {successResult ? (
        <div className="space-y-5 py-4">
          <div className="p-4 bg-emerald-500/10 border border-emerald-500/30 rounded-nav text-emerald-300 space-y-2">
            <div className="flex items-center gap-2 text-sm font-semibold">
              <CheckCircle2 className="w-5 h-5 text-emerald-400" />
              <span>{successResult.provider} Berhasil Dikonfigurasi!</span>
            </div>
            <p className="text-xs text-emerald-400/90 leading-relaxed">
              Jalur proxy egress baru dengan nama <strong>{successResult.name}</strong> telah aktif dan tersimpan terenkripsi di Route-X.
              {successResult.alreadyExist && ' (Menggunakan AI Gateway yang sudah terdaftar di Cloudflare Anda).'}
            </p>
          </div>

          <div className="p-4 bg-bg-surface-2 border border-border rounded-nav space-y-2">
            <label className="text-xs font-semibold text-text-secondary block">
              URL Endpoint Proxy:
            </label>
            <div className="flex items-center gap-2">
              <code className="flex-1 px-3 py-2 bg-bg-surface-1 border border-border rounded text-xs text-accent font-mono break-all select-all">
                {successResult.url}
              </code>
              <Button
                size="sm"
                variant="secondary"
                onClick={() => handleCopy(successResult.url)}
                icon={isCopied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
              >
                {isCopied ? 'Tersalin' : 'Salin'}
              </Button>
            </div>
          </div>

          <div className="p-4 bg-bg-surface-1 border border-border rounded-nav text-xs text-text-muted space-y-2">
            <h4 className="font-semibold text-white flex items-center gap-1.5">
              <Sparkles className="w-4 h-4 text-accent" /> Langkah Selanjutnya:
            </h4>
            <p>
              Buka menu <strong>AI Providers</strong>, edit provider yang Anda inginkan (misal OpenAI atau Anthropic), lalu pilih <strong>{successResult.name}</strong> pada kolom <em>Jalur Egress Outbound</em>.
            </p>
          </div>

          <div className="flex justify-end gap-2 pt-4">
            <Button variant="secondary" onClick={resetForm}>
              Tambah Lagi
            </Button>
            <Button variant="primary" onClick={handleClose}>
              Selesai & Tutup
            </Button>
          </div>
        </div>
      ) : (
        /* Form View */
        <div className="space-y-6">
          {/* Tab Selector */}
          <div className="flex border-b border-border gap-2">
            <button
              type="button"
              onClick={() => { setActiveTab('cloudflare'); setErrorMsg(null); }}
              className={`pb-3 px-3 text-xs font-semibold flex items-center gap-2 border-b-2 transition-colors ${
                activeTab === 'cloudflare'
                  ? 'border-accent text-white'
                  : 'border-transparent text-text-muted hover:text-white'
              }`}
            >
              <Cloud className="w-4 h-4 text-amber-400" />
              <span>Cloudflare AI Gateway</span>
              <span className="text-[10px] font-normal px-1.5 py-0.2 bg-emerald-500/20 text-emerald-300 rounded">
                Rekomendasi
              </span>
            </button>
            <button
              type="button"
              onClick={() => { setActiveTab('deno'); setErrorMsg(null); }}
              className={`pb-3 px-3 text-xs font-semibold flex items-center gap-2 border-b-2 transition-colors ${
                activeTab === 'deno'
                  ? 'border-accent text-white'
                  : 'border-transparent text-text-muted hover:text-white'
              }`}
            >
              <span>🦕 Deno Deploy Relay</span>
              <span className="text-[10px] font-normal px-1.5 py-0.2 bg-blue-500/20 text-blue-300 rounded">
                Streaming V8
              </span>
            </button>
          </div>

          {/* Error Banner */}
          {errorMsg && (
            <div className="p-3 bg-rose-500/10 border border-rose-500/30 rounded-nav text-rose-300 text-xs flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-rose-400 flex-shrink-0 mt-0.5" />
              <span>{errorMsg}</span>
            </div>
          )}

          {/* Tab Content: Cloudflare */}
          {activeTab === 'cloudflare' && (
            <form onSubmit={handleProvisionCloudflare} className="space-y-4 text-xs">
              <div className="p-3.5 bg-amber-500/10 border border-amber-500/20 rounded-nav text-amber-200/90 text-xs space-y-1.5">
                <div className="font-semibold flex items-center gap-1.5 text-amber-300">
                  <Cloud className="w-4 h-4" />
                  <span>Keunggulan Cloudflare AI Gateway:</span>
                </div>
                <p className="text-[11px] leading-relaxed text-amber-200/80">
                  Kuota gratis tanpa batas (unlimited), fitur auto-caching prompt, dan koneksi peering langsung ke OpenAI/Azure.
                </p>
              </div>

              <div>
                <label className="block text-xs font-medium text-text-secondary mb-1.5">
                  Cloudflare Account ID *
                </label>
                <input
                  type="text"
                  required
                  placeholder="Contoh: 8f3a92b04c8172948e9102cba8471..."
                  value={cfAccountId}
                  onChange={(e) => setCfAccountId(e.target.value)}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                />
                <span className="text-[11px] text-text-muted mt-1 block">
                  Dapat disalin dari URL dashboard Cloudflare (setelah dash.cloudflare.com/...).
                </span>
              </div>

              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-xs font-medium text-text-secondary">
                    Cloudflare API Token *
                  </label>
                  <a
                    href="https://dash.cloudflare.com/profile/api-tokens"
                    target="_blank"
                    rel="noreferrer"
                    className="text-[11px] text-accent hover:underline flex items-center gap-1"
                  >
                    <span>Buat Token di Cloudflare</span>
                    <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
                <div className="relative">
                  <input
                    type={showCfToken ? 'text' : 'password'}
                    required
                    placeholder="Token dengan izin 'AI Gateway: Edit'"
                    value={cfApiToken}
                    onChange={(e) => setCfApiToken(e.target.value)}
                    className="w-full px-3 py-2 pr-10 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                  />
                  <button
                    type="button"
                    onClick={() => setShowCfToken(!showCfToken)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white"
                  >
                    {showCfToken ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1.5">
                    Gateway ID (Cloudflare)
                  </label>
                  <input
                    type="text"
                    placeholder="routex-ai-gateway"
                    value={cfGatewayId}
                    onChange={(e) => setCfGatewayId(e.target.value)}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1.5">
                    Nama Pool di Route-X (Opsional)
                  </label>
                  <input
                    type="text"
                    placeholder="☁️ Cloudflare AI Gateway"
                    value={cfPoolName}
                    onChange={(e) => setCfPoolName(e.target.value)}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
                  />
                </div>
              </div>

              <div className="p-3 bg-bg-surface-1 border border-border rounded-nav space-y-2">
                <Checkbox
                  id="cf-cache"
                  label="Aktifkan Edge Cache"
                  checked={cfEnableCache}
                  onChange={(e) => setCfEnableCache(e.target.checked)}
                />
                <span className="text-[11px] text-text-muted block pl-6">
                  Menyimpan respons prompt yang identik di CDN Cloudflare untuk menghemat saldo OpenAI Anda.
                </span>

                <div className="pt-1">
                  <Checkbox
                    id="cf-logs"
                    label="Kumpulkan Log & Analytics di Cloudflare"
                    checked={cfCollectLogs}
                    onChange={(e) => setCfCollectLogs(e.target.checked)}
                  />
                </div>
              </div>

              <div className="pt-2">
                <Button
                  type="submit"
                  variant="primary"
                  className="w-full justify-center"
                  isLoading={isLoading}
                  icon={<Zap className="w-4 h-4" />}
                >
                  {isLoading ? progressMsg : '🚀 Deploy & Hubungkan Cloudflare Otomatis'}
                </Button>
              </div>
            </form>
          )}

          {/* Tab Content: Deno */}
          {activeTab === 'deno' && (
            <form onSubmit={handleProvisionDeno} className="space-y-4 text-xs">
              <div className="p-3.5 bg-blue-500/10 border border-blue-500/20 rounded-nav text-blue-200/90 text-xs space-y-1.5">
                <div className="font-semibold flex items-center gap-1.5 text-blue-300">
                  <span>🦕 Keunggulan Deno Deploy Relay:</span>
                </div>
                <p className="text-[11px] leading-relaxed text-blue-200/80">
                  Didukung Web Streams API native tanpa batas timeout 30 detik. Sangat stabil untuk streaming SSE percakapan panjang.
                </p>
              </div>

              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-xs font-medium text-text-secondary">
                    Deno Deploy Access Token *
                  </label>
                  <a
                    href="https://dash.deno.com/account#access-tokens"
                    target="_blank"
                    rel="noreferrer"
                    className="text-[11px] text-accent hover:underline flex items-center gap-1"
                  >
                    <span>Dapatkan Token Deno</span>
                    <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
                <div className="relative">
                  <input
                    type={showDenoToken ? 'text' : 'password'}
                    required
                    placeholder="ddp_xxxxxxxxxxxxxxxxxxxxxxxx"
                    value={denoToken}
                    onChange={(e) => setDenoToken(e.target.value)}
                    className="w-full px-3 py-2 pr-10 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                  />
                  <button
                    type="button"
                    onClick={() => setShowDenoToken(!showDenoToken)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted hover:text-white"
                  >
                    {showDenoToken ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1.5">
                    Nama Project Deno
                  </label>
                  <input
                    type="text"
                    placeholder="routex-relay-xxxx"
                    value={denoProjectName}
                    onChange={(e) => setDenoProjectName(e.target.value)}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white font-mono focus:outline-none focus:border-accent"
                  />
                  <span className="text-[10px] text-text-muted mt-1 block">
                    Domain akhir: {denoProjectName || 'project'}.deno.dev
                  </span>
                </div>
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1.5">
                    Nama Pool di Route-X (Opsional)
                  </label>
                  <input
                    type="text"
                    placeholder="🦕 Deno Relay"
                    value={denoPoolName}
                    onChange={(e) => setDenoPoolName(e.target.value)}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-white focus:outline-none focus:border-accent"
                  />
                </div>
              </div>

              <div className="pt-2">
                <Button
                  type="submit"
                  variant="primary"
                  className="w-full justify-center"
                  isLoading={isLoading}
                  icon={<Zap className="w-4 h-4" />}
                >
                  {isLoading ? progressMsg : '🚀 Deploy & Hubungkan Deno Otomatis'}
                </Button>
              </div>
            </form>
          )}
        </div>
      )}
    </Drawer>
  );
};
