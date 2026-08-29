import { useNavigate } from '@solidjs/router';
import { onSettled } from 'solid-js';

// No RPC supplies recently-added, play history, or statistics, so there is
// nothing to put on a home page. Browsing starts at the albums grid.
export default function Home() {
  const navigate = useNavigate();
  onSettled(() => navigate('/albums', { replace: true }));
  return null;
}
