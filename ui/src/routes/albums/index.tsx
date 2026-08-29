import { Title } from '@solidjs/meta';
import { useSearchParams } from '@solidjs/router';
import { Show } from 'solid-js';
import { AlbumOrder } from '@proto/library/v1/library_pb';

import InfiniteScrollSentinel from '../../components/InfiniteScrollSentinel';
import AlbumGrid from '../../features/library/AlbumGrid';
import {
  createInfiniteList,
  createScrollRestoration,
} from '../../lib/pagination';
import { rpc } from '../../lib/rpc/client';

export default function Albums() {
  const [params] = useSearchParams();

  // The key is the filter set. ListAlbums returns one entry per release by
  // default, so alternate versions stay on the album page rather than the grid.
  const key = () => `albums:${JSON.stringify(params)}`;

  // Artist order groups a discography together, each artist's albums in
  // original release order. The order is bound to the page token, so it has to
  // be sent with every page.
  const list = createInfiniteList(key, (pageToken) =>
    rpc.library.listAlbums({ pageToken, order: AlbumOrder.ARTIST })
  );
  const { items, loading, done, error, loadMore } = list;
  createScrollRestoration(list);

  return (
    <main class="px-6 py-8">
      <Title>Albums - byom</Title>
      <h1 class="mb-6 font-serif text-3xl">Albums</h1>
      <AlbumGrid albums={items} />
      <Show when={items.length === 0 && done()}>
        <p class="text-muted text-sm">
          No albums yet.{' '}
          <a href="/settings" class="text-accent underline underline-offset-4">
            Add a library source
          </a>{' '}
          and run a scan.
        </p>
      </Show>
      <InfiniteScrollSentinel
        onIntersect={loadMore}
        disabled={loading() || done()}
      />
      <Show when={loading()}>
        <p class="text-muted py-4 font-mono text-xs">Loading…</p>
      </Show>
      <Show when={error()}>
        <button
          type="button"
          onClick={loadMore}
          class="border-line text-muted mt-4 rounded border px-3 py-1 text-xs"
        >
          Failed to load more — retry
        </button>
      </Show>
    </main>
  );
}
