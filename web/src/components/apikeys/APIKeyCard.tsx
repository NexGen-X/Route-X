import React, { useState } from 'react';
import {
  KeyRound,
  RotateCw,
  Trash2,
  Copy,
  Check,
  Eye,
  EyeOff,
  ShieldAlert,
  Calendar,
  Layers,
  Activity,
  Sliders,
} from 'lucide-react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';
import { useToast } from '../../context/ToastContext';
import { copyTextToClipboard } from '../../utils/clipboard';
import { getErrorMessage } from '../../utils/error';
import type { APIKeyCardProps } from './types';

export const APIKeyCard: React.FC<APIKeyCardProps> = ({
  apiKey,
  onOpenAllowed,
  onRotate,
  onRevoke,
}) => {
  const { toast } = useToast();
  const [copied, setCopied] = useState(false);
  const [isRevealed, setIsRevealed] = useState(false);

  const displayKey = isRevealed && apiKey.raw_key ? apiKey.raw_key : apiKey.masked_key;

  const handleCopy = async () => {
    const textToCopy = apiKey.raw_key || apiKey.masked_key;
    try {
      await copyTextToClipboard(textToCopy);
      setCopied(true);
      toast.success('Kunci API berhasil disalin ke clipboard.');
      setTimeout(() => setCopied(false), 2000);
    } catch (err: unknown) {
      toast.error('Gagal menyalin kunci API: ' + getErrorMessage(err));
    }
  };

  // Expiration calculation
  const getExpirationBadge = () => {
    if (!apiKey.expires_at) {
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-border/40 text-text-muted border border-border">
          <Calendar className="w-3 h-3 text-text-muted" aria-hidden="true" />
          Tanpa Kedaluwarsa
        </span>
      );
    }

    const expiryDate = new Date(apiKey.expires_at);
    const now = new Date();
    const diffMs = expiryDate.getTime() - now.getTime();
    const diffDays = Math.ceil(diffMs / (1000 * 60 * 60 * 24));

    if (diffMs <= 0) {
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-rose-500/10 text-rose-400 border border-rose-500/20">
          <ShieldAlert className="w-3 h-3 text-rose-400" aria-hidden="true" />
          Kedaluwarsa
        </span>
      );
    }

    if (diffDays <= 7) {
      return (
        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-amber-500/10 text-amber-400 border border-amber-500/20">
          <Calendar className="w-3 h-3 text-amber-400" aria-hidden="true" />
          Sisa {diffDays} hari
        </span>
      );
    }

    return (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
        <Calendar className="w-3 h-3 text-emerald-400" aria-hidden="true" />
        Hingga {expiryDate.toLocaleDateString()}
      </span>
    );
  };

  return (
    <Card className="p-5 flex flex-col justify-between border-border hover:border-border-hover bg-bg-surface transition-all duration-200 rounded-card shadow-sm">
      <div className="space-y-4">
        {/* Header: Nama Kunci, Status Badge & Expiration Badge */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-10 h-10 rounded-xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center shrink-0">
              <KeyRound className="w-5 h-5" aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <h4 className="text-sm font-bold text-white tracking-tight truncate">
                {apiKey.name}
              </h4>
              <div className="mt-1 flex items-center gap-1.5 flex-wrap">
                {getExpirationBadge()}
              </div>
            </div>
          </div>
          <Badge variant={apiKey.enabled ? 'success' : 'neutral'} className="shrink-0">
            {apiKey.enabled ? 'active' : 'disabled'}
          </Badge>
        </div>

        {/* Monospace Masked Key Box dengan Tombol Copy dan Reveal Instan */}
        <div className="bg-bg-surface-2 p-2.5 rounded-inner border border-border/80 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2 min-w-0 flex-1">
            <span className="px-1.5 py-0.5 rounded text-[9px] font-mono font-bold bg-accent/15 text-accent border border-accent/25 shrink-0">
              LIVE
            </span>
            <span className="font-mono text-xs text-text-primary truncate select-all">
              {displayKey}
            </span>
          </div>

          <div className="flex items-center gap-1 shrink-0">
            {apiKey.raw_key && (
              <Tooltip content={isRevealed ? 'Sembunyikan Kunci' : 'Tampilkan Kunci'} position="top">
                <button
                  type="button"
                  onClick={() => setIsRevealed(!isRevealed)}
                  aria-label={isRevealed ? 'Sembunyikan kunci API mentah' : 'Lihat kunci API mentah'}
                  className="min-h-[44px] min-w-[44px] p-2 text-text-muted hover:text-white rounded-nav transition-colors flex items-center justify-center cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
                >
                  {isRevealed ? (
                    <EyeOff className="w-4 h-4" aria-hidden="true" />
                  ) : (
                    <Eye className="w-4 h-4" aria-hidden="true" />
                  )}
                </button>
              </Tooltip>
            )}

            <Tooltip content={copied ? 'Tersalin ke clipboard!' : 'Salin Kunci API'} position="top">
              <button
                type="button"
                onClick={handleCopy}
                aria-label={`Salin kunci API untuk ${apiKey.name}`}
                className="min-h-[44px] min-w-[44px] p-2 text-text-muted hover:text-accent rounded-nav transition-colors flex items-center justify-center cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
              >
                {copied ? (
                  <Check className="w-4 h-4 text-accent" aria-hidden="true" />
                ) : (
                  <Copy className="w-4 h-4" aria-hidden="true" />
                )}
              </button>
            </Tooltip>
          </div>
        </div>

        {/* Metric Details: RPM/TPM, Last used, Allowed Scope */}
        <div className="space-y-2 text-xs divide-y divide-border/40">
          <div className="flex justify-between py-1 items-center">
            <span className="text-text-muted flex items-center gap-1.5">
              <Activity className="w-3.5 h-3.5 text-text-muted" aria-hidden="true" />
              Batas RPM / TPM
            </span>
            <span className="font-mono text-white font-medium">
              {apiKey.rate_limit_rpm ?? apiKey.rpm_limit ?? '∞'} RPM /{' '}
              {apiKey.rate_limit_tpm ?? apiKey.tpm_limit ?? '∞'} TPM
            </span>
          </div>

          <div className="flex justify-between py-1 items-center">
            <span className="text-text-muted flex items-center gap-1.5">
              <Calendar className="w-3.5 h-3.5 text-text-muted" aria-hidden="true" />
              Terakhir Digunakan
            </span>
            <span className="font-mono text-text-secondary">
              {apiKey.last_used_at ? new Date(apiKey.last_used_at).toLocaleDateString() : 'Belum pernah'}
            </span>
          </div>

          <div className="flex justify-between py-1 items-center">
            <span className="text-text-muted flex items-center gap-1.5">
              <Layers className="w-3.5 h-3.5 text-text-muted" aria-hidden="true" />
              Allowed Scope
            </span>
            <span className="font-mono text-xs">
              {apiKey.allowed_models && apiKey.allowed_models.length > 0 ? (
                <span className="text-amber-400 font-medium">
                  {apiKey.allowed_models.length} Model Dibatasi
                  {apiKey.allowed_providers && apiKey.allowed_providers.length > 0
                    ? ` (${apiKey.allowed_providers.length} Provider)`
                    : ''}
                </span>
              ) : (
                <span className="text-accent font-medium">Semua Model (Bebas)</span>
              )}
            </span>
          </div>
        </div>
      </div>

      {/* Action Buttons: Allowed Scope, Rotasi, Cabut */}
      <div className="mt-5 pt-3 border-t border-border flex items-center gap-2 flex-wrap justify-between">
        <Button
          variant="secondary"
          size="sm"
          onClick={() => onOpenAllowed(apiKey)}
          icon={<Sliders className="w-3.5 h-3.5" aria-hidden="true" />}
          className="w-full sm:w-auto min-h-[44px] sm:min-h-0 text-xs"
          aria-label={`Konfigurasi scope model untuk ${apiKey.name}`}
        >
          Allowed Scope
        </Button>
        <div className="flex items-center gap-2 w-full sm:w-auto justify-end">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => onRotate(apiKey.id)}
            icon={<RotateCw className="w-3.5 h-3.5" aria-hidden="true" />}
            className="flex-1 sm:flex-initial min-h-[44px] sm:min-h-0 text-xs"
            aria-label={`Putar rotasi kunci API untuk ${apiKey.name}`}
          >
            Rotasi
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => onRevoke(apiKey.id)}
            icon={<Trash2 className="w-3.5 h-3.5" aria-hidden="true" />}
            className="flex-1 sm:flex-initial min-h-[44px] sm:min-h-0 text-xs"
            aria-label={`Cabut kunci API ${apiKey.name} secara permanen`}
          >
            Cabut
          </Button>
        </div>
      </div>
    </Card>
  );
};
