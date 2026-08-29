import { createEffect, createSignal, createStore, onSettled, untrack } from 'solid-js';

interface Page<T> {
  items: T[];
  nextPageToken: string;
}

interface Entry<T> {
  items: T[];
  pageToken: string;
  done: boolean;
  scrollY: number;
  // In-flight lives per key, not in a shared signal: otherwise a key change
  // during a fetch makes the new key's loadMore early-return and never retry.
  inflight: boolean;
}

// Accumulated pages, keyed by filter set. Module-level so returning to a list —
// from a detail page, or via back/forward — restores it without refetching.
const cache = new Map<string, Entry<unknown>>();

/** clearListCache drops every accumulated list. Called by invalidateLibrary. */
export function clearListCache(): void {
  cache.clear();
}

function entryFor<T>(key: string): Entry<T> {
  let entry = cache.get(key) as Entry<T> | undefined;
  if (!entry) {
    entry = { items: [], pageToken: '', done: false, scrollY: 0, inflight: false };
    cache.set(key, entry as Entry<unknown>);
  }
  return entry;
}

/**
 * createInfiniteList accumulates pages from a token-cursored list RPC into one
 * growing list, loading the first page immediately.
 *
 * The key identifies the filter set. A page token is only meaningful within the
 * filters that issued it, so changing the key starts a fresh accumulation rather
 * than appending; the previous key's items stay cached under their own entry.
 */
export function createInfiniteList<T>(
  key: () => string,
  fetchPage: (pageToken: string) => Promise<Page<T>>,
) {
  const [items, setItems] = createStore<T[]>([]);
  const [loading, setLoading] = createSignal(false);
  const [done, setDone] = createSignal(false);
  const [error, setError] = createSignal<unknown>();
  const [scrollY, setScrollY] = createSignal(0);

  // loadMore is called imperatively (from an event or an effect's non-tracking
  // effect function), so its reads are untracked on purpose.
  async function loadMore() {
    const k = untrack(key);
    const entry = entryFor<T>(k);
    if (entry.inflight || entry.done) return;
    entry.inflight = true;
    setLoading(true);
    try {
      const page = await fetchPage(entry.pageToken);
      // The entry belongs to k, so these mutations are always correct even if
      // the key has since changed; only the signals are guarded below.
      entry.items = entry.items.concat(page.items);
      entry.pageToken = page.nextPageToken;
      entry.done = !page.nextPageToken;
      if (untrack(key) !== k) return; // filters changed mid-flight; drop the result
      setItems(() => entry.items.slice());
      setDone(entry.done);
      setError(undefined);
    } catch (e) {
      if (untrack(key) === k) setError(e);
    } finally {
      entry.inflight = false;
      // Leave loading set if a newer key started its own fetch in the meantime.
      if (untrack(key) === k) setLoading(false);
    }
  }

  createEffect(key, (k) => {
    const entry = entryFor<T>(k);
    setItems(() => entry.items.slice());
    setDone(entry.done);
    setScrollY(entry.scrollY);
    setError(undefined);
    if (entry.items.length === 0 && !entry.done) void loadMore();
  });

  return {
    items,
    loading,
    done,
    error,
    loadMore,
    scrollY,
    setScrollY: (y: number) => {
      entryFor(untrack(key)).scrollY = y;
      setScrollY(y);
    },
  };
}

interface Restorable {
  scrollY: () => number;
  setScrollY: (y: number) => void;
}

/**
 * createScrollRestoration restores a list's scroll offset on mount and records
 * it as the reader scrolls, so returning to a list lands where they left it.
 */
export function createScrollRestoration(list: Restorable): void {
  onSettled(() => {
    const saved = list.scrollY();
    if (saved > 0) window.scrollTo(0, saved);

    const onScroll = () => list.setScrollY(window.scrollY);
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  });
}
