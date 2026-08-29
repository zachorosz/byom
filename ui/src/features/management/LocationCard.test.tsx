import { fireEvent, render } from "@solidjs/testing-library";
import { Code, ConnectError } from "@connectrpc/connect";
import type { Location } from "@proto/management/v1/location_pb";
import { ScanState, type Scan } from "@proto/management/v1/scan_pb";
import { Loading } from "solid-js";
import { beforeEach, describe, expect, test } from "vitest";
import { createRouter, memoryHistory, query } from "@solidjs/router";

import LocationCard from "./LocationCard";
import { fakeManagement } from "../../lib/rpc/testing";

const location = { id: "l1", path: "/mnt/music" } as Location;

const scan = (over: Partial<Scan>): Scan =>
  ({
    id: "s1",
    locationId: "l1",
    state: ScanState.DONE,
    error: "",
    finishTime: {
      seconds: BigInt(Math.floor(Date.now() / 1000)) - 172_800n,
      nanos: 0,
    },
    progress: {
      dirsSeen: 412n,
      dirsMissing: 0n,
      filesSeen: 3118n,
      filesMissing: 0n,
    },
    ...over,
  }) as Scan;

function card(running: Scan | undefined) {
  const TestRouter = createRouter({
    routes: [
      {
        path: "/",
        component: () => (
          <Loading fallback={<span>loading</span>}>
            <LocationCard location={location} running={running} />
          </Loading>
        ),
      },
    ],
    history: memoryHistory("/"),
  });
  return render(() => <TestRouter />);
}

describe("<LocationCard />", () => {
  beforeEach(() => query.clear());

  test("it shows the source path", async () => {
    fakeManagement({ listScans: () => ({ items: [], nextPageToken: "" }) });
    const { findByText } = card(undefined);
    expect(await findByText("/mnt/music")).toBeInTheDocument();
  });

  test("an idle source offers a Scan action", async () => {
    fakeManagement({ listScans: () => ({ items: [], nextPageToken: "" }) });
    const { findByRole } = card(undefined);
    expect(
      await findByRole("button", { name: "Force Rescan" }),
    ).toBeInTheDocument();
  });

  test("a source that has never been scanned says so", async () => {
    fakeManagement({ listScans: () => ({ items: [], nextPageToken: "" }) });
    const { findByText } = card(undefined);
    expect(await findByText(/never scanned/i)).toBeInTheDocument();
  });

  test("it summarises the last scan with state, age and counts", async () => {
    fakeManagement({
      listScans: () => ({ items: [scan({})], nextPageToken: "" }),
    });
    const { findByText } = card(undefined);
    expect(await findByText(/DONE/)).toBeInTheDocument();
    expect(await findByText(/2d ago/)).toBeInTheDocument();
    expect(await findByText(/412 dirs/)).toBeInTheDocument();
  });

  test("a failed last scan surfaces its error on the row", async () => {
    fakeManagement({
      listScans: () => ({
        items: [scan({ state: ScanState.FAILED, error: "permission denied" })],
        nextPageToken: "",
      }),
    });
    const { findByText } = card(undefined);
    expect(await findByText(/permission denied/)).toBeInTheDocument();
  });

  test("a running source shows the scan panel instead of a Scan button", async () => {
    fakeManagement({ listScans: () => ({ items: [], nextPageToken: "" }) });
    const { findByRole, queryByRole } = card(
      scan({ state: ScanState.RUNNING }),
    );
    expect(await findByRole("progressbar")).toBeInTheDocument();
    expect(queryByRole("button", { name: "Force Rescan" })).toBeNull();
  });

  test("a failing scan start surfaces the error on the row", async () => {
    fakeManagement({
      listScans: () => ({ items: [], nextPageToken: "" }),
      scanLocation: () => {
        throw new ConnectError("permission denied", Code.PermissionDenied);
      },
    });
    const { findByRole, findByText } = card(undefined);
    fireEvent.click(await findByRole("button", { name: "Force Rescan" }));
    expect(await findByText(/permission denied/)).toBeInTheDocument();
  });

  test("a failing cancel surfaces the error on the row", async () => {
    fakeManagement({
      listScans: () => ({ items: [], nextPageToken: "" }),
      cancelScan: () => {
        throw new ConnectError("already finished", Code.FailedPrecondition);
      },
    });
    const { findByRole, findByText } = card(scan({ state: ScanState.RUNNING }));
    fireEvent.click(await findByRole("button", { name: "Cancel" }));
    expect(await findByText(/already finished/)).toBeInTheDocument();
  });
});
