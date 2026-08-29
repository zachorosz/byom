import { render } from '@solidjs/testing-library';
import { query } from '@solidjs/router';
import { beforeEach, describe, expect, test } from 'vitest';

import App from './App';
import { fakeManagement } from './lib/rpc/testing';

describe('<App />', () => {
  // Shell starts the scan monitor on mount; fake the transport so it never
  // hits the network. An empty running-scan list also keeps the indicator hidden.
  beforeEach(() => {
    query.clear();
    fakeManagement({ listScans: () => ({ items: [], nextPageToken: '' }) });
  });

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
