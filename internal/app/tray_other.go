//go:build !linux && !darwin

package app

type unavailableTray struct{}

func newTrayService(*App, []byte) trayService { return unavailableTray{} }
func (unavailableTray) Start() bool           { return false }
func (unavailableTray) Refresh()              {}
func (unavailableTray) Stop()                 {}
