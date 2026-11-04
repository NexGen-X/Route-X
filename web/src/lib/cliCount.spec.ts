import { describe, it, expect, vi, beforeEach } from 'vitest';

// api/client dimock agar muatJumlahToolCli bisa diuji tanpa jaringan.
const detectedMock = vi.fn();
vi.mock('../api/client', () => ({
  api: { cli: { detected: () => detectedMock() } },
}));

describe('muatJumlahToolCli (audit3 V-FJ-11)', () => {
  beforeEach(() => {
    vi.resetModules();
    detectedMock.mockReset();
  });

  it('mengembalikan total_detected, bukan panjang registri tools', async () => {
    detectedMock.mockResolvedValue({
      tools: Array.from({ length: 20 }, (_, i) => ({ id: `tool-${i}` })),
      total_detected: 0,
    });
    const { muatJumlahToolCli } = await import('./cliCount');
    // Registri (mock) berisi banyak tool tetapi tidak ada yang
    // terdeteksi di mesin: badge harus 0, bukan panjang registri.
    await expect(muatJumlahToolCli()).resolves.toBe(0);
  });

  it('mengembalikan jumlah terdeteksi sebagian', async () => {
    detectedMock.mockResolvedValue({
      tools: Array.from({ length: 20 }, (_, i) => ({ id: `tool-${i}` })),
      total_detected: 3,
    });
    const { muatJumlahToolCli } = await import('./cliCount');
    await expect(muatJumlahToolCli()).resolves.toBe(3);
  });

  it('kegagalan memuat menghasilkan null (badge disembunyikan)', async () => {
    detectedMock.mockRejectedValue(new Error('jaringan mati'));
    const { muatJumlahToolCli } = await import('./cliCount');
    await expect(muatJumlahToolCli()).resolves.toBeNull();
  });
});
