import { fireEvent, render } from '@solidjs/testing-library';
import { ScanState, type Scan } from '@proto/management/v1/scan_pb';
import { describe, expect, test, vi } from 'vitest';

import ScanPanel from './ScanPanel';

const scan = (over: Partial<Scan> = {}): Scan =>
  ({
    id: 's1',
    locationId: 'l1',
    state: ScanState.RUNNING,
    error: '',
    startTime: {
      seconds: BigInt(Math.floor(Date.now() / 1000)) - 494n,
      nanos: 0,
    },
    progress: {
      dirsSeen: 1204n,
      dirsMissing: 0n,
      filesSeen: 18332n,
      filesMissing: 0n,
    },
    ...over,
  }) as Scan;

describe('<ScanPanel />', () => {
  test('it shows the live counters', () => {
    const { getByText } = render(() => (
      <ScanPanel scan={scan()} onCancel={() => {}} />
    ));
    expect(getByText(/1,204/)).toBeInTheDocument();
    expect(getByText(/18,332/)).toBeInTheDocument();
  });

  test('the progress bar is indeterminate — it carries no value attributes', () => {
    const { getByRole } = render(() => (
      <ScanPanel scan={scan()} onCancel={() => {}} />
    ));
    const bar = getByRole('progressbar');
    expect(bar).not.toHaveAttribute('aria-valuenow');
    expect(bar).toHaveAttribute('aria-valuetext', 'Scanning');
  });

  test('cancel calls back', () => {
    const onCancel = vi.fn();
    const { getByRole } = render(() => (
      <ScanPanel scan={scan()} onCancel={onCancel} />
    ));
    fireEvent.click(getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  test('a cancelling scan disables the cancel button', () => {
    const { getByRole } = render(() => (
      <ScanPanel
        scan={scan({ state: ScanState.CANCELLING })}
        onCancel={() => {}}
      />
    ));
    expect(getByRole('button', { name: 'Cancelling…' })).toBeDisabled();
  });
});
