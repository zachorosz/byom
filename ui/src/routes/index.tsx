import { useNavigate } from '@solidjs/router';

// No RPC supplies recently-added, play history, or statistics, so there is
// nothing to put on a home page. Browsing starts at the albums grid.
export default function Home() {
  const navigate = useNavigate();
  navigate('/albums', { replace: true });
  return null;
}
