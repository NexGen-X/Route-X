import React from 'react';

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
    <div className="space-y-6">
      {/* Header Section */}
      <div className="p-6 rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-2xl bg-gradient-to-br from-accent/20 to-accent/5 text-accent flex items-center justify-center border border-accent/10 shadow-inner">
            <Database className="w-6 h-6" />
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="text-base font-semibold text-white tracking-wide">
                Pencadangan & Pemulihan Database (SQL)
              </h2>
              <span className="inline-flex items-center gap-1.5 text-[11px] font-semibold text-accent bg-accent/10 px-2.5 py-1 rounded-full border border-accent/20 whitespace-nowrap shadow-sm shadow-accent/5">
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
          className="bg-white/5 hover:bg-white/10 border-white/5 text-white shadow-sm"
        >
          <span className="hidden sm:inline">Segarkan Status</span>
        </Button>
      </div>

      {/* Feedback Alert */}
      {backupFeedback && (
        <div
          role={backupFeedback.type === 'error' ? 'alert' : 'status'}
          aria-live="polite"
          className={`p-4 rounded-xl text-xs flex items-start gap-3 backdrop-blur-md border shadow-lg ${
            backupFeedback.type === 'success'
              ? 'bg-emerald-500/10 text-emerald-200 border-emerald-500/20 shadow-emerald-500/5'
              : 'bg-red-500/10 text-red-200 border-red-500/20 shadow-red-500/5'
          }`}
        >
          {backupFeedback.type === 'success' ? (
            <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-400 mt-0.5" />
          ) : (
            <AlertCircle className="w-4 h-4 shrink-0 text-red-400 mt-0.5" />
          )}
          <div className="flex-1 font-medium leading-relaxed">{backupFeedback.message}</div>
        </div>
      )}

      {/* Ringkasan Status Database */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-bg-surface/40 backdrop-blur-md p-4 rounded-2xl border border-white/5 shadow-lg shadow-black/10 flex flex-col">
          <span className="text-text-muted text-[11px] font-medium uppercase tracking-wider mb-2">Basis Data</span>
          <span className="font-mono text-white text-sm font-semibold truncate">
            {backupStatus?.database_name || 'Memuat...'}
          </span>
          <span className="text-[11px] text-accent font-mono mt-1">
            Ukuran: {backupStatus?.database_size || '-'}
          </span>
        </div>
        <div className="bg-bg-surface/40 backdrop-blur-md p-4 rounded-2xl border border-white/5 shadow-lg shadow-black/10 flex flex-col">
          <span className="text-text-muted text-[11px] font-medium uppercase tracking-wider mb-2">Total Tabel Sistem</span>
          <span className="font-mono text-white text-sm font-semibold">
            {backupStatus ? `${backupStatus.total_tables} Tabel` : '-'}
          </span>
          <span className="text-[11px] text-text-muted font-mono mt-1">
            DDL & DML terindeks
          </span>
        </div>
        <div className="bg-bg-surface/40 backdrop-blur-md p-4 rounded-2xl border border-white/5 shadow-lg shadow-black/10 flex flex-col">
          <span className="text-text-muted text-[11px] font-medium uppercase tracking-wider mb-2">Entitas Terkonfigurasi</span>
          <span className="font-mono text-white text-xs font-semibold mt-1">
            {backupStatus ? `${backupStatus.models_count} Model · ${backupStatus.providers_count} Provider` : '-'}
          </span>
          <span className="text-[11px] text-text-muted font-mono mt-1">
            {backupStatus ? `${backupStatus.routing_rules_count} Rules · ${backupStatus.api_keys_count} Keys` : '-'}
          </span>
        </div>
        <div className="bg-bg-surface/40 backdrop-blur-md p-4 rounded-2xl border border-white/5 shadow-lg shadow-black/10 flex flex-col">
          <span className="text-text-muted text-[11px] font-medium uppercase tracking-wider mb-2">Cadangan Server Terakhir</span>
          <span className="font-mono text-emerald-400 text-xs font-semibold truncate">
            {backupStatus?.last_server_backup ? backupStatus.last_server_backup.file_size : 'Belum ada'}
          </span>
          <span className="text-[11px] text-text-muted font-mono mt-1 truncate">
            {backupStatus?.last_server_backup
              ? new Date(backupStatus.last_server_backup.created_at).toLocaleString('id-ID')
              : 'Sistem siap di-backup'}
          </span>
        </div>
      </div>

      {/* Dua Kolom Aksi Ekspor vs Pemulihan */}
      <div className="mt-6 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 lg:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {/* Kolom Kiri: Ekspor Cadangan SQL */}
        <div className="flex flex-col justify-between p-6 rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10">
          <div className="space-y-4">
            <div className="flex items-center gap-3 text-white font-semibold text-base">
              <div className="w-10 h-10 rounded-xl bg-accent/10 flex items-center justify-center border border-accent/20">
                <Download className="w-5 h-5 text-accent" />
              </div>
              <span>Ekspor Snapshot Database (SQL)</span>
            </div>
            <p className="text-xs text-text-secondary leading-relaxed font-medium">
              Mengekstrak seluruh data Route-X menggunakan utilitas native <code className="text-accent bg-accent/10 px-1 py-0.5 rounded font-mono border border-accent/20">pg_dump</code> dengan opsi <code className="text-accent bg-accent/10 px-1 py-0.5 rounded font-mono border border-accent/20">--clean --if-exists</code>. File cadangan dapat langsung diimpor ke Route-X atau server PostgreSQL lain.
            </p>

            <div className="pt-2">
              <label className="block text-[11px] font-semibold text-text-muted mb-3 uppercase tracking-wider">Pilih Format Berkas:</label>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
                <button
                  type="button"
                  onClick={() => setExportFormat('sql.gz')}
                  className={`p-3 rounded-xl border transition-all duration-300 cursor-pointer ${
                    exportFormat === 'sql.gz'
                      ? 'border-accent bg-accent/10 text-white shadow-lg shadow-accent/5'
                      : 'border-white/5 bg-white/5 text-text-muted hover:border-white/10 hover:bg-white/10'
                  }`}
                >
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-mono text-sm font-bold text-accent">.sql.gz</span>
                    <span className="text-[9px] bg-accent/20 border border-accent/30 text-accent font-semibold px-2 py-0.5 rounded-full">Rekomendasi</span>
                  </div>
                  <span className="text-[10px] text-text-secondary block mt-1 leading-relaxed">Kompresi Gzip ringkas, cepat diunduh.</span>
                </button>
                <button
                  type="button"
                  onClick={() => setExportFormat('sql')}
                  className={`p-3 rounded-xl border transition-all duration-300 cursor-pointer ${
                    exportFormat === 'sql'
                      ? 'border-accent bg-accent/10 text-white shadow-lg shadow-accent/5'
                      : 'border-white/5 bg-white/5 text-text-muted hover:border-white/10 hover:bg-white/10'
                  }`}
                >
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-mono text-sm font-bold text-white">.sql</span>
                    <span className="text-[9px] bg-white/10 border border-white/20 text-text-muted font-medium px-2 py-0.5 rounded-full">Plain</span>
                  </div>
                  <span className="text-[10px] text-text-secondary block mt-1 leading-relaxed">Teks SQL mentah yang dapat dibaca manusia.</span>
                </button>
              </div>
            </div>
          </div>

          <div className="pt-6 mt-6 border-t border-white/5">
            <a
              href={exportUrl}
              download
              className="inline-flex items-center justify-center gap-2 w-full px-5 py-3 bg-gradient-to-r from-accent to-accent/80 hover:from-accent hover:to-accent text-white font-semibold text-sm rounded-xl transition-all shadow-lg shadow-accent/25"
            >
              <Download className="w-4 h-4" />
              <span>Unduh Cadangan SQL ({exportFormat === 'sql.gz' ? '.sql.gz' : '.sql'})</span>
            </a>
          </div>
        </div>

        {/* Kolom Kanan: Pemulihan Database (Restore) */}
        <div className="flex flex-col justify-between p-6 rounded-2xl bg-bg-surface/40 backdrop-blur-md border border-white/5 shadow-lg shadow-black/10">
          <div className="space-y-4">
            <div className="flex items-center gap-3 text-white font-semibold text-base">
              <div className="w-10 h-10 rounded-xl bg-amber-500/10 flex items-center justify-center border border-amber-500/20">
                <Upload className="w-5 h-5 text-amber-400" />
              </div>
              <span>Pemulihan Database (Restore SQL)</span>
            </div>
            <p className="text-xs text-text-secondary leading-relaxed font-medium">
              Memulihkan skema dan data Route-X dari file cadangan SQL. Mendukung file <code className="text-amber-400/80 bg-amber-400/10 px-1 py-0.5 rounded font-mono border border-amber-400/20">.sql</code> maupun terkompresi <code className="text-amber-400/80 bg-amber-400/10 px-1 py-0.5 rounded font-mono border border-amber-400/20">.sql.gz</code> (Maksimum 50 MB).
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
                className={`flex flex-col items-center justify-center p-6 border-2 border-dashed rounded-2xl cursor-pointer transition-all duration-300 ${
                  selectedBackupFile
                    ? 'border-amber-400/40 bg-amber-400/5 shadow-inner shadow-amber-400/5'
                    : 'border-white/10 hover:border-white/20 bg-white/5 hover:bg-white/10'
                }`}
              >
                {selectedBackupFile ? (
                  <div className="flex items-center gap-3 text-sm text-white w-full">
                    <FileText className="w-6 h-6 text-amber-400 shrink-0" />
                    <div className="text-left overflow-hidden flex-1">
                      <span className="font-mono font-semibold block truncate">
                        {selectedBackupFile.name}
                      </span>
                      <span className="text-[11px] text-text-muted font-mono mt-0.5 block">
                        {(selectedBackupFile.size / 1024).toFixed(1)} KB (
                        {(selectedBackupFile.size / (1024 * 1024)).toFixed(2)} MB)
                      </span>
                    </div>
                  </div>
                ) : (
                  <div className="text-center">
                    <div className="w-10 h-10 rounded-full bg-white/5 flex items-center justify-center mx-auto mb-2 border border-white/10">
                      <Upload className="w-5 h-5 text-text-muted" />
                    </div>
                    <span className="text-sm text-white font-medium block">Pilih Berkas Cadangan</span>
                    <span className="text-[11px] text-text-muted block mt-1">
                      Klik untuk memilih berkas .sql atau .sql.gz
                    </span>
                  </div>
                )}
              </label>
            </div>

            {/* Peringatan Bahaya */}
            <div className="p-4 rounded-xl bg-amber-500/10 border border-amber-500/20 text-xs text-amber-300/90 flex items-start gap-3 backdrop-blur-sm shadow-sm shadow-amber-500/5">
              <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
              <span className="leading-relaxed">
                <strong className="text-amber-400">Perhatian:</strong> Pemulihan data akan menggantikan data konfigurasi saat ini. Disarankan melakukan ekspor cadangan terlebih dahulu sebelum melanjutkan.
              </span>
            </div>
          </div>

          <div className="pt-6 mt-6 border-t border-white/5">
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={!selectedBackupFile || isRestoring}
              onClick={onOpenRestoreModal}
              icon={<Upload className="w-4 h-4" />}
              className="w-full justify-center bg-red-500/20 hover:bg-red-500/30 text-red-400 border border-red-500/30 shadow-lg shadow-red-500/10 py-3 rounded-xl text-sm transition-all"
            >
              {selectedBackupFile ? 'Mulai Pemulihan Database...' : 'Pilih Berkas Dahulu'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
