package main

import (
	"context"

	wa "whatsapp-desktop/internal/whatsapp"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds the application state and lifecycle hooks
type App struct {
	ctx context.Context
	wa  *wa.WhatsAppService
}

// PickAttachments opens the native picker and immediately stages its results.
// File paths never need to be passed through React state.
func (a *App) PickAttachments(kind string) ([]wa.AttachmentDraft, error) {
	filters := []runtime.FileFilter{{DisplayName: "Semua file", Pattern: "*"}}
	switch kind {
	case "media":
		filters = []runtime.FileFilter{{DisplayName: "Foto & video", Pattern: "*.jpg;*.jpeg;*.png;*.webp;*.gif;*.mp4;*.mov;*.mkv;*.3gp"}}
	case "audio":
		filters = []runtime.FileFilter{{DisplayName: "Audio", Pattern: "*.ogg;*.opus;*.mp3;*.m4a;*.wav;*.webm"}}
	case "document":
		filters = []runtime.FileFilter{{DisplayName: "Dokumen", Pattern: "*"}}
	}
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Filters: filters})
	if err != nil || len(paths) == 0 {
		return []wa.AttachmentDraft{}, err
	}
	return a.wa.StageAttachments(paths, kind)
}

// StageDroppedAttachments receives paths resolved by Wails' native drop API.
func (a *App) StageDroppedAttachments(paths []string) ([]wa.AttachmentDraft, error) {
	return a.wa.StageAttachments(paths, "auto")
}

func (a *App) StageCapturedMedia(dataURL, fileName, kind string, isPTT bool) (wa.AttachmentDraft, error) {
	return a.wa.StageDataURL(dataURL, fileName, kind, isPTT)
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
