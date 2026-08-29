import { render, waitFor } from '@solidjs/testing-library';
import { query } from '@solidjs/router';
import { beforeEach, describe, expect, test } from 'vitest';
import { AlbumOrder } from '@proto/library/v1/library_pb';
import type { ListAlbumsRequest } from '@proto/library/v1/library_pb';

import App from '../../App';
import { clearListCache } from '../../lib/pagination';
import { fakeServices } from '../../lib/rpc/testing';

describe('<Albums />', () => {
  beforeEach(() => {
    query.clear();
    clearListCache();
  });

  test('it lists albums grouped by artist', async () => {
    let got: ListAlbumsRequest | undefined;
    fakeServices({
      library: {
        listAlbums: (req) => {
          got = req;
          return { items: [], nextPageToken: '' };
        },
        listArtists: () => ({ items: [], nextPageToken: '' }),
      },
      management: { listScans: () => ({ items: [], nextPageToken: '' }) },
    });

    window.history.pushState({}, '', '/albums');
    render(() => <App />);
    await waitFor(() => expect(got).toBeDefined());

    expect(got?.order).toBe(AlbumOrder.ARTIST);
    expect(got?.descending).toBe(false);
  });
});
