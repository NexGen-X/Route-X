import React from 'react';

import { Button } from '../common/Button';
import { Badge } from '../common/Badge';
import {
  Sliders,
  Plus,
  RefreshCw,
  Save,
  Trash2,
  Lock,
  ExternalLink,
} from 'lucide-react';
import type { AdvancedSettingsTabProps } from './types';
import { isReservedSetting } from './utils';

export const AdvancedSettingsTab: React.FC<AdvancedSettingsTabProps> = ({
  settings,
  editValues,
  isLoading,
  savingKey,
  onRefresh,
  onOpenCreateModal,
  onEditChange,
  onSaveSetting,
  onDeleteSetting,
}) => {
  const customSettings = settings.filter((s) => !isReservedSetting(s.key));
  const managedSettings = settings.filter((s) => isReservedSetting(s.key));

  return (
    <div className="space-y-6">
      <div className="p-6 rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-center gap-4 min-w-0">
          <div className="w-12 h-12 rounded-2xl bg-gradient-to-br from-accent/20 to-accent/5 text-accent flex items-center justify-center shrink-0 border border-accent/10 shadow-inner">
            <Sliders className="w-6 h-6" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="text-base font-semibold text-white tracking-wide">
                Runtime Parameters
              </h2>
              <span className="inline-flex items-center text-[11px] font-semibold text-accent bg-accent/10 px-2.5 py-1 rounded-full border border-accent/20 whitespace-nowrap shadow-sm shadow-accent/5">
                Operasional Dinamis
              </span>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-3 w-full sm:w-auto shrink-0 pt-2 sm:pt-0">
          <Button
            variant="secondary"
            size="sm"
            onClick={onRefresh}
            isLoading={isLoading}
            icon={<RefreshCw className="w-4 h-4" />}
            title="Segarkan Parameter Runtime"
            className="flex-1 sm:flex-initial justify-center bg-white/5 hover:bg-white/10 border-white/5 text-white shadow-sm"
          >
            Segarkan
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={onOpenCreateModal}
            icon={<Plus className="w-4 h-4" />}
            className="flex-1 sm:flex-initial justify-center bg-gradient-to-r from-accent to-accent/80 hover:from-accent hover:to-accent text-white shadow-lg shadow-accent/25 border-transparent"
          >
            Tambah
          </Button>
        </div>
      </div>

      {/* Custom Operasional Parameters */}
      {customSettings.length === 0 ? (
        <div className="py-12 px-6 text-center rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10">
          <div className="max-w-md mx-auto space-y-4">
            <div className="w-12 h-12 rounded-2xl bg-accent/10 border border-accent/20 text-accent flex items-center justify-center mx-auto shadow-inner">
              <Sliders className="w-6 h-6" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-white tracking-wide">Belum Ada Parameter Runtime Kustom</h4>
              <p className="text-xs text-text-muted mt-2 leading-relaxed">
                Gunakan parameter runtime untuk menyimpan nilai operasional dinamis (seperti flag fitur, batas jeda timeout, atau konfigurasi eksperimental).
              </p>
            </div>
            <Button
              variant="secondary"
              size="sm"
              onClick={onOpenCreateModal}
              icon={<Plus className="w-4 h-4" />}
              className="text-xs mx-auto mt-4 bg-white/5 hover:bg-white/10 border-white/10"
            >
              Tambah Parameter Pertama
            </Button>
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 md:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 lg:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
          {customSettings.map((s) => (
            <div key={s.key} className="p-5 flex flex-col justify-between rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10 transition-all hover:bg-bg-surface/60 group">
              <div>
                <div className="flex items-start justify-between gap-3 mb-4">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="w-9 h-9 rounded-xl bg-accent/10 text-accent flex items-center justify-center flex-shrink-0 border border-accent/20">
                      <Sliders className="w-4 h-4" />
                    </div>
                    <div className="min-w-0">
                      <h4 className="text-sm font-bold text-white font-mono truncate">{s.key}</h4>
                      <span className="text-[11px] text-text-muted block truncate mt-0.5">
                        {s.description || 'Parameter runtime kustom'}
                      </span>
                    </div>
                  </div>
                  <Badge variant="neutral" className="bg-white/5 border-white/10">Kustom</Badge>
                </div>

                <div className="mt-4 relative group-focus-within:z-10">
                  <div className="absolute inset-0 bg-gradient-to-b from-accent/10 to-accent/0 rounded-xl opacity-0 group-focus-within:opacity-100 transition-opacity duration-500 blur-sm pointer-events-none" />
                  <textarea
                    rows={3}
                    value={editValues[s.key] ?? ''}
                    onChange={(e) => onEditChange(s.key, e.target.value)}
                    className="relative w-full px-4 py-3 bg-bg-surface-2/50 backdrop-blur-xl border border-white/10 rounded-xl text-xs text-white font-mono focus:outline-none focus:border-accent/50 focus:bg-bg-surface-2/80 transition-all shadow-inner resize-none"
                    placeholder="Nilai parameter (teks biasa atau JSON)..."
                  />
                </div>
              </div>

              <div className="mt-5 flex items-center justify-between gap-3 pt-4 border-t border-white/5">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => onDeleteSetting(s.key)}
                  icon={<Trash2 className="w-4 h-4 text-red-400" />}
                  className="text-red-400 hover:text-red-300 hover:bg-red-500/10 border-transparent bg-transparent shadow-none px-2"
                >
                  <span className="sr-only">Hapus</span>
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => onSaveSetting(s.key)}
                  isLoading={savingKey === s.key}
                  icon={<Save className="w-4 h-4" />}
                  className="flex-1 justify-center bg-white/10 hover:bg-white/20 border-white/10 shadow-sm"
                >
                  Simpan
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Managed Subsystem Settings (CLI & System) */}
      {managedSettings.length > 0 && (
        <div className="pt-8 border-t border-white/10 space-y-5">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h4 className="text-sm font-semibold text-white flex items-center gap-2.5">
                <div className="w-8 h-8 rounded-lg bg-sky-500/10 text-sky-400 flex items-center justify-center border border-sky-500/20">
                  <Lock className="w-4 h-4" />
                </div>
                Setelan Terkelola Subsistem (Read-Only)
              </h4>
            </div>
            <Badge variant="info" className="w-fit bg-sky-500/10 border-sky-500/20 text-sky-400 shadow-sm shadow-sky-500/5">
              {managedSettings.length} Terkelola
            </Badge>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 md:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 lg:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
            {managedSettings.map((s) => {
              const isCLI = s.key.startsWith('cli:config:');
              return (
                <div
                  key={s.key}
                  className="p-5 rounded-2xl bg-bg-surface/30 backdrop-blur-md border border-white/5 flex flex-col justify-between space-y-4 shadow-lg shadow-black/5"
                >
                  <div>
                    <div className="flex items-start justify-between gap-3 mb-2.5">
                      <h5 className="text-xs font-bold text-white font-mono truncate">{s.key}</h5>
                      <Badge variant={isCLI ? 'neutral' : 'info'} className={isCLI ? 'bg-white/5 border-white/10' : 'bg-sky-500/10 border-sky-500/20 text-sky-400'}>
                        {isCLI ? 'CLI' : 'Sistem'}
                      </Badge>
                    </div>
                    <p className="text-[11px] text-text-muted mb-3 leading-relaxed">
                      {s.description || 'Konfigurasi subsistem internal'}
                    </p>

                    <div className="relative">
                      <div className="absolute inset-y-0 left-0 w-1 bg-gradient-to-b from-emerald-400/50 to-emerald-400/10 rounded-l-lg" />
                      <pre className="p-3 pl-4 rounded-lg bg-black/40 border border-white/5 text-[10px] font-mono text-emerald-400/90 overflow-x-auto max-h-32 leading-relaxed shadow-inner">
                        {editValues[s.key] || '(kosong)'}
                      </pre>
                    </div>
                  </div>

                  <div className="pt-4 flex items-center justify-end border-t border-white/5">
                    {isCLI ? (
                      <a href="#/cli" className="w-full">
                        <Button variant="secondary" size="sm" icon={<ExternalLink className="w-4 h-4" />} className="w-full justify-center bg-white/5 hover:bg-white/10 border-white/5">
                          Buka CLI
                        </Button>
                      </a>
                    ) : (
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
                        className="w-full justify-center bg-white/5 hover:bg-white/10 border-white/5"
                      >
                        Kelola di Atas
                      </Button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};
