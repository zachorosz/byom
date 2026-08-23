import type { ParentProps } from 'solid-js';

interface ButtonProps extends ParentProps {
  variant?: 'default' | 'primary';
  disabled?: boolean;
  onClick?: () => void;
}

/** Button renders an action. The primary variant carries the accent and is used once per view. */
export default function Button(props: ButtonProps) {
  return (
    <button
      type="button"
      disabled={props.disabled}
      onClick={() => props.onClick?.()}
      class={[
        'rounded border px-3 py-1 text-xs transition-colors disabled:opacity-40',
        {
          'border-line bg-panel text-muted hover:text-ink': props.variant !== 'primary',
          'border-accent-dim bg-accent-tint text-accent hover:brightness-125':
            props.variant === 'primary',
        },
      ]}
    >
      {props.children}
    </button>
  );
}
