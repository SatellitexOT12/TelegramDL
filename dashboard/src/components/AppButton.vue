<script setup>
import { computed, useAttrs } from 'vue'

const props = defineProps({
  variant: {
    type: String,
    default: 'primary',
    validator: (v) =>
      ['primary', 'secondary', 'text', 'icon', 'pager'].includes(v)
  },
  size: {
    type: String,
    default: 'md',
    validator: (v) => ['sm', 'md', 'lg'].includes(v)
  },
  tone: {
    type: String,
    default: 'default',
    validator: (v) =>
      ['default', 'primary', 'accent', 'danger', 'warning', 'success'].includes(
        v
      )
  },
  active: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  title: { type: String, default: undefined }
})

const attrs = useAttrs()

const classes = computed(() => [
  'btn',
  `btn--${props.variant}`,
  `btn--size-${props.size}`,
  ...(props.tone !== 'default' ? [`btn--tone-${props.tone}`] : []),
  ...(props.variant === 'pager' && props.active ? ['btn--active'] : []),
  ...(props.loading ? ['btn--loading'] : [])
])

// La vista marca la página actual con aria-current="page" en el paginador.
const ariaCurrent = computed(
  () =>
    attrs['aria-current'] ??
    (props.variant === 'pager' && props.active ? 'page' : undefined)
)
</script>

<template>
  <button
    :class="classes"
    :aria-busy="loading || undefined"
    :aria-current="ariaCurrent"
    :title="title"
  >
    <slot />
  </button>
</template>

<style scoped>
.btn {
  font: inherit;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  cursor: pointer;
  transition:
    background 0.2s ease,
    border-color 0.2s ease,
    transform 0.2s ease,
    box-shadow 0.2s ease,
    color 0.2s ease;
}

.btn:focus-visible {
  outline: 2px solid var(--user-accent);
  outline-offset: 2px;
}

/* --- primary: llamada principal (hero "Iniciar descarga") ---------------- */
.btn--primary {
  border: 0;
  border-radius: 10px;
  background: var(--user-gradient);
  color: var(--user-bg-base);
  font-size: 13px;
  font-weight: 700;
  padding: 12px 18px;
  gap: 10px;
}

.btn--primary:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 9px 24px var(--user-glow);
}

.btn--primary:active:not(:disabled) {
  transform: translateY(0);
}

.btn--primary:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

/* --- secondary: acción de cabecera ("Pausar todo", "Cancelar todo") ------- */
.btn--secondary {
  background: var(--user-surface-light);
  border: 1px solid var(--user-border);
  color: var(--user-text-dim);
  padding: 6px 10px;
  border-radius: 8px;
  font-size: 11px;
  font-weight: 600;
}

.btn--secondary:hover:not(:disabled) {
  background: var(--user-icon-bg);
  color: var(--user-accent);
  border-color: var(--user-primary);
}

.btn--secondary.btn--tone-danger:hover:not(:disabled) {
  background: rgba(125, 48, 61, 0.3);
  border-color: #a95663;
  color: #ffadb5;
}

.btn--secondary:disabled {
  opacity: 0.45;
  cursor: default;
}

/* --- text: acción de texto dentro de una fila ("Pausar", "Cancelar") -------- */
.btn--text {
  border: 0;
  background: transparent;
  padding: 4px 0;
  font-size: 10px;
  gap: 4px;
  color: #e88888;
}
.btn--text.btn--tone-warning {
  color: #ffc764;
}
.btn--text.btn--tone-success {
  color: #69d2ac;
}
.btn--text:hover:not(:disabled) {
  color: #ffb0b0;
  transform: translateX(-2px);
}
.btn--text.btn--tone-warning:hover:not(:disabled) {
  color: #ffd084;
}
.btn--text.btn--tone-success:hover:not(:disabled) {
  color: #8fe3c0;
}
.btn--text:disabled {
  opacity: 0.45;
  cursor: default;
}

/* --- icon: botón cuadrado solo-icono del historial ------------------------ */
.btn--icon {
  display: inline-grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--user-text-dim);
}
.btn--icon.btn--tone-primary {
  color: var(--user-primary);
}
.btn--icon.btn--tone-accent {
  color: var(--user-accent);
}
.btn--icon.btn--tone-danger {
  color: #d9828b;
}
.btn--icon:hover:not(:disabled) {
  background: var(--user-icon-bg);
  color: var(--user-accent);
}
.btn--icon.btn--tone-primary:hover:not(:disabled) {
  color: var(--user-accent);
}
.btn--icon.btn--tone-accent:hover:not(:disabled) {
  color: #fff;
  transform: rotate(-45deg);
}
.btn--icon.btn--tone-danger:hover:not(:disabled) {
  background: #48252c;
  color: #ffb0b0;
}
.btn--icon:disabled {
  opacity: 0.45;
  cursor: default;
}

/* --- pager: paginación del historial --------------------------------------
   Sin transición a propósito: al cambiar de página la fila de números se
   desplaza y animar el borde produce un destello. Igual que el original. */
.btn--pager {
  min-width: 30px;
  height: 30px;
  padding: 0 8px;
  display: inline-grid;
  place-items: center;
  border: 1px solid var(--user-border);
  background: var(--user-surface-light);
  color: var(--user-text-dim);
  border-radius: 8px;
  font-size: 11px;
  font-weight: 600;
  transition: none;
}
.btn--pager:hover:not(:disabled):not(.btn--active) {
  background: var(--user-icon-bg);
  color: var(--user-accent);
}
.btn--pager.btn--active {
  background: var(--user-gradient);
  color: var(--user-bg-base);
  border-color: transparent;
  cursor: default;
}
.btn--pager:disabled {
  opacity: 0.4;
  cursor: default;
}

/* --- size: escala del botón, se combina con cualquier variant -------------- */
.btn--size-sm {
  font-size: 11px;
  font-weight: 600;
  padding: 6px 10px;
  gap: 6px;
  border-radius: 8px;
}
.btn--size-lg {
  font-size: 14px;
  padding: 14px 22px;
  gap: 10px;
  border-radius: 10px;
}
.btn--text.btn--size-sm {
  font-size: 10px;
  padding: 4px 0;
  border-radius: 0;
}
.btn--text.btn--size-lg {
  font-size: 12px;
  padding: 6px 0;
}
.btn--icon.btn--size-sm {
  width: 28px;
  height: 28px;
  padding: 0;
}
.btn--icon.btn--size-lg {
  width: 36px;
  height: 36px;
  padding: 0;
}
.btn--pager.btn--size-sm {
  min-width: 26px;
  height: 26px;
  font-size: 10px;
}
.btn--pager.btn--size-lg {
  min-width: 34px;
  height: 34px;
  font-size: 12px;
}

.btn.btn--loading {
  cursor: wait;
}
</style>
