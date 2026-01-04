package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Package            string              `json:"package"`
	Output             string              `json:"output"`
	DefaultConverters  []ConverterDef      `json:"defaultConverters"`
	FieldNameTransform string              `json:"fieldNameTransform"`
	NilPointersForNull bool                `json:"nilPointersForNull"`
	GenerateInit       bool                `json:"generateInit"`
	ExternalPackages   []ExternalPackage   `json:"externalPackages"` // Changed to struct
}

type ExternalPackage struct {
	Alias      string `json:"alias"`      // Optional alias for the import
	ImportPath string `json:"importPath"` // Full import path like "git.weirdcat.su/automapper-gen/example/db"
	LocalPath  string `json:"localPath"`  // Local relative path for parsing source files
}

type ConverterDef struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Name     string `json:"name"`
	Function string `json:"function"`
}

type DTOMapping struct {
	Name        string
	Sources     []string
	Fields      []FieldInfo
	PackageName string
}

type FieldInfo struct {
	Name         string
	Type         string
	Tag          string
	ConverterTag string
	FieldTag     string
	Ignore       bool
}

type SourceStruct struct {
	Name       string
	Fields     map[string]FieldTypeInfo
	Package    string
	IsExternal bool
	ImportPath string
	Alias      string // Import alias
}

type FieldTypeInfo struct {
	Type      string
	IsPointer bool
	IsSlice   bool
	BaseType  string
}

func main() {
	flag.Parse()
	args := flag.Args()
	
	if len(args) < 1 {
		fmt.Println("Usage: automapper-gen <package-path>")
		os.Exit(1)
	}

	pkgPath := args[0]
	
	// Load configuration
	config, err := loadConfig(filepath.Join(pkgPath, "automapper.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Parse Go files
	dtos, sources, pkgName, err := parsePackage(pkgPath, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing package: %v\n", err)
		os.Exit(1)
	}

	if len(dtos) == 0 {
		fmt.Println("No DTOs with automapper annotations found")
		return
	}

	// Generate code
	code, err := generateCode(dtos, sources, config, pkgName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating code: %v\n", err)
		os.Exit(1)
	}

	// Write output
	outputPath := filepath.Join(pkgPath, config.Output)
	if err := os.WriteFile(outputPath, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s with %d DTO mappings\n", outputPath, len(dtos))
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.Output == "" {
		config.Output = "automappers.go"
	}
	if config.FieldNameTransform == "" {
		config.FieldNameTransform = "snake_to_camel"
	}

	return &config, nil
}

func parsePackage(pkgPath string, config *Config) ([]DTOMapping, map[string]SourceStruct, string, error) {
	fset := token.NewFileSet()
	
	// Parse main package
	dtos, sources, pkgName, err := parseDir(fset, pkgPath, "", pkgPath, false, config)
	if err != nil {
		return nil, nil, "", err
	}

	// Parse external packages
	for _, extPkg := range config.ExternalPackages {
		// Determine local path for parsing
		localPath := extPkg.LocalPath
		if localPath == "" {
			// If no local path specified, try to find it relative to main package
			// This is a fallback for backward compatibility
			localPath = filepath.Join(pkgPath, "..", strings.TrimPrefix(extPkg.ImportPath, "git.weirdcat.su/automapper-gen/"))
		} else if !filepath.IsAbs(localPath) {
			// Make relative path absolute relative to main package
			localPath = filepath.Join(pkgPath, localPath)
		}

		// Determine package alias
		alias := extPkg.Alias
		if alias == "" {
			// Extract alias from import path (last part)
			parts := strings.Split(extPkg.ImportPath, "/")
			alias = parts[len(parts)-1]
		}

		if _, err := os.Stat(localPath); err == nil {
			extDtos, extSources, _, err := parseDir(fset, localPath, alias, extPkg.ImportPath, true, config)
			if err != nil {
				fmt.Printf("Warning: could not parse external package %s: %v\n", extPkg.ImportPath, err)
				continue
			}
			
			// Merge sources from external package
			for k, v := range extSources {
				sources[k] = v
			}
			
			// Note: We don't merge DTOs from external packages
			_ = extDtos // Silence unused variable warning
		} else {
			fmt.Printf("Warning: local path not found for external package %s: %v\n", extPkg.ImportPath, err)
		}
	}

	return dtos, sources, pkgName, nil
}

func parseDir(fset *token.FileSet, dir string, alias string, importPath string, isExternal bool, config *Config) ([]DTOMapping, map[string]SourceStruct, string, error) {
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && fi.Name() != config.Output
	}, parser.ParseComments)
	if err != nil {
		return nil, nil, "", err
	}

	dtos := []DTOMapping{}
	sources := make(map[string]SourceStruct)
	var pkgName string

	for name, pkg := range pkgs {
		pkgName = name
		
		for _, file := range pkg.Files {
			// First pass: collect all structs (potential sources)
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								sourceStruct := parseStruct(structType)
								sourceStruct.Name = typeSpec.Name.Name
								sourceStruct.IsExternal = isExternal
								sourceStruct.ImportPath = importPath
								sourceStruct.Alias = alias
								
								// For external packages, use alias as package name
								if isExternal {
									sourceStruct.Package = alias
									// Store with alias prefix
									sources[alias+"."+typeSpec.Name.Name] = sourceStruct
								} else {
									sourceStruct.Package = pkgName
									sources[typeSpec.Name.Name] = sourceStruct
								}
							}
						}
					}
				}
			}

			// Second pass: find DTOs with annotations (only in main package)
			if !isExternal {
				for _, decl := range file.Decls {
					if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
						for _, spec := range genDecl.Specs {
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
								// Check for automapper annotation
								var annotation string
								if genDecl.Doc != nil {
									annotation = extractAnnotation(genDecl.Doc)
								}
								if annotation == "" && typeSpec.Doc != nil {
									annotation = extractAnnotation(typeSpec.Doc)
								}

								if annotation != "" {
									if structType, ok := typeSpec.Type.(*ast.StructType); ok {
										dto := DTOMapping{
											Name:        typeSpec.Name.Name,
											Sources:     parseSourceList(annotation),
											Fields:      parseFields(structType),
											PackageName: pkgName,
										}
										dtos = append(dtos, dto)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return dtos, sources, pkgName, nil
}

func parseStruct(structType *ast.StructType) SourceStruct {
	s := SourceStruct{
		Fields: make(map[string]FieldTypeInfo),
	}

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue // embedded field
		}
		
		fieldName := field.Names[0].Name
		typeInfo := extractTypeInfo(field.Type)
		s.Fields[fieldName] = typeInfo
	}

	return s
}

func extractTypeInfo(expr ast.Expr) FieldTypeInfo {
	info := FieldTypeInfo{}

	switch t := expr.(type) {
	case *ast.StarExpr:
		info.IsPointer = true
		info.BaseType = exprToString(t.X)
		info.Type = "*" + info.BaseType
	case *ast.ArrayType:
		info.IsSlice = true
		info.BaseType = exprToString(t.Elt)
		info.Type = "[]" + info.BaseType
	default:
		info.BaseType = exprToString(expr)
		info.Type = info.BaseType
	}

	return info
}

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	default:
		return ""
	}
}

func parseFields(structType *ast.StructType) []FieldInfo {
	fields := []FieldInfo{}

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue
		}

		fieldInfo := FieldInfo{
			Name: field.Names[0].Name,
			Type: exprToString(field.Type),
		}

		if field.Tag != nil {
			tag := field.Tag.Value
			tag = strings.Trim(tag, "`")
			fieldInfo.Tag = tag
			
			// Parse automapper tag
			if strings.Contains(tag, "automapper:") {
				fieldInfo.ConverterTag, fieldInfo.FieldTag, fieldInfo.Ignore = parseAutomapperTag(tag)
			}
		}

		fields = append(fields, fieldInfo)
	}

	return fields
}

func parseAutomapperTag(tag string) (converter, field string, ignore bool) {
	// Extract automapper:"..." part
	start := strings.Index(tag, `automapper:"`)
	if start == -1 {
		return
	}
	start += len(`automapper:"`)
	end := strings.Index(tag[start:], `"`)
	if end == -1 {
		return
	}
	
	automapperTag := tag[start : start+end]
	
	if automapperTag == "-" {
		ignore = true
		return
	}

	parts := strings.Split(automapperTag, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			
			switch key {
			case "converter":
				converter = value
			case "field":
				field = value
			}
		}
	}

	return
}

func extractAnnotation(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	
	for _, comment := range doc.List {
		text := comment.Text
		text = strings.TrimSpace(text)
		
		// Remove // or /* */ markers
		if strings.HasPrefix(text, "//") {
			text = strings.TrimSpace(text[2:])
		} else if strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/") {
			text = strings.TrimSpace(text[2 : len(text)-2])
		}
		
		if strings.HasPrefix(text, "automapper:from=") {
			return strings.TrimSpace(strings.TrimPrefix(text, "automapper:from="))
		}
	}
	return ""
}

func parseSourceList(annotation string) []string {
	parts := strings.Split(annotation, ",")
	sources := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			sources = append(sources, part)
		}
	}
	return sources
}

func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func generateCode(dtos []DTOMapping, sources map[string]SourceStruct, config *Config, pkgName string) (string, error) {
	var sb strings.Builder

	// Header
	sb.WriteString("// Code generated by automapper-gen. DO NOT EDIT.\n\n")
	sb.WriteString(fmt.Sprintf("package %s\n\n", pkgName))
	
	// Collect required imports
	imports := map[string]bool{
		"errors": true,
		"fmt":    true,
	}
	
	// Check for time usage in converters
	for _, conv := range config.DefaultConverters {
		if strings.Contains(conv.From, "time.") || strings.Contains(conv.To, "time.") {
			imports["time"] = true
		}
	}
	
	// Map to store import paths by alias
	importMap := make(map[string]string) // alias -> import path
	
	// Check for external package usage in DTOs
	for _, dto := range dtos {
		for _, sourceName := range dto.Sources {
			if source, ok := sources[sourceName]; ok && source.IsExternal {
				importMap[source.Alias] = source.ImportPath
			}
		}
	}
	
	// Write imports
	if len(imports) > 0 || len(importMap) > 0 {
		sb.WriteString("import (\n")
		
		// Write standard imports
		for imp := range imports {
			sb.WriteString(fmt.Sprintf("\t\"%s\"\n", imp))
		}
		
		// Write external package imports
		for alias, importPath := range importMap {
			// If alias is the same as the last part of import path, don't use alias
			parts := strings.Split(importPath, "/")
			lastPart := parts[len(parts)-1]
			
			if alias == lastPart {
				sb.WriteString(fmt.Sprintf("\t\"%s\"\n", importPath))
			} else {
				sb.WriteString(fmt.Sprintf("\t%s \"%s\"\n", alias, importPath))
			}
		}
		
		sb.WriteString(")\n\n")
	}

	// Converter infrastructure
	sb.WriteString(generateConverterInfrastructure(config))

	// Generate MapFrom methods for each DTO
	for _, dto := range dtos {
		for _, sourceName := range dto.Sources {
			source, ok := sources[sourceName]
			if !ok {
				return "", fmt.Errorf("source struct %s not found for DTO %s", sourceName, dto.Name)
			}

			methodName := "MapFrom"
			if len(dto.Sources) > 1 || source.IsExternal {
				// If multiple sources OR source is external, use explicit method name
				methodName = "MapFrom" + extractTypeNameWithoutPackage(sourceName)
			}

			sb.WriteString(generateMapFromMethod(dto, source, sourceName, methodName, config))
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func generateConverterInfrastructure(config *Config) string {
	var sb strings.Builder

	sb.WriteString("// Converter type for type-safe conversions\n")
	sb.WriteString("type Converter[From any, To any] func(From) (To, error)\n\n")

	sb.WriteString("// Global converter registry\n")
	sb.WriteString("var converters = make(map[string]interface{})\n\n")

	sb.WriteString("// RegisterConverter registers a type-safe converter\n")
	sb.WriteString("func RegisterConverter[From any, To any](name string, fn Converter[From, To]) {\n")
	sb.WriteString("\tconverters[name] = fn\n")
	sb.WriteString("}\n\n")

	sb.WriteString("// Convert performs a type-safe conversion using a registered converter\n")
	sb.WriteString("func Convert[From any, To any](name string, value From) (To, error) {\n")
	sb.WriteString("\tvar zero To\n")
	sb.WriteString("\tconverterIface, ok := converters[name]\n")
	sb.WriteString("\tif !ok {\n")
	sb.WriteString("\t\treturn zero, fmt.Errorf(\"converter %s not registered\", name)\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\tconverter, ok := converterIface.(Converter[From, To])\n")
	sb.WriteString("\tif !ok {\n")
	sb.WriteString("\t\treturn zero, fmt.Errorf(\"converter %s has wrong type\", name)\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn converter(value)\n")
	sb.WriteString("}\n\n")

	// Generate init with default converters
	if config.GenerateInit && len(config.DefaultConverters) > 0 {
		sb.WriteString("func init() {\n")
		for _, conv := range config.DefaultConverters {
			sb.WriteString(fmt.Sprintf("\t// Register %s: %s -> %s\n", conv.Name, conv.From, conv.To))
			sb.WriteString(fmt.Sprintf("\tRegisterConverter[%s, %s](\"%s\", %s)\n", conv.From, conv.To, conv.Name, conv.Function))
		}
		sb.WriteString("}\n\n")

		// Generate built-in converters
		sb.WriteString("// TimeToJSString converts time.Time to JavaScript ISO 8601 string\n")
		sb.WriteString("func TimeToJSString(t time.Time) (string, error) {\n")
		sb.WriteString("\treturn t.Format(time.RFC3339), nil\n")
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func generateMapFromMethod(dto DTOMapping, source SourceStruct, sourceName, methodName string, config *Config) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("// %s maps from %s to %s\n", methodName, sourceName, dto.Name))
	
	// Use the full qualified name with package prefix if present
	paramType := sourceName
	sb.WriteString(fmt.Sprintf("func (d *%s) %s(src *%s) error {\n", dto.Name, methodName, paramType))
	sb.WriteString("\tif src == nil {\n")
	sb.WriteString("\t\treturn errors.New(\"source is nil\")\n")
	sb.WriteString("\t}\n\n")

	// Generate field mappings
	for _, dtoField := range dto.Fields {
		if dtoField.Ignore {
			continue
		}

		// Determine source field name
		sourceFieldName := dtoField.Name
		if dtoField.FieldTag != "" {
			sourceFieldName = dtoField.FieldTag
		} else if config.FieldNameTransform == "snake_to_camel" {
			// Try to find snake_case version
			for srcFieldName := range source.Fields {
				if snakeToCamel(srcFieldName) == dtoField.Name {
					sourceFieldName = srcFieldName
					break
				}
			}
		}

		sourceField, exists := source.Fields[sourceFieldName]
		if !exists {
			// Field doesn't exist in source, skip
			sb.WriteString(fmt.Sprintf("\t// %s: not found in source, will be zero value\n", dtoField.Name))
			continue
		}

		// Generate assignment based on type
		if dtoField.ConverterTag != "" {
			// Use converter
			sb.WriteString("\t{\n")
			sb.WriteString("\t\tvar err error\n")
			sb.WriteString(fmt.Sprintf("\t\td.%s, err = Convert[%s, %s](\"%s\", src.%s)\n", 
				dtoField.Name, sourceField.BaseType, extractBaseType(dtoField.Type), dtoField.ConverterTag, sourceFieldName))
			sb.WriteString("\t\tif err != nil {\n")
			sb.WriteString(fmt.Sprintf("\t\t\treturn fmt.Errorf(\"converting field %s: %%w\", err)\n", dtoField.Name))
			sb.WriteString("\t\t}\n")
			sb.WriteString("\t}\n")
		} else {
			// Direct assignment (simplified)
			sb.WriteString(fmt.Sprintf("\td.%s = src.%s\n", dtoField.Name, sourceFieldName))
		}
	}

	sb.WriteString("\n\treturn nil\n")
	sb.WriteString("}\n")

	return sb.String()
}

func isNestedStruct(typeName string) bool {
	// Simple heuristic: uppercase first letter suggests custom struct
	typeName = strings.TrimPrefix(typeName, "*")
	typeName = strings.TrimPrefix(typeName, "[]")
	if len(typeName) == 0 {
		return false
	}
	// Built-in types start with lowercase or are in standard packages
	if typeName[0] >= 'A' && typeName[0] <= 'Z' && !strings.Contains(typeName, ".") {
		return true
	}
	return false
}

func extractBaseType(typeName string) string {
	typeName = strings.TrimPrefix(typeName, "*")
	typeName = strings.TrimPrefix(typeName, "[]")
	return typeName
}

// Helper function to extract type name without package prefix
func extractTypeNameWithoutPackage(typeName string) string {
	if strings.Contains(typeName, ".") {
		parts := strings.Split(typeName, ".")
		return parts[len(parts)-1]
	}
	return typeName
}
