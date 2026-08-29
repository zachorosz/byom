import { render } from '@solidjs/testing-library';
import { query } from '@solidjs/router';
import { beforeEach, describe, expect, test } from 'vitest';

import App from '../App';
import { fakeServices } from '../lib/rpc/testing';

/** at renders the app as if the browser were at the given path. */
function at(path: string) {
  window.history.pushState({}, '', path);
  return render(() => <App />);
}

describe('routing', () => {
  beforeEach(() => {
    query.clear();
    fakeServices({
      library: {
        listAlbums: () => ({ items: [], nextPageToken: '' }),
        listArtists: () => ({ items: [], nextPageToken: '' }),
      },
      management: { listScans: () => ({ items: [], nextPageToken: '' }) },
    });
  });

  test('/ redirects to the album grid', async () => {
    const { findByRole } = at('/');
    expect(await findByRole('heading', { name: 'Albums' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/albums');
  });

  test('/albums renders the album list, not the 404 page', async () => {
    const { findByRole, queryByText } = at('/albums');
    expect(await findByRole('heading', { name: 'Albums' })).toBeInTheDocument();
    expect(queryByText('Not found')).toBeNull();
  });

  test('/artists renders the artist list, not the 404 page', async () => {
    const { findByRole, queryByText } = at('/artists');
    expect(
      await findByRole('heading', { name: 'Artists' })
    ).toBeInTheDocument();
    expect(queryByText('Not found')).toBeNull();
  });
});
