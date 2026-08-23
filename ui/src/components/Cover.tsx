import { coverGradient, coverInitials } from '../lib/format';

interface CoverProps {
  id: string;
  title: string;
  size?: 'tile' | 'hero';
}

/**
 * Cover renders an album's square artwork.
 *
 * No image bytes are reachable from the frontend yet, so this always draws the
 * placeholder: a gradient derived from the id, stable across reloads. When an
 * image endpoint lands, only this component changes.
 */
export default function Cover(props: CoverProps) {
  const gradient = () => coverGradient(props.id);

  return (
    <div
      data-testid="cover"
      role="img"
      aria-label={`${props.title} — no cover art`}
      class={{
        'flex aspect-square w-full items-center justify-center rounded-[2px] font-serif text-white/70 shadow-lg shadow-black/50': true,
        'text-2xl': props.size !== 'hero',
        'text-5xl': props.size === 'hero',
      }}
      style={{
        'background-image': `linear-gradient(155deg, ${gradient().from}, ${gradient().to})`,
      }}
    >
      {coverInitials(props.title)}
    </div>
  );
}
