import { For } from 'solid-js';
import type { Artist } from '@proto/library/v1/artist_pb';

interface ArtistListProps {
  artists: readonly Artist[];
}

/**
 * ArtistList renders artists as an alphabetical list.
 */
export default function ArtistList(props: ArtistListProps) {
  return (
    <ul class="columns-1 gap-8 sm:columns-2 lg:columns-3">
      <For each={props.artists}>
        {(artist) => (
          <li class="break-inside-avoid">
            <a
              href={`/artists/${artist.id}`}
              class="hover:text-accent block py-1 font-serif text-base no-underline transition-colors"
            >
              {artist.name}
            </a>
          </li>
        )}
      </For>
    </ul>
  );
}
