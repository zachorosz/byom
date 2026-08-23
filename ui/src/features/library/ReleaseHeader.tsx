import { For, Show } from 'solid-js';
import type { Album } from '@proto/library/v1/album_pb';
import type { Duration } from '@bufbuild/protobuf/wkt';

import Chip from '../../components/Chip';
import Cover from '../../components/Cover';
import { albumFlags, albumTypeLabel, formatDuration } from '../../lib/format';

interface ReleaseHeaderProps {
  album: Album;
  trackCount: number;
  totalDuration: Duration;
}

/** ReleaseHeader renders an album's identity: artwork, credits, and release facts. */
export default function ReleaseHeader(props: ReleaseHeaderProps) {
  const releaseLine = () =>
    [
      `RELEASED ${props.album.releaseDate || 'UNKNOWN'}`,
      props.album.releaseCountry,
      props.album.media,
    ]
      .filter(Boolean)
      .join(' · ');

  const credits = () => props.album.artists.map((a) => a.creditedName).join(', ');

  return (
    <header class="mb-8 flex flex-col gap-6 sm:flex-row">
      <div class="w-40 flex-none">
        <Cover id={props.album.id} title={props.album.title} size="hero" />
      </div>
      <div>
        <h1 class="font-serif text-3xl leading-tight">{props.album.title}</h1>
        <Show when={props.album.artists.length > 0}>
          <p class="text-muted mt-1 font-serif text-base italic">{credits()}</p>
        </Show>
        <p class="text-faint mt-3 font-mono text-[10px] tracking-[0.05em]">
          {releaseLine()}
        </p>
        <div class="mt-3 flex flex-wrap gap-1.5">
          <Show when={albumTypeLabel(props.album.albumType)}>
            {(label) => <Chip>{label()}</Chip>}
          </Show>
          <For each={albumFlags(props.album)}>{(flag) => <Chip accent>{flag}</Chip>}</For>
          <Show when={props.album.version}>{(v) => <Chip>{v()}</Chip>}</Show>
          <Show when={props.album.primaryVersion}>
            <Chip>PRIMARY VERSION</Chip>
          </Show>
        </div>
        <p class="text-faint mt-3 font-mono text-[10px] tracking-[0.05em]">
          {props.trackCount} TRACKS · {formatDuration(props.totalDuration)}
        </p>
      </div>
    </header>
  );
}
