package typst

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/UNSAReport/UNSAReport/internal/dependencies"
	"github.com/UNSAReport/UNSAReport/internal/ports"
)

var _ ports.Compiler = (*Adapter)(nil)

// Adapter implements ports.Compiler by invoking the typst CLI for querying variables and compiling reports.
type Adapter struct{}

// New returns a new Adapter for typst compilation.
func New() *Adapter {
	return &Adapter{}
}

type queryItem struct {
	Value *struct {
		Name  string `json:"name"`
		Value any    `json:"value"`
	} `json:"value"`
}

// QueryVars extracts exported variables from a Typst report and returns them as a string map.
func (a *Adapter) QueryVars(ctx context.Context, reportPath string) (map[string]string, error) {
	if err := dependencies.Check(dependencies.Typst); err != nil {
		return nil, err
	}

	args := []string{"query", "--root", ".", reportPath, "<var_export>"}

	cmd := exec.CommandContext(ctx, "typst", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("typst query failed: %s", msg)
	}

	var items []queryItem
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		return nil, fmt.Errorf("failed to parse typst query output: %w", err)
	}

	vars := make(map[string]string)
	for _, it := range items {
		if it.Value == nil || it.Value.Name == "" {
			continue
		}

		switch v := it.Value.Value.(type) {
		case []any:
			parts := make([]string, 0, len(v))
			for _, p := range v {
				parts = append(parts, fmt.Sprint(p))
			}
			vars[it.Value.Name] = strings.Join(parts, "-")
		default:
			vars[it.Value.Name] = fmt.Sprint(v)
		}
	}
	return vars, nil
}

// Compile runs typst compile to produce reportPDF from reportPath, passing inputs as --input flags.
func (a *Adapter) Compile(ctx context.Context, reportPath, reportPDF string, inputs map[string]string) error {
	if err := dependencies.Check(dependencies.Typst); err != nil {
		return err
	}

	args := []string{"compile", "--root", "."}
	for k, v := range inputs {
		args = append(args, "--input", k+"="+v)
	}
	args = append(args, reportPath, reportPDF)

	cmd := exec.CommandContext(ctx, "typst", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("typst compile failed: %w", err)
	}
	return nil
}

// Watch runs typst watch to recompile reportPDF on file changes, passing inputs as --input flags.
func (a *Adapter) Watch(ctx context.Context, reportPath, reportPDF string, inputs map[string]string) error {
	if err := dependencies.Check(dependencies.Typst); err != nil {
		return err
	}

	args := []string{"watch", "--root", "."}
	for k, v := range inputs {
		args = append(args, "--input", k+"="+v)
	}
	args = append(args, reportPath, reportPDF)

	cmd := exec.CommandContext(ctx, "typst", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("typst watch failed: %w", err)
	}
	return nil
}
