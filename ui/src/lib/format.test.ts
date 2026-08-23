import { describe, expect, test } from 'vitest';
import { AlbumType, type Album } from '@proto/library/v1/album_pb';
import type { Duration, Timestamp } from '@bufbuild/protobuf/wkt';

import {
  albumFlags,
  albumTypeLabel,
  coverGradient,
  coverInitials,
  formatDuration,
  formatTrackDuration,
  relativeTime,
  sumDurations,
} from './format';

const dur = (seconds: number): Duration =>
  ({ seconds: BigInt(seconds), nanos: 0 }) as Duration;

describe('formatTrackDuration', () => {
  test('formatTrackDuration(251s) = got, want "4:11"', () => {
    expect(formatTrackDuration(dur(251))).toBe('4:11');
  });

  test('formatTrackDuration(9s) pads seconds', () => {
    expect(formatTrackDuration(dur(9))).toBe('0:09');
  });

  test('formatTrackDuration(undefined) = got, want "--:--"', () => {
    expect(formatTrackDuration(undefined)).toBe('--:--');
  });
});

describe('formatDuration', () => {
  test('formatDuration under an hour omits the hour field', () => {
    expect(formatDuration(dur(2842))).toBe('47:22');
  });

  test('formatDuration over an hour includes it and pads minutes', () => {
    expect(formatDuration(dur(3963))).toBe('1:06:03');
  });
});

describe('sumDurations', () => {
  test('sumDurations adds seconds and skips undefined entries', () => {
    expect(sumDurations([dur(60), undefined, dur(30)])).toEqual({
      seconds: 90n,
      nanos: 0,
    });
  });

  test('sumDurations carries nanos into whole seconds', () => {
    const half = { seconds: 0n, nanos: 600_000_000 } as Duration;
    expect(sumDurations([half, half])).toEqual({ seconds: 1n, nanos: 200_000_000 });
  });
});

describe('albumTypeLabel', () => {
  test('albumTypeLabel(MAIN) = got, want "ALBUM"', () => {
    expect(albumTypeLabel(AlbumType.MAIN)).toBe('ALBUM');
  });

  test('albumTypeLabel(UNSPECIFIED) is empty so no chip renders', () => {
    expect(albumTypeLabel(AlbumType.UNSPECIFIED)).toBe('');
  });
});

describe('albumFlags', () => {
  test('albumFlags returns only the flags that are set, in a stable order', () => {
    const album = { live: true, bootleg: false, compilation: true } as Album;
    expect(albumFlags(album)).toEqual(['LIVE', 'COMPILATION']);
  });

  test('albumFlags on a plain album is empty', () => {
    const album = { live: false, bootleg: false, compilation: false } as Album;
    expect(albumFlags(album)).toEqual([]);
  });
});

describe('coverInitials', () => {
  test('coverInitials takes the first letter of up to two words', () => {
    expect(coverInitials('Kid A')).toBe('KA');
  });

  test('coverInitials on one word takes its first two letters', () => {
    expect(coverInitials('Loveless')).toBe('LO');
  });

  test('coverInitials on an empty title = got, want "?"', () => {
    expect(coverInitials('')).toBe('?');
  });
});

describe('coverGradient', () => {
  test('coverGradient is deterministic for the same id', () => {
    expect(coverGradient('abc-123')).toEqual(coverGradient('abc-123'));
  });

  test('coverGradient separates different ids', () => {
    expect(coverGradient('abc-123')).not.toEqual(coverGradient('def-456'));
  });

  test('coverGradient returns two hsl stops', () => {
    const { from, to } = coverGradient('abc-123');
    expect(from).toMatch(/^hsl\(\d+ \d+% \d+%\)$/);
    expect(to).toMatch(/^hsl\(\d+ \d+% \d+%\)$/);
  });
});

describe('relativeTime', () => {
  const now = new Date('2026-08-23T12:00:00Z');
  const ts = (iso: string): Timestamp =>
    ({ seconds: BigInt(Math.floor(Date.parse(iso) / 1000)), nanos: 0 }) as Timestamp;

  test('relativeTime under a minute = got, want "just now"', () => {
    expect(relativeTime(ts('2026-08-23T11:59:30Z'), now)).toBe('just now');
  });

  test('relativeTime in hours', () => {
    expect(relativeTime(ts('2026-08-23T09:00:00Z'), now)).toBe('3h ago');
  });

  test('relativeTime in days', () => {
    expect(relativeTime(ts('2026-08-21T12:00:00Z'), now)).toBe('2d ago');
  });

  test('relativeTime(undefined) = got, want "never"', () => {
    expect(relativeTime(undefined, now)).toBe('never');
  });
});
