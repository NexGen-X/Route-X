import React, { useEffect, useState } from 'react';
import type { Provider, EgressPool } from '../../types';
import type { api as apiClient } from '../../api/client';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';
import { Globe, Trash2, Check, AlertTriangle } from 'lucide-react';

export interface SettingsTabProps {
  selectedProvider: Provider | null;
  isManualProvider: boolean;
  egressPools: EgressPool[];
  handleSelectProxyPreset: (poolId: string | null, poolName: string) => void | Promise<void>;
  handleDeleteProvider: (provider: Provider) => void | Promise<void>;
  loadData: () => Promise<void>;
  handleOpenDrawer: (prov: Provider, tab?: 'models' | 'credentials' | 'settings') => void;
  toast: {
    success: (msg: string) => void;
    error: (msg: string) => void;
    info?: (msg: string) => void;
  };
  api: typeof apiClient;
}

export const SettingsTab: React.FC<SettingsTabProps> = ({
  selectedProvider,
  isManualProvider,
  egressPools,
  handleSelectProxyPreset,
  handleDeleteProvider,
  loadData,
  handleOpenDrawer,
  toast,
  api,
}) => {
  const [configForm, setConfigForm] = useState({
    display_name: selectedProvider?.display_name || '',
    base_url: selectedProvider?.base_url || '',
    priority: selectedProvider?.priority || 100,
    weight: selectedProvider?.weight || 100,
  });
  const [isSavingConfig, setIsSavingConfig] = useState(false);

  useEffect(() => {
    if (selectedProvider) {
      setConfigForm({
        display_name: selectedProvider.display_name || '',
        base_url: selectedProvider.base_url || '',
        priority: selectedProvider.priority || 100,
        weight: selectedProvider.weight || 100,
      });
    }
  }, [selectedProvider]);

  const handleSaveConfig = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!selectedProvider) return;

    setIsSavingConfig(true);
    try {
      await api.providers.update(selectedProvider.id, configForm);
      toast.success('Konfigurasi teknis berhasil diperbarui');
      await loadData();
      handleOpenDrawer(
        { ...selectedProvider, ...configForm },
        'settings'
      );
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.error('Gagal menyimpan konfigurasi: ' + errMsg);
    } finally {
      setIsSavingConfig(false);
    }
  };

  return (
    <div className="space-y-4">
      {isManualProvider && (
        <form noValidate onSubmit={handleSaveConfig} className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Nama Tampilan</label>
              <input
                type="text"
                value={configForm.display_name}
                onChange={(e) => setConfigForm({ ...configForm, display_name: e.target.value })}
                className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Base URL Endpoint</label>
              <input
                type="text"
                value={configForm.base_url}
                onChange={(e) => setConfigForm({ ...configForm, base_url: e.target.value })}
                className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white font-mono outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Prioritas Routing</label>
              <input
                type="number"
                value={configForm.priority}
                onChange={(e) => setConfigForm({ ...configForm, priority: parseInt(e.target.value) || 100 })}
                className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white font-mono outline-none focus:border-accent"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">Bobot Load Balance</label>
              <input
                type="number"
                value={configForm.weight}
                onChange={(e) => setConfigForm({ ...configForm, weight: parseInt(e.target.value) || 100 })}
                className="w-full px-2.5 py-1.5 text-xs bg-bg-surface border border-border rounded-lg text-white font-mono outline-none focus:border-accent"
              />
            </div>
          </div>
          <div className="flex justify-end pt-1">
            <Button type="submit" variant="primary" size="sm" isLoading={isSavingConfig} icon={<Check className="w-3.5 h-3.5" />}>
              Simpan Konfigurasi
            </Button>
          </div>
        </form>
      )}

      <div>
        <label className="block text-xs font-medium text-text-secondary mb-2">Jalur Egress Outbound</label>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
          <div
            onClick={() => handleSelectProxyPreset(null, 'Direct Outbound (Tanpa Proxy)')}
            className={`px-2.5 py-2 rounded-lg border cursor-pointer transition-all flex items-center justify-between gap-2 ${
              !selectedProvider?.egress_pool_id
                ? 'border-accent bg-accent/10 shadow-sm'
                : 'border-border bg-bg-surface-2/40 hover:border-border/80'
            }`}
          >
            <span className="flex items-center gap-1.5 min-w-0">
              <Globe className="w-3.5 h-3.5 text-accent flex-shrink-0" />
              <span className="font-bold text-white truncate">Direct (Tanpa Proxy)</span>
            </span>
            {!selectedProvider?.egress_pool_id && (
              <span className="px-1.5 py-0.2 text-[10px] rounded bg-accent text-black font-bold flex-shrink-0">Aktif</span>
            )}
          </div>
          {egressPools.map((pool) => {
            const isSelected = selectedProvider?.egress_pool_id === pool.id;
            return (
              <div
                key={pool.id}
                onClick={() => handleSelectProxyPreset(pool.id, pool.name)}
                className={`px-2.5 py-2 rounded-lg border cursor-pointer transition-all flex items-center justify-between gap-2 ${
                  isSelected
                    ? 'border-accent bg-accent/10 shadow-sm'
                    : 'border-border bg-bg-surface-2/40 hover:border-border/80'
                }`}
              >
                <span className="flex items-center gap-1.5 min-w-0">
                  <Globe className="w-3.5 h-3.5 text-accent flex-shrink-0" />
                  <span className="font-bold text-white truncate">{pool.name}</span>
                </span>
                {isSelected && (
                  <span className="px-1.5 py-0.2 text-[10px] rounded bg-accent text-black font-bold flex-shrink-0">Aktif</span>
                )}
              </div>
            );
          })}
        </div>
      </div>

      <div className="pt-3 border-t border-border">
        <div className="px-2.5 py-2 rounded-lg border border-red-500/30 bg-red-500/10 flex items-center justify-between gap-2">
          <span className="text-[11px] text-red-400 font-bold flex items-center gap-1.5 min-w-0">
            <AlertTriangle className="w-3.5 h-3.5 text-red-400 flex-shrink-0" />
            <span className="truncate">Hapus · permanen, ikut key + model</span>
          </span>
          <Tooltip content="Hapus Provider Permanen" position="left">
            <button
              type="button"
              onClick={() => selectedProvider && handleDeleteProvider(selectedProvider)}
              aria-label="Hapus provider"
              className="p-1.5 rounded-md bg-red-500/15 text-red-400 border border-red-500/30 hover:bg-red-500/25 transition-colors cursor-pointer flex-shrink-0 focus:outline-none focus-visible:ring-1 focus-visible:ring-red-500"
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
          </Tooltip>
        </div>
      </div>
    </div>
  );
};
