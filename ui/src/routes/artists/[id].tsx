import { Title } from '@solidjs/meta';
import type { RouteDefinition, RouteProps } from '@solidjs/router';
import { createMemo, Show } from 'solid-js';

import InfiniteScrollSentinel from '../../components/InfiniteScrollSentinel';
import AlbumGrid from '../../features/library/AlbumGrid';
import { createInfiniteList } from '../../lib/pagination';
import { rpc } from '../../lib/rpc/client';
import { getArtist } from '../../lib/rpc/library';

export const route = {
  preload: ({ params }) => void getArtist(params.id!),
} satisfies RouteDefinition;

export default function ArtistDetail(props: RouteProps<'/artists/:id'>) {
  // Await: the query returns a Promise, so reading `.artist` off it directly
  // yields undefined.
  const artist = createMemo(async () => (await getArtist(props.params.id)).artist);

  const { items, loading, done, loadMore } = createInfiniteList(
    () => `albums:artist=${props.params.id}`,
    (pageToken) => rpc.library.listAlbums({ artistId: props.params.id, pageToken }),
  );

  return (
    <main class="px-6 py-8">
      <Show when={artist()}>
        {(a) => (
          <>
            <Title>{`${a().name} - byom`}</Title>
            <h1 class="mb-6 font-serif text-3xl">{a().name}</h1>
          </>
        )}
      </Show>
      <AlbumGrid albums={items} />
      <InfiniteScrollSentinel onIntersect={loadMore} disabled={loading() || done()} />
      <Show when={loading()}>
        <p class="text-muted py-4 font-mono text-xs">Loading…</p>
      </Show>
    </main>
  );
}
