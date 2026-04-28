// Package logger fornece logging estruturado em JSON com rotação diária
// dos arquivos. Mantém também um buffer circular em memória com as últimas
// N decisões para o endpoint admin /logs/recent.
package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Decision é o registro estruturado de cada decisão do proxy.
type Decision struct {
	Time    time.Time `json:"time"`
	Action  string    `json:"action"` // "allow" ou "block"
	Proto   string    `json:"proto"`  // "http" ou "https"
	Host    string    `json:"host"`
	Client  string    `json:"client"` // IP origem
	Reason  string    `json:"reason,omitempty"`
}

// Logger escreve em arquivo (rotacionado por dia) e mantém um ring buffer.
type Logger struct {
	dir       string
	bufferCap int

	mu       sync.Mutex
	file     *os.File
	currDate string // YYYY-MM-DD do arquivo aberto
	ring     []Decision
	ringHead int
	ringSize int

	// Contadores totais desde a inicialização.
	startedAt    time.Time
	allowCount   uint64
	blockCount   uint64
}

// Stats devolve um snapshot dos contadores acumulados.
type Stats struct {
	StartedAt  time.Time `json:"started_at"`
	AllowCount uint64    `json:"allow_count"`
	BlockCount uint64    `json:"block_count"`
}

// New abre/cria a pasta de logs e retorna um logger pronto para uso.
// bufferCap define o tamanho do ring buffer em memória (sugestão: 500–2000).
func New(dir string, bufferCap int) (*Logger, error) {
	if bufferCap <= 0 {
		bufferCap = 1000
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Logger{
		dir:       dir,
		bufferCap: bufferCap,
		ring:      make([]Decision, bufferCap),
		startedAt: time.Now(),
	}
	if err := l.rotateLocked(time.Now()); err != nil {
		return nil, err
	}
	return l, nil
}

// Log registra uma decisão. Erros de I/O são silenciados de propósito —
// melhor perder uma linha de log do que derrubar o proxy.
func (l *Logger) Log(d Decision) {
	if d.Time.IsZero() {
		d.Time = time.Now()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Rotaciona se o dia mudou.
	if date := d.Time.Format("2006-01-02"); date != l.currDate {
		_ = l.rotateLocked(d.Time)
	}

	// Ring buffer
	l.ring[l.ringHead] = d
	l.ringHead = (l.ringHead + 1) % l.bufferCap
	if l.ringSize < l.bufferCap {
		l.ringSize++
	}

	// Contadores acumulados (info não conta).
	switch d.Action {
	case "allow":
		l.allowCount++
	case "block":
		l.blockCount++
	}

	// Arquivo (uma linha JSON por decisão — formato JSONL).
	if l.file != nil {
		b, err := json.Marshal(d)
		if err == nil {
			b = append(b, '\n')
			_, _ = l.file.Write(b)
		}
	}
}

// Stats devolve um snapshot dos contadores acumulados.
func (l *Logger) Stats() Stats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Stats{
		StartedAt:  l.startedAt,
		AllowCount: l.allowCount,
		BlockCount: l.blockCount,
	}
}

// Recent devolve as últimas n decisões em ordem cronológica (mais antiga primeiro).
func (l *Logger) Recent(n int) []Decision {
	l.mu.Lock()
	defer l.mu.Unlock()

	if n <= 0 || n > l.ringSize {
		n = l.ringSize
	}
	out := make([]Decision, n)
	// Posição do mais antigo no ring buffer.
	start := (l.ringHead - l.ringSize + l.bufferCap) % l.bufferCap
	// Saltar para os últimos n.
	start = (start + (l.ringSize - n) + l.bufferCap) % l.bufferCap
	for i := 0; i < n; i++ {
		out[i] = l.ring[(start+i)%l.bufferCap]
	}
	return out
}

// Close fecha o arquivo de log atual.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// Infof escreve uma linha de log informativo (não-decisão), útil para
// eventos do ciclo de vida do serviço.
func (l *Logger) Infof(format string, args ...any) {
	l.Log(Decision{
		Time:   time.Now(),
		Action: "info",
		Reason: fmt.Sprintf(format, args...),
	})
}

func (l *Logger) rotateLocked(now time.Time) error {
	date := now.Format("2006-01-02")
	if l.file != nil && date == l.currDate {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	path := filepath.Join(l.dir, fmt.Sprintf("proxy-%s.log", date))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.file = f
	l.currDate = date
	return nil
}
