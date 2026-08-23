import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';

import { LibraryService } from '@proto/library/v1/library_pb';
import { ManagementService } from '@proto/management/v1/management_pb';

// Relative baseUrl keeps requests same-origin: the Vite dev proxy forwards
// them to the Go server in dev, and the Go binary will serve both the SPA
// and the RPC handlers from one origin in production.
const transport = createConnectTransport({ baseUrl: '/' });

export const libraryClient = createClient(LibraryService, transport);
export const managementClient = createClient(ManagementService, transport);
