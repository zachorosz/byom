import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';
import { AlbumType, type Album } from '@proto/library/v1/album_pb';

import ReleaseHeader from './ReleaseHeader';

const album = (over: Partial<Album>): Album =>
  ({
    id: 'a1',
    title: 'The Köln Concert',
    albumType: AlbumType.MAIN,
    releaseDate: '1975-11',
    releaseCountry: 'DE',
    media: '2LP',
    version: '',
    primaryVersion: false,
    live: false,
    bootleg: false,
    compilation: false,
    artists: [{ artistId: 'k1', creditedName: 'Keith Jarrett' }],
    ...over,
  }) as Album;

describe('<ReleaseHeader />', () => {
  test('it renders the title and credited artists', () => {
    const { getByText } = render(() => <ReleaseHeader album={album({})} />);
    expect(getByText('The Köln Concert')).toBeInTheDocument();
    expect(getByText('Keith Jarrett')).toBeInTheDocument();
  });

  test('the release line carries date, country and media', () => {
    const { getByText } = render(() => <ReleaseHeader album={album({})} />);
    expect(getByText('RELEASED 1975-11 · DE · 2LP')).toBeInTheDocument();
  });

  test('it chips the album type', () => {
    const { getByText } = render(() => <ReleaseHeader album={album({})} />);
    expect(getByText('ALBUM')).toBeInTheDocument();
  });

  test('it chips only the flags that are set', () => {
    const { getByText, queryByText } = render(() => (
      <ReleaseHeader album={album({ live: true })} />
    ));
    expect(getByText('LIVE')).toBeInTheDocument();
    expect(queryByText('BOOTLEG')).toBeNull();
  });

  test('it chips the version string when there is one', () => {
    const { getByText } = render(() => (
      <ReleaseHeader album={album({ version: '2014 Remaster' })} />
    ));
    expect(getByText('2014 Remaster')).toBeInTheDocument();
  });

  test('it marks the primary version', () => {
    const { getByText } = render(() => (
      <ReleaseHeader album={album({ primaryVersion: true })} />
    ));
    expect(getByText('PRIMARY VERSION')).toBeInTheDocument();
  });
});
