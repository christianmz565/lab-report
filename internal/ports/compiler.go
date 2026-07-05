package ports

import "context"

// Compiler abstracts a LaTeX (or similar) document compiler that can query variables and produce PDF output.
type Compiler interface {
	QueryVars(ctx context.Context, reportPath string) (map[string]string, error)
	Compile(ctx context.Context, reportPath, reportPDF string, inputs map[string]string) error
	Watch(ctx context.Context, reportPath, reportPDF string, inputs map[string]string) error
}
