import type { DomainConfig, BackupStatusResponse } from '../../types';

export type SettingsTab = 'domain' | 'backup' | 'advanced';

export type SSLProviderMode = 'letsencrypt' | 'cloudflare';

export interface RuntimeSettingItem {
  key: string;
  value: unknown;
  description?: string;
}

export type DomainFeedback = {
  type: 'success' | 'error' | 'info';
  message: string;
} | null;

export type BackupFeedback = {
  type: 'success' | 'error';
  message: string;
} | null;

export interface NewSettingForm {
  key: string;
  value: string;
  description: string;
}

export interface DomainTabProps {
  domainConfig: DomainConfig | null;
  inputDomain: string;
  inputMode: SSLProviderMode;
  isDomainLoading: boolean;
  isDomainSaving: boolean;
  domainFeedback: DomainFeedback;
  serverIP: string | null;
  setInputDomain: (domain: string) => void;
  setInputMode: (mode: SSLProviderMode) => void;
  onRefresh: () => void;
  onSaveDomain: (e: React.FormEvent) => void;
  onDeleteDomain: () => void;
}

export interface BackupTabProps {
  backupStatus: BackupStatusResponse | null;
  isBackupLoading: boolean;
  exportFormat: 'sql.gz' | 'sql';
  selectedBackupFile: File | null;
  isRestoring: boolean;
  backupFeedback: BackupFeedback;
  fileInputRef: React.RefObject<HTMLInputElement>;
  setExportFormat: (format: 'sql.gz' | 'sql') => void;
  onRefresh: () => void;
  onFileChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onOpenRestoreModal: () => void;
  exportUrl: string;
}

export interface AdvancedSettingsTabProps {
  settings: RuntimeSettingItem[];
  editValues: Record<string, string>;
  isLoading: boolean;
  savingKey: string | null;
  onRefresh: () => void;
  onOpenCreateModal: () => void;
  onEditChange: (key: string, value: string) => void;
  onSaveSetting: (key: string) => void;
  onDeleteSetting: (key: string) => void;
}

export interface CreateSettingModalProps {
  isOpen: boolean;
  isCreating: boolean;
  newSetting: NewSettingForm;
  onClose: () => void;
  onChange: (field: keyof NewSettingForm, value: string) => void;
  onSubmit: (e: React.FormEvent) => void;
}

export interface RestoreConfirmModalProps {
  isOpen: boolean;
  isRestoring: boolean;
  selectedBackupFile: File | null;
  databaseName?: string;
  onClose: () => void;
  onConfirmRestore: () => void;
}
