import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';

import Cover from './Cover';

describe('<Cover />', () => {
  test('it labels the placeholder for screen readers', () => {
    const { getByLabelText } = render(() => <Cover title="Kid A" />);
    expect(getByLabelText('Kid A — no cover art')).toBeInTheDocument();
  });

  test('it draws a skeleton rather than the title', () => {
    const { getByTestId, queryByText } = render(() => <Cover title="Kid A" />);
    expect(getByTestId('cover')).toHaveClass('animate-pulse');
    expect(queryByText('KA')).not.toBeInTheDocument();
  });
});
