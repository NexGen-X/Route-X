import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { createRef } from 'react';
import { BackupTab } from './BackupTab';

// B-447: sel Basis Data tidak lagi mengklaim 'Memuat...' selamanya
// saat status cadangan GAGAL dimuat — penanda netral '—' seperti
// sel lain (keadaanMuat bersama).

const propsDasar = {
  exportFormat: 'sql.gz' as const,
  selectedBackupFile: null,
  isRestoring: false,
  backupFeedback: null,
  fileInputRef: createRef<HTMLInputElement>(),
  setExportFormat: vi.fn(),
  onRefresh: vi.fn(),
  onFileChange: vi.fn(),
  onOpenRestoreModal: vi.fn(),
  onExport: vi.fn(),
  isExporting: false,
};

describe('BackupTab — sel Basis Data (B-447)', () => {
  it('status gagal dimuat → tidak ada klaim Memuat... permanen', () => {
    render(
      <BackupTab
        {...propsDasar}
        backupStatus={null}
        isBackupLoading={false}
        backupError="jaringan putus"
      />
    );
    expect(screen.queryByText('Memuat...')).toBeNull();
    expect(screen.getByText(/Gagal memuat status backup/)).toBeInTheDocument();
  });

  it('status terukur → nama basis data tampil', () => {
    render(
      <BackupTab
        {...propsDasar}
        backupStatus={{
          database_name: 'routex_prod',
          database_size: '12 MB',
          database_bytes: 12582912,
          total_tables: 40,
          models_count: 3,
          providers_count: 2,
          routing_rules_count: 1,
          api_keys_count: 4,
        }}
        isBackupLoading={false}
        backupError={null}
      />
    );
    expect(screen.getByText('routex_prod')).toBeInTheDocument();
  });
});
