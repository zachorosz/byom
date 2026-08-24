import { query } from '@solidjs/router';
import { ScanState } from '@proto/management/v1/scan_pb';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { getAlbum } from './library';
import { clearListCache } from '../pagination';
import { runningScans, startScanMonitor } from './scan-monitor';
import { fakeManagement, fakeServices } from './testing';

const scan = (state: ScanState) => ({
  id: 's1',
  locationId: 'l1',
  state,
  error: '',
  progress: { dirsSeen: 10n, dirsMissing: 0n, filesSeen: 100n, filesMissing: 0n },
});

async function tick(ms: number) {
  await vi.advanceTimersByTimeAsync(ms);
}

describe('scan monitor', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    query.clear();
    clearListCache();
  });
  afterEach(() => vi.useRealTimers());

  test('it polls once on start and exposes the running scans', async () => {
    fakeManagement({
      listScans: () => ({ items: [scan(ScanState.RUNNING)], nextPageToken: '' }),
    });
    const stop = startScanMonitor();
    await tick(0);
    expect(runningScans()).toHaveLength(1);
    stop();
  });

  test('it keeps polling while a scan is running', async () => {
    let calls = 0;
    fakeManagement({
      listScans: () => {
        calls++;
        return { items: [scan(ScanState.RUNNING)], nextPageToken: '' };
      },
    });
    const stop = startScanMonitor();
    await tick(0);
    await tick(2000);
    await tick(2000);
    expect(calls).toBeGreaterThanOrEqual(3);
    stop();
  });

  test('it stops polling once nothing is running', async () => {
    let calls = 0;
    fakeManagement({
      listScans: () => {
        calls++;
        return { items: [], nextPageToken: '' };
      },
    });
    const stop = startScanMonitor();
    await tick(0);
    const afterFirst = calls;
    await tick(10_000);
    expect(calls).toBe(afterFirst);
    stop();
  });

  // Asserted through observable behaviour, not cache internals: `revalidate`
  // forces a miss on the next call but deliberately keeps the stale value, so
  // query.get() still returns it. What must change is that the next read refetches.
  test('it invalidates the library when a running scan disappears', async () => {
    let running = true;
    let albumCalls = 0;
    fakeServices({
      library: {
        getAlbum: () => {
          albumCalls++;
          return { album: { id: 'a1', title: 'Kid A' } };
        },
      },
      management: {
        listScans: () => ({
          items: running ? [scan(ScanState.RUNNING)] : [],
          nextPageToken: '',
        }),
      },
    });

    await getAlbum('a1');
    await getAlbum('a1');
    expect(albumCalls).toBe(1); // second read served from cache

    const stop = startScanMonitor();
    await tick(0);
    running = false;
    await tick(2000);

    // The scan finished, so the library changed underneath and the cached album
    // must be refetched rather than served stale.
    await getAlbum('a1');
    expect(albumCalls).toBe(2);
    stop();
  });

  test('stopping the monitor halts polling', async () => {
    let calls = 0;
    fakeManagement({
      listScans: () => {
        calls++;
        return { items: [scan(ScanState.RUNNING)], nextPageToken: '' };
      },
    });
    const stop = startScanMonitor();
    await tick(0);
    stop();
    const afterStop = calls;
    await tick(10_000);
    expect(calls).toBe(afterStop);
  });
});
