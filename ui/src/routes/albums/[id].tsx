import { Title } from '@solidjs/meta';
import type { RouteDefinition, RouteProps } from '@solidjs/router';
import { createMemo, Show } from 'solid-js';

import ReleaseHeader from '../../features/library/ReleaseHeader';
import TrackList from '../../features/library/TrackList';
import { sumDurations } from '../../lib/format';
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
            <ReleaseHeader
              album={a()}
              trackCount={tracks().length}
              totalDuration={sumDurations(tracks().map((t) => t.duration))}
            />
            <TrackList tracks={tracks()} />
          </>
        )}
      </Show>
    </main>
  );
}
