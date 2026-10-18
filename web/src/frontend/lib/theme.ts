import { useSyncExternalStore } from "react";

export type Theme = "light" | "dark" | "system";

const storageKey = "m2m-theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");
const listeners = new Set<() => void>();

function stored(): Theme {
  try {
    const v = localStorage.getItem(storageKey);
    return v === "light" || v === "dark" ? v : "system";
  } catch {
    return "system";
  }
}

let current: Theme = stored();

function apply() {
  const dark = current === "dark" || (current === "system" && media.matches);
  document.documentElement.classList.toggle("dark", dark);
}

apply();
media.addEventListener("change", apply);

export function setTheme(theme: Theme) {
  current = theme;
  try {
    localStorage.setItem(storageKey, theme);
  } catch {}
  apply();
  listeners.forEach((l) => l());
}

export function useTheme(): Theme {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
}
