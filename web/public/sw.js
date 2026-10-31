/* Service worker Route-X — sengaja konservatif.
   Hanya aset statis yang boleh masuk cache. HTML, API, dan semua request
   lain selalu lewat jaringan agar tidak ada risiko menyajikan data basi
   atau mengganggu sesi gateway.

   Strategi per kelompok (audit F8):
   - /assets/ adalah hasil build Vite bernama hash — immutable, jadi
     cache-first aman selamanya.
   - /icons/ bernama TETAP (icon-192.png dst.): network-first dengan
     fallback cache. Cache-first permanen di sini membuat ikon baru
     tidak pernah sampai ke pengguna lama. */
const CACHE = 'routex-static-v2';
const ASSETS_RE = /^\/assets\//;
const ICONS_RE = /^\/icons\//;

self.addEventListener('install', (event) => {
  self.skipWaiting();
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
