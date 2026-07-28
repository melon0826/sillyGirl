const WATCHDOG_ENDPOINTS = [
  'https://watchdog.apiplus.plus',
  'http://watchdog.apiplus.plus',
  'https://watchdog.smallfawn.workers.dev',
];

const HEALTH_TIMEOUT_MS = 3000;

declare global {
  interface Window {
    __SILLYGIRL_WATCHDOG__?: Promise<void>;
  }
}

export function bootWatchdog(userType: 'home' | 'user' | 'admin') {
  if (typeof window === 'undefined' || window.__SILLYGIRL_WATCHDOG__) return;
  window.__SILLYGIRL_WATCHDOG__ = loadWatchdog(userType);
}

async function loadWatchdog(userType: string) {
  const base = await selectWatchdogEndpoint();
  if (!base) return;

  const script = document.createElement('script');
  script.async = true;
  script.src = `${base}/watchdog.js`;
  script.dataset.siteId = 'sillygirl';
  script.dataset.userType = userType;
  script.dataset.endpoint = `${base}/collect`;
  document.head.appendChild(script);
}

async function selectWatchdogEndpoint() {
  for (const raw of WATCHDOG_ENDPOINTS) {
    const base = raw.replace(/\/+$/, '');
    if (window.location.protocol === 'https:' && base.startsWith('http://')) continue;
    if (await isWatchdogHealthy(base)) return base;
  }
  return '';
}

async function isWatchdogHealthy(base: string) {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), HEALTH_TIMEOUT_MS);
  try {
    const response = await fetch(`${base}/health`, {
      cache: 'no-store',
      mode: 'cors',
      signal: controller.signal,
    });
    if (!response.ok) return false;
    const data = await response.json().catch(() => null);
    return data?.status === true;
  } catch {
    return false;
  } finally {
    window.clearTimeout(timeout);
  }
}
