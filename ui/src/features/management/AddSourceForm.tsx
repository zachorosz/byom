import { createSignal, Show } from 'solid-js';

import Button from '../../components/Button';
import { createLocation } from '../../lib/rpc/management';

interface AddSourceFormProps {
  onAdded: () => void;
}

/** AddSourceForm adds a library source by filesystem path. */
export default function AddSourceForm(props: AddSourceFormProps) {
  const [open, setOpen] = createSignal(false);
  const [path, setPath] = createSignal('');
  const [error, setError] = createSignal('');

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    setError('');
    try {
      await createLocation(path());
      setPath('');
      setOpen(false);
      props.onAdded();
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <Show
      when={open()}
      fallback={<Button onClick={() => setOpen(true)}>+ Add source</Button>}
    >
      <form onSubmit={(e) => void submit(e)} class="flex items-center gap-2">
        <input
          value={path()}
          onInput={(e) => setPath(e.currentTarget.value)}
          placeholder="/mnt/music"
          aria-label="Source path"
          class="border-line bg-ground text-ink flex-1 rounded border px-2.5 py-1.5 font-mono text-xs"
        />
        {/* Not <Button>: it renders type="button" and would not submit the form. */}
        <button
          type="submit"
          class="border-accent-dim bg-accent-tint text-accent rounded border px-3 py-1 text-xs"
        >
          Add
        </button>
        <Button onClick={() => setOpen(false)}>Cancel</Button>
        <Show when={error()}>
          <span class="font-mono text-[10px] text-danger">{error()}</span>
        </Show>
      </form>
    </Show>
  );
}
