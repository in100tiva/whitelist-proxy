// Package config carrega o arquivo whitelist.json e oferece um watcher
// simples baseado em polling de mtime. Não usamos fsnotify pra manter o
// binário sem CGo e sem dependências externas além de golang.org/x/sys.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/in100tiva/goproxy/whitelist-proxy/internal/filter"
)

// File representa o conteúdo do whitelist.json.
type File struct {
	Mode  string        `json:"mode,omitempty"` // "blacklist" | "whitelist" (default: "blacklist")
	Rules []filter.Rule `json:"rules"`
}

// Load lê e faz parse do arquivo de whitelist no caminho informado.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lendo %s: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse json %s: %w", path, err)
	}
	return &f, nil
}

// EnsureExists cria um whitelist.json mínimo se ele não existir, para que
// o serviço suba mesmo na primeira execução. O default bloqueia tudo,
// exceto um único domínio de exemplo (que o admin troca depois).
func EnsureExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	defaults := File{
		Mode:  "blacklist",
		Rules: []filter.Rule{},
	}
	b, _ := json.MarshalIndent(defaults, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// Watcher observa mtime do arquivo de whitelist e dispara o callback
// sempre que ele muda. É deliberadamente simples — polling de 2s é mais
// que suficiente para um arquivo de configuração.
type Watcher struct {
	path     string
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewWatcher cria (mas não inicia) o watcher. Use Start para começar.
func NewWatcher(path string, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Watcher{path: path, interval: interval, stop: make(chan struct{})}
}

// Start inicia o loop de polling em background. onChange recebe o conteúdo
// recém-carregado; se onChange devolver erro, o watcher loga (via callback
// onError) e mantém a configuração anterior.
func (w *Watcher) Start(onChange func(*File) error, onError func(error)) {
	var lastMod time.Time
	if st, err := os.Stat(w.path); err == nil {
		lastMod = st.ModTime()
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-ticker.C:
				st, err := os.Stat(w.path)
				if err != nil {
					if onError != nil {
						onError(err)
					}
					continue
				}
				if !st.ModTime().After(lastMod) {
					continue
				}
				lastMod = st.ModTime()
				f, err := Load(w.path)
				if err != nil {
					if onError != nil {
						onError(err)
					}
					continue
				}
				if err := onChange(f); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()
}

// Stop encerra o watcher e espera o loop sair.
func (w *Watcher) Stop() {
	close(w.stop)
	w.wg.Wait()
}
