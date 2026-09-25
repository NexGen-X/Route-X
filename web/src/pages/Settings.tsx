import React, { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { DomainConfig, BackupStatusResponse } from '../types';
import { PageHeader } from '../components/common/PageHeader';
import { QueryError } from '../components/common/QueryError';
import { Globe, Sliders, Database } from 'lucide-react';
import { useToast } from '../context/ToastContext';
import {
  DomainTab,
  BackupTab,
  AdvancedSettingsTab,
  CreateSettingModal,
  RestoreConfirmModal,
  type SettingsTab,
  type SSLProviderMode,
  type RuntimeSettingItem,
  type DomainFeedback,
  type BackupFeedback,
  type NewSettingForm,
  formatSettingValue,
  isReservedSetting,
  getErrorMessage,
} from '../components/settings';

export const Settings: React.FC = () => {
  const { toast, confirmModal } = useToast();

  // Tab Navigasi: 'domain' | 'backup' | 'advanced'
  const [activeTab, setActiveTab] = useState<SettingsTab>('domain');

  // Runtime Settings state
  const [settings, setSettings] = useState<RuntimeSettingItem[]>([]);
  const [editValues, setEditValues] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [isCreateSettingOpen, setIsCreateSettingOpen] = useState(false);
  const [newSetting, setNewSetting] = useState<NewSettingForm>({ key: '', value: '', description: '' });
  const [isCreatingSetting, setIsCreatingSetting] = useState(false);

  // Domain & HTTPS state
  const [domainConfig, setDomainConfig] = useState<DomainConfig | null>(null);
  const [inputDomain, setInputDomain] = useState('');
  const [inputMode, setInputMode] = useState<SSLProviderMode>('letsencrypt');
  const [isDomainLoading, setIsDomainLoading] = useState(false);
  const [isDomainSaving, setIsDomainSaving] = useState(false);
  const [domainFeedback, setDomainFeedback] = useState<DomainFeedback>(null);

  // Backup & Disaster Recovery state
  const [backupStatus, setBackupStatus] = useState<BackupStatusResponse | null>(null);
  const [isBackupLoading, setIsBackupLoading] = useState(false);
  const [exportFormat, setExportFormat] = useState<'sql.gz' | 'sql'>('sql.gz');
  const [selectedBackupFile, setSelectedBackupFile] = useState<File | null>(null);
  const [isRestoring, setIsRestoring] = useState(false);
  const [isRestoreModalOpen, setIsRestoreModalOpen] = useState(false);
  const [backupFeedback, setBackupFeedback] = useState<BackupFeedback>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const res = await api.system.settings();
      setSettings(res.items || []);
      const map: Record<string, string> = {};
      (res.items || []).forEach((s) => {
        map[s.key] = formatSettingValue(s.value);
      });
      setEditValues(map);
      setLoadError(null);
    } catch (err: unknown) {
      setLoadError(getErrorMessage(err));
    } finally {
      setIsLoading(false);
    }
  };

  const loadDomainConfig = async () => {
    setIsDomainLoading(true);
    try {
      const res = await api.system.domain.get();
      setDomainConfig(res);
      if (res.domain) {
        setInputDomain(res.domain);
        setInputMode(res.mode || 'letsencrypt');
      }
    } catch (err: unknown) {
      toast.error('Gagal memuat status domain: ' + getErrorMessage(err));
    } finally {
      setIsDomainLoading(false);
    }
  };

  const loadBackupStatus = async () => {
    setIsBackupLoading(true);
    try {
      const res = await api.system.backup.status();
      setBackupStatus(res);
    } catch (err: unknown) {
      console.error('Gagal memuat status backup:', err);
    } finally {
      setIsBackupLoading(false);
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      const file = e.target.files[0];
      const name = file.name.toLowerCase();
      if (!name.endsWith('.sql') && !name.endsWith('.sql.gz')) {
        toast.error('Berkas cadangan harus berupa format .sql atau .sql.gz');
        return;
      }
      if (file.size > 50 * 1024 * 1024) {
        toast.error('Ukuran berkas melebihi batas 50 MB.');
        return;
      }
      setSelectedBackupFile(file);
      setBackupFeedback(null);
    }
  };

  const handleExecuteRestore = async () => {
    if (!selectedBackupFile) return;
    setIsRestoring(true);
    setBackupFeedback(null);
    try {
      await api.system.backup.restore(selectedBackupFile);
      toast.success('Basis data berhasil dipulihkan!');
      setBackupFeedback({
        type: 'success',
        message: 'Pemulihan berhasil dilakukan. Seluruh konfigurasi dan tabel sistem telah disinkronkan kembali.',
      });
      setIsRestoreModalOpen(false);
      setSelectedBackupFile(null);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
      void loadBackupStatus();
      void loadSettings();
    } catch (err: unknown) {
      const msg = getErrorMessage(err);
      toast.error('Gagal memulihkan database: ' + msg);
      setBackupFeedback({
        type: 'error',
        message: `Gagal memulihkan database: ${msg}`,
      });
    } finally {
      setIsRestoring(false);
    }
  };

  useEffect(() => {
    void loadSettings();
    void loadDomainConfig();
    void loadBackupStatus();
  }, []);

  const handleSave = async (key: string) => {
    if (isReservedSetting(key)) {
      toast.error('Parameter ini dicadangkan untuk subsistem internal dan tidak dapat diubah dari sini.');
      return;
    }
    setSavingKey(key);
    try {
      const original = settings.find((s) => s.key === key);
      await api.system.updateSetting(key, editValues[key] || '', original?.description);
      toast.success(`Parameter "${key}" berhasil diperbarui.`);
      void loadSettings();
    } catch (err: unknown) {
      toast.error('Gagal menyimpan pengaturan: ' + getErrorMessage(err));
    } finally {
      setSavingKey(null);
    }
  };

  const handleDeleteSetting = async (key: string) => {
    const ok = await confirmModal({
      title: 'Hapus Parameter Runtime?',
      message: `Hapus parameter "${key}" dari basis data? Pengaturan akan kembali ke nilai bawaan sistem.`,
      confirmText: 'Ya, Hapus',
      danger: true,
    });
    if (!ok) return;

    try {
      await api.system.deleteSetting(key);
      toast.success(`Parameter "${key}" berhasil dihapus.`);
      void loadSettings();
    } catch (err: unknown) {
      toast.error('Gagal menghapus parameter: ' + getErrorMessage(err));
    }
  };

  const handleCreateSetting = async (e: React.FormEvent) => {
    e.preventDefault();
    const key = newSetting.key.trim();
    if (!key) {
      toast.error('Kunci parameter tidak boleh kosong.');
      return;
    }
    if (isReservedSetting(key)) {
      toast.error('Kunci awalan "cli:config:" dan "system:" dicadangkan untuk modul internal.');
      return;
    }
    setIsCreatingSetting(true);
    try {
      await api.system.updateSetting(key, newSetting.value, newSetting.description.trim() || undefined);
      toast.success(`Parameter "${key}" berhasil dibuat.`);
      setIsCreateSettingOpen(false);
      setNewSetting({ key: '', value: '', description: '' });
      void loadSettings();
    } catch (err: unknown) {
      toast.error('Gagal membuat parameter: ' + getErrorMessage(err));
    } finally {
      setIsCreatingSetting(false);
    }
  };

  const handleSaveDomain = async (e: React.FormEvent) => {
    e.preventDefault();
    const domain = inputDomain.trim().toLowerCase();
    if (!domain || !/^(?!-)(?!.*--)([a-z0-9-]{1,63}\.)+[a-z]{2,}$/.test(domain)) {
      setDomainFeedback({ type: 'error', message: 'Nama domain tidak valid (contoh: gateway.example.com).' });
      return;
    }

    setIsDomainSaving(true);
    setDomainFeedback(null);
    try {
      const res = await api.system.domain.update({
        domain,
        mode: inputMode,
      });
      setDomainConfig(res);
      setDomainFeedback({
        type: 'success',
        message: res.message || 'Domain berhasil diverifikasi dan HTTPS otomatis telah diaktifkan!',
      });
    } catch (err: unknown) {
      setDomainFeedback({
        type: 'error',
        message: getErrorMessage(err, 'Gagal mengonfigurasi domain. Periksa kembali DNS Record Anda.'),
      });
    } finally {
      setIsDomainSaving(false);
    }
  };

  const handleDeleteDomain = async () => {
    const confirmed = await confirmModal({
      title: 'Lepas Domain Kustom',
      message: 'Apakah Anda yakin ingin melepas domain dan mengembalikan akses ke IP publik default?',
      danger: true,
      confirmText: 'Ya, Lepas Domain',
    });
    if (!confirmed) {
      return;
    }

    setIsDomainSaving(true);
    setDomainFeedback(null);
    try {
      await api.system.domain.delete();
      setInputDomain('');
      setDomainFeedback({
        type: 'info',
        message: 'Domain kustom berhasil dilepas. Sistem kembali menggunakan alamat IP server.',
      });
      void loadDomainConfig();
    } catch (err: unknown) {
      setDomainFeedback({
        type: 'error',
        message: 'Gagal menghapus domain: ' + getErrorMessage(err),
      });
    } finally {
      setIsDomainSaving(false);
    }
  };

  const serverIP = domainConfig?.server_ip || null;

  return (
    <div className="space-y-6">
      <PageHeader title="Pengaturan Sistem" />

      {loadError && (
        <QueryError message={loadError} onRetry={() => void loadSettings()} />
      )}

      {/* Tab Switcher Pills */}
      <div className="flex flex-wrap border-b border-border/80 gap-1 mb-6">
        <button
          type="button"
          onClick={() => setActiveTab('domain')}
          className={`px-4 py-2.5 text-xs font-semibold rounded-t-lg transition-colors flex items-center gap-2 cursor-pointer ${
            activeTab === 'domain'
              ? 'border-b-2 border-accent text-white bg-bg-surface-2'
              : 'text-text-muted hover:text-white hover:bg-bg-surface-2/40'
          }`}
        >
          <Globe className="w-4 h-4 text-accent" />
          <span>Domain & HTTPS</span>
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('backup')}
          className={`px-4 py-2.5 text-xs font-semibold rounded-t-lg transition-colors flex items-center gap-2 cursor-pointer ${
            activeTab === 'backup'
              ? 'border-b-2 border-accent text-white bg-bg-surface-2'
              : 'text-text-muted hover:text-white hover:bg-bg-surface-2/40'
          }`}
        >
          <Database className="w-4 h-4 text-accent" />
          <span>Backup & Restore</span>
        </button>
        <button
          type="button"
          onClick={() => setActiveTab('advanced')}
          className={`px-4 py-2.5 text-xs font-semibold rounded-t-lg transition-colors flex items-center gap-2 cursor-pointer ${
            activeTab === 'advanced'
              ? 'border-b-2 border-accent text-white bg-bg-surface-2'
              : 'text-text-muted hover:text-white hover:bg-bg-surface-2/40'
          }`}
        >
          <Sliders className="w-4 h-4 text-accent" />
          <span>Pengaturan Lanjutan</span>
        </button>
      </div>

      {/* 1. Pengaturan Domain & HTTPS Otomatis */}
      {activeTab === 'domain' && (
        <DomainTab
          domainConfig={domainConfig}
          inputDomain={inputDomain}
          inputMode={inputMode}
          isDomainLoading={isDomainLoading}
          isDomainSaving={isDomainSaving}
          domainFeedback={domainFeedback}
          serverIP={serverIP}
          setInputDomain={setInputDomain}
          setInputMode={setInputMode}
          onRefresh={() => void loadDomainConfig()}
          onSaveDomain={handleSaveDomain}
          onDeleteDomain={() => void handleDeleteDomain()}
        />
      )}

      {/* 2. Pencadangan & Pemulihan Database (SQL Backup & Disaster Recovery) */}
      {activeTab === 'backup' && (
        <BackupTab
          backupStatus={backupStatus}
          isBackupLoading={isBackupLoading}
          exportFormat={exportFormat}
          selectedBackupFile={selectedBackupFile}
          isRestoring={isRestoring}
          backupFeedback={backupFeedback}
          fileInputRef={fileInputRef}
          setExportFormat={setExportFormat}
          onRefresh={() => void loadBackupStatus()}
          onFileChange={handleFileChange}
          onOpenRestoreModal={() => setIsRestoreModalOpen(true)}
          exportUrl={api.system.backup.exportUrl(exportFormat)}
        />
      )}

      {/* 3. Runtime Parameters Section (Pengaturan Lanjutan) */}
      {activeTab === 'advanced' && (
        <AdvancedSettingsTab
          settings={settings}
          editValues={editValues}
          isLoading={isLoading}
          savingKey={savingKey}
          onRefresh={() => void loadSettings()}
          onOpenCreateModal={() => setIsCreateSettingOpen(true)}
          onEditChange={(key, val) => setEditValues((prev) => ({ ...prev, [key]: val }))}
          onSaveSetting={(key) => void handleSave(key)}
          onDeleteSetting={(key) => void handleDeleteSetting(key)}
        />
      )}

      {/* Modal Tambah Parameter Runtime */}
      <CreateSettingModal
        isOpen={isCreateSettingOpen}
        isCreating={isCreatingSetting}
        newSetting={newSetting}
        onClose={() => setIsCreateSettingOpen(false)}
        onChange={(field, value) => setNewSetting((prev) => ({ ...prev, [field]: value }))}
        onSubmit={handleCreateSetting}
      />

      {/* Modal Konfirmasi Pemulihan Database */}
      <RestoreConfirmModal
        isOpen={isRestoreModalOpen}
        isRestoring={isRestoring}
        selectedBackupFile={selectedBackupFile}
        databaseName={backupStatus?.database_name}
        onClose={() => setIsRestoreModalOpen(false)}
        onConfirmRestore={() => void handleExecuteRestore()}
      />
    </div>
  );
};
