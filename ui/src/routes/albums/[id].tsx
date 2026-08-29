import { Title } from '@solidjs/meta';
import type { RouteDefinition, RouteProps } from '@solidjs/router';
import { createMemo, Loading, Show } from 'solid-js';

import ReleaseHeader from '../../features/library/ReleaseHeader';
import TrackList from '../../features/library/TrackList';
import { formatDuration, sumDurations } from '../../lib/format';
import { getAlbum, listTracks } from '../../lib/rpc/library';

// Starts both fetches as soon as navigation begins, before the page renders.
export const route = {
  preload: ({ params }) => {
    void getAlbum(params.id!);
    void listTracks(params.id!);
  },
} satisfies RouteDefinition;

export default function AlbumDetail(props: RouteProps<'/albums/:id'>) {
  // The memo must await: a query returns a Promise, so reading `.album` off it
  // directly yields undefined and the page renders empty forever.
  const album = createMemo(async () => (await getAlbum(props.params.id)).album);
  const tracks = createMemo(async () => (await listTracks(props.params.id)).items);

  return (
    <main class="px-6 py-8">
      <Show when={album()}>
        {(a) => (
          <>
            <Title>{`${a().title} - byom`}</Title>
            <ReleaseHeader album={a()} />
            {/* Its own boundary: the header depends only on GetAlbum, so a slow
                ListTracks must not hold the whole page on the shell fallback. */}
            <Loading
              fallback={<p class="text-muted py-4 font-mono text-xs">Loading tracks…</p>}
            >
              <p class="text-faint mb-3 font-mono text-[10px] tracking-[0.05em]">
                {tracks().length} TRACKS ·{' '}
                {formatDuration(sumDurations(tracks().map((t) => t.duration)))}
              </p>
              <TrackList tracks={tracks()} />
            </Loading>
          </>
        )}
      </Show>
    </main>
  );
}
