import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';
import type { Album } from '@proto/library/v1/album_pb';

import AlbumGrid from './AlbumGrid';

const album = (over: Partial<Album>): Album =>
  ({
    id: 'a1',
    title: 'Kid A',
    releaseDate: '2000-10-02',
    media: 'CD',
    artists: [{ artistId: 'r1', creditedName: 'Radiohead' }],
    ...over,
  }) as Album;

describe('<AlbumGrid />', () => {
  test('it renders a tile per album with its title', () => {
    const { getByText } = render(() => (
      <AlbumGrid albums={[album({}), album({ id: 'a2', title: 'In Rainbows' })]} />
    ));
    expect(getByText('Kid A')).toBeInTheDocument();
    expect(getByText('In Rainbows')).toBeInTheDocument();
  });

  test('it credits the album artists', () => {
    const { getByText } = render(() => <AlbumGrid albums={[album({})]} />);
    expect(getByText('Radiohead')).toBeInTheDocument();
  });

  test('it joins multiple credited artists', () => {
    const { getByText } = render(() => (
      <AlbumGrid
        albums={[
          album({
            artists: [
              { artistId: 'r1', creditedName: 'Miles Davis' },
              { artistId: 'r2', creditedName: 'John Coltrane' },
            ] as Album['artists'],
          }),
        ]}
      />
    ));
    expect(getByText('Miles Davis, John Coltrane')).toBeInTheDocument();
  });

  test('the metadata line shows release year and media', () => {
    const { getByText } = render(() => <AlbumGrid albums={[album({})]} />);
    expect(getByText('2000 · CD')).toBeInTheDocument();
  });

  test('the metadata line omits media when the album has none', () => {
    const { getByText } = render(() => <AlbumGrid albums={[album({ media: '' })]} />);
    expect(getByText('2000')).toBeInTheDocument();
  });

  test('each tile links to the album detail route', () => {
    const { getByRole } = render(() => <AlbumGrid albums={[album({})]} />);
    expect(getByRole('link', { name: /Kid A/ })).toHaveAttribute('href', '/albums/a1');
  });
});
