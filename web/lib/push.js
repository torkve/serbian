// Web Push subscription helpers. Browser-side counterpart to internal/push.

function urlBase64ToUint8Array(base64) {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const arr = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
  return arr;
}

async function getRegistration() {
  if (!('serviceWorker' in navigator)) throw new Error('Service Worker није подржан.');
  const reg = await navigator.serviceWorker.ready;
  if (!reg) throw new Error('Service Worker није активан.');
  return reg;
}

export async function pushStatus() {
  if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
    return { supported: false, subscribed: false };
  }
  try {
    const reg = await navigator.serviceWorker.getRegistration();
    if (!reg) return { supported: true, subscribed: false };
    const sub = await reg.pushManager.getSubscription();
    return { supported: true, subscribed: !!sub };
  } catch (e) {
    return { supported: true, subscribed: false, error: e.message };
  }
}

export async function subscribe() {
  if (!('PushManager' in window)) throw new Error('Push није подржан у овом прегледачу.');
  const perm = await Notification.requestPermission();
  if (perm !== 'granted') throw new Error('Дозвола за обавештења није дата.');

  const vapidResp = await fetch('api/push/vapid');
  if (!vapidResp.ok) throw new Error('Не могу да добијем VAPID кључ.');
  const { public_key } = await vapidResp.json();

  const reg = await getRegistration();
  const sub = await reg.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(public_key),
  });

  // Convert PushSubscription to the shape the backend expects.
  const subJSON = sub.toJSON();
  const body = {
    endpoint: subJSON.endpoint,
    keys: {
      p256dh: subJSON.keys.p256dh,
      auth: subJSON.keys.auth,
    },
  };
  const r = await fetch('api/push/subscribe', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!r.ok) {
    const text = await r.text().catch(() => '');
    throw new Error(`Грешка слања претплате: ${r.status} ${text}`);
  }
  return sub;
}

export async function unsubscribe() {
  const reg = await navigator.serviceWorker.getRegistration();
  if (!reg) return;
  const sub = await reg.pushManager.getSubscription();
  if (!sub) return;
  const endpoint = sub.endpoint;
  await sub.unsubscribe();
  await fetch('api/push/unsubscribe', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint }),
  });
}

export async function sendTest() {
  const r = await fetch('api/push/test', { method: 'POST' });
  if (!r.ok) {
    const text = await r.text().catch(() => '');
    throw new Error(`Тест није прошао: ${r.status} ${text}`);
  }
}
