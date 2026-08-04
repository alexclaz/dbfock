package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// DesktopFiles exposes native save dialogs to the frontend, which otherwise has
// no way to write files: the webview silently ignores anchor downloads.
type DesktopFiles struct {
	mu  sync.RWMutex
	ctx context.Context
}

func (f *DesktopFiles) setContext(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctx = ctx
}

func (f *DesktopFiles) context() (context.Context, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ctx, f.ctx != nil
}

// SaveTextFile asks the user where to store contents and returns the chosen
// path, or an empty string when the dialog is cancelled.
func (f *DesktopFiles) SaveTextFile(defaultName string, contents string) (string, error) {
	ctx, ok := f.context()
	if !ok {
		return "", fmt.Errorf("desktop window is not ready")
	}

	path, err := wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{
		Title:                "DBfock",
		DefaultFilename:      defaultName,
		Filters:              saveDialogFilters(defaultName),
		CanCreateDirectories: true,
	})
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return path, nil
}

func saveDialogFilters(defaultName string) []wailsruntime.FileFilter {
	extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(defaultName), "."))
	names := map[string]string{"sql": "SQL dump", "csv": "CSV file", "json": "JSON file"}
	if extension == "" {
		return nil
	}
	name, ok := names[extension]
	if !ok {
		name = strings.ToUpper(extension) + " file"
	}
	return []wailsruntime.FileFilter{
		{DisplayName: fmt.Sprintf("%s (*.%s)", name, extension), Pattern: "*." + extension},
		{DisplayName: "All files (*.*)", Pattern: "*.*"},
	}
}
