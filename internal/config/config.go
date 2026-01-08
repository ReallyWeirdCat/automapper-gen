package config

import (
	"encoding/json"
	"os"
)

// Config represents the automapper configuration
type Config struct {
	Output             string            `json:"output"`
	Converters         []ConverterDef    `json:"converters"`
	NilPointersForNull bool              `json:"nilPointersForNull"`
	ExternalPackages   []ExternalPackage `json:"externalPackages"`
}

// ExternalPackage defines an external package to include in parsing
type ExternalPackage struct {
	Alias      string `json:"alias"`
	ImportPath string `json:"importPath"`
	LocalPath  string `json:"localPath"`
}

// ConverterDef defines a converter function registration
type ConverterDef struct {
	Name     string `json:"name"`
	Function string `json:"function"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Output == "" {
		cfg.Output = "automappers.go"
	}

	return &cfg, nil
}
