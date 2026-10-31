/* Service worker Route-X — sengaja konservatif.
   Hanya aset statis immutable (hasil build Vite dan ikon) yang boleh masuk cache.
   HTML, API, dan semua request lain selalu lewat jaringan agar tidak ada risiko
   menyajikan data basi atau mengganggu sesi gateway. */
const CACHE = 'routex-static-v1';
const STATIC_RE = /^\/(assets|icons)\//;

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
  if (!STATIC_RE.test(url.pathname)) return;
  event.respondWith(
    caches.open(CACHE).then(async (cache) => {
      const hit = await cache.match(request);
      if (hit) return hit;
      const resp = await fetch(request);
      if (resp.ok) cache.put(request, resp.clone());
      return resp;
    })
  );
});
