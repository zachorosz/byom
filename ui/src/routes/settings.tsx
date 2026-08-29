import { Title } from '@solidjs/meta';
import { revalidate, type RouteDefinition } from '@solidjs/router';
import { createMemo, For, Show } from 'solid-js';

import AddSourceForm from '../features/management/AddSourceForm';
import LocationCard from '../features/management/LocationCard';
import { listLocations } from '../lib/rpc/management';
import { runningScans } from '../lib/rpc/scan-monitor';

export const route = {
  preload: () => void listLocations(),
} satisfies RouteDefinition;

export default function Settings() {
  // Await: the query returns a Promise, so reading `.items` off it directly
  // yields undefined.
  const locations = createMemo(async () => (await listLocations()).items);
  const runningFor = (locationId: string) =>
    runningScans().find((scan) => scan.locationId === locationId);

  return (
    <main class="max-w-3xl px-6 py-8">
      <Title>Settings - byom</Title>
      <h1 class="font-serif text-2xl">Library sources</h1>
      <p class="text-muted mt-1 mb-5 text-xs">
        Directories byom walks. Scanning is incremental — only changed folders are
        re-parsed.
      </p>
      <ul>
        <For each={locations()}>
          {(location) => (
            <LocationCard location={location} running={runningFor(location.id)} />
          )}
        </For>
      </ul>
      <Show when={locations().length === 0}>
        <p class="text-muted mb-4 text-sm">No sources yet.</p>
      </Show>
      <AddSourceForm onAdded={() => revalidate(listLocations.key)} />
    </main>
  );
}
