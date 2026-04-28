//go:build windows

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/in100tiva/goproxy/whitelist-proxy/internal/sysproxy"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// runService é o entry point quando o binário é invocado pelo Service
// Control Manager (sem argumentos). svc.Run bloqueia até o serviço parar.
func runService() {
	isService, err := svc.IsWindowsService()
	if err != nil {
		fmt.Fprintln(os.Stderr, "svc.IsWindowsService:", err)
		os.Exit(1)
	}
	if !isService {
		// Sem args e não é serviço: o usuário rodou o exe sem nada.
		// Mostra ajuda em vez de tentar registrar handlers do SCM.
		printUsage()
		return
	}
	if err := svc.Run(serviceName, &winService{}); err != nil {
		fmt.Fprintln(os.Stderr, "svc.Run:", err)
		os.Exit(1)
	}
}

// winService implementa svc.Handler. Toda lógica pesada delega ao runtime.
type winService struct{}

func (s *winService) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}

	rt, err := startRuntime()
	if err != nil {
		// Se não conseguimos subir, devolve erro específico para o SCM.
		status <- svc.Status{State: svc.Stopped}
		return false, 1
	}
	status <- svc.Status{State: svc.Running, Accepts: accepted}

loop:
	for c := range req {
		switch c.Cmd {
		case svc.Interrogate:
			status <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			break loop
		}
	}

	status <- svc.Status{State: svc.StopPending}
	rt.stop()
	status <- svc.Status{State: svc.Stopped}
	return false, 0
}

// installService registra o binário no SCM e configura auto-start.
func installService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w (rode como administrador)", err)
	}
	defer m.Disconnect()

	if existing, err := m.OpenService(serviceName); err == nil {
		existing.Close()
		return fmt.Errorf("serviço %q já existe — use uninstall primeiro", serviceName)
	}

	cfg := mgr.Config{
		StartType:        mgr.StartAutomatic,
		DisplayName:      "Whitelist Proxy",
		Description:      serviceDesc,
		ServiceStartName: "", // LocalSystem
	}
	srv, err := m.CreateService(serviceName, exe, cfg)
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	defer srv.Close()

	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w (rode como administrador)", err)
	}
	defer m.Disconnect()

	srv, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("OpenService: %w", err)
	}
	defer srv.Close()

	// Tenta parar se estiver rodando — ignora erro se já parado.
	_, _ = srv.Control(svc.Stop)
	// Espera curta antes de remover; se não parar, deleta mesmo assim
	// (o Windows agenda a remoção para depois).
	for i := 0; i < 10; i++ {
		st, err := srv.Query()
		if err != nil || st.State == svc.Stopped {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	return srv.Delete()
}

func startService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w", err)
	}
	defer m.Disconnect()

	srv, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("OpenService: %w", err)
	}
	defer srv.Close()

	return srv.Start()
}

func stopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("mgr.Connect: %w", err)
	}
	defer m.Disconnect()

	srv, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("OpenService: %w", err)
	}
	defer srv.Close()

	st, err := srv.Control(svc.Stop)
	if err != nil {
		return err
	}
	for i := 0; i < 30 && st.State != svc.Stopped; i++ {
		time.Sleep(300 * time.Millisecond)
		st, err = srv.Query()
		if err != nil {
			return err
		}
	}
	return nil
}

func printStatus() error {
	m, err := mgr.Connect()
	if err == nil {
		defer m.Disconnect()
		if srv, err := m.OpenService(serviceName); err == nil {
			defer srv.Close()
			if st, err := srv.Query(); err == nil {
				fmt.Println("Serviço:", stateName(st.State))
			} else {
				fmt.Println("Serviço: erro ao consultar:", err)
			}
		} else {
			fmt.Println("Serviço: NÃO INSTALADO")
		}
	} else {
		fmt.Println("Serviço: SCM indisponível:", err)
	}

	sp := sysproxy.NewManager()
	if cur, err := sp.Current(); err == nil {
		fmt.Printf("Proxy do sistema: enabled=%v server=%q bypass=%q\n",
			cur.Enabled, cur.Server, cur.Bypass)
	} else {
		fmt.Println("Proxy do sistema: erro ao ler registro:", err)
	}
	return nil
}

func stateName(s svc.State) string {
	switch s {
	case svc.Stopped:
		return "Stopped"
	case svc.StartPending:
		return "StartPending"
	case svc.StopPending:
		return "StopPending"
	case svc.Running:
		return "Running"
	case svc.ContinuePending:
		return "ContinuePending"
	case svc.PausePending:
		return "PausePending"
	case svc.Paused:
		return "Paused"
	default:
		return fmt.Sprintf("State(%d)", s)
	}
}
