# AGENTS.md - UNSAReport CLI

## Repository Overview

Go CLI tool for automating lab report creation in UNSA Software Engineering program. Scaffolds Typst-based projects, manages versioned templates, captures terminal output, compiles submissions.

## Quick Commands

```bash
# Build
go build ./cmd/unsarep

# Run all tests (unit)
go test ./...

# Run integration tests (requires external tools)
go test -tags integration ./internal/services/...

# Lint
golangci-lint run

# Single test
go test -run TestSpecificName ./internal/cmd/...
```

## Architecture

**Ports and Adapters pattern** with dependency injection:

- `internal/ports/` - Interfaces (Archiver, Compiler, Fetcher, etc.)
- `internal/adapters/` - Concrete implementations (osfs, github, config, typst, zipper)
- `internal/services/` - Business logic (install, update, prepare, capture, component)
- `internal/cmd/` - CLI commands (cobra)
- `internal/mocks/` - testify mocks for all port interfaces

Entry point: `cmd/unsarep/main.go` → `internal/cmd.Execute()`

## Key Facts

- **Version control**: Uses Jujutsu (jj), not git directly
- **Version injection**: Set via ldflags: `-X github.com/UNSAReport/UNSAReport/internal/cmd.Version={{.Version}}`
- **External tools**: Typst, Freeze (charmbracelet), ImageMagick - validated at runtime, not build time
- **Goroutine leak detection**: All test packages use `goleak.VerifyTestMain`
- **Integration tests**: Require `//go:build integration` tag and external tools on PATH
- **Config**: Uses viper with `UNSAREP_` env prefix
- **Nix dev shell**: Run `nix develop` or `direnv allow` for pre-configured environment

## Testing Patterns

- Unit tests: `testify` (assert/require) + `mock` for port interfaces
- Integration tests: `internal/services/e2e_test.go` with mock fetchers/registries
- Test parallelism: Use `t.Parallel()` for independent tests
- Test cleanup: Use `t.TempDir()` for filesystem tests

## Build & Release

```bash
# Development build
go build ./cmd/unsarep

# Release build with version
go build -ldflags "-X github.com/UNSAReport/UNSAReport/internal/cmd.Version=1.0.0" ./cmd/unsarep

# Nix build
nix build
```

## File Structure

```
cmd/unsarep/main.go          # Entry point
internal/
  cmd/                        # CLI commands (cobra)
  adapters/                   # Port implementations
    config/                   # Config file handling
    github/                   # GitHub API fetcher
    osfs/                     # OS filesystem operations
    registry/                 # Template/component registries
    typst/                    # Typst compiler wrapper
    zipper/                   # ZIP archiver
  ports/                      # Interfaces
  services/                   # Business logic
  mocks/                      # Test mocks
  dependencies/               # External tool checks
schemas/                      # JSON schemas for config
```

## Conventions

- Error handling: Use `samber/oops` for stack traces in debug mode
- CLI framework: cobra commands with viper config binding
- Logging: `log/slog` with text handler to stderr
- Formatting: `gofmt` + `goimports` (enforced by golangci-lint)
- Linters: revive (exported/package-comments disabled), misspell
