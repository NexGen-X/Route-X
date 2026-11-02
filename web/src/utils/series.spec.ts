import { describe, it, expect } from 'vitest';
import { bucketSecondsFromSeries } from './series';

describe('bucketSecondsFromSeries (audit3 V-FJ-12)', () => {
  it('menurunkan lebar bucket dari jarak timestamp nyata', () => {
    // Bucket backend untuk rentang <= 48 jam adalah 1 jam — throughput
    // harus dibagi 3600, bukan konstanta 60 lama yang membengkakkan 60x.
    expect(
      bucketSecondsFromSeries([
        { timestamp: '2026-11-02T00:00:00Z' },
        { timestamp: '2026-11-02T01:00:00Z' },
        { timestamp: '2026-11-02T02:00:00Z' },
      ]),
    ).toBe(3600);
    expect(
      bucketSecondsFromSeries([
        { timestamp: '2026-11-02T00:00:00Z' },
        { timestamp: '2026-11-03T00:00:00Z' },
      ]),
    ).toBe(86400);
    expect(
      bucketSecondsFromSeries([
        { timestamp: '2026-11-02T00:00:00Z' },
        { timestamp: '2026-11-02T00:01:00Z' },
      ]),
    ).toBe(60);
  });

  it('jatuh ke 3600 bila jarak tidak bisa diturunkan', () => {
    expect(bucketSecondsFromSeries([])).toBe(3600);
    expect(bucketSecondsFromSeries([{ timestamp: '2026-11-02T00:00:00Z' }])).toBe(3600);
    expect(
      bucketSecondsFromSeries([
        { timestamp: 'bukan-waktu' },
        { timestamp: 'juga-bukan' },
      ]),
    ).toBe(3600);
  });
});
