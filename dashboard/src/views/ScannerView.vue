<script setup>
import { computed, ref } from 'vue'
import {
  Download,
  FileText,
  Image,
  Loader2,
  Music,
  Search,
  Video,
  AlertCircle,
  Clock3,
  Filter
} from '../icons'
import { useAuthToken } from '../composables/useAuthToken'
import { useI18n } from '../i18n'

const { authHeaders } = useAuthToken()
const { t } = useI18n()

const props = defineProps({
  notify: { type: Function, default: () => {} },
  disk: { type: Object, default: null },
  settings: { type: Object, default: () => ({}) },
  active: { type: Boolean, default: true }
})

const target = ref('')
const startDate = ref('')
const endDate = ref('')
const fPhotos = ref(true)
const fVideos = ref(true)
const fAudios = ref(true)
const fDocs = ref(true)
const fStickers = ref(true)

const minSizeVal = ref('')
const minSizeUnit = ref('MB')
const maxSizeVal = ref('')
const maxSizeUnit = ref('MB')

const loading = ref(false)
const searchingMsg = ref('')
const error = ref('')

const results = ref([])
const scannedCount = ref(0)
const matchCount = ref(0)
const reachedLimit = ref(false)
const serverMessage = ref('')
const selectedIds = ref(new Set())
const downloadingBatch = ref(false)

const unitMultipliers = {
  B: 1,
  KB: 1024,
  MB: 1024 * 1024,
  GB: 1024 * 1024 * 1024
}

const parseSizeBytes = (val, unit) => {
  if (!val || isNaN(val) || Number(val) <= 0) return 0
  const mult = unitMultipliers[unit] || unitMultipliers.MB
  return Math.floor(Number(val) * mult)
}

const api = async (url, options = {}) => {
  const response = await fetch(url, {
    ...options,
    headers: { ...(options.headers || {}), ...authHeaders() }
  })
  const data = await response.json().catch(() => ({}))
  if (!response.ok)
    throw new Error(data.detail || data.error || t('common.serverError'))
  return data
}

const onSearch = async () => {
  const cleanTarget = target.value.trim()
  if (!cleanTarget) {
    error.value = t('scanner.targetPlaceholder')
    return
  }

  error.value = ''
  loading.value = true
  searchingMsg.value = t('scanner.scanningNote')
  results.value = []
  selectedIds.value = new Set()
  serverMessage.value = ''

  try {
    const payload = {
      target: cleanTarget,
      start_date: startDate.value,
      end_date: endDate.value,
      f_photos: fPhotos.value,
      f_videos: fVideos.value,
      f_audios: fAudios.value,
      f_docs: fDocs.value,
      f_stickers: fStickers.value,
      min_size: parseSizeBytes(minSizeVal.value, minSizeUnit.value),
      max_size: parseSizeBytes(maxSizeVal.value, maxSizeUnit.value),
      max_scan: 1000
    }

    const data = await api('/api/scanner/search', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })

    results.value = data.items || []
    scannedCount.value = data.scanned_count || 0
    matchCount.value = data.match_count || 0
    reachedLimit.value = !!data.reached_limit
    serverMessage.value = data.message || ''

    if (results.value.length === 0) {
      props.notify(t('scanner.noResults'), 'info')
    }
  } catch (err) {
    error.value = err.message || t('scanner.searchError')
    props.notify(error.value, 'danger')
  } finally {
    loading.value = false
  }
}

const isAllSelected = computed(() => {
  if (results.value.length === 0) return false
  return results.value.every((item) => selectedIds.value.has(item.id))
})

const toggleSelectAll = () => {
  if (isAllSelected.value) {
    selectedIds.value = new Set()
  } else {
    selectedIds.value = new Set(results.value.map((item) => item.id))
  }
}

const toggleSelectItem = (id) => {
  const newSet = new Set(selectedIds.value)
  if (newSet.has(id)) {
    newSet.delete(id)
  } else {
    newSet.add(id)
  }
  selectedIds.value = newSet
}

const selectedItemsList = computed(() => {
  return results.value.filter((item) => selectedIds.value.has(item.id))
})

const onDownloadItems = async (itemsToDownload) => {
  if (!itemsToDownload || itemsToDownload.length === 0) return
  downloadingBatch.value = true
  try {
    const data = await api('/api/scanner/download', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ items: itemsToDownload })
    })

    const count = data.queued || itemsToDownload.length
    props.notify(t('scanner.downloadingQueued', { n: count }), 'success')

    // Remover los descargados de la selección
    const newSet = new Set(selectedIds.value)
    itemsToDownload.forEach((item) => newSet.delete(item.id))
    selectedIds.value = newSet
  } catch (err) {
    props.notify(err.message || t('common.serverError'), 'danger')
  } finally {
    downloadingBatch.value = false
  }
}

const getKindIcon = (kind) => {
  switch (kind) {
    case 'photo':
      return Image
    case 'video':
      return Video
    case 'song':
      return Music
    default:
      return FileText
  }
}
</script>

<template>
  <div class="scanner-container">
    <!-- Encabezado de la Sección -->
    <div class="scanner-card">
      <div class="card-header">
        <div class="title-wrap">
          <Search :size="20" class="icon-accent" />
          <div>
            <h2>{{ t('scanner.title') }}</h2>
            <p class="subtitle">{{ t('scanner.subtitle') }}</p>
          </div>
        </div>
      </div>

      <!-- Formulario de Búsqueda e Histórico -->
      <div class="card-body">
        <div class="form-grid">
          <!-- Campo 1: Objetivo (ID / Username / URL) -->
          <div class="form-group full-width">
            <label class="form-label">
              {{ t('scanner.targetLabel') }}
            </label>
            <div class="input-with-button">
              <input
                v-model="target"
                type="text"
                class="form-input"
                :placeholder="t('scanner.targetPlaceholder')"
                @keyup.enter="onSearch"
              />
              <button
                type="button"
                class="btn btn-primary"
                :disabled="loading || !target.trim()"
                @click="onSearch"
              >
                <Loader2 v-if="loading" :size="16" class="spin" />
                <Search v-else :size="16" />
                <span>{{ t('scanner.searchButton') }}</span>
              </button>
            </div>
          </div>

          <!-- Campo 2: Rango de Fechas -->
          <div class="form-group">
            <label class="form-label">
              <Clock3 :size="14" /> {{ t('scanner.startDateLabel') }}
            </label>
            <input v-model="startDate" type="date" class="form-input" />
          </div>

          <div class="form-group">
            <label class="form-label">
              <Clock3 :size="14" /> {{ t('scanner.endDateLabel') }}
            </label>
            <input v-model="endDate" type="date" class="form-input" />
          </div>

          <!-- Campo 3: Filtros por Tipo de Archivo -->
          <div class="form-group full-width">
            <label class="form-label">
              <Filter :size="14" /> {{ t('scanner.mediaTypesLabel') }}
            </label>
            <div class="checkbox-group">
              <label class="checkbox-label">
                <input v-model="fPhotos" type="checkbox" />
                <span>{{ t('scanner.photos') }}</span>
              </label>
              <label class="checkbox-label">
                <input v-model="fVideos" type="checkbox" />
                <span>{{ t('scanner.videos') }}</span>
              </label>
              <label class="checkbox-label">
                <input v-model="fAudios" type="checkbox" />
                <span>{{ t('scanner.audios') }}</span>
              </label>
              <label class="checkbox-label">
                <input v-model="fDocs" type="checkbox" />
                <span>{{ t('scanner.docs') }}</span>
              </label>
              <label class="checkbox-label">
                <input v-model="fStickers" type="checkbox" />
                <span>{{ t('scanner.stickers') }}</span>
              </label>
            </div>
          </div>

          <!-- Campo 4: Filtros por Tamaño Mínimo y Máximo -->
          <div class="form-group">
            <label class="form-label">{{ t('scanner.minSizeLabel') }}</label>
            <div class="size-input-group">
              <input
                v-model="minSizeVal"
                type="number"
                min="0"
                step="any"
                class="form-input"
                placeholder="0"
              />
              <select v-model="minSizeUnit" class="form-select">
                <option value="B">B</option>
                <option value="KB">KB</option>
                <option value="MB">MB</option>
                <option value="GB">GB</option>
              </select>
            </div>
          </div>

          <div class="form-group">
            <label class="form-label">{{ t('scanner.maxSizeLabel') }}</label>
            <div class="size-input-group">
              <input
                v-model="maxSizeVal"
                type="number"
                min="0"
                step="any"
                class="form-input"
                placeholder="0"
              />
              <select v-model="maxSizeUnit" class="form-select">
                <option value="B">B</option>
                <option value="KB">KB</option>
                <option value="MB">MB</option>
                <option value="GB">GB</option>
              </select>
            </div>
          </div>
        </div>

        <p class="hint-text">{{ t('scanner.sizeOpenHint') }}</p>

        <div v-if="error" class="alert alert-danger">
          <AlertCircle :size="16" /> {{ error }}
        </div>
      </div>
    </div>

    <!-- Banner de Carga en Búsqueda -->
    <div v-if="loading" class="scanner-card loading-card">
      <div class="loading-wrap">
        <Loader2 :size="24" class="spin icon-accent" />
        <div>
          <h3>{{ t('scanner.scanning') }}</h3>
          <p class="subtitle">{{ searchingMsg }}</p>
        </div>
      </div>
    </div>

    <!-- Resultados Encontrados -->
    <div v-else-if="results.length > 0" class="scanner-card">
      <div class="results-header">
        <div class="results-info">
          <h3>
            {{
              t('scanner.resultsFound', {
                count: matchCount,
                scanned: scannedCount
              })
            }}
          </h3>
          <p v-if="serverMessage" class="server-msg">{{ serverMessage }}</p>
        </div>

        <div class="results-actions">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="toggleSelectAll"
          >
            {{
              isAllSelected ? t('scanner.deselectAll') : t('scanner.selectAll')
            }}
          </button>

          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="selectedIds.size === 0 || downloadingBatch"
            @click="onDownloadItems(selectedItemsList)"
          >
            <Download :size="14" />
            <span>{{
              t('scanner.downloadSelected', { n: selectedIds.size })
            }}</span>
          </button>

          <button
            type="button"
            class="btn btn-outline btn-sm"
            :disabled="downloadingBatch"
            @click="onDownloadItems(results)"
          >
            <Download :size="14" />
            <span>{{ t('scanner.downloadAll') }}</span>
          </button>
        </div>
      </div>

      <!-- Tabla de Resultados -->
      <div class="table-responsive">
        <table class="results-table">
          <thead>
            <tr>
              <th class="col-check">
                <input
                  type="checkbox"
                  :checked="isAllSelected"
                  @change="toggleSelectAll"
                />
              </th>
              <th class="col-type">{{ t('scanner.type') }}</th>
              <th class="col-name">{{ t('scanner.fileName') }}</th>
              <th class="col-date">{{ t('scanner.date') }}</th>
              <th class="col-size">{{ t('scanner.size') }}</th>
              <th class="col-action">{{ t('scanner.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="item in results"
              :key="item.id"
              :class="{ selected: selectedIds.has(item.id) }"
            >
              <td class="col-check">
                <input
                  type="checkbox"
                  :checked="selectedIds.has(item.id)"
                  @change="toggleSelectItem(item.id)"
                />
              </td>
              <td class="col-type">
                <component
                  :is="getKindIcon(item.kind)"
                  :size="18"
                  class="kind-icon"
                />
              </td>
              <td class="col-name">
                <div class="name-box">
                  <span class="main-name">{{ item.file_name }}</span>
                  <span
                    v-if="
                      item.caption_file_name &&
                      item.caption_file_name !== item.file_name
                    "
                    class="sub-name"
                  >
                    {{ item.caption_file_name }}
                  </span>
                </div>
              </td>
              <td class="col-date">{{ item.date_str }}</td>
              <td class="col-size">{{ item.formatted_size }}</td>
              <td class="col-action">
                <button
                  type="button"
                  class="btn btn-icon btn-sm"
                  :title="t('scanner.downloadItem')"
                  @click="onDownloadItems([item])"
                >
                  <Download :size="14" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.scanner-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.scanner-card {
  background: var(--bg-card, #1e1e2d);
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.08));
  border-radius: 12px;
  padding: 20px;
}

.card-header {
  margin-bottom: 20px;
}

.title-wrap {
  display: flex;
  align-items: center;
  gap: 12px;
}

.title-wrap h2 {
  font-size: 1.15rem;
  font-weight: 600;
  margin: 0;
}

.subtitle {
  font-size: 0.85rem;
  color: var(--text-muted, #8a8aa0);
  margin-top: 2px;
}

.icon-accent {
  color: var(--accent-color, #3b82f6);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.full-width {
  grid-column: span 2;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  font-size: 0.85rem;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-color, #e1e1e6);
}

.input-with-button {
  display: flex;
  gap: 8px;
}

.form-input,
.form-select {
  background: var(--bg-input, rgba(0, 0, 0, 0.2));
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.12));
  color: var(--text-color, #ffffff);
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 0.9rem;
  outline: none;
  transition: border-color 0.2s;
}

.form-input:focus,
.form-select:focus {
  border-color: var(--accent-color, #3b82f6);
}

.size-input-group {
  display: flex;
  gap: 8px;
}

.size-input-group .form-input {
  flex: 1;
}

.checkbox-group {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  padding-top: 4px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.85rem;
  cursor: pointer;
}

.hint-text {
  font-size: 0.8rem;
  color: var(--text-muted, #8a8aa0);
  margin-top: 12px;
}

.alert {
  padding: 10px 14px;
  border-radius: 8px;
  font-size: 0.85rem;
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.alert-danger {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
  border: 1px solid rgba(239, 68, 68, 0.3);
}

.loading-card {
  padding: 30px;
  text-align: center;
}

.loading-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.results-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}

.results-info h3 {
  font-size: 1rem;
  font-weight: 600;
  margin: 0;
}

.server-msg {
  font-size: 0.8rem;
  color: #f59e0b;
  margin-top: 4px;
}

.results-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.table-responsive {
  overflow-x: auto;
}

.results-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
}

.results-table th,
.results-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid var(--border-color, rgba(255, 255, 255, 0.06));
}

.results-table th {
  color: var(--text-muted, #8a8aa0);
  font-weight: 600;
  font-size: 0.8rem;
  text-transform: uppercase;
}

.results-table tr.selected {
  background: rgba(59, 130, 246, 0.08);
}

.col-check {
  width: 36px;
}

.col-type {
  width: 48px;
  text-align: center;
}

.kind-icon {
  color: var(--accent-color, #3b82f6);
}

.name-box {
  display: flex;
  flex-direction: column;
}

.main-name {
  font-weight: 500;
  word-break: break-all;
}

.sub-name {
  font-size: 0.75rem;
  color: var(--text-muted, #8a8aa0);
}

.col-date {
  white-space: nowrap;
  color: var(--text-muted, #8a8aa0);
}

.col-size {
  white-space: nowrap;
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 0.85rem;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all 0.2s;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: var(--accent-color, #3b82f6);
  color: #ffffff;
}

.btn-primary:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-secondary {
  background: rgba(255, 255, 255, 0.08);
  color: var(--text-color, #ffffff);
}

.btn-secondary:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.15);
}

.btn-outline {
  background: transparent;
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.2));
  color: var(--text-color, #ffffff);
}

.btn-outline:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.05);
}

.btn-sm {
  padding: 6px 12px;
  font-size: 0.8rem;
}

.btn-icon {
  padding: 6px;
  background: rgba(255, 255, 255, 0.05);
  color: var(--text-color, #ffffff);
  border-radius: 6px;
}

.btn-icon:hover:not(:disabled) {
  background: var(--accent-color, #3b82f6);
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .full-width {
    grid-column: span 1;
  }
}
</style>
