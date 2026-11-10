// Состояние интерфейса (Zustand): текущий раздел, тосты, связь с сервером.
import { createStore, useStore } from 'zustand';

export type View = 'chat' | 'plan' | 'requests' | 'knowledge' | 'inbox' | 'approvals' | 'team' | 'insights';

export interface Toast {
  id: number;
  text: string;
  tone: 'info' | 'error';
}

interface UiState {
  view: View;
  internId: string | null;
  docId: string | null;
  online: boolean;
  toasts: Toast[];
  draft: string;
  go: (view: View, opts?: { internId?: string | null; docId?: string | null }) => void;
  setOnline: (v: boolean) => void;
  toast: (text: string, tone?: Toast['tone']) => void;
  dismiss: (id: number) => void;
  setDraft: (v: string) => void;
}

let seq = 0;

export const uiStore = createStore<UiState>((set) => ({
  view: 'chat',
  internId: null,
  docId: null,
  online: true,
  toasts: [],
  draft: '',
  go: (view, opts = {}) => set({ view, internId: opts.internId ?? null, docId: opts.docId ?? null }),
  setOnline: (online) => set({ online }),
  toast: (text, tone = 'info') => {
    const id = ++seq;
    set((s) => ({ toasts: [...s.toasts.slice(-2), { id, text, tone }] }));
    if (typeof setTimeout !== 'undefined') setTimeout(() => uiStore.getState().dismiss(id), 5000);
  },
  dismiss: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  setDraft: (draft) => set({ draft }),
}));

const identity = (s: UiState) => s;

/** Хук стора интерфейса: useUi() — всё состояние, useUi(sel) — срез. */
export function useUi(): UiState;
export function useUi<T>(selector: (s: UiState) => T): T;
export function useUi<T>(selector?: (s: UiState) => T) {
  return useStore(uiStore, selector ?? (identity as unknown as (s: UiState) => T));
}
useUi.getState = uiStore.getState;
