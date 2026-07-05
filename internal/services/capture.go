package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/UNSAReport/UNSAReport/internal/ports"
)

// CaptureService orchestrates terminal session rendering into static images.
type CaptureService struct {
	Renderer ports.Renderer
	FS       ports.FileSystem
	Config   ports.ConfigStore
	Stdout   io.Writer
	Stderr   io.Writer
}

// CaptureOption configures a CaptureService via functional options.
type CaptureOption func(*CaptureService)

// WithCaptureRenderer sets the renderer used to produce capture images.
func WithCaptureRenderer(r ports.Renderer) CaptureOption {
	return func(s *CaptureService) { s.Renderer = r }
}

// WithCaptureFS sets the filesystem used for file operations during capture.
func WithCaptureFS(fs ports.FileSystem) CaptureOption {
	return func(s *CaptureService) { s.FS = fs }
}

// WithCaptureConfig sets the configuration store for reading project settings.
func WithCaptureConfig(c ports.ConfigStore) CaptureOption {
	return func(s *CaptureService) { s.Config = c }
}

// WithCaptureStdout sets the writer for standard output messages.
func WithCaptureStdout(w io.Writer) CaptureOption {
	return func(s *CaptureService) { s.Stdout = w }
}

// WithCaptureStderr sets the writer for standard error messages.
func WithCaptureStderr(w io.Writer) CaptureOption {
	return func(s *CaptureService) { s.Stderr = w }
}

// NewCaptureService creates a CaptureService with the given functional options applied.
func NewCaptureService(opts ...CaptureOption) *CaptureService {
	s := &CaptureService{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CaptureOptions holds the parameters for a single capture execution.
type CaptureOptions struct {
	Cwd             string
	Args            []string
	FreezeFlags     []string
	SaveFreezeFlags bool
}

// Execute runs the capture pipeline: parses instructions, renders them via the terminal, and writes the resulting image.
func (s *CaptureService) Execute(ctx context.Context, opts CaptureOptions) error {
	cwd, err := s.FS.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}

	projectRoot, cfg, ok, err := s.Config.FindProjectRoot(cwd)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	if !ok {
		projectRoot = cwd
		cfg, _, err = s.Config.ReadConfig(cwd)
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}
	}

	if opts.SaveFreezeFlags {
		cfg.Capture.FreezeFlags = opts.FreezeFlags
		if err := s.Config.WriteConfig(projectRoot, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	if len(opts.Args) < 1 {
		return fmt.Errorf("result image path is required")
	}
	resultPath := opts.Args[0]
	instructions := opts.Args[1:]

	if err := s.FS.EnsureDir(filepath.Dir(resultPath)); err != nil {
		return fmt.Errorf("ensure result directory: %w", err)
	}

	var commands []ports.CaptureCommand

	if opts.Cwd != "" {
		absCwd, err := filepath.Abs(opts.Cwd)
		if err != nil {
			return fmt.Errorf("invalid cwd path: %w", err)
		}
		info, err := s.FS.Stat(absCwd)
		if err != nil {
			return fmt.Errorf("cwd path not accessible: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("cwd path is not a directory: %s", absCwd)
		}
		safeCwd := shellQuotePath(absCwd)
		commands = append(commands, ports.CaptureCommand{Type: "Type", Args: "cd " + safeCwd})
		commands = append(commands, ports.CaptureCommand{Type: "Enter"})
		commands = append(commands, ports.CaptureCommand{Type: "Type", Args: "clear"})
		commands = append(commands, ports.CaptureCommand{Type: "Enter"})
	}

	for _, instr := range instructions {
		if _, err := fmt.Fprintf(s.Stdout, "Capturing instruction: %s\n", instr); err != nil {
			return fmt.Errorf("write instruction: %w", err)
		}
		if after, ok := strings.CutPrefix(instr, "w:"); ok {
			d, err := time.ParseDuration(after)
			if err != nil {
				d, err = time.ParseDuration(after + "ms")
				if err != nil {
					d, err = time.ParseDuration(after + "s")
				}
			}
			if err == nil {
				commands = append(commands, ports.CaptureCommand{Type: "Sleep", Delay: d})
				continue
			}
		}

		if after, ok := strings.CutPrefix(instr, "r:"); ok {
			commands = append(commands, ports.CaptureCommand{Type: "Raw", Args: after})
			continue
		}

		if after, ok := strings.CutPrefix(instr, "c:"); ok {
			commands = append(commands, ports.CaptureCommand{Type: "Ctrl", Args: after})
			continue
		}

		if after, ok := strings.CutPrefix(instr, "k:"); ok {
			commands = append(commands, ports.CaptureCommand{Type: "Key", Args: after})
			continue
		}

		if after, ok := strings.CutPrefix(instr, "x:"); ok {
			commands = append(commands, ports.CaptureCommand{Type: "CommandStart", Args: after})
			continue
		}

		commands = append(commands, ports.CaptureCommand{Type: "Command", Args: instr})
	}

	finalFlags := cfg.Capture.FreezeFlags
	if !opts.SaveFreezeFlags {
		finalFlags = append(cfg.Capture.FreezeFlags, opts.FreezeFlags...)
	}

	output, err := s.Renderer.Render(ctx, resultPath, commands, finalFlags, cfg.Capture)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	if err := s.FS.EnsureDir("capture_logs"); err == nil {
		timestamp := time.Now().Format("02-01-2006_15-04-05")
		logPath := filepath.Join("capture_logs", timestamp+".log")
		if err := s.FS.WriteFileAtomic(logPath, []byte(output), 0644); err != nil {
			slog.Warn("failed to write capture log", "path", logPath, "error", err)
		}
	}

	return nil
}

// shellQuotePath quotes a file path for safe use in shell commands.
// It wraps the path in single quotes, escaping any embedded single quotes.
func shellQuotePath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
