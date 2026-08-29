import { revalidate } from '@solidjs/router';
import { createSignal } from 'solid-js';
import type { Scan } from '@proto/management/v1/scan_pb';

import { invalidateLibrary } from './invalidate';
import { listRunningScans } from './management';

const POLL_INTERVAL_MS = 2000;

const [runningScans, setRunningScans] = createSignal<Scan[]>([]);
export { runningScans };

let timer: ReturnType<typeof setTimeout> | undefined;
let stopped = true;

async function poll(): Promise<void> {
  if (stopped) return;
  revalidate(listRunningScans.key, true);
  const previous = runningScans();
  const response = await listRunningScans();
  if (stopped) return;

  setRunningScans(response.items);

  // A scan leaving the running set means the library changed underneath.
  if (previous.length > 0 && response.items.length === 0) invalidateLibrary();

  if (response.items.length > 0)
    timer = setTimeout(() => void poll(), POLL_INTERVAL_MS);
}

/** pokeScanMonitor polls immediately, for use right after starting or cancelling a scan. */
export function pokeScanMonitor(): void {
  if (stopped) return;
  clearTimeout(timer);
  void poll();
}

function onVisibility(): void {
  if (document.visibilityState === 'visible') pokeScanMonitor();
  else clearTimeout(timer);
}

function stopMonitor(): void {
  stopped = true;
  clearTimeout(timer);
  document.removeEventListener('visibilitychange', onVisibility);
}

/**
 * startScanMonitor begins watching for running scans and returns a stop function.
 *
 * Polls only while at least one scan is running, and pauses while the tab is
 * hidden. Call once, from the app shell.
 */
export function startScanMonitor(): () => void {
  stopped = false;
  document.addEventListener('visibilitychange', onVisibility);
  void poll();
  return stopMonitor;
}
