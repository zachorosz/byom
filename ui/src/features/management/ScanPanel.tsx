import { ScanState, type Scan } from "@proto/management/v1/scan_pb";

import Button from "../../components/Button";

interface ScanPanelProps {
  scan: Scan;
  onCancel: () => void;
}

function elapsed(scan: Scan): string {
  if (!scan.startTime) return "--:--";
  const seconds = Math.max(
    0,
    Math.floor(Date.now() / 1000 - Number(scan.startTime.seconds)),
  );
  const minutes = Math.floor(seconds / 60);
  return `${minutes}:${String(seconds % 60).padStart(2, "0")}`;
}

/**
 * ScanPanel renders a scan in flight.
 */
export default function ScanPanel(props: ScanPanelProps) {
  const cancelling = () => props.scan.state === ScanState.CANCELLING;
  const count = (n?: bigint) => Number(n ?? 0).toLocaleString();

  return (
    <div class="border-line border-t px-3 py-2.5">
      <div class="flex items-center justify-between">
        <span class="text-muted font-mono text-[10px]">
          {cancelling() ? "CANCELLING" : "RUNNING"} · elapsed{" "}
          <b class="text-ink font-medium">{elapsed(props.scan)}</b>
        </span>
        <Button onClick={props.onCancel} disabled={cancelling()}>
          {cancelling() ? "Cancelling…" : "Cancel"}
        </Button>
      </div>
      <div
        role="progressbar"
        aria-valuetext="Scanning"
        class="bg-line mt-2 h-0.5 overflow-hidden rounded"
      >
        <div class="bg-accent animate-shuttle h-full w-1/3" />
      </div>
      <div class="text-muted mt-2 font-mono text-[10px]">
        dirs seen{" "}
        <b class="text-ink font-medium">
          {count(props.scan.progress?.dirsSeen)}
        </b>
        {"  "}files seen{" "}
        <b class="text-ink font-medium">
          {count(props.scan.progress?.filesSeen)}
        </b>
        {"  "}missing{" "}
        <b class="text-ink font-medium">
          {count(props.scan.progress?.dirsMissing)}
        </b>
      </div>
    </div>
  );
}
