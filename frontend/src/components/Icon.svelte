<script lang="ts">
  export let name: string
  export let size = 14

  const paths: Record<string, string> = {
    search: 'M11 11L14.5 14.5M12.5 7A5.5 5.5 0 1 1 1.5 7a5.5 5.5 0 0 1 11 0Z',
    'star-filled':
      'M8 1.5l1.9 3.9 4.3.6-3.1 3 .7 4.3L8 11.3l-3.8 2 .7-4.3-3.1-3 4.3-.6L8 1.5Z',
    star: 'M8 1.5l1.9 3.9 4.3.6-3.1 3 .7 4.3L8 11.3l-3.8 2 .7-4.3-3.1-3 4.3-.6L8 1.5Z',
    folder: 'M1.5 3.5h4l1.2 1.5h6.3a.5.5 0 0 1 .5.5v7a.5.5 0 0 1-.5.5H1.5a.5.5 0 0 1-.5-.5v-8.5a.5.5 0 0 1 .5-.5Z',
    sftp: 'M2 5h11M10 2l3 3-3 3M14 11H3M6 8l-3 3 3 3',
    cloud: 'M4.5 12.5h7a3 3 0 0 0 .3-6A4.3 4.3 0 0 0 3.5 7.7a2.4 2.4 0 0 0 1 4.8Z',
    database: 'M2.5 4c0-1.1 2.5-2 5.5-2s5.5.9 5.5 2-2.5 2-5.5 2-5.5-.9-5.5-2Zm0 0v4c0 1.1 2.5 2 5.5 2s5.5-.9 5.5-2V4m-11 4v4c0 1.1 2.5 2 5.5 2s5.5-.9 5.5-2V8',
    globe: 'M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13Zm0 0c2 1.8 2.8 4 2.8 6.5S10 12.7 8 14.5C6 12.7 5.2 10.5 5.2 8S6 3.3 8 1.5ZM1.5 8h13',
    code: 'M5.5 3.5 1.5 8l4 4.5M10.5 3.5l4 4.5-4 4.5M9 2.5 7 13.5',
    home: 'M1.5 7.5 8 2l6.5 5.5M3.5 6v7.5h9V6M6.5 13.5V9h3v4.5',
    shield: 'M8 1.5 13 3v4.2c0 3.3-2 5.8-5 7.3-3-1.5-5-4-5-7.3V3l5-1.5Z',
    archive: 'M2 2h12v3H2V2Zm1 3h10v9H3V5Zm3 3h4',
    file: 'M3 1.5h6l4 4v9H3v-13ZM9 1.5v4h4',
    upload: 'M8 11V2.5M4.5 6 8 2.5 11.5 6M2.5 10.5v3h11v-3',
    download: 'M8 2.5V11M4.5 7.5 8 11l3.5-3.5M2.5 10.5v3h11v-3',
    up: 'M8 13V4M4.5 7.5 8 4l3.5 3.5',
    back: 'M13 8H3M6.5 4.5 3 8l3.5 3.5',
    tag: 'M2 2h5.5L14 8.5 7.5 15 1 8.5V3a1 1 0 0 1 1-1Zm3 4.2a1.2 1.2 0 1 0 0-2.4 1.2 1.2 0 0 0 0 2.4Z',
    plus: 'M8 2.5v11M2.5 8h11',
    x: 'M3 3l10 10M13 3L3 13',
    'chevron-right': 'M6 3l5 5-5 5',
    'chevron-up': 'M3 10l5-5 5 5',
    'chevron-down': 'M3 6l5 5 5-5',
    refresh: 'M13.5 8a5.5 5.5 0 1 1-1.6-3.9M13.5 2.5V6H10',
    link: 'M6.5 9.5l3-3M6 4.5 7.6 3a3 3 0 0 1 4.2 4.2L10.3 8.7M10 11.5 8.4 13A3 3 0 0 1 4.2 8.8L5.7 7.3',
    unlink: 'M3 3l10 10M6 4.5 7.6 3a3 3 0 0 1 4.2 4.2L10.3 8.7M10 11.5 8.4 13A3 3 0 0 1 4.2 8.8L5.7 7.3',
    copy: 'M5.5 5.5h7v7h-7v-7ZM3.5 10.5h-1v-7h7v1',
    terminal: 'M2.5 3h11a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1ZM4 6.5l2.5 2-2.5 2M8 10.5h3.5',
    settings:
      'M8 5.8A2.2 2.2 0 1 0 8 10.2 2.2 2.2 0 0 0 8 5.8ZM8 1.5v1.4M8 13.1v1.4M14.5 8h-1.4M2.9 8H1.5M12.5 3.5l-1 1M4.5 11.5l-1 1M12.5 12.5l-1-1M4.5 4.5l-1-1',
    'more-horizontal': 'M3 8h.01M8 8h.01M13 8h.01',
    grid: 'M2.5 2.5h4v4h-4zM9.5 2.5h4v4h-4zM2.5 9.5h4v4h-4zM9.5 9.5h4v4h-4z',
    layers: 'M8 2 14 5 8 8 2 5l6-3ZM2 8l6 3 6-3M2 11l6 3 6-3',
    server: 'M2 3h12v4H2V3ZM2 9h12v4H2V9ZM4.5 5h.01M4.5 11h.01',
    key: 'M6 10a3.5 3.5 0 1 1 3.4-4.3l4.6-.2v2l-1.5 1.5-1.3-1.2-1.2 1.2-1.1-1a3.5 3.5 0 0 1-2.9 2Z',
    dot: 'M8 8m-3 0a3 3 0 1 0 6 0a3 3 0 1 0-6 0',
    warning: 'M8 1.5 15 14H1L8 1.5ZM8 6.5v3.2M8 11.7h.01',
    bell: 'M3.5 11.5h9l-1.2-1.7V7a3.3 3.3 0 0 0-6.6 0v2.8l-1.2 1.7ZM6.5 13a1.7 1.7 0 0 0 3 0',
    check: 'M3 8.5l3.3 3.3L13 5',
    inspector: 'M2 2.5h12v11H2v-11ZM10 2.5v11',
    sidebar: 'M2 2.5h12v11H2v-11ZM6 2.5v11',
    broadcast: 'M8 8m-1.5 0a1.5 1.5 0 1 0 3 0a1.5 1.5 0 1 0-3 0M4.8 4.8a4.5 4.5 0 0 0 0 6.4M11.2 4.8a4.5 4.5 0 0 1 0 6.4M2.7 2.7a7.5 7.5 0 0 0 0 10.6M13.3 2.7a7.5 7.5 0 0 1 0 10.6',
    scissors: 'M4.5 4.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3ZM4.5 8.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3ZM5.8 6.5 13 11M5.8 9.5 13 5',
    'split-h': 'M2 2.5h12v11H2zM8 2.5v11',
    'split-v': 'M2 2.5h12v11H2zM2 8h12',
    folderTree: 'M2 3.5h4l1 1.2h7v7.8H2zM5 8.5h6M5 10.5h4',
    play: 'M4 3l9 5-9 5z'
  }

  // Glyphs taken verbatim from Lucide. They are drawn on a 24×24 grid with a
  // heavier stroke than the hand-drawn set above, and several need more than
  // one subpath, so they keep their own geometry instead of being rescaled by
  // hand into the 16×16 grid.
  const lucidePaths: Record<string, string[]> = {
    container: [
      'M22 7.7c0-.6-.4-1.2-.8-1.5l-6.3-3.9a1.72 1.72 0 0 0-1.7 0l-10.3 6c-.5.3-.9.9-.9 1.5v6.6c0 .5.4 1.2.8 1.5l6.3 3.9a1.72 1.72 0 0 0 1.7 0l10.3-6c.5-.3.9-1 .9-1.5Z',
      'M10 21.9V14L2.1 9.1',
      'M10 14l11.9-6.9',
      'M14 19.8v-8.1',
      'M18 17.5V9.4'
    ]
  }

  $: lucide = lucidePaths[name]
  $: shapes = lucide ?? [paths[name] ?? '']
</script>

<svg
  width={size}
  height={size}
  viewBox={lucide ? '0 0 24 24' : '0 0 16 16'}
  fill="none"
  stroke="currentColor"
  stroke-width={lucide ? 2 : 1.3}
  stroke-linecap="round"
  stroke-linejoin="round"
  class="icon"
  class:filled={name === 'star-filled'}
  aria-hidden="true"
>
  {#each shapes as d}
    <path {d} />
  {/each}
</svg>

<style>
  .icon {
    display: inline-block;
    vertical-align: middle;
    flex-shrink: 0;
  }
  .icon.filled {
    fill: currentColor;
  }
</style>
