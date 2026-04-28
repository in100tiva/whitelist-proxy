package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/in100tiva/goproxy/whitelist-proxy/internal/filter"
	"github.com/in100tiva/goproxy/whitelist-proxy/internal/logger"
)

// Idle timeout das conexões tuneladas. Conexões sem tráfego por mais tempo
// são fechadas para evitar goroutines zumbis.
const idleTimeout = 5 * time.Minute

// Tamanho máximo do ClientHello que aceitamos peekar. 16 KiB cobre o limite
// de um TLSPlaintext (2^14 bytes). Quem mandar mais que isso (raro) será
// bloqueado por segurança.
const maxClientHello = 16 * 1024

// Server é o proxy HTTP/HTTPS com whitelist.
type Server struct {
	addr      string
	adminAddr string // usado para redirecionar para página de bloqueio
	matcher   *filter.Matcher
	log       *logger.Logger

	srv *http.Server
}

// New cria o servidor mas não escuta ainda. addr no formato "host:porta".
// adminAddr é o endereço da UI admin (ex: "127.0.0.1:8081") usado para
// redirecionar para a página de acesso bloqueado.
func New(addr, adminAddr string, m *filter.Matcher, lg *logger.Logger) *Server {
	s := &Server{addr: addr, adminAddr: adminAddr, matcher: m, log: lg}
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           http.HandlerFunc(s.handle),
		ReadHeaderTimeout: 30 * time.Second,
		// Desligamos timeouts de leitura/escrita do server porque ele
		// também atende CONNECT, e o tunneling pode ficar parado por
		// muito tempo entre lampejos de tráfego.
	}
	return s
}

// ListenAndServe inicia o proxy. Bloqueia até erro ou Shutdown.
func (s *Server) ListenAndServe() error {
	if s.log != nil {
		s.log.Infof("proxy escutando em %s", s.addr)
	}
	err := s.srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown encerra o servidor graciosamente.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// handle é o dispatcher: CONNECT vira tunneling com SNI; resto é HTTP.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}
	s.handleHTTP(w, r)
}

// ---------------- HTTP em texto puro ----------------

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	host := hostOnly(r.Host)
	if host == "" {
		host = hostOnly(r.URL.Host)
	}
	clientIP := remoteIP(r.RemoteAddr)

	if !s.matcher.Allowed(host) {
		s.log.Log(logger.Decision{
			Action: "block", Proto: "http", Host: host, Client: clientIP,
			Reason: "host fora da whitelist",
		})
		s.redirectBlocked(w, r, host)
		return
	}

	// Encaminha o request preservando método/cabeçalhos/body.
	target := *r.URL
	if target.Scheme == "" {
		target.Scheme = "http"
	}
	if target.Host == "" {
		target.Host = r.Host
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
	if err != nil {
		http.Error(w, "request inválido", http.StatusBadRequest)
		return
	}
	copyHopByHopFiltered(outReq.Header, r.Header)

	tr := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 30 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	defer tr.CloseIdleConnections()

	resp, err := tr.RoundTrip(outReq)
	if err != nil {
		s.log.Log(logger.Decision{
			Action: "allow", Proto: "http", Host: host, Client: clientIP,
			Reason: "upstream error: " + err.Error(),
		})
		http.Error(w, "erro upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHopByHopFiltered(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)

	s.log.Log(logger.Decision{
		Action: "allow", Proto: "http", Host: host, Client: clientIP,
	})
}

// ---------------- HTTPS via CONNECT + SNI ----------------

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	clientIP := remoteIP(r.RemoteAddr)
	target := r.URL.Host // formato "host:port" no CONNECT
	if target == "" {
		target = r.Host
	}

	// Pré-verificação com o host do CONNECT (antes de aceitar o túnel).
	// Permite mostrar a página de bloqueio em vez de apenas fechar a conexão.
	connectHost := hostOnly(target)
	if !s.matcher.Allowed(connectHost) {
		s.log.Log(logger.Decision{
			Action: "block", Proto: "https", Host: connectHost, Client: clientIP,
			Reason: "host fora da whitelist (CONNECT)",
		})
		s.redirectBlocked(w, r, connectHost)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack não suportado", http.StatusInternalServerError)
		return
	}
	clientConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "hijack falhou: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Confirma o CONNECT antes do cliente mandar o ClientHello.
	if _, err := bufrw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err := bufrw.Flush(); err != nil {
		return
	}

	// Faz peek do ClientHello sem consumir do bufio.Reader. Lemos em loop
	// até ParseSNI conseguir extrair o nome ou desistir.
	peeked, sni, err := peekSNI(bufrw.Reader)
	if err != nil {
		s.log.Log(logger.Decision{
			Action: "block", Proto: "https", Host: hostOnly(target), Client: clientIP,
			Reason: "falha SNI: " + err.Error(),
		})
		return
	}

	if !s.matcher.Allowed(sni) {
		s.log.Log(logger.Decision{
			Action: "block", Proto: "https", Host: sni, Client: clientIP,
			Reason: "SNI fora da whitelist",
		})
		return
	}

	// Conecta no destino real. Usamos o host:porta do CONNECT como destino,
	// mas o nome decisivo para a regra é o SNI (que já foi validado).
	upstream, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		s.log.Log(logger.Decision{
			Action: "allow", Proto: "https", Host: sni, Client: clientIP,
			Reason: "falha dial upstream: " + err.Error(),
		})
		return
	}
	defer upstream.Close()

	// Replay dos bytes já lidos (ClientHello completo) para o upstream.
	if _, err := upstream.Write(peeked); err != nil {
		return
	}

	s.log.Log(logger.Decision{
		Action: "allow", Proto: "https", Host: sni, Client: clientIP,
	})

	// Tunneling bidirecional com idle timeout via deadline contínua.
	tunnel(clientConn, upstream)
}

// peekSNI lê do bufio.Reader (sem consumir além do necessário) até conseguir
// extrair o SNI. Devolve os bytes brutos lidos para que sejam replayados ao
// upstream. Importante: precisamos peekar em vez de Read porque os mesmos
// bytes precisam fluir para o destino real depois.
func peekSNI(br *bufio.Reader) ([]byte, string, error) {
	// Lê pelo menos o cabeçalho do record (5 bytes) para descobrir o tamanho.
	header, err := br.Peek(5)
	if err != nil {
		return nil, "", fmt.Errorf("lendo header TLS: %w", err)
	}
	if header[0] != 22 {
		return nil, "", fmt.Errorf("primeiro byte 0x%02x não é Handshake(0x16)", header[0])
	}
	recordLen := int(header[3])<<8 | int(header[4])
	total := 5 + recordLen
	if total > maxClientHello {
		return nil, "", fmt.Errorf("ClientHello %d bytes excede o limite %d", total, maxClientHello)
	}

	// Aumenta o buffer interno se necessário e peeka tudo de uma vez.
	full, err := br.Peek(total)
	if err != nil {
		return nil, "", fmt.Errorf("lendo ClientHello: %w", err)
	}

	sni, err := ParseSNI(full)
	if err != nil {
		return nil, "", err
	}

	// Copia para um buffer próprio antes de consumir do bufio (evita aliasing).
	out := make([]byte, total)
	copy(out, full)
	if _, err := br.Discard(total); err != nil {
		return nil, "", err
	}
	return out, sni, nil
}

// tunnel faz io.Copy nos dois sentidos, encerrando ao primeiro EOF ou erro.
// Aplica idle timeout via SetReadDeadline rolante.
func tunnel(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyWithDeadline := func(dst, src net.Conn) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			_ = src.SetReadDeadline(time.Now().Add(idleTimeout))
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}

	go copyWithDeadline(a, b)
	go copyWithDeadline(b, a)

	wg.Wait()
	// Forçar fechamento de ambos para destravar a outra goroutine se ainda
	// estiver presa em Read (em geral o SetReadDeadline já cobre).
	_ = a.Close()
	_ = b.Close()
}

// ---------------- helpers ----------------

func hostOnly(hp string) string {
	hp = strings.TrimSpace(hp)
	if hp == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(hp); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(hp)
}

func remoteIP(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

// hopByHopHeaders são removidos durante o forward do request HTTP. Lista
// definida na RFC 7230 §6.1.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Proxy-Connection":    true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// redirectBlocked redireciona o cliente para a página de acesso bloqueado na UI admin.
// Para requisições HTTP comuns, faz um redirect 302 normal.
// Para CONNECT (HTTPS), responde com 302 antes de aceitar o túnel — Firefox segue
// redirectBlocked envia a resposta de bloqueio adequada para cada tipo de request:
//
//   - HTTP normal: redirect 302 para a página bonita do admin (funciona em todos os browsers).
//   - CONNECT (HTTPS): respond com 403 + HTML com meta-refresh. Firefox renderiza o corpo
//     da resposta de erro do CONNECT e segue o meta-refresh; Chrome mostra ERR_TUNNEL_CONNECTION_FAILED
//     (sem MITM não há alternativa para Chrome em HTTPS).
func (s *Server) redirectBlocked(w http.ResponseWriter, r *http.Request, host string) {
	dest := "http://" + s.adminAddr + "/blocked?host=" + url.QueryEscape(host)
	if r.Method == http.MethodConnect {
		// CONNECT: serve HTML direto — Firefox renderiza o corpo de respostas de erro
		// do CONNECT (4xx). O meta-refresh navega para a página completa do admin.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, connectBlockedHTML, dest, host, dest)
		return
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// connectBlockedHTML é a página mínima enviada no corpo de uma resposta 403 ao CONNECT.
// O meta-refresh navega imediatamente para a página de bloqueio completa no admin server.
const connectBlockedHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="UTF-8">
<meta http-equiv="refresh" content="0; url=%s">
<title>Acesso Bloqueado</title>
<style>
body{font-family:sans-serif;background:#0d1117;color:#e6edf3;display:flex;
align-items:center;justify-content:center;min-height:100vh;margin:0}
.box{text-align:center;padding:40px}
h1{color:#f85149;margin-bottom:12px}
a{color:#58a6ff}
</style>
</head>
<body>
<div class="box">
  <h1>🚫 Acesso Bloqueado</h1>
  <p>%s está bloqueado pela política de rede.</p>
  <p><a href="%s">Ver detalhes</a></p>
</div>
</body>
</html>`

func copyHopByHopFiltered(dst, src http.Header) {
	for k, vs := range src {
		if hopByHopHeaders[http.CanonicalHeaderKey(k)] {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
