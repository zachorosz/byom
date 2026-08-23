import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';

import App from './App';

describe('<App />', () => {
  test('the sidebar links to the two library sections and settings', () => {
    const { getByRole } = render(() => <App />);
    expect(getByRole('link', { name: 'Artists' })).toHaveAttribute('href', '/artists');
    expect(getByRole('link', { name: 'Albums' })).toHaveAttribute('href', '/albums');
    expect(getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/settings');
  });

  test('the sidebar groups the library nav under a labelled heading', () => {
    const { getByText } = render(() => <App />);
    expect(getByText('Library')).toBeInTheDocument();
  });
});
