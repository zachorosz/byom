import { createSignal, Loading, Show } from "solid-js";
import type { Location } from "@proto/management/v1/location_pb";
import type { Scan } from "@proto/management/v1/scan_pb";

import Button from "../../components/Button";
import { cancelScan, scanLocation } from "../../lib/rpc/management";
import { pokeScanMonitor } from "../../lib/rpc/scan-monitor";
import ScanHistory from "./ScanHistory";
import ScanPanel from "./ScanPanel";

interface LocationCardProps {
  location: Location;
  running: Scan | undefined;
}

/** LocationCard renders one library source: its path, scan action, and history. */
export default function LocationCard(props: LocationCardProps) {
  const [showHistory, setShowHistory] = createSignal(false);
  const [error, setError] = createSignal("");

  async function start() {
    setError("");
    try {
      await scanLocation(props.location.id, true);
      pokeScanMonitor();
    } catch (e) {
      setError(String(e));
    }
  }

  async function stop() {
    if (!props.running) return;
    setError("");
    try {
      await cancelScan(props.running.id);
      pokeScanMonitor();
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <li class="border-line bg-panel mb-2.5 rounded border">
      <div class="flex items-center gap-3 px-3 py-2.5">
        <span class="flex-1 font-mono text-xs">{props.location.path}</span>
        <Show when={!props.running}>
          <Button variant="primary" onClick={() => void start()}>
            Force Rescan
          </Button>
        </Show>
        <button
          type="button"
          onClick={() => setShowHistory(!showHistory())}
          class="text-faint hover:text-muted font-mono text-[10px]"
        >
          history
        </button>
      </div>
      <Show when={error()}>
        <p class="text-danger px-3 pb-2.5 font-mono text-[10px]">{error()}</p>
      </Show>
      <Show
        when={props.running}
        fallback={
          // Its own boundary: one source's scan history must not hold the whole
          // settings page on the shell fallback while it loads.
          <Loading
            fallback={
              <p class="text-faint px-3 pb-2.5 font-mono text-[10px]">
                loading history…
              </p>
            }
          >
            <ScanHistory
              locationId={props.location.id}
              expanded={showHistory()}
            />
          </Loading>
        }
      >
        {(scan) => <ScanPanel scan={scan()} onCancel={() => void stop()} />}
      </Show>
    </li>
  );
}
