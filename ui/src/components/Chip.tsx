import type { ParentProps } from 'solid-js';

interface ChipProps extends ParentProps {
  accent?: boolean;
}

/** Chip renders a mono uppercase metadata tag. Set accent for a flag worth noticing. */
export default function Chip(props: ChipProps) {
  return (
    <span
      class={[
        'border-line rounded-[2px] border px-1.5 py-0.5 font-mono text-[9px] tracking-[0.08em]',
        {
          'text-muted bg-panel': !props.accent,
          'text-accent border-accent-dim bg-accent-tint': !!props.accent,
        },
      ]}
    >
      {props.children}
    </span>
  );
}
