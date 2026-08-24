// Registers @testing-library/jest-dom's matchers (toHaveTextContent etc.)
// with vitest's expect, including their types.
import '@testing-library/jest-dom/vitest';

// jsdom does not implement scrollTo; the app calls it for scroll restoration.
window.scrollTo = () => {};

// jsdom does not implement IntersectionObserver; InfiniteScrollSentinel
// needs the constructor to exist even though nothing here ever intersects.
class FakeIntersectionObserver {
  readonly root = null;
  readonly rootMargin = '';
  readonly thresholds: ReadonlyArray<number> = [];
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords(): IntersectionObserverEntry[] {
    return [];
  }
}
window.IntersectionObserver =
  FakeIntersectionObserver as unknown as typeof IntersectionObserver;
