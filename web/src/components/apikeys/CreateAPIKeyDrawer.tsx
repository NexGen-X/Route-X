import React, { useState } from 'react';
import { KeyRound, Shield, Clock, DollarSign, Activity } from 'lucide-react';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { InputBilangan } from '../common/InputBilangan';
import { useToast } from '../../context/ToastContext';
import { parseJumlahUsd } from '../../utils/money';
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
  const { toast } = useToast();
  const [name, setName] = useState('');
  // RPM/TPM dipegang sebagai TEKS mentah (audit3 V-FF-06): Number(teks)
  // versi lama mengubah "1.5" menjadi float yang ditolak server dengan
  // pesan buram, dan teks/kosong menjadi NaN/0 senyap. Kini keduanya
  // divalidasi sebagai bilangan bulat >= 0 saat submit dengan pesan jelas.
  const [rpmTeks, setRpmTeks] = useState('120');
  const [tpmTeks, setTpmTeks] = useState('100000');
  const [galatBatas, setGalatBatas] = useState<string | null>(null);
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
    if (!name.trim()) {
      toast.error('Nama kunci wajib diisi.');
      return;
    }
    // Validasi batas laju (V-FF-06): hanya bilangan bulat >= 0. Pecahan
    // seperti 1.5 dulu terkirim sebagai float dan ditolak server; kini
    // berhenti di sini dengan penjelasan.
    const rpmBersih = rpmTeks.trim();
    const tpmBersih = tpmTeks.trim();
    if (!/^\d+$/.test(rpmBersih) || !/^\d+$/.test(tpmBersih)) {
      const pesan =
        'Batas RPM dan TPM harus bilangan bulat 0 atau lebih (0 = tanpa batas) — pecahan, teks, dan kolom kosong tidak diterima.';
      setGalatBatas(pesan);
      toast.error(pesan, 'Batas Kecepatan Tidak Valid');
      return;
    }
    setGalatBatas(null);
    // Anggaran divalidasi di sini juga (V-FF-07) agar format koma desimal
    // "10,5" tidak lolos menjadi NaN di halaman: halaman tetap memagarinya
    // sekali lagi sebelum kunci dibuat.
    const anggaranTeks = monthlyBudgetUsd.trim();
    if (anggaranTeks !== '') {
      const hasil = parseJumlahUsd(anggaranTeks);
      if (!hasil.ok) {
        toast.error(hasil.pesan, 'Anggaran Tidak Valid');
        return;
      }
    }
    const expiresAt = calculateExpiresAt(expirationPreset, customDays);
    await onSubmit({
      name: name.trim(),
      rpm_limit: Number(rpmBersih),
      tpm_limit: Number(tpmBersih),
      monthlyBudgetUsd: anggaranTeks,
      expiresAt,
    });
  };

  const handleResetAndClose = () => {
    setName('');
    setRpmTeks('120');
    setTpmTeks('100000');
    setGalatBatas(null);
    setMonthlyBudgetUsd('');
    setExpirationPreset('never');
    setCustomDays(30);
    onClose();
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={handleResetAndClose}
      title={
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-lg bg-accent/15 text-accent-text flex items-center justify-center">
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
            <Clock className="w-3.5 h-3.5 text-accent-text" aria-hidden="true" />
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
                    ? 'bg-accent/15 border-accent text-accent-text font-semibold shadow-sm'
                    : 'bg-bg-surface-2 border-border text-text-secondary hover:text-white hover:bg-bg-surface-3'
                }`}
              >
                {preset.label}
              </button>
            ))}
          </div>

          {expirationPreset === 'custom' && (
            <div className="pt-2 flex items-start gap-2">
              {/* InputBilangan menegakkan rentang 1–730 (V-FF-17): atribut
                  max saja dulu tidak mencegah nilai seperti 99999 tersimpan
                  sebagai tanggal kedaluwarsa tahun 2300. */}
              <div className="w-28">
                <InputBilangan
                  ariaLabel="Hari kedaluwarsa kustom"
                  value={customDays}
                  onChange={setCustomDays}
                  min={1}
                  max={730}
                  className="w-full px-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-white font-mono"
                />
              </div>
              <span className="text-xs text-text-secondary pt-2">hari dari sekarang (maks 730)</span>
            </div>
          )}
        </div>

        {/* Batas Trafik & Laju (RPM & TPM) */}
        <div className="p-3.5 rounded-xl bg-bg-surface-2 border border-border space-y-3">
          <div className="flex items-center gap-2 text-white font-semibold text-xs">
            <Activity className="w-3.5 h-3.5 text-accent-text" aria-hidden="true" />
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
                value={rpmTeks}
                onChange={(e) => setRpmTeks(e.target.value)}
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
                value={tpmTeks}
                onChange={(e) => setTpmTeks(e.target.value)}
                className="w-full px-3 py-2 bg-bg-surface border border-border rounded-nav text-white font-mono"
              />
              <span className="text-[10px] text-text-muted block mt-0.5">0 = Tanpa Batas</span>
            </div>
          </div>
          {galatBatas && (
            <p role="alert" className="text-[11px] text-red-400">
              {galatBatas}
            </p>
          )}
        </div>

        {/* Anggaran Bulanan USD */}
        <div className="space-y-1.5">
          <label htmlFor="monthly-budget-usd" className="block text-xs font-semibold text-white flex items-center gap-1.5">
            <DollarSign className="w-3.5 h-3.5 text-accent-text" aria-hidden="true" />
            Batas Anggaran Bulanan (USD, Opsional)
          </label>
          <div className="relative">
            <span className="absolute left-3 top-2.5 text-text-muted font-mono">$</span>
            <input
              id="monthly-budget-usd"
              name="monthlyBudgetUsd"
              type="text"
              inputMode="decimal"
              placeholder="Contoh: 50.00 atau 50,00"
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
          <div className="flex items-center gap-1.5 text-accent-text font-semibold">
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
