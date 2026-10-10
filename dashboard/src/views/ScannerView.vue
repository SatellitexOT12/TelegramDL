<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  Clock3,
  Download,
  FileText,
  Image,
  Loader2,
  Music,
  Search,
  Settings2,
  Trash2,
  Video
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

const rawResults = ref([])
const scannedCount = ref(0)
const matchCount = ref(0)
const serverMessage = ref('')
const selectedIds = ref(new Set())
const downloadingBatch = ref(false)

// Menús desplegables para selección de nombres (individual y masivo)
const nameSelectionMenus = ref({})
const bulkNameMenuOpen = ref(false)

// Paginación de resultados idéntica al historial de descargas
const PAGE_SIZES = [10, 25, 50, 100]
const pageSize = ref(25)
const currentPage = ref(1)

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
  rawResults.value = []
  selectedIds.value = new Set()
  serverMessage.value = ''
  currentPage.value = 1
  nameSelectionMenus.value = {}
  bulkNameMenuOpen.value = false

  try {
    const payload = {
      target: cleanTarget,
      start_date: startDate.value,
      end_date: endDate.value,
      f_photos: true,
      f_videos: true,
      f_audios: true,
      f_docs: true,
      f_stickers: true,
      min_size: 0,
      max_size: 0,
      max_scan: 1000
    }

    const data = await api('/api/scanner/search', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })

    rawResults.value = data.items || []
    scannedCount.value = data.scanned_count || 0
    matchCount.value = data.match_count || 0
    serverMessage.value = data.message || ''

    if (rawResults.value.length === 0) {
      props.notify(t('scanner.noResults'), 'info')
    }
  } catch (err) {
    error.value = err.message || t('scanner.searchError')
    props.notify(error.value, 'danger')
  } finally {
    loading.value = false
  }
}

const onResetFilters = () => {
  target.value = ''
  startDate.value = ''
  endDate.value = ''
  fPhotos.value = true
  fVideos.value = true
  fAudios.value = true
  fDocs.value = true
  fStickers.value = true
  minSizeVal.value = ''
  minSizeUnit.value = 'MB'
  maxSizeVal.value = ''
  maxSizeUnit.value = 'MB'
  rawResults.value = []
  scannedCount.value = 0
  matchCount.value = 0
  selectedIds.value = new Set()
  error.value = ''
  serverMessage.value = ''
  currentPage.value = 1
  nameSelectionMenus.value = {}
  bulkNameMenuOpen.value = false
}

// Reactividad instantánea en el cliente al activar/desactivar filtros
const filteredResults = computed(() => {
  const minB = parseSizeBytes(minSizeVal.value, minSizeUnit.value)
  const maxB = parseSizeBytes(maxSizeVal.value, maxSizeUnit.value)

  return rawResults.value.filter((item) => {
    if (item.kind === 'photo' && !fPhotos.value) return false
    if (item.kind === 'video' && !fVideos.value) return false
    if (item.kind === 'song' && !fAudios.value) return false
    if (item.kind === 'file' && !fDocs.value) return false
    if (item.kind === 'sticker' && !fStickers.value) return false

    if (minB > 0 && item.file_size < minB) return false
    if (maxB > 0 && item.file_size > maxB) return false

    return true
  })
})

// Reiniciar a la página 1 cuando cambian los resultados filtrados o el tamaño de página
watch([filteredResults, pageSize], () => {
  currentPage.value = 1
})

const totalPages = computed(() => {
  return Math.max(1, Math.ceil(filteredResults.value.length / pageSize.value))
})

const paginatedResults = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredResults.value.slice(start, start + pageSize.value)
})

const visiblePages = computed(() => {
  const total = totalPages.value
  const maxVis = 5
  if (total <= maxVis) return Array.from({ length: total }, (_, i) => i + 1)
  const end = Math.min(total, Math.max(1, currentPage.value - 2) + maxVis - 1)
  const start = Math.max(1, end - maxVis + 1)
  return Array.from({ length: end - start + 1 }, (_, i) => start + i)
})

const goToPage = (page) => {
  currentPage.value = Math.min(totalPages.value, Math.max(1, page))
}

const isAllSelected = computed(() => {
  if (filteredResults.value.length === 0) return false
  return filteredResults.value.every((item) => selectedIds.value.has(item.id))
})

const toggleSelectAll = () => {
  if (isAllSelected.value) {
    selectedIds.value = new Set()
  } else {
    selectedIds.value = new Set(filteredResults.value.map((item) => item.id))
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
  return filteredResults.value.filter((item) => selectedIds.value.has(item.id))
})

// Funciones para selección de nombres (individual y masivo)
const toggleNameMenu = (itemId) => {
  const current = !!nameSelectionMenus.value[itemId]
  nameSelectionMenus.value = { [itemId]: !current }
  bulkNameMenuOpen.value = false
}

const toggleBulkNameMenu = () => {
  bulkNameMenuOpen.value = !bulkNameMenuOpen.value
  nameSelectionMenus.value = {}
}

const selectFileName = (item, selectedName) => {
  if (selectedName) {
    item.file_name = selectedName
  }
  nameSelectionMenus.value[item.id] = false
}

const selectBulkFileName = (type) => {
  bulkNameMenuOpen.value = false
  let updatedCount = 0

  filteredResults.value.forEach((item) => {
    const targetName =
      type === 'caption' ? item.caption_file_name : item.original_file_name
    if (
      targetName &&
      targetName.trim() !== '' &&
      item.file_name !== targetName
    ) {
      item.file_name = targetName
      updatedCount++
    }
  })

  if (updatedCount > 0) {
    const targetLabel =
      type === 'caption'
        ? t('listener.targetCaption')
        : t('listener.targetOriginal')
    props.notify(
      t('listener.bulkUpdated', { n: updatedCount, target: targetLabel }),
      'success'
    )
  }
}

const onWindowClick = () => {
  bulkNameMenuOpen.value = false
  nameSelectionMenus.value = {}
}

onMounted(() => {
  window.addEventListener('click', onWindowClick)
})

onUnmounted(() => {
  window.removeEventListener('click', onWindowClick)
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
  <div class="scanner-view">
    <!-- Panel de Control y Búsqueda -->
    <section class="panel scanner-search-panel">
      <div class="panel-heading">
        <div>
          <span class="eyebrow">
            <Search :size="12" /> {{ t('scanner.kicker') }}
          </span>
          <h2>{{ t('scanner.title') }}</h2>
          <p class="subtitle">{{ t('scanner.subtitle') }}</p>
          <p class="flood-warning-text">
            <AlertCircle :size="12" />
            <span>{{ t('scanner.floodWarningNote') }}</span>
          </p>
        </div>
      </div>

      <!-- Fila 1: Entrada de ID + Botón Escanear Histórico + Botón Eliminar Filtros -->
      <div class="scanner-row-1">
        <div class="field-target">
          <input
            v-model="target"
            type="text"
            class="scanner-input"
            :placeholder="t('scanner.targetPlaceholder')"
            @keyup.enter="onSearch"
          />
        </div>

        <button
          type="button"
          class="save-button search-btn"
          :disabled="loading || !target.trim()"
          @click="onSearch"
        >
          <Loader2 v-if="loading" :size="14" class="spin" />
          <Search v-else :size="14" />
          <span>{{ t('scanner.searchButton') }}</span>
        </button>

        <button
          type="button"
          class="clear-history-button boton-historial clear-btn"
          :title="t('scanner.clearFilters')"
          @click="onResetFilters"
        >
          <Trash2 :size="15" />
          <span>{{ t('scanner.clearFilters') }}</span>
        </button>
      </div>

      <!-- Fila 2: Filtros por Tipo + Fechas + Tamaños Mínimo/Máximo -->
      <div class="scanner-row-2">
        <div class="media-filters-group">
          <label class="filter-tag f-photos" :class="{ active: fPhotos }">
            <input v-model="fPhotos" type="checkbox" />
            <span>{{ t('scanner.photos') }}</span>
          </label>
          <label class="filter-tag f-videos" :class="{ active: fVideos }">
            <input v-model="fVideos" type="checkbox" />
            <span>{{ t('scanner.videos') }}</span>
          </label>
          <label class="filter-tag f-audios" :class="{ active: fAudios }">
            <input v-model="fAudios" type="checkbox" />
            <span>{{ t('scanner.audios') }}</span>
          </label>
          <label class="filter-tag f-docs" :class="{ active: fDocs }">
            <input v-model="fDocs" type="checkbox" />
            <span>{{ t('scanner.docs') }}</span>
          </label>
          <label class="filter-tag f-stickers" :class="{ active: fStickers }">
            <input v-model="fStickers" type="checkbox" />
            <span>{{ t('scanner.stickers') }}</span>
          </label>
        </div>

        <div class="date-filters-group">
          <div class="field-compact-date">
            <label class="compact-label">
              <Clock3 :size="11" /> {{ t('scanner.startDateLabel') }}
            </label>
            <input
              v-model="startDate"
              type="date"
              class="scanner-input compact-input"
            />
          </div>

          <div class="field-compact-date">
            <label class="compact-label">
              <Clock3 :size="11" /> {{ t('scanner.endDateLabel') }}
            </label>
            <input
              v-model="endDate"
              type="date"
              class="scanner-input compact-input"
            />
          </div>
        </div>

        <div class="size-filters-group">
          <div class="field-compact-size">
            <label class="compact-label">{{ t('scanner.minSizeLabel') }}</label>
            <div class="size-input-wrapper">
              <input
                v-model="minSizeVal"
                type="number"
                min="0"
                step="any"
                class="scanner-input compact-number"
                placeholder="0"
              />
              <select v-model="minSizeUnit" class="compact-select">
                <option value="B">B</option>
                <option value="KB">KB</option>
                <option value="MB">MB</option>
                <option value="GB">GB</option>
              </select>
            </div>
          </div>

          <div class="field-compact-size">
            <label class="compact-label">{{ t('scanner.maxSizeLabel') }}</label>
            <div class="size-input-wrapper">
              <input
                v-model="maxSizeVal"
                type="number"
                min="0"
                step="any"
                class="scanner-input compact-number"
                placeholder="0"
              />
              <select v-model="maxSizeUnit" class="compact-select">
                <option value="B">B</option>
                <option value="KB">KB</option>
                <option value="MB">MB</option>
                <option value="GB">GB</option>
              </select>
            </div>
          </div>
        </div>
      </div>

      <div v-if="error" class="scanner-error">
        <AlertCircle :size="14" /> {{ error }}
      </div>
    </section>

    <!-- Estado de Carga -->
    <section v-if="loading" class="panel scanner-loading-panel">
      <div class="loading-content">
        <Loader2 :size="22" class="spin icon-accent" />
        <div>
          <strong>{{ t('scanner.scanning') }}</strong>
          <p class="small-hint">{{ searchingMsg }}</p>
        </div>
      </div>
    </section>

    <!-- Resultados Encontrados -->
    <section
      v-else-if="rawResults.length > 0"
      class="panel scanner-results-panel"
    >
      <div class="panel-heading results-heading">
        <!-- Texto del contador dividido en 2 líneas y más grande -->
        <div class="results-counter-box">
          <span class="results-count-title">
            {{
              t('scanner.resultsCount', {
                count: filteredResults.length
              })
            }}
          </span>
          <span class="scanned-count-sub">
            {{
              t('scanner.messagesScanned', {
                scanned: scannedCount
              })
            }}
          </span>
          <p v-if="serverMessage" class="server-msg">{{ serverMessage }}</p>
        </div>

        <div class="results-toolbar">
          <!-- Menú masivo de cambio de nombre idéntico al del Listener -->
          <div class="bulk-name-wrapper">
            <button
              type="button"
              class="bulk-name-button btn-compact"
              :title="t('listener.bulkNameTitle')"
              @click.stop="toggleBulkNameMenu"
            >
              <Settings2 :size="13" />
              <span>{{ t('listener.nameLabel') }}</span>
            </button>
            <div
              v-if="bulkNameMenuOpen"
              class="name-dropdown bulk-name-dropdown"
            >
              <div class="name-option" @click="selectBulkFileName('caption')">
                <span class="name-preview">{{ t('listener.useCaption') }}</span>
                <small>{{ t('scanner.useCaptionSub') }}</small>
              </div>
              <div class="name-option" @click="selectBulkFileName('original')">
                <span class="name-preview">{{
                  t('listener.useOriginal')
                }}</span>
                <small>{{ t('scanner.useOriginalSub') }}</small>
              </div>
            </div>
          </div>

          <button
            type="button"
            class="ghost-button btn-compact"
            @click="toggleSelectAll"
          >
            {{
              isAllSelected ? t('scanner.deselectAll') : t('scanner.selectAll')
            }}
          </button>

          <button
            type="button"
            class="save-button btn-compact"
            :disabled="selectedIds.size === 0 || downloadingBatch"
            @click="onDownloadItems(selectedItemsList)"
          >
            <Download :size="13" />
            <span>{{
              t('scanner.downloadSelected', { n: selectedIds.size })
            }}</span>
          </button>

          <button
            type="button"
            class="ghost-button btn-compact"
            :disabled="downloadingBatch || filteredResults.length === 0"
            @click="onDownloadItems(filteredResults)"
          >
            <Download :size="13" />
            <span>{{ t('scanner.downloadAll') }}</span>
          </button>
        </div>
      </div>

      <!-- Tabla de Resultados -->
      <div class="results-table-wrapper">
        <table class="scanner-table">
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
              <th class="col-name-mode">{{ t('scanner.nameModeColumn') }}</th>
              <th class="col-date">{{ t('scanner.date') }}</th>
              <th class="col-size">{{ t('scanner.size') }}</th>
              <th class="col-action"></th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="item in paginatedResults"
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
                  :size="16"
                  class="kind-icon"
                />
              </td>
              <td class="col-name">
                <div class="name-box">
                  <span class="main-name" :title="item.file_name">{{
                    item.file_name
                  }}</span>
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
              <td class="col-name-mode">
                <div
                  v-if="
                    item.caption_file_name &&
                    item.caption_file_name.trim() !== ''
                  "
                  class="name-select-wrapper"
                >
                  <button
                    type="button"
                    class="name-select-button"
                    :title="t('listener.nameLabel')"
                    @click.stop="toggleNameMenu(item.id)"
                  >
                    <Settings2 :size="12" />
                  </button>
                  <div v-if="nameSelectionMenus[item.id]" class="name-dropdown">
                    <div
                      class="name-option"
                      @click="selectFileName(item, item.caption_file_name)"
                    >
                      <span class="name-preview">{{
                        item.caption_file_name
                      }}</span>
                      <small>({{ t('listener.captionTag') }})</small>
                    </div>
                    <div
                      v-if="item.original_file_name"
                      class="name-option"
                      @click="selectFileName(item, item.original_file_name)"
                    >
                      <span class="name-preview">{{
                        item.original_file_name
                      }}</span>
                      <small>({{ t('listener.originalTag') }})</small>
                    </div>
                  </div>
                </div>
              </td>
              <td class="col-date">{{ item.date_str }}</td>
              <td class="col-size">{{ item.formatted_size }}</td>
              <td class="col-action">
                <button
                  type="button"
                  class="ghost-button btn-icon"
                  :title="t('scanner.downloadItem')"
                  @click="onDownloadItems([item])"
                >
                  <Download :size="13" />
                </button>
              </td>
            </tr>
            <tr v-if="filteredResults.length === 0">
              <td colspan="7" class="no-filtered-matches">
                {{ t('scanner.noResults') }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Paginador de resultados idéntico al del historial de descargas -->
      <div v-if="filteredResults.length" class="history-pager">
        <template v-if="totalPages > 1">
          <button
            class="pager-btn"
            type="button"
            :title="t('downloads.prevPage')"
            :aria-label="t('downloads.prevPage')"
            :disabled="currentPage === 1"
            @click="goToPage(currentPage - 1)"
          >
            <ArrowLeft :size="14" />
          </button>
          <button
            v-for="page in visiblePages"
            :key="page"
            class="pager-btn"
            :class="{ active: page === currentPage }"
            type="button"
            :aria-current="page === currentPage ? 'page' : undefined"
            @click="goToPage(page)"
          >
            {{ page }}
          </button>
          <button
            class="pager-btn"
            type="button"
            :title="t('downloads.nextPage')"
            :aria-label="t('downloads.nextPage')"
            :disabled="currentPage === totalPages"
            @click="goToPage(currentPage + 1)"
          >
            <ArrowRight :size="14" />
          </button>
        </template>

        <div class="pager-side">
          <span v-if="totalPages > 1" class="pager-info">
            {{
              t('downloads.pageOf', {
                page: currentPage,
                total: totalPages
              })
            }}
          </span>
          <select
            v-model="pageSize"
            class="pager-select"
            :title="t('downloads.perPageLabel')"
            :aria-label="t('downloads.perPageLabel')"
          >
            <option v-for="size in PAGE_SIZES" :key="size" :value="size">
              {{ t('downloads.perPage', { n: size }) }}
            </option>
          </select>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.scanner-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.panel {
  padding: 22px 24px;
  background: var(--user-surface);
  border: 1px solid var(--user-border);
  border-radius: 15px;
}

.scanner-search-panel {
  position: relative;
  overflow: hidden;
  padding: 28px 30px;
  border: 1px solid var(--user-border-light);
  border-radius: 15px;
  background: linear-gradient(
    110deg,
    var(--user-surface-light),
    var(--user-surface) 70%
  );
  box-shadow: 0 18px 45px rgba(0, 0, 0, 0.3);
}

.scanner-search-panel .panel-heading {
  margin-bottom: 20px;
}

.flood-warning-text {
  font-size: 12px;
  color: var(--user-text-dim, #94a8bc);
  margin-top: 6px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.flood-warning-text svg {
  color: #f59e0b;
  flex-shrink: 0;
}

/* Fila 1: Target input + Botón de escaneo + Botón de eliminar filtros */
.scanner-row-1 {
  display: flex !important;
  align-items: center !important;
  gap: 12px !important;
  width: 100% !important;
}

.field-target {
  flex: 1 1 260px !important;
  min-width: 200px !important;
  max-width: 380px !important;
}

.field-target .scanner-input {
  height: 42px !important;
  padding: 0 16px !important;
  font-size: 13px !important;
  border-radius: 12px !important;
  box-sizing: border-box !important;
}

.scanner-input {
  background: var(--user-bg-base);
  border: 1px solid var(--user-border-light);
  color: var(--user-text-main, #dbe7f5);
  border-radius: 10px;
  padding: 8px 12px;
  outline: none;
  font: inherit;
  font-size: 12px;
  width: 100%;
  transition:
    border-color 0.2s,
    background-color 0.2s,
    color 0.2s;
}

.scanner-input:focus {
  border-color: var(--user-primary);
}

.compact-select {
  background: var(--user-bg-base);
  border: 1px solid var(--user-border-light);
  color: var(--user-text-main, #dbe7f5);
  border-radius: 10px;
  padding: 5px 8px;
  font-size: 11px;
  height: 36px;
  outline: none;
}

/* Visibilidad clara del icono de calendario en temas oscuros (blanco brillante) */
.scanner-input[type='date']::-webkit-calendar-picker-indicator {
  cursor: pointer !important;
  filter: invert(1) !important;
  -webkit-filter: invert(1) !important;
  opacity: 0.95 !important;
}

/* Reglas específicas para Temas Claros (Silent Cherry, OSX Light, etc. - html.tema-claro) */
html.tema-claro .scanner-input {
  color: #0f172a !important;
  background: var(--claro-superficie, #ffffff) !important;
  border-color: var(--claro-borde, rgba(0, 0, 0, 0.15)) !important;
}

html.tema-claro .compact-select {
  color: #0f172a !important;
  background: var(--claro-superficie, #ffffff) !important;
  border-color: var(--claro-borde, rgba(0, 0, 0, 0.15)) !important;
}

html.tema-claro .compact-label {
  color: var(--claro-texto-atenuado, #64748b) !important;
}

html.tema-claro .scanner-input[type='date']::-webkit-calendar-picker-indicator {
  filter: none !important;
  -webkit-filter: none !important;
  opacity: 0.8 !important;
}

.search-btn.save-button {
  width: auto !important;
  margin-top: 0 !important;
  height: 42px !important;
  padding: 0 20px !important;
  font-size: 13px !important;
  border-radius: 12px !important;
  white-space: nowrap !important;
  flex-shrink: 0 !important;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  box-sizing: border-box !important;
}

.clear-btn.clear-history-button {
  width: auto !important;
  margin-top: 0 !important;
  height: 42px !important;
  padding: 0 18px !important;
  font-size: 13px !important;
  border-radius: 12px !important;
  white-space: nowrap !important;
  flex-shrink: 0 !important;
  box-sizing: border-box !important;
}

/* Fila 2: Filtros por Tipo + Fechas Inicio/Fin + Tamaños Mínimo/Máximo en la misma línea */
.scanner-row-2 {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 14px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--user-border);
  flex-wrap: wrap;
}

.media-filters-group {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.filter-tag {
  display: flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 20px;
  font-size: 10px;
  font-weight: 700;
  color: var(--user-text-dim, #94a8bc);
  cursor: pointer;
  background: var(--user-bg-base);
  border: 1px solid var(--user-border-light);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  user-select: none;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.filter-tag input {
  display: none;
}
.filter-tag:hover {
  border-color: var(--user-primary);
  transform: translateY(-1px);
}
.filter-tag.active {
  color: #000;
  border-color: transparent;
  box-shadow: 0 4px 10px rgba(0, 0, 0, 0.3);
  transform: translateY(-1px);
}
.filter-tag.f-photos.active {
  background: #38bdf8;
  box-shadow: 0 4px 12px rgba(56, 189, 248, 0.3);
}
.filter-tag.f-videos.active {
  background: #c084fc;
  box-shadow: 0 4px 12px rgba(192, 132, 252, 0.3);
}
.filter-tag.f-audios.active {
  background: #4ade80;
  box-shadow: 0 4px 12px rgba(74, 222, 128, 0.3);
}
.filter-tag.f-docs.active {
  background: #94a3b8;
  box-shadow: 0 4px 12px rgba(148, 163, 184, 0.3);
}
.filter-tag.f-stickers.active {
  background: #f472b6;
  box-shadow: 0 4px 12px rgba(244, 114, 182, 0.3);
}

.date-filters-group,
.size-filters-group {
  display: flex;
  align-items: center;
  gap: 10px;
}

.field-compact-date,
.field-compact-size {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.compact-label {
  font-size: 10px;
  font-weight: 600;
  color: var(--user-text-dim, #94a8bc);
  display: flex;
  align-items: center;
  gap: 4px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.compact-input {
  width: 125px;
  padding: 5px 8px;
  font-size: 11px;
}

.size-input-wrapper {
  display: flex;
  align-items: center;
  gap: 4px;
}

.compact-number {
  width: 60px;
  padding: 5px 6px;
  font-size: 11px;
  text-align: right;
}

.scanner-error {
  margin-top: 10px;
  padding: 8px 12px;
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: 8px;
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.scanner-loading-panel {
  padding: 20px;
}

.loading-content {
  display: flex;
  align-items: center;
  gap: 14px;
}

.icon-accent {
  color: var(--user-primary);
}

.small-hint {
  font-size: 11px;
  color: var(--user-text-dim, #94a8bc);
  margin-top: 2px;
}

.results-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 12px;
}

.results-counter-box {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.results-count-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--user-primary);
  line-height: 1.2;
}

.scanned-count-sub {
  font-size: 11px;
  color: var(--user-text-dim, #94a8bc);
  line-height: 1.2;
}

.server-msg {
  font-size: 11px;
  color: #f59e0b;
  margin-top: 2px;
}

.results-toolbar {
  display: flex !important;
  align-items: center !important;
  gap: 8px !important;
}

.results-toolbar .btn-compact {
  padding: 5px 12px !important;
  font-size: 11px !important;
  height: 32px !important;
  margin: 0 !important;
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  vertical-align: middle !important;
  line-height: 1 !important;
  box-sizing: border-box !important;
}

.btn-icon {
  padding: 4px 6px;
  border-radius: 6px;
}

.ghost-button {
  background: transparent;
  border: 1px solid var(--user-border-light);
  color: var(--user-text-main, #dbe7f5);
  border-radius: 8px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  transition: all 0.2s;
}

.ghost-button:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.06);
  border-color: var(--user-primary);
}

.ghost-button:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.results-table-wrapper {
  overflow-x: auto;
}

.scanner-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  table-layout: fixed; /* Evita desplazamientos horizontales de columnas entre páginas */
}

.scanner-table th,
.scanner-table td {
  padding: 8px 10px;
  text-align: left;
  border-bottom: 1px solid var(--user-border);
  box-sizing: border-box;
}

.scanner-table th {
  color: var(--user-text-dim, #94a8bc);
  font-weight: 600;
  font-size: 11px;
  text-transform: uppercase;
}

.scanner-table tr.selected {
  background: rgba(56, 167, 255, 0.08);
}

.col-check {
  width: 32px;
  text-align: center;
}

.col-type {
  width: 44px;
  text-align: center;
}

.col-name {
  width: auto;
  overflow: hidden;
}

.main-name {
  font-weight: 500;
  color: var(--user-text-main, #dbe7f5);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}

.sub-name {
  font-size: 11px;
  color: var(--user-text-dim, #94a8bc);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}

/* Columna propia y centrada para el botón individual de cambio de nombre */
.col-name-mode {
  width: 65px;
  text-align: center;
  vertical-align: middle;
}

.col-name-mode .name-select-wrapper {
  position: relative;
  display: inline-flex;
  justify-content: center;
}

.name-select-wrapper {
  position: relative;
  display: inline-flex;
}

.name-select-button {
  border: 1px solid var(--user-primary);
  background: var(--user-icon-bg);
  color: var(--user-primary);
  border-radius: 6px;
  padding: 4px 6px;
  font-size: 10px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 3px;
  transition: all 0.2s;
  font-weight: 600;
}

.name-select-button:hover {
  background: var(--user-surface-light);
  border-color: var(--user-accent);
  color: var(--user-accent);
}

/* Selector masivo de nombres en la barra de herramientas */
.bulk-name-wrapper {
  position: relative;
  display: inline-flex;
}

.bulk-name-button {
  border: 1px solid var(--user-primary);
  background: var(--user-icon-bg);
  color: var(--user-primary);
  border-radius: 8px;
  padding: 5px 10px;
  font-size: 11px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  transition: all 0.2s;
  font-weight: 600;
  height: 32px;
}

.bulk-name-button:hover {
  background: var(--user-surface-light);
  border-color: var(--user-accent);
  color: var(--user-accent);
}

.bulk-name-dropdown {
  left: 0;
  right: auto;
}

.name-dropdown {
  position: absolute;
  right: 0;
  top: calc(100% + 6px);
  background: var(--user-surface);
  border: 2px solid var(--user-primary);
  border-radius: 10px;
  padding: 6px;
  min-width: 200px;
  max-width: 320px;
  z-index: 1000;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
}

.name-dropdown::before {
  content: '';
  position: absolute;
  top: -6px;
  right: 16px;
  width: 10px;
  height: 10px;
  background: var(--user-surface);
  border-left: 2px solid var(--user-primary);
  border-top: 2px solid var(--user-primary);
  transform: rotate(45deg);
}

.bulk-name-dropdown::before {
  left: 16px;
  right: auto;
}

.name-option {
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 2px;
  transition: all 0.2s;
  border: 1px solid transparent;
}

.name-option:hover {
  background: var(--user-icon-bg);
  border-color: var(--user-primary);
}

.name-preview {
  font-size: 11px;
  font-weight: 600;
  color: var(--user-text-main, #dbe7f5);
  word-break: break-all;
}

.name-option small {
  font-size: 10px;
  color: var(--user-text-dim, #94a8bc);
}

.col-date {
  width: 150px;
  white-space: nowrap;
  color: var(--user-text-dim, #94a8bc);
}

.col-size {
  width: 90px;
  white-space: nowrap;
  color: var(--user-text-main, #dbe7f5);
}

.col-action {
  width: 44px;
  text-align: right;
}

.no-filtered-matches {
  text-align: center;
  padding: 20px;
  color: var(--user-text-dim, #94a8bc);
  font-size: 12px;
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
</style>
