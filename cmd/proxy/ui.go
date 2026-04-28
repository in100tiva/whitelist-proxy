package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// openUI imprime a URL completa da UI (com o token na query) e tenta abrir
// no navegador padrão. Não exige que o serviço esteja rodando para imprimir
// — só lê o admin.token ao lado do binário.
func openUI() error {
	token := time.Now().Format("1504") // token = HHMM do horário atual
	url := "http://" + adminAddr + "/?t=" + token

	fmt.Println("UI:", url)
	fmt.Println("(token = horário atual HHMM; muda a cada minuto)")

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
