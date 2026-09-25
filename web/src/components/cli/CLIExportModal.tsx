import React, { useEffect, useState, useMemo } from 'react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { Checkbox } from '../common/Checkbox';
import { Copy, Check } from 'lucide-react';
import { api } from '../../api/client';

export interface CLIExportModalProps {
  isOpen: boolean;
  onClose: () => void;
  userApiKey: string;
  copiedKey: string | null;
  onCopy: (text: string, key: string) => void;
}

export const CLIExportModal: React.FC<CLIExportModalProps> = ({
  isOpen,
  onClose,
  userApiKey,
  copiedKey,
  onCopy,
}) => {
  const [exportScriptContent, setExportScriptContent] = useState('');
  const [exportScriptLoading, setExportScriptLoading] = useState(false);
  const [includeKeyInModal, setIncludeKeyInModal] = useState(true);

  useEffect(() => {
    if (!isOpen) return;

    let isMounted = true;
    setExportScriptLoading(true);

    api.cli
      .exportScript()
      .then((res) => {
        if (isMounted) setExportScriptContent(res.content);
      })
      .catch((err: unknown) => {
        if (isMounted) {
          const msg = err instanceof Error ? err.message : String(err);
          setExportScriptContent(`# Gagal memuat berkas: ${msg}`);
        }
      })
      .finally(() => {
        if (isMounted) setExportScriptLoading(false);
      });

    return () => {
      isMounted = false;
    };
  }, [isOpen]);

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

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Skrip Lingkungan Terpadu CLI Route-X"
      maxWidth="lg"
      footer={
        <div className="flex items-center justify-between w-full">
          <Button
            variant="secondary"
            onClick={() => onCopy(modalRenderedScript, 'modal-full-script')}
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
          <Button variant="ghost" onClick={onClose}>
            Tutup
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        <p className="text-xs text-text-secondary">
          Berkas ini tersimpan di <code>~/.routex/cli-env.sh</code> dan otomatis disinkronkan setiap
          kali Anda menekan tombol "Terapkan ke CLI".
        </p>

        {userApiKey && (
          <div className="flex items-center justify-between bg-bg-surface-2 px-3 py-2 rounded-inner border border-border text-xs">
            <span className="text-text-muted">Sertakan Kunci API Anda ke skrip ekspor:</span>
            <Checkbox
              checked={includeKeyInModal}
              onChange={(e) => setIncludeKeyInModal(e.target.checked)}
              label={
                <span className="text-white font-mono">
                  Inject OPENAI_API_KEY &amp; ANTHROPIC_API_KEY
                </span>
              }
            />
          </div>
        )}

        <div className="relative">
          <pre className="bg-bg-surface-2 p-4 rounded-inner border border-border text-xs font-mono text-white overflow-x-auto max-h-96">
            {exportScriptLoading ? 'Memuat skrip...' : modalRenderedScript}
          </pre>
        </div>

        <div className="bg-bg-surface-2 p-3 rounded-inner border border-border text-xs text-text-muted">
          <strong className="text-white block mb-1">Tips Integrasi Shell:</strong>
          Untuk memuat otomatis setiap membuka sesi terminal baru di mesin Anda:
          <code className="block mt-1 bg-bg-base p-1.5 rounded-inner font-mono text-accent">
            echo 'source ~/.routex/cli-env.sh' &gt;&gt; ~/.bashrc
          </code>
        </div>
      </div>
    </Modal>
  );
};
