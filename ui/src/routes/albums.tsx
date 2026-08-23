import { Title } from '@solidjs/meta';
import { query, type RouteDefinition } from '@solidjs/router';
import { For, createMemo } from 'solid-js';

import { libraryClient } from '../lib/rpc/client';

const getAlbums = query(async () => {
  const res = await libraryClient.listAlbums({});
  return res.items;
}, 'albums');

export const route = {
  preload: () => void getAlbums(),
} satisfies RouteDefinition;

export default function Albums() {
  const albums = createMemo(() => getAlbums());

  return (
    <main class="px-4 py-12">
      <Title>Albums - Solid App</Title>
      <h1 class="my-4 text-4xl font-bold">Albums</h1>
      <ul class="my-4">
        <For each={albums()}>
          {(album) => (
            <li class="py-1">
              {album.title}
              {album.artists.length > 0 && (
                <span class="text-slate-500">
                  {' — '}
                  {album.artists.map((a) => a.creditedName).join(', ')}
                </span>
              )}
            </li>
          )}
        </For>
      </ul>
    </main>
  );
}
