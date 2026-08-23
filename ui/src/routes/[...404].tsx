import { Title } from '@solidjs/meta';
import type { RouteDefinition } from '@solidjs/router';
import { httpStatus } from '@solidjs/web';

// The catch-all route. httpStatus() is a no-op in the browser and takes
// effect when SSR is enabled; it runs in preload so the status code is set
// before the response head flushes.
export const route = {
  preload: () => httpStatus(404),
} satisfies RouteDefinition;

export default function NotFound() {
  return (
    <main class="px-6 py-12">
      <Title>Not Found - byom</Title>
      <h1 class="font-serif text-3xl">Not found</h1>
      <p class="text-muted mt-2 text-sm">
        No such page.{' '}
        <a href="/albums" class="text-accent underline underline-offset-4">
          Back to albums
        </a>
      </p>
    </main>
  );
}
