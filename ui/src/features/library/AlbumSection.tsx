import { Show } from 'solid-js';
import type { Album } from '@proto/library/v1/album_pb';

import AlbumGrid from './AlbumGrid';
import { createInfiniteList } from '../../lib/pagination';

interface AlbumPage {
  items: Album[];
  nextPageToken: string;
}

interface AlbumSectionProps {
  heading: string;
  /** listKey identifies the section's filter set for the page cache. */
  listKey: string;
  fetchPage: (pageToken: string) => Promise<AlbumPage>;
}

/**
 * AlbumSection renders one filtered slice of an artist's releases.
 *
 * Paging is by an explicit Load more rather than a scroll sentinel: several
 * sections share a page, and their sentinels would all sit on screen at once
 * and fetch together.
 */
export default function AlbumSection(props: AlbumSectionProps) {
  const { items, loading, done, error, loadMore } = createInfiniteList<Album>(
    () => props.listKey,
    (pageToken) => props.fetchPage(pageToken),
  );

  // An empty section is absent, not empty: an artist with no bootlegs should
  // see no Bootlegs heading at all.
  return (
    <Show when={items.length > 0 || loading()}>
      <section class="mb-10">
        <h2 class="text-muted mb-3 font-mono text-[10px] tracking-[0.05em] uppercase">
          {props.heading}
        </h2>
        <AlbumGrid albums={items} />
        <Show when={loading()}>
          <p class="text-muted py-4 font-mono text-xs">Loading…</p>
        </Show>
        <Show when={!done() && !loading()}>
          <button
            type="button"
            onClick={loadMore}
            class="border-line text-muted mt-4 rounded border px-3 py-1 font-mono text-[10px] tracking-[0.05em]"
          >
            {error() ? 'FAILED — RETRY' : 'LOAD MORE'}
          </button>
        </Show>
      </section>
    </Show>
  );
}
