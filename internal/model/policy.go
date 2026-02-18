package model

type RepoPolicy struct {
	Version        string               `yaml:"version"`
	Contract       PolicyContract       `yaml:"contract,omitempty"`
	TaskContract   PolicyTaskContract   `yaml:"task_contract,omitempty"`
	Scope          PolicyScope          `yaml:"scope,omitempty"`
	Classification PolicyClassification `yaml:"classification,omitempty"`
	Merge          PolicyMerge          `yaml:"merge,omitempty"`
}

type PolicyContract struct {
	RequiredFields []string `yaml:"required_fields,omitempty"`
}

type PolicyTaskContract struct {
	RequiredFields []string `yaml:"required_fields,omitempty"`
}

type PolicyScope struct {
	AllowedPrefixes []string `yaml:"allowed_prefixes,omitempty"`
}

type PolicyClassification struct {
	Test       PolicyClassificationTest       `yaml:"test,omitempty"`
	Dependency PolicyClassificationDependency `yaml:"dependency,omitempty"`
}

type PolicyClassificationTest struct {
	Suffixes     []string `yaml:"suffixes,omitempty"`
	PathContains []string `yaml:"path_contains,omitempty"`
}

type PolicyClassificationDependency struct {
	FileNames []string `yaml:"file_names,omitempty"`
}

type PolicyMerge struct {
	AllowOverride *bool `yaml:"allow_override,omitempty"`
}

