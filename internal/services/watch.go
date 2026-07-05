package services

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UNSAReport/UNSAReport/internal/ports"
)

// WatchService runs typst watch for live preview of a lab report.
type WatchService struct {
	Compiler ports.Compiler
	FS       ports.FileSystem
	Config   ports.ConfigStore
	Stdout   io.Writer
	Stderr   io.Writer
}

// WatchOption configures a WatchService via functional options.
type WatchOption func(*WatchService)

// WithWatchCompiler sets the typst compiler used to watch the report.
func WithWatchCompiler(c ports.Compiler) WatchOption {
	return func(s *WatchService) { s.Compiler = c }
}

// WithWatchFS sets the filesystem used for file operations during watch.
func WithWatchFS(fs ports.FileSystem) WatchOption {
	return func(s *WatchService) { s.FS = fs }
}

// WithWatchConfig sets the configuration store for reading project settings.
func WithWatchConfig(cfg ports.ConfigStore) WatchOption {
	return func(s *WatchService) { s.Config = cfg }
}

// WithWatchStdout sets the writer for standard output messages.
func WithWatchStdout(w io.Writer) WatchOption {
	return func(s *WatchService) { s.Stdout = w }
}

// WithWatchStderr sets the writer for standard error messages.
func WithWatchStderr(w io.Writer) WatchOption {
	return func(s *WatchService) { s.Stderr = w }
}

// NewWatchService creates a WatchService with the given functional options applied.
func NewWatchService(opts ...WatchOption) *WatchService {
	s := &WatchService{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type watchContext struct {
	projectRoot string
	cfg         ports.UnsareportConfig
	isMulti     bool
	labDir      string
}

// Execute starts typst watch for live recompilation of the report.
func (s *WatchService) Execute(ctx context.Context, labDirArg string) error {
	cwd, err := s.FS.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}

	wctx, err := s.resolveWatchContext(cwd, labDirArg)
	if err != nil {
		return fmt.Errorf("resolve context: %w", err)
	}

	if err := s.FS.Chdir(wctx.projectRoot); err != nil {
		return fmt.Errorf("chdir to project root: %w", err)
	}

	reportPath := wctx.cfg.Prepare.Input.ReportFile
	reportPDF := strings.TrimSuffix(wctx.cfg.Prepare.Input.ReportFile, filepath.Ext(wctx.cfg.Prepare.Input.ReportFile)) + ".pdf"
	if wctx.isMulti {
		reportPath = filepath.Join(wctx.labDir, wctx.cfg.Prepare.Input.ReportFile)
		reportPDF = filepath.Join(wctx.labDir, strings.TrimSuffix(wctx.cfg.Prepare.Input.ReportFile, filepath.Ext(wctx.cfg.Prepare.Input.ReportFile))+".pdf")
	}

	if !s.FS.FileExists(reportPath) {
		return fmt.Errorf("report file not found: %s", reportPath)
	}

	inputs := map[string]string{"unsarep-root": "/"}

	if _, err := fmt.Fprintln(s.Stdout, "Watching for changes..."); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	if err := s.Compiler.Watch(ctx, reportPath, reportPDF, inputs); err != nil {
		return fmt.Errorf("watch report: %w", err)
	}
	return nil
}

func (s *WatchService) resolveWatchContext(cwd, labDirArg string) (watchContext, error) {
	projectRoot, cfg, ok, err := s.Config.FindProjectRoot(cwd)
	if err != nil {
		return watchContext{}, fmt.Errorf("find project root: %w", err)
	}

	if !ok {
		return watchContext{}, fmt.Errorf("unsareport.json not found in current or parent directories.\nAre you in a lab report project?")
	}

	wctx := watchContext{
		projectRoot: projectRoot,
		cfg:         cfg,
		isMulti:     cfg.Mode == "multi",
	}

	if !wctx.isMulti {
		if labDirArg != "" {
			return wctx, fmt.Errorf("lab argument provided but template is not multi-mode")
		}
		return wctx, nil
	}

	if labDirArg != "" {
		wctx.labDir = labDirArg
	} else {
		rel, err := filepath.Rel(projectRoot, cwd)
		if err != nil {
			return wctx, fmt.Errorf("could not determine relative path: %w", err)
		}

		if rel == "." {
			return wctx, fmt.Errorf("in a multi-mode project, you must either provide a lab directory or run this command from inside a lab directory")
		}

		parts := strings.Split(filepath.ToSlash(rel), "/")
		wctx.labDir = parts[0]
	}

	sessionValid := slices.Contains(wctx.cfg.Sessions, wctx.labDir)
	if !sessionValid {
		return wctx, fmt.Errorf("session '%s' is not registered in unsareport.json", wctx.labDir)
	}

	return wctx, nil
}
