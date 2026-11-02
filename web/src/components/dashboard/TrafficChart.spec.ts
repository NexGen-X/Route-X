import { describe, it, expect } from 'vitest';
import { formatWaktuSumbu, formatLabelTooltip } from './TrafficChart';

describe('formatter waktu TrafficChart (audit3 V-FJ-07)', () => {
  it('sumbu menampilkan waktu lokal, bukan string RFC3339 mentah', () => {
    const out = formatWaktuSumbu('2026-11-02T04:05:00Z', '24h');
    expect(out).not.toContain('2026');
    expect(out).not.toContain('T04:05');
    expect(out).toMatch(/\d{2}:\d{2}/);
  });

  it('jendela 7d menampilkan tanggal singkat', () => {
    const out = formatWaktuSumbu('2026-11-02T04:05:00Z', '7d');
    expect(out).not.toContain('T04:05');
    expect(out.length).toBeGreaterThan(0);
  });

  it('label tooltip memakai format tanggal+waktu lokal lengkap', () => {
    const out = formatLabelTooltip('2026-11-02T04:05:00Z');
    expect(out).not.toBe('2026-11-02T04:05:00Z');
    expect(out).toContain('2026');
  });

  it('input cacat dikembalikan apa adanya', () => {
    expect(formatWaktuSumbu('bukan-waktu', '24h')).toBe('bukan-waktu');
    expect(formatLabelTooltip('')).toBe('');
  });
});
