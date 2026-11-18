<script setup lang="ts">
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 20 })
const paths: Record<string, string> = {
  compare: 'M3 3h7v18H3z M14 3h7v18h-7z M6 7h1 M6 11h1 M6 15h1 M17 7h1 M17 11h1 M17 15h1',
  menu: 'M4 6h16 M4 12h16 M4 18h16',
  sun: 'M12 3v2 M12 19v2 M3 12h2 M19 12h2 M5.64 5.64l1.42 1.42 M16.94 16.94l1.42 1.42 M5.64 18.36l1.42-1.42 M16.94 7.06l1.42-1.42',
  moon: 'M20.5 13A8.5 8.5 0 0 1 11 3.5 8.5 8.5 0 1 0 20.5 13',
  power: 'M12 2v10 M7 4.5a9 9 0 1 0 10 0',
  book: 'M3 3h6l3 2 3-2h6v17h-6l-3 2-3-2H3z M12 5v17 M6 7h3 M6 11h3 M15 7h3 M15 11h3',
  panel: 'M3 3h18v18H3z M9 3v18 m7-13-4 4 4 4',
  grid: 'M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z',
  cpu: 'M8 8h8v8H8z M5 5h14v14H5z M9 2v3 M15 2v3 M9 19v3 M15 19v3 M2 9h3 M2 15h3 M19 9h3 M19 15h3',
  memory: 'M3 6h18v12H3z M7 10v4 M11 10v4 M15 10v4 M6 18v3 M10 18v3 M14 18v3 M18 18v3',
  disk: 'M5 3h14l3 12v6H2v-6z M2 15h20 M6 18h.01 M10 18h.01',
  network: 'M8 3v12 M4 11l4 4 4-4 M16 21V9 M12 13l4-4 4 4',
  bell: 'M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9 M10 21h4',
  sliders: 'M4 3v7 M4 14v7 M12 3v12 M12 19v2 M20 3v2 M20 9v12 M1 10h6 M9 15h6 M17 5h6',
  info: 'M12 17v-5 M12 8h.01 M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
  server: 'M3 3h18v7H3z M3 14h18v7H3z M7 6h.01 M7 17h.01 M11 6h6 M11 17h6',
  arrow: 'M7 17 17 7 M7 7h10v10',
  'chevron-left': 'm15 5-7 7 7 7',
  chevron: 'm9 5 7 7-7 7',
  download: 'M12 3v12 M7 10l5 5 5-5 M4 16v5h16v-5',
  pause: 'M8 4v16 M16 4v16',
  play: 'm7 3 14 9-14 9z',
  check: 'm5 12 4 4L19 6',
  refresh: 'M20 7v5h-5 M4 17v-5h5 M5 8a8 8 0 0 1 13-3l2 3 M4 16l2 3a8 8 0 0 0 13-3',
  terminal: 'm4 6 6 6-6 6 M13 18h7',
  close: 'm6 6 12 12 M6 18 18 6',
  clock: 'M12 6v6l4 2 M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
  help: 'M9 9a3 3 0 1 1 5 2c-2 1-2 2-2 3 M12 18h.01 M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0'
}
</script>
<template><svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle v-if="name === 'sun'" cx="12" cy="12" r="4" /><path :d="paths[name] || paths.info" /></svg></template>
