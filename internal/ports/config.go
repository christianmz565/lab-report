package ports

// CaptureConfig defines terminal capture parameters such as dimensions, freeze regions, and color scheme.
type CaptureConfig struct {
	Columns        int               `json:"columns"`
	Rows           int               `json:"rows,omitempty"`
	FreezeFlags    []string          `json:"freezeFlags"`
	Prompt         string            `json:"prompt"`
	Colors         map[string]string `json:"colors"`
	CommandTimeout int               `json:"commandTimeout,omitempty"`
}

// PrepareInputConfig specifies the source directory and report file used as input for report preparation.
type PrepareInputConfig struct {
	SrcDir     string `json:"srcDir"`
	ReportFile string `json:"reportFile"`
}

// PrepareOutputConfig specifies the submission directory and naming conventions for prepared output files.
type PrepareOutputConfig struct {
	SubmissionDir string `json:"submissionDir"`
	FileTemplate  string `json:"fileTemplate"`
	ReportWord    string `json:"reportWord"`
	CodeWord      string `json:"codeWord"`
}

// PrepareConfig groups input and output configuration for the report preparation workflow.
type PrepareConfig struct {
	Input  PrepareInputConfig  `json:"input"`
	Output PrepareOutputConfig `json:"output"`
}

// UnsareportConfig represents the full project configuration including template, capture, preparation, and component settings.
type UnsareportConfig struct {
	Schema          string                          `json:"$schema,omitempty"`
	Template        string                          `json:"template"`
	TemplateVersion string                          `json:"templateVersion,omitempty"`
	Mode            string                          `json:"mode"`
	LocalSource     string                          `json:"localSource,omitempty"`
	Sessions        []string                        `json:"sessions"`
	Capture         CaptureConfig                   `json:"capture"`
	Prepare         PrepareConfig                   `json:"prepare"`
	Components      map[string]ComponentConfigEntry `json:"components,omitempty"`
}

// ConfigStore abstracts reading and writing the project configuration and lockfile.
type ConfigStore interface {
	FindProjectRoot(startDir string) (projectRoot string, cfg UnsareportConfig, ok bool, err error)
	ReadConfig(destDir string) (cfg UnsareportConfig, ok bool, err error)
	WriteConfig(destDir string, cfg UnsareportConfig) error
	ReadLockfile(destDir string) (Lockfile, error)
	WriteLockfile(destDir string, lf Lockfile) error
}
