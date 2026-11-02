/* Service worker Route-X — sengaja konservatif.
   Hanya aset statis yang boleh masuk cache. API dan semua request
   non-navigasi lain selalu lewat jaringan agar tidak ada risiko
   menyajikan data basi atau mengganggu sesi gateway.

   Strategi per kelompok:
   - /assets/ adalah hasil build Vite bernama hash — immutable, jadi
     cache-first aman selamanya.
   - /icons/ bernama TETAP (icon-192.png dst.): network-first dengan
     fallback cache. Cache-first permanen di sini membuat ikon baru
     tidak pernah sampai ke pengguna lama.
   - Navigasi (mode "navigate"): network-first; saat jaringan gagal,
     fallback ke app shell "/" yang di-precache saat install, supaya
     PWA terpasang tetap terbuka offline dan router sisi klien mengambil
     alih (audit F8/V-FF-43). Shell yang ter-cache hanyalah dokumen
     index — datanya tetap selalu dari API saat online.

   Nama cache membawa sidik build: token __ROUTEX_VERSI_SW__ diganti
   skrip scripts/cap-versi-sw.mjs pada tahap postbuild dengan hash isi
   dist/. Setiap deploy dengan aset berbeda menghasilkan nama cache
   baru, dan activate menghapus seluruh cache generasi lama — entri
   /assets/ lintas generasi tidak lagi menumpuk tanpa batas di cache
   yang sama (audit V-FF-44). */
const CACHE = 'routex-static-__ROUTEX_VERSI_SW__';
const ASSETS_RE = /^\/assets\//;
const ICONS_RE = /^\/icons\//;
const SHELL = ['/', '/manifest.webmanifest', '/icons/icon-192.png'];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((cache) => cache.addAll(SHELL))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  if (request.mode === 'navigate') {
    // Navigasi: jaringan dulu (HTML tidak pernah basi), shell cache
    // hanya cadangan saat benar-benar offline.
    event.respondWith(
      fetch(request).catch(async () => {
        const cache = await caches.open(CACHE);
        const shell = await cache.match('/');
        if (shell) return shell;
        throw new Error('offline dan app shell belum ter-cache');
      })
    );
    return;
  }

  if (ASSETS_RE.test(url.pathname)) {
    // Aset ber-hash: cache-first.
    event.respondWith(
      caches.open(CACHE).then(async (cache) => {
        const hit = await cache.match(request);
        if (hit) return hit;
        const resp = await fetch(request);
        if (resp.ok) cache.put(request, resp.clone());
        return resp;
      })
    );
    return;
  }

  if (ICONS_RE.test(url.pathname)) {
    // Ikon bernama tetap: network-first, cache hanya cadangan offline.
    event.respondWith(
      caches.open(CACHE).then(async (cache) => {
        try {
          const resp = await fetch(request);
          if (resp.ok) cache.put(request, resp.clone());
          return resp;
        } catch (err) {
          const hit = await cache.match(request);
          if (hit) return hit;
          throw err;
        }
      })
    );
  }
});
