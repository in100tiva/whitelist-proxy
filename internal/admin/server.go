// Package admin expõe endpoints REST + UI web para inspeção e gestão da
// whitelist. Escuta apenas em loopback (127.0.0.1) e exige um Bearer token
// salvo no arquivo admin.token (gerado no primeiro start). A UI carrega o
// token via parâmetro ?t=... ou via campo de login com persistência em
// localStorage.
package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/in100tiva/goproxy/whitelist-proxy/internal/browsers"
	"github.com/in100tiva/goproxy/whitelist-proxy/internal/config"
	"github.com/in100tiva/goproxy/whitelist-proxy/internal/filter"
	"github.com/in100tiva/goproxy/whitelist-proxy/internal/logger"
)

//go:embed ui/*
var uiAssets embed.FS

// Server é a HTTP API administrativa.
type Server struct {
	addr         string
	token        string
	matcher      *filter.Matcher
	log          *logger.Logger
	whitelistPth string
	proxyAddr    string
	browsers     *browsers.Manager

	srv *http.Server
}

// New cria o servidor admin. tokenPath é o caminho do arquivo onde o token
// é persistido (criado se não existir). whitelistPath é o caminho do
// whitelist.json para que /whitelist/reload saiba o que recarregar.
// proxyAddr é só informativo (mostrado na UI).
func New(addr, tokenPath, whitelistPath, proxyAddr string, m *filter.Matcher, lg *logger.Logger) (*Server, error) {
	tok, err := loadOrCreateToken(tokenPath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		addr:         addr,
		token:        tok,
		matcher:      m,
		log:          lg,
		whitelistPth: whitelistPath,
		proxyAddr:    proxyAddr,
		browsers:     browsers.New(proxyAddr),
	}

	mux := http.NewServeMux()
	// API (autenticada)
	mux.HandleFunc("/api/whitelist", s.auth(s.handleWhitelist))
	mux.HandleFunc("/api/whitelist/reload", s.auth(s.handleReload))
	mux.HandleFunc("/api/logs/recent", s.auth(s.handleLogs))
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/test", s.auth(s.handleTest))
	mux.HandleFunc("/api/browsers", s.auth(s.handleBrowsers))
	mux.HandleFunc("/api/browsers/configure", s.auth(s.handleBrowsersConfigure))

	// Aliases legados (compatibilidade com /whitelist sem prefixo /api).
	mux.HandleFunc("/whitelist", s.auth(s.handleWhitelist))
	mux.HandleFunc("/whitelist/reload", s.auth(s.handleReload))
	mux.HandleFunc("/logs/recent", s.auth(s.handleLogs))

	// UI estática (sem auth — quem fizer fetch da API ainda precisa do token).
	uiSub, err := fs.Sub(uiAssets, "ui")
	if err != nil {
		return nil, err
	}
	mux.Handle("/", http.FileServer(http.FS(uiSub)))

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// Token devolve o token administrativo (usado pelo subcomando "ui").
func (s *Server) Token() string { return s.token }

// ListenAndServe inicia a API admin. Bloqueia até erro ou Shutdown.
func (s *Server) ListenAndServe() error {
	if s.log != nil {
		s.log.Infof("admin escutando em %s", s.addr)
	}
	err := s.srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown encerra a API admin.
func (s *Server) Shutdown() error {
	return s.srv.Close()
}

// auth aplica verificação do header Authorization: Bearer <token> ou de
// um cookie/query "t" (para a UI carregar via link).
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	wantHeader := []byte("Bearer " + s.token)
	wantToken := []byte(s.token)
	return func(w http.ResponseWriter, r *http.Request) {
		// Header Authorization tem precedência.
		if got := []byte(r.Header.Get("Authorization")); len(got) == len(wantHeader) &&
			subtle.ConstantTimeCompare(got, wantHeader) == 1 {
			next(w, r)
			return
		}
		// Fallback: query string (?t=...). Só serve para chamadas GET de UI;
		// como o admin escuta apenas em loopback e o token é único, é ok.
		if got := []byte(r.URL.Query().Get("t")); len(got) == len(wantToken) &&
			subtle.ConstantTimeCompare(got, wantToken) == 1 {
			next(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

// ---------------- handlers ----------------

func (s *Server) handleWhitelist(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"mode":  s.matcher.Mode(),
			"rules": s.matcher.Rules(),
		})

	case http.MethodPut:
		var body struct {
			Mode  string        `json:"mode"`
			Rules []filter.Rule `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "json inválido: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.matcher.Load(body.Rules); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.matcher.SetMode(body.Mode)
		raw, err := json.MarshalIndent(struct {
			Mode  string        `json:"mode,omitempty"`
			Rules []filter.Rule `json:"rules"`
		}{body.Mode, body.Rules}, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(s.whitelistPth, append(raw, '\n'), 0o644); err != nil {
			http.Error(w, "gravando whitelist: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.log.Infof("whitelist salva via UI (%d regras, modo=%s)", len(body.Rules), s.matcher.Mode())
		writeJSON(w, http.StatusOK, map[string]any{"saved": len(body.Rules)})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	f, err := config.Load(s.whitelistPth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.matcher.Load(f.Rules); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.matcher.SetMode(f.Mode)
	s.log.Infof("whitelist recarregada via admin (%d regras, modo=%s)", len(f.Rules), s.matcher.Mode())
	writeJSON(w, http.StatusOK, map[string]any{"reloaded": len(f.Rules)})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n := 100
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"decisions": s.log.Recent(n)})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stats := s.log.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"proxy_addr":     s.proxyAddr,
		"admin_addr":     s.addr,
		"whitelist_path": s.whitelistPth,
		"rule_count":     len(s.matcher.Rules()),
		"started_at":     stats.StartedAt,
		"uptime_seconds": int64(time.Since(stats.StartedAt).Seconds()),
		"allow_count":    stats.AllowCount,
		"block_count":    stats.BlockCount,
	})
}

func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		http.Error(w, "host obrigatório", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"host":    host,
		"allowed": s.matcher.Allowed(host),
	})
}

func (s *Server) handleBrowsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"browsers": s.browsers.Detect()})
}

func (s *Server) handleBrowsersConfigure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		http.Error(w, "json inválido: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		list := s.browsers.ConfigureAll()
		writeJSON(w, http.StatusOK, map[string]any{"browsers": list})
		return
	}
	if err := s.browsers.Configure(body.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"browsers": s.browsers.Detect()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// loadOrCreateToken lê o token do disco ou gera um novo (32 bytes hex).
func loadOrCreateToken(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		// Remove whitespace para tolerar editores que acrescentam newline.
		out := make([]byte, 0, len(b))
		for _, c := range b {
			if c != '\n' && c != '\r' && c != ' ' && c != '\t' {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return string(out), nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(tok), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}
