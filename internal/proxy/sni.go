// Package proxy contém o servidor de proxy HTTP/CONNECT e o parser de SNI.
//
// O parser de SNI nesta unidade lê os primeiros bytes de um ClientHello TLS
// (TLS 1.0 a 1.3) e extrai o valor da extensão server_name (RFC 6066).
// Não fazemos handshake nem decifragem — apenas inspeção do plaintext que
// o cliente envia logo no início da conexão.
package proxy

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrNoSNI é devolvido quando o ClientHello é válido porém não contém a
// extensão server_name (raro mas possível: clientes legados).
var ErrNoSNI = errors.New("ClientHello sem extensão SNI")

// ErrIncompleteHello indica que os bytes recebidos ainda não cobrem o
// ClientHello inteiro — o chamador deve ler mais e tentar de novo.
var ErrIncompleteHello = errors.New("ClientHello incompleto")

// ParseSNI extrai o server_name do primeiro ClientHello presente em data.
//
// Layout esperado (RFC 5246 §7.4.1.2 + RFC 6066 §3):
//
//	TLSPlaintext (record):
//	  uint8  ContentType = 22 (handshake)
//	  uint16 ProtocolVersion
//	  uint16 length
//	  opaque fragment[length]
//
//	Handshake:
//	  uint8  HandshakeType = 1 (client_hello)
//	  uint24 length
//	  ClientHello body:
//	    uint16  client_version
//	    opaque  random[32]
//	    uint8   session_id_length; opaque session_id[session_id_length]
//	    uint16  cipher_suites_length; opaque cipher_suites[...]
//	    uint8   compression_methods_length; opaque compression_methods[...]
//	    uint16  extensions_length; Extension extensions[extensions_length]
//
//	Extension:
//	  uint16 extension_type
//	  uint16 extension_data_length
//	  opaque extension_data[extension_data_length]
//
//	Extension server_name (type=0):
//	  uint16 server_name_list_length
//	  ServerName entries[]:
//	    uint8  name_type (0 = host_name)
//	    uint16 host_name_length
//	    opaque host_name[host_name_length]
func ParseSNI(data []byte) (string, error) {
	c := &cursor{buf: data}

	// --- Record layer ---
	contentType, err := c.u8()
	if err != nil {
		return "", ErrIncompleteHello
	}
	if contentType != 22 {
		return "", fmt.Errorf("ContentType=%d não é Handshake(22)", contentType)
	}
	if _, err := c.u16(); err != nil { // ProtocolVersion do record (ignorada)
		return "", ErrIncompleteHello
	}
	recordLen, err := c.u16()
	if err != nil {
		return "", ErrIncompleteHello
	}
	if int(recordLen) > c.remaining() {
		// Aviso: alguns clientes mandam o ClientHello em mais de um record TLS.
		// Para simplificar (e por ser raro com handshakes modernos) exigimos
		// que tudo caiba num único record — basta o caller ler mais bytes.
		return "", ErrIncompleteHello
	}

	// --- Handshake header ---
	hsType, err := c.u8()
	if err != nil {
		return "", ErrIncompleteHello
	}
	if hsType != 1 {
		return "", fmt.Errorf("HandshakeType=%d não é ClientHello(1)", hsType)
	}
	if _, err := c.u24(); err != nil { // tamanho do handshake (ignorado, vamos pelo record)
		return "", ErrIncompleteHello
	}

	// --- ClientHello body ---
	if _, err := c.u16(); err != nil { // client_version
		return "", ErrIncompleteHello
	}
	if err := c.skip(32); err != nil { // random
		return "", ErrIncompleteHello
	}

	// session_id (uint8 length)
	sessLen, err := c.u8()
	if err != nil {
		return "", ErrIncompleteHello
	}
	if err := c.skip(int(sessLen)); err != nil {
		return "", ErrIncompleteHello
	}

	// cipher_suites (uint16 length)
	csLen, err := c.u16()
	if err != nil {
		return "", ErrIncompleteHello
	}
	if err := c.skip(int(csLen)); err != nil {
		return "", ErrIncompleteHello
	}

	// compression_methods (uint8 length)
	cmLen, err := c.u8()
	if err != nil {
		return "", ErrIncompleteHello
	}
	if err := c.skip(int(cmLen)); err != nil {
		return "", ErrIncompleteHello
	}

	// extensions — campo opcional na spec, mas todo cliente moderno envia.
	if c.remaining() == 0 {
		return "", ErrNoSNI
	}
	extLen, err := c.u16()
	if err != nil {
		return "", ErrIncompleteHello
	}
	if int(extLen) > c.remaining() {
		return "", ErrIncompleteHello
	}

	// Trabalhamos apenas dentro do bloco de extensões.
	ext := &cursor{buf: c.buf[c.off : c.off+int(extLen)]}
	for ext.remaining() >= 4 {
		extType, err := ext.u16()
		if err != nil {
			return "", ErrIncompleteHello
		}
		extDataLen, err := ext.u16()
		if err != nil {
			return "", ErrIncompleteHello
		}
		if int(extDataLen) > ext.remaining() {
			return "", ErrIncompleteHello
		}

		if extType != 0 { // 0 == server_name
			if err := ext.skip(int(extDataLen)); err != nil {
				return "", ErrIncompleteHello
			}
			continue
		}

		// Achamos a extensão SNI. Estrutura aninhada começa aqui.
		sni := &cursor{buf: ext.buf[ext.off : ext.off+int(extDataLen)]}

		listLen, err := sni.u16()
		if err != nil {
			return "", ErrIncompleteHello
		}
		if int(listLen) > sni.remaining() {
			return "", ErrIncompleteHello
		}
		list := &cursor{buf: sni.buf[sni.off : sni.off+int(listLen)]}

		for list.remaining() > 0 {
			nameType, err := list.u8()
			if err != nil {
				return "", ErrIncompleteHello
			}
			nameLen, err := list.u16()
			if err != nil {
				return "", ErrIncompleteHello
			}
			if int(nameLen) > list.remaining() {
				return "", ErrIncompleteHello
			}
			if nameType != 0 { // só host_name(0) interessa; outros tipos são reservados
				if err := list.skip(int(nameLen)); err != nil {
					return "", ErrIncompleteHello
				}
				continue
			}
			name := string(list.buf[list.off : list.off+int(nameLen)])
			return name, nil
		}
		return "", ErrNoSNI
	}
	return "", ErrNoSNI
}

// cursor é um leitor sequencial sobre um buffer com checagem de bounds.
// Mantém o código de parse linear e fácil de auditar.
type cursor struct {
	buf []byte
	off int
}

func (c *cursor) remaining() int { return len(c.buf) - c.off }

func (c *cursor) u8() (uint8, error) {
	if c.remaining() < 1 {
		return 0, errors.New("eof u8")
	}
	v := c.buf[c.off]
	c.off++
	return v, nil
}

func (c *cursor) u16() (uint16, error) {
	if c.remaining() < 2 {
		return 0, errors.New("eof u16")
	}
	v := binary.BigEndian.Uint16(c.buf[c.off:])
	c.off += 2
	return v, nil
}

func (c *cursor) u24() (uint32, error) {
	if c.remaining() < 3 {
		return 0, errors.New("eof u24")
	}
	v := uint32(c.buf[c.off])<<16 | uint32(c.buf[c.off+1])<<8 | uint32(c.buf[c.off+2])
	c.off += 3
	return v, nil
}

func (c *cursor) skip(n int) error {
	if n < 0 || c.remaining() < n {
		return errors.New("eof skip")
	}
	c.off += n
	return nil
}
