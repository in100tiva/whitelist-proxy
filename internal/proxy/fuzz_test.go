package proxy

import (
	"bytes"
	"testing"
)

// FuzzParseSNI alimenta o parser de ClientHello com bytes arbitrários. Esses
// bytes vêm da rede antes de qualquer validação, então o parser não pode
// entrar em pânico, estourar buffer nem inventar dados: quando devolve um
// nome, ele precisa ser exatamente um trecho da entrada.
func FuzzParseSNI(f *testing.F) {
	seeds := [][]byte{
		nil,
		{},
		{22},
		{22, 3, 1, 0, 0},
		{22, 3, 1, 0, 44, 1, 0, 0, 40, 3, 3, 0, 1, 2, 3},
		append([]byte{22, 3, 1, 0, 47, 1, 0, 0, 43}, make([]byte, 43)...),
		[]byte("GET / HTTP/1.1\r\nHost: exemplo.com\r\n\r\n"),
		[]byte{0x16, 0x03, 0x01, 0xff, 0xff, 0x01, 0xff, 0xff, 0xff},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		name, err := ParseSNI(data)
		if err != nil {
			if name != "" {
				t.Fatalf("erro %v com nome não vazio %q", err, name)
			}
			return
		}
		if name != "" && !bytes.Contains(data, []byte(name)) {
			t.Fatalf("nome %q não é trecho da entrada", name)
		}
	})
}
