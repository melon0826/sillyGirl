import { mountSillyGirlVersion, SILLYGIRL_VERSION } from './version';

const WATCHDOG_ENDPOINTS = [
  'https://watchdog.apiplus.plus',
  'http://watchdog.apiplus.plus',
  'https://watchdog.smallfawn.workers.dev',
];

const SCRIPT_TIMEOUT_MS = 3000;

declare global {
  interface Window {
    __SILLYGIRL_WATCHDOG__?: Promise<void>;
  }
}

export function bootWatchdog(userType: 'home' | 'user' | 'admin') {
  if (typeof window === 'undefined' || window.__SILLYGIRL_WATCHDOG__) return;
  mountSillyGirlVersion();
  window.__SILLYGIRL_WATCHDOG__ = loadWatchdog(userType);
}

async function loadWatchdog(userType: string) {
  for (const raw of WATCHDOG_ENDPOINTS) {
    const base = raw.replace(/\/+$/, '');
    if (window.location.protocol === 'https:' && base.startsWith('http://')) continue;
    if (await appendWatchdogScript(base, userType)) return;
  }
}

function appendWatchdogScript(base: string, userType: string) {
  return new Promise<boolean>((resolve) => {
    const script = document.createElement('script');
    const timeout = window.setTimeout(() => {
      script.remove();
      resolve(false);
    }, SCRIPT_TIMEOUT_MS);

    script.async = true;
    script.src = `${base}/watchdog.js`;
    script.dataset.siteId = 'sillygirl';
    script.dataset.userType = userType;
    script.dataset.version = watchdogVersion();
    script.dataset.endpoint = `${base}/collect`;
    script.onload = () => {
      window.clearTimeout(timeout);
      resolve(true);
    };
    script.onerror = () => {
      window.clearTimeout(timeout);
      script.remove();
      resolve(false);
    };
    document.head.appendChild(script);
  });
}

function watchdogVersion() {
  return window.__SILLYGIRL_VERSION__ || window.SILLYGIRL_VERSION || SILLYGIRL_VERSION;
}
