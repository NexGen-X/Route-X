import React from 'react';
import {
  Server,
  Laptop,
  FileCode,
  ExternalLink,
  Copy,
  Check,
  RefreshCw,
} from 'lucide-react';
import { Card } from '../common/Card';
import { Button } from '../common/Button';

export interface CLIEnvBannerProps {
  envContext: 'server' | 'remote';
  setEnvContext: (ctx: 'server' | 'remote') => void;
  installedCount: number;
  totalCount: number;
  publicBaseURL: string;
  userApiKey: string;
  copiedKey: string | null;
  onCopy: (text: string, key: string) => void;
  onOpenExportModal: () => void;
  onRefresh: () => void;
  refreshing: boolean;
}

export const CLIEnvBanner: React.FC<CLIEnvBannerProps> = ({
  envContext,
  setEnvContext,
  installedCount,
  totalCount,
  publicBaseURL,
  userApiKey,
  copiedKey,
  onCopy,
  onOpenExportModal,
  onRefresh,
  refreshing,
}) => {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
      {/* Kolom 1: Status Terdeteksi & Toggle Lingkungan */}
      <Card className="p-5 flex flex-col justify-between border-l-4 border-l-accent">
        <div>
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-semibold text-text-muted uppercase tracking-wider">
              Konektivitas CLI
            </span>
            <div className="flex items-center gap-1 bg-bg-surface-2 p-0.5 rounded border border-border">
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
            <h3 className="text-2xl font-extrabold text-white">{installedCount}</h3>
            <span className="text-sm text-text-muted">dari {totalCount} Perkakas Terpasang</span>
          </div>

          <p className="text-xs text-text-secondary mt-2">
            {envContext === 'server' ? (
              <span>
                🟢 <strong>Mode Server Host:</strong> Tombol <em>"Terapkan ke CLI"</em> langsung
                memperbarui file konfigurasi di server ini (<code>~/.config/opencode</code>,{' '}
                <code>~/.claude</code>, dll.).
              </span>
            ) : (
              <span>
                🌐 <strong>Mode Remote Client:</strong> Untuk terminal laptop/komputer Anda,
                gunakan tombol <em>"Salin Snippet"</em> menuju endpoint publik{' '}
                <code>{publicBaseURL}</code>.
              </span>
            )}
          </p>
        </div>

        <div className="mt-4 pt-3 border-t border-border flex items-center justify-between text-xs font-mono">
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
              onClick={onOpenExportModal}
              className="text-xs h-7 gap-1 text-text-muted hover:text-white"
            >
              <ExternalLink className="w-3.5 h-3.5" />
              Lihat Skrip Penuh
            </Button>
          </div>

          {envContext === 'server' ? (
            <div className="space-y-2">
              <p className="text-xs text-text-secondary">
                Host server ini sudah dilengkapi <strong>Auto-Loader</strong> di{' '}
                <code>~/.bashrc</code> dan <code>/etc/profile.d/routex.sh</code>. Setiap kali Anda
                membuka terminal baru, seluruh konfigurasi CLI otomatis aktif:
              </p>
              <div className="flex items-center gap-2 bg-bg-surface-2 border border-border p-2.5 rounded-lg">
                <code className="text-xs font-mono text-accent flex-1 select-all">
                  source ~/.routex/cli-env.sh
                </code>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs gap-1 text-text-muted hover:text-white"
                  onClick={() => onCopy('source ~/.routex/cli-env.sh', 'global-source')}
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
                Jalankan perintah ini di terminal komputer/laptop Anda untuk mengarahkan semua CLI ke
                Route-X Gateway:
              </p>
              <div className="flex items-center gap-2 bg-bg-surface-2 border border-border p-2 rounded-lg">
                <code className="text-[11px] font-mono text-accent flex-1 truncate select-all">
                  export ANTHROPIC_BASE_URL="{publicBaseURL}" OPENAI_API_BASE="{publicBaseURL}/v1"{' '}
                  ROUTEX_API_KEY="{userApiKey || '<KUNCI_API>'}"
                </code>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs gap-1 text-text-muted hover:text-white"
                  onClick={() => {
                    const snippet = `export ANTHROPIC_BASE_URL="${publicBaseURL}"\nexport OPENAI_API_BASE="${publicBaseURL}/v1"\nexport OPENCODE_API_BASE="${publicBaseURL}/v1"\nexport ROUTEX_API_KEY="${userApiKey || '<KUNCI_API_ROUTEX>'}"`;
                    onCopy(snippet, 'laptop-snippet');
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

        <div className="mt-3 pt-3 border-t border-border flex items-center justify-between text-[11px] text-text-muted">
          <span>
            💡 <em>Stabilitas:</em> Route-X otomatis melakukan verifikasi protokol dan menjaga file
            konfigurasi klien.
          </span>
          <Button
            variant="secondary"
            size="sm"
            onClick={onRefresh}
            disabled={refreshing}
            className="gap-1.5 h-7 text-xs"
          >
            <RefreshCw className={`w-3 h-3 ${refreshing ? 'animate-spin' : ''}`} />
            Pindai Ulang
          </Button>
        </div>
      </Card>
    </div>
  );
};
