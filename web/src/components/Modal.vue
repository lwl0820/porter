<script setup lang="ts">
import LanguageSwitcher from "./LanguageSwitcher.vue";
import { nextTick, onMounted, onUnmounted, ref } from "vue";

defineProps<{ label: string; busy?: boolean }>();
const emit = defineEmits<{ close: [] }>();
const dialog = ref<HTMLDialogElement>();
let previousFocus: HTMLElement | null = null;

onMounted(async () => {
  previousFocus =
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
  dialog.value?.showModal();
  await nextTick();
  dialog.value?.querySelector<HTMLElement>("[data-autofocus]")?.focus();
});
onUnmounted(() => {
  previousFocus?.focus();
});
</script>

<template>
  <dialog
    ref="dialog"
    :aria-label="label"
    class="modal"
    @cancel.prevent="!busy && emit('close')"
    @click="$event.target === dialog && !busy && emit('close')"
  >
    <div class="modal-language"><LanguageSwitcher /></div>
    <slot />
  </dialog>
</template>
