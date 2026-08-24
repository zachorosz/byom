import { Show } from 'solid-js';

import { runningScans } from '../../lib/rpc/scan-monitor';

/** ScanIndicator shows live scan activity in the sidebar, and nothing when idle. */
export default function ScanIndicator() {
  const dirs = () =>
    runningScans().reduce((total, scan) => total + Number(scan.progress?.dirsSeen ?? 0), 0);

  return (
    <Show when={runningScans().length > 0}>
      <a
        href="/settings"
        class="text-accent flex items-center gap-1.5 px-2 py-1 font-mono text-[9px] no-underline"
      >
        <span class="bg-accent h-1.5 w-1.5 flex-none rounded-full" />
        SCANNING · {dirs().toLocaleString()} dirs
      </a>
    </Show>
  );
}
