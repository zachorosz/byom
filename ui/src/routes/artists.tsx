import { Title } from '@solidjs/meta';
import { query, type RouteDefinition } from '@solidjs/router';
import { For, createMemo } from 'solid-js';

import { libraryClient } from '../lib/rpc/client';

const getArtists = query(async () => {
  const res = await libraryClient.listArtists({});
  return res.items;
}, 'artists');

export const route = {
  preload: () => void getArtists(),
} satisfies RouteDefinition;

export default function Artists() {
  const artists = createMemo(() => getArtists());

  return (
    <main class="px-4 py-12">
      <Title>Artists - Solid App</Title>
      <h1 class="my-4 text-4xl font-bold">Artists</h1>
      <ul class="my-4">
        <For each={artists()}>
          {(artist) => <li class="py-1">{artist.name}</li>}
        </For>
      </ul>
    </main>
  );
}
