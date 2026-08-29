import { query } from '@solidjs/router';
import { beforeEach, describe, expect, test } from 'vitest';

import { getAlbum, listTracks } from './library';
import { fakeLibrary } from './testing';

describe('library queries', () => {
  beforeEach(() => query.clear());

  test('getAlbum returns the album from the service', async () => {
    fakeLibrary({
      getAlbum: () => ({ album: { id: 'a1', title: 'Kid A' } }),
    });
    const response = await getAlbum('a1');
    expect(response.album?.title).toBe('Kid A');
  });

  test('getAlbum caches by id, so a repeat call does not hit the service', async () => {
    let calls = 0;
    fakeLibrary({
      getAlbum: () => {
        calls++;
        return { album: { id: 'a1', title: 'Kid A' } };
      },
    });
    await getAlbum('a1');
    await getAlbum('a1');
    expect(calls).toBe(1);
  });

  test('getAlbum keys separately per id', async () => {
    let calls = 0;
    fakeLibrary({
      getAlbum: () => {
        calls++;
        return { album: { id: 'a1', title: 'Kid A' } };
      },
    });
    await getAlbum('a1');
    await getAlbum('a2');
    expect(calls).toBe(2);
  });

  test('listTracks passes the album id through', async () => {
    fakeLibrary({
      listTracks: (req) => ({
        items: [{ id: 't1', albumId: req.albumId, title: 'Everything' }],
        nextPageToken: '',
      }),
    });
    const response = await listTracks('a1');
    expect(response.items[0]?.albumId).toBe('a1');
  });
});
