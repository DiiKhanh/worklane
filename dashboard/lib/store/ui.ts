import { create } from "zustand";

/**
 * Client/UI state only. Server data (api keys, requests, logs) never lives here -
 * it belongs to the TanStack Query cache. Theme is owned by next-themes.
 */
export type RequestFilters = {
  state?: string;
  search?: string;
};

type UIState = {
  filters: RequestFilters;
  setFilter: (key: keyof RequestFilters, value?: string) => void;
  resetFilters: () => void;
  token: string;
  setToken: (token: string) => void;
};

export const useUIStore = create<UIState>((set) => ({
  filters: {},
  setFilter: (key, value) =>
    set((s) => ({ filters: { ...s.filters, [key]: value } })),
  resetFilters: () => set({ filters: {} }),
  token: "",
  setToken: (token) => set({ token }),
}));
