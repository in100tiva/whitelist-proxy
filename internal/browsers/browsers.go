// Package browsers detecta navegadores instalados e configura o proxy neles.
package browsers

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Browser representa um navegador detectado/configurável.
type Browser struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Detected   bool   `json:"detected"`
	Configured bool   `json:"configured"`
	Error      string `json:"error,omitempty"`
}

// Manager gerencia detecção e configuração de navegadores.
type Manager struct {
	host string
	port int
}

// New cria um Manager. addr deve ser no formato "host:port" (ex.: "127.0.0.1:8888").
func New(addr string) *Manager {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		host = "127.0.0.1"
		portStr = "8888"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 8888
	}
	return &Manager{host: host, port: port}
}

// Detect retorna a lista de navegadores conhecidos com status de detecção e configuração.
func (m *Manager) Detect() []Browser {
	return []Browser{
		m.firefoxStatus(),
		m.chromiumFamilyStatus(),
	}
}

// Configure configura o proxy no navegador identificado por id.
func (m *Manager) Configure(id string) error {
	switch id {
	case "firefox":
		return m.configureFirefox()
	case "chromium-family":
		return m.configureGnome()
	default:
		return fmt.Errorf("navegador desconhecido: %s", id)
	}
}

// ConfigureAll configura o proxy em todos os navegadores detectados e retorna
// a lista atualizada.
func (m *Manager) ConfigureAll() []Browser {
	browsers := m.Detect()
	for i, b := range browsers {
		if !b.Detected {
			continue
		}
		if err := m.Configure(b.ID); err != nil {
			browsers[i].Error = err.Error()
		} else {
			browsers[i].Error = ""
		}
	}
	// Atualiza status após configuração.
	return m.Detect()
}

// --------------- Firefox ---------------

func (m *Manager) firefoxStatus() Browser {
	b := Browser{
		ID:   "firefox",
		Name: "Firefox",
	}
	b.Detected = isFirefoxInstalled()
	if b.Detected {
		b.Configured = isFirefoxConfigured()
	}
	return b
}

func isFirefoxInstalled() bool {
	for _, bin := range []string{"firefox", "firefox-esr"} {
		if p, err := exec.LookPath(bin); err == nil && p != "" {
			return true
		}
	}
	// Verifica também se o diretório de perfis existe.
	mozDir := firefoxMozillaDir()
	if _, err := os.Stat(mozDir); err == nil {
		return true
	}
	return false
}

func isFirefoxConfigured() bool {
	profiles := firefoxProfiles()
	if len(profiles) == 0 {
		return false
	}
	for _, p := range profiles {
		userJS := filepath.Join(p, "user.js")
		data, err := os.ReadFile(userJS)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "Whitelist Proxy") {
			return true
		}
	}
	return false
}

func (m *Manager) configureFirefox() error {
	profiles := firefoxProfiles()
	if len(profiles) == 0 {
		return fmt.Errorf("nenhum perfil Firefox encontrado em %s", firefoxMozillaDir())
	}
	content := m.firefoxUserJS()
	var lastErr error
	configured := 0
	for _, p := range profiles {
		userJS := filepath.Join(p, "user.js")
		if err := os.WriteFile(userJS, []byte(content), 0o644); err != nil {
			lastErr = err
			continue
		}
		configured++
	}
	if configured == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}

func (m *Manager) firefoxUserJS() string {
	return fmt.Sprintf(`// Configurado automaticamente pelo Whitelist Proxy
user_pref("network.proxy.type", 1);
user_pref("network.proxy.http", "%s");
user_pref("network.proxy.http_port", %d);
user_pref("network.proxy.ssl", "%s");
user_pref("network.proxy.ssl_port", %d);
user_pref("network.proxy.no_proxies_on", "127.0.0.1,localhost");
`, m.host, m.port, m.host, m.port)
}

// firefoxMozillaDir retorna o primeiro diretório de perfis Firefox encontrado.
// Firefox pode instalar perfis em ~/.mozilla/firefox (pacote nativo) ou
// ~/.config/mozilla/firefox (Flatpak/snap em modo confinado).
func firefoxMozillaDir() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".mozilla", "firefox"),
		filepath.Join(home, ".config", "mozilla", "firefox"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
	}
	for _, d := range candidates {
		if _, err := os.Stat(d); err == nil {
			return d
		}
	}
	return filepath.Join(home, ".mozilla", "firefox")
}

// firefoxProfiles retorna os caminhos de perfis válidos (contêm prefs.js).
func firefoxProfiles() []string {
	mozDir := firefoxMozillaDir()
	entries, err := os.ReadDir(mozDir)
	if err != nil {
		return nil
	}
	var profiles []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		profilePath := filepath.Join(mozDir, e.Name())
		if _, err := os.Stat(filepath.Join(profilePath, "prefs.js")); err == nil {
			profiles = append(profiles, profilePath)
		}
	}
	return profiles
}

// --------------- Chromium family (via GNOME gsettings) ---------------

func (m *Manager) chromiumFamilyStatus() Browser {
	b := Browser{
		ID:   "chromium-family",
		Name: "Chrome / Chromium / Brave / Edge",
	}
	b.Detected = isChromiumFamilyInstalled()
	if b.Detected {
		b.Configured = isGnomeConfigured()
	}
	return b
}

func isChromiumFamilyInstalled() bool {
	binaries := []string{
		"google-chrome",
		"chromium",
		"chromium-browser",
		"brave-browser",
		"microsoft-edge",
	}
	for _, bin := range binaries {
		if p, err := exec.LookPath(bin); err == nil && p != "" {
			return true
		}
	}
	// Verifica diretórios de configuração.
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".config", "google-chrome"),
		filepath.Join(home, ".config", "chromium"),
		filepath.Join(home, ".config", "BraveSoftware"),
		filepath.Join(home, ".config", "microsoft-edge"),
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			return true
		}
	}
	return false
}

func isGnomeConfigured() bool {
	out, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "'manual'"
}

func (m *Manager) configureGnome() error {
	portStr := strconv.Itoa(m.port)
	cmds := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
		{"gsettings", "set", "org.gnome.system.proxy.http", "host", m.host},
		{"gsettings", "set", "org.gnome.system.proxy.http", "port", portStr},
		{"gsettings", "set", "org.gnome.system.proxy.https", "host", m.host},
		{"gsettings", "set", "org.gnome.system.proxy.https", "port", portStr},
		{"gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", "['127.0.0.1', 'localhost']"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("gsettings: %w — %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
