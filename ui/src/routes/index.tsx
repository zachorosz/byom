import { useNavigate } from '@solidjs/router';

// No RPC supplies recently-added, play history, or statistics, so there is
// nothing to put on a home page. Browsing starts at the albums grid.
export default function Home() {
  // Redirect while rendering, not from onSettled: the router's first
  // navigation calls flush(), which throws inside a tracked queue callback.
  useNavigate()('/albums', { replace: true });
  return null;
}
