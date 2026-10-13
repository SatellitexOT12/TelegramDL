//go:build windows

package server

// Abrir un archivo con la aplicación que le corresponda, en Windows.
//
// Este archivo existe por un problema de seguridad concreto. La primera versión
// usaba exec.Command("cmd", "/c", "start", "", ruta), y eso era una inyección de
// comandos: cmd.exe interpreta «&», «^» y «%», Go solo entrecomilla los
// argumentos que llevan espacios, y el nombre del archivo lo elige quien lo sube
// a Telegram. Un archivo llamado «video&calc.mp4» ejecutaba calc al pulsar
// «abrir».
//
// El siguiente intento fue rundll32.exe url.dll,FileProtocolHandler. Resolvía la
// inyección, pero rundll32 es una de las vías que más usa el malware para
// ejecutar cosas, así que los antivirus lo bloquean con frecuencia y el archivo
// simplemente no se abría.
//
// Lo que queda es la llamada de verdad: ShellExecuteW, que es exactamente lo que
// hace el Explorador al hacer doble clic. La ruta viaja como parámetro de la
// función en UTF-16, no como línea de comandos, así que no hay ningún intérprete
// de por medio y no hay nada que un «&» pueda romper.

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"tgdown/pkg/i18n"
)

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

// swShowNormal es SW_SHOWNORMAL. Se define aquí en vez de importarlo para no
// depender de ningún paquete externo: esto es solo la stdlib.
const swShowNormal = 1

func openInSystem(target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}

	if _, err := os.Stat(target); err != nil {
		// El archivo ya no está: se abre la carpeta que lo contenía, que es más
		// útil que no hacer nada.
		openWithExplorer(filepath.Dir(target))
		return
	}

	if err := shellExecute("open", target, filepath.Dir(target)); err == nil {
		return
	} else {
		log.Printf("[SERVER] %s", i18n.T("server.shellOpenError", target, err))
	}

	// Segundo intento. explorer.exe abre un archivo con su aplicación
	// predeterminada cuando se le pasa la ruta, y tampoco es un intérprete de
	// comandos, así que sigue sin haber inyección posible.
	openWithExplorer(target)
}

func openWithExplorer(ruta string) {
	if strings.TrimSpace(ruta) == "" {
		return
	}
	// explorer.exe devuelve código 1 incluso cuando funciona, así que su error
	// no dice nada y no se comprueba.
	_ = exec.Command("explorer.exe", ruta).Start()
}

// shellExecute llama a ShellExecuteW de shell32.dll.
//
// La función devuelve un valor mayor que 32 cuando ha conseguido lanzar algo;
// cualquier valor igual o menor es un código de error.
func shellExecute(verb, file, directory string) error {
	verbPtr, err := syscall.UTF16PtrFromString(verb)
	if err != nil {
		return err
	}
	filePtr, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	directoryPtr, err := syscall.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}

	ret, _, errLlamada := procShellExecute.Call(
		0, // sin ventana padre
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		0, // sin argumentos: se abre un archivo, no se ejecuta un programa
		uintptr(unsafe.Pointer(directoryPtr)),
		uintptr(swShowNormal),
	)

	if ret <= 32 {
		return fmt.Errorf(i18n.T("server.openErrorCode"), ret, errLlamada)
	}
	return nil
}
