import { createRouterTransport, type ServiceImpl } from '@connectrpc/connect';
import { LibraryService } from '@proto/library/v1/library_pb';
import { ManagementService } from '@proto/management/v1/management_pb';

import { setTransport } from './client';

interface FakeServices {
  library?: Partial<ServiceImpl<typeof LibraryService>>;
  management?: Partial<ServiceImpl<typeof ManagementService>>;
}

/**
 * fakeServices points the clients at in-memory implementations.
 *
 * Both services are installed in one transport, so a test needing both must
 * pass both in a single call — each call replaces the previous transport. Only
 * the methods a test needs have to be supplied; calling an unsupplied method
 * throws, which surfaces as a failing test rather than a silent stub.
 */
export function fakeServices(impls: FakeServices): void {
  setTransport(
    createRouterTransport(({ service }) => {
      if (impls.library) {
        service(
          LibraryService,
          impls.library as ServiceImpl<typeof LibraryService>
        );
      }
      if (impls.management) {
        service(
          ManagementService,
          impls.management as ServiceImpl<typeof ManagementService>
        );
      }
    })
  );
}

/** fakeLibrary installs only a LibraryService. Shorthand for fakeServices({ library }). */
export function fakeLibrary(
  library: Partial<ServiceImpl<typeof LibraryService>>
): void {
  fakeServices({ library });
}

/** fakeManagement installs only a ManagementService. */
export function fakeManagement(
  management: Partial<ServiceImpl<typeof ManagementService>>
): void {
  fakeServices({ management });
}
