# Directrices de Desarrollo y Reglas para Asistentes de IA (AGENTS.md)

Este archivo define la arquitectura del proyecto `tgdown` y establece las reglas obligatorias de conducta y codificación para cualquier asistente de inteligencia artificial (Claude, Copilot, Cursor, Windsurf, etc.) que trabaje en este código fuente.

---

## 🎯 PROPÓSITO DEL PROYECTO (`tgdown`)
`tgdown` (Telegram DL) es una aplicación de escritorio y panel web para la descarga de archivos y medios desde Telegram, con soporte para descargas concurrentes, chunks paralelos, límite de velocidad, escuchador automático de chats/canales (listener), acceso remoto mediante token/Tailscale y gestión de descargas guardadas en SQLite local.

---

## 🛑 REGLAS DE ORO OBLIGATORIAS PARA LA IA

### 1. Principio de Cirugía Mínima y No Regresión
- **Si el código funciona, NO lo refactorices por gusto**: Haz únicamente modificaciones quirúrgicas en las líneas requeridas para la tarea o bug reportado.
- **Prohibido reescribir arquitectura existente**: No reemplaces librerías, no cambies patrones de diseño globales ni restructures componentes completos a menos que el usuario lo solicite expresamente.
- **Verifica el contexto global antes de editar**: Analiza la comunicación entre el backend en Go (`pkg/server`, `pkg/config`) y el frontend Vue (`dashboard/src/App.vue`).

### 2. Gestión de Estado y Reactividad en Vue 3
- **Única fuente de verdad (Single Source of Truth)**: El objeto de configuración `settings` está centralizado en `dashboard/src/App.vue`. Los componentes hijos deben usar ese objeto reactivo directamente o mediante `props.settings`.
- **PROHIBIDO crear copias desincronizadas**: No dupliques objetos reactivos en componentes hijos sin un mecanismo claro que mantenga sincronizado el estado con `App.vue`.
- **PROHIBIDO los `watch` circulares / cruzados**: Nunca vincules dos referencias con `watch` que se modifiquen mutuamente de forma síncrona o asíncrona, ya que provocan bucles infinitos en el hilo de JavaScript y congelan la interfaz.

### 3. Compatibilidad Dual (Wails Desktop + Web / Acceso Remoto)
- El panel funciona tanto dentro de la ventana nativa de escritorio (Wails runtime) como en navegador web local/remoto.
- Siempre mantén los mecanismos de fallback: comprueba `window.go?.main?.App` para llamadas nativas de Wails y ofrece la llamada alternativa mediante las API REST HTTP (`/api/...`) para cuando se accede desde la web o dispositivos remotos.

### 4. Internacionalización (i18n)
- Todas las cadenas visibles para el usuario deben estar registradas en los archivos de traducción (`dashboard/src/i18n/es.js` y `dashboard/src/i18n/en.js`).
- Utiliza siempre la función `t('clave.subclave')` de `useI18n()`.

### 5. Errores Nunca en Silencio
- **Todo `catch` (JS) o manejo de `err` (Go) debe, como mínimo, dejar constancia del error**: `console.error` / `showMessage` en el panel, `log.Printf` o `logbus` en el backend.
- **PROHIBIDO los `catch {}` vacíos y los `_ = err` sin comentario**: un fallo invisible es imposible de diagnosticar. Si hay razón explícita para ignorar un error, coméntala en el código.

### 6. Composables (`use*`) Solo con Estado Reactivo
- Si maneja estado reactivo (`ref`, `reactive`, `computed`), ciclo de vida (`onMounted`, `watch`, …) o recursos por instancia (temporizadores, sockets, suscripciones) → composable en `dashboard/src/composables/` con prefijo `use*`.
- Si es lógica pura sin reactividad (mapas de datos, formato, ayudantes del DOM, `fetch` puros) → módulo simple con exportaciones nombradas en `dashboard/src/utils/`, sin prefijo `use*`.
- **PROHIBIDO mutar props en componentes hijos** (`vue/no-mutating-props`): emite el cambio (p. ej. `update:settings`) y deja que el padre lo aplique. Ligado a la regla 2: el `settings` de `App.vue` se edita por emisión, nunca por asignación directa en el hijo.

### 7. Lint y Formato Obligatorios
- **Go**: código con formato `gofmt`, `go vet ./...` y `go test ./...` en verde antes de dar la tarea por concluida.
- **Frontend**: `npm --prefix dashboard run lint` debe terminar con **0 errores** (ESLint incluye Prettier como regla `prettier/prettier`: un descuadre de formato es un error de lint). Si falla, corrige con `npm --prefix dashboard run format` y verifica con `format:check`; **PROHIBIDO** dejar errores de lint en archivos que hayas tocado.
- **No toques `dashboard/wailsjs/` ni `dashboard/dist/`**: son código generado (excluidos del lint); se regeneran con cada build.

### 8. Idioma: Código en Inglés, Comentarios en Español
- **Todo el código debe estar en inglés**: nombres de variables, funciones, clases CSS, componentes, archivos, claves de i18n y mensajes de log internos. Nada de identificadores en español (`seccionAbierta`, `ajuste-bloque`, `alternar`, …).
- **Los comentarios van en español**: explica el porqué en español, pero sin usar palabras españolas como identificadores dentro del comentario cuando se refieran al código (usa el nombre real en inglés).
- Las cadenas visibles para el usuario siguen la regla 4 (i18n `es.js` / `en.js`); esta regla cubre solo código y comentarios.

### 9. Fuente Completa de Convenciones
- La guía completa para contribuidores humanos vive en `CONTRIBUTING.md`; estas reglas son el subconjunto de obligado cumplimiento para asistentes de IA.

### 10. Uso de emojis
- No uses emojis en ninguna implementación del programa, consola o interfaz, si es necesario un ícono en el dashboard usa la librería instalada y en consola no pongas íconos ni emojis porque puede dar error.

---

## 🛠️ ARQUITECTURA DEL PROYECTO

### Backend (Go)
- **`main.go` / `app.go`**: Punto de entrada de la aplicación de escritorio Wails.
- **`pkg/config/`**: Gestión de configuración, estructura `Config`, normalización, validación de carpetas y base de datos SQLite local.
- **`pkg/downloader/`**: Motor de descargas, descarga por bloques (parallel chunks), control de concurrencia y límites de velocidad.
- **`pkg/server/`**: Servidor REST HTTP e historial. Maneja endpoints como `/api/settings`, `/api/downloads`, `/api/listener/settings`, `/api/auth/...`, etc.

### Frontend (Vue 3 + Vite)
- **`dashboard/src/App.vue`**: Componente raíz del panel. Gestiona el estado reactivo global (`settings`, `downloads`, autenticación), polling periódico y las funciones de autoguardado.
- **`dashboard/src/views/`**:
  - `DownloadsView.vue`: Lista de descargas activas e historial.
  - `ListenerView.vue`: Configuración del escuchador automático de Telegram (chats y filtros).
  - `SettingsView.vue`: Ajustes de velocidad, descargas, carpeta destino, temas e idioma.
  - `LogsView.vue`: Registro de eventos del sistema en vivo.
- **`dashboard/src/components/`**: Componentes auxiliares como `FolderPicker.vue` (navegador nativo y explorador web), `AuthWizard.vue`, `RemoteLogin.vue`, etc.
- **`dashboard/src/i18n/`**: Sistema de internacionalización (español e inglés).

---

## 📋 VERIFICACIONES OBLIGATORIAS ANTES DE DAR UNA TAREA POR CONCLUIDA

1. **Análisis estático de archivos**: Verificar que no existan errores de sintaxis o variables no declaradas en los archivos modificados.
2. **Sin bloqueos en la UI**: Comprobar que no hay bucles en watchers, computados o peticiones HTTP infinitas.
3. **Consistencia visual y funcional**: Asegurar que las opciones editadas persistan en SQLite y se mantengan al recargar la página.
