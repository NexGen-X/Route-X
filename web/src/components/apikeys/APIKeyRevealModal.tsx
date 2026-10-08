import React, { useState } from 'react';
import { Copy, Check, AlertTriangle } from 'lucide-react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';
import { useToast } from '../../context/ToastContext';
import { copyTextToClipboard } from '../../utils/clipboard';
import { getErrorMessage } from '../../utils/error';

export interface APIKeyRevealModalProps {
  rawKey: string | null;
  onClose: () => void;
}

export const APIKeyRevealModal: React.FC<APIKeyRevealModalProps> = ({
  rawKey,
  onClose,
}) => {
  const { toast } = useToast();
  const [copied, setCopied] = useState(false);

  if (!rawKey) return null;

  const handleCopy = async () => {
    try {
      await copyTextToClipboard(rawKey);
      setCopied(true);
      toast.success('Kunci API disalin ke clipboard.');
      setTimeout(() => setCopied(false), 2000);
    } catch (err: unknown) {
      toast.error('Gagal menyalin kunci API: ' + getErrorMessage(err));
    }
  };

  return (
    <Modal
      isOpen={Boolean(rawKey)}
      onClose={onClose}
      title="Kunci API Berhasil Dibuat"
      footer={
        <Button
          variant="primary"
          size="md"
          onClick={onClose}
          className="w-full sm:w-auto min-h-[44px] px-6 font-semibold"
        >
          Saya Sudah Menyimpan Kunci Ini
        </Button>
      }
    >
      <div className="space-y-4">
        <div className="p-3.5 bg-bg-surface-2 rounded-inner border border-accent/40 shadow-glow-subtle flex items-center justify-between gap-3">
          <span className="font-mono text-xs text-accent break-all select-all font-semibold tracking-wide">
            {rawKey}
          </span>
          <Tooltip content={copied ? 'Tersalin ke clipboard!' : 'Salin Kunci API'} position="left">
            <button
              type="button"
              onClick={handleCopy}
              aria-label="Salin kunci API ke clipboard"
              className="min-h-[44px] min-w-[44px] p-2 text-text-muted hover:text-accent rounded-nav flex-shrink-0 cursor-pointer flex items-center justify-center focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
            >
              {copied ? (
                <Check className="w-5 h-5 text-accent" aria-hidden="true" />
              ) : (
                <Copy className="w-5 h-5" aria-hidden="true" />
              )}
            </button>
          </Tooltip>
        </div>

        <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/25 flex items-start gap-2.5 text-xs text-amber-200">
          <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" aria-hidden="true" />
          <p className="leading-relaxed">
            Pastikan Anda telah menyalin dan menyimpan token di atas di tempat yang aman (seperti environment variable). Token ini tidak akan dapat ditampilkan lagi setelah jendela ini ditutup.
          </p>
        </div>
      </div>
    </Modal>
  );
};
