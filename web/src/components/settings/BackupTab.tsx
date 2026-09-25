import React from 'react';
import { Card } from '../common/Card';
import { Button } from '../common/Button';
import {
  Database,
  Download,
  Upload,
  FileText,
  AlertTriangle,
  RefreshCw,
  CheckCircle2,
  AlertCircle,
} from 'lucide-react';
import type { BackupTabProps } from './types';

export const BackupTab: React.FC<BackupTabProps> = ({
  backupStatus,
  isBackupLoading,
  exportFormat,
  selectedBackupFile,
  isRestoring,
  backupFeedback,
  fileInputRef,
  setExportFormat,
  onRefresh,
  onFileChange,
  onOpenRestoreModal,
  exportUrl,
}) => {
  return (
    <Card className="p-6 border-accent/20 bg-bg-surface-2">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-5 border-b border-border">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-accent/15 text-accent flex items-center justify-center">
            <Database className="w-5 h-5" />
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-base sm:text-lg font-bold text-white">
                Pencadangan & Pemulihan Database (SQL)
              </h2>
              <span className="inline-flex items-center gap-1 text-[10px] sm:text-[11px] font-semibold text-accent bg-accent/10 px-2 py-0.5 rounded-full border border-accent/20 whitespace-nowrap">
                PostgreSQL Native
              </span>
            </div>
          </div>
        </div>
        <Button
          variant="secondary"
          size="sm"
          onClick={onRefresh}
          isLoading={isBackupLoading}
          icon={<RefreshCw className="w-4 h-4" />}
          title="Segarkan Status Pencadangan"
        >
          <span className="hidden sm:inline">Segarkan Status</span>
        </Button>
      </div>

      {/* Feedback Alert */}
      {backupFeedback && (
        <div
          role={backupFeedback.type === 'error' ? 'alert' : 'status'}
          aria-live="polite"
          className={`mt-4 p-3.5 rounded-lg text-xs flex items-start gap-2.5 border ${
            backupFeedback.type === 'success'
              ? 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
              : 'bg-red-500/10 text-red-300 border-red-500/30'
          }`}
        >
          {backupFeedback.type === 'success' ? (
            <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-400 mt-0.5" />
          ) : (
            <AlertCircle className="w-4 h-4 shrink-0 text-red-400 mt-0.5" />
          )}
          <div className="flex-1">{backupFeedback.message}</div>
        </div>
      )}

      {/* Ringkasan Status Database */}
      <div className="mt-5 grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
          <span className="text-text-muted block text-[11px] font-medium">Basis Data</span>
          <span className="font-mono text-white text-sm font-semibold truncate block">
            {backupStatus?.database_name || 'Memuat...'}
          </span>
          <span className="text-[10px] text-accent font-mono mt-0.5 block">
            Ukuran: {backupStatus?.database_size || '-'}
          </span>
        </div>
        <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
          <span className="text-text-muted block text-[11px] font-medium">Total Tabel Sistem</span>
          <span className="font-mono text-white text-sm font-semibold block">
            {backupStatus ? `${backupStatus.total_tables} Tabel` : '-'}
          </span>
          <span className="text-[10px] text-text-muted font-mono mt-0.5 block">
            DDL & DML terindeks
          </span>
        </div>
        <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
          <span className="text-text-muted block text-[11px] font-medium">Entitas Terkonfigurasi</span>
          <span className="font-mono text-white text-xs font-semibold block mt-0.5">
            {backupStatus ? `${backupStatus.models_count} Model · ${backupStatus.providers_count} Provider` : '-'}
          </span>
          <span className="text-[10px] text-text-muted font-mono mt-0.5 block">
            {backupStatus ? `${backupStatus.routing_rules_count} Rules · ${backupStatus.api_keys_count} Keys` : '-'}
          </span>
        </div>
        <div className="bg-bg-surface-2 p-3 rounded-xl border border-border">
          <span className="text-text-muted block text-[11px] font-medium">Cadangan Server Terakhir</span>
          <span className="font-mono text-emerald-400 text-xs font-semibold truncate block">
            {backupStatus?.last_server_backup ? backupStatus.last_server_backup.file_size : 'Belum ada'}
          </span>
          <span className="text-[10px] text-text-muted font-mono mt-0.5 truncate block">
            {backupStatus?.last_server_backup
              ? new Date(backupStatus.last_server_backup.created_at).toLocaleString('id-ID')
              : 'Sistem siap di-backup'}
          </span>
        </div>
      </div>

      {/* Dua Kolom Aksi Ekspor vs Pemulihan */}
      <div className="mt-6 grid grid-cols-1 lg:grid-cols-2 gap-6 pt-5 border-t border-border">
        {/* Kolom Kiri: Ekspor Cadangan SQL */}
        <div className="flex flex-col justify-between p-5 rounded-xl bg-bg-surface-2 border border-border/80">
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-white font-semibold text-sm">
              <Download className="w-4 h-4 text-accent" />
              <span>Ekspor Snapshot Database (SQL)</span>
            </div>
            <p className="text-xs text-text-secondary leading-relaxed">
              Mengekstrak seluruh data Route-X menggunakan utilitas native <code className="text-text-muted font-mono">pg_dump</code> dengan opsi <code className="text-text-muted font-mono">--clean --if-exists</code>. File cadangan dapat langsung diimpor ke Route-X atau server PostgreSQL lain.
            </p>

            <div className="pt-2">
              <label className="block text-[11px] font-medium text-text-muted mb-2">Pilih Format Berkas:</label>
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  onClick={() => setExportFormat('sql.gz')}
                  className={`p-2.5 rounded-lg border text-left transition-all cursor-pointer ${
                    exportFormat === 'sql.gz'
                      ? 'border-accent bg-accent/10 text-white'
                      : 'border-border bg-bg-surface-2 text-text-muted hover:border-border/80'
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs font-bold text-accent">.sql.gz</span>
                    <span className="text-[9px] bg-accent/20 text-accent font-semibold px-1.5 py-0.2 rounded">Rekomendasi</span>
                  </div>
                  <span className="text-[10px] text-text-secondary block mt-1">Kompresi Gzip ringkas, cepat diunduh.</span>
                </button>
                <button
                  type="button"
                  onClick={() => setExportFormat('sql')}
                  className={`p-2.5 rounded-lg border text-left transition-all cursor-pointer ${
                    exportFormat === 'sql'
                      ? 'border-accent bg-accent/10 text-white'
                      : 'border-border bg-bg-surface-2 text-text-muted hover:border-border/80'
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs font-bold text-white">.sql</span>
                    <span className="text-[9px] bg-bg-surface-2 text-text-muted font-medium px-1.5 py-0.2 rounded">Plain</span>
                  </div>
                  <span className="text-[10px] text-text-secondary block mt-1">Teks SQL mentah yang dapat dibaca manusia.</span>
                </button>
              </div>
            </div>
          </div>

          <div className="pt-5 mt-4 border-t border-border/60">
            <a
              href={exportUrl}
              download
              className="inline-flex items-center justify-center gap-2 w-full px-4 py-2.5 bg-accent hover:bg-accent/90 text-bg-base font-semibold text-xs rounded-nav transition-colors shadow-sm"
            >
              <Download className="w-4 h-4" />
              <span>Unduh Cadangan SQL ({exportFormat === 'sql.gz' ? '.sql.gz' : '.sql'})</span>
            </a>
          </div>
        </div>

        {/* Kolom Kanan: Pemulihan Database (Restore) */}
        <div className="flex flex-col justify-between p-5 rounded-xl bg-bg-surface-2 border border-border/80">
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-white font-semibold text-sm">
              <Upload className="w-4 h-4 text-amber-400" />
              <span>Pemulihan Database (Restore SQL)</span>
            </div>
            <p className="text-xs text-text-secondary leading-relaxed">
              Memulihkan skema dan data Route-X dari file cadangan SQL. Mendukung file <code className="text-text-muted font-mono">.sql</code> maupun terkompresi <code className="text-text-muted font-mono">.sql.gz</code> (Maksimum 50 MB).
            </p>

            {/* Area File Picker */}
            <div className="pt-2">
              <input
                ref={fileInputRef}
                type="file"
                id="restore-file-input"
                accept=".sql,.sql.gz,application/x-gzip,application/gzip,application/sql,text/sql"
                onChange={onFileChange}
                className="hidden"
              />
              <label
                htmlFor="restore-file-input"
                className={`flex flex-col items-center justify-center p-4 border-2 border-dashed rounded-xl cursor-pointer transition-all ${
                  selectedBackupFile
                    ? 'border-accent/60 bg-accent/5'
                    : 'border-border hover:border-border/80 bg-bg-surface-2'
                }`}
              >
                {selectedBackupFile ? (
                  <div className="flex items-center gap-2.5 text-xs text-white">
                    <FileText className="w-5 h-5 text-accent shrink-0" />
                    <div className="text-left overflow-hidden">
                      <span className="font-mono font-semibold block truncate max-w-[220px]">
                        {selectedBackupFile.name}
                      </span>
                      <span className="text-[10px] text-text-muted font-mono">
                        {(selectedBackupFile.size / 1024).toFixed(1)} KB (
                        {(selectedBackupFile.size / (1024 * 1024)).toFixed(2)} MB)
                      </span>
                    </div>
                  </div>
                ) : (
                  <div className="text-center">
                    <Upload className="w-6 h-6 text-text-muted mx-auto mb-1.5" />
                    <span className="text-xs text-white font-medium block">Pilih Berkas Cadangan</span>
                    <span className="text-[10px] text-text-muted block mt-0.5">
                      Klik untuk memilih berkas .sql atau .sql.gz
                    </span>
                  </div>
                )}
              </label>
            </div>

            {/* Peringatan Bahaya */}
            <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-300 flex items-start gap-2">
              <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
              <span>
                <strong>Perhatian:</strong> Pemulihan data akan menggantikan data konfigurasi saat ini. Disarankan melakukan ekspor cadangan terlebih dahulu sebelum melanjutkan.
              </span>
            </div>
          </div>

          <div className="pt-5 mt-4 border-t border-border/60">
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={!selectedBackupFile || isRestoring}
              onClick={onOpenRestoreModal}
              icon={<Upload className="w-4 h-4" />}
              className="w-full justify-center"
            >
              {selectedBackupFile ? 'Mulai Pemulihan Database...' : 'Pilih Berkas Dahulu'}
            </Button>
          </div>
        </div>
      </div>
    </Card>
  );
};
