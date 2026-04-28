//go:build !windows

package main

import "fmt"

// Stubs para builds não-Windows. Apenas o subcomando "run" é útil fora
// do Windows; os demais existem só para o código compilar e o usuário
// receber uma mensagem clara de incompatibilidade.

func runService() {
	fmt.Println("Modo serviço só é suportado em Windows. Use './proxy run'.")
}

func installService() error   { return fmt.Errorf("install: suportado apenas em Windows") }
func uninstallService() error { return fmt.Errorf("uninstall: suportado apenas em Windows") }
func startService() error     { return fmt.Errorf("start: suportado apenas em Windows") }
func stopService() error      { return fmt.Errorf("stop: suportado apenas em Windows") }
func printStatus() error {
	fmt.Println("status: suportado apenas em Windows.")
	return nil
}
