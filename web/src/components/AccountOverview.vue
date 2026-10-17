<script setup lang="ts">
import { t } from "../i18n";
import { accountingLabels, resetLabels, type Account } from "../types";
import { formatBytes } from "../traffic";
defineProps<{ account: Account }>();
function utc(value: string): string { return new Date(value).toISOString().replace("T", " ").replace(/\.\d{3}Z$/, " UTC"); }
</script>
<template>
  <section class="account-overview" :aria-label="t('账户概览')">
    <header class="panel-heading"><h2>{{ account.user.username }} · {{ account.plan?.name }}</h2><strong>{{ account.user.port_start }}–{{ account.user.port_end }}</strong></header>
    <div v-if="account.usage && account.plan" class="account-details">
      <p v-if="account.usage.exceeded" class="notice error" role="status">{{ t('套餐流量已用尽，将在下次重置后自动恢复。') }}</p>
      <dl class="resource-grid">
        <div><dt>{{ t('计费用量 / 额度') }}</dt><dd :title="`${account.usage.used_bytes} / ${account.usage.limit_bytes} B`">{{ formatBytes(account.usage.used_bytes) }} / {{ formatBytes(account.usage.limit_bytes) }}</dd></div>
        <div><dt>{{ t('上行流量') }}</dt><dd :title="`${account.usage.upload_bytes} B`">{{ formatBytes(account.usage.upload_bytes) }}</dd></div>
        <div><dt>{{ t('下行流量') }}</dt><dd :title="`${account.usage.download_bytes} B`">{{ formatBytes(account.usage.download_bytes) }}</dd></div>
        <div><dt>{{ t('流量计费') }}</dt><dd>{{ t(accountingLabels[account.plan.accounting_mode]) }}</dd></div>
        <div><dt>{{ t('重置方式') }}</dt><dd>{{ t(resetLabels[account.plan.reset_mode]) }}{{ account.plan.reset_day ? ` · ${account.plan.reset_day}` : '' }}</dd></div>
        <div><dt>{{ t('下次重置（UTC）') }}</dt><dd>{{ utc(account.usage.cycle_end) }}</dd></div>
      </dl>
      <p class="field-hint">{{ t('上行：客户端到目标；下行：目标到客户端。单向按本周期全部规则的上下行总量取较大值。') }}</p>
    </div>
  </section>
</template>
