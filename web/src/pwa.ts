// PWA: регистрация service worker, установка приложения, ярлыки.
import { useEffect, useState } from 'react';
import type { View } from './store/ui';

interface SWContainer {
  register(url: string): Promise<unknown>;
}

/** Регистрирует service worker в production-сборке. Возвращает true, если регистрация запланирована. */
export function registerServiceWorker(
  nav: { serviceWorker?: SWContainer } = typeof navigator === 'undefined' ? {} : (navigator as unknown as { serviceWorker?: SWContainer }),
  isProd: boolean = process.env.NODE_ENV === 'production',
  onLoad: (cb: () => void) => void = (cb) => window.addEventListener('load', cb),
): boolean {
  if (!isProd || !nav.serviceWorker) return false;
  const sw = nav.serviceWorker;
  onLoad(() => {
    sw.register('/sw.js').catch(() => undefined);
  });
  return true;
}

const views: View[] = ['chat', 'plan', 'requests', 'knowledge', 'inbox', 'approvals', 'team', 'insights'];

/** Раздел из ярлыка приложения (?view=plan). */
export function viewFromSearch(search: string): View | null {
  const v = new URLSearchParams(search).get('view');
  return v && (views as string[]).includes(v) ? (v as View) : null;
}

export interface InstallPromptEvent extends Event {
  prompt(): Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

/** Предложение установить приложение (Chrome/Edge/Android). */
export function useInstallPrompt(target: EventTarget | undefined = typeof window === 'undefined' ? undefined : window) {
  const [evt, setEvt] = useState<InstallPromptEvent | null>(null);
  useEffect(() => {
    if (!target) return;
    const onPrompt = (e: Event) => {
      e.preventDefault();
      setEvt(e as InstallPromptEvent);
    };
    const onInstalled = () => setEvt(null);
    target.addEventListener('beforeinstallprompt', onPrompt);
    target.addEventListener('appinstalled', onInstalled);
    return () => {
      target.removeEventListener('beforeinstallprompt', onPrompt);
      target.removeEventListener('appinstalled', onInstalled);
    };
  }, [target]);
  return {
    canInstall: evt !== null,
    install: async () => {
      if (!evt) return false;
      await evt.prompt();
      const { outcome } = await evt.userChoice;
      setEvt(null);
      return outcome === 'accepted';
    },
  };
}
