import { createClient, type Transport } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';

import { LibraryService } from '@proto/library/v1/library_pb';
import { ManagementService } from '@proto/management/v1/management_pb';

// Relative baseUrl keeps requests same-origin: the Vite dev proxy forwards
// them to the Go server in dev, and the Go binary will serve both the SPA
// and the RPC handlers from one origin in production.
const defaultTransport = createConnectTransport({ baseUrl: '/' });

/** rpc holds the service clients. Read through it so tests can swap the transport. */
export const rpc = {
  library: createClient(LibraryService, defaultTransport),
  management: createClient(ManagementService, defaultTransport),
};

/** setTransport rebuilds both clients on a new transport. Tests use it; the app does not. */
export function setTransport(transport: Transport): void {
  rpc.library = createClient(LibraryService, transport);
  rpc.management = createClient(ManagementService, transport);
}
