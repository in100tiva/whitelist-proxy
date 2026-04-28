// Package admin expõe endpoints REST + UI web para inspeção e gestão da
// whitelist. Escuta apenas em loopback (127.0.0.1) e exige um Bearer token
// salvo no arquivo admin.token (gerado no primeiro start). A UI carrega o
// token via parâmetro ?t=... ou via campo de login com persistência em
// localStorage.
package admin

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
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
	matcher      *filter.Matcher
	log          *logger.Logger
	whitelistPth string
	proxyAddr    string
	browsers     *browsers.Manager

	srv *http.Server
}

// New cria o servidor admin. whitelistPath é o caminho do
// whitelist.json para que /whitelist/reload saiba o que recarregar.
// proxyAddr é só informativo (mostrado na UI).
func New(addr, whitelistPath, proxyAddr string, m *filter.Matcher, lg *logger.Logger) (*Server, error) {
	s := &Server{
		addr:         addr,
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

	// Página de acesso bloqueado — sem auth, acessível pelo proxy ao redirecionar.
	mux.HandleFunc("/blocked", s.handleBlocked)

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
func (s *Server) Token() string { return currentToken() }

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

// currentToken retorna o token baseado na hora e minuto atuais, ex: "0953".
func currentToken() string {
	return time.Now().Format("1504")
}

// prevToken retorna o token do minuto anterior (para tolerância na virada do minuto).
func prevToken() string {
	return time.Now().Add(-time.Minute).Format("1504")
}

// tokenOK verifica o token recebido contra atual e anterior.
func tokenOK(got, want string) bool {
	g, w := []byte(got), []byte(want)
	if len(g) != len(w) {
		return false
	}
	return subtle.ConstantTimeCompare(g, w) == 1
}

// auth aplica verificação do header Authorization: Bearer <token> ou de
// um cookie/query "t" (para a UI carregar via link).
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cur, prv := currentToken(), prevToken()
		if got := r.Header.Get("Authorization"); tokenOK(got, "Bearer "+cur) || tokenOK(got, "Bearer "+prv) {
			next(w, r)
			return
		}
		if got := r.URL.Query().Get("t"); tokenOK(got, cur) || tokenOK(got, prv) {
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

func (s *Server) handleBlocked(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		host = "domínio bloqueado"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprintf(w, blockedPageHTML, host)
}

const blockedPageHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Acesso Bloqueado</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #0d1117;
      color: #e6edf3;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 24px;
    }
    .card {
      background: #161b22;
      border: 1px solid #30363d;
      border-radius: 16px;
      padding: 56px 48px;
      max-width: 520px;
      width: 100%%;
      text-align: center;
      box-shadow: 0 16px 48px rgba(0,0,0,.4);
    }
    .shield {
      width: 80px; height: 80px;
      background: rgba(248,81,73,.12);
      border-radius: 50%%;
      display: flex; align-items: center; justify-content: center;
      margin: 0 auto 28px;
    }
    .shield svg { width: 40px; height: 40px; }
    h1 {
      font-size: 26px; font-weight: 700;
      color: #f85149;
      margin-bottom: 12px;
      letter-spacing: -.3px;
    }
    .subtitle {
      font-size: 14px; color: #8b949e;
      margin-bottom: 24px;
      line-height: 1.6;
    }
    .host-box {
      background: #0d1117;
      border: 1px solid #30363d;
      border-radius: 8px;
      padding: 12px 20px;
      font-family: 'SFMono-Regular', Consolas, monospace;
      font-size: 15px;
      color: #f0883e;
      margin-bottom: 28px;
      word-break: break-all;
    }
    .info {
      font-size: 13px; color: #6e7681;
      line-height: 1.7;
      margin-bottom: 32px;
    }
    .info strong { color: #8b949e; }
    .actions { display: flex; gap: 12px; justify-content: center; flex-wrap: wrap; }
    .btn {
      display: inline-flex; align-items: center; gap: 6px;
      padding: 10px 20px;
      border-radius: 8px;
      font-size: 14px; font-weight: 500;
      text-decoration: none;
      cursor: pointer;
      border: none;
      transition: opacity .15s;
    }
    .btn:hover { opacity: .85; }
    .btn-secondary {
      background: #21262d;
      border: 1px solid #30363d;
      color: #e6edf3;
    }
    .divider {
      border: none; border-top: 1px solid #21262d;
      margin: 32px 0 24px;
    }
    .badge {
      display: inline-flex; align-items: center; gap: 6px;
      font-size: 12px; color: #6e7681;
    }
    .dot {
      width: 8px; height: 8px; border-radius: 50%%;
      background: #3fb950;
      display: inline-block;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="shield">
      <svg viewBox="0 0 24 24" fill="none" stroke="#f85149" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
        <line x1="12" y1="8" x2="12" y2="12"/>
        <line x1="12" y1="16" x2="12.01" y2="16"/>
      </svg>
    </div>

    <h1>Acesso Bloqueado</h1>
    <p class="subtitle">Este conteúdo foi bloqueado pela política de acesso da rede.</p>

    <div class="host-box">%s</div>

    <p class="info">
      O acesso a este domínio está <strong>restrito</strong> pelo administrador.<br>
      Se você precisa acessar este conteúdo por motivos de trabalho,<br>
      entre em contato com o responsável pela rede.
    </p>

    <div class="actions">
      <button class="btn btn-secondary" onclick="history.back()">
        ← Voltar
      </button>
    </div>

    <hr class="divider">
    <span class="badge">
      <span class="dot"></span>
      Whitelist Proxy — proteção ativa
    </span>
  </div>
</body>
</html>`

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

