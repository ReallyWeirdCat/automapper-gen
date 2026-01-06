package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
)

// ParsePackage parses the main package and external packages
func ParsePackage(
	pkgPath string, cfg *config.Config,
) ([]types.DTOMapping, map[string]types.SourceStruct, string, error) {
	fset := token.NewFileSet()

	// Parse main package
	dtos, sources, pkgName, err := parseDir(fset, pkgPath, "", pkgPath, false, cfg)
	if err != nil {
		return nil, nil, "", err
	}

	// Parse external packages
	for _, extPkg := range cfg.ExternalPackages {
		alias := extPkg.Alias
		if alias == "" {
			parts := strings.Split(extPkg.ImportPath, "/")
			alias = parts[len(parts)-1]
		}

		var extSources map[string]types.SourceStruct
		var parseErr error

		// Try local path first if provided (for development)
		if extPkg.LocalPath != "" {
			localPath := extPkg.LocalPath
			if !filepath.IsAbs(localPath) {
				localPath = filepath.Join(pkgPath, localPath)
			}

			if _, err := os.Stat(localPath); err == nil {
				fmt.Printf("Loading external package %s from local path: %s\n", extPkg.ImportPath, localPath)
				_, extSources, _, parseErr = parseDir(fset, localPath, alias, extPkg.ImportPath, true, cfg)
			} else {
				fmt.Printf("Local path not found for %s, loading from module cache\n", extPkg.ImportPath)
			}
		}

		// Load from module cache if local path not available or failed
		if extPkg.LocalPath == "" || parseErr != nil {
			fmt.Printf("Loading external package %s from module cache\n", extPkg.ImportPath)
			extSources, parseErr = LoadExternalPackage(extPkg.ImportPath, alias)
		}

		if parseErr != nil {
			return nil, nil, "", fmt.Errorf("loading external package %s: %w", extPkg.ImportPath, parseErr)
		}

		// Merge sources
		for k, v := range extSources {
			sources[k] = v
		}

		fmt.Printf("Loaded %d structs from %s\n", len(extSources), extPkg.ImportPath)
	}

	return dtos, sources, pkgName, nil
}

// parseDir parses a directory of Go files
func parseDir(
	fset *token.FileSet,
	dir string,
	alias string,
	importPath string,
	isExternal bool,
	cfg *config.Config,
) ([]types.DTOMapping, map[string]types.SourceStruct, string, error) {
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && fi.Name() != cfg.Output
	}, parser.ParseComments)
	if err != nil {
		return nil, nil, "", err
	}

	dtos := []types.DTOMapping{}
	sources := make(map[string]types.SourceStruct)
	var pkgName string

	for name, pkg := range pkgs {
		pkgName = name

		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								sourceStruct := ParseStruct(structType)
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

			// Parse DTOs (only in non-external packages)
			if !isExternal {
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
									if structType, ok := typeSpec.Type.(*ast.StructType); ok {
										dto := types.DTOMapping{
											Name:        typeSpec.Name.Name,
											Sources:     ParseSourceList(annotation),
											Fields:      ParseFields(structType),
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
