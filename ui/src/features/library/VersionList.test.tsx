import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';
import type { Album } from '@proto/library/v1/album_pb';

import VersionList from './VersionList';

const album = (over: Partial<Album>): Album =>
  ({
    id: 'a1',
    title: 'Polygondwanaland',
    releaseDate: '2017-11-17',
    media: 'Digital',
    version: '',
    primaryVersion: false,
    artists: [
      { artistId: 'r1', creditedName: 'King Gizzard & the Lizard Wizard' },
    ],
    ...over,
  }) as Album;

const primary = album({ id: 'a1', primaryVersion: true });
const reissue = album({
  id: 'a2',
  title: 'Polygondwanaland (ATO Edition)',
  releaseDate: '2018-04-21',
  media: 'Vinyl',
  version: 'ATO Edition',
});

describe('<VersionList />', () => {
  test('it renders every version in the group', () => {
    const { getByText } = render(() => (
      <VersionList versions={[primary, reissue]} currentId="a1" />
    ));
    expect(getByText('ATO Edition')).toBeInTheDocument();
    expect(getByText('Polygondwanaland')).toBeInTheDocument();
  });

  test('it falls back to the album title when a version has no version name', () => {
    const { getByText } = render(() => (
      <VersionList versions={[primary, reissue]} currentId="a2" />
    ));
    expect(getByText('Polygondwanaland')).toBeInTheDocument();
  });

  test('it renders nothing when the release has no alternate versions', () => {
    const { container } = render(() => (
      <VersionList versions={[primary]} currentId="a1" />
    ));
    expect(container).toBeEmptyDOMElement();
  });

  test('it renders nothing before the versions have loaded', () => {
    const { container } = render(() => (
      <VersionList versions={undefined} currentId="a1" />
    ));
    expect(container).toBeEmptyDOMElement();
  });

  test('it marks the version being viewed', () => {
    const { getByText } = render(() => (
      <VersionList versions={[primary, reissue]} currentId="a2" />
    ));
    expect(getByText('ATO Edition').closest('li')).toHaveAttribute(
      'aria-current',
      'true'
    );
    expect(getByText('Polygondwanaland').closest('li')).not.toHaveAttribute(
      'aria-current'
    );
  });

  test('it links other versions to their detail page', () => {
    const { getByRole } = render(() => (
      <VersionList versions={[primary, reissue]} currentId="a1" />
    ));
    expect(getByRole('link', { name: /ATO Edition/ })).toHaveAttribute(
      'href',
      '/albums/a2'
    );
  });

  test('the version being viewed is not a link', () => {
    const { queryByRole } = render(() => (
      <VersionList versions={[primary, reissue]} currentId="a1" />
    ));
    expect(
      queryByRole('link', { name: /Polygondwanaland/ })
    ).not.toBeInTheDocument();
  });

  test('it shows release year and media per version', () => {
    const { getByText } = render(() => (
      <VersionList versions={[primary, reissue]} currentId="a1" />
    ));
    expect(getByText('2018 · Vinyl')).toBeInTheDocument();
  });

  test('it marks which version the library treats as primary', () => {
    const { getByText } = render(() => (
      <VersionList versions={[primary, reissue]} currentId="a2" />
    ));
    expect(getByText('Polygondwanaland').closest('li')).toHaveTextContent(
      'PRIMARY'
    );
  });
});
