package cmd

import (
	"github.com/UNSAReport/UNSAReport/internal/adapters/config"
	"github.com/UNSAReport/UNSAReport/internal/adapters/osfs"
	"github.com/UNSAReport/UNSAReport/internal/adapters/typst"
	"github.com/UNSAReport/UNSAReport/internal/services"
	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch [lab-dir]",
		Short: "Watch the report for changes and recompile",
		Long: `Start typst watch to recompile the report automatically on file changes.

This is useful for quick previewing during report editing. The report is
recompiled with the same flags as 'unsarep prepare' but without creating
submission files.`,
		Example: `  # Watch in a single-lab project
  unsarep watch

  # Watch in a multi-lab project (from root)
  unsarep watch l1

  # Watch from inside a lab directory
  cd l1 && unsarep watch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var labDir string
			if len(args) > 0 {
				labDir = args[0]
			}
			if len(args) > 1 {
				return cmd.Help()
			}

			fs := osfs.New()
			compiler := typst.New()
			cfg := config.New()

			svc := services.NewWatchService(
				services.WithWatchCompiler(compiler),
				services.WithWatchFS(fs),
				services.WithWatchConfig(cfg),
				services.WithWatchStdout(cmd.OutOrStdout()),
				services.WithWatchStderr(cmd.ErrOrStderr()),
			)
			return svc.Execute(cmd.Context(), labDir)
		},
	}
}
