import type { ParentProps } from 'solid-js';
import { HydrationScript } from '@solidjs/web';

// The document shell — head tags go here. Compiled only into the prerendered
// static shell; <HydrationScript /> is stripped in client mode and activates
// if vite.config.ts flips to `ssr: true`.
export default function Document(props: ParentProps) {
  return (
    <html lang="en">
      <head>
        <meta charset="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" href="/favicon.ico" />
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="" />
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Spectral:ital,wght@0,400;0,600;1,400&family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap"
        />
        <title>byom</title>
        <HydrationScript />
      </head>
      <body class="bg-ground text-ink font-sans">{props.children}</body>
    </html>
  );
}
