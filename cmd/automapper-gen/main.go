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

	"github.com/dave/jennifer/jen"
)

type Config struct {
	Package            string              `json:"package"`
	Output             string              `json:"output"`
	DefaultConverters  []ConverterDef      `json:"defaultConverters"`
	FieldNameTransform string              `json:"fieldNameTransform"`
	NilPointersForNull bool                `json:"nilPointersForNull"`
	GenerateInit       bool                `json:"generateInit"`
	ExternalPackages   []ExternalPackage   `json:"externalPackages"`
}

type ExternalPackage struct {
	Alias      string `json:"alias"`
	ImportPath string `json:"importPath"`
	LocalPath  string `json:"localPath"`
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
	Alias      string
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
	file, err := generateCode(dtos, sources, config, pkgName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating code: %v\n", err)
		os.Exit(1)
	}

	// Write output
	outputPath := filepath.Join(pkgPath, config.Output)
	if err := file.Save(outputPath); err != nil {
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
		localPath := extPkg.LocalPath
		if !filepath.IsAbs(localPath) {
			localPath = filepath.Join(pkgPath, localPath)
		}

		alias := extPkg.Alias
		if alias == "" {
			parts := strings.Split(extPkg.ImportPath, "/")
			alias = parts[len(parts)-1]
		}

		if _, err := os.Stat(localPath); err == nil {
			extDtos, extSources, _, err := parseDir(fset, localPath, alias, extPkg.ImportPath, true, config)
			if err != nil {
				fmt.Printf("Warning: could not parse external package %s: %v\n", extPkg.ImportPath, err)
				continue
			}
			
			for k, v := range extSources {
				sources[k] = v
			}
			
			_ = extDtos
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
								
								if isExternal {
									sourceStruct.Package = alias
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

			if !isExternal {
				for _, decl := range file.Decls {
					if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
						for _, spec := range genDecl.Specs {
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
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
			continue
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
			
			if strings.Contains(tag, "automapper:") {
				fieldInfo.ConverterTag, fieldInfo.FieldTag, fieldInfo.Ignore = parseAutomapperTag(tag)
			}
		}

		fields = append(fields, fieldInfo)
	}

	return fields
}

func parseAutomapperTag(tag string) (converter, field string, ignore bool) {
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

func generateCode(dtos []DTOMapping, sources map[string]SourceStruct, config *Config, pkgName string) (*jen.File, error) {
	f := jen.NewFile(pkgName)
	
	// Add header comment
	f.HeaderComment("Code generated by automapper-gen. DO NOT EDIT.")
	
	// Build import mapping (alias -> importPath) for external packages
	importMap := make(map[string]string)
	for _, source := range sources {
		if source.IsExternal && source.Alias != "" && source.ImportPath != "" {
			importMap[source.Alias] = source.ImportPath
		}
	}
	
	// Generate converter infrastructure
	generateConverterInfrastructure(f, config, importMap)
	
	// Generate MapFrom methods
	for _, dto := range dtos {
		for _, sourceName := range dto.Sources {
			source, ok := sources[sourceName]
			if !ok {
				return nil, fmt.Errorf("source struct %s not found for DTO %s", sourceName, dto.Name)
			}

			methodName := "MapFrom"
			if len(dto.Sources) > 1 || source.IsExternal {
				methodName = "MapFrom" + extractTypeNameWithoutPackage(sourceName)
			}

			generateMapFromMethod(f, dto, source, sourceName, methodName, config, importMap)
		}
	}
	
	return f, nil
}

func generateConverterInfrastructure(f *jen.File, config *Config, importMap map[string]string) {
	// Converter type
	f.Comment("Converter type for type-safe conversions")
	f.Type().Id("Converter").Types(
		jen.Id("From").Any(),
		jen.Id("To").Any(),
	).Func().Params(jen.Id("From")).Params(jen.Id("To"), jen.Error())
	
	f.Line()
	
	// Global registry
	f.Comment("Global converter registry")
	f.Var().Id("converters").Op("=").Make(jen.Map(jen.String()).Interface())
	
	f.Line()
	
	// RegisterConverter
	f.Comment("RegisterConverter registers a type-safe converter")
	f.Func().Id("RegisterConverter").Types(
		jen.Id("From").Any(),
		jen.Id("To").Any(),
	).Params(
		jen.Id("name").String(),
		jen.Id("fn").Id("Converter").Types(jen.Id("From"), jen.Id("To")),
	).Block(
		jen.Id("converters").Index(jen.Id("name")).Op("=").Id("fn"),
	)
	
	f.Line()
	
	// Convert
	f.Comment("Convert performs a type-safe conversion using a registered converter")
	f.Func().Id("Convert").Types(
		jen.Id("From").Any(),
		jen.Id("To").Any(),
	).Params(
		jen.Id("name").String(),
		jen.Id("value").Id("From"),
	).Params(jen.Id("To"), jen.Error()).Block(
		jen.Var().Id("zero").Id("To"),
		jen.List(jen.Id("converterIface"), jen.Id("ok")).Op(":=").Id("converters").Index(jen.Id("name")),
		jen.If(jen.Op("!").Id("ok")).Block(
			jen.Return(jen.Id("zero"), jen.Qual("fmt", "Errorf").Call(
				jen.Lit("converter %s not registered"),
				jen.Id("name"),
			)),
		),
		jen.List(jen.Id("converter"), jen.Id("ok")).Op(":=").Id("converterIface").Assert(
			jen.Id("Converter").Types(jen.Id("From"), jen.Id("To")),
		),
		jen.If(jen.Op("!").Id("ok")).Block(
			jen.Return(jen.Id("zero"), jen.Qual("fmt", "Errorf").Call(
				jen.Lit("converter %s has wrong type"),
				jen.Id("name"),
			)),
		),
		jen.Return(jen.Id("converter").Call(jen.Id("value"))),
	)
	
	f.Line()
	
	// Generate init with default converters
	if config.GenerateInit && len(config.DefaultConverters) > 0 {
		initStatements := []jen.Code{}
		
		for _, conv := range config.DefaultConverters {
			initStatements = append(initStatements,
				jen.Comment(fmt.Sprintf("Register %s: %s -> %s", conv.Name, conv.From, conv.To)),
			)
			
			// Parse types to generate proper type parameters
			fromType := parseTypeForJen(conv.From, importMap)
			toType := parseTypeForJen(conv.To, importMap)
			
			initStatements = append(initStatements,
				jen.Id("RegisterConverter").Types(fromType, toType).Call(
					jen.Lit(conv.Name),
					jen.Id(conv.Function),
				),
			)
		}
		
		f.Func().Id("init").Params().Block(initStatements...)
		
		f.Line()
		
		// Built-in converter: TimeToJSString
		f.Comment("TimeToJSString converts time.Time to JavaScript ISO 8601 string")
		f.Func().Id("TimeToJSString").Params(
			jen.Id("t").Qual("time", "Time"),
		).Params(jen.String(), jen.Error()).Block(
			jen.Return(jen.Id("t").Dot("Format").Call(jen.Qual("time", "RFC3339")), jen.Nil()),
		)
		
		f.Line()
	}
}

func generateMapFromMethod(f *jen.File, dto DTOMapping, source SourceStruct, sourceName, methodName string, config *Config, importMap map[string]string) {
	// Parse parameter type
	paramType := parseTypeRefForJen(sourceName, importMap)
	
	f.Comment(fmt.Sprintf("%s maps from %s to %s", methodName, sourceName, dto.Name))
	
	// Build method body
	methodBody := []jen.Code{
		jen.If(jen.Id("src").Op("==").Nil()).Block(
			jen.Return(jen.Qual("errors", "New").Call(jen.Lit("source is nil"))),
		),
		jen.Line(),
	}
	
	// Generate field mappings
	for _, dtoField := range dto.Fields {
		if dtoField.Ignore {
			continue
		}
		
		sourceFieldName := dtoField.Name
		if dtoField.FieldTag != "" {
			sourceFieldName = dtoField.FieldTag
		} else if config.FieldNameTransform == "snake_to_camel" {
			for srcFieldName := range source.Fields {
				if snakeToCamel(srcFieldName) == dtoField.Name {
					sourceFieldName = srcFieldName
					break
				}
			}
		}
		
		sourceField, exists := source.Fields[sourceFieldName]
		if !exists {
			methodBody = append(methodBody,
				jen.Comment(fmt.Sprintf("%s: not found in source, will be zero value", dtoField.Name)),
			)
			continue
		}
		
		if dtoField.ConverterTag != "" {
			// Use converter
			fromType := parseTypeForJen(sourceField.BaseType, importMap)
			toType := parseTypeForJen(extractBaseType(dtoField.Type), importMap)
			
			methodBody = append(methodBody,
				jen.Block(
					jen.Var().Err().Error(),
					jen.List(jen.Id("d").Dot(dtoField.Name), jen.Err()).Op("=").Id("Convert").Types(
						fromType, toType,
					).Call(
						jen.Lit(dtoField.ConverterTag),
						jen.Id("src").Dot(sourceFieldName),
					),
					jen.If(jen.Err().Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
							jen.Err(),
						)),
					),
				),
			)
		} else {
			// Direct assignment
			methodBody = append(methodBody,
				jen.Id("d").Dot(dtoField.Name).Op("=").Id("src").Dot(sourceFieldName),
			)
		}
	}
	
	methodBody = append(methodBody, jen.Line(), jen.Return(jen.Nil()))
	
	// Generate method
	f.Func().Params(
		jen.Id("d").Op("*").Id(dto.Name),
	).Id(methodName).Params(
		jen.Id("src").Op("*").Add(paramType),
	).Error().Block(methodBody...)
	
	f.Line()
}

func parseTypeForJen(typeName string, importMap map[string]string) jen.Code {
	// Handle pointers
	if strings.HasPrefix(typeName, "*") {
		return jen.Op("*").Add(parseTypeForJen(strings.TrimPrefix(typeName, "*"), importMap))
	}
	
	// Handle slices
	if strings.HasPrefix(typeName, "[]") {
		return jen.Index().Add(parseTypeForJen(strings.TrimPrefix(typeName, "[]"), importMap))
	}
	
	// Handle qualified types (e.g., time.Time, db.User)
	if strings.Contains(typeName, ".") {
		parts := strings.Split(typeName, ".")
		if len(parts) == 2 {
			alias := parts[0]
			typeIdent := parts[1]
			
			// Look up the actual import path from the alias
			if importPath, ok := importMap[alias]; ok {
				return jen.Qual(importPath, typeIdent)
			}
			
			// Fallback to standard library packages
			return jen.Qual(alias, typeIdent)
		}
	}
	
	// Simple type
	return jen.Id(typeName)
}

func parseTypeRefForJen(typeName string, importMap map[string]string) jen.Code {
	// For type references in parameters, handle package prefixes
	if strings.Contains(typeName, ".") {
		parts := strings.Split(typeName, ".")
		if len(parts) == 2 {
			alias := parts[0]
			typeIdent := parts[1]
			
			// Look up the actual import path from the alias
			if importPath, ok := importMap[alias]; ok {
				return jen.Qual(importPath, typeIdent)
			}
			
			// Fallback to standard library packages
			return jen.Qual(alias, typeIdent)
		}
	}
	
	return jen.Id(typeName)
}

func extractBaseType(typeName string) string {
	typeName = strings.TrimPrefix(typeName, "*")
	typeName = strings.TrimPrefix(typeName, "[]")
	return typeName
}

func extractTypeNameWithoutPackage(typeName string) string {
	if strings.Contains(typeName, ".") {
		parts := strings.Split(typeName, ".")
		return parts[len(parts)-1]
	}
	return typeName
}
