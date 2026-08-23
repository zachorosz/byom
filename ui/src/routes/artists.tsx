import { Title } from '@solidjs/meta';
import { useSearchParams } from '@solidjs/router';
import { Show } from 'solid-js';

import InfiniteScrollSentinel from '../components/InfiniteScrollSentinel';
import ArtistList from '../features/library/ArtistList';
import { createInfiniteList, createScrollRestoration } from '../lib/pagination';
import { rpc } from '../lib/rpc/client';

export default function Artists() {
  const [params] = useSearchParams();
  const key = () => `artists:${JSON.stringify(params)}`;

  const list = createInfiniteList(key, (pageToken) =>
    rpc.library.listArtists({ pageToken }),
  );
  const { items, loading, done, error, loadMore } = list;
  createScrollRestoration(list);

  return (
    <main class="px-6 py-8">
      <Title>Artists - byom</Title>
      <h1 class="mb-6 font-serif text-3xl">Artists</h1>
      <ArtistList artists={items} />
      <Show when={items.length === 0 && done()}>
        <p class="text-muted text-sm">
          No artists yet.{' '}
          <a href="/settings" class="text-accent underline underline-offset-4">
            Add a library source
          </a>{' '}
          and run a scan.
        </p>
      </Show>
      <InfiniteScrollSentinel onIntersect={loadMore} disabled={loading() || done()} />
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
