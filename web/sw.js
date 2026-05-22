self.addEventListener('install', (e) => { self.skipWaiting(); });
self.addEventListener('activate', (e) => { e.waitUntil(self.clients.claim()); });

self.addEventListener('push', (event) => {
  let payload = { title: 'Српски', body: 'Време је за вежбу.' };
  try { if (event.data) payload = Object.assign(payload, event.data.json()); } catch (_) {}
  event.waitUntil(self.registration.showNotification(payload.title, {
    body: payload.body,
    tag: 'serbian-reminder',
  }));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  // Always open/focus the app's own scope root, which is automatically
  // subpath-aware (it's the URL the SW was registered under).
  const targetUrl = self.registration.scope;
  event.waitUntil((async () => {
    const all = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    for (const c of all) {
      // Only refocus a window already inside our scope; otherwise open fresh.
      if (c.url && c.url.startsWith(targetUrl) && 'focus' in c) {
        return c.focus();
      }
    }
    if (self.clients.openWindow) return self.clients.openWindow(targetUrl);
  })());
});
