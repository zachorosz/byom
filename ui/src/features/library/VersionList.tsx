import { For, Show } from 'solid-js';
import type { Album } from '@proto/library/v1/album_pb';

import Chip from '../../components/Chip';

interface VersionListProps {
  versions: readonly Album[] | undefined;
  currentId: string;
}

// primaryVersion records which version the library lists a release
// under, so it is a label about the catalog rather than the pressing.
function label(album: Album): string {
  return album.version || album.title;
}

function meta(album: Album): string {
  const year = album.releaseDate.slice(0, 4);
  return [year, album.media].filter(Boolean).join(' · ');
}

/** VersionList renders the releases sharing an album's version group. */
export default function VersionList(props: VersionListProps) {
  // A group of one is just the album already on screen.
  const versions = () => ((props.versions?.length ?? 0) > 1 ? props.versions : undefined);

  return (
    <Show when={versions()}>
      {(items) => (
        <section class="mb-8">
          <h2 class="text-faint mb-3 font-mono text-[10px] tracking-[0.05em]">
            {items().length} VERSIONS
          </h2>
          <ul class="border-line divide-line divide-y border-y">
            <For each={items()}>
              {(album) => (
                <li
                  aria-current={album.id === props.currentId ? 'true' : undefined}
                  class="flex items-baseline gap-3 py-2"
                >
                  <Show
                    when={album.id !== props.currentId}
                    fallback={<VersionSummary album={album} />}
                  >
                    <a href={`/albums/${album.id}`} class="flex flex-1 gap-3 no-underline">
                      <VersionSummary album={album} />
                    </a>
                  </Show>
                  <Show when={album.primaryVersion}>
                    <Chip>PRIMARY</Chip>
                  </Show>
                </li>
              )}
            </For>
          </ul>
        </section>
      )}
    </Show>
  );
}

function VersionSummary(props: { album: Album }) {
  return (
    <>
      <span class="flex-1 font-serif text-sm">{label(props.album)}</span>
      <span class="text-faint font-mono text-[9px] tracking-[0.05em]">{meta(props.album)}</span>
    </>
  );
}
