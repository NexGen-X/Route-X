import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Regresi audit3 grup 14 (B-059): berkas PWA di public/ adalah kontrak
// yang dilanggar diam-diam sebelumnya — favicon.ico & robots.txt tidak
// ada sama sekali (keduanya 404), dan sw.js tidak punya handler navigasi
// maupun versi cache per build. Tes ini membaca berkas sumbernya
// langsung supaya ketiadaan/perubahan regresif tertangkap di CI.
// cwd saat vitest berjalan adalah direktori web/.
const publik = (nama: string) => join(process.cwd(), 'public', nama);

describe('aset PWA di web/public', () => {
  it('favicon.ico ada dan berformat ICO', () => {
    const path = publik('favicon.ico');
    expect(existsSync(path)).toBe(true);
    const buf = readFileSync(path);
    expect(buf.length).toBeGreaterThan(100);
    // Magic ICO: reserved 0x0000 + type 0x0001.
    expect([...buf.subarray(0, 4)]).toEqual([0, 0, 1, 0]);
  });

  it('robots.txt ada dan melarang pengindeksan (konsol admin privat)', () => {
    const isi = readFileSync(publik('robots.txt'), 'utf8');
    expect(isi).toContain('User-agent: *');
    expect(isi).toContain('Disallow: /');
  });

  it('sw.js membawa token versi build, precache shell, dan handler navigasi', () => {
    const sw = readFileSync(publik('sw.js'), 'utf8');
    // Nama cache WAJIB memuat token yang diganti postbuild dengan sidik
    // isi dist/ — tanpa itu generasi aset menumpuk di satu cache abadi.
    expect(sw).toContain('__ROUTEX_VERSI_SW__');
    // Precache app shell saat install + fallback navigasi offline.
    expect(sw).toContain('addAll');
    expect(sw).toContain("request.mode === 'navigate'");
    // Activate membersihkan cache generasi lain.
    expect(sw).toContain('caches.delete');
  });
});
