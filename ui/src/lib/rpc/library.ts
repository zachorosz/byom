import { query } from '@solidjs/router';

import { rpc } from './client';

/** getAlbum reads one album, cached per id. */
export const getAlbum = query((id: string) => rpc.library.getAlbum({ id }), 'album');

/** getArtist reads one artist, cached per id. */
export const getArtist = query((id: string) => rpc.library.getArtist({ id }), 'artist');

/** listTracks reads an album's tracks in one page, cached per album id. */
export const listTracks = query(
  (albumId: string) => rpc.library.listTracks({ albumId, pageSize: 500 }),
  'tracks',
);
