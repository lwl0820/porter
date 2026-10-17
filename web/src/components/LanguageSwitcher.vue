<script setup lang="ts">
import { language, setLanguage, t } from "../i18n";

function change(event: Event): void {
  const value = (event.target as HTMLSelectElement).value;
  if (value === "zh-CN" || value === "en") setLanguage(value);
}
</script>

<template>
  <label class="language-switcher">
    <span class="sr-only">{{ t("语言") }}</span>
    <select :value="language" :aria-label="t('语言')" @change="change">
      <option value="zh-CN" lang="zh-CN">中文</option>
      <option value="en" lang="en">English</option>
    </select>
  </label>
</template>
