package parser

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/logger"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
	"golang.org/x/tools/go/packages"
)

// ParsePackage parses the main package and external packages
func ParsePackage(
	pkgPath string, cfg *config.Config,
) (
	[]types.DTOMapping,
	map[string]types.SourceStruct,
	map[string]types.FunctionInfo,
	string,
	error,
) {
	// Parse main package using go/packages
	logger.Verbose("Parsing main package: %s", pkgPath)
	dtos, sources, functions, pkgName, err := parsePackageWithGoPackages(pkgPath, "", "", false, cfg)
	if err != nil {
		return nil, nil, nil, "", err
	}

	logger.Verbose("Main package parsed: %d DTOs, %d sources, %d functions", len(dtos), len(sources), len(functions))

	// Parse external packages
	if len(cfg.ExternalPackages) > 0 {
		logger.Verbose("Loading %d external packages...", len(cfg.ExternalPackages))
	}

	for i, extPkg := range cfg.ExternalPackages {
		logger.Verbose("[%d/%d] Loading external package: %s", i+1, len(cfg.ExternalPackages), extPkg.ImportPath)

		alias := extPkg.Alias
		if alias == "" {
			parts := strings.Split(extPkg.ImportPath, "/")
			alias = parts[len(parts)-1]
			logger.Debug("  Using default alias: %s", alias)
		}

		var extSources map[string]types.SourceStruct
		var parseErr error

		// Try local path first if provided (for development)
		if extPkg.LocalPath != "" {
			localPath := extPkg.LocalPath
			if !filepath.IsAbs(localPath) {
				localPath = filepath.Join(pkgPath, localPath)
			}

			logger.Verbose("  Loading from local path: %s", localPath)
			_, extSources, _, _, parseErr = parsePackageWithGoPackages(localPath, alias, extPkg.ImportPath, true, cfg)
		}

		// Load from module cache if local path not available or failed
		if extPkg.LocalPath == "" || parseErr != nil {
			if parseErr != nil {
				logger.Verbose("  Local path failed, trying module cache")
			} else {
				logger.Verbose("  Loading from module cache")
			}
			extSources, parseErr = LoadExternalPackage(extPkg.ImportPath, alias)
		}

		if parseErr != nil {
			return nil, nil, nil, "", fmt.Errorf("loading external package %s: %w", extPkg.ImportPath, parseErr)
		}

		// Merge sources
		for k, v := range extSources {
			sources[k] = v
			logger.Debug("  Added external struct: %s", k)
		}

		logger.Verbose("  Loaded %d structs from %s", len(extSources), extPkg.ImportPath)
	}

	return dtos, sources, functions, pkgName, nil
}

// parsePackageWithGoPackages uses go/packages to parse a package
func parsePackageWithGoPackages(
	pkgPath string, alias string, importPath string, isExternal bool, cfg *config.Config,
) (
	[]types.DTOMapping,
	map[string]types.SourceStruct,
	map[string]types.FunctionInfo,
	string,
	error,
) {
	logger.Debug("Parsing package with go/packages: %s (external: %v)", pkgPath, isExternal)

	// Configure package loading
	pkgCfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Dir: pkgPath,
	}

	// Load the package - use "." to load the package in the current directory
	logger.Debug("Invoking packages.Load for directory: %s", pkgPath)
	pkgs, err := packages.Load(pkgCfg, ".")
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("loading package: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, nil, nil, "", fmt.Errorf("no packages found in: %s", pkgPath)
	}

	// Use the first package (there should typically be only one when loading ".")
	pkg := pkgs[0]

	// Check for errors
	if len(pkg.Errors) > 0 {
		var errMsgs []string
		for _, e := range pkg.Errors {
			errMsgs = append(errMsgs, e.Error())
			logger.Debug("  Package error: %s", e.Error())
		}
		return nil, nil, nil, "", fmt.Errorf("package errors: %s", strings.Join(errMsgs, "; "))
	}

	logger.Debug("Package loaded: %s (files: %d)", pkg.Name, len(pkg.Syntax))

	dtos := []types.DTOMapping{}
	sources := make(map[string]types.SourceStruct)
	functions := make(map[string]types.FunctionInfo)
	pkgName := pkg.Name

	if importPath == "" {
		importPath = pkg.PkgPath
	}

	if alias == "" {
		alias = pkg.Name
	}

	fileCount := 0
	totalStructs := 0
	totalFunctions := 0
	totalFiles := 0

	// Get the correct file list to use
	fileList := pkg.GoFiles
	if len(fileList) == 0 {
		fileList = pkg.CompiledGoFiles
	}

	logger.Debug("Using file list with %d files", len(fileList))

	// Count total files to process
	for i := range pkg.Syntax {
		if i >= len(fileList) {
			continue
		}
		fileName := fileList[i]
		baseName := filepath.Base(fileName)
		if !strings.HasSuffix(baseName, "_test.go") && baseName != cfg.Output {
			totalFiles++
		}
	}

	// Parse all files
	for i, file := range pkg.Syntax {
		if i >= len(fileList) {
			logger.Debug("  Warning: file index %d out of range for file list", i)
			continue
		}

		fileName := fileList[i]
		baseName := filepath.Base(fileName)

		// Skip test files and the output file
		if strings.HasSuffix(baseName, "_test.go") || baseName == cfg.Output {
			logger.Debug("  Skipping file: %s", baseName)
			continue
		}

		logger.Debug("  Including file: %s", baseName)
		fileCount++

		logger.Debug("  [%d/%d] Parsing file: %s", fileCount, totalFiles, baseName)

		structsInFile := 0

		// Extract structs from the file
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							structsInFile++
							totalStructs++

							sourceStruct := ParseStruct(structType)
							sourceStruct.Name = typeSpec.Name.Name
							sourceStruct.IsExternal = isExternal
							sourceStruct.ImportPath = importPath
							sourceStruct.Alias = alias

							if isExternal {
								sourceStruct.Package = alias
								key := alias + "." + typeSpec.Name.Name
								sources[key] = sourceStruct
								logger.Debug("    Found struct: %s (%d fields)", key, len(sourceStruct.Fields))
							} else {
								sourceStruct.Package = pkgName
								sources[typeSpec.Name.Name] = sourceStruct
								logger.Debug("    Found struct: %s (%d fields)", typeSpec.Name.Name, len(sourceStruct.Fields))
							}
						}
					}
				}
			}
		}

		if structsInFile > 0 {
			logger.Verbose("    Found %d structs in %s", structsInFile, baseName)
		}

		// Parse functions (only in non-external packages)
		if !isExternal {
			fileFunctions := ParseFunctions(file)
			for name, fn := range fileFunctions {
				functions[name] = fn
				totalFunctions++
				logger.Debug("    Found function: %s (params: %d, returns: %d)", name, len(fn.ParamTypes), len(fn.ReturnTypes))
			}
		}

		// Parse DTOs (only in non-external packages)
		if !isExternal {
			dtoCount := 0
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							var annotation string
							if genDecl.Doc != nil {
								annotation = ExtractAnnotation(genDecl.Doc)
							}
							if annotation == "" && typeSpec.Doc != nil {
								annotation = ExtractAnnotation(typeSpec.Doc)
							}

							if annotation != "" {
								dtoCount++
								if structType, ok := typeSpec.Type.(*ast.StructType); ok {
									dto := types.DTOMapping{
										Name:        typeSpec.Name.Name,
										Sources:     ParseSourceList(annotation),
										Fields:      ParseFields(structType),
										PackageName: pkgName,
									}
									dtos = append(dtos, dto)
									logger.Verbose("    Found DTO: %s <- %v (%d fields)",
										dto.Name, dto.Sources, len(dto.Fields))

									// Log field details in debug mode
									if logger.IsDebugEnabled() {
										for _, field := range dto.Fields {
											tags := ""
											if field.ConverterTag != "" {
												tags += fmt.Sprintf(" [converter=%s]", field.ConverterTag)
											}
											if field.FieldTag != "" {
												tags += fmt.Sprintf(" [field=%s]", field.FieldTag)
											}
											if field.NestedDTO != "" {
												tags += fmt.Sprintf(" [dto=%s]", field.NestedDTO)
											}
											if field.Ignore {
												tags += " [ignored]"
											}
											logger.Debug("      - %s: %s%s", field.Name, field.Type, tags)
										}
									}
								}
							}
						}
					}
				}
			}

			if dtoCount > 0 {
				logger.Verbose("    Found %d DTOs in %s", dtoCount, baseName)
			}
		}
	}

	logger.Debug("Completed parsing package: %d DTOs, %d sources, %d functions", len(dtos), len(sources), len(functions))
	return dtos, sources, functions, pkgName, nil
}
