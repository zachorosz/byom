import type { Duration, Timestamp } from '@bufbuild/protobuf/wkt';
import { AlbumType, type Album } from '@proto/library/v1/album_pb';

const NANOS_PER_SECOND = 1_000_000_000;

function totalSeconds(d?: Duration): number {
  if (!d) return 0;
  return Number(d.seconds) + d.nanos / NANOS_PER_SECOND;
}

/** formatTrackDuration renders a track length as m:ss, or "--:--" when absent. */
export function formatTrackDuration(d?: Duration): string {
  if (!d) return '--:--';
  const total = Math.round(totalSeconds(d));
  const minutes = Math.floor(total / 60);
  return `${minutes}:${String(total % 60).padStart(2, '0')}`;
}

/** formatDuration renders a total length, including an hours field past 3600s. */
export function formatDuration(d?: Duration): string {
  if (!d) return '--:--';
  const total = Math.round(totalSeconds(d));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = String(total % 60).padStart(2, '0');
  if (hours === 0) return `${minutes}:${seconds}`;
  return `${hours}:${String(minutes).padStart(2, '0')}:${seconds}`;
}

/** sumDurations totals a list of durations, ignoring absent entries. */
export function sumDurations(ds: (Duration | undefined)[]): Duration {
  let seconds = 0n;
  let nanos = 0;
  for (const d of ds) {
    if (!d) continue;
    seconds += d.seconds;
    nanos += d.nanos;
  }
  seconds += BigInt(Math.floor(nanos / NANOS_PER_SECOND));
  return { seconds, nanos: nanos % NANOS_PER_SECOND } as Duration;
}

const ALBUM_TYPE_LABELS: Record<AlbumType, string> = {
  [AlbumType.UNSPECIFIED]: '',
  [AlbumType.MAIN]: 'ALBUM',
  [AlbumType.SINGLE]: 'SINGLE',
  [AlbumType.EP]: 'EP',
  [AlbumType.OTHER]: 'OTHER',
};

/** albumTypeLabel returns the chip label for a type, or "" when unspecified. */
export function albumTypeLabel(t: AlbumType): string {
  return ALBUM_TYPE_LABELS[t] ?? '';
}

/** albumFlags returns the chip labels for the release flags that are set. */
export function albumFlags(a: Album): string[] {
  const flags: string[] = [];
  if (a.live) flags.push('LIVE');
  if (a.bootleg) flags.push('BOOTLEG');
  if (a.compilation) flags.push('COMPILATION');
  return flags;
}

/** coverInitials returns up to two letters standing in for missing cover art. */
export function coverInitials(title: string): string {
  const words = title.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return '?';
  if (words.length === 1) return words[0]!.slice(0, 2).toUpperCase();
  return (words[0]![0]! + words[1]![0]!).toUpperCase();
}

/**
 * coverGradient returns the two gradient stops for an album's placeholder.
 *
 * Stable across reloads for a given id, so a placeholder tile reads as the
 * same object every time rather than reshuffling on each render.
 */
export function coverGradient(id: string): { from: string; to: string } {
  // FNV-1a: cheap, well-distributed, and stable across engines.
  let hash = 0x811c9dc5;
  for (let i = 0; i < id.length; i++) {
    hash ^= id.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  const hue = hash % 360;
  return {
    from: `hsl(${hue} 18% 32%)`,
    to: `hsl(${(hue + 18) % 360} 22% 12%)`,
  };
}

/** relativeTime renders how long ago a timestamp was, or "never" when absent. */
export function relativeTime(ts?: Timestamp, now: Date = new Date()): string {
  if (!ts) return 'never';
  const elapsed = (now.getTime() - Number(ts.seconds) * 1000) / 1000;
  if (elapsed < 60) return 'just now';
  if (elapsed < 3600) return `${Math.floor(elapsed / 60)}m ago`;
  if (elapsed < 86_400) return `${Math.floor(elapsed / 3600)}h ago`;
  return `${Math.floor(elapsed / 86_400)}d ago`;
}
