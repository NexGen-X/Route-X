import React from 'react';
import { Key, Eye, EyeOff } from 'lucide-react';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import type { APIKey } from '../../types';
import { useToast } from '../../context/ToastContext';

export interface CLIApiKeyToolbarProps {
  userApiKey: string;
  setUserApiKey: (val: string) => void;
  showGlobalKey: boolean;
  setShowGlobalKey: (show: boolean) => void;
  registeredKeys: APIKey[];
}

export const CLIApiKeyToolbar: React.FC<CLIApiKeyToolbarProps> = ({
  userApiKey,
  setUserApiKey,
  showGlobalKey,
  setShowGlobalKey,
  registeredKeys,
}) => {
  const { toast } = useToast();

  const handleKeyChange = (val: string) => {
    setUserApiKey(val);
    try {
      sessionStorage.setItem('routex_cli_apikey', val);
    } catch {}
  };

  const handleClearKey = () => {
    setUserApiKey('');
    try {
      sessionStorage.removeItem('routex_cli_apikey');
    } catch {}
  };

  return (
    <div className="bg-bg-surface p-5 rounded-box border border-border space-y-3">
      <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-accent/10 text-accent flex-shrink-0">
            <Key className="w-4 h-4" />
          </div>
          <div>
            <h4 className="text-xs font-bold text-white flex items-center gap-2">
              Kunci API Route-X untuk CLI
              <Badge variant={userApiKey ? 'lime' : 'neutral'}>
                {userApiKey ? 'Kunci Terpasang' : 'Belum Diisi'}
              </Badge>
            </h4>
            <p className="text-[11px] text-text-muted">
              Kunci ini digunakan saat melakukan uji sambungan dan disematkan ke snippet shell agar
              terbebas dari error 401.
            </p>
            <p className="text-[11px] text-text-muted">
              Kunci hanya disimpan untuk sesi tab ini (sessionStorage) dan terhapus otomatis saat
              tab/browser ditutup.
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 w-full md:w-auto">
          <div className="relative flex-1 md:w-80">
            <input
              type={showGlobalKey ? 'text' : 'password'}
              placeholder="sk_live_..."
              value={userApiKey}
              onChange={(e) => handleKeyChange(e.target.value.trim())}
              className="w-full bg-bg-surface-2 border border-border rounded-md px-3 py-1.5 text-xs text-white placeholder:text-text-muted focus:border-accent focus:outline-none font-mono pr-8"
            />
            <button
              type="button"
              onClick={() => setShowGlobalKey(!showGlobalKey)}
              className="absolute right-2 top-2 text-text-muted hover:text-white"
              title={showGlobalKey ? 'Sembunyikan' : 'Tampilkan'}
            >
              {showGlobalKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
            </button>
          </div>
          {userApiKey && (
            <Button
              variant="ghost"
              size="sm"
              onClick={handleClearKey}
              className="text-xs min-h-[36px] text-text-muted hover:text-red-400"
              title="Kosongkan kunci tersimpan"
            >
              Hapus
            </Button>
          )}
        </div>
      </div>

      {/* Pilihan Cepat Kunci Terdaftar di Database */}
      {registeredKeys.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-border text-[11px]">
          <span className="text-text-muted flex items-center gap-1 font-medium">
            <Key className="w-3 h-3 text-accent" /> Kunci Terdaftar di Sistem:
          </span>
          {registeredKeys.map((k) => (
            <button
              key={k.id}
              type="button"
              onClick={() => {
                if (k.raw_key) {
                  handleKeyChange(k.raw_key);
                  toast.success(`Kunci "${k.name}" dipilih dan diterapkan.`);
                } else {
                  toast.info(
                    `Kunci "${k.name}" berstatus aktif (${k.masked_key}). Jika Anda memiliki salinan raw key, tempelkan di kotak input.`,
                  );
                }
              }}
              className="px-2.5 py-0.5 rounded text-[11px] font-mono bg-bg-surface-2 hover:bg-bg-surface-3 text-text-secondary hover:text-white border border-border transition-colors flex items-center gap-1"
              title={`Gunakan kunci ${k.name}`}
            >
              <span className="text-white font-medium">{k.name}</span>
              <span className="text-text-muted">({k.masked_key})</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
};
