package proxy

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// captureClientHello sobe um listener TCP local, dispara um cliente TLS
// real apontando para ele com o ServerName desejado e devolve os bytes
// brutos do primeiro ClientHello recebido. É a forma mais confiável de
// gerar um ClientHello válido para os testes — gerar à mão seria frágil.
func captureClientHello(t *testing.T, sni string) []byte {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- result{nil, err}
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			done <- result{nil, err}
			return
		}
		done <- result{buf[:n], nil}
	}()

	go func() {
		// O handshake vai falhar (não temos servidor TLS), mas o ClientHello
		// é enviado antes do erro.
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return
		}
		defer conn.Close()
		c := tls.Client(conn, &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		})
		_ = c.SetDeadline(time.Now().Add(1 * time.Second))
		_ = c.Handshake() // ignorado de propósito
	}()

	r := <-done
	if r.err != nil {
		t.Fatalf("erro capturando ClientHello: %v", r.err)
	}
	if len(r.data) == 0 {
		t.Fatal("nenhum byte capturado")
	}
	return r.data
}

func TestParseSNIRealClientHello(t *testing.T) {
	cases := []string{
		"example.com",
		"www.google.com",
		"a.b.c.d.empresa.com.br",
	}
	for _, sni := range cases {
		hello := captureClientHello(t, sni)
		got, err := ParseSNI(hello)
		if err != nil {
			t.Errorf("ParseSNI(%q) erro: %v", sni, err)
			continue
		}
		if got != sni {
			t.Errorf("ParseSNI(%q) = %q, want %q", sni, got, sni)
		}
	}
}

func TestParseSNIEmpty(t *testing.T) {
	if _, err := ParseSNI(nil); err == nil {
		t.Fatal("esperava erro para buffer vazio")
	}
}

func TestParseSNIWrongContentType(t *testing.T) {
	// Forja um record com ContentType=23 (ApplicationData) — deve recusar.
	bad := []byte{23, 0x03, 0x03, 0x00, 0x00}
	if _, err := ParseSNI(bad); err == nil {
		t.Fatal("esperava erro para ContentType inválido")
	}
}

func TestParseSNITruncated(t *testing.T) {
	// Pega um hello válido e corta no meio — precisa devolver ErrIncompleteHello.
	hello := captureClientHello(t, "example.com")
	for cut := 1; cut < len(hello)/2; cut += 7 {
		if _, err := ParseSNI(hello[:cut]); err == nil {
			t.Errorf("esperava erro com hello truncado em %d bytes", cut)
		}
	}
}
