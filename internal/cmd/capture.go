package cmd

import (
	"fmt"

	"github.com/UNSAReport/UNSAReport/internal/adapters/config"
	"github.com/UNSAReport/UNSAReport/internal/adapters/freeze"
	"github.com/UNSAReport/UNSAReport/internal/adapters/osfs"
	"github.com/UNSAReport/UNSAReport/internal/services"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"
)

func newCaptureCmd() *cobra.Command {
	var cwdFlag string
	var freezeFlags string
	var saveFlags bool

	cmd := &cobra.Command{
		Use:   "capture [flags] <result.png> [instructions...]",
		Short: "Capture terminal output and render it to a PNG via freeze",
		Long: `Capture terminal output and render it to a PNG using charmbracelet/freeze and ImageMagick.

This command executes terminal instructions directly in a virtual terminal and captures the
resulting output. It applies custom prompt formatting and terminal width (columns) defined 
in the unsareport.json configuration.

Instructions are processed sequentially:
- Regular text: Typed into the terminal followed by Enter.
- "w:<duration>": Delays execution (e.g., "w:2s", "w:500ms").
- "r:<text>": Writes text directly without Enter.
- "c:<key>": Sends Ctrl + key combination (e.g., "c:c" for Ctrl+C).
- "k:<key>": Sends a control key (e.g., "k:enter", "k:tab", "k:backspace", "k:esc").
- "x:<text>": Types text and presses Enter but does not wait for the command to finish (for interactive programs like node, python).

A raw log of the session (including ANSI colors) is automatically saved in
the capture_logs/ directory as a .log file.`,
		Example: `  # Simple capture
  unsarep capture output.png "ls -la" "cat README.md"

  # Interactive program (node REPL)
  unsarep capture output.png "x:node" "r:console.log('hello')" "k:enter" "c:d"

  # With custom directory and delays
  unsarep capture --cwd ./src result.png "x:python3" "r:print('hello')" "k:enter" "c:d"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return cmd.Help()
			}

			fs := osfs.New()
			renderer := freeze.New()
			cfg := config.New()

			var parsedFlags []string
			var err error
			if freezeFlags != "" {
				parsedFlags, err = shellquote.Split(freezeFlags)
				if err != nil {
					return fmt.Errorf("parse freeze-flags: %w", err)
				}
			}

			svc := services.NewCaptureService(
				services.WithCaptureRenderer(renderer),
				services.WithCaptureFS(fs),
				services.WithCaptureConfig(cfg),
				services.WithCaptureStdout(cmd.OutOrStdout()),
				services.WithCaptureStderr(cmd.ErrOrStderr()),
			)
			return svc.Execute(cmd.Context(), services.CaptureOptions{
				Cwd:             cwdFlag,
				Args:            args,
				FreezeFlags:     parsedFlags,
				SaveFreezeFlags: saveFlags,
			})
		},
	}

	cmd.Flags().StringVar(&cwdFlag, "cwd", "", "Directory to cd into at the start of the capture")
	cmd.Flags().StringVar(&freezeFlags, "freeze-flags", "", "Additional flags to pass to freeze (e.g., \"--theme dracula\")")
	cmd.Flags().BoolVar(&saveFlags, "save-freeze-flags", false, "Save the passed freeze-flags to unsareport.json")

	return cmd
}
