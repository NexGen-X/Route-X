import React, { useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { Button } from '../components/common/Button';
import { ApiError } from '../api/client';
import { ShieldCheck, AlertCircle, ArrowRight } from 'lucide-react';

export const Login: React.FC = () => {
  const { login } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [keepSignedIn, setKeepSignedIn] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      await login(email, password, keepSignedIn);
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
      <div className="w-full max-w-md bg-bg-surface border border-border rounded-card shadow-2xl p-8 flex flex-col">
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
          <div className="mb-5 p-3.5 rounded-inner bg-status-error/10 border border-status-error/20 flex items-start gap-2.5 text-status-error text-xs">
            <AlertCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wider mb-1.5">
              Alamat Email
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              autoFocus
              placeholder="admin@routex.internal"
              className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wider mb-1.5">
              Kata Sandi
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              placeholder="••••••••••••"
              className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent transition-colors"
            />
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
