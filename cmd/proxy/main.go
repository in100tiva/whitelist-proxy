// Comando proxy.exe — entry point e dispatcher de subcomandos.
//
// Subcomandos (Windows):
//
//	proxy.exe install     instala como Windows Service (auto-start)
//	proxy.exe uninstall   remove o serviço
//	proxy.exe start       inicia o serviço
//	proxy.exe stop        para o serviço
//	proxy.exe run         roda em foreground (debug, sem registrar serviço)
//	proxy.exe status      mostra estado do serviço e do proxy do sistema
//
// Em outros SOs apenas "run" funciona — útil para desenvolvimento.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const serviceName = "WhitelistProxy"
const serviceDesc = "Proxy local com whitelist de domínios"

func main() {
	if len(os.Args) < 2 {
		// Quando o Service Control Manager invoca o binário, ele NÃO passa
		// argumentos — entramos direto no modo serviço.
		runService()
		return
	}

	switch os.Args[1] {
	case "run":
		if err := runForeground(); err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}

	case "install":
		if err := installService(); err != nil {
			fmt.Fprintln(os.Stderr, "erro instalando serviço:", err)
			os.Exit(1)
		}
		fmt.Println("Serviço instalado:", serviceName)

	case "uninstall":
		if err := uninstallService(); err != nil {
			fmt.Fprintln(os.Stderr, "erro removendo serviço:", err)
			os.Exit(1)
		}
		fmt.Println("Serviço removido:", serviceName)

	case "start":
		if err := startService(); err != nil {
			fmt.Fprintln(os.Stderr, "erro iniciando serviço:", err)
			os.Exit(1)
		}
		fmt.Println("Serviço iniciado.")

	case "stop":
		if err := stopService(); err != nil {
			fmt.Fprintln(os.Stderr, "erro parando serviço:", err)
			os.Exit(1)
		}
		fmt.Println("Serviço parado.")

	case "status":
		if err := printStatus(); err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}

	case "ui":
		if err := openUI(); err != nil {
			fmt.Fprintln(os.Stderr, "erro:", err)
			os.Exit(1)
		}

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintln(os.Stderr, "subcomando desconhecido:", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	exe := filepath.Base(os.Args[0])
	fmt.Printf(`Uso: %s <subcomando>

Subcomandos:
  run         executa em foreground (debug)
  install     instala como Windows Service (auto-start)
  uninstall   remove o serviço
  start       inicia o serviço
  stop        para o serviço
  status      mostra estado do serviço e do proxy do sistema
  ui          imprime e abre a URL da UI web (com token)
  help        mostra esta mensagem
`, exe)
}
