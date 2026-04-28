// Package sysproxy controla as configurações de proxy do sistema operacional.
//
// A implementação real está em windows.go (build tag windows). Em outros SOs
// existe um stub apenas para o build cross-plataforma — o uso pretendido é
// rodar como Windows Service.
package sysproxy

// Settings representa o estado de proxy que persistimos no registro do Windows.
type Settings struct {
	Enabled bool   // ProxyEnable
	Server  string // ProxyServer (ex.: "127.0.0.1:8080")
	Bypass  string // ProxyOverride (ex.: "<local>;127.0.0.1")
}

// Manager é a interface para aplicar e reverter as configurações.
type Manager interface {
	// Apply ativa o proxy do sistema apontando para Settings, salvando o
	// estado atual para que Restore possa revertê-lo depois.
	Apply(s Settings) error
	// Restore volta para o estado anterior (capturado em Apply).
	Restore() error
	// Current devolve o estado atual do registro (apenas leitura).
	Current() (Settings, error)
}
