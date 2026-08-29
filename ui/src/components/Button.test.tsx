import { fireEvent, render } from '@solidjs/testing-library';
import { describe, expect, test, vi } from 'vitest';

import Button from './Button';

describe('<Button />', () => {
  test('it calls onClick when pressed', () => {
    const onClick = vi.fn();
    const { getByRole } = render(() => <Button onClick={onClick}>Scan</Button>);
    fireEvent.click(getByRole('button'));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  test('a disabled button does not call onClick', () => {
    const onClick = vi.fn();
    const { getByRole } = render(() => (
      <Button onClick={onClick} disabled>
        Scan
      </Button>
    ));
    fireEvent.click(getByRole('button'));
    expect(onClick).not.toHaveBeenCalled();
  });

  test('the primary variant carries the accent border', () => {
    const { getByRole } = render(() => <Button variant="primary">Scan</Button>);
    expect(getByRole('button').className).toContain('border-accent-dim');
  });
});
