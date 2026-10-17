<script setup lang="ts">
import type { Rule } from "../types";
import { formatBytes, formatSpeed } from "../traffic";
defineProps<{ rule: Rule; busy: boolean }>();
const emit = defineEmits<{ reset: [rule: Rule] }>();
</script>

<template>
  <div class="rule-traffic">
    <div class="traffic-heading">
      <span>{{ rule.traffic_enabled ? "累计流量 · 当前速度" : "统计已关闭" }}</span>
      <button class="text-button" :aria-label="`重置 ${rule.name} 的流量`" :disabled="busy" @click="emit('reset', rule)">重置流量</button>
    </div>
    <div class="traffic-direction" title="客户端到目标">
      <span>↑ 上行</span><strong :title="`${rule.traffic.upload_bytes} B`">{{ formatBytes(rule.traffic.upload_bytes) }}</strong><span>{{ formatSpeed(rule.traffic.upload_bytes_per_second) }}</span>
    </div>
    <div class="traffic-direction" title="目标到客户端">
      <span>↓ 下行</span><strong :title="`${rule.traffic.download_bytes} B`">{{ formatBytes(rule.traffic.download_bytes) }}</strong><span>{{ formatSpeed(rule.traffic.download_bytes_per_second) }}</span>
    </div>
  </div>
</template>
