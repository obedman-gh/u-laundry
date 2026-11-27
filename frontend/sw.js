'use strict';

// Change for EVERY frontend release, including a rollback. Never reuse a version.
const VERSION = '2026-11-27-2';
const PREFIX = 'ulaundry-shell-';
const CACHE = PREFIX + VERSION;
// Only public, data-free files. No app pages, business scripts, or runtime cache.
const SHELL = new Map([
  ['/offline.html', 'text/html'],
  ['/ui/tokens.css', 'text/css'],
  ['/ui/ui.css', 'text/css'],
  ['/pwa/pwa.css', 'text/css'],
  ['/pwa/pwa.js', 'javascript'],
  ['/manifest.webmanifest', 'json'],
  ['/ui/favicon.svg', 'image/svg+xml'],
  ['/icons/icon-192.png', 'image/png'],
  ['/icons/icon-512.png', 'image/png']
]);
const PAGES = new Set(['/', '/index.html', '/login.html', '/dashboard.html', '/pos.html',
  '/customers.html', '/services.html', '/staff.html', '/shifts.html', '/audit.html', '/tracking.html']);
// Public code only: bypass browser HTTP cache, with NO offline/runtime cache.
// Keep in sync when adding business entry points; API/receipt HTML never belongs here.
const NETWORK_ASSETS = new Set([
  '/ui/ui.js', '/ui/session.js', '/login.js', '/queue.js', '/queue-model.js',
  '/dashboard.js', '/reports-model.js', '/pos.js', '/pos-model.js', '/customers.js',
  '/services.js', '/staff.js', '/shifts.js', '/audit.js', '/tracking.js', '/receipt.js',
  '/customers.css', '/queue.css', '/reports.css', '/pos.css', '/staff.css',
  '/shifts.css', '/audit.css', '/receipt.css'
]);

self.addEventListener('install', event => {
  event.waitUntil((async () => {
    try {
      const cache = await caches.open(CACHE);
      await Promise.all([...SHELL].map(async ([path, type]) => {
        const response = await fetch(path, {cache: 'no-store', credentials: 'omit', redirect: 'error'});
        if (!response.ok || !(response.headers.get('Content-Type') || '').includes(type)) {
          throw new Error('Offline shell unavailable');
        }
        await cache.put(path, response);
      }));
    } catch (error) {
      await caches.delete(CACHE);
      throw error; // Failed installation leaves the previous worker active.
    }
  })());
  // Updates wait for user consent; never skipWaiting automatically.
});

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    for (const key of await caches.keys()) {
      if (key.startsWith(PREFIX) && key !== CACHE) await caches.delete(key);
    }
    await self.clients.claim();
  })());
});

self.addEventListener('message', event => {
  if (event.data?.type === 'ACTIVATE_UPDATE') event.waitUntil(self.skipWaiting());
});

function noStore(response) {
  if (response.type === 'opaque' || response.type === 'opaqueredirect') return response;
  const headers = new Headers(response.headers);
  headers.set('Cache-Control', 'no-store');
  return new Response(response.body, {status: response.status, statusText: response.statusText, headers});
}

async function networkFirst(request, fallback, navigation) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 8000);
  try {
    const response = await fetch(request, {cache: 'no-store', signal: controller.signal});
    // Preserve auth/validation/404 responses. Only gateway/outage errors use shell.
    if (!navigation || ![502, 503, 504].includes(response.status)) return noStore(response);
  } catch (_) { /* Fetch failed: return a data-free, known shell file only. */ }
  finally { clearTimeout(timeout); }
  const cached = await (await caches.open(CACHE)).match(fallback);
  if (cached) return noStore(cached);
  return new Response('Koneksi tidak tersedia. Coba kembali saat online.', {
    status: 503, headers: {'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'no-store'}
  });
}

self.addEventListener('fetch', event => {
  const request = event.request;
  const url = new URL(request.url);
  // No respondWith for API/auth/receipt/report/unknown/cross-origin requests or writes.
  if (request.method !== 'GET' || url.origin !== self.location.origin ||
      request.headers.has('Authorization') || request.headers.has('Range')) return;
  if (request.mode === 'navigate' && PAGES.has(url.pathname)) {
    event.respondWith(networkFirst(request, '/offline.html', true));
  } else if (!url.search && SHELL.has(url.pathname)) {
    event.respondWith(networkFirst(request, url.pathname, false));
  } else if (!url.search && NETWORK_ASSETS.has(url.pathname)) {
    event.respondWith(fetch(request, {cache: 'no-store'}).then(noStore));
  }
});
