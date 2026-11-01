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
import { isReservedSetting, labelSettingDikenal } from './utils';

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
    <div className="space-y-5">
      <div className="flex flex-col sm:flex-row sm:items-start md:items-center justify-between gap-4">
        <div className="flex items-start gap-3 min-w-0">
          <Sliders className="w-4 h-4 text-accent-text shrink-0 mt-1" />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-sm sm:text-base font-bold text-text-primary tracking-tight">
                Runtime Parameters
              </h2>
              <span className="inline-flex items-center text-[10px] font-semibold text-accent-text bg-accent/10 px-2 py-0.5 rounded-full border border-accent/20 whitespace-nowrap">
                Operasional Dinamis
              </span>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2 w-full sm:w-auto shrink-0 pt-1 sm:pt-0">
          <Button
            variant="secondary"
            size="sm"
            onClick={onRefresh}
            isLoading={isLoading}
            icon={<RefreshCw className="w-3.5 h-3.5" />}
            title="Segarkan Parameter Runtime"
            className="flex-1 sm:flex-initial justify-center"
          >
            Segarkan
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={onOpenCreateModal}
            icon={<Plus className="w-3.5 h-3.5" />}
            className="flex-1 sm:flex-initial justify-center"
          >
            Tambah Parameter
          </Button>
        </div>
      </div>

      {/* Custom Operasional Parameters */}
      {customSettings.length === 0 ? (
        <div className="py-8 text-center">
          <div className="max-w-md mx-auto space-y-3">
            <Sliders className="w-5 h-5 text-accent-text mx-auto" />
            <div>
              <h4 className="text-sm font-bold text-text-primary">Belum Ada Parameter Runtime Kustom</h4>
              <p className="text-xs text-text-muted mt-1 leading-relaxed">
                Gunakan parameter runtime untuk menyimpan nilai operasional dinamis (seperti flag fitur, batas jeda timeout, atau konfigurasi eksperimental).
              </p>
            </div>
            <Button
              variant="secondary"
              size="sm"
              onClick={onOpenCreateModal}
              icon={<Plus className="w-4 h-4" />}
              className="text-xs mx-auto"
            >
              Tambah Parameter
            </Button>
          </div>
        </div>
      ) : (
        <div className="divide-y divide-border/60 border-t border-border/60">
          {customSettings.map((s) => (
            <div key={s.key} className="py-5 flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between gap-2 mb-2">
                  <div className="flex items-center gap-2.5 min-w-0">
                    <Sliders className="w-3.5 h-3.5 text-accent-text flex-shrink-0 mt-0.5" />
                    <div className="min-w-0">
                      {labelSettingDikenal(s.key) && (
                        <span className="text-xs font-semibold text-text-primary block truncate">
                          {labelSettingDikenal(s.key)}
                        </span>
                      )}
                      <h4 className="text-sm font-bold text-text-primary font-mono truncate">{s.key}</h4>
                      <span className="text-[11px] text-text-muted block truncate">
                        {s.description || 'Parameter runtime kustom'}
                      </span>
                    </div>
                  </div>
                  <Badge variant="neutral">Kustom</Badge>
                </div>

                <div className="mt-3">
                  <textarea
                    rows={3}
                    value={editValues[s.key] ?? ''}
                    onChange={(e) => onEditChange(s.key, e.target.value)}
                    className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-text-primary font-mono focus:outline-none focus:border-accent"
                    placeholder="Nilai parameter (teks biasa atau JSON)..."
                  />
                </div>
              </div>

              <div className="mt-3 flex items-center justify-between">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => onDeleteSetting(s.key)}
                  icon={<Trash2 className="w-3.5 h-3.5 text-red-400" />}
                  className="text-red-400 hover:text-red-300"
                >
                  Hapus
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => onSaveSetting(s.key)}
                  isLoading={savingKey === s.key}
                  icon={<Save className="w-3.5 h-3.5" />}
                >
                  Simpan Perubahan
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Managed Subsystem Settings (CLI & System) */}
      {managedSettings.length > 0 && (
        <div className="pt-6 border-t border-border/60 space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h4 className="text-sm font-bold text-text-primary flex items-center gap-2">
                <Lock className="w-4 h-4 text-sky-400" />
                Setelan Terkelola Subsistem (Read-Only)
              </h4>
            </div>
            <Badge variant="info">
              {managedSettings.length} Terkelola
            </Badge>
          </div>

          <div className="divide-y divide-border/60">
            {managedSettings.map((s) => {
              const isCLI = s.key.startsWith('cli:config:');
              return (
                <div key={s.key} className="py-4 flex flex-col justify-between space-y-3">
                  <div>
                    <div className="flex items-start justify-between gap-2 mb-1.5">
                      <h5 className="text-xs font-bold text-text-primary font-mono truncate">{s.key}</h5>
                      <Badge variant={isCLI ? 'neutral' : 'info'}>
                        {isCLI ? 'Integrasi CLI' : 'Sistem'}
                      </Badge>
                    </div>
                    <p className="text-[11px] text-text-muted mb-2">
                      {s.description || 'Konfigurasi subsistem internal'}
                    </p>

                    <pre className="p-2.5 rounded-lg bg-bg-base/80 text-[10px] font-mono text-emerald-400/90 overflow-x-auto max-h-32 leading-relaxed">
                      {editValues[s.key] || '(kosong)'}
                    </pre>
                  </div>

                  <div className="pt-2 flex items-center justify-end">
                    {isCLI ? (
                      <a href="#/cli">
                        <Button variant="secondary" size="sm" icon={<ExternalLink className="w-3.5 h-3.5" />}>
                          Buka Integrasi CLI
                        </Button>
                      </a>
                    ) : (
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
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
