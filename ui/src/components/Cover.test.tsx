import { render } from '@solidjs/testing-library';
import { flush } from 'solid-js';
import { describe, expect, test } from 'vitest';

import Cover from './Cover';

describe('<Cover />', () => {
  test('it labels the placeholder for screen readers', () => {
    const { getByLabelText } = render(() => <Cover title="Kid A" />);
    expect(getByLabelText('Kid A — no cover art')).toBeInTheDocument();
  });

  test('it renders no image element without a cover hash', () => {
    const { queryByRole } = render(() => <Cover title="Kid A" />);
    expect(queryByRole('presentation')).toBeNull();
  });

  test('it rests without a pulse when there is no cover to wait for', () => {
    const { getByTestId } = render(() => <Cover title="Kid A" />);
    expect(getByTestId('cover-skeleton')).not.toHaveClass('animate-pulse');
  });

  test('it renders the artwork when a cover hash is present', () => {
    const { getByRole } = render(() => <Cover title="Kid A" coverHash="abc123" />);
    const img = getByRole('presentation') as HTMLImageElement;
    expect(img.getAttribute('src')).toBe('/images/abc123?size=160');
    expect(img.getAttribute('srcset')).toBe(
      '/images/abc123?size=160 1x, /images/abc123?size=320 2x',
    );
  });

  test('it requests the sharper pair for the hero', () => {
    const { getByRole } = render(() => (
      <Cover title="Kid A" coverHash="abc123" size="hero" />
    ));
    const img = getByRole('presentation') as HTMLImageElement;
    expect(img.getAttribute('src')).toBe('/images/abc123?size=320');
    expect(img.getAttribute('srcset')).toBe(
      '/images/abc123?size=320 1x, /images/abc123?size=640 2x',
    );
  });

  test('it labels the tile with the title when artwork is present', () => {
    const { getByLabelText } = render(() => <Cover title="Kid A" coverHash="abc123" />);
    expect(getByLabelText('Kid A')).toBeInTheDocument();
  });

  test('it pulses the skeleton until the artwork loads', () => {
    const { getByRole, getByTestId } = render(() => (
      <Cover title="Kid A" coverHash="abc123" />
    ));
    expect(getByTestId('cover-skeleton')).toHaveClass('animate-pulse');
    getByRole('presentation').dispatchEvent(new Event('load'));
    // Solid 2.0 schedules updates; flush applies them before asserting.
    flush();
    expect(getByTestId('cover-skeleton')).not.toHaveClass('animate-pulse');
  });

  test('it falls back to the placeholder when the artwork fails to load', () => {
    const { getByRole, getByLabelText, getByTestId, queryByRole } = render(() => (
      <Cover title="Kid A" coverHash="abc123" />
    ));
    getByRole('presentation').dispatchEvent(new Event('error'));
    flush();
    expect(queryByRole('presentation')).toBeNull();
    expect(getByLabelText('Kid A — no cover art')).toBeInTheDocument();
    expect(getByTestId('cover-skeleton')).not.toHaveClass('animate-pulse');
  });
});
