package freeze

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/UNSAReport/UNSAReport/internal/dependencies"
	"github.com/UNSAReport/UNSAReport/internal/ports"
	"github.com/aymanbagabas/go-pty"
	"github.com/samber/oops"
	"github.com/taigrr/bubbleterm/emulator"
)

var _ ports.Renderer = (*Adapter)(nil)

// Adapter implements ports.Renderer by capturing terminal sessions in a PTY and converting them to images via freeze and ImageMagick.
type Adapter struct{}

// New returns a new Adapter for terminal capture rendering.
func New() *Adapter {
	return &Adapter{}
}

// Render replays commands in a PTY, captures the terminal output, and produces an image file at resultPath.
func (a *Adapter) Render(ctx context.Context, resultPath string, commands []ports.CaptureCommand, flags []string, cfg ports.CaptureConfig) (string, error) {
	if err := dependencies.Check(dependencies.Freeze); err != nil {
		return "", err
	}
	if err := dependencies.Check(dependencies.ImageMagick); err != nil {
		return "", err
	}

	width := cfg.Columns
	height := cfg.Rows
	if height <= 0 {
		height = 500
	}

	output, err := runInPTY(ctx, commands, cfg, width, height)
	if err != nil && output == "" {
		return "", oops.Wrapf(err, "run in pty")
	}

	tempInput, err := os.CreateTemp("", "unsarep-freeze-input-*.txt")
	if err != nil {
		return output, oops.Wrapf(err, "create temp input file")
	}
	defer func() {
		if closeErr := tempInput.Close(); closeErr != nil {
			slog.Warn("failed to close temp input file", "path", tempInput.Name(), "error", closeErr)
		}
	}()
	defer func() {
		if err := os.Remove(tempInput.Name()); err != nil {
			slog.Warn("failed to remove temp input file", "path", tempInput.Name(), "error", err)
		}
	}()

	if _, err := tempInput.WriteString(output); err != nil {
		return output, oops.Wrapf(err, "write temp input file")
	}

	if filepath.Ext(resultPath) == "" {
		resultPath += ".png"
	}

	svgPath := resultPath + ".svg"
	if err := os.Remove(svgPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove old svg file", "path", svgPath, "error", err)
	}
	defer func() {
		if err := os.Remove(svgPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove svg file", "path", svgPath, "error", err)
		}
	}()

	freezeArgs := []string{
		tempInput.Name(),
		"--language", "ansi",
		"--wrap", strconv.Itoa(width),
		"--output", svgPath,
	}
	freezeArgs = append(freezeArgs, flags...)

	freezeCmd := exec.CommandContext(ctx, "freeze", freezeArgs...)
	if out, err := freezeCmd.CombinedOutput(); err != nil {
		return output, oops.With("output", string(out)).Wrapf(err, "freeze failed")
	}

	magickCmd := exec.CommandContext(ctx, "magick", svgPath, resultPath)
	if out, err := magickCmd.CombinedOutput(); err != nil {
		return output, oops.With("output", string(out)).Wrapf(err, "magick failed")
	}

	return output, nil
}

func getDefaultShell() (string, []string) {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh", []string{"-NoProfile"}
		}
		if _, err := exec.LookPath("powershell"); err == nil {
			return "powershell", []string{"-NoProfile"}
		}
		return "cmd", []string{}
	}

	shell := "bash"
	if _, err := exec.LookPath("bash"); err != nil {
		shell = "sh"
	}
	return shell, []string{"--norc", "--noprofile"}
}

func getAnsi(colors map[string]string, name string) string {
	if colors == nil {
		if name == "reset" {
			return "\x1b[0m"
		}
		return ""
	}
	code, ok := colors[name]
	if !ok {
		if name == "reset" {
			return "\x1b[0m"
		}
		return ""
	}
	if code == "0" {
		return "\x1b[0m"
	}
	return "\x1b[" + code + "m"
}

func typeColored(ptmx io.Writer, vtWrite io.Writer, cmdStr string, cfg ports.CaptureConfig) error {
	cCol := getAnsi(cfg.Colors, "command")
	aCol := getAnsi(cfg.Colors, "args")
	rCol := getAnsi(cfg.Colors, "reset")

	parts := strings.SplitN(cmdStr, " ", 2)
	firstWord := parts[0]
	var rest string
	if len(parts) > 1 {
		rest = " " + parts[1]
	}

	if _, err := vtWrite.Write([]byte(cCol)); err != nil {
		return fmt.Errorf("write command color: %w", err)
	}
	if _, err := ptmx.Write([]byte(firstWord)); err != nil {
		return fmt.Errorf("write first word: %w", err)
	}
	if rest != "" {
		time.Sleep(20 * time.Millisecond)
		if _, err := vtWrite.Write([]byte(aCol)); err != nil {
			return fmt.Errorf("write args color: %w", err)
		}
		if _, err := ptmx.Write([]byte(rest)); err != nil {
			return fmt.Errorf("write rest: %w", err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := vtWrite.Write([]byte(rCol)); err != nil {
		return fmt.Errorf("write reset color: %w", err)
	}
	return nil
}

func runInPTY(ctx context.Context, commands []ports.CaptureCommand, cfg ports.CaptureConfig, width, height int) (string, error) {
	shell, args := getDefaultShell()

	ptmx, err := pty.New()
	if err != nil {
		return "", err
	}
	defer func() {
		if err := ptmx.Close(); err != nil {
			slog.Warn("failed to close ptmx", "error", err)
		}
	}()

	c := ptmx.Command(shell, args...)

	if cmd, ok := any(c).(*exec.Cmd); ok {
		cmd.Env = []string{
			"HOME=" + os.Getenv("HOME"),
			"USER=" + os.Getenv("USER"),
			"TERM=xterm-256color",
			"FORCE_COLOR=1",
			"PATH=" + os.Getenv("PATH"),
			"LANG=" + os.Getenv("LANG"),
		}
	}

	if err := c.Start(); err != nil {
		return "", err
	}

	if err := ptmx.Resize(width, height); err != nil {
		return "", err
	}

	vtRead, vtWrite := io.Pipe()
	emu, err := emulator.NewFromPipes(width, height, vtRead, ptmx)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := emu.Close(); err != nil {
			slog.Warn("failed to close emulator", "error", err)
		}
	}()

	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(vtWrite, ptmx)
		copyDone <- copyErr
	}()

	prompt := cfg.Prompt
	if prompt == "" {
		prompt = "❯ "
	}

	pCol := getAnsi(cfg.Colors, "prompt")
	rCol := getAnsi(cfg.Colors, "reset")

	if runtime.GOOS == "windows" {
		styledPrompt := pCol + prompt + rCol
		if _, err := io.WriteString(ptmx, fmt.Sprintf("function prompt { \"%s\" }\r", styledPrompt)); err != nil {
			return "", fmt.Errorf("write prompt function: %w", err)
		}
		if _, err := io.WriteString(ptmx, "Clear-Host\r"); err != nil {
			return "", fmt.Errorf("write clear host: %w", err)
		}
	} else {
		if _, err := io.WriteString(ptmx, fmt.Sprintf("export PS1='\\[\\e[%sm\\]%s\\[\\e[0m\\]'\r", cfg.Colors["prompt"], prompt)); err != nil {
			return "", fmt.Errorf("write PS1: %w", err)
		}
		if _, err := io.WriteString(ptmx, "clear\r"); err != nil {
			return "", fmt.Errorf("write clear: %w", err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	if _, err := io.WriteString(ptmx, "echo ---START---\r"); err != nil {
		return "", fmt.Errorf("write echo start: %w", err)
	}

	anchorFound := false
	for range 20 {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		frame := emu.GetScreen()
		for _, row := range frame.Rows {
			if strings.Contains(row, "---START---") {
				anchorFound = true
				break
			}
		}
		if anchorFound {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	shellPGID, err := getForegroundPGID(ptmx.Fd())
	if err != nil {
		return "", fmt.Errorf("get shell pgid: %w", err)
	}

	for _, cmd := range commands {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		switch cmd.Type {
		case "Command":
			if err := typeColored(ptmx, vtWrite, cmd.Args, cfg); err != nil {
				return "", fmt.Errorf("type colored command: %w", err)
			}
			if _, err := io.WriteString(ptmx, "\r"); err != nil {
				return "", fmt.Errorf("write carriage return: %w", err)
			}
			waitForCommand(ptmx.Fd(), shellPGID, cfg)
		case "CommandStart":
			if err := typeColored(ptmx, vtWrite, cmd.Args, cfg); err != nil {
				return "", fmt.Errorf("type colored command: %w", err)
			}
			if _, err := io.WriteString(ptmx, "\r"); err != nil {
				return "", fmt.Errorf("write carriage return: %w", err)
			}
			time.Sleep(500 * time.Millisecond)
		case "Type":
			if err := typeColored(ptmx, vtWrite, cmd.Args, cfg); err != nil {
				return "", fmt.Errorf("type colored: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
		case "Enter":
			if _, err := io.WriteString(ptmx, "\r"); err != nil {
				return "", fmt.Errorf("write carriage return: %w", err)
			}
			waitForCommand(ptmx.Fd(), shellPGID, cfg)
		case "Raw":
			if _, err := io.WriteString(ptmx, cmd.Args); err != nil {
				return "", fmt.Errorf("write raw: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
		case "Ctrl":
			if len(cmd.Args) > 0 {
				char := strings.ToLower(cmd.Args)[0]
				if char >= 'a' && char <= 'z' {
					ctrlChar := char - 'a' + 1
					if _, err := ptmx.Write([]byte{ctrlChar}); err != nil {
						return "", fmt.Errorf("write ctrl char: %w", err)
					}
				}
			}
			waitForCommand(ptmx.Fd(), shellPGID, cfg)
		case "Key":
			switch strings.ToLower(cmd.Args) {
			case "enter":
				if _, err := io.WriteString(ptmx, "\r"); err != nil {
					return "", fmt.Errorf("write enter key: %w", err)
				}
			case "tab":
				if _, err := io.WriteString(ptmx, "\t"); err != nil {
					return "", fmt.Errorf("write tab key: %w", err)
				}
			case "backspace":
				if _, err := io.WriteString(ptmx, "\x7f"); err != nil {
					return "", fmt.Errorf("write backspace key: %w", err)
				}
			case "escape", "esc":
				if _, err := io.WriteString(ptmx, "\x1b"); err != nil {
					return "", fmt.Errorf("write escape key: %w", err)
				}
			}
			time.Sleep(500 * time.Millisecond)
		case "Sleep":
			time.Sleep(cmd.Delay)
		}
	}

	frame := emu.GetScreen()
	_ = vtRead.Close()
	_ = vtWrite.Close()
	if err := ptmx.Close(); err != nil {
		slog.Warn("failed to close ptmx", "error", err)
	}

	select {
	case <-copyDone:
	case <-time.After(1 * time.Second):
	}

	done := make(chan struct{})
	go func() {
		_ = c.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
	}

	startRow := 0
	for i, row := range frame.Rows {
		if strings.Contains(row, "---START---") {
			startRow = i + 1
		}
	}

	lastIdx := -1
	for i := len(frame.Rows) - 1; i >= startRow; i-- {
		clean := strings.ReplaceAll(frame.Rows[i], "\033[0m", "")
		if strings.TrimSpace(clean) != "" {
			lastIdx = i
			break
		}
	}

	if lastIdx == -1 || lastIdx < startRow {
		return "", nil
	}

	return strings.Join(frame.Rows[startRow:lastIdx+1], "\n") + "\n", nil
}
