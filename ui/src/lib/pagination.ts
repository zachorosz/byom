import { createSignal, createStore, onSettled, untrack } from "solid-js";

interface Page<T> {
  items: T[];
  nextPageToken: string;
}

// createInfiniteList accumulates pages from a token-cursored list RPC
// into a single growing list, loading the first page immediately.
export function createInfiniteList<T>(
  fetchPage: (pageToken: string) => Promise<Page<T>>,
) {
  const [items, setItems] = createStore<T[]>([]);
  const [pageToken, setPageToken] = createSignal("");
  const [loading, setLoading] = createSignal(false);
  const [done, setDone] = createSignal(false);
  const [error, setError] = createSignal<unknown>();

  // loadMore is called imperatively (from an event or an effect's non-tracking
  // effect function), so its reads are untracked on purpose.
  async function loadMore() {
    if (untrack(() => loading() || done())) return;
    setLoading(true);
    try {
      const page = await fetchPage(untrack(pageToken));
      setItems((list) => {
        list.push(...page.items);
      });
      setPageToken(page.nextPageToken);
      if (!page.nextPageToken) setDone(true);
    } catch (e) {
      setError(e);
    } finally {
      setLoading(false);
    }
  }

  onSettled(() => void loadMore());

  return { items, loading, done, error, loadMore };
}
