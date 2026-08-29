import { query } from '@solidjs/router';
import { ScanState } from '@proto/management/v1/scan_pb';

import { rpc } from './client';

/** listLocations reads every configured library source. */
export const listLocations = query(
  () => rpc.management.listLocations({ pageSize: 100 }),
  'locations'
);

/** listRunningScans reads the scans currently in flight. Polled by the scan monitor. */
export const listRunningScans = query(
  () => rpc.management.listScans({ state: ScanState.RUNNING, pageSize: 50 }),
  'running-scans'
);

/** listScansFor reads a location's scan history, most recent first. */
export const listScansFor = query(
  (locationId: string) =>
    rpc.management.listScans({ locationId, pageSize: 20 }),
  'scans'
);

/** scanLocation starts a scan and returns it. */
export function scanLocation(locationId: string, force?: boolean) {
  return rpc.management.scanLocation({ locationId, force });
}

/** cancelScan requests cancellation of a running scan. */
export function cancelScan(id: string) {
  return rpc.management.cancelScan({ id });
}

/** createLocation adds a library source at the given filesystem path. */
export function createLocation(path: string) {
  return rpc.management.createLocation({ location: { path } as never });
}

/** deleteLocation removes a library source. */
export function deleteLocation(id: string) {
  return rpc.management.deleteLocation({ id });
}
