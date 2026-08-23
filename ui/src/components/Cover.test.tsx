import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';

import Cover from './Cover';
import { coverGradient } from '../lib/format';

describe('<Cover />', () => {
  test('it renders the title initials when there is no artwork', () => {
    const { getByText } = render(() => <Cover id="a1" title="Kid A" />);
    expect(getByText('KA')).toBeInTheDocument();
  });

  test('it paints the deterministic gradient for the id', () => {
    const { getByTestId } = render(() => <Cover id="a1" title="Kid A" />);
    const { from, to } = coverGradient('a1');
    expect(getByTestId('cover').style.backgroundImage).toContain(from);
    expect(getByTestId('cover').style.backgroundImage).toContain(to);
  });

  test('it labels the placeholder for screen readers', () => {
    const { getByLabelText } = render(() => <Cover id="a1" title="Kid A" />);
    expect(getByLabelText('Kid A — no cover art')).toBeInTheDocument();
  });
});
