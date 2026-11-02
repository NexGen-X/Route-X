// Menanam sidik build ke nama cache service worker (dist/sw.js).
//
// sw.js di public/ disalin Vite apa adanya, jadi nama cache statis di
// sumber akan hidup lintas deploy: entri /assets/ dari semua generasi
// build menumpuk di cache yang sama tanpa pernah dibersihkan
// (audit3 V-FF-44). Skrip ini — dijalankan sebagai postbuild — mengganti
// token __ROUTEX_VERSI_SW__ dengan hash atas daftar berkas dist/assets/
// ditambah isi index.html, sehingga setiap build dengan aset berbeda
// memakai nama cache baru dan activate SW menghapus generasi lama.
//
// Gagal keras (exit 1) bila sw.js atau tokennya tidak ditemukan: lebih
// baik build berhenti daripada mengirim service worker tak berversi
// secara senyap.
import { createHash } from 'node:crypto';
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { join, relative } from 'node:path';

const dist = new URL('../dist/', import.meta.url).pathname;
const swPath = join(dist, 'sw.js');
const TOKEN = '__ROUTEX_VERSI_SW__';

async function daftarBerkas(dir) {
  const hasil = [];
  for (const entri of await readdir(dir, { withFileTypes: true })) {
    const penuh = join(dir, entri.name);
    if (entri.isDirectory()) {
      hasil.push(...(await daftarBerkas(penuh)));
    } else {
      hasil.push(relative(dist, penuh));
    }
  }
  return hasil;
}

const sw = await readFile(swPath, 'utf8').catch(() => null);
if (sw === null) {
  console.error(`[cap-versi-sw] ${swPath} tidak ditemukan — build tidak lengkap`);
  process.exit(1);
}
if (!sw.includes(TOKEN)) {
  console.error('[cap-versi-sw] token versi tidak ditemukan di dist/sw.js — periksa web/public/sw.js');
  process.exit(1);
}

const hash = createHash('sha256');
for (const nama of (await daftarBerkas(join(dist, 'assets'))).sort()) {
  hash.update(nama);
  hash.update('\0');
}
hash.update(await readFile(join(dist, 'index.html')));
const versi = hash.digest('hex').slice(0, 16);

await writeFile(swPath, sw.replaceAll(TOKEN, versi));
console.log(`[cap-versi-sw] nama cache service worker: routex-static-${versi}`);
