package model

const (
	StatusInProgress = "in_progress"
	StatusMerged     = "merged"

	TaskStatusOpen = "open"
	TaskStatusDone = "done"
)

type Config struct {
	Version    string `yaml:"version"`
	BaseBranch string `yaml:"base_branch"`
}

type Index struct {
	NextID int `yaml:"next_id"`
}

type Event struct {
	TS              string            `yaml:"ts"`
	Actor           string            `yaml:"actor"`
	Type            string            `yaml:"type"`
	Summary         string            `yaml:"summary"`
	Ref             string            `yaml:"ref,omitempty"`
	Redacted        bool              `yaml:"redacted,omitempty"`
	RedactionReason string            `yaml:"redaction_reason,omitempty"`
	Meta            map[string]string `yaml:"meta,omitempty"`
}

type Subtask struct {
	ID          int    `yaml:"id"`
	Title       string `yaml:"title"`
	Status      string `yaml:"status"`
	CreatedAt   string `yaml:"created_at"`
	UpdatedAt   string `yaml:"updated_at"`
	CompletedAt string `yaml:"completed_at,omitempty"`
	CreatedBy   string `yaml:"created_by"`
	CompletedBy string `yaml:"completed_by,omitempty"`
}

type CR struct {
	ID                int       `yaml:"id"`
	Title             string    `yaml:"title"`
	Description       string    `yaml:"description"`
	Status            string    `yaml:"status"`
	BaseBranch        string    `yaml:"base_branch"`
	Branch            string    `yaml:"branch"`
	Notes             []string  `yaml:"notes"`
	Subtasks          []Subtask `yaml:"subtasks"`
	Events            []Event   `yaml:"events"`
	MergedAt          string    `yaml:"merged_at,omitempty"`
	MergedBy          string    `yaml:"merged_by,omitempty"`
	MergedCommit      string    `yaml:"merged_commit,omitempty"`
	FilesTouchedCount int       `yaml:"files_touched_count,omitempty"`
	CreatedAt         string    `yaml:"created_at"`
	UpdatedAt         string    `yaml:"updated_at"`
}
