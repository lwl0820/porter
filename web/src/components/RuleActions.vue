<script setup lang="ts">
import type { Rule } from "../types";
import Icon from "./Icon.vue";

defineProps<{ rule: Rule; busy: boolean }>();
defineEmits<{
  edit: [rule: Rule];
  remove: [rule: Rule];
  toggle: [rule: Rule];
  retry: [rule: Rule];
}>();
</script>

<template>
  <div class="rule-actions">
    <button
      v-if="rule.status === 'error'"
      type="button"
      class="icon-button retry-button"
      :disabled="busy"
      :aria-label="`重试 ${rule.name}`"
      title="重试"
      @click="$emit('retry', rule)"
    >
      <Icon name="refresh" :size="17" />
    </button>
    <button
      type="button"
      class="switch"
      role="switch"
      :aria-checked="rule.enabled"
      :aria-label="`${rule.enabled ? '停用' : '启用'} ${rule.name}`"
      :disabled="busy"
      :title="rule.enabled ? '停用规则，会断开现有会话' : '启用规则'"
      @click="$emit('toggle', rule)"
    >
      <span />
    </button>
    <span class="action-divider" />
    <button
      type="button"
      class="icon-button"
      :disabled="busy"
      :aria-label="`编辑 ${rule.name}`"
      title="编辑"
      @click="$emit('edit', rule)"
    >
      <Icon name="edit" :size="17" />
    </button>
    <button
      type="button"
      class="icon-button danger-hover"
      :disabled="busy"
      :aria-label="`删除 ${rule.name}`"
      title="删除"
      @click="$emit('remove', rule)"
    >
      <Icon name="trash" :size="17" />
    </button>
  </div>
</template>
