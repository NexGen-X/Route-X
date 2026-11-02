import { describe, it, expect, vi, afterEach } from 'vitest';
import { formatTimeAgo } from './utils';

describe('formatTimeAgo (audit3 B-053)', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('memakai satuan hari untuk umur >= 24 jam, bukan jam raksasa', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-11-05T12:00:00Z'));
    // 73 jam sebelumnya harus tampil "3 hari lalu", bukan "73j lalu".
    expect(formatTimeAgo('2026-11-02T11:00:00Z')).toBe('3 hari lalu');
    expect(formatTimeAgo('2026-11-04T12:00:00Z')).toBe('1 hari lalu');
  });

  it('satuan di bawah sehari tidak berubah', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-11-05T12:00:00Z'));
    expect(formatTimeAgo('2026-11-05T11:59:58Z')).toBe('baru saja');
    expect(formatTimeAgo('2026-11-05T11:59:30Z')).toBe('30 detik lalu');
    expect(formatTimeAgo('2026-11-05T11:45:00Z')).toBe('15m lalu');
    expect(formatTimeAgo('2026-11-05T07:00:00Z')).toBe('5j lalu');
    expect(formatTimeAgo('bukan-waktu')).toBe('waktu tak dikenal');
  });
});
