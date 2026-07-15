package config

import (
	"os"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// Watcher polls a config file's mtime and calls onChange with a freshly
// loaded Config whenever the file changes on disk — this is what makes
// `flc config set` (which rewrites flow.yaml) take effect on a running
// daemon without a restart, satisfying "Hot Reload Config" for real
// rather than just re-reading at next boot.
type Watcher struct {
	path     string
	interval time.Duration
	lastMod  time.Time
}

func NewWatcher(path string, interval time.Duration) *Watcher {
	w := &Watcher{path: path, interval: interval}
	if info, err := os.Stat(path); err == nil {
		w.lastMod = info.ModTime()
	}
	return w
}

func (w *Watcher) Run(onChange func(*utils.Config), stopCh <-chan struct{}) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			info, err := os.Stat(w.path)
			if err != nil {
				continue
			}
			if info.ModTime().After(w.lastMod) {
				w.lastMod = info.ModTime()
				newCfg, err := utils.LoadConfig(".")
				if err != nil {
					utils.LogWarn("hot reload: failed to load updated config: %v", err)
					continue
				}
				utils.LogInfo("config file changed, reloading")
				onChange(newCfg)
			}
		}
	}
}
