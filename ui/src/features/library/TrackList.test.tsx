import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';
import type { Track } from '@proto/library/v1/track_pb';
import type { Duration } from '@bufbuild/protobuf/wkt';

import TrackList from './TrackList';

const dur = (s: number) => ({ seconds: BigInt(s), nanos: 0 }) as Duration;

const track = (over: Partial<Track>): Track =>
  ({
    id: 't1',
    albumId: 'a1',
    title: 'Part I',
    discNumber: 1,
    discSubtitle: '',
    trackNumber: 1,
    duration: dur(251),
    credits: [],
    ...over,
  }) as Track;

describe('<TrackList />', () => {
  test('it renders each track with number and duration', () => {
    const { getByText } = render(() => <TrackList tracks={[track({})]} />);
    expect(getByText('Part I')).toBeInTheDocument();
    expect(getByText('1')).toBeInTheDocument();
    expect(getByText('4:11')).toBeInTheDocument();
  });

  test('a single-disc album renders no disc heading', () => {
    const { queryByText } = render(() => (
      <TrackList tracks={[track({}), track({ id: 't2', trackNumber: 2 })]} />
    ));
    expect(queryByText('Disc 1')).toBeNull();
  });

  test('a multi-disc album groups tracks under disc headings', () => {
    const { getByText } = render(() => (
      <TrackList tracks={[track({}), track({ id: 't2', discNumber: 2 })]} />
    ));
    expect(getByText('Disc 1')).toBeInTheDocument();
    expect(getByText('Disc 2')).toBeInTheDocument();
  });

  test('a disc subtitle replaces the generic disc heading', () => {
    const { getByText } = render(() => (
      <TrackList
        tracks={[
          track({ discSubtitle: 'The Concert' }),
          track({ id: 't2', discNumber: 2 }),
        ]}
      />
    ));
    expect(getByText('The Concert')).toBeInTheDocument();
  });

  test('a track with no duration renders the placeholder', () => {
    const { getByText } = render(() => (
      <TrackList tracks={[track({ duration: undefined })]} />
    ));
    expect(getByText('--:--')).toBeInTheDocument();
  });
});
