import { Title } from '@solidjs/meta';
import { For } from 'solid-js';

import InfiniteScrollSentinel from '../components/InfiniteScrollSentinel';
import { libraryClient } from '../lib/rpc/client';
import { createInfiniteList } from '../lib/pagination';

export default function Albums() {
  const { items, loading, done, loadMore } = createInfiniteList((pageToken) =>
    libraryClient.listAlbums({ pageToken }),
  );

  return (
    <main class="px-4 py-12">
      <Title>Albums - Solid App</Title>
      <h1 class="my-4 text-4xl font-bold">Albums</h1>
      <ul class="my-4">
        <For each={items}>
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
      <InfiniteScrollSentinel onIntersect={loadMore} disabled={loading() || done()} />
      {loading() && <p class="py-4 text-slate-500">Loading…</p>}
    </main>
  );
}
