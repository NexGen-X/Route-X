import React, { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { Button } from '../components/common/Button';
import { Checkbox } from '../components/common/Checkbox';
import { RouteXLogo } from '../components/common/RouteXLogo';
import { api, ApiError } from '../api/client';
import type { SetupHintResponse } from '../types';
import {
  ShieldCheck,
  AlertCircle,
  ArrowRight,
  Eye,
  EyeOff,
  Sparkles,
  Info,
  Copy,
  Check,
  Cpu,
} from 'lucide-react';
import pkg from '../../package.json';

export const Login: React.FC = () => {
  const { login } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [keepSignedIn, setKeepSignedIn] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [setupHint, setSetupHint] = useState<SetupHintResponse | null>(null);
  const [copiedEmail, setCopiedEmail] = useState(false);

  useEffect(() => {
    let mounted = true;
    api.auth
      .setupHint()
      .then((res) => {
        if (mounted && res && res.has_default_admin) {
          setSetupHint(res);
        }
      })
      .catch(() => {
        // Setup hint fail-safe jika endpoint tidak dapat dijangkau
      });
    return () => {
      mounted = false;
    };
  }, []);

  const handleCopyEmail = (adminEmail: string) => {
    setEmail(adminEmail);
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(adminEmail).catch(() => {});
      setCopiedEmail(true);
      setTimeout(() => setCopiedEmail(false), 2000);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      await login(email.trim(), password, keepSignedIn);
      window.location.hash = '#/';
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError('Gagal masuk ke sistem. Periksa kembali kredensial Anda.');
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <main
      role="main"
      className="min-h-screen relative flex items-center justify-center bg-bg-base overflow-hidden p-4 sm:p-6"
    >
      {/* Background Layer 1: High-Tech Infrastructure Mesh Grid */}
      <div
        className="absolute inset-0 bg-grid-tech opacity-40 pointer-events-none"
        aria-hidden="true"
      />

      {/* Background Layer 2: Ambient Glowing Mesh Aurora */}
      <div
        className="absolute -top-32 left-1/2 -translate-x-1/2 w-[900px] h-[500px] bg-aurora-radial pointer-events-none opacity-80"
        aria-hidden="true"
      />
      <div
        className="absolute -bottom-48 left-1/2 -translate-x-1/2 w-[700px] h-[400px] bg-radial-ambient pointer-events-none opacity-60"
        aria-hidden="true"
      />
      <div
        className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-radial-card pointer-events-none opacity-50 blur-3xl"
        aria-hidden="true"
      />

      {/* Central Login Container */}
      <div className="relative w-full max-w-[420px] z-10">
        {/* Minimalist Zinc Card */}
        <div className="relative rounded-xl bg-zinc-900/60 border border-zinc-800/80 shadow-lg p-6 sm:p-8 flex flex-col">
          {/* Header Brand */}
          <div className="flex items-center gap-3.5 mb-6">
            <RouteXLogo size={36} glow={false} className="group cursor-default" />
            <div className="flex flex-col">
              <div className="flex items-center gap-2">
                <h1 className="text-xl font-bold text-white tracking-tight leading-tight">
                  Route-X
                </h1>
                <span className="text-[10px] font-mono font-medium px-2 py-0.5 rounded-full bg-indigo-500/10 text-indigo-400 border border-indigo-500/30">
                  Gateway
                </span>
              </div>
              <p className="text-xs text-zinc-400 mt-0.5 flex items-center gap-1.5 font-mono">
                <Cpu className="w-3 h-3 text-cyan-400 shrink-0" aria-hidden="true" />
                <span>Next-Gen AI Routing Fabric</span>
              </p>
            </div>
          </div>

          {/* Kartu Setup Awal (Instalasi Baru) Berdesain Ringkas & Modern */}
          {setupHint?.has_default_admin && (
            <div className="mb-5 p-3.5 rounded-xl bg-bg-surface-1/90 border border-cyan-500/25 shadow-[0_0_20px_rgba(6,182,212,0.06)] space-y-2.5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-1.5 text-cyan-300 font-semibold text-xs">
                  <Sparkles className="w-3.5 h-3.5 text-cyan-400" aria-hidden="true" />
                  <span>Instalasi Baru</span>
                </div>
                <span className="text-[10px] uppercase font-mono font-semibold px-2 py-0.5 rounded-full bg-amber-500/10 text-amber-300 border border-amber-500/30 shadow-[0_0_10px_rgba(245,158,11,0.2)]">
                  Setup Awal
                </span>
              </div>

              <p className="text-[11px] text-text-secondary leading-relaxed">
                Akun admin pertama memakai kredensial awal. Masuk dengan email di bawah, lalu ambil kata sandi awal dari berkas <code className="px-1 py-0.5 rounded bg-bg-base/90 text-cyan-300 font-mono text-[10px]">.env</code> atau perintah <code className="px-1 py-0.5 rounded bg-bg-base/90 text-cyan-300 font-mono text-[10px]">routex-rotate</code>.
              </p>

              {/* 1-Klik Salin & Isi Email Admin */}
              <div className="flex items-center justify-between p-2 rounded-lg bg-bg-base/90 border border-white/[0.08] text-xs font-mono">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-text-muted text-[11px]">Email:</span>
                  <button
                    type="button"
                    onClick={() => setupHint.default_email && handleCopyEmail(setupHint.default_email)}
                    className="text-white hover:text-cyan-300 truncate transition-colors cursor-pointer select-all font-mono"
                    aria-label="Isi alamat email admin"
                  >
                    {setupHint.default_email ?? '-'}
                  </button>
                </div>
                {setupHint.default_email && (
                  <button
                    type="button"
                    onClick={() => handleCopyEmail(setupHint.default_email!)}
                    className="inline-flex items-center gap-1 px-2 py-1 rounded bg-bg-surface-2 hover:bg-bg-surface-3 border border-white/[0.08] text-[10px] text-cyan-400 hover:text-white transition-all active:scale-95 cursor-pointer shrink-0 ml-2"
                    aria-label="Salin dan isi email otomatis"
                  >
                    {copiedEmail ? (
                      <>
                        <Check className="w-3 h-3 text-emerald-400" aria-hidden="true" />
                        <span className="text-emerald-400">Tersalin</span>
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3" aria-hidden="true" />
                        <span>Salin &amp; Isi</span>
                      </>
                    )}
                  </button>
                )}
              </div>

              <div className="flex items-start gap-1.5 text-[10px] text-text-muted leading-tight pt-0.5">
                <Info className="w-3 h-3 shrink-0 mt-0.5 text-cyan-400" aria-hidden="true" />
                <span>Kata sandi tidak disajikan di antarmuka ini demi keamanan instance.</span>
              </div>
            </div>
          )}

          {/* Alert Error Box */}
          {error && (
            <div
              id="login-error"
              role="alert"
              aria-live="assertive"
              className="mb-5 p-3 rounded-xl bg-status-error/10 border border-status-error/25 flex items-start gap-2.5 text-status-error text-xs shadow-sm"
            >
              <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" aria-hidden="true" />
              <span className="leading-relaxed">{error}</span>
            </div>
          )}

          {/* Form Login */}
          <form noValidate onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="login-email"
                className="block text-xs font-medium text-text-secondary mb-1.5 tracking-wide"
              >
                Alamat Email
              </label>
              <input
                id="login-email"
                name="email"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                aria-describedby={error ? 'login-error' : undefined}
                placeholder="admin@routex.local"
                className="w-full min-h-[44px] px-3.5 py-2.5 bg-bg-surface-1/80 hover:bg-bg-surface-1 border border-white/[0.08] hover:border-white/[0.16] focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 rounded-xl text-sm text-text-primary placeholder:text-text-muted transition-all duration-150 outline-none"
              />
            </div>

            <div>
              <label
                htmlFor="login-password"
                className="block text-xs font-medium text-text-secondary mb-1.5 tracking-wide"
              >
                Kata Sandi
              </label>
              <div className="relative">
                <input
                  id="login-password"
                  name="password"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  aria-describedby={error ? 'login-error' : undefined}
                  placeholder="••••••••••••"
                  className="w-full min-h-[44px] px-3.5 py-2.5 pr-12 bg-bg-surface-1/80 hover:bg-bg-surface-1 border border-white/[0.08] hover:border-white/[0.16] focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 rounded-xl text-sm text-text-primary placeholder:text-text-muted transition-all duration-150 outline-none"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-1 top-1/2 -translate-y-1/2 text-text-muted hover:text-white p-2.5 min-w-[44px] min-h-[44px] flex items-center justify-center transition-colors cursor-pointer rounded-xl focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500/40"
                  aria-label={showPassword ? 'Sembunyikan kata sandi' : 'Lihat kata sandi'}
                >
                  {showPassword ? (
                    <EyeOff className="w-4 h-4" aria-hidden="true" />
                  ) : (
                    <Eye className="w-4 h-4" aria-hidden="true" />
                  )}
                </button>
              </div>
            </div>

            <div className="flex items-center justify-between text-xs py-0.5">
              <Checkbox
                checked={keepSignedIn}
                onChange={(e) => setKeepSignedIn(e.target.checked)}
                label="Ingat saya selama 30 hari"
              />
            </div>

            {/* Primary Electric Indigo Submit Button */}
            <Button
              type="submit"
              variant="primary"
              size="lg"
              isLoading={isLoading}
              className="w-full mt-2 min-h-[42px] font-medium"
              icon={<ArrowRight className="w-4 h-4 text-white" aria-hidden="true" />}
            >
              Masuk ke Konsol
            </Button>
          </form>

          {/* Futuristic Micro-Telemetry Footer */}
          <div className="mt-8 pt-5 border-t border-white/[0.06] flex items-center justify-between text-[11px] text-text-secondary">
            <div className="flex items-center gap-1.5 font-mono text-[10px]">
              <ShieldCheck className="w-3.5 h-3.5 text-cyan-400" aria-hidden="true" />
              <span>Argon2id Enforced</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="relative flex h-1.5 w-1.5" aria-hidden="true">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500" />
              </span>
              <span className="font-mono text-[10px] text-text-muted">v{pkg.version}</span>
            </div>
          </div>
        </div>
      </div>
    </main>
  );
};
