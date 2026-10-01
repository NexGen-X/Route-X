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
      <div className="space-y-5 text-sm">
        <div className="p-4 rounded-2xl bg-red-500/10 border border-red-500/20 text-red-300 flex items-start gap-4 backdrop-blur-md shadow-lg shadow-red-500/5">
          <div className="w-10 h-10 rounded-xl bg-red-500/20 text-red-400 flex items-center justify-center shrink-0 border border-red-500/30">
            <AlertCircle className="w-5 h-5" />
          </div>
          <div className="space-y-1.5 pt-0.5">
            <span className="font-bold block text-white text-sm tracking-wide">Tindakan Ini Berdampak Besar</span>
            <p className="text-xs text-red-200/90 leading-relaxed font-medium">
              Basis data Route-X akan dieksekusi ulang menggunakan skrip SQL yang diunggah. Seluruh konfigurasi, model, aturan, dan kunci API akan diselaraskan dengan isi berkas tersebut.
            </p>
          </div>
        </div>

        <div className="p-4 bg-bg-surface-2/50 backdrop-blur-xl rounded-2xl border border-white/5 shadow-inner space-y-2.5 font-mono text-xs">
          <div className="flex justify-between items-center text-text-secondary border-b border-white/5 pb-2.5">
            <span className="uppercase tracking-wider text-[10px]">Berkas Cadangan</span>
            <span className="text-white font-bold truncate max-w-[200px] bg-white/5 px-2 py-1 rounded">
              {selectedBackupFile?.name}
            </span>
          </div>
          <div className="flex justify-between items-center text-text-secondary border-b border-white/5 pb-2.5 pt-1">
            <span className="uppercase tracking-wider text-[10px]">Ukuran Berkas</span>
            <span className="text-accent font-semibold bg-accent/10 px-2 py-1 rounded">
              {selectedBackupFile ? `${(selectedBackupFile.size / 1024).toFixed(1)} KB` : '-'}
            </span>
          </div>
          <div className="flex justify-between items-center text-text-secondary pt-1">
            <span className="uppercase tracking-wider text-[10px]">Target Database</span>
            <span className="text-white font-semibold bg-white/5 px-2 py-1 rounded">{databaseName}</span>
          </div>
        </div>

        <p className="text-text-muted text-[11px] leading-relaxed text-center px-4">
          Setelah proses pemulihan selesai, sistem akan otomatis memperbarui tampilan dashboard dan parameter yang tersimpan.
        </p>
      </div>
    </Modal>
  );
};
