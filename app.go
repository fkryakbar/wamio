package main

import (
	"context"

	wa "whatsapp-desktop/internal/whatsapp"
)

// App struct holds the application state and lifecycle hooks
type App struct {
	ctx context.Context
	wa  *wa.WhatsAppService
}

// NewApp creates a new App application struct
func NewApp(waService *wa.WhatsAppService) *App {
	return &App{
		wa: waService,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.wa.SetContext(ctx)
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	if a.wa != nil {
		a.wa.Disconnect()
	}
}
