import React, { useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { Button } from '../components/common/Button';
import { ApiError } from '../api/client';
import { ShieldCheck, AlertCircle, ArrowRight, Eye, EyeOff } from 'lucide-react';

export const Login: React.FC = () => {
  const { login } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [keepSignedIn, setKeepSignedIn] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

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

        {error && (
          <div id="login-error" role="alert" aria-live="assertive" className="mb-5 p-3.5 rounded-inner bg-status-error/10 border border-status-error/20 flex items-start gap-2.5 text-status-error text-xs">
            <AlertCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
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
              placeholder="admin@routex.internal"
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
            <label className="flex items-center gap-2 text-text-secondary cursor-pointer">
              <input
                type="checkbox"
                checked={keepSignedIn}
                onChange={(e) => setKeepSignedIn(e.target.checked)}
                className="w-4 h-4 rounded bg-bg-surface-2 border-border text-accent focus:ring-0 cursor-pointer"
              />
              <span>Ingat saya selama 30 hari</span>
            </label>
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
