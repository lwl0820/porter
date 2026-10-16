<script setup lang="ts">
import { nextTick, reactive, ref } from "vue";
import { ApiError, errorMessage } from "../api";
import { protocols, protocolDetails } from "../types";
import type { FieldErrors, Rule, RuleInput } from "../types";
import Modal from "./Modal.vue";
import Icon from "./Icon.vue";

const props = defineProps<{
  rule?: Rule;
  save: (input: RuleInput) => Promise<void>;
}>();
const emit = defineEmits<{ close: []; busy: [value: boolean] }>();
const form = reactive({
  name: props.rule?.name ?? "",
  protocol: props.rule?.protocol ?? "tcp",
  listen_host: props.rule?.listen_host ?? "0.0.0.0",
  listen_port: String(props.rule?.listen_port ?? ""),
  target_host: props.rule?.target_host ?? "",
  target_port: String(props.rule?.target_port ?? ""),
  enabled: props.rule?.enabled ?? true,
});
const fields = ref<FieldErrors>({});
const error = ref("");
const busy = ref(false);
const formElement = ref<HTMLFormElement>();

function ipAddress(host: string): boolean {
  if (host.includes(":")) {
    try {
      return new URL(`http://[${host}]/`).hostname.startsWith("[");
    } catch {
      return false;
    }
  }
  return (
    /^(\d{1,3}\.){3}\d{1,3}$/.test(host) &&
    host
      .split(".")
      .every((part) => Number(part) <= 255 && String(Number(part)) === part)
  );
}

function validate(): boolean {
  const errors: FieldErrors = {};
  if (!form.name.trim() || [...form.name.trim()].length > 80)
    errors.name = "请输入 1–80 个字符的规则名称。";
  if (!ipAddress(form.listen_host.trim()))
    errors.listen_host = "请输入有效的 IPv4 或 IPv6 地址。";
  for (const key of ["listen_port", "target_port"] as const) {
    if (
      !/^\d+$/.test(form[key]) ||
      Number(form[key]) < 1 ||
      Number(form[key]) > 65535
    )
      errors[key] = "请输入 1–65535 的整数端口。";
  }
  const host = form.target_host.trim();
  const domain =
    /^(?=.{1,253}$)(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*\.?$/;
  if (!host || (!ipAddress(host) && !domain.test(host)))
    errors.target_host = "请输入 IP 地址或域名，不含协议、路径或端口。";
  fields.value = errors;
  return Object.keys(errors).length === 0;
}

async function focusError(): Promise<void> {
  await nextTick();
  formElement.value
    ?.querySelector<HTMLElement>('[aria-invalid="true"]')
    ?.focus();
}

async function submit(): Promise<void> {
  if (busy.value) return;
  error.value = "";
  if (!validate()) {
    await focusError();
    return;
  }
  busy.value = true;
  emit("busy", true);
  let saved = false;
  try {
    await props.save({
      name: form.name.trim(),
      protocol: form.protocol,
      listen_host: form.listen_host.trim(),
      listen_port: Number(form.listen_port),
      target_host: form.target_host.trim(),
      target_port: Number(form.target_port),
      enabled: form.enabled,
    });
    saved = true;
  } catch (failure) {
    error.value = errorMessage(failure);
    if (failure instanceof ApiError) fields.value = failure.fields;
  } finally {
    busy.value = false;
    emit("busy", false);
  }
  if (saved) emit("close");
  else await focusError();
}
</script>

<template>
  <Modal
    :label="rule ? '编辑规则' : '新增规则'"
    :busy="busy"
    drawer
    @close="emit('close')"
  >
    <form ref="formElement" class="editor" novalidate @submit.prevent="submit">
      <header class="modal-header">
        <div>
          <span class="eyebrow">转发规则</span>
          <h2>{{ rule ? "编辑规则" : "新增规则" }}</h2>
        </div>
        <button
          type="button"
          class="icon-button"
          aria-label="关闭表单"
          :disabled="busy"
          @click="emit('close')"
        >
          <Icon name="close" />
        </button>
      </header>
      <div class="editor-body">
        <p class="editor-intro">将本机端口的流量转发到指定的目标服务。</p>
        <div v-if="error" role="alert" class="notice error">
          <Icon name="alert" /><span>{{ error }}</span>
        </div>
        <fieldset :disabled="busy">
          <div class="field">
            <label for="rule-name">规则名称</label
            ><input
              id="rule-name"
              v-model="form.name"
              data-autofocus
              autocomplete="off"
              placeholder="例如：应用服务"
              :aria-invalid="Boolean(fields.name)"
              :aria-describedby="fields.name ? 'error-name' : undefined"
            />
            <p v-if="fields.name" id="error-name" class="field-error">
              {{ fields.name }}
            </p>
          </div>
          <div class="field">
            <label id="protocol-label">传输协议</label>
            <div
              class="protocol-choice"
              role="group"
              aria-labelledby="protocol-label"
            >
              <label
                v-for="value in protocols"
                :key="value"
                :class="{ selected: form.protocol === value }"
              >
                <input
                  v-model="form.protocol"
                  type="radio"
                  name="protocol"
                  :value="value"
                  :aria-label="protocolDetails[value].label"
                />
                <strong>{{ protocolDetails[value].label }}</strong>
                <span>{{ protocolDetails[value].description }}</span>
                <Icon v-if="form.protocol === value" name="check" :size="17" />
              </label>
            </div>
          </div>
          <div class="form-section-label">
            <span class="step-dot">1</span> 本机监听
          </div>
          <div class="address-fields">
            <div class="field">
              <label for="listen-host">监听 IP</label
              ><input
                id="listen-host"
                v-model="form.listen_host"
                spellcheck="false"
                :aria-invalid="Boolean(fields.listen_host)"
                :aria-describedby="
                  fields.listen_host
                    ? 'listen-host-hint error-listen_host'
                    : 'listen-host-hint'
                "
              />
              <p id="listen-host-hint" class="field-hint">
                0.0.0.0 监听所有 IPv4 地址；IPv6 使用 ::。
              </p>
              <p
                v-if="fields.listen_host"
                id="error-listen_host"
                class="field-error"
              >
                {{ fields.listen_host }}
              </p>
            </div>
            <div class="field">
              <label for="listen-port">监听端口</label
              ><input
                id="listen-port"
                v-model="form.listen_port"
                inputmode="numeric"
                placeholder="8081"
                :aria-invalid="Boolean(fields.listen_port)"
                :aria-describedby="
                  fields.listen_port ? 'error-listen_port' : undefined
                "
              />
              <p
                v-if="fields.listen_port"
                id="error-listen_port"
                class="field-error"
              >
                {{ fields.listen_port }}
              </p>
            </div>
          </div>
          <div class="flow-connector">
            <Icon name="arrow" :size="17" /><span>转发到</span>
          </div>
          <div class="form-section-label">
            <span class="step-dot">2</span> 目标服务
          </div>
          <div class="address-fields">
            <div class="field">
              <label for="target-host">目标主机</label
              ><input
                id="target-host"
                v-model="form.target_host"
                autocomplete="off"
                spellcheck="false"
                placeholder="10.0.0.2 或 example.com"
                :aria-invalid="Boolean(fields.target_host)"
                :aria-describedby="
                  fields.target_host ? 'error-target_host' : undefined
                "
              />
              <p
                v-if="fields.target_host"
                id="error-target_host"
                class="field-error"
              >
                {{ fields.target_host }}
              </p>
            </div>
            <div class="field">
              <label for="target-port">目标端口</label
              ><input
                id="target-port"
                v-model="form.target_port"
                inputmode="numeric"
                placeholder="443"
                :aria-invalid="Boolean(fields.target_port)"
                :aria-describedby="
                  fields.target_port ? 'error-target_port' : undefined
                "
              />
              <p
                v-if="fields.target_port"
                id="error-target_port"
                class="field-error"
              >
                {{ fields.target_port }}
              </p>
            </div>
          </div>
          <label class="enable-field"
            ><span
              ><strong>启用规则</strong
              ><span>保存后立即开始监听和转发</span></span
            ><input v-model="form.enabled" type="checkbox" class="toggle-input"
          /></label>
        </fieldset>
        <div v-if="rule" class="notice subtle">
          <Icon name="alert" :size="18" /><span
            >修改转发参数会断开该规则的现有会话。仅修改名称不会影响转发。</span
          >
        </div>
      </div>
      <footer class="modal-footer">
        <button
          type="button"
          class="button secondary"
          :disabled="busy"
          @click="emit('close')"
        >
          取消
</button
        ><button type="submit" class="button primary" :disabled="busy">
          <span v-if="busy" class="spinner small" /><Icon
            v-else
            name="check"
            :size="18"
          />{{ busy ? "正在保存…" : "保存规则" }}
        </button>
      </footer>
    </form>
  </Modal>
</template>
