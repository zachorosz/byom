import { render } from '@solidjs/testing-library';
import { describe, expect, test } from 'vitest';

import Document from './Document';

describe('<Document />', () => {
  test('it links the three font families the design system uses', () => {
    const { container } = render(() => (
      <Document>
        <div />
      </Document>
    ));
    const fonts = container.querySelector('link[href*="fonts.googleapis.com"]');
    expect(fonts).toBeTruthy();
    expect(fonts!.getAttribute('href')).toContain('Spectral');
    expect(fonts!.getAttribute('href')).toContain('Inter');
    expect(fonts!.getAttribute('href')).toContain('JetBrains+Mono');
  });

  test('it titles the document byom', () => {
    const { container } = render(() => (
      <Document>
        <div />
      </Document>
    ));
    expect(container.querySelector('title')?.textContent).toBe('byom');
  });
});
