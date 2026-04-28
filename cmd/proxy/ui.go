package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// openUI imprime a URL completa da UI (com o token na query) e tenta abrir
// no navegador padrão. Não exige que o serviço esteja rodando para imprimir
// — só lê o admin.token ao lado do binário.
func openUI() error {
	tokenPath := stableTokenPath()
	tokBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("não consegui ler %s: %w (o serviço já foi iniciado pelo menos uma vez?)", tokenPath, err)
	}
	token := strings.TrimSpace(string(tokBytes))
	url := "http://" + adminAddr + "/?t=" + token

	fmt.Println("UI:", url)
	fmt.Println("(o token é salvo no navegador no primeiro acesso)")

	if err := openBrowser(url); err != nil {
		fmt.Println("Não foi possível abrir o navegador automaticamente:", err)
		fmt.Println("Abra manualmente o link acima.")
	}
	return nil
}

// openBrowser abre uma URL no navegador padrão do sistema.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		// rundll32 evita problemas de quoting com cmd /c start.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
