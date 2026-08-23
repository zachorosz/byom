import { Title } from '@solidjs/meta';
import { For } from 'solid-js';

import InfiniteScrollSentinel from '../components/InfiniteScrollSentinel';
import { libraryClient } from '../lib/rpc/client';
import { createInfiniteList } from '../lib/pagination';

export default function Artists() {
  const { items, loading, done, loadMore } = createInfiniteList((pageToken) =>
    libraryClient.listArtists({ pageToken }),
  );

  return (
    <main class="px-4 py-12">
      <Title>Artists - Solid App</Title>
      <h1 class="my-4 text-4xl font-bold">Artists</h1>
      <ul class="my-4">
        <For each={items}>{(artist) => <li class="py-1">{artist.name}</li>}</For>
      </ul>
      <InfiniteScrollSentinel onIntersect={loadMore} disabled={loading() || done()} />
      {loading() && <p class="py-4 text-slate-500">Loading…</p>}
    </main>
  );
}
