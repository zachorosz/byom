import { For } from 'solid-js';
import type { Artist } from '@proto/library/v1/artist_pb';

interface ArtistListProps {
  // readonly: createInfiniteList's items come from a store, which is readonly.
  artists: readonly Artist[];
}

/**
 * ArtistList renders artists as an alphabetical list.
 *
 * Artist carries only an id and a name — no image, no album count — so a list
 * is honest where a grid of empty tiles would not be. Order comes from the RPC.
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
