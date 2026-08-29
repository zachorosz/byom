interface CoverProps {
  title: string;
}

/**
 * Cover renders an album's square artwork.
 *
 * No image bytes are reachable from the frontend yet, so this always draws a
 * skeleton. When an image endpoint lands, only this component changes.
 */
export default function Cover(props: CoverProps) {
  return (
    <div
      data-testid="cover"
      role="img"
      aria-label={`${props.title} — no cover art`}
      class="bg-panel border-line aspect-square w-full animate-pulse rounded-[2px] border"
    />
  );
}
