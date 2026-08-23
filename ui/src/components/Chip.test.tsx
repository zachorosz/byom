import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';

import Chip from './Chip';

describe('<Chip />', () => {
  test('it renders its label', () => {
    const { getByText } = render(() => <Chip>ALBUM</Chip>);
    expect(getByText('ALBUM')).toBeInTheDocument();
  });

  test('a plain chip uses the muted foreground', () => {
    const { getByText } = render(() => <Chip>ALBUM</Chip>);
    expect(getByText('ALBUM').className).toContain('text-muted');
  });

  test('an accent chip uses the accent foreground', () => {
    const { getByText } = render(() => <Chip accent>LIVE</Chip>);
    expect(getByText('LIVE').className).toContain('text-accent');
  });
});
