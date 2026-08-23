import { createEffect, createSignal, onSettled } from "solid-js";

interface InfiniteScrollSentinelProps {
  onIntersect: () => void;
  disabled?: boolean;
}

// InfiniteScrollSentinel calls onIntersect when its element approaches the
// viewport, unless disabled.
export default function InfiniteScrollSentinel(
  props: InfiniteScrollSentinelProps,
) {
  let ref: HTMLDivElement | undefined;
  // Intersecting and disabled are tracked as separate signals rather than
  // handled inside the observer callback because IntersectionObserver only
  // fires on state changes — a page short enough to leave the sentinel visible
  // after loading would never get a second "entering" event otherwise.
  const [intersecting, setIntersecting] = createSignal(false);

  onSettled(() => {
    const observer = new IntersectionObserver(
      (entries) => setIntersecting(!!entries[0]?.isIntersecting),
      { rootMargin: "200px" },
    );
    if (ref) observer.observe(ref);
    return () => observer.disconnect();
  });

  createEffect(
    () => intersecting() && !props.disabled,
    (shouldLoad) => {
      if (shouldLoad) props.onIntersect();
    },
  );

  return <div ref={ref} class="h-1" />;
}
