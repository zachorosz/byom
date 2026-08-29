import { fireEvent, render, waitFor } from '@solidjs/testing-library';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import type { Album } from '@proto/library/v1/album_pb';

import AlbumSection from './AlbumSection';
import { clearListCache } from '../../lib/pagination';

const album = (id: string, title: string): Album =>
  ({
    id,
    title,
    releaseDate: '2017-11-17',
    media: 'Digital',
    artists: [
      { artistId: 'r1', creditedName: 'King Gizzard & the Lizard Wizard' },
    ],
  }) as Album;

// Each test gets its own key: the page cache is module-level and would
// otherwise serve one test's albums to the next. Solid compiles JSX prop
// expressions to getters, so the key is read once here rather than inline.
let keySeq = 0;
const nextKey = () => `test-section-${(keySeq += 1)}`;

const onePage = (...albums: Album[]) =>
  vi.fn().mockResolvedValue({ items: albums, nextPageToken: '' });

beforeEach(() => {
  clearListCache();
});

describe('<AlbumSection />', () => {
  test('it renders its heading and albums', async () => {
    const listKey = nextKey();
    const { findByText, getByText } = render(() => (
      <AlbumSection
        heading="Albums"
        listKey={listKey}
        fetchPage={onePage(album('a1', 'Nonagon Infinity'))}
      />
    ));

    expect(await findByText('Nonagon Infinity')).toBeInTheDocument();
    expect(getByText('Albums')).toBeInTheDocument();
  });

  test('it renders nothing when the section has no albums', async () => {
    const listKey = nextKey();
    const { container } = render(() => (
      <AlbumSection
        heading="Bootlegs"
        listKey={listKey}
        fetchPage={onePage()}
      />
    ));

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  test('it offers Load more only while pages remain', async () => {
    const fetchPage = vi.fn().mockResolvedValue({
      items: [album('a1', 'Nonagon Infinity')],
      nextPageToken: 'page-2',
    });
    const listKey = nextKey();
    const { findByRole } = render(() => (
      <AlbumSection heading="Albums" listKey={listKey} fetchPage={fetchPage} />
    ));

    expect(
      await findByRole('button', { name: /load more/i })
    ).toBeInTheDocument();
  });

  test('it hides Load more once the last page arrives', async () => {
    const listKey = nextKey();
    const { findByText, queryByRole } = render(() => (
      <AlbumSection
        heading="Albums"
        listKey={listKey}
        fetchPage={onePage(album('a1', 'Nonagon Infinity'))}
      />
    ));
    await findByText('Nonagon Infinity');

    expect(
      queryByRole('button', { name: /load more/i })
    ).not.toBeInTheDocument();
  });

  test('Load more appends the next page', async () => {
    const fetchPage = vi
      .fn()
      .mockResolvedValueOnce({
        items: [album('a1', 'Nonagon Infinity')],
        nextPageToken: 'page-2',
      })
      .mockResolvedValueOnce({
        items: [album('a2', 'Polygondwanaland')],
        nextPageToken: '',
      });

    const listKey = nextKey();
    const { findByRole, findByText, getByText } = render(() => (
      <AlbumSection heading="Albums" listKey={listKey} fetchPage={fetchPage} />
    ));
    fireEvent.click(await findByRole('button', { name: /load more/i }));

    expect(await findByText('Polygondwanaland')).toBeInTheDocument();
    expect(getByText('Nonagon Infinity')).toBeInTheDocument();
    expect(fetchPage).toHaveBeenLastCalledWith('page-2');
  });
});
