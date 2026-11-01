import { describe, it, expect } from 'vitest';
import { statusKonfigurasiTool, verifikasiHasilApply } from './statusTool';
import type { CLITool } from '../../types';

// Status tool CLI + verifikasi sesudah apply (Fase 2 R4).
// updated_at adalah satu-satunya penanda jujur "dikelola Route-X" dari
// pindaian backend; verifikasi menolak melaporkan sukses bila pindai
// ulang tidak cocok dengan yang diterapkan.

function tool(partial: Partial<CLITool>): CLITool {
  return {
    id: 'claude-code',
    name: 'Claude Code',
    description: '',
    category: 'AI Coding',
    installed: true,
    supported_modes: ['model_only', 'routing', 'combo'],
    active_mode: 'routing',
    active_target: 'coding-fast',
    env_vars: {},
    ...partial,
  } as CLITool;
}

describe('statusKonfigurasiTool', () => {
  it('tidak terpasang bila executable tidak ditemukan', () => {
    expect(statusKonfigurasiTool(tool({ installed: false }))).toBe('tidak-terpasang');
  });

  it('belum dikonfigurasi bila belum ada updated_at dari Route-X', () => {
    expect(statusKonfigurasiTool(tool({ updated_at: undefined }))).toBe('belum-dikonfigurasi');
  });

  it('terkonfigurasi-routex bila updated_at terisi', () => {
    expect(statusKonfigurasiTool(tool({ updated_at: '2026-11-01T00:00:00Z' }))).toBe(
      'terkonfigurasi-routex',
    );
  });
});

describe('verifikasiHasilApply', () => {
  it('ok bila pindai ulang cocok persis dengan yang diterapkan', () => {
    const hasil = tool({ updated_at: '2026-11-01T00:00:00Z' });
    expect(verifikasiHasilApply(hasil, 'routing', 'coding-fast')).toEqual({ ok: true });
  });

  it('gagal bila tool hilang dari pindai ulang', () => {
    const ver = verifikasiHasilApply(undefined, 'routing', 'coding-fast');
    expect(ver.ok).toBe(false);
    expect(ver.pesan).toMatch(/tidak ditemukan/i);
  });

  it('gagal bila konfigurasi tidak tercatat (updated_at kosong)', () => {
    const ver = verifikasiHasilApply(tool({}), 'routing', 'coding-fast');
    expect(ver.ok).toBe(false);
    expect(ver.pesan).toMatch(/tidak tercatat/i);
  });

  it('gagal dan menyebut keadaan aktual bila target berbeda', () => {
    const hasil = tool({ updated_at: '2026-11-01T00:00:00Z', active_target: 'model-lain' });
    const ver = verifikasiHasilApply(hasil, 'routing', 'coding-fast');
    expect(ver.ok).toBe(false);
    expect(ver.pesan).toContain('model-lain');
  });
});
