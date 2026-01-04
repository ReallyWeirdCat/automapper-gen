package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnakeToCamel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user_name", "UserName"},
		{"created_at", "CreatedAt"},
		{"id", "Id"},
		{"user_id", "UserId"},
		{"http_status_code", "HttpStatusCode"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := snakeToCamel(tt.input)
			if result != tt.expected {
				t.Errorf("snakeToCamel(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseAutomapperTag(t *testing.T) {
	tests := []struct {
		tag               string
		expectedConverter string
		expectedField     string
		expectedIgnore    bool
	}{
		{
			tag:               `json:"name" automapper:"converter=jsTime"`,
			expectedConverter: "jsTime",
			expectedField:     "",
			expectedIgnore:    false,
		},
		{
			tag:               `automapper:"field=created_at"`,
			expectedConverter: "",
			expectedField:     "created_at",
			expectedIgnore:    false,
		},
		{
			tag:               `automapper:"field=email,converter=lowercase"`,
			expectedConverter: "lowercase",
			expectedField:     "email",
			expectedIgnore:    false,
		},
		{
			tag:               `automapper:"-"`,
			expectedConverter: "",
			expectedField:     "",
			expectedIgnore:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			converter, field, ignore := parseAutomapperTag(tt.tag)
			if converter != tt.expectedConverter {
				t.Errorf("converter = %q; want %q", converter, tt.expectedConverter)
			}
			if field != tt.expectedField {
				t.Errorf("field = %q; want %q", field, tt.expectedField)
			}
			if ignore != tt.expectedIgnore {
				t.Errorf("ignore = %v; want %v", ignore, tt.expectedIgnore)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "automapper.json")

	configJSON := `{
		"package": "./models",
		"output": "automappers.go",
		"fieldNameTransform": "snake_to_camel",
		"generateInit": true
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if config.Package != "./models" {
		t.Errorf("Package = %q; want %q", config.Package, "./models")
	}
	if config.Output != "automappers.go" {
		t.Errorf("Output = %q; want %q", config.Output, "automappers.go")
	}
	if config.FieldNameTransform != "snake_to_camel" {
		t.Errorf("FieldNameTransform = %q; want %q", config.FieldNameTransform, "snake_to_camel")
	}
	if !config.GenerateInit {
		t.Error("GenerateInit = false; want true")
	}
}

func TestExtractAnnotation(t *testing.T) {
	tests := []struct {
		docText  string
		expected string
	}{
		{
			docText:  "//automapper:from=UserDB\n",
			expected: "UserDB",
		},
		{
			docText:  "// automapper:from=UserDB,UserAPI\n",
			expected: "UserDB,UserAPI",
		},
		{
			docText: `Some description
//automapper:from=Source1,Source2
More description`,
			expected: "Source1,Source2",
		},
		{
			docText:  "No annotation here\n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.docText, func(t *testing.T) {
			result := extractAnnotation(tt.docText)
			if result != tt.expected {
				t.Errorf("extractAnnotation() = %q; want %q", result, tt.expected)
			}
		})
	}
}

func TestParseSourceList(t *testing.T) {
	tests := []struct {
		annotation string
		expected   []string
	}{
		{
			annotation: "UserDB",
			expected:   []string{"UserDB"},
		},
		{
			annotation: "UserDB,UserAPI",
			expected:   []string{"UserDB", "UserAPI"},
		},
		{
			annotation: "Source1, Source2, Source3",
			expected:   []string{"Source1", "Source2", "Source3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.annotation, func(t *testing.T) {
			result := parseSourceList(tt.annotation)
			if len(result) != len(tt.expected) {
				t.Fatalf("len(result) = %d; want %d", len(result), len(tt.expected))
			}
			for i, source := range result {
				if source != tt.expected[i] {
					t.Errorf("result[%d] = %q; want %q", i, source, tt.expected[i])
				}
			}
		})
	}
}

func TestIsNestedStruct(t *testing.T) {
	tests := []struct {
		typeName string
		expected bool
	}{
		{"UserDTO", true},
		{"*AddressDTO", true},
		{"[]OrderDTO", true},
		{"string", false},
		{"int64", false},
		{"time.Time", false},
		{"*time.Time", false},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			result := isNestedStruct(tt.typeName)
			if result != tt.expected {
				t.Errorf("isNestedStruct(%q) = %v; want %v", tt.typeName, result, tt.expected)
			}
		})
	}
}

func TestExtractBaseType(t *testing.T) {
	tests := []struct {
		typeName string
		expected string
	}{
		{"string", "string"},
		{"*string", "string"},
		{"[]string", "string"},
		{"[]*UserDTO", "*UserDTO"},
		{"*[]UserDTO", "[]UserDTO"},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			result := extractBaseType(tt.typeName)
			if result != tt.expected {
				t.Errorf("extractBaseType(%q) = %q; want %q", tt.typeName, result, tt.expected)
			}
		})
	}
}
