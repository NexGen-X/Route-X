import React, { useState } from 'react';
import { KeyRound, Shield, Clock, DollarSign, Activity } from 'lucide-react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import type { ExpirationPreset } from './types';

export interface CreateAPIKeyDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (formData: {
    name: string;
    rpm_limit: number;
    tpm_limit: number;
    monthlyBudgetUsd: string;
    expiresAt?: string;
  }) => Promise<void>;
  isSubmitting?: boolean;
}

export const CreateAPIKeyDrawer: React.FC<CreateAPIKeyDrawerProps> = ({
  isOpen,
  onClose,
  onSubmit,
  isSubmitting = false,
}) => {
  const [name, setName] = useState('');
  const [rpmLimit, setRpmLimit] = useState(120);
  const [tpmLimit, setTpmLimit] = useState(100000);
  const [monthlyBudgetUsd, setMonthlyBudgetUsd] = useState('');
  const [expirationPreset, setExpirationPreset] = useState<ExpirationPreset>('never');
  const [customDays, setCustomDays] = useState(30);

  const calculateExpiresAt = (preset: ExpirationPreset, days: number): string | undefined => {
    if (preset === 'never') return undefined;
    const now = new Date();
    let addDays = 30;
    if (preset === '30d') addDays = 30;
    else if (preset === '60d') addDays = 60;
    else if (preset === '90d') addDays = 90;
    else if (preset === '1y') addDays = 365;
    else if (preset === 'custom') addDays = Math.max(1, days);

    now.setDate(now.getDate() + addDays);
    return now.toISOString();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const expiresAt = calculateExpiresAt(expirationPreset, customDays);
    await onSubmit({
      name: name.trim(),
      rpm_limit: rpmLimit,
      tpm_limit: tpmLimit,
      monthlyBudgetUsd: monthlyBudgetUsd.trim(),
      expiresAt,
    });
  };

  const handleResetAndClose = () => {
    setName('');
    setRpmLimit(120);
    setTpmLimit(100000);
    setMonthlyBudgetUsd('');
    setExpirationPreset('never');
    onClose();
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={handleResetAndClose}
      title={
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-lg bg-accent/15 text-accent flex items-center justify-center">
            <KeyRound className="w-4 h-4" aria-hidden="true" />
          </div>
          <span>Buat Kunci API Klien Baru</span>
        </div>
      }
      subtitle="Terbitkan token autentikasi (sk_live_...) untuk editor kode, agen AI, atau layanan backend."
      maxWidth="lg"
      footer={
        <div className="flex items-center justify-end gap-2.5 w-full">
          <Button
            variant="ghost"
            onClick={handleResetAndClose}
            className="min-h-[44px] px-4"
          >
            Batal
          </Button>
          <Button
            variant="primary"
            type="submit"
            form="create-api-key-form"
            isLoading={isSubmitting}
            className="min-h-[44px] px-5 font-semibold"
          >
            Hasilkan Kunci API
          </Button>
        </div>
      }
    >
      <form
        id="create-api-key-form"
        noValidate
        onSubmit={handleSubmit}
        className="space-y-5 text-xs text-text-primary"
      >
        {/* Nama Kunci */}
        <div className="space-y-1.5">
          <label htmlFor="api-key-name-drawer" className="block text-xs font-semibold text-white">
            Nama Kunci Klien *
          </label>
          <input
            id="api-key-name-drawer"
            name="name"
            type="text"
            required
            autoFocus
            placeholder="Contoh: Cursor IDE, Backend Production, OpenWebUI"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-white placeholder:text-text-muted focus:outline-none focus:border-accent font-sans"
          />
          <p className="text-[11px] text-text-muted">
            Gunakan nama deskriptif untuk memudahkan pelacakan metrik dan rotasi.
          </p>
        </div>

        {/* Masa Berlaku (Expiration) */}
        <div className="space-y-2">
          <label className="block text-xs font-semibold text-white flex items-center gap-1.5">
            <Clock className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
            Masa Berlaku Kunci (Expiration Badge)
          </label>
          <div className="grid grid-cols-3 sm:grid-cols-6 gap-1.5">
            {[
              { id: 'never', label: 'Selamanya' },
              { id: '30d', label: '30 Hari' },
              { id: '60d', label: '60 Hari' },
              { id: '90d', label: '90 Hari' },
              { id: '1y', label: '1 Tahun' },
              { id: 'custom', label: 'Kustom' },
            ].map((preset) => (
              <button
                key={preset.id}
                type="button"
                onClick={() => setExpirationPreset(preset.id as ExpirationPreset)}
                className={`py-2 px-1 text-[11px] font-medium rounded-nav transition-all cursor-pointer border text-center ${
                  expirationPreset === preset.id
                    ? 'bg-accent/15 border-accent text-accent font-semibold shadow-sm'
                    : 'bg-bg-surface-2 border-border text-text-secondary hover:text-white hover:bg-bg-surface-3'
                }`}
              >
                {preset.label}
              </button>
            ))}
          </div>

          {expirationPreset === 'custom' && (
            <div className="pt-2 flex items-center gap-2">
              <input
                type="number"
                min="1"
                max="730"
                value={customDays}
                onChange={(e) => setCustomDays(Number(e.target.value))}
                className="w-24 px-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
              />
              <span className="text-xs text-text-secondary">hari dari sekarang</span>
            </div>
          )}
        </div>

        {/* Batas Trafik & Laju (RPM & TPM) */}
        <div className="p-3.5 rounded-xl bg-bg-surface-2 border border-border space-y-3">
          <div className="flex items-center gap-2 text-white font-semibold text-xs">
            <Activity className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
            Batas Kecepatan Kunci (Rate Limiting)
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label htmlFor="key-rpm" className="block text-[11px] text-text-muted mb-1">
                Batas Permintaan (RPM)
              </label>
              <input
                id="key-rpm"
                type="number"
                min="0"
                value={rpmLimit}
                onChange={(e) => setRpmLimit(Number(e.target.value))}
                className="w-full px-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono"
              />
              <span className="text-[10px] text-text-muted block mt-0.5">0 = Tanpa Batas</span>
            </div>

            <div>
              <label htmlFor="key-tpm" className="block text-[11px] text-text-muted mb-1">
                Batas Token (TPM)
              </label>
              <input
                id="key-tpm"
                type="number"
                min="0"
                step="1000"
                value={tpmLimit}
                onChange={(e) => setTpmLimit(Number(e.target.value))}
                className="w-full px-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono"
              />
              <span className="text-[10px] text-text-muted block mt-0.5">0 = Tanpa Batas</span>
            </div>
          </div>
        </div>

        {/* Anggaran Bulanan USD */}
        <div className="space-y-1.5">
          <label htmlFor="monthly-budget-usd" className="block text-xs font-semibold text-white flex items-center gap-1.5">
            <DollarSign className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
            Batas Anggaran Bulanan (USD, Opsional)
          </label>
          <div className="relative">
            <span className="absolute left-3 top-2.5 text-text-muted font-mono">$</span>
            <input
              id="monthly-budget-usd"
              name="monthlyBudgetUsd"
              type="number"
              step="0.01"
              min="0"
              placeholder="Contoh: 50.00"
              value={monthlyBudgetUsd}
              onChange={(e) => setMonthlyBudgetUsd(e.target.value)}
              className="w-full pl-7 pr-3 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-white font-mono placeholder:text-text-muted focus:outline-none focus:border-accent"
            />
          </div>
          <p className="text-[11px] text-text-muted">
            Otomatis mendaftarkan alokasi anggaran dengan hard-stop 80% notifikasi peringatan.
          </p>
        </div>

        {/* Informasi Keamanan & Best Practice */}
        <div className="p-3.5 rounded-xl bg-accent/5 border border-accent/20 text-xs text-text-secondary space-y-1.5">
          <div className="flex items-center gap-1.5 text-accent font-semibold">
            <Shield className="w-3.5 h-3.5" aria-hidden="true" />
            Standar Keamanan Route-X
          </div>
          <p className="text-[11px] leading-relaxed">
            Kunci API akan diawali prefix <code className="text-white font-mono">sk_live_...</code> dan disimpan menggunakan hashing HMAC-SHA256. Nilai mentah hanya akan diperlihatkan satu kali setelah proses pembuatan berhasil.
          </p>
        </div>
      </form>
    </Drawer>
  );
};
