import { Title } from "@solidjs/meta";
import type { RouteDefinition, RouteProps } from "@solidjs/router";
import { createMemo, For, Show } from "solid-js";
import { AlbumType } from "@proto/library/v1/album_pb";
import { AlbumOrder, BootlegFilter } from "@proto/library/v1/library_pb";

import AlbumSection from "../../features/library/AlbumSection";
import { rpc } from "../../lib/rpc/client";
import { getArtist } from "../../lib/rpc/library";

export const route = {
  preload: ({ params }) => void getArtist(params.id!),
} satisfies RouteDefinition;

interface Section {
  heading: string;
  albumTypes: AlbumType[];
  bootlegs: BootlegFilter;
}

// Bootlegs are pulled out of the type sections and given their own, so no
// release is listed twice and none is hidden. Untagged releases ride with
// Other; without that they would belong to no section at all.
const SECTIONS: Section[] = [
  {
    heading: "Albums",
    albumTypes: [AlbumType.MAIN, AlbumType.UNSPECIFIED],
    bootlegs: BootlegFilter.EXCLUDE,
  },
  {
    heading: "Singles & EPs",
    albumTypes: [AlbumType.SINGLE, AlbumType.EP],
    bootlegs: BootlegFilter.EXCLUDE,
  },
  {
    heading: "Other",
    albumTypes: [AlbumType.OTHER],
    bootlegs: BootlegFilter.EXCLUDE,
  },
  {
    heading: "Bootlegs",
    albumTypes: [],
    bootlegs: BootlegFilter.ONLY,
  },
];

export default function ArtistDetail(props: RouteProps<"/artists/:id">) {
  // Await: the query returns a Promise, so reading `.artist` off it directly
  // yields undefined.
  const artist = createMemo(
    async () => (await getArtist(props.params.id)).artist,
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
      <For each={SECTIONS}>
        {(section) => (
          <AlbumSection
            heading={section.heading}
            listKey={`albums:artist=${props.params.id}:${section.heading}`}
            fetchPage={(pageToken) =>
              rpc.library.listAlbums({
                artistId: props.params.id,
                albumTypes: section.albumTypes,
                bootlegs: section.bootlegs,
                // Oldest first: a discography reads chronologically, and
                // ascending keeps undated releases off the top.
                order: AlbumOrder.ORIGINAL_DATE,
                pageToken,
              })
            }
          />
        )}
      </For>
    </main>
  );
}
