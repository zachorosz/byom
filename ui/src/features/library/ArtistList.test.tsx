import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';
import type { Artist } from '@proto/library/v1/artist_pb';

import ArtistList from './ArtistList';

const artists = [
  { id: 'r1', name: 'Radiohead' },
  { id: 'k1', name: 'Keith Jarrett' },
] as Artist[];

describe('<ArtistList />', () => {
  test('it renders every artist name', () => {
    const { getByText } = render(() => <ArtistList artists={artists} />);
    expect(getByText('Radiohead')).toBeInTheDocument();
    expect(getByText('Keith Jarrett')).toBeInTheDocument();
  });

  test('each name links to the artist detail route', () => {
    const { getByRole } = render(() => <ArtistList artists={artists} />);
    expect(getByRole('link', { name: 'Radiohead' })).toHaveAttribute(
      'href',
      '/artists/r1'
    );
  });

  test('it preserves the order the RPC returned', () => {
    const { getAllByRole } = render(() => <ArtistList artists={artists} />);
    expect(getAllByRole('link').map((a) => a.textContent)).toEqual([
      'Radiohead',
      'Keith Jarrett',
    ]);
  });
});
