//go:build !windows

package sysproxy

import "errors"

// stubManager existe só para permitir build cross-plataforma e roda em
// modo no-op. Em não-Windows, o Apply/Restore não fazem nada e Current
// devolve um Settings zero — o que é correto pois esse binário só faz
// sentido como Windows Service.
type stubManager struct{}

func NewManager() Manager { return stubManager{} }

func (stubManager) Apply(Settings) error          { return errors.New("sysproxy: suportado apenas em Windows") }
func (stubManager) Restore() error                { return nil }
func (stubManager) Current() (Settings, error)    { return Settings{}, nil }
