import { render } from '@solidjs/testing-library';
import { query } from '@solidjs/router';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import { AlbumOrder } from '@proto/library/v1/library_pb';
import type { ListAlbumsRequest } from '@proto/library/v1/library_pb';

import App from '../../App';
import { clearListCache } from '../../lib/pagination';
import { fakeServices } from '../../lib/rpc/testing';

const artistID = '00000000-0000-0000-0000-0000000000a1';

describe('<ArtistDetail />', () => {
  beforeEach(() => {
    query.clear();
    clearListCache();
  });

  test('every section lists its albums in original release order', async () => {
    const requests: ListAlbumsRequest[] = [];
    fakeServices({
      library: {
        getArtist: () => ({
          artist: { id: artistID, name: 'King Gizzard & the Lizard Wizard' },
        }),
        listAlbums: (req) => {
          requests.push(req);
          return { items: [], nextPageToken: '' };
        },
        listArtists: () => ({ items: [], nextPageToken: '' }),
      },
      management: { listScans: () => ({ items: [], nextPageToken: '' }) },
    });

    window.history.pushState({}, '', `/artists/${artistID}`);
    render(() => <App />);

    // One request per section, each oldest first: a discography reads
    // chronologically, and it keeps undated albums off the top.
    await vi.waitFor(() => expect(requests.length).toBe(4));
    for (const req of requests) {
      expect(req.order).toBe(AlbumOrder.ORIGINAL_DATE);
      expect(req.descending).toBe(false);
    }
  });
});
