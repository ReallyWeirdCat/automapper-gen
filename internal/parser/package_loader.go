package parser

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/logger"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
	"golang.org/x/tools/go/packages"
)

// LoadExternalPackage loads a package from the module system (can be remote)
func LoadExternalPackage(importPath, alias string) (map[string]types.SourceStruct, error) {
	logger.Debug("Loading package via go/packages: %s", importPath)

	// Configure package loading
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
	}

	// Load the package
	logger.Debug("Invoking packages.Load for: %s", importPath)
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("loading package %s: %w", importPath, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for import path: %s", importPath)
	}

	pkg := pkgs[0]
	logger.Debug("Package loaded: %s (files: %d)", pkg.Name, len(pkg.Syntax))

	// Check for errors
	if len(pkg.Errors) > 0 {
		var errMsgs []string
		for _, e := range pkg.Errors {
			errMsgs = append(errMsgs, e.Error())
			logger.Debug("  Package error: %s", e.Error())
		}
		return nil, fmt.Errorf("package errors: %s", strings.Join(errMsgs, "; "))
	}

	sources := make(map[string]types.SourceStruct)

	// Use the package name if no alias provided
	if alias == "" {
		alias = pkg.Name
		logger.Debug("Using package name as alias: %s", alias)
	}

	// Parse all syntax trees in the package
	fileCount := 0
	totalStructs := 0
	for _, file := range pkg.Syntax {
		fileCount++
		structsInFile := 0

		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							structsInFile++
							totalStructs++

							sourceStruct := ParseStruct(structType)
							sourceStruct.Name = typeSpec.Name.Name
							sourceStruct.IsExternal = true
							sourceStruct.ImportPath = importPath
							sourceStruct.Alias = alias
							sourceStruct.Package = alias

							// Store with alias prefix
							key := alias + "." + typeSpec.Name.Name
							sources[key] = sourceStruct

							logger.Debug("  Found external struct: %s (%d fields)", key, len(sourceStruct.Fields))

							// Log field details if debug is enabled
							if logger.IsDebugEnabled() {
								for fieldName, fieldInfo := range sourceStruct.Fields {
									logger.Debug("    - %s: %s", fieldName, fieldInfo.Type)
								}
							}
						}
					}
				}
			}
		}

		if structsInFile > 0 {
			logger.Verbose("  File %d: found %d structs", fileCount, structsInFile)
		}
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no structs found in package: %s", importPath)
	}

	logger.Verbose("Successfully loaded %d structs from %s", totalStructs, importPath)
	return sources, nil
}
