import { For, Show, createMemo } from 'solid-js';
import type { Track } from '@proto/library/v1/track_pb';

import { formatTrackDuration } from '../../lib/format';

interface TrackListProps {
  tracks: Track[];
}

interface Disc {
  number: number;
  heading: string;
  tracks: Track[];
}

/** TrackList renders an album's tracks, grouped by disc when there is more than one. */
export default function TrackList(props: TrackListProps) {
  const discs = createMemo<Disc[]>(() => {
    const byNumber = new Map<number, Disc>();
    for (const track of props.tracks) {
      const number = track.discNumber || 1;
      let disc = byNumber.get(number);
      if (!disc) {
        disc = { number, heading: '', tracks: [] };
        byNumber.set(number, disc);
      }
      if (track.discSubtitle) disc.heading = track.discSubtitle;
      disc.tracks.push(track);
    }
    return [...byNumber.values()].sort((a, b) => a.number - b.number);
  });

  return (
    <For each={discs()}>
      {(disc) => (
        <section>
          <Show when={discs().length > 1}>
            <h2 class="text-faint mt-4 mb-1.5 ml-2 font-mono text-[8px] tracking-[0.16em] uppercase">
              {disc.heading || `Disc ${disc.number}`}
            </h2>
          </Show>
          <ul>
            <For each={disc.tracks}>
              {(track) => (
                <li class="hover:bg-panel border-b-line flex items-baseline gap-3 rounded-[3px] border-b px-2 py-1.5">
                  <span class="text-faint w-5 flex-none text-right font-mono text-[10px]">
                    {track.trackNumber}
                  </span>
                  <span class="flex-1 text-sm">{track.title}</span>
                  <span class="text-muted font-mono text-[10px]">
                    {formatTrackDuration(track.duration)}
                  </span>
                </li>
              )}
            </For>
          </ul>
        </section>
      )}
    </For>
  );
}
