import { For, Show } from 'solid-js';
import type { Album } from '@proto/library/v1/album_pb';

import Cover from '../../components/Cover';

interface AlbumGridProps {
  albums: Album[];
}

function credits(album: Album): string {
  return album.artists.map((a) => a.creditedName).join(', ');
}

function meta(album: Album): string {
  const year = album.releaseDate.slice(0, 4);
  return [year, album.media].filter(Boolean).join(' · ');
}

/** AlbumGrid renders albums as a responsive grid of cover tiles. */
export default function AlbumGrid(props: AlbumGridProps) {
  return (
    <ul class="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
      <For each={props.albums}>
        {(album) => (
          <li>
            <a href={`/albums/${album.id}`} class="block no-underline">
              <Cover id={album.id} title={album.title} />
              <div class="mt-2.5 font-serif text-sm leading-tight">{album.title}</div>
              <Show when={album.artists.length > 0}>
                <div class="text-muted mt-0.5 text-xs">{credits(album)}</div>
              </Show>
              <div class="text-faint mt-1 font-mono text-[9px] tracking-[0.05em]">
                {meta(album)}
              </div>
            </a>
          </li>
        )}
      </For>
    </ul>
  );
}
