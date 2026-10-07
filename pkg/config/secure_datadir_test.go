package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSecureDataDirClosesPermissions fija el comportamiento de SecureDataDir
// (SEC-30): la carpeta de datos queda en 0700 y cada fichero sensible en 0600,
// tanto los de la lista fija —incluido el .env.migrado, que solo aparece al
// migrar el .env viejo— como los que llegan como ruta extra, que es como se
// pasan la base de datos recién abierta y sus ficheros -wal/-shm. Los ficheros
// ajenos a la lista no se tocan y repetir la llamada no cambia nada
// (idempotente).
func TestSecureDataDirClosesPermissions(t *testing.T) {
	dir := t.TempDir()

	// DataDir es global y apunta al ~/.tgdown real: se apunta al directorio
	// temporal y se restaura al terminar, para no tocar la máquina.
	prev := DataDir
	DataDir = dir
	t.Cleanup(func() { DataDir = prev })

	dbPath := filepath.Join(dir, "db.db")
	secured := []string{
		filepath.Join(dir, ".env"),
		filepath.Join(dir, ".env.migrado"),
		filepath.Join(dir, "tg_session.json"),
		dbPath,
		dbPath + "-wal",
		dbPath + "-shm",
	}
	for _, path := range secured {
		// Se crean con los permisos abiertos por defecto para comprobar que
		// SecureDataDir los cierra.
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("could not create %s: %v", filepath.Base(path), err)
		}
	}

	// Un fichero cualquiera de la carpeta no está en la lista: SecureDataDir
	// no debe ni mirarlo.
	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("x"), 0o644); err != nil {
		t.Fatalf("could not create unrelated file: %v", err)
	}
	unrelatedBefore := filePerm(t, unrelated)

	// Dos llamadas a propósito: la segunda no debe cambiar el resultado.
	SecureDataDir(dbPath, dbPath+"-wal", dbPath+"-shm")
	SecureDataDir(dbPath, dbPath+"-wal", dbPath+"-shm")

	// En Windows los bits de modo no significan nada —mandan las ACL de NTFS
	// y la carpeta del usuario ya está restringida—, así que la comprobación
	// solo tiene sentido donde Go sí los aplica. La llamada de arriba se
	// ejecuta igualmente para dejar constancia de que no revienta.
	if runtime.GOOS == "windows" {
		return
	}

	for _, path := range secured {
		if got := filePerm(t, path); got != 0o600 {
			t.Errorf("%s perm = %#o, want 0600", filepath.Base(path), uint32(got))
		}
	}
	if got := filePerm(t, dir); got != 0o700 {
		t.Errorf("data dir perm = %#o, want 0700", uint32(got))
	}
	if got := filePerm(t, unrelated); got != unrelatedBefore {
		t.Errorf("unrelated file perm = %#o, want %#o (out of scope)",
			uint32(got), uint32(unrelatedBefore))
	}
}

// filePerm devuelve los bits de permiso de un fichero que acaba de crearse;
// si no se puede leer el modo, el test no tiene nada que comprobar.
func filePerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("could not stat %s: %v", path, err)
	}
	return fi.Mode().Perm()
}
