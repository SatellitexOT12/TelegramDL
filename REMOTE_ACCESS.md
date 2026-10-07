# Acceso remoto a TelegramDL

TelegramDL expone un servidor HTTP/WebSocket que el panel usa tanto dentro de
la ventana de escritorio como si lo abres en un navegador. En las instalaciones
nuevas escucha por defecto en `127.0.0.1:8000` (solo el propio equipo); las
instalaciones existentes conservan la dirección que ya tuvieran guardada y
pueden cambiarla cuando quieran.

Casi toda la API (`/api/*`) exige un **token de acceso** (Ajustes → Acceso
remoto, dentro de la app), así que nadie puede controlar tus descargas ni leer
tus credenciales de Telegram sin ese token; el WebSocket lo pide igualmente
(viaja en la cabecera `Sec-WebSocket-Protocol`, como `tgdl-v1, <token>`). Las
únicas excepciones son:

- cuatro endpoints públicos: `/api/theme`, `/api/auth/has-credentials` y
  `/api/auth/token/send` (estos dos últimos con sus propios límites de
  frecuencia) más `/api/ipc/download` (solo desde el propio equipo, con las
  cabeceras `X-TGDL-Request: 1` y `Content-Type: application/json`);
- la primera configuración de Telegram (`/api/auth/status`, `credentials`,
  `send-code`, `verify-code`, `verify-2fa` y `logout`), que responde sin
  token **solo mientras no exista ninguna sesión de Telegram guardada** y
  exige además `POST` (`status` admite `GET`), cabecera `X-TGDL-Request: 1`,
  `Content-Type: application/json` y un límite de 5 peticiones/minuto por
  origen y 30/min global (el `429` lleva `Retry-After`). Con sesión guardada,
  responde `401`.

## Acceso desde tu propia red (WiFi de casa)

`0.0.0.0` significa que el panel atiende en todas las interfaces del equipo, no
solo en la de loopback. Es lo que permite abrirlo desde el móvil o desde otro
ordenador de la casa escribiendo `http://<ip-local-de-la-pc>:8000`; la primera
vez te pedirá el token.

La dirección de escucha vive en la configuración de la aplicación (clave
`bind_host`; lo que hubiera en el antiguo `~/.tgdown/.env` se migra allí y el
fichero se aparta). En las instalaciones nuevas es `127.0.0.1` por defecto, y
las existentes conservan el valor que ya tuvieran. Para atender en todas las
interfaces, cambia esa clave a `0.0.0.0` (o define la variable de entorno
`TGDL_BIND_HOST=0.0.0.0`, que manda sobre el valor guardado) y reinicia. La
aplicación nunca sobrescribe ese valor una vez elegido.

Ten en cuenta que con `0.0.0.0` el puerto queda visible para **cualquier
dispositivo conectado a tu red local**, no solo para los tuyos. En un WiFi
doméstico es razonable; en una red compartida (residencia, oficina, WiFi
público) conviene usar `127.0.0.1`.

## Sin TLS propio (decisión tomada)

TelegramDL **no implementa TLS propio**: es una decisión tomada, no un
descuido pendiente. El servidor atiende por HTTP y `ws://`, así que el tráfico
—token, credenciales de API y contraseña 2FA— viaja **en claro** por la red
por la que viaje: el token autentica, pero no cifra nada.

Con `127.0.0.1` eso no importa, porque el tráfico no sale del equipo. En
cuanto el panel se abre en la red local o desde fuera, quien pueda observar
ese tráfico (por ejemplo, la misma WiFi) lo lee entero. Por eso la
recomendación es escuchar en `127.0.0.1` y, para llegar desde otro equipo,
hacerlo por una red ya cifrada: [Tailscale](#opción-recomendada-tailscale-vpn-de-malla)
(u otro túnel equivalente).

Si arrancas con una dirección de escucha que **no** es de loopback (p. ej.
`0.0.0.0`), la aplicación lo avisa: aparece una advertencia en el registro al
arrancar y otro aviso visible en **Ajustes** con enlace a este documento.

## Acceso desde fuera de casa

Eso resuelve la red local, pero **no** cómo llegar al servidor desde internet.
Para eso, evita a toda costa abrir el puerto 8000 al exterior con
port-forwarding en el router: un puerto así recibe tráfico de escáneres
automáticos en cuestión de minutos. En su lugar, usa una de estas dos opciones.

## Opción recomendada: Tailscale (VPN de malla)

[Tailscale](https://tailscale.com) crea una red privada cifrada entre tus
dispositivos sin abrir ningún puerto en el router.

1. Instala Tailscale en la PC donde corre TelegramDL y en tu celular (o
   cualquier otro dispositivo desde el que quieras controlarlo).
2. Inicia sesión con la misma cuenta en ambos.
3. En la PC, revisa la IP de Tailscale asignada (suele ser `100.x.y.z`);
   Tailscale la muestra en su propio panel/bandeja del sistema.
4. Desde el celular (con Tailscale activo), abre en el navegador:
   `http://100.x.y.z:8000`
5. La primera vez te pedirá el token de acceso: ábrelo en la app de
   escritorio en **Ajustes → Acceso remoto**, cópialo y pégalo. El
   navegador lo recordará para la próxima vez.

Alternativas equivalentes si prefieres no depender de la infraestructura de
Tailscale: **WireGuard** (autogestionado) o **ZeroTier**.

## Alternativa: túnel público (Cloudflare Tunnel)

Si prefieres una URL pública en vez de instalar un cliente VPN en cada
dispositivo:

1. Instala `cloudflared` en la PC con TelegramDL.
2. Crea un túnel apuntando a `http://127.0.0.1:8000`.
3. En el dashboard de Cloudflare, activa **Cloudflare Access** sobre ese
   túnel y restringe el acceso a tu email (login por código de un solo uso)
   u otro proveedor de identidad.
4. Accede desde cualquier dispositivo a la URL pública que te da Cloudflare
   y, la primera vez, pega el token de acceso de la app.

Esto añade una capa de autenticación *antes* de que la petición llegue
siquiera a TelegramDL, además del token propio de la app.

## Qué hace el token y cómo se gestiona

- Se genera automáticamente la primera vez que arranca la app; no hace
  falta configurar nada para seguir usándola normalmente en el mismo
  equipo (la propia ventana de escritorio lo obtiene por su cuenta).
- Se ve y se puede copiar desde **Ajustes → Acceso remoto**.
- Si estás fuera de casa y no lo llevas encima, la propia pantalla de acceso
  remoto tiene un botón **Enviármelo a Telegram**: deja el token en tus
  *Mensajes guardados*, donde solo tú puedes leerlo. Ábrelo en Telegram, toca
  el token para copiarlo y pégalo en el panel.
- Si sospechas que se filtró (por ejemplo, lo compartiste sin querer),
  pulsa **Regenerar token** ahí mismo: el anterior deja de funcionar de
  inmediato y tendrás que volver a introducir el nuevo en cada dispositivo
  remoto.
- Nunca lo compartas por canales inseguros (chat sin cifrar, capturas de
  pantalla públicas, etc.): quien lo tenga puede controlar la app por
  completo mientras siga siendo válido.

### El botón «Enviármelo a Telegram»

Ese botón es uno de los pocos endpoints de la API que responden **sin
token** —junto al color del panel, la consulta de credenciales y la descarga
IPC (esta última solo desde el propio equipo)—: no puede exigirlo, porque el
token es justo lo que le falta a quien lo pulsa. Lo que sí hace es no devolver
nada: el token viaja al chat privado de tu cuenta de Telegram, no a la
respuesta HTTP.

En la práctica, lo máximo que puede conseguir alguien que alcance el puerto es
provocar un mensaje en tus *Mensajes guardados*. Para que eso no se convierta
en una molestia, el servidor solo admite **un envío por minuto** y anota en el
registro de actividad cada intento con la dirección desde la que llegó.

Contrapartida a tener en cuenta: el token queda guardado en tu historial de
Telegram. Si prefieres que no sea así, bórralo del chat después de usarlo, o
regenera el token cuando termines.

## Qué NO hacer

- No abras el puerto 8000 en el router (port-forwarding) para "simplificar"
  el acceso desde fuera de casa. Aunque el token protege casi toda la API,
  estarías exponiendo el servicio a escaneos e intentos de fuerza bruta de
  todo internet sin necesidad, teniendo alternativas mejores arriba.
- No dejes `TGDL_BIND_HOST=0.0.0.0` si el equipo está en una red que no
  controlas (WiFi público, residencia, oficina compartida): ahí cualquiera
  conectado a esa red ve el puerto. Usa `127.0.0.1` en esos casos.
- No compartas el token por el mismo canal que uses para compartir el
  enlace/IP de acceso (si uno se filtra, que el otro no se filtre con él).
