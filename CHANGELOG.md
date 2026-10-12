# Historial de Cambios (Changelog)

Todas las novedades, mejoras y correcciones de **tgdown** (Telegram DL) organizadas rigurosamente por versión desde la v2.0.6 hasta la v2.5.9.

---

## [2.5.9]

### 🚀 Novedades

- **Modo Consola (CLI) Completo:** Se añadió el comando `--download` para permitir descargar archivos directamente desde la terminal de comandos sin necesidad de abrir la interfaz gráfica.
- **Traducción en Consola:** Soporte multilenguaje (Español/Inglés) aplicado al modo consola.
- **Documentación Actualizada:** Revisión y actualización completa de la guía de uso y del archivo `README.md`.

### 🛠️ Correcciones

- Solucionados errores al ingresar y procesar enlaces directos desde la terminal.

---

## [2.5.8]

### 🚀 Novedades

- **Agregar descarga desde consola:** Se añadió la opción de agregar un enlace desde la consola y si el programa está abierot lo añade a la cola si está cerrado abre el programa y comienza la descarga.

### 🛠️ Correcciones

- Integración de mejoras internas de estabilidad y corrección de advertencias en el código de la interfaz.

---

## [2.5.7]

### 🛠️ Correcciones

- Sincronización general del código base para mejorar la respuesta de la aplicación ante múltiples peticiones.

---

## [2.5.6]

### 🚀 Novedades

- **Soporte para archivos en migración:** Se añadió soporte para archivos que están en otros datacenters de telegram y fallaban al descargar al estar en movimiento de un datacenter a otro.

---

## [2.5.5]

### 🚀 Novedades

- **Soporte Idiomas (i18n):** Se implementó el cambio de idioma completo (Español e Inglés) tanto en la pantalla (frontend) como en el servidor interno (backend).
- **Desactivar Sonido:** Nueva opción en la configuración para apagar la alerta sonora que suena cuando se termina una descarga.
- **Ajustes de Nombre en Escucha:** Ahora es posible configurar y guardar el formato de nombre para los archivos de la escucha automática de forma persistente.

### 🛠️ Correcciones

- Se corrigió el comportamiento de los conectores de descarga (workers) cuando se pausan o reanudan tareas.

---

## [2.5.4]

### 🛠️ Correcciones y Diseño

- Se solucionaron errores visuales en la barra lateral (Sidebar) con la distribución del botón de apagado.
- Correcciones en el refresco de datos en tiempo real dentro de la vista principal.

---

## [2.5.3]

### 🛠️ Correcciones

- Se corrigió un fallo por el cual algunos archivos descargados automáticamente mediante la escucha perdían su nombre original al guardarse.
- Corrección en la lectura de preferencias de nombres por cada chat o canal.

---

## [2.5.2]

### 🚀 Novedades

- **Apagado Automático Programado:** Nueva función que permite apagar el equipo automáticamente cuando se completen todas las descargas en cola.

---

## [2.5.1]

### 🚀 Novedades

- **Cambio de Nombre Masivo:** Botón y herramienta para renombrar varios archivos descargados al mismo tiempo.
- Selector para elegir la regla de nombrado que tendrá la escucha automática en canales específicos.

---

## [2.5.0]

### 🚀 Novedades

- **Documentación Integrada:** Lanzamiento del nuevo portal de documentación del proyecto.

### 🛠️ Correcciones

- Solucionados los problemas para iniciar sesión y autenticarse mediante token de acceso remoto.
- Correcciones internas en el sistema de almacenamiento en base de datos (`storage.go`) para evitar cierres inesperados.
- Corrección en la validación de texto para enlaces permitidos.

---

## [2.4.9]

### 🚀 Novedades

- **Organización por Chats:** En la vista de Escucha Automática (Listener), ahora puedes filtrar y organizar los archivos detectados agrupándolos por el chat o canal de origen.
- **Ajustes Módulares:** La pantalla de Configuración se reestructuró en acordeones desplegables para hacerla más limpia e intuitiva.

### 🛠️ Correcciones

- Se arregló una pérdida de velocidad de descarga que ocurría al repartir tareas entre varios conectores de red (workers).

---

## [2.4.8]

### 🛠️ Correcciones

- Se corrigieron los mensajes emergentes de notificación que avisas cuando una descarga finaliza con éxito.
- Retoques en las hojas de estilo globales para mejorar el contraste de texto.

---

## [2.4.7]

### 🎨 Estilo y Apariencia

- **Nuevo Tema:** Se añadió el tema de colores **Herbal Apothecary** (tonos verdosos y relajantes).
- Se corrigieron detalles de visibilidad y color en el tema **Silent Cherry**.

---

## [2.4.6]

### 🚀 Novedades

- **Tiempo Estimado (ETA):** Ahora la aplicación calcula y muestra cuántos minutos y segundos faltan para que finalice cada descarga activa.

---

## [2.4.5]

### 🚀 Novedades

- **Paginador en Historial:** Ahora puedes avanzar por páginas en la lista de descargas pasadas y elegir cuántos elementos ver a la vez (10, 25, 50, etc.).
- **Indicador Animado:** El icono de carga (loader) ahora adopta el color del tema que tengas seleccionado.

### 🛠️ Correcciones

- Arreglado un error que provocaba que una descarga fallara si el mensaje original era borrado del chat en Telegram.
- Correcciones en el orden de la cola de descargas para evitar bloqueos.

---

## [2.4.4]

### 🎨 Estilo y Apariencia

- Cuatro nuevos temas visuales disponibles: **Edge Runner UI**, **Rainy Garden**, **Silent Cherry** y **osX**.
- Nuevas combinaciones de colores de acento para adaptar la app a tu estilo preferido.

---

## [2.4.3]

### 🛠️ Correcciones

- Se mejoró la compatibilidad al hacer clic en "Abrir archivo" para que el sistema operativo abra el archivo con su programa correspondiente.

---

## [2.4.2]

### 🛠️ Correcciones

- Pruebas internas de envío y validación de seguridad de tokens actualizadas.

---

## [2.4.1]

### 🛠️ Correcciones

- **Migración a Base de Datos:** Tu configuración antigua almacenada en archivos se traslada automáticamente a la base de datos interna SQLite.
- **Navegación Web Externa:** Los enlaces a páginas de internet externas ahora se abren en tu navegador web habitual (Chrome, Edge, Firefox) y no dentro del programa.
- Se mejoró la rutina de actualización automática del sistema en computadoras Mac.
- Se eliminó el resaltado involuntario de texto al hacer clic rápido en la ventana.

---

## [2.4.0]

### 🚀 Novedades

- **Soporte para Grupos con Temas/Foros:** Ahora puedes escuchar y descargar archivos de súpergrupos de Telegram divididos en subtemas o foros.
- **Token por Telegram:** Botón directo en el bot de Telegram para obtener la clave de acceso remoto con un solo toque.

### 🛠️ Correcciones

- Arreglados errores de alineación en las pantallas de inicio de sesión.

---

## [2.3.9]

### 🎨 Estilo y Apariencia

- Renovación completa de los íconos de la interfaz utilizando la librería moderna **Hugeicons**.

---

## [2.3.8]

### 🚀 Novedades

- **Consola de Registro en Vivo (Logs):** Nueva pestaña dentro de la app para monitorear en tiempo real todas las acciones, conexiones y eventos del sistema.

### 🛠️ Correcciones

- Se eliminó el destello o parpadeo blanco que aparecía al abrir la ventana de inicio de sesión remoto.
- Corrección en las rutas internas de autenticación web.
- Se corrigió el explorador web de carpetas para que no falle al elegir una ruta de descarga.

---

## [2.3.7]

### 🛠️ Correcciones

- Corrección en la prioridad y orden de la cola de descargas automáticas.

---

## [2.3.6]

### 🚀 Novedades

- **Importar/Exportar escucha:** Opción en ajustes para importar y exportar los ajustes de la escucha.ç
- **Token de acceso remoto:** Se agrego un token de acceso remoto para evitar que cualquiera pudiera abrir mediante navegador la aplicación y tener control sobre ella.

### 🛠️ Correcciones

- Adaptación del diseño de la pantalla de Ajustes para que se visualice correctamente en monitores pequeños o celulares.
- Correcciones de pruebas unitarias internas.

---

## [2.0.6] a [2.3.5]

### 🚀 Funciones Base del Proyecto

- **Descargas Multi-bloque (Parallel Chunks):** División de archivos grandes en partes simultáneas para maximizar la velocidad de descarga.
- **Escuchador Automático (Listener):** Detección automática de enlaces y archivos adjuntos en chats de Telegram con filtros por tipo de archivo y extensiones.
- **Acceso Remoto por Web:** Panel reactivo en Vue 3 que funciona como aplicación de escritorio y como página web accesible desde red local o Tailscale.
- **Almacenamiento Local SQLite:** Base de datos ligera para mantener tus preferencias, colas de descarga e historial sin depender de servidores externos.
