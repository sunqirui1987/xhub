import { useSyncExternalStore } from "react";
import { getLocalStorageItem, LOCAL_STORAGE_EVENT } from "@/utils/localStorageUtils";

export const HIDE_AUTO_ROUTER_ANNOUNCEMENT_KEY = "litellmHideAutoRouterAnnouncement";

function subscribe(callback: () => void) {
  const onStorage = (e: StorageEvent) => {
    if (e.key === HIDE_AUTO_ROUTER_ANNOUNCEMENT_KEY) {
      callback();
    }
  };

  const onCustom = (e: Event) => {
    const { key } = (e as CustomEvent).detail;
    if (key === HIDE_AUTO_ROUTER_ANNOUNCEMENT_KEY) {
      callback();
    }
  };

  window.addEventListener("storage", onStorage);
  window.addEventListener(LOCAL_STORAGE_EVENT, onCustom);

  return () => {
    window.removeEventListener("storage", onStorage);
    window.removeEventListener(LOCAL_STORAGE_EVENT, onCustom);
  };
}

function getSnapshot() {
  return getLocalStorageItem(HIDE_AUTO_ROUTER_ANNOUNCEMENT_KEY) === "true";
}

// The server has no localStorage. A constant snapshot lets the navbar render
// there; the client reads the stored flag after hydration.
function getServerSnapshot() {
  return false;
}

export function useHideAutoRouterAnnouncement() {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
