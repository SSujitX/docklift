//go:build !linux

package system

import (
	"context"
	"log/slog"
)

// unsupportedHost is the Host of non-Linux builds. Windows and macOS never
// reach it (the routes simulate the action there, as 2.x did).
type unsupportedHost struct{}

func newHost(context.Context, *slog.Logger) Host { return unsupportedHost{} }

func (unsupportedHost) Reboot() error       { return ErrHostActionsUnsupported }
func (unsupportedHost) UpdateSystem() error { return ErrHostActionsUnsupported }
func (unsupportedHost) Upgrade() error      { return ErrHostActionsUnsupported }
