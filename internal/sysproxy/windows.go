//go:build windows

package sysproxy

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// Caminho onde o IE/Edge/Chrome (configuração padrão de proxy do Windows)
// lê o estado de proxy do usuário corrente.
const internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// WinManager implementa Manager mexendo no registro do Windows e notificando
// o WinINet (via InternetSetOption) para que as mudanças sejam aplicadas
// imediatamente, sem precisar reabrir o navegador.
type WinManager struct {
	saved   Settings
	hasSave bool
}

func NewManager() Manager { return &WinManager{} }

// Current lê o estado atual do registro.
func (m *WinManager) Current() (Settings, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.QUERY_VALUE)
	if err != nil {
		return Settings{}, fmt.Errorf("abrindo registry: %w", err)
	}
	defer k.Close()

	var s Settings
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		s.Enabled = v != 0
	}
	if v, _, err := k.GetStringValue("ProxyServer"); err == nil {
		s.Server = v
	}
	if v, _, err := k.GetStringValue("ProxyOverride"); err == nil {
		s.Bypass = v
	}
	return s, nil
}

// Apply salva o estado atual e aplica o novo.
func (m *WinManager) Apply(s Settings) error {
	cur, err := m.Current()
	if err != nil {
		return err
	}
	m.saved = cur
	m.hasSave = true
	return writeSettings(s)
}

// Restore reverte para o estado capturado por Apply. É idempotente.
func (m *WinManager) Restore() error {
	if !m.hasSave {
		return nil
	}
	if err := writeSettings(m.saved); err != nil {
		return err
	}
	m.hasSave = false
	return nil
}

func writeSettings(s Settings) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("abrindo registry para escrita: %w", err)
	}
	defer k.Close()

	enabled := uint32(0)
	if s.Enabled {
		enabled = 1
	}
	if err := k.SetDWordValue("ProxyEnable", enabled); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", s.Server); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", s.Bypass); err != nil {
		return err
	}

	// Avisa o WinINet para recarregar as configurações imediatamente.
	// Sem essas duas chamadas, o navegador só pega a mudança ao reiniciar.
	return notifyWinINet()
}

// Constantes do WinINet (wininet.h):
//
//	INTERNET_OPTION_SETTINGS_CHANGED  = 39
//	INTERNET_OPTION_REFRESH           = 37
const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

func notifyWinINet() error {
	wininet, err := syscall.LoadDLL("wininet.dll")
	if err != nil {
		return fmt.Errorf("LoadDLL wininet: %w", err)
	}
	defer wininet.Release()

	proc, err := wininet.FindProc("InternetSetOptionW")
	if err != nil {
		return fmt.Errorf("FindProc InternetSetOptionW: %w", err)
	}
	// InternetSetOptionW(hInternet=NULL, dwOption, lpBuffer=NULL, dwBufferLength=0)
	if r, _, callErr := proc.Call(0, uintptr(internetOptionSettingsChanged), uintptr(unsafe.Pointer(nil)), 0); r == 0 {
		return fmt.Errorf("InternetSetOptionW(SETTINGS_CHANGED) falhou: %v", callErr)
	}
	if r, _, callErr := proc.Call(0, uintptr(internetOptionRefresh), uintptr(unsafe.Pointer(nil)), 0); r == 0 {
		return fmt.Errorf("InternetSetOptionW(REFRESH) falhou: %v", callErr)
	}
	return nil
}
