import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { Sliders, Save, RefreshCw } from 'lucide-react';

export const Settings: React.FC = () => {
  const [settings, setSettings] = useState<{ key: string; value: string; description?: string }[]>([]);
  const [editValues, setEditValues] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [savingKey, setSavingKey] = useState<string | null>(null);

  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const res = await api.system.settings();
      setSettings(res.items || []);
      const map: Record<string, string> = {};
      (res.items || []).forEach((s) => {
        map[s.key] = s.value;
      });
      setEditValues(map);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadSettings();
  }, []);

  const handleSave = async (key: string) => {
    setSavingKey(key);
    try {
      await api.system.updateSetting(key, editValues[key] || '');
      alert('Pengaturan berhasil diperbarui.');
      loadSettings();
    } catch (err) {
      alert('Gagal menyimpan pengaturan: ' + err);
    } finally {
      setSavingKey(null);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Runtime Settings</h2>
          <p className="text-xs text-text-secondary mt-1">
            Konfigurasi parameter operasional gateway yang dapat disesuaikan tanpa perlu me-restart biner.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={loadSettings} isLoading={isLoading}>
          <RefreshCw className="w-3.5 h-3.5" />
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {settings.map((s) => (
          <Card key={s.key} className="p-5 flex flex-col justify-between">
            <div>
              <div className="flex items-center gap-3 mb-2">
                <div className="w-8 h-8 rounded-full bg-accent/10 text-accent flex items-center justify-center">
                  <Sliders className="w-4 h-4" />
                </div>
                <div>
                  <h4 className="text-sm font-bold text-white font-mono">{s.key}</h4>
                  <span className="text-[11px] text-text-muted">{s.description || 'Pengaturan runtime'}</span>
                </div>
              </div>

              <div className="mt-4">
                <input
                  type="text"
                  value={editValues[s.key] ?? s.value}
                  onChange={(e) => setEditValues({ ...editValues, [s.key]: e.target.value })}
                  className="w-full px-3 py-2 bg-bg-surface-2 border border-border rounded-nav text-xs text-white font-mono"
                />
              </div>
            </div>

            <div className="mt-4 pt-3 border-t border-border flex justify-end">
              <Button
                variant="primary"
                size="sm"
                onClick={() => handleSave(s.key)}
                isLoading={savingKey === s.key}
                icon={<Save className="w-3.5 h-3.5" />}
              >
                Simpan Perubahan
              </Button>
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
};
