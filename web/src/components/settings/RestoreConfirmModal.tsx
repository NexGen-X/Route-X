import React from 'react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { Upload, AlertCircle } from 'lucide-react';
import type { RestoreConfirmModalProps } from './types';

export const RestoreConfirmModal: React.FC<RestoreConfirmModalProps> = ({
  isOpen,
  isRestoring,
  selectedBackupFile,
  databaseName = 'routex_prod',
  onClose,
  onConfirmRestore,
}) => {
  return (
    <Modal
      isOpen={isOpen}
      onClose={() => {
        if (!isRestoring) onClose();
      }}
      title="Konfirmasi Pemulihan Database"
      footer={
        <div className="flex items-center justify-end gap-3 w-full">
          <Button
            variant="secondary"
            size="sm"
            disabled={isRestoring}
            onClick={onClose}
          >
            Batal
          </Button>
          <Button
            variant="danger"
            size="sm"
            isLoading={isRestoring}
            onClick={onConfirmRestore}
            icon={<Upload className="w-4 h-4" />}
          >
            {isRestoring ? 'Memulihkan Basis Data...' : 'Ya, Pulihkan Sekarang'}
          </Button>
        </div>
      }
    >
      <div className="space-y-4 text-xs">
        <div className="p-3.5 rounded-lg bg-red-500/10 border border-red-500/30 text-red-300 flex items-start gap-3">
          <AlertCircle className="w-5 h-5 text-red-400 shrink-0 mt-0.5" />
          <div className="space-y-1">
            <span className="font-bold block text-white text-sm">Tindakan Ini Berdampak Besar</span>
            <p className="text-xs text-red-300/90 leading-relaxed">
              Basis data Route-X akan dieksekusi ulang menggunakan skrip SQL yang diunggah. Seluruh konfigurasi, model, aturan, dan kunci API akan diselaraskan dengan isi berkas tersebut.
            </p>
          </div>
        </div>

        <div className="p-3 bg-bg-surface-2 rounded-lg border border-border space-y-1.5 font-mono">
          <div className="flex justify-between text-text-secondary">
            <span>Berkas Cadangan:</span>
            <span className="text-white font-bold truncate max-w-[200px]">
              {selectedBackupFile?.name}
            </span>
          </div>
          <div className="flex justify-between text-text-secondary">
            <span>Ukuran Berkas:</span>
            <span className="text-accent">
              {selectedBackupFile ? `${(selectedBackupFile.size / 1024).toFixed(1)} KB` : '-'}
            </span>
          </div>
          <div className="flex justify-between text-text-secondary">
            <span>Target Database:</span>
            <span className="text-white">{databaseName}</span>
          </div>
        </div>

        <p className="text-text-muted text-[11px]">
          Setelah proses pemulihan selesai, sistem akan otomatis memperbarui tampilan dashboard dan parameter yang tersimpan.
        </p>
      </div>
    </Modal>
  );
};
