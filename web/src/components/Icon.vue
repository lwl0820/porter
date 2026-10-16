<script setup lang="ts">
defineProps<{ name: string; size?: number }>();
const paths: Record<string, string> = {
  arrows: "M4 7h15m-4-4 4 4-4 4M20 17H5m4-4-4 4 4 4",
  plus: "M12 5v14M5 12h14",
  search: "m21 21-4.4-4.4M19 10.5a8.5 8.5 0 1 1-17 0 8.5 8.5 0 0 1 17 0",
  refresh:
    "M20 7v5h-5M4 17v-5h5M6.1 7a7 7 0 0 1 11.5-2L20 8M4 16l2.4 3A7 7 0 0 0 18 17",
  edit: "m15 5 4 4M4 20l5-1L20 8a2.8 2.8 0 0 0-4-4L5 15l-1 5Z",
  trash: "M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7",
  logout: "M9 4H4v16h5M13 7l5 5-5 5M8 12h13",
  user: "M16 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0M4 21v-2a8 8 0 0 1 16 0v2",
  close: "m6 6 12 12M18 6 6 18",
  arrow: "M4 12h16m-5-5 5 5-5 5",
  shield: "M12 3 4 6v6c0 5 8 9 8 9s8-4 8-9V6l-8-3Zm-4 9 3 3 5-6",
  alert:
    "M12 8v5m0 4v.1M10.3 4.7 2.7 18a2 2 0 0 0 1.7 3h15.2a2 2 0 0 0 1.7-3L13.7 4.7a2 2 0 0 0-3.4 0Z",
  layers: "m12 3 10 5-10 5L2 8l10-5Zm-10 9 10 5 10-5M2 17l10 5 10-5",
  check: "m5 12 4 4L19 6",
  pause: "M8 5v14M16 5v14",
  activity: "M2 12h4l3-8 6 16 3-8h4",
  network: "M9 3h6v6H9V3ZM3 16h6v6H3v-6Zm12 0h6v6h-6v-6ZM12 9v4M6 16v-3h12v3",
  lock: "M6 10h12v11H6V10Zm3 0V6a3 3 0 0 1 6 0v4m-3 5v2",
};
</script>

<template>
  <svg
    :width="size ?? 20"
    :height="size ?? 20"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.7"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="paths[name] ?? paths.arrows" />
  </svg>
</template>
