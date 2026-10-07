// Utilidad pura (sin reactividad, por eso vive en utils/ y no en un composable
// con prefijo use*: regla 6 de AGENTS.md) para decidir si una dirección es de
// loopback. Solo loopback significa "accesible únicamente desde este equipo":
// cualquier otra cosa (0.0.0.0, IPs de LAN, nombres desconocidos) significa que
// el panel queda expuesto a la red local sin cifrar (SEC-05).

// Devuelve true para `localhost`, `127.0.0.1`, `::1` y cualquier 127.x.x.x,
// también cuando traen puerto o espacios sobrantes ("127.0.0.1:8080",
// " ::1 "). Devuelve false para cadena vacía, 0.0.0.0, ::, IPs de LAN y
// cualquier nombre desconocido.
export const isLoopbackHost = (host) => {
  if (typeof host !== 'string') return false
  let value = host.trim().toLowerCase()
  if (!value) return false

  // Forma IPv6 con corchetes y puerto opcional: [::1]:8080 → ::1.
  const bracketed = value.match(/^\[([^\]]+)\]/)
  if (bracketed) {
    value = bracketed[1]
  } else {
    // Puerto final solo cuando hay un único ":" (127.0.0.1:8080 o
    // localhost:8080); con varios ":" es IPv6 y el último grupo de dígitos
    // forma parte de la dirección, no un puerto.
    const ported = value.match(/^(.+):(\d+)$/)
    if (ported && !ported[1].includes(':')) value = ported[1]
  }

  // Nombres e IPv6 habituales de loopback (incluida la forma expandida de ::1).
  const loopbackNames = ['localhost', '::1', '0:0:0:0:0:0:0:1']
  if (loopbackNames.includes(value)) return true

  // En IPv4 solo decide el primer cuarteto: 127.x.x.x es loopback; 0.0.0.0 y
  // cualquier IP de LAN no lo son. Un valor no numérico (p. ej. un hostname
  // desconocido) no supera la comprobación y cae al false final.
  const quads = value.split('.')
  if (quads.length === 4 && quads.every((q) => /^\d{1,3}$/.test(q)))
    return Number(quads[0]) === 127

  return false
}
