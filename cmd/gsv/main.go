// Command gsv is a read-only, vim-style TUI viewer for Google Sheets.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"gsv/internal/auth"
	"gsv/internal/config"
	"gsv/internal/gdrive"
	"gsv/internal/gsheets"
	"gsv/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gsv:", err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return err
	}
	credPath := filepath.Join(dir, "credentials.json")
	oc, err := auth.LoadOAuthConfig(credPath)
	if errors.Is(err, auth.ErrNoCredentials) {
		return fmt.Errorf("%w\nGoogle Cloud Console で作成した OAuth クライアント（デスクトップアプリ）の JSON を\n  %s\nに置いてください。手順は docs/setup.md を参照してください", err, credPath)
	}
	if err != nil {
		return err
	}

	ctx := context.Background()
	ts, err := auth.TokenSource(ctx, oc, filepath.Join(dir, "token.json"), openBrowser, os.Stderr)
	if err != nil {
		return err
	}
	httpClient := oauth2.NewClient(ctx, ts)
	dsvc, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return err
	}
	ssvc, err := sheets.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return err
	}

	m := ui.New(ui.Deps{Drive: gdrive.New(dsvc), Sheets: gsheets.New(ssvc), Shared: cfg.SharedRoots})
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func openBrowser(u string) error { return exec.Command("open", u).Start() }
