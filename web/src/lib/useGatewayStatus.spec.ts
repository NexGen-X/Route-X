import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

// Spec B-224: status gateway pada chrome aplikasi harus DITURUNKAN
// dari hasil panggilan overview nyata — online hanya sesudah endpoint
// benar-benar menjawab, offline bila gagal, dan berubah mengikuti
// hasil polling berikutnya. Modul hook memakai store singleton, jadi
// setiap test memuat ulang modulnya.

const overviewMock = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    system: {
      overview: () => overviewMock(),
    },
  },
}));

async function muatHook() {
  vi.resetModules();
  return import('./useGatewayStatus');
}

beforeEach(() => {
  overviewMock.mockReset();
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useGatewayStatus (B-224)', () => {
  it('mulai dari "memeriksa", menjadi "online" hanya sesudah overview menjawab', async () => {
    overviewMock.mockResolvedValue({ pid: 1 });
    const { useGatewayStatus } = await muatHook();
    const { result } = renderHook(() => useGatewayStatus());
    expect(result.current).toBe('memeriksa');
    await act(async () => {});
    expect(result.current).toBe('online');
  });

  it('overview gagal → "offline", bukan klaim online', async () => {
    overviewMock.mockRejectedValue(new Error('jaringan putus'));
    const { useGatewayStatus } = await muatHook();
    const { result } = renderHook(() => useGatewayStatus());
    await act(async () => {});
    expect(result.current).toBe('offline');
  });

  it('status mengikuti polling: online lalu putus → offline', async () => {
    overviewMock.mockResolvedValue({ pid: 1 });
    const { useGatewayStatus } = await muatHook();
    const { result } = renderHook(() => useGatewayStatus());
    await act(async () => {});
    expect(result.current).toBe('online');

    overviewMock.mockRejectedValue(new Error('gateway mati'));
    await act(async () => {
      vi.advanceTimersByTime(30_000);
    });
    expect(result.current).toBe('offline');
  });

  it('dua pemakai berbagi satu polling (satu panggilan per siklus)', async () => {
    overviewMock.mockResolvedValue({ pid: 1 });
    const { useGatewayStatus } = await muatHook();
    renderHook(() => useGatewayStatus());
    renderHook(() => useGatewayStatus());
    await act(async () => {});
    expect(overviewMock).toHaveBeenCalledTimes(1);
  });
});
