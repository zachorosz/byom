import { revalidate } from '@solidjs/router';

import { clearListCache } from '../pagination';

/**
 * invalidateLibrary drops every cached view of the music library.
 *
 * Called when a scan finishes: albums, artists and tracks may all have changed
 * underneath, and page tokens issued before the scan are no longer meaningful.
 */
export function invalidateLibrary(): void {
  revalidate(['album', 'artist', 'tracks']);
  clearListCache();
}
