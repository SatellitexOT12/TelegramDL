//go:build !windows

package server

// Abrir un archivo con la aplicación que le corresponda, fuera de Windows.
//
// Aquí nunca hubo problema: tanto open como xdg-open se ejecutan directamente,
// sin pasar por un shell, así que la ruta llega como un único argumento y no se
// interpreta. El caso delicado es Windows, y vive en open_windows.go.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func openInSystem(target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}

	if _, err := os.Stat(target); err != nil {
		// El archivo ya no existe en la ruta: abrir la carpeta que lo contenía.
		target = filepath.Dir(target)
	}

	if runtime.GOOS == "darwin" {
		// El «--» evita que un nombre que empiece por guion se lea como opción.
		_ = exec.Command("open", "--", target).Start()
		return
	}

	_ = exec.Command("xdg-open", target).Start()
}
