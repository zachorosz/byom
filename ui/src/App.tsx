import { Title } from '@solidjs/meta';
import { Errored, Loading, onSettled, type ParentProps } from 'solid-js';
import ScanIndicator from './features/management/ScanIndicator';
import { startScanMonitor } from './lib/rpc/scan-monitor';
import { paths, Router } from './router';
import './App.css';

const NAV_LINK =
  'block rounded px-2 py-1 text-sm text-muted no-underline transition-colors hover:bg-sel hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent';

// error is an accessor, not a value — <Errored> hands the fallback a getter so
// the message tracks the latest captured error.
function Failed(props: { error: () => unknown; reset: () => void }) {
  return (
    <main class="px-6 py-12">
      <h1 class="font-serif text-2xl">Something went wrong</h1>
      <p class="text-muted mt-2 font-mono text-xs">{String(props.error())}</p>
      <button
        type="button"
        onClick={() => props.reset()}
        class="border-accent-dim bg-accent-tint text-accent mt-4 rounded border px-3 py-1 text-xs"
      >
        Retry
      </button>
    </main>
  );
}

function Shell(props: ParentProps) {
  onSettled(() => startScanMonitor());

  return (
    <div class="flex min-h-screen">
      {/* sticky + self-start keeps the sidebar in view while the window scrolls.
          Scrolling stays on the window because pagination restores window.scrollY. */}
      <nav class="bg-panel border-line sticky top-0 flex h-screen w-44 flex-none flex-col self-start border-r p-3">
        <a href={paths()} class="mb-5 px-2 font-serif text-xl italic no-underline">
          byom
        </a>
        <div class="text-faint px-2 pb-1.5 font-mono text-[8px] tracking-[0.16em] uppercase">
          Library
        </div>
        <a class={NAV_LINK} href={paths.artists()}>
          Artists
        </a>
        <a class={NAV_LINK} href={paths.albums()}>
          Albums
        </a>
        <div class="border-line mt-auto border-t pt-2">
          <ScanIndicator />
          <a class={NAV_LINK} href="/settings">
            Settings
          </a>
        </div>
      </nav>
      {/* Errored wraps Loading so a rejected async read renders the error UI
          rather than hanging on the loading fallback forever. */}
      <div class="flex-1 overflow-x-hidden">
        <Errored fallback={(error, reset) => <Failed error={error} reset={reset} />}>
          <Loading fallback={<main class="text-muted px-6 py-12">Loading…</main>}>
            {props.children}
          </Loading>
        </Errored>
      </div>
    </div>
  );
}

// The app root: the router and the site-wide shell. Pages are the modules
// under src/routes.
export default function App() {
  return (
    <Router>
      {(props) => (
        <>
          <Title>byom</Title>
          <Shell>{props.children}</Shell>
        </>
      )}
    </Router>
  );
}
