import React, { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { Button } from '../components/common/Button';
import { Checkbox } from '../components/common/Checkbox';
import { api, ApiError } from '../api/client';
import type { SetupHintResponse } from '../types';
import { ShieldCheck, AlertCircle, ArrowRight, Eye, EyeOff, Sparkles, Key, Check } from 'lucide-react';

export const Login: React.FC = () => {
  const { login } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [keepSignedIn, setKeepSignedIn] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [setupHint, setSetupHint] = useState<SetupHintResponse | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let mounted = true;
    api.auth.setupHint().then((res) => {
      if (mounted && res && res.has_default_admin) {
        setSetupHint(res);
      }
    }).catch(() => {
      // Setup hint fail-safe jika endpoint tidak dapat dijangkau
    });
    return () => {
      mounted = false;
    };
  }, []);

  const handleAutofill = () => {
    if (setupHint?.default_email && setupHint?.default_password) {
      setEmail(setupHint.default_email);
      setPassword(setupHint.default_password);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      await login(email.trim(), password, keepSignedIn);
      window.location.hash = '#/';
    } catch (err: any) {
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
    <div className="min-h-screen flex items-center justify-center bg-bg-base p-4">
      <div className="w-full max-w-md bg-bg-surface border border-border rounded-card shadow-2xl p-6 sm:p-8 flex flex-col">
        <div className="flex items-center gap-3 mb-6">
          <div className="w-10 h-10 rounded-full bg-accent flex items-center justify-center font-extrabold text-black text-lg shadow-md shadow-accent/20">
            RX
          </div>
          <div>
            <h2 className="text-xl font-bold text-text-primary tracking-tight">Route-X Gateway</h2>
            <p className="text-xs text-text-secondary">Masuk ke Konsol Administrasi</p>
          </div>
        </div>

        {setupHint?.has_default_admin && (
          <div className="mb-6 p-4 rounded-xl bg-accent/5 border border-accent/20 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-accent font-semibold text-xs">
                <Sparkles className="w-4 h-4" />
                <span>Instalasi Baru (First-Run)</span>
              </div>
              <span className="text-[10px] uppercase font-mono px-2 py-0.5 rounded bg-accent/10 text-accent border border-accent/20">
                Setup Awal
              </span>
            </div>
            <p className="text-xs text-text-secondary leading-relaxed">
              Gunakan kredensial default di bawah ini untuk login pertama kali. Anda akan langsung diarahkan untuk membuat kata sandi baru.
            </p>
            <div className="bg-bg-base/80 rounded-lg p-2.5 border border-border space-y-1 font-mono text-xs">
              <div className="flex justify-between text-text-muted text-[11px]">
                <span>Email:</span>
                <span className="text-white select-all">{setupHint.default_email}</span>
              </div>
              <div className="flex justify-between text-text-muted text-[11px]">
                <span>Password:</span>
                <span className="text-white select-all">{setupHint.default_password}</span>
              </div>
            </div>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={handleAutofill}
              icon={copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Key className="w-3.5 h-3.5 text-accent" />}
              className="w-full text-xs font-semibold"
            >
              {copied ? 'Kredensial Terisi!' : 'Gunakan Kredensial Default (1-Klik)'}
            </Button>
          </div>
        )}

        {error && (
          <div id="login-error" role="alert" aria-live="assertive" className="mb-5 p-3.5 rounded-inner bg-status-error/10 border border-status-error/20 flex items-start gap-2.5 text-status-error text-xs">
            <AlertCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span>{error}</span>
          </div>
        )}

        <form noValidate onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="login-email" className="block text-xs font-medium text-text-secondary mb-1.5">
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
              className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
            />
          </div>

          <div>
            <label htmlFor="login-password" className="block text-xs font-medium text-text-secondary mb-1.5">
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
                className="w-full px-3.5 py-2.5 pr-10 bg-bg-surface-2 border border-border rounded-nav text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-text-muted hover:text-white p-2 min-w-[36px] min-h-[36px] flex items-center justify-center transition-colors cursor-pointer rounded-nav"
                aria-label={showPassword ? 'Sembunyikan kata sandi' : 'Lihat kata sandi'}
              >
                {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>

          <div className="flex items-center justify-between text-xs py-1">
            <Checkbox
              checked={keepSignedIn}
              onChange={(e) => setKeepSignedIn(e.target.checked)}
              label="Ingat saya selama 30 hari"
            />
          </div>

          <Button
            type="submit"
            variant="primary"
            size="lg"
            isLoading={isLoading}
            className="w-full mt-2"
            icon={<ArrowRight className="w-4 h-4" />}
          >
            Masuk ke Konsol
          </Button>
        </form>

        <div className="mt-8 pt-6 border-t border-border/60 flex items-center justify-between text-[11px] text-text-muted">
          <span className="flex items-center gap-1.5">
            <ShieldCheck className="w-3.5 h-3.5 text-accent" />
            Argon2id + Sesi Mandiri
          </span>
          <span className="font-mono">v1.0.0-prod</span>
        </div>
      </div>
    </div>
  );
};
