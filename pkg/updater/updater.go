package updater

// Comprobación e instalación de actualizaciones.
//
// Aquí no se usa ninguna librería de terceros a propósito. Lo que hace falta es
// una petición a la API de releases de GitHub, comparar dos números de versión y
// cambiar de sitio un archivo; las tres cosas están en la biblioteca estándar.
// Antes esto lo hacían github.com/creativeprojects/go-selfupdate y
// github.com/minio/selfupdate, que entre las dos arrastraban los SDK de GitHub,
// GitLab y Gitea, OAuth2 y media docena de módulos más para, en la práctica,
// una llamada HTTP y un os.Rename.

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"tgdown/pkg/config"
	"tgdown/pkg/i18n"
	"tgdown/pkg/logbus"
)

type Progress struct {
	Status     string `json:"status"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Percentage int    `json:"percentage"`
}

type ReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

type ReleaseInfo struct {
	TagName string         `json:"tag_name"`
	Body    string         `json:"body"`
	Assets  []ReleaseAsset `json:"assets"`

	// asset es el archivo que corresponde a este sistema, ya elegido durante la
	// comprobación. InstallUpdate no vuelve a buscarlo.
	asset ReleaseAsset
}

type AppUpdater struct {
	currentVersion  string
	repoURL         string
	mu              sync.RWMutex
	progress        Progress
	postponedUpdate string

	// Estado para no repetir en el registro lo mismo cada 5 minutos: la
	// comprobación periódica solo se narra la primera vez (al abrir la app) y
	// cuando el resultado cambia de verdad.
	checkCount       int
	announcedVersion string
	lastCheckError   string

	// rateLimitUntil marca hasta cuándo GitHub nos tiene cortados por exceso de
	// peticiones. Mientras no pase esa hora no tiene sentido volver a preguntar.
	rateLimitUntil time.Time
}

// errRateLimited distingue el "GitHub no nos deja preguntar ahora mismo" de un
// fallo de verdad, para no llenar el registro de avisos por algo que se arregla
// solo con el tiempo.
var errRateLimited = errors.New("GitHub rate limited update requests")

const (
	githubAPI = "https://api.github.com"

	// noDataTimeout es cuánto se tolera que una descarga no reciba un solo
	// byte antes de darla por colgada. Sin esto, una conexión que se queda a
	// medias dejaba la actualización en "descargando" para siempre.
	noDataTimeout = 60 * time.Second
)

// updaterTransport acota cada fase de la conexión por separado. Un
// Timeout global del cliente no sirve para la descarga del instalador: cortaría
// un archivo grande por una línea lenta aunque estuviera avanzando bien.
var updaterTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ForceAttemptHTTP2:     true,
}

// apiClient es para las consultas cortas (la release y el checksums.txt).
var apiClient = &http.Client{
	Transport: updaterTransport,
	Timeout:   30 * time.Second,
}

// downloadClient no lleva Timeout global: la descarga se vigila por
// inactividad dentro de downloadWithProgress.
var downloadClient = &http.Client{Transport: updaterTransport}

// userAgent identifica a la aplicación. La API de GitHub rechaza con 403
// cualquier petición que no lo lleve.
func userAgent() string {
	return "TelegramDL/" + config.AppVersion + " (+https://github.com/" + config.GithubRepo + ")"
}

func NewAppUpdater() *AppUpdater {
	config.InitPaths()
	u := &AppUpdater{
		currentVersion: config.AppVersion,
		repoURL:        config.GithubRepo,
		progress: Progress{
			Status: "idle",
		},
	}
	u.CleanOldVersion()
	return u
}

// CleanOldVersion busca y elimina archivos temporales de actualizaciones previas (.old)
func (u *AppUpdater) CleanOldVersion() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	dir := filepath.Dir(exePath)
	base := filepath.Base(exePath)

	// Se contemplan las dos formas en que ha podido quedar la versión anterior:
	// la que deja aplicarBinario y la variante oculta con punto delante.
	targets := []string{
		exePath + ".old",
		filepath.Join(dir, "."+base+".old"),
	}

	for _, target := range targets {
		if _, err := os.Stat(target); err == nil {
			log.Printf("[UPDATER] %s", i18n.T("updater.oldFileFound", target))
			// En Windows, intentamos eliminarlo. os.Remove funciona incluso con atributo oculto.
			if err := os.Remove(target); err != nil {
				log.Printf("[UPDATER] %s", i18n.T("updater.oldFileError", target, err))
			} else {
				log.Printf("[UPDATER] %s", i18n.T("updater.oldFileRemoved", target))
			}
		}
	}

	// En macOS lo que queda atrás no es un archivo .old sino un bundle .app.old.
	u.cleanPlatform()
}

func (u *AppUpdater) GetProgress() Progress {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.progress
}

func (u *AppUpdater) setProgress(status string, downloaded, total int64, pct int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.progress = Progress{
		Status:     status,
		Downloaded: downloaded,
		Total:      total,
		Percentage: pct,
	}
}

// Postpone marca una versión específica para no volver a notificar en esta sesión
func (u *AppUpdater) Postpone(version string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.postponedUpdate = version
}

// IsPostponed verifica si una versión ya fue pospuesta
func (u *AppUpdater) IsPostponed(version string) bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.postponedUpdate == version
}

// routineLog registra un paso rutinario de la comprobación de actualizaciones:
// visible la primera vez (al abrir la aplicación) y en nivel detalle en las
// comprobaciones periódicas posteriores, que se repiten cada pocos minutos.
func (u *AppUpdater) routineLog(first bool, message, detail string) {
	if first {
		logbus.Info(logbus.CatUpdater, message, detail)
	} else {
		logbus.Debug(logbus.CatUpdater, message, detail)
	}
}

// checkErrorLog evita repetir el mismo fallo: avisa la primera vez y cada vez
// que el error cambia de verdad, y el resto queda en nivel detalle.
func (u *AppUpdater) checkErrorLog(first bool, message string, err error) {
	text := ""
	if err != nil {
		text = err.Error()
	}

	u.mu.Lock()
	changed := u.lastCheckError != text
	u.lastCheckError = text
	u.mu.Unlock()

	if first || changed {
		logbus.Warn(logbus.CatUpdater, message, text)
		return
	}
	logbus.Debug(logbus.CatUpdater, message, text)
}

// ---------------------------------------------------------------------------
// Consulta a la API de GitHub
// ---------------------------------------------------------------------------

// releaseGitHub es la parte de la respuesta de
// /repos/{owner}/{repo}/releases/latest que nos interesa. El resto de campos se
// descartan solos al decodificar.
type releaseGitHub struct {
	TagName    string        `json:"tag_name"`
	Name       string        `json:"name"`
	Body       string        `json:"body"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []assetGitHub `json:"assets"`
}

type assetGitHub struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// recordRateLimit apunta hasta cuándo no merece la pena volver a preguntar
// cuando GitHub responde que se agotó la cuota de peticiones por hora. El propio
// GitHub lo dice en las cabeceras, así que no hay que adivinar nada.
func (u *AppUpdater) recordRateLimit(resp *http.Response) bool {
	retryAfter := resp.Header.Get("Retry-After")
	exhausted := resp.Header.Get("X-RateLimit-Remaining") == "0"
	if !exhausted && retryAfter == "" {
		return false
	}

	wait := 15 * time.Minute
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
			wait = time.Duration(seconds) * time.Second
		}
	} else if reset := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); reset != "" {
		if epoch, err := strconv.ParseInt(reset, 10, 64); err == nil {
			if remaining := time.Until(time.Unix(epoch, 0)); remaining > 0 {
				wait = remaining
			}
		}
	}
	wait += 30 * time.Second // un margen por si el reloj va justo

	u.mu.Lock()
	u.rateLimitUntil = time.Now().Add(wait)
	u.mu.Unlock()

	logbus.Warn(logbus.CatUpdater,
		i18n.T("updater.rateLimited"),
		i18n.T("updater.rateLimitedDetail", wait.Round(time.Second)))
	return true
}

// fetchLatestRelease pide a GitHub la última versión publicada. Devuelve
// (nil, nil) cuando el repositorio todavía no tiene ninguna.
func (u *AppUpdater) fetchLatestRelease(ctx context.Context) (*releaseGitHub, error) {
	slug := strings.Trim(strings.TrimSpace(u.repoURL), "/")
	if slug == "" || !strings.Contains(slug, "/") {
		return nil, fmt.Errorf(i18n.T("updater.repoMisconfigured"), u.repoURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/repos/%s/releases/latest", githubAPI, slug), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent())

	resp, err := apiClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		// El repositorio existe pero no ha publicado ninguna versión.
		return nil, nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		if u.recordRateLimit(resp) {
			return nil, errRateLimited
		}
		return nil, fmt.Errorf(i18n.T("updater.githubRejected"), resp.StatusCode)
	default:
		return nil, fmt.Errorf(i18n.T("updater.githubResponseCode"), resp.StatusCode)
	}

	// Una respuesta legítima son unos pocos kilobytes; el límite evita que una
	// respuesta enorme agote la memoria.
	var rel releaseGitHub
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf(i18n.T("updater.githubReadError"), err)
	}
	return &rel, nil
}

// nonExecutableExtensions son archivos que acompañan a la release pero no son
// la aplicación: sumas de verificación, firmas y notas.
var nonExecutableExtensions = map[string]bool{
	".txt": true, ".md": true, ".sig": true, ".minisig": true,
	".asc": true, ".sha256": true, ".sha512": true, ".pem": true,
}

// systemKeywords son los trozos de nombre con los que se publica el archivo
// de cada plataforma.
func systemKeywords() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"windows", "win", ".exe"}
	case "darwin":
		return []string{"darwin", "macos", "mac", "osx"}
	case "linux":
		return []string{"linux"}
	default:
		return []string{runtime.GOOS}
	}
}

// archKeywords permite preferir el archivo de la arquitectura correcta
// cuando la release publica varios para el mismo sistema.
func archKeywords() []string {
	switch runtime.GOARCH {
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	case "386":
		return []string{"386", "i386", "x86"}
	default:
		return []string{runtime.GOARCH}
	}
}

func containsAnyKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// assetForThisSystem elige de la release el archivo que corresponde a esta
// plataforma. Si hay varios del mismo sistema, gana el que además coincide en
// arquitectura.
func assetForThisSystem(assets []assetGitHub) *ReleaseAsset {
	system := systemKeywords()
	architecture := archKeywords()

	var candidate *assetGitHub
	for i := range assets {
		asset := &assets[i]
		if asset.URL == "" {
			continue
		}
		name := strings.ToLower(asset.Name)
		if nonExecutableExtensions[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		if !containsAnyKeyword(name, system) {
			continue
		}
		if containsAnyKeyword(name, architecture) {
			candidate = asset
			break
		}
		if candidate == nil {
			candidate = asset
		}
	}

	if candidate == nil {
		return nil
	}
	return &ReleaseAsset{
		Name:        candidate.Name,
		DownloadURL: candidate.URL,
		Size:        candidate.Size,
	}
}

// ---------------------------------------------------------------------------
// Comparación de versiones
//
// Basta con el orden semántico habitual (mayor.menor.parche) y con que una
// versión de prueba (2.5.0-rc1) valga menos que la definitiva (2.5.0). No hace
// falta una librería para eso.
// ---------------------------------------------------------------------------

// numericPrefix devuelve los dígitos del principio de una cadena, para que un
// "4a" o un "3 " no tumben la comparación entera.
func numericPrefix(s string) int {
	fin := 0
	for fin < len(s) && s[fin] >= '0' && s[fin] <= '9' {
		fin++
	}
	if fin == 0 {
		return 0
	}
	n, err := strconv.Atoi(s[:fin])
	if err != nil {
		return 0
	}
	return n
}

// normalizeVersion parte "v2.5.0-rc1+build" en [2 5 0] y "rc1".
func normalizeVersion(v string) (numbers [3]int, preRelease string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		preRelease = v[i+1:]
		v = v[:i]
	}

	for i, part := range strings.SplitN(v, ".", 3) {
		numbers[i] = numericPrefix(strings.TrimSpace(part))
	}
	return numbers, preRelease
}

// compareVersions devuelve -1 si a es anterior a b, 1 si es posterior y 0 si
// son la misma.
func compareVersions(a, b string) int {
	na, preReleaseA := normalizeVersion(a)
	nb, preReleaseB := normalizeVersion(b)

	for i := 0; i < len(na); i++ {
		if na[i] != nb[i] {
			if na[i] < nb[i] {
				return -1
			}
			return 1
		}
	}

	switch {
	case preReleaseA == "" && preReleaseB == "":
		return 0
	case preReleaseA == "":
		// 2.5.0 es posterior a 2.5.0-rc1.
		return 1
	case preReleaseB == "":
		return -1
	}
	return strings.Compare(preReleaseA, preReleaseB)
}

// ---------------------------------------------------------------------------
// Comprobación
// ---------------------------------------------------------------------------

func (u *AppUpdater) CheckForUpdate() (*ReleaseInfo, *ReleaseAsset, error) {
	u.mu.Lock()
	u.checkCount++
	first := u.checkCount == 1
	pausedUntil := u.rateLimitUntil
	u.mu.Unlock()

	// Si GitHub ya nos dijo que agotamos la cuota, esperamos a que pase el
	// tiempo que él mismo indicó en vez de seguir insistiendo (y llenando el
	// registro) cada pocos minutos.
	if !pausedUntil.IsZero() && time.Now().Before(pausedUntil) {
		logbus.Debug(logbus.CatUpdater, i18n.T("updater.checkPaused"),
			i18n.T("updater.checkPausedDetail", time.Until(pausedUntil).Round(time.Second)))
		return nil, nil, nil
	}

	u.routineLog(first, i18n.T("updater.checking", u.repoURL),
		i18n.T("updater.checkingDetail", u.currentVersion))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	rel, err := u.fetchLatestRelease(ctx)
	if err != nil {
		// El aviso de cuota agotada ya lo dio recordRateLimit; repetirlo
		// aquí solo duplicaría la línea en el registro.
		if !errors.Is(err, errRateLimited) {
			u.checkErrorLog(first, i18n.T("updater.checkError"), err)
		}
		return nil, nil, err
	}
	if rel == nil || rel.Draft || strings.TrimSpace(rel.TagName) == "" {
		u.routineLog(first, i18n.T("updater.noReleases"), "")
		return nil, nil, nil
	}

	asset := assetForThisSystem(rel.Assets)
	if asset == nil {
		u.routineLog(first, i18n.T("updater.noAsset"),
			i18n.T("updater.noAssetDetail", rel.TagName, len(rel.Assets)))
		return nil, nil, nil
	}

	u.routineLog(first, i18n.T("updater.latestRelease", rel.TagName),
		i18n.T("updater.latestReleaseDetail", asset.Name))

	if compareVersions(rel.TagName, u.currentVersion) <= 0 {
		u.routineLog(first, i18n.T("updater.upToDate", u.currentVersion), "")
		return nil, nil, nil
	}

	// La novedad se anuncia una sola vez por versión: la comprobación se repite
	// cada pocos minutos y no tiene sentido repetir el aviso en cada ronda.
	u.mu.Lock()
	alreadyAnnounced := u.announcedVersion == rel.TagName
	u.announcedVersion = rel.TagName
	u.mu.Unlock()

	if alreadyAnnounced {
		logbus.Debug(logbus.CatUpdater,
			i18n.T("updater.stillAvailable", rel.TagName), "")
	} else {
		logbus.Success(logbus.CatUpdater,
			i18n.T("updater.newVersion", rel.TagName),
			i18n.T("updater.newVersionDetail", u.currentVersion))
	}

	info := &ReleaseInfo{
		TagName: rel.TagName,
		Body:    rel.Body,
		Assets:  []ReleaseAsset{*asset},
		asset:   *asset,
	}
	return info, &info.Assets[0], nil
}

// ---------------------------------------------------------------------------
// Instalación
// ---------------------------------------------------------------------------

func (u *AppUpdater) InstallUpdate(rel *ReleaseInfo) error {
	if rel == nil || strings.TrimSpace(rel.asset.DownloadURL) == "" {
		return errors.New(i18n.T("updater.updateInfoInvalid"))
	}
	asset := rel.asset

	go func() {
		u.setProgress("starting", 0, 0, 0)

		tempDir, err := os.MkdirTemp("", "tgdown_update")
		if err != nil {
			u.setProgress("error: "+i18n.T("updater.tempDirError"), 0, 0, 0)
			return
		}
		defer os.RemoveAll(tempDir)

		// 1. Descarga con progreso manual
		archivePath := filepath.Join(tempDir, filepath.Base(asset.Name))
		u.setProgress("downloading", 0, asset.Size, 0)

		if err := u.downloadWithProgress(asset.DownloadURL, archivePath); err != nil {
			u.setProgress(i18n.T("updater.downloadFailedPrefix")+err.Error(), 0, 0, 0)
			return
		}

		// 1.a Comprobar que lo descargado es exactamente lo que se publicó.
		// HTTPS cubre a un intermediario en la red, pero no a nadie que consiga
		// publicar en el repositorio, y el archivo acaba ejecutándose en la
		// máquina del usuario.
		u.setProgress("verifying", 0, 0, 100)
		if err := verifyChecksum(archivePath, asset.DownloadURL, asset.Name); err != nil {
			u.setProgress("error: "+err.Error(), 0, 0, 0)
			logbus.Error(logbus.CatUpdater, i18n.T("updater.verifyRejected"), err.Error())
			return
		}

		// 1.b Instalación propia de la plataforma. En macOS la aplicación es un
		// bundle .app y hay que sustituirlo entero, no el binario de dentro;
		// installPlatform se encarga de eso y no regresa si lo consigue. En
		// Windows y Linux devuelve handled=false y seguimos con el reemplazo
		// del ejecutable de siempre.
		if handled, errPlat := u.installPlatform(archivePath, tempDir); handled {
			if errPlat != nil {
				u.setProgress("error: "+errPlat.Error(), 0, 0, 0)
				logbus.Error(logbus.CatUpdater, i18n.T("updater.applyError"), errPlat.Error())
			}
			return
		}

		// 2. Extracción
		u.setProgress("extracting", 0, 0, 100)
		extractPath := filepath.Join(tempDir, "extracted")
		_ = os.MkdirAll(extractPath, 0755)

		lowerArch := strings.ToLower(archivePath)
		if strings.HasSuffix(lowerArch, ".zip") {
			if err := unzip(archivePath, extractPath); err != nil {
				u.setProgress(i18n.T("updater.unzipFailedPrefix")+err.Error(), 0, 0, 0)
				return
			}
		} else if strings.HasSuffix(lowerArch, ".tar.gz") || strings.HasSuffix(lowerArch, ".tgz") {
			if err := untarGzip(archivePath, extractPath); err != nil {
				u.setProgress(i18n.T("updater.untarFailedPrefix")+err.Error(), 0, 0, 0)
				return
			}
		} else {
			// Si no es un archivo comprimido, lo tratamos como el binario directo
			extractPath = archivePath
		}

		// 3. Buscar el binario ejecutable real dentro de lo extraído
		newBinaryPath := u.findExecutable(extractPath)
		if newBinaryPath == "" {
			u.setProgress(i18n.T("updater.exeNotFound"), 0, 0, 0)
			return
		}

		// 4. Sustituir el ejecutable en marcha
		u.setProgress("finishing", 0, 0, 100)
		exePath, err := os.Executable()
		if err != nil {
			u.setProgress("error: "+i18n.T("updater.exePathError"), 0, 0, 0)
			return
		}

		log.Printf("[UPDATER] %s", i18n.T("updater.applying", exePath))
		if err := applyBinary(newBinaryPath, exePath); err != nil {
			u.setProgress(i18n.T("updater.applyFailedPrefix")+err.Error(), 0, 0, 0)
			logbus.Error(logbus.CatUpdater, i18n.T("updater.applyError"), err.Error())
			return
		}

		logbus.Success(logbus.CatUpdater, i18n.T("updater.applied"), exePath)
		u.setProgress("finishing", 0, 0, 100)
		time.Sleep(2 * time.Second)

		u.restartApp(exePath)
	}()

	return nil
}

// applyBinary sustituye el ejecutable en marcha por el recién descargado.
//
// Esto es lo que hacía github.com/minio/selfupdate. El mecanismo es el mismo que
// usa cualquier actualizador en Windows: un ejecutable en ejecución no se puede
// borrar ni sobrescribir, pero sí se puede *renombrar*, así que el actual se
// aparta a «.old», el nuevo ocupa su sitio y el viejo se borra en el siguiente
// arranque desde CleanOldVersion.
//
// Si algo falla a mitad se deshace el cambio: el usuario se queda con la versión
// que tenía, nunca sin aplicación.
func applyBinary(source, target string) error {
	// Los permisos del ejecutable actual se conservan. En Windows esto no
	// significa nada (mandan las ACL), pero en Linux y macOS sí.
	perms := os.FileMode(0o755)
	if fi, err := os.Stat(target); err == nil {
		if p := fi.Mode().Perm(); p&0o100 != 0 {
			perms = p
		}
	}

	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf(i18n.T("updater.readNewVersionError"), err)
	}
	defer sourceFile.Close()

	// El temporal se crea en la MISMA carpeta que el destino: renombrar entre
	// volúmenes distintos falla, y todo esto depende de que el último cambio de
	// nombre sea atómico.
	tmpFile, err := os.CreateTemp(filepath.Dir(target), ".tgdown-new-*")
	if err != nil {
		return fmt.Errorf(i18n.T("updater.writeError"), filepath.Dir(target), err)
	}
	tempPath := tmpFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(tmpFile, sourceFile); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf(i18n.T("updater.copyError"), err)
	}
	// Sin esto, un corte de corriente justo después del cambio de nombre podría
	// dejar en su sitio un ejecutable a medio escribir.
	_ = tmpFile.Sync()
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf(i18n.T("updater.closeError"), err)
	}
	if err := os.Chmod(tempPath, perms); err != nil {
		return fmt.Errorf(i18n.T("updater.chmodError"), err)
	}

	oldFile := target + ".old"
	_ = os.Remove(oldFile)
	if err := os.Rename(target, oldFile); err != nil {
		return fmt.Errorf(i18n.T("updater.moveOldError"), target, err)
	}

	if err := os.Rename(tempPath, target); err != nil {
		// Deshacer.
		if errRollback := os.Rename(oldFile, target); errRollback != nil {
			return fmt.Errorf(i18n.T("updater.rollbackFailed"), err, oldFile, errRollback)
		}
		return fmt.Errorf(i18n.T("updater.putNewError"), err)
	}
	removeTemp = false

	// En Windows el archivo apartado sigue en uso mientras este proceso viva, así
	// que no poder borrarlo ahora es lo normal y no es un fallo.
	if err := os.Remove(oldFile); err != nil {
		log.Printf("[UPDATER] %s", i18n.T("updater.cleanupNextBoot", err))
	}
	return nil
}

func (u *AppUpdater) downloadWithProgress(downloadURL, dest string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Vigilante de inactividad: si no llega un solo byte en noDataTimeout, se
	// corta la conexión. El cliente no lleva Timeout global a propósito, para no
	// abortar una descarga grande que va lenta pero avanza.
	watchdog := time.AfterFunc(noDataTimeout, cancel)
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(i18n.T("updater.githubStatusCode"), resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	total := resp.ContentLength
	var downloaded int64
	buffer := make([]byte, 64*1024)

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			watchdog.Reset(noDataTimeout)
			if _, werr := out.Write(buffer[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			pct := 0
			if total > 0 {
				pct = int(float64(downloaded) / float64(total) * 100) // 0% a 100%
			}
			u.setProgress("downloading", downloaded, total, pct)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf(i18n.T("updater.downloadTimeout"), noDataTimeout)
			}
			return err
		}
	}

	// Si el servidor anunció un tamaño y llegó otra cosa, el archivo está
	// incompleto: mejor decirlo aquí que fallar luego al verificar la firma.
	if total > 0 && downloaded != total {
		return fmt.Errorf(i18n.T("updater.downloadIncomplete"), downloaded, total)
	}
	return nil
}

func (u *AppUpdater) findExecutable(path string) string {
	fi, err := os.Stat(path)
	if err == nil && !fi.IsDir() {
		return path
	}

	var found string
	var priority int = -1

	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		name := strings.ToLower(info.Name())
		ext := strings.ToLower(filepath.Ext(name))

		// En Windows solo aceptamos .exe
		// En otros sistemas aceptamos sin extensión o cualquier cosa que parezca el binario
		isWindows := runtime.GOOS == "windows"
		if isWindows && ext != ".exe" {
			return nil
		}

		currPriority := -1
		// Nombres exactos tienen prioridad máxima
		if name == "telegramdl.exe" || name == "tgdown.exe" || name == "telegramdl" || name == "tgdown" {
			currPriority = 100
		} else if strings.Contains(name, "telegramdl") || strings.Contains(name, "tgdown") {
			currPriority = 50
		} else if !isWindows && ext == "" {
			// En Linux/Mac, un archivo sin extensión podría ser el binario
			currPriority = 10
		}

		if currPriority > priority {
			priority = currPriority
			found = p
		}

		if priority == 100 {
			return io.EOF // Encontrado el mejor match posible
		}
		return nil
	})
	return found
}

func (u *AppUpdater) restartApp(exePath string) {
	// Usamos la ruta original donde instalamos el nuevo binario
	// En lugar de os.Executable() que podría devolver la ruta del archivo .old en Windows
	cmd := exec.Command(exePath, os.Args[1:]...)

	log.Printf("[UPDATER] %s", i18n.T("updater.relaunching", exePath))
	err := cmd.Start()
	if err != nil {
		log.Printf("[UPDATER] %s", i18n.T("updater.relaunchError", err))
	}

	os.Exit(0)
}

// isWithin comprueba que una ruta extraída no se escapa del directorio de
// destino. Es la defensa contra las entradas con «..» dentro de un archivo
// comprimido, conocido como Zip Slip.
func isWithin(dest, target string) bool {
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	if targetAbs == destAbs {
		return true
	}
	return strings.HasPrefix(targetAbs, destAbs+string(os.PathSeparator))
}

// safeMode se queda con los permisos del archivo comprimido pero descarta
// setuid, setgid y sticky, que en un archivo bajado de internet no pintan nada.
func safeMode(mode int64) os.FileMode {
	perm := os.FileMode(mode).Perm()
	if perm == 0 {
		perm = 0o644
	}
	// Si el archivo era ejecutable para alguien, se conserva para el dueño.
	if perm&0o111 != 0 {
		perm |= 0o100
	}
	return perm
}

// ---------------------------------------------------------------------------
// Verificación de integridad
//
// Cada release publica un checksums.txt en el formato de sha256sum:
//
//	<sha256 en hexadecimal>  <nombre del archivo>
//
// Se exige que exista y que coincida. Si falta, la actualización se rechaza en
// vez de instalarse a ciegas: una release sin checksums es indistinguible de
// una manipulada.
// ---------------------------------------------------------------------------

const checksumsFileName = "checksums.txt"

func verifyChecksum(archivePath, assetURL, assetName string) error {
	expected, err := downloadChecksum(assetURL, assetName)
	if err != nil {
		return err
	}

	actual, err := sha256OfFile(archivePath)
	if err != nil {
		return fmt.Errorf(i18n.T("updater.signatureError"), err)
	}

	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf(i18n.T("updater.signatureMismatch"), expected, actual)
	}

	logbus.Info(logbus.CatUpdater, i18n.T("updater.verified"), assetName)
	return nil
}

func sha256OfFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// downloadChecksum busca el checksums.txt junto al asset y devuelve la suma
// que corresponde a assetName.
func downloadChecksum(assetURL, assetName string) (string, error) {
	checksumURL, err := checksumURL(assetURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent())

	resp, err := apiClient.Do(req)
	if err != nil {
		return "", fmt.Errorf(i18n.T("updater.checksumDownloadError"), checksumsFileName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(i18n.T("updater.checksumNotPublished"), checksumsFileName, resp.StatusCode)
	}

	// Un checksums.txt legítimo son unos pocos cientos de bytes; el límite evita
	// que una respuesta enorme agote la memoria.
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		// sha256sum antepone «*» al nombre en modo binario.
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if filepath.Base(name) == filepath.Base(assetName) {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf(i18n.T("updater.checksumReadError"), checksumsFileName, err)
	}

	return "", fmt.Errorf(i18n.T("updater.checksumMissingEntry"), checksumsFileName, assetName)
}

// checksumURL cambia el nombre del archivo al final de la URL del asset por
// checksums.txt, que es donde lo deja el flujo de publicación.
func checksumURL(assetURL string) (string, error) {
	u, err := url.Parse(assetURL)
	if err != nil {
		return "", fmt.Errorf(i18n.T("updater.invalidDownloadURL"), err)
	}
	i := strings.LastIndex(u.Path, "/")
	if i < 0 {
		return "", fmt.Errorf(i18n.T("updater.invalidDownloadURLText"), assetURL)
	}
	u.Path = u.Path[:i+1] + checksumsFileName
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// Funciones auxiliares de extracción
func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if !isWithin(dest, fpath) {
			continue
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, 0o755)
			continue
		}
		if err = os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, safeMode(int64(f.Mode())))
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untarGzip(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)
		// Una entrada con «..» en el nombre escribiría fuera del directorio de
		// destino. unzip ya lo comprobaba; esto faltaba aquí.
		if !isWithin(dest, target) {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, safeMode(header.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(outFile, tr)
			outFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
