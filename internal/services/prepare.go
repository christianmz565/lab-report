package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UNSAReport/UNSAReport/internal/adapters/zipper"
	"github.com/UNSAReport/UNSAReport/internal/ports"
	"github.com/charmbracelet/huh"
)

// PrepareOptions holds the parameters for a single prepare execution.
type PrepareOptions struct {
	Configure bool
}

// PrepareService compiles a Typst report into PDF and archives source code for submission.
type PrepareService struct {
	Compiler ports.Compiler
	Archiver ports.Archiver
	FS       ports.FileSystem
	Config   ports.ConfigStore
	Stdout   io.Writer
	Stderr   io.Writer
}

// PrepareOption configures a PrepareService via functional options.
type PrepareOption func(*PrepareService)

// WithPrepareCompiler sets the typst compiler used to render the report.
func WithPrepareCompiler(c ports.Compiler) PrepareOption {
	return func(s *PrepareService) { s.Compiler = c }
}

// WithPrepareArchiver sets the archiver used to create the source code zip.
func WithPrepareArchiver(a ports.Archiver) PrepareOption {
	return func(s *PrepareService) { s.Archiver = a }
}

// WithPrepareFS sets the filesystem used for file operations during prepare.
func WithPrepareFS(fs ports.FileSystem) PrepareOption {
	return func(s *PrepareService) { s.FS = fs }
}

// WithPrepareConfig sets the configuration store for reading project settings.
func WithPrepareConfig(cfg ports.ConfigStore) PrepareOption {
	return func(s *PrepareService) { s.Config = cfg }
}

// WithPrepareStdout sets the writer for standard output messages.
func WithPrepareStdout(w io.Writer) PrepareOption {
	return func(s *PrepareService) { s.Stdout = w }
}

// WithPrepareStderr sets the writer for standard error messages.
func WithPrepareStderr(w io.Writer) PrepareOption {
	return func(s *PrepareService) { s.Stderr = w }
}

// NewPrepareService creates a PrepareService with the given functional options applied.
func NewPrepareService(opts ...PrepareOption) *PrepareService {
	s := &PrepareService{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type prepareContext struct {
	projectRoot string
	cfg         ports.UnsareportConfig
	isMulti     bool
	labDir      string
}

// Execute compiles the report to PDF, archives the source directory, and places both in the submission folder.
func (s *PrepareService) Execute(ctx context.Context, opt PrepareOptions, labDirArg string) error {
	cwd, err := s.FS.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}

	pctx, err := s.resolvePrepareContext(cwd, labDirArg)
	if err != nil {
		return fmt.Errorf("resolve context: %w", err)
	}

	if err := s.FS.Chdir(pctx.projectRoot); err != nil {
		return fmt.Errorf("chdir to project root: %w", err)
	}

	reportPath := pctx.cfg.Prepare.Input.ReportFile
	reportPDF := strings.TrimSuffix(pctx.cfg.Prepare.Input.ReportFile, filepath.Ext(pctx.cfg.Prepare.Input.ReportFile)) + ".pdf"
	srcDir := pctx.cfg.Prepare.Input.SrcDir
	if pctx.isMulti {
		reportPath = filepath.Join(pctx.labDir, pctx.cfg.Prepare.Input.ReportFile)
		reportPDF = filepath.Join(pctx.labDir, strings.TrimSuffix(pctx.cfg.Prepare.Input.ReportFile, filepath.Ext(pctx.cfg.Prepare.Input.ReportFile))+".pdf")
		srcDir = filepath.Join(pctx.labDir, pctx.cfg.Prepare.Input.SrcDir)
	}

	if !s.FS.FileExists(reportPath) {
		return fmt.Errorf("report file not found: %s", reportPath)
	}

	vars, err := s.Compiler.QueryVars(ctx, reportPath)
	if err != nil {
		return fmt.Errorf("query vars: %w", err)
	}

	if pctx.cfg.Prepare.Output.FileTemplate == "" || opt.Configure {
		if err := s.promptConfiguration(&pctx, vars); err != nil {
			return fmt.Errorf("prompt config: %w", err)
		}
	}

	reportWord := "Informe"
	if pctx.cfg.Prepare.Output.ReportWord != "" {
		reportWord = pctx.cfg.Prepare.Output.ReportWord
	}
	codeWord := "Código Fuente"
	if pctx.cfg.Prepare.Output.CodeWord != "" {
		codeWord = pctx.cfg.Prepare.Output.CodeWord
	}

	generatedReportName := ApplyTemplate(pctx.cfg.Prepare.Output.FileTemplate, vars, reportWord)

	if _, err := fmt.Fprintln(s.Stdout, "Compiling typst report..."); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	inputs := map[string]string{"title": generatedReportName}
	inputs["unsarep-root"] = "/"
	if err := s.Compiler.Compile(ctx, reportPath, reportPDF, inputs); err != nil {
		return fmt.Errorf("compile report: %w", err)
	}

	submissionDir := pctx.cfg.Prepare.Output.SubmissionDir
	if pctx.isMulti {
		submissionDir = filepath.Join(pctx.labDir, pctx.cfg.Prepare.Output.SubmissionDir)
	}
	if err := s.FS.EnsureDir(submissionDir); err != nil {
		return fmt.Errorf("ensure submission dir: %w", err)
	}

	reportFile := generatedReportName + ".pdf"
	codeFile := ApplyTemplate(pctx.cfg.Prepare.Output.FileTemplate, vars, codeWord) + ".zip"

	if err := s.FS.CopyFile(reportPDF, filepath.Join(submissionDir, reportFile), 0o644); err != nil {
		return fmt.Errorf("copy pdf: %w", err)
	}

	zipPath := filepath.Join(submissionDir, codeFile)
	if err := s.FS.Remove(zipPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("could not remove old zip", "path", zipPath, "error", err)
	}

	if _, err := fmt.Fprintf(s.Stdout, "Archiving %s to %s...\n", srcDir, zipPath); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	files, err := s.listGitFiles(ctx, srcDir)
	if err != nil {
		return fmt.Errorf("list git files: %w", err)
	}

	if files != nil {
		if err := s.Archiver.ArchiveFiles(zipPath, srcDir, files); err != nil {
			return fmt.Errorf("archive files: %w", err)
		}
		return nil
	}

	if err := s.Archiver.ArchiveDir(zipPath, srcDir); err != nil {
		if errors.Is(err, zipper.ErrSourceMissing) {
			slog.Warn("source directory not found, skipping zip generation", "dir", srcDir)
			return nil
		}
		return fmt.Errorf("archive dir: %w", err)
	}

	if _, err := fmt.Fprintf(s.Stdout, "\nReport: %s\n", filepath.Join(submissionDir, reportFile)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if _, err := fmt.Fprintf(s.Stdout, "Code:  %s\n", filepath.Join(submissionDir, codeFile)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

func (s *PrepareService) listGitFiles(ctx context.Context, srcDir string) ([]string, error) {
	if !s.FS.FileExists(".git") {
		return nil, nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard", srcDir)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		rel, err := filepath.Rel(srcDir, line)
		if err != nil {
			continue
		}
		files = append(files, rel)
	}

	if len(files) == 0 && !s.FS.FileExists(srcDir) {
		return nil, fmt.Errorf("source directory not found")
	}

	return files, nil
}

func (s *PrepareService) resolvePrepareContext(cwd, labDirArg string) (prepareContext, error) {
	projectRoot, cfg, ok, err := s.Config.FindProjectRoot(cwd)
	if err != nil {
		return prepareContext{}, fmt.Errorf("find project root: %w", err)
	}

	if !ok {
		return prepareContext{}, fmt.Errorf("unsareport.json not found in current or parent directories.\nAre you in a lab report project?")
	}

	pctx := prepareContext{
		projectRoot: projectRoot,
		cfg:         cfg,
		isMulti:     cfg.Mode == "multi",
	}

	if !pctx.isMulti {
		if labDirArg != "" {
			return pctx, fmt.Errorf("lab argument provided but template is not multi-mode")
		}
		return pctx, nil
	}

	if labDirArg != "" {
		pctx.labDir = labDirArg
	} else {
		rel, err := filepath.Rel(projectRoot, cwd)
		if err != nil {
			return pctx, fmt.Errorf("could not determine relative path: %w", err)
		}

		if rel == "." {
			return pctx, fmt.Errorf("in a multi-mode project, you must either provide a lab directory or run this command from inside a lab directory")
		}

		parts := strings.Split(filepath.ToSlash(rel), "/")
		pctx.labDir = parts[0]
	}

	sessionValid := slices.Contains(pctx.cfg.Sessions, pctx.labDir)
	if !sessionValid {
		return pctx, fmt.Errorf("session '%s' is not registered in unsareport.json", pctx.labDir)
	}

	return pctx, nil
}

func (s *PrepareService) promptConfiguration(pctx *prepareContext, vars map[string]string) error {
	reportWord := "Informe"
	if pctx.cfg.Prepare.Output.ReportWord != "" {
		reportWord = pctx.cfg.Prepare.Output.ReportWord
	}
	codeWord := "Código Fuente"
	if pctx.cfg.Prepare.Output.CodeWord != "" {
		codeWord = pctx.cfg.Prepare.Output.CodeWord
	}

	input := "{output_type}_{lab_number}"
	if pctx.cfg.Prepare.Output.FileTemplate != "" {
		input = pctx.cfg.Prepare.Output.FileTemplate
	}

	for {
		if _, err := fmt.Fprintln(s.Stdout, "\nVariable configuration for report naming:"); err != nil {
			return fmt.Errorf("write message: %w", err)
		}
		if _, err := fmt.Fprintln(s.Stdout, "Available variables:"); err != nil {
			return fmt.Errorf("write message: %w", err)
		}

		keys := make([]string, 0, len(vars))
		for k := range vars {
			keys = append(keys, k)
		}
		keys = append(keys, "output_type")
		for _, k := range keys {
			desc := vars[k]
			if k == "output_type" {
				desc = "Deliverable type (e.g., Informe or Código Fuente)"
			}
			if _, err := fmt.Fprintf(s.Stdout, "  {%s}: %s\n", k, desc); err != nil {
				return fmt.Errorf("write variable: %w", err)
			}
		}
		if _, err := fmt.Fprintln(s.Stdout, "\nExample: {output_type}_{lab_number}_{members_abbr_list}"); err != nil {
			return fmt.Errorf("write message: %w", err)
		}

		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Enter the base name template (no extension)").
				Placeholder("{output_type}_{lab_number}").
				SuggestionsFunc(func() []string {
					suggestions := make([]string, 0)
					lastBrace := strings.LastIndex(input, "{")
					if lastBrace != -1 && lastBrace > strings.LastIndex(input, "}") {
						prefix := input[:lastBrace+1]
						for _, k := range keys {
							suggestions = append(suggestions, prefix+k+"}")
						}
					} else {
						suggestions = []string{"{output_type}_{lab_number}", "{output_type}_{lab_number}_{members_abbr_list}"}
					}
					return suggestions
				}, &input).
				Value(&input),
			huh.NewInput().
				Title("Word for the report file").
				Value(&reportWord),
			huh.NewInput().
				Title("Word for the source code file").
				Value(&codeWord),
		))
		if err := form.Run(); err != nil {
			return err
		}

		input = strings.TrimSpace(input)
		if input == "" {
			input = "{output_type}_{lab_number}"
		}
		reportWord = strings.TrimSpace(reportWord)
		if reportWord == "" {
			reportWord = "Informe"
		}
		codeWord = strings.TrimSpace(codeWord)
		if codeWord == "" {
			codeWord = "Código Fuente"
		}

		previewReport := ApplyTemplate(input, vars, reportWord)
		previewCode := ApplyTemplate(input, vars, codeWord)

		var keep bool
		confirm := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Preview: %s.pdf | %s.zip", previewReport, previewCode)).
				Value(&keep),
		))
		if err := confirm.Run(); err != nil {
			return err
		}
		if keep {
			pctx.cfg.Prepare.Output.FileTemplate = input
			pctx.cfg.Prepare.Output.ReportWord = reportWord
			pctx.cfg.Prepare.Output.CodeWord = codeWord
			if err := s.Config.WriteConfig(pctx.projectRoot, pctx.cfg); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(s.Stdout, "Configuration saved to unsareport.json\n"); err != nil {
				return fmt.Errorf("write message: %w", err)
			}
			break
		}
	}
	return nil
}
