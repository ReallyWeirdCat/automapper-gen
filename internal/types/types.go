package types

// DTOMapping represents a DTO with its mapping configuration
type DTOMapping struct {
	Name        string
	Sources     []string
	Fields      []FieldInfo
	PackageName string
}

// FieldInfo contains information about a struct field
type FieldInfo struct {
	Name         string
	Type         string
	Tag          string
	ConverterTag string
	FieldTag     string
	Ignore       bool
	NestedDTO    string
}

// SourceStruct represents a source struct that can be mapped from
type SourceStruct struct {
	Name       string
	Fields     map[string]FieldTypeInfo
	Package    string
	IsExternal bool
	ImportPath string
	Alias      string
}

// FieldTypeInfo contains detailed type information about a field
type FieldTypeInfo struct {
	Type      string
	IsPointer bool
	IsSlice   bool
	BaseType  string
}
