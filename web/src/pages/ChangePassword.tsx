import React, { useState, useEffect, useRef } from 'react';
import { api, ApiError } from '../api/client';
import { useAuth } from '../context/AuthContext';
import { Button } from '../components/common/Button';
import { AlertCircle, KeyRound, CheckCircle2 } from 'lucide-react';

export const ChangePassword: React.FC = () => {
  const { refresh, logout } = useAuth();
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  // Timer redirect sukses; disimpan agar bisa dibatalkan bila user pergi duluan,
  // supaya tidak ada navigasi paksa setelah unmount.
  const redirectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (redirectTimerRef.current) {
        clearTimeout(redirectTimerRef.current);
        redirectTimerRef.current = null;
      }
    };
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (newPassword !== confirmPassword) {
      setError('Konfirmasi kata sandi baru tidak cocok.');
      return;
    }

    if (newPassword.length < 12) {
      setError('Kata sandi baru minimal 12 karakter.');
      return;
    }

    setIsLoading(true);
    try {
      await api.auth.changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });
      setSuccess(true);
      await refresh();
      if (redirectTimerRef.current) clearTimeout(redirectTimerRef.current);
      redirectTimerRef.current = setTimeout(() => {
        redirectTimerRef.current = null;
        window.location.hash = '#/';
      }, 1500);
    } catch (err: any) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError('Gagal memperbarui kata sandi.');
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-bg-base p-4">
      <div className="w-full max-w-md bg-bg-surface border border-border rounded-card shadow-2xl p-8 flex flex-col">
        <div className="flex items-center gap-3 mb-6">
          <div className="w-10 h-10 rounded-full bg-status-warn/20 text-status-warn flex items-center justify-center">
            <KeyRound className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-xl font-bold text-text-primary tracking-tight">Perbarui Kata Sandi</h2>
            <p className="text-xs text-text-secondary">Akun Anda memerlukan pergantian kata sandi pertama</p>
          </div>
        </div>

        {error && (
          <div className="mb-5 p-3.5 rounded-inner bg-status-error/10 border border-status-error/20 flex items-start gap-2.5 text-status-error text-xs">
            <AlertCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span>{error}</span>
          </div>
        )}

        {success && (
          <div className="mb-5 p-3.5 rounded-inner bg-status-success/10 border border-status-success/20 flex items-start gap-2.5 text-status-success text-xs">
            <CheckCircle2 className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <span>Kata sandi berhasil diperbarui! Mengalihkan...</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wider mb-1.5">
              Kata Sandi Saat Ini
            </label>
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-sm text-text-primary focus:outline-none focus:border-accent"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wider mb-1.5">
              Kata Sandi Baru (Minimal 12 Karakter)
            </label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-sm text-text-primary focus:outline-none focus:border-accent"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wider mb-1.5">
              Ulangi Kata Sandi Baru
            </label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              className="w-full px-3.5 py-2.5 bg-bg-surface-2 border border-border rounded-nav text-sm text-text-primary focus:outline-none focus:border-accent"
            />
          </div>

          <Button type="submit" variant="primary" size="lg" isLoading={isLoading} className="w-full mt-2">
            Simpan Kata Sandi Baru
          </Button>

          <Button type="button" variant="ghost" onClick={logout} className="w-full text-xs">
            Batalkan & Keluar
          </Button>
        </form>
      </div>
    </div>
  );
};
