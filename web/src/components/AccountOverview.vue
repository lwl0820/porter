<script setup lang="ts">
import { computed } from "vue";
import { t } from "../i18n";
import { accountingLabels, resetLabels, type Account } from "../types";
import { formatBytes, formatUTC, usagePercent } from "../traffic";
import Icon from "./Icon.vue";
const props = withDefaults(defineProps<{ account: Account; compact?: boolean }>(), { compact: false });
const percent = computed(() => props.account.usage ? usagePercent(props.account.usage.used_bytes, props.account.usage.limit_bytes) : 0);
</script>
<template>
  <section class="account-overview" :class="{ compact }" :aria-label="t('账户概览')">
    <header v-if="!compact" class="page-heading">
      <div>
        <h1>{{ t('账户概览') }}</h1>
        <p>{{ account.user.username }}</p>
      </div>
    </header>
    <p v-if="account.usage?.exceeded" class="notice error" role="status">{{ t('套餐流量已用尽，将在下次重置后自动恢复。') }}</p>
    <div class="account-layout">
      <article class="account-plan account-panel">
        <h2>
          <Icon name="layers" :size="20" />
          <span>{{ compact ? (account.plan?.name ?? t('未分配套餐')) : t('套餐信息') }}</span>
        </h2>
        <strong v-if="!compact" class="plan-name">{{ account.plan?.name ?? t('未分配套餐') }}</strong>
        <p>
          {{ t('分配端口') }} <strong>{{ account.user.port_start }}–{{ account.user.port_end }}</strong>
        </p>
        <span v-if="account.plan" class="field-hint">{{ account.plan.port_count }} {{ t('个端口') }}</span>
      </article>
      <article v-if="account.usage" class="account-usage account-panel">
        <h2 v-if="!compact">
          <Icon name="activity" :size="20" />{{ t('流量用量') }}
        </h2>
        <div class="usage-layout">
          <div v-if="!compact" class="usage-ring" :style="{ '--usage': `${percent}%` }">
            <strong>{{ percent }}%</strong>
          </div>
          <div class="usage-summary">
            <span class="field-hint">{{ t('计费用量 / 额度') }}</span>
            <p class="usage-numbers" :title="`${account.usage.used_bytes} / ${account.usage.limit_bytes} B`">
              <strong>{{ formatBytes(account.usage.used_bytes) }}</strong> / {{ formatBytes(account.usage.limit_bytes) }}
            </p>
            <div class="usage-progress">
              <div class="progress-track" role="progressbar" :aria-label="t('计费用量 / 额度')" :aria-valuenow="percent" aria-valuemin="0" aria-valuemax="100">
                <i :style="{ width: `${percent}%` }" />
              </div>
              <span>{{ percent }}%</span>
            </div>
            <dl class="usage-directions">
              <div>
                <dt>{{ t('上行流量') }}</dt>
                <dd :title="`${account.usage.upload_bytes} B`">{{ formatBytes(account.usage.upload_bytes) }}</dd>
              </div>
              <div>
                <dt>{{ t('下行流量') }}</dt>
                <dd :title="`${account.usage.download_bytes} B`">{{ formatBytes(account.usage.download_bytes) }}</dd>
              </div>
            </dl>
          </div>
        </div>
      </article>
      <article v-if="account.plan" class="account-cycle account-panel">
        <h2 v-if="!compact">
          <Icon name="clock" :size="20" />{{ t('计费与重置') }}
        </h2>
        <dl class="resource-grid">
          <div>
            <dt>{{ t('流量计费') }}</dt>
            <dd>{{ t(accountingLabels[account.plan.accounting_mode]) }}</dd>
          </div>
          <div>
            <dt>{{ t('重置方式') }}</dt>
            <dd>{{ t(resetLabels[account.plan.reset_mode]) }}{{ account.plan.reset_day ? ` · ${account.plan.reset_day}` : '' }}</dd>
          </div>
          <div v-if="account.usage">
            <dt>{{ t('下次重置（UTC）') }}</dt>
            <dd>{{ formatUTC(account.usage.cycle_end) }}</dd>
          </div>
        </dl>
        <p v-if="!compact" class="notice subtle">{{ t('上行：客户端到目标；下行：目标到客户端。单向按本周期全部规则的上下行总量取较大值。') }}</p>
      </article>
    </div>
  </section>
</template>
