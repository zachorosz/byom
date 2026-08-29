import { createRoot, createSignal, flush } from 'solid-js';
import { beforeEach, describe, expect, test } from 'vitest';

import { clearListCache, createInfiniteList } from './pagination';

interface Row {
  id: string;
}

/** page returns a fetcher that walks a fixed set of pages by token. */
function page(pages: Record<string, { items: Row[]; nextPageToken: string }>) {
  const calls: string[] = [];
  const fetchPage = async (token: string) => {
    calls.push(token);
    return pages[token]!;
  };
  return { fetchPage, calls };
}

describe('createInfiniteList', () => {
  beforeEach(() => clearListCache());

  test('it loads the first page immediately', async () => {
    const { fetchPage } = page({
      '': { items: [{ id: 'a' }], nextPageToken: 'p2' },
    });
    await createRoot(async (dispose) => {
      const list = createInfiniteList(() => 'albums:', fetchPage);
      await flushAsync();
      expect(list.items.map((r) => r.id)).toEqual(['a']);
      dispose();
    });
  });

  test('loadMore appends the next page', async () => {
    const { fetchPage } = page({
      '': { items: [{ id: 'a' }], nextPageToken: 'p2' },
      p2: { items: [{ id: 'b' }], nextPageToken: '' },
    });
    await createRoot(async (dispose) => {
      const list = createInfiniteList(() => 'albums:', fetchPage);
      await flushAsync();
      await list.loadMore();
      expect(list.items.map((r) => r.id)).toEqual(['a', 'b']);
      expect(list.done()).toBe(true);
      dispose();
    });
  });

  test('an empty next page token marks the list done', async () => {
    const { fetchPage } = page({
      '': { items: [{ id: 'a' }], nextPageToken: '' },
    });
    await createRoot(async (dispose) => {
      const list = createInfiniteList(() => 'albums:', fetchPage);
      await flushAsync();
      expect(list.done()).toBe(true);
      dispose();
    });
  });

  // The key must be a signal, not a plain `let`. createEffect(key, fn) tracks its
  // source; a derive with no reactive dependencies never recomputes.
  test('a key change starts a fresh accumulation instead of appending', async () => {
    const byKey: Record<string, Row[]> = {
      'albums:all': [{ id: 'a' }],
      'albums:ep': [{ id: 'x' }],
    };
    await createRoot(async (dispose) => {
      const [key, setKey] = createSignal('albums:all');
      const list = createInfiniteList(key, async () => ({
        items: byKey[key()]!,
        nextPageToken: '',
      }));
      await flushAsync();
      expect(list.items.map((r) => r.id)).toEqual(['a']);

      setKey('albums:ep');
      await flushAsync();
      expect(list.items.map((r) => r.id)).toEqual(['x']);
      dispose();
    });
  });

  test('returning to a previous key restores its items without refetching', async () => {
    const { fetchPage, calls } = page({
      '': { items: [{ id: 'a' }], nextPageToken: '' },
    });
    await createRoot(async (dispose) => {
      createInfiniteList(() => 'albums:all', fetchPage);
      await flushAsync();
      dispose();
    });
    await createRoot(async (dispose) => {
      const list = createInfiniteList(() => 'albums:all', fetchPage);
      await flushAsync();
      expect(list.items.map((r) => r.id)).toEqual(['a']);
      expect(calls.length).toBe(1);
      dispose();
    });
  });

  test('clearListCache forces the next mount to refetch', async () => {
    const { fetchPage, calls } = page({
      '': { items: [{ id: 'a' }], nextPageToken: '' },
    });
    await createRoot(async (dispose) => {
      createInfiniteList(() => 'albums:all', fetchPage);
      await flushAsync();
      dispose();
    });
    clearListCache();
    await createRoot(async (dispose) => {
      createInfiniteList(() => 'albums:all', fetchPage);
      await flushAsync();
      expect(calls.length).toBe(2);
      dispose();
    });
  });

  // Regression: in-flight state must be per key. With a single shared `loading`
  // signal, the new key's loadMore early-returns while the old fetch is still
  // running and the new list is left permanently empty.
  test('a key change during an in-flight fetch still loads the new list', async () => {
    let resolveFirst:
      ((p: { items: Row[]; nextPageToken: string }) => void) | undefined;
    await createRoot(async (dispose) => {
      const [key, setKey] = createSignal('albums:all');
      const list = createInfiniteList(key, () => {
        if (key() === 'albums:all') {
          return new Promise<{ items: Row[]; nextPageToken: string }>(
            (resolve) => {
              resolveFirst = resolve;
            }
          );
        }
        return Promise.resolve({ items: [{ id: 'x' }], nextPageToken: '' });
      });
      await flushAsync();
      expect(list.items.length).toBe(0);

      setKey('albums:ep');
      await flushAsync();
      expect(list.items.map((r) => r.id)).toEqual(['x']);

      // The superseded fetch must not leak into the list now showing.
      resolveFirst!({ items: [{ id: 'a' }], nextPageToken: '' });
      await flushAsync();
      expect(list.items.map((r) => r.id)).toEqual(['x']);
      dispose();
    });
  });

  test('a fetch failure keeps the items already accumulated', async () => {
    let first = true;
    await createRoot(async (dispose) => {
      const list = createInfiniteList(
        () => 'albums:all',
        async () => {
          if (first) {
            first = false;
            return { items: [{ id: 'a' }], nextPageToken: 'p2' };
          }
          throw new Error('boom');
        }
      );
      await flushAsync();
      await list.loadMore();
      expect(list.items.map((r) => r.id)).toEqual(['a']);
      expect(String(list.error())).toContain('boom');
      dispose();
    });
  });

  test('the accumulator remembers a scroll offset per key', async () => {
    const { fetchPage } = page({
      '': { items: [{ id: 'a' }], nextPageToken: '' },
    });
    await createRoot(async (dispose) => {
      const list = createInfiniteList(() => 'albums:all', fetchPage);
      await flushAsync();
      list.setScrollY(420);
      dispose();
    });
    await createRoot(async (dispose) => {
      const list = createInfiniteList(() => 'albums:all', fetchPage);
      await flushAsync();
      expect(list.scrollY()).toBe(420);
      dispose();
    });
  });
});

/** flushAsync lets pending promises settle, then applies Solid's batched updates. */
async function flushAsync() {
  await new Promise((resolve) => setTimeout(resolve, 0));
  flush();
}
