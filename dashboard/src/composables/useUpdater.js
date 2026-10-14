import { ref } from 'vue'
import { t } from '../i18n'

// Chequeo, aviso e instalacion de actualizaciones de la app. Se aisla del
// resto de App.vue porque su ciclo (poll de progreso, dialogo de
// confirmacion, posposicion) es independiente del estado de las descargas
// o del websocket. Requiere que quien la use ya tenga listos api(),
// showMessage() y openConfirm() (del composable useConfirmModal).
export function useUpdater({ api, showMessage, openConfirm }) {
  const version = ref('')
  const updateInfo = ref(null)
  const isUpdating = ref(false)
  const isUpdateForced = ref(false)
  const updatePostponedVersion = ref(null)
  const updateProgress = ref({
    status: 'idle',
    downloaded: 0,
    total: 0,
    percentage: 0
  })

  const installUpdate = async () => {
    try {
      isUpdating.value = true
      isUpdateForced.value = true
      await api('/api/update/install', { method: 'POST' })

      const pollProgress = async () => {
        try {
          const res = await api('/api/update/progress')
          updateProgress.value = res
          if (res.status.startsWith('error')) {
            isUpdating.value = false
            showMessage(t('update.progressError', { status: res.status }), true)
            return
          }
          if (res.status !== 'finishing') {
            setTimeout(pollProgress, 500)
          }
        } catch {
          setTimeout(pollProgress, 1000)
        }
      }
      pollProgress()
    } catch (err) {
      isUpdating.value = false
      showMessage(t('update.installError', { message: err.message }), true)
    }
  }

  const checkForUpdates = async (force = false) => {
    try {
      const data = await api('/api/update/check')
      version.value = data.current
      if (data.update_available) {
        updateInfo.value = data
        if (force) {
          isUpdateForced.value = true
        } else if (
          !isUpdateForced.value &&
          updatePostponedVersion.value !== data.latest
        ) {
          openConfirm({
            title: t('update.availableTitle'),
            message: t('update.availableText', { version: data.latest }),
            confirmText: t('update.updateNow'),
            cancelText: t('update.postpone'),
            type: 'primary',
            action: () => {
              isUpdateForced.value = true
              installUpdate()
            },
            cancelAction: () => {
              updatePostponedVersion.value = data.latest
            }
          })
        }
      }
    } catch (err) {
      console.error(t('console.updateCheckError'), err)
    }
  }

  return {
    version,
    updateInfo,
    isUpdating,
    isUpdateForced,
    updatePostponedVersion,
    updateProgress,
    installUpdate,
    checkForUpdates
  }
}
