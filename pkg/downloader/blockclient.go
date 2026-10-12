package downloader

// Control de las peticiones de bloques de archivo a Telegram.
//
// Todo lo que pide bytes de un archivo —el downloader de gotd para una descarga
// nueva y downloadMissingParts para una reanudación— pasa por el mismo sitio:
// el envoltorio de este archivo. Así hay un único punto donde se limita cuántas
// peticiones van en vuelo y donde se atiende el FLOOD_WAIT, en vez de dos
// caminos con comportamientos distintos, que era la razón de que una descarga
// empezara rápida por un lado y siguiera arrastrándose por el otro.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"tgdown/pkg/i18n"
	"tgdown/pkg/logbus"
	"tgdown/pkg/telegram"
)

// maxBloquesEnVuelo es el tope de peticiones de bloque simultáneas de TODO el
// programa, sumando todos los workers de todas las descargas.
//
// Antes no había ninguno: con 6 descargas a la vez y 4 workers cada una se
// pedían 24 bloques al mismo tiempo, y con los topes de la configuración se
// podía llegar a 256. Pedir más de lo que las conexiones pueden servir no
// acelera nada —la cola se forma igual, solo que dentro del cliente— y es la
// forma más segura de que Telegram empiece a contestar FLOOD_WAIT.
//
// Ocho por conexión permite que las descargas con pocos workers trabajen a
// pleno rendimiento mientras se mantiene un límite razonable cuando se usan
// muchas descargas a la vez.
var maxBloquesEnVuelo = telegram.ConexionesDescarga * 8

// bloquesEnVuelo reparte esas ranuras entre todos los workers del programa.
var bloquesEnVuelo = make(chan struct{}, maxBloquesEnVuelo)

func adquirirRanura(ctx context.Context) error {
	select {
	case bloquesEnVuelo <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func liberarRanura() {
	select {
	case <-bloquesEnVuelo:
	default:
	}
}

// intentosPorFlood es cuántas veces se aguanta una pausa de Telegram para la
// misma petición antes de devolver el error hacia arriba.
const intentosPorFlood = 3

// puertaFlood para a TODOS los workers cuando Telegram pide esperar.
//
// El límite de Telegram es por cuenta y método, no por conexión ni por worker.
// Si uno se para y los demás siguen pidiendo, la cuenta sigue castigada y lo
// único que se consigue es que el castigo se renueve: cada worker se lleva su
// propia pausa, se duermen escalonados y la tubería nunca vuelve a llenarse.
// Eso es exactamente lo que se ve como «los workers no trabajan todos juntos».
// Parando todos a la vez, el tiempo se cumple una sola vez y la descarga
// recupera el ritmo completo de golpe.
var (
	puertaFlood struct {
		mu    sync.Mutex
		hasta time.Time
	}
	onFloodStateChange func()
)

// RegisterFloodNotifier registra la función a llamar cuando se activa o extiende un FLOOD_WAIT.
func RegisterFloodNotifier(fn func()) {
	puertaFlood.mu.Lock()
	defer puertaFlood.mu.Unlock()
	onFloodStateChange = fn
}

// GetFloodWaitUntil devuelve el instante hasta el cual Telegram mantendrá el FLOOD_WAIT activo.
func GetFloodWaitUntil() time.Time {
	puertaFlood.mu.Lock()
	defer puertaFlood.mu.Unlock()
	if time.Now().Before(puertaFlood.hasta) {
		return puertaFlood.hasta
	}
	return time.Time{}
}

// SetFloodWait activa una pausa global por FLOOD_WAIT desde cualquier punto de la aplicación.
func SetFloodWait(espera time.Duration) {
	cerrarPuertaFlood(espera)
}

// cerrarPuertaFlood deja a todos los workers parados hasta dentro de espera. Si
// ya había una pausa en curso más larga, se respeta la más larga.
func cerrarPuertaFlood(espera time.Duration) {
	limite := time.Now().Add(espera)
	puertaFlood.mu.Lock()
	changed := false
	if limite.After(puertaFlood.hasta) {
		puertaFlood.hasta = limite
		changed = true
	}
	notifier := onFloodStateChange
	puertaFlood.mu.Unlock()

	if changed && notifier != nil {
		go notifier()
	}
}

// esperarPuertaFlood duerme lo que quede de la pausa global, si hay alguna.
func esperarPuertaFlood(ctx context.Context) error {
	for {
		puertaFlood.mu.Lock()
		hasta := puertaFlood.hasta
		puertaFlood.mu.Unlock()

		restante := time.Until(hasta)
		if restante <= 0 {
			return nil
		}
		if restante > esperaMaximaFlood {
			restante = esperaMaximaFlood
		}

		temporizador := time.NewTimer(restante)
		select {
		case <-ctx.Done():
			temporizador.Stop()
			return ctx.Err()
		case <-temporizador.C:
		}
	}
}

type clienteBloquesState struct {
	mu        sync.RWMutex
	client    *tg.Client
	clientMgr *telegram.ClientManager
}

// clienteBloques es el cliente de Telegram con el control de bloques puesto.
//
// Lleva *tg.Client incrustado a propósito: así hereda tal cual todos los
// métodos que el downloader de gotd necesita (hashes, CDN, archivos web) y solo
// se cambia el que importa, UploadGetFile. Escribir a mano esa lista de métodos
// sería pedir que se rompa la compilación cada vez que gotd toque su interfaz.
type clienteBloques struct {
	*tg.Client
	state *clienteBloquesState
}

// envolverCliente prepara el cliente de bloques. Devuelve el original sin
// envolver si no hay cliente, para no esconder un nil detrás de un struct.
func envolverCliente(cliente *tg.Client, cm *telegram.ClientManager) clienteBloques {
	if cliente == nil {
		return clienteBloques{}
	}
	state := &clienteBloquesState{
		client:    cliente,
		clientMgr: cm,
	}
	return clienteBloques{
		Client: cliente,
		state:  state,
	}
}

func (c clienteBloques) currentClient() *tg.Client {
	if c.state != nil {
		c.state.mu.RLock()
		defer c.state.mu.RUnlock()
		if c.state.client != nil {
			return c.state.client
		}
	}
	return c.Client
}

func (c clienteBloques) setClient(newClient *tg.Client) {
	if c.state != nil {
		c.state.mu.Lock()
		c.state.client = newClient
		c.state.mu.Unlock()
	}
	c.Client = newClient
}

func detectarFileMigrate(err error) int {
	if err == nil {
		return 0
	}
	var rpcErr *tgerr.Error
	if errors.As(err, &rpcErr) && (rpcErr.Code == 303 || rpcErr.Type == "FILE_MIGRATE") {
		if rpcErr.Argument > 0 {
			return rpcErr.Argument
		}
	}
	msg := err.Error()
	if strings.Contains(msg, "FILE_MIGRATE") {
		re := regexp.MustCompile(`FILE_MIGRATE(?:_|\s*\()(\d+)`)
		matches := re.FindStringSubmatch(msg)
		if len(matches) > 1 {
			if dc, convErr := strconv.Atoi(matches[1]); convErr == nil && dc > 0 {
				return dc
			}
		}
	}
	return 0
}

// UploadGetFile pide un bloque respetando la pausa global y la ranura, y
// aguanta el FLOOD_WAIT en vez de dejarlo subir. Además, detecta si el archivo
// reside en otro centro de datos (FILE_MIGRATE) y conmuta la conexión
// transparentemente al DC adecuado.
func (c clienteBloques) UploadGetFile(ctx context.Context, request *tg.UploadGetFileRequest) (tg.UploadFileClass, error) {
	var (
		respuesta tg.UploadFileClass
		err       error
	)

	for intento := 0; intento < intentosPorFlood; intento++ {
		if errEspera := esperarPuertaFlood(ctx); errEspera != nil {
			return nil, errEspera
		}
		if errRanura := adquirirRanura(ctx); errRanura != nil {
			return nil, errRanura
		}

		client := c.currentClient()
		respuesta, err = client.UploadGetFile(ctx, request)
		liberarRanura()

		if err == nil {
			return respuesta, nil
		}

		if targetDC := detectarFileMigrate(err); targetDC > 0 && c.state != nil && c.state.clientMgr != nil {
			logbus.Info(logbus.CatDownloads,
				fmt.Sprintf("Detectada migración a DC %d, reconectando al centro de datos adecuado...", targetDC), "")
			newClient, dcErr := c.state.clientMgr.DownloadClientForDC(ctx, targetDC)
			if dcErr == nil && newClient != nil {
				c.setClient(newClient)
				intento-- // No penalizar el contador de intentos ante migración de DC
				continue
			}
			logbus.Warn(logbus.CatDownloads,
				fmt.Sprintf("Error conectando a DC %d: %v", targetDC, dcErr), "")
		}

		espera, esFlood := esperaPorFlood(err)
		if !esFlood {
			return respuesta, err
		}

		// Se avisa en el registro porque es la única forma de saber desde el
		// panel que una descarga lenta es Telegram frenándonos y no un problema
		// del programa.
		logbus.Warn(logbus.CatDownloads,
			i18n.T("downloads.floodWait", espera),
			i18n.T("downloads.floodWaitDetail",
				request.Offset, intento+1, intentosPorFlood))

		// 500ms extra es margen suficiente para no volver justo al borde. La espera
		// la cumple la puerta al principio de la vuelta siguiente, y la cumplen
		// todos los workers a la vez.
		cerrarPuertaFlood(espera + 500*time.Millisecond)
	}

	return respuesta, err
}

func (c clienteBloques) UploadGetFileHashes(ctx context.Context, request *tg.UploadGetFileHashesRequest) ([]tg.FileHash, error) {
	client := c.currentClient()
	hashes, err := client.UploadGetFileHashes(ctx, request)
	if err != nil {
		if targetDC := detectarFileMigrate(err); targetDC > 0 && c.state != nil && c.state.clientMgr != nil {
			newClient, dcErr := c.state.clientMgr.DownloadClientForDC(ctx, targetDC)
			if dcErr == nil && newClient != nil {
				c.setClient(newClient)
				return newClient.UploadGetFileHashes(ctx, request)
			}
		}
	}
	return hashes, err
}
