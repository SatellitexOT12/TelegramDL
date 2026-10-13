package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"tgdown/pkg/config"
	"tgdown/pkg/downloader"
	"tgdown/pkg/i18n"
	"tgdown/pkg/listener"
	"tgdown/pkg/logbus"
	"tgdown/pkg/server"
	"tgdown/pkg/storage"
	"tgdown/pkg/telegram"
	"tgdown/pkg/updater"
)

type App struct {
	ctx        context.Context
	clientMgr  *telegram.ClientManager
	storage    *storage.Storage
	downloader *downloader.Engine
	listener   *listener.ListenerEngine
	updater    *updater.AppUpdater
	server     *server.Server
	config     config.Config
	assets     fs.FS
	mu         sync.Mutex
	startupURL string // URL pasada desde línea de comandos al iniciar
}

func NewApp(assets fs.FS) *App {
	config.InitPaths()

	// 0. Arrancar el registro antes que nada: a partir de aquí, todo lo que la
	// aplicación escriba con log.Printf acaba también en la vista de logs del
	// panel y en ~/.tgdown/logs/tgdown.log.
	logFileErr := logbus.Init(config.DataDir)
	logbus.Attach()
	logbus.Success(logbus.CatSystem, i18n.T("system.appStarted", config.AppVersion), "")
	if logFileErr != nil {
		logbus.Warn(logbus.CatSystem, i18n.T("system.logFileError"), logFileErr.Error())
	} else if path := logbus.FilePath(); path != "" {
		logbus.Info(logbus.CatSystem, i18n.T("system.logFileActive"), path)
	}

	// 1. Usar base de datos SQLite original de Python (tgdown.sqlite3)
	dbPath := filepath.Join(config.DataDir, "tgdown.sqlite3")
	st, err := storage.NewStorage(dbPath)
	if err != nil {
		logbus.Warn(logbus.CatSystem, i18n.T("system.dbOpenError"), err.Error())
		// Fallback si no se puede abrir tgdown.sqlite3
		dbPath = filepath.Join(config.DataDir, "tgdown.db")
		var errBackup error
		st, errBackup = storage.NewStorage(dbPath)
		if errBackup != nil {
			// Antes el error del respaldo se descartaba, así que se seguía con
			// un puntero nil y la aplicación reventaba en la línea siguiente sin
			// decir por qué. Sin base de datos no hay nada que arrancar.
			logbus.Error(logbus.CatSystem,
				i18n.T("system.dbOpenFatal"),
				errBackup.Error())
			return nil
		}
	}

	// 1.b Volcar a la base de datos lo que quedara en el .env de una versión
	// anterior y fijar desde ahí la dirección de escucha. Tiene que ir antes de
	// cualquier config.GetServerPort/GetServerHost, porque es de donde esas
	// funciones sacan el valor: pkg/config no puede leer la base de datos por sí
	// mismo sin crear un ciclo de imports.
	st.MigrateLegacyEnv()
	bindHost, bindPort := st.ServerBinding()
	config.SetServerBinding(bindHost, bindPort)

	// 2. Cargar configuración previa de SQLite o migrar desde config.json legacy
	cfg, err := st.LoadConfig(config.DefaultConfig(), filepath.Join(config.BaseDir, "config.json"))
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// El idioma del registro manda lo que el usuario eligió en Ajustes. Hasta
	// que se cargó esta línea, los mensajes salían en el idioma detectado del
	// sistema (fijado en main), que es lo único disponible antes de abrir la
	// base de datos.
	if cfg.Language != "" {
		i18n.SetLanguage(cfg.Language)
	}

	cm := telegram.NewClientManager()
	eng := downloader.NewEngine(cm, st, cfg)
	le := listener.NewListenerEngine(cm, st, eng, cfg)
	up := updater.NewAppUpdater()

	app := &App{
		clientMgr:  cm,
		storage:    st,
		downloader: eng,
		listener:   le,
		updater:    up,
		config:     cfg,
		assets:     assets,
	}

	srv := server.NewServer(cm, st, eng, le, up, cfg, assets, func() {
		if ctx := app.context(); ctx != nil {
			wailsRuntime.Quit(ctx)
		}
	})
	app.server = srv

	// Permitir que el panel abra el diálogo nativo de carpetas aunque se esté
	// viendo en un navegador del propio equipo, y no solo dentro de la ventana
	// de la aplicación: así el selector es siempre el mismo en local. El
	// servidor solo atiende esta llamada desde el propio equipo.
	srv.SetFolderPicker(func() (string, error) {
		if app.context() == nil {
			return "", fmt.Errorf("la ventana de la aplicación todavía no está lista")
		}
		return app.SelectDirectory()
	})

	// Registrar en el log todo lo que hagan los motores de descarga y escucha.
	app.attachLogWatchers()

	// Iniciar servidor HTTP/WS INMEDIATAMENTE en el puerto configurado (default 8000)
	port := config.GetServerPort()
	if err := srv.Start(port); err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("server.startError")+": %v\n", port, err)
		logbus.Error(logbus.CatServer, i18n.T("server.startError", port), err.Error())
	} else {
		logbus.Success(logbus.CatServer, i18n.T("server.listening", config.GetServerHost(), port), "")
	}

	// Cargar credenciales desde la base de datos (con fallback a .env) e inicializar cliente
	apiID, apiHash, _ := st.GetCredentials()
	if apiID != "" && apiHash != "" {
		_ = cm.InitClient(apiID, apiHash)
	} else {
		logbus.Warn(logbus.CatTelegram, i18n.T("telegram.noCredentials"), i18n.T("telegram.noCredentialsHint"))
	}

	return app
}

// setContext y context guardan el contexto de Wails detrás del mutex. Lo
// escribe startup (el hilo de la interfaz) y lo leen los observadores de estado
// de los motores de descarga y escucha, que corren en otras goroutines: sin el
// candado eso es una carrera de datos, y el campo mu existía sin usarse.
func (a *App) setContext(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx
}

func (a *App) context() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctx
}

func (a *App) startup(ctx context.Context) {
	a.setContext(ctx)

	// Emitir cambios de estado directamente a la interfaz nativa de Wails
	a.downloader.OnStateChange(func(item storage.DownloadItem) {
		if c := a.context(); c != nil && a.server != nil {
			wailsRuntime.EventsEmit(c, "tgdl:state", a.server.BuildStateSnapshot())
		}
	})
	a.listener.OnStateChange(func(item listener.ListenerItem) {
		if c := a.context(); c != nil && a.server != nil {
			wailsRuntime.EventsEmit(c, "tgdl:state", a.server.BuildStateSnapshot())
		}
	})
}

func (a *App) shutdown(ctx context.Context) {
	logbus.Info(logbus.CatSystem, i18n.T("system.closing"), "")
	if a.clientMgr != nil {
		a.clientMgr.Stop()
	}
	if a.downloader != nil {
		_ = a.downloader.Shutdown(ctx)
	}
	if a.server != nil {
		a.server.Stop()
	}
	if a.storage != nil {
		_ = a.storage.Close()
	}
	logbus.CloseFile()
}

func (a *App) Handler() http.Handler {
	if a.server != nil {
		return a.server.Handler()
	}
	return nil
}

// SelectDirectory abre el diálogo nativo del sistema para seleccionar carpetas
func (a *App) SelectDirectory() (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", fmt.Errorf("la ventana de la aplicación todavía no está lista")
	}
	selected, err := wailsRuntime.OpenDirectoryDialog(ctx, wailsRuntime.OpenDialogOptions{
		Title:            "Seleccionar carpeta de descargas",
		DefaultDirectory: a.config.DownloadFolder,
	})
	if err != nil {
		return "", err
	}
	return selected, nil
}

type ServerInfo struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (a *App) GetServerInfo() ServerInfo {
	return ServerInfo{
		Host: config.GetServerHost(),
		Port: config.GetServerPort(),
	}
}

// PostponeUpdate marca la actualización para no mostrarse más en esta sesión
func (a *App) PostponeUpdate(version string) {
	if a.updater != nil {
		a.updater.Postpone(version)
	}
}

// GetLocalToken devuelve el token de acceso a la API HTTP local a través del
// binding nativo de Wails (IPC interno, nunca por red). Es el mecanismo que
// usa la propia app de escritorio para autenticar sus llamadas a /api/* sin
// que el usuario tenga que escribir nada: solo un cliente remoto (navegador
// en otro dispositivo) necesita introducirlo manualmente una vez.
func (a *App) GetLocalToken() string {
	if a.server == nil {
		return ""
	}
	return a.server.APIToken()
}

// RegenerateLocalToken rota el token de acceso a la API. Cualquier
// dispositivo remoto que use el token anterior deberá reconfigurarse con el
// nuevo valor devuelto aquí.
func (a *App) RegenerateLocalToken() string {
	if a.server == nil {
		return ""
	}
	return a.server.RegenerateToken()
}

// GetStartupURL devuelve la URL pasada desde línea de comandos al iniciar y la limpia.
func (a *App) GetStartupURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	url := a.startupURL
	a.startupURL = ""
	return url
}

// SetStartupURL establece la URL de inicio.
func (a *App) SetStartupURL(url string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.startupURL = url
}
