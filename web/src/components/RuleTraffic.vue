<script setup lang="ts">
import { t } from "../i18n";
import type { Rule } from "../types";
import { formatBytes, formatSpeed } from "../traffic";
defineProps<{ rule: Rule; busy: boolean }>();
const emit = defineEmits<{ reset: [rule: Rule] }>();
</script>

<template>
  <div class="rule-traffic">
    <div class="traffic-heading">
      <span>{{ rule.traffic_enabled ? t("当前速度 · 累计流量") : t("统计已关闭") }}</span>
      <button class="text-button" :aria-label="t('重置 {name} 的流量', { name: rule.name })" :disabled="busy" @click="emit('reset', rule)">{{ t("重置流量") }}</button>
    </div>
    <div class="traffic-direction" :title="t('客户端到目标')">
      <span>{{ t("↑ 上行") }}</span><span>{{ formatSpeed(rule.traffic.upload_bytes_per_second) }}</span><strong :title="`${rule.traffic.upload_bytes} B`">{{ formatBytes(rule.traffic.upload_bytes) }}</strong>
    </div>
    <div class="traffic-direction" :title="t('目标到客户端')">
      <span>{{ t("↓ 下行") }}</span><span>{{ formatSpeed(rule.traffic.download_bytes_per_second) }}</span><strong :title="`${rule.traffic.download_bytes} B`">{{ formatBytes(rule.traffic.download_bytes) }}</strong>
    </div>
  </div>
</template>
