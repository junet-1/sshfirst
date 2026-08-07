package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) ListWorkspaces() ([]string, error) {
	if a.workspaces == nil {
		return nil, fmt.Errorf("workspace storage is not available")
	}
	return a.workspaces.List()
}

func (a *App) LoadWorkspace(name string) (string, error) {
	if a.workspaces == nil {
		return "", fmt.Errorf("workspace storage is not available")
	}
	return a.workspaces.Load(name)
}

func (a *App) SaveWorkspace(name, contents string) error {
	if a.workspaces == nil {
		return fmt.Errorf("workspace storage is not available")
	}
	return a.workspaces.Save(name, contents)
}

func (a *App) DeleteWorkspace(name string) error {
	if a.workspaces == nil {
		return fmt.Errorf("workspace storage is not available")
	}
	return a.workspaces.Delete(name)
}

func (a *App) ImportWorkspaceJSON(ctx context.Context) (string, error) {
	path, err := a.openFileDialog(ctx).
		SetTitle("Import Workspace").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		AddFilter("SSH First Workspaces", "*.json").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read workspace: %w", err)
	}
	return string(contents), nil
}

func (a *App) ExportWorkspaceJSON(ctx context.Context, defaultFilename, contents string) (string, error) {
	if defaultFilename == "" {
		defaultFilename = "workspace.json"
	}
	dialog := a.ui.Dialog.SaveFile()
	dialog.SetOptions(&application.SaveFileDialogOptions{
		Title:                "Export Workspace",
		Filename:             filepath.Base(defaultFilename),
		CanCreateDirectories: true,
		Filters: []application.FileFilter{
			{DisplayName: "SSH First Workspaces", Pattern: "*.json"},
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
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return "", fmt.Errorf("write workspace: %w", err)
	}
	return path, nil
}
