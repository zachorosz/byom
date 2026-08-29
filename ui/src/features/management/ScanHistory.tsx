import { createMemo, For, Show } from 'solid-js';
import { ScanState, type Scan } from '@proto/management/v1/scan_pb';

import { relativeTime } from '../../lib/format';
import { listScansFor } from '../../lib/rpc/management';

interface ScanHistoryProps {
  locationId: string;
  expanded: boolean;
}

const STATE_LABELS: Record<ScanState, string> = {
  [ScanState.UNSPECIFIED]: 'UNKNOWN',
  [ScanState.RUNNING]: 'RUNNING',
  [ScanState.CANCELLING]: 'CANCELLING',
  [ScanState.DONE]: 'DONE',
  [ScanState.FAILED]: 'FAILED',
  [ScanState.ABORTED]: 'ABORTED',
};

function ScanRow(props: { scan: Scan }) {
  const failed = () =>
    props.scan.state === ScanState.FAILED ||
    props.scan.state === ScanState.ABORTED;

  return (
    <div class="text-faint flex flex-wrap gap-x-4 py-1 font-mono text-[10px]">
      <span class={failed() ? 'text-danger' : ''}>
        {STATE_LABELS[props.scan.state]}
      </span>
      <span>{relativeTime(props.scan.finishTime ?? props.scan.startTime)}</span>
      <span>
        {Number(props.scan.progress?.dirsSeen ?? 0).toLocaleString()} dirs
      </span>
      <span>
        {Number(props.scan.progress?.filesSeen ?? 0).toLocaleString()} files
      </span>
      <Show when={props.scan.error}>
        <span class="truncate text-danger">{props.scan.error}</span>
      </Show>
    </div>
  );
}

/**
 * ScanHistory summarises a source's most recent scan, and lists the rest when
 * expanded. Renders nothing but a note when the source has never been scanned.
 */
export default function ScanHistory(props: ScanHistoryProps) {
  const scans = createMemo(
    async () => (await listScansFor(props.locationId)).items
  );

  return (
    <div class="px-3 pb-2.5">
      <Show
        when={scans().length > 0}
        fallback={<p class="text-faint font-mono text-[10px]">never scanned</p>}
      >
        <ScanRow scan={scans()[0]!} />
        <Show when={props.expanded}>
          <div class="border-line mt-1.5 border-t pt-1.5">
            <For each={scans().slice(1)}>
              {(scan) => <ScanRow scan={scan} />}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}
