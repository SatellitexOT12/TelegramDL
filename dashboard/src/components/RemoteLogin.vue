<script setup>
import { computed, onUnmounted, ref } from 'vue'
import {
  KeyRound,
  Loader2,
  AlertCircle,
  CheckCircle2,
  Eye,
  EyeOff,
  Send,
  ShieldCheck
} from '../icons'
import { useI18n } from '../i18n'

const { t, has } = useI18n()

defineProps({
  error: { type: String, default: '' }
})
const emit = defineEmits(['submit'])

const tokenInput = ref('')
const showToken = ref(false)
const submitting = ref(false)

// Envío del token a los Mensajes guardados de Telegram, para no tener que ir
// al ordenador a copiarlo. El endpoint no lleva token (es justo lo que falta
// aquí) y el servidor solo admite un envío por minuto.
const sending = ref(false)
const sendState = ref('')
const sendMsgKey = ref('')
const sendMessage = computed(() => {
  if (!sendMsgKey.value) return ''
  return has(sendMsgKey.value) ? t(sendMsgKey.value) : sendMsgKey.value
})
const cooldown = ref(0)
let cooldownTimer = null

const startCooldown = (seconds) => {
  clearInterval(cooldownTimer)
  cooldown.value = Math.max(0, Math.round(Number(seconds) || 0))
  if (!cooldown.value) return
  cooldownTimer = setInterval(() => {
    cooldown.value -= 1
    if (cooldown.value <= 0) clearInterval(cooldownTimer)
  }, 1000)
}

onUnmounted(() => clearInterval(cooldownTimer))

const sendTokenToTelegram = async () => {
  if (sending.value || cooldown.value > 0) return
  sending.value = true
  sendState.value = ''
  sendMsgKey.value = ''
  try {
    // La cabecera propia no es un secreto: obliga al navegador a hacer el
    // preflight de CORS, que es lo que impide que otra página web dispare este
    // endpoint (responde sin token) desde el navegador del usuario.
    const response = await fetch('/api/auth/token/send', {
      method: 'POST',
      headers: { 'X-TGDL-Request': '1' }
    })
    const data = await response.json().catch(() => ({}))
    if (!response.ok) {
      sendState.value = 'error'
      sendMsgKey.value = data.detail || data.error || 'remote.sendFail'
      startCooldown(data.retry_after)
      return
    }
    sendState.value = 'ok'
    sendMsgKey.value = 'remote.sentOk'
    startCooldown(data.retry_after || 60)
  } catch {
    sendState.value = 'error'
    sendMsgKey.value = 'remote.unreachable'
  } finally {
    sending.value = false
  }
}

const handleSubmit = () => {
  const value = tokenInput.value.trim()
  if (!value) return
  submitting.value = true
  try {
    emit('submit', value)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="auth-overlay">
    <div class="auth-card">
      <div class="auth-header">
        <div class="auth-logo">
          <ShieldCheck :size="22" />
        </div>
        <div>
          <h2>{{ t('remote.title') }}</h2>
          <p>{{ t('remote.subtitle') }}</p>
        </div>
      </div>

      <div v-if="error" class="auth-alert danger">
        <AlertCircle :size="18" />
        <span>{{ error }}</span>
      </div>

      <div class="info-banner">
        <ShieldCheck :size="16" />
        <span>
          {{ t('remote.bannerA') }} <b>{{ t('remote.bannerB') }}</b
          >.
          {{ t('remote.bannerC') }}
        </span>
      </div>

      <div class="form-group">
        <label>{{ t('remote.tokenLabel') }}</label>
        <div class="token-input-row">
          <input
            v-model="tokenInput"
            :type="showToken ? 'text' : 'password'"
            :placeholder="t('remote.tokenPlaceholder')"
            autocomplete="off"
            spellcheck="false"
            @keyup.enter="handleSubmit"
          />
          <button
            type="button"
            :aria-label="
              showToken ? t('remote.hideToken') : t('remote.showToken')
            "
            class="ghost-icon-btn"
            @click="showToken = !showToken"
          >
            <EyeOff v-if="showToken" :size="16" />
            <Eye v-else :size="16" />
          </button>
        </div>
      </div>

      <button
        class="auth-button primary"
        :disabled="submitting || !tokenInput.trim()"
        @click="handleSubmit"
      >
        <Loader2 v-if="submitting" class="spin" :size="18" />
        <template v-else>
          <span>{{ t('remote.enter') }}</span>
          <KeyRound :size="18" />
        </template>
      </button>

      <div class="send-divider">
        <span>{{ t('remote.noToken') }}</span>
      </div>

      <button
        class="auth-button ghost"
        :disabled="sending || cooldown > 0"
        @click="sendTokenToTelegram"
      >
        <Loader2 v-if="sending" class="spin" :size="17" />
        <Send v-else :size="17" />
        <span v-if="sending">{{ t('remote.sending') }}</span>
        <span v-else-if="cooldown > 0">
          {{ t('remote.retryIn', { n: cooldown }) }}
        </span>
        <span v-else>{{ t('remote.sendToTelegram') }}</span>
      </button>
      <p class="send-hint">
        {{ t('remote.hintPre') }} <b>{{ t('remote.hintBold') }}</b
        >{{ t('remote.hintPost') }}
      </p>

      <div
        v-if="sendMessage"
        class="auth-alert send-result"
        :class="sendState === 'ok' ? 'success' : 'danger'"
      >
        <CheckCircle2 v-if="sendState === 'ok'" :size="18" />
        <AlertCircle v-else :size="18" />
        <span>{{ sendMessage }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.auth-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: color-mix(in srgb, var(--user-bg-base), transparent 8%);
  backdrop-filter: blur(12px);
  display: grid;
  place-items: center;
  padding: 20px;
}

.auth-card {
  width: min(440px, 100%);
  background: var(--user-surface);
  border: 1px solid var(--user-border);
  border-radius: 20px;
  padding: 30px;
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.6);
}

.auth-header {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 20px;
}

.auth-logo {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(135deg, var(--user-primary), var(--user-accent));
  color: #fff;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}

.auth-header h2 {
  font-family: 'Space Grotesk', sans-serif;
  font-size: 19px;
  font-weight: 700;
  color: #f1f7ff;
  margin: 0;
}

.auth-header p {
  font-size: 13px;
  color: #7d96b0;
  margin: 2px 0 0;
}

.auth-alert.success {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(74, 222, 128, 0.12);
  border: 1px solid rgba(74, 222, 128, 0.35);
  color: #86efac;
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 13px;
  line-height: 1.45;
}

.auth-alert.send-result {
  margin-top: 14px;
  margin-bottom: 0;
  align-items: flex-start;
}

.auth-alert.danger {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(220, 60, 60, 0.12);
  border: 1px solid rgba(220, 60, 60, 0.35);
  color: #ff8f8f;
  border-radius: 10px;
  padding: 10px 12px;
  font-size: 13px;
  margin-bottom: 16px;
}

.info-banner {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  background: var(--user-surface-light);
  border: 1px solid var(--user-border);
  border-radius: 12px;
  padding: 12px 14px;
  font-size: 12.5px;
  color: var(--user-text-dim);
  margin-bottom: 18px;
  line-height: 1.5;
}

.form-group {
  margin-bottom: 20px;
}

.form-group label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--user-text-dim);
  margin-bottom: 8px;
}

.token-input-row {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--user-bg-base);
  border: 1px solid var(--user-border);
  border-radius: 10px;
  padding: 0 6px 0 12px;
}

.token-input-row input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: #f1f7ff;
  font-size: 14px;
  padding: 11px 0;
  font-family: 'JetBrains Mono', monospace;
}

.ghost-icon-btn {
  background: transparent;
  border: none;
  color: var(--user-text-dim);
  cursor: pointer;
  padding: 8px;
  border-radius: 8px;
  display: grid;
  place-items: center;
}

.ghost-icon-btn:hover {
  background: var(--user-border);
  color: #fff;
}

.auth-button.primary {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: var(--user-gradient, var(--user-primary));
  color: #fff;
  border: none;
  border-radius: 12px;
  padding: 13px;
  font-size: 14px;
  font-weight: 700;
  cursor: pointer;
  transition:
    opacity 0.2s,
    transform 0.2s;
}

.auth-button.primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.auth-button.primary:not(:disabled):hover {
  transform: translateY(-1px);
}

.send-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 20px 0 14px;
  color: var(--user-text-dim);
  font-size: 11.5px;
}

.send-divider::before,
.send-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--user-border);
}

.auth-button.ghost {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: var(--user-surface-light);
  color: #dbe7f5;
  border: 1px solid var(--user-border);
  border-radius: 12px;
  padding: 12px;
  font-size: 13.5px;
  font-weight: 600;
  cursor: pointer;
  transition:
    border-color 0.2s,
    color 0.2s,
    opacity 0.2s;
}

.auth-button.ghost:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.auth-button.ghost:not(:disabled):hover {
  border-color: var(--user-primary);
  color: #fff;
}

.send-hint {
  margin: 9px 0 0;
  font-size: 11.5px;
  line-height: 1.45;
  color: var(--user-text-dim);
  text-align: center;
}

.spin {
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
