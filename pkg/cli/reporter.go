package cli

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"tgdown/pkg/i18n"
	"tgdown/pkg/storage"
)

// CLIProgressReporter reporta el progreso de descargas en la terminal
type CLIProgressReporter struct {
	mu         sync.Mutex
	lastUpdate map[string]time.Time
	activeIDs  map[string]bool
}

// NewCLIProgressReporter crea un nuevo reporter de progreso para CLI
func NewCLIProgressReporter() *CLIProgressReporter {
	return &CLIProgressReporter{
		lastUpdate: make(map[string]time.Time),
		activeIDs:  make(map[string]bool),
	}
}

// OnStateChange es llamado cuando cambia el estado de una descarga
func (r *CLIProgressReporter) OnStateChange(item storage.DownloadItem) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// No spamear: solo actualizar si pasó al menos 500ms
	if last, ok := r.lastUpdate[item.ID]; ok {
		if time.Since(last) < 500*time.Millisecond {
			return
		}
	}
	r.lastUpdate[item.ID] = time.Now()

	switch item.Status {
	case "queued":
		fmt.Print(i18n.T("cli.statusQueued", truncate(item.FileName, 50)))
	case "downloading":
		r.printProgress(item)
	case "completed":
		fmt.Print(i18n.T("cli.statusCompleted", truncate(item.FileName, 50), item.TotalStr))
		delete(r.activeIDs, item.ID)
	case "failed":
		fmt.Print(i18n.T("cli.statusFailed", truncate(item.FileName, 50)))
		if item.Error != "" {
			fmt.Print(i18n.T("cli.statusErrorDetail", item.Error))
		}
		delete(r.activeIDs, item.ID)
	case "cancelled":
		fmt.Print(i18n.T("cli.statusCancelled", truncate(item.FileName, 50)))
		delete(r.activeIDs, item.ID)
	case "skipped":
		fmt.Print(i18n.T("cli.statusSkipped", truncate(item.FileName, 50)))
		delete(r.activeIDs, item.ID)
	case "duplicate":
		fmt.Print(i18n.T("cli.statusDuplicate", truncate(item.FileName, 50)))
		delete(r.activeIDs, item.ID)
	}

	r.activeIDs[item.ID] = true
}

// printProgress muestra la barra de progreso
func (r *CLIProgressReporter) printProgress(item storage.DownloadItem) {
	progress := item.Progress
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	// Barra de progreso simple
	barWidth := 30
	filled := int(progress * float64(barWidth) / 100)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	fmt.Printf("\r %s [%s] %.1f%% %s/s",
		truncate(item.FileName, 30),
		bar,
		progress,
		item.Speed,
	)

	if item.ETA != "" {
		fmt.Printf(" ETA: %s", item.ETA)
	}
}

// truncate acorta un string a un máximo de caracteres
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
