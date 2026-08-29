import { createSignal, Show } from 'solid-js';

interface CoverProps {
  title: string;
  coverHash?: string;
  size?: 'tile' | 'hero';
}

// The hero renders at the same width as a tile but is the page's focal
// point, so it takes the sharper pair.
const WIDTHS = {
  tile: [160, 320],
  hero: [320, 640],
} as const;

/**
 * Cover renders an album's square artwork.
 *
 * The placeholder sits under the image and only pulses while artwork is
 * actually in flight; an album with no hash rests as a flat tile rather than
 * promising a cover that will never arrive.
 */
export default function Cover(props: CoverProps) {
  const [loaded, setLoaded] = createSignal(false);
  const [failed, setFailed] = createSignal(false);

  const widths = () => WIDTHS[props.size === 'hero' ? 'hero' : 'tile'];
  const src = (width: number) => `/images/${props.coverHash}?size=${width}`;
  const hasArtwork = () => Boolean(props.coverHash) && !failed();
  const pending = () => hasArtwork() && !loaded();

  return (
    <div
      data-testid="cover"
      role="img"
      aria-label={hasArtwork() ? props.title : `${props.title} — no cover art`}
      class="relative aspect-square w-full overflow-hidden rounded-[2px] shadow-lg shadow-black/50"
    >
      <div
        data-testid="cover-skeleton"
        class={[
          'bg-panel border-line absolute inset-0 border',
          { 'animate-pulse': pending() },
        ]}
      />
      <Show when={hasArtwork()}>
        <img
          src={src(widths()[0])}
          srcset={`${src(widths()[0])} 1x, ${src(widths()[1])} 2x`}
          alt=""
          loading="lazy"
          decoding="async"
          class={[
            'absolute inset-0 h-full w-full object-cover transition-opacity duration-300',
            { 'opacity-0': !loaded(), 'opacity-100': loaded() },
          ]}
          onLoad={() => setLoaded(true)}
          onError={() => setFailed(true)}
        />
      </Show>
    </div>
  );
}
