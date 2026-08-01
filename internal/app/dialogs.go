package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) ConfirmDialog(ctx context.Context, title, message, confirmLabel, cancelLabel string) (bool, error) {
	if a.ui == nil {
		return false, nil
	}
	result := make(chan bool, 1)
	dialog := a.ui.Dialog.Question().SetTitle(title).SetMessage(message)
	if window := a.callingWindow(ctx); window != nil {
		dialog.AttachToWindow(window)
	}
	dialog.AddButton(confirmLabel).SetAsDefault().OnClick(func() { result <- true })
	dialog.AddButton(cancelLabel).SetAsCancel().OnClick(func() { result <- false })
	dialog.Show()
	return <-result, nil
}

func (a *App) PickDirectory(ctx context.Context) (string, error) {
	return a.openFileDialog(ctx).
		SetTitle("Select Local Folder").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
}

func (a *App) PickFile(ctx context.Context) (string, error) {
	return a.openFileDialog(ctx).
		SetTitle("Select Local File").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		PromptForSingleSelection()
}

func (a *App) ExportTerminalOutput(ctx context.Context, defaultFilename, content string) (string, error) {
	if defaultFilename == "" {
		defaultFilename = "terminal-output.txt"
	}
	dialog := a.ui.Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{
		Title:                "Export Terminal Output",
		Filename:             filepath.Base(defaultFilename),
		CanCreateDirectories: true,
		Filters: []application.FileFilter{
			{DisplayName: "Text Files", Pattern: "*.txt;*.log"},
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
	if window := a.callingWindow(ctx); window != nil {
		dialog.AttachToWindow(window)
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil || path == "" {
		return path, err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) PickIdentityFiles(ctx context.Context) ([]string, error) {
	dir := ""
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, ".ssh")
	}
	files, err := a.openFileDialog(ctx).
		SetTitle("Select Identity File(s)").
		SetDirectory(dir).
		CanChooseFiles(true).
		CanChooseDirectories(false).
		PromptForMultipleSelection()
	if files == nil {
		files = []string{}
	}
	return files, err
}

func (a *App) openFileDialog(ctx context.Context) *application.OpenFileDialogStruct {
	dialog := a.ui.Dialog.OpenFile()
	if window := a.callingWindow(ctx); window != nil {
		dialog.AttachToWindow(window)
	}
	return dialog
}

func (a *App) callingWindow(ctx context.Context) application.Window {
	if ctx != nil {
		if window, ok := ctx.Value(application.WindowKey).(application.Window); ok {
			return window
		}
	}
	return a.mainWindow
}
