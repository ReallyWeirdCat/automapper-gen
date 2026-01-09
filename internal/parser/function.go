package parser

import (
	"go/ast"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
)

// ParseFunctions extracts function declarations from AST
func ParseFunctions(file *ast.File) map[string]types.FunctionInfo {
	functions := make(map[string]types.FunctionInfo)

	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Skip methods (functions with receivers)
			if funcDecl.Recv != nil {
				continue
			}

			funcInfo := types.FunctionInfo{
				Name: funcDecl.Name.Name,
			}

			// Analyze function signature
			if funcDecl.Type.Params != nil && len(funcDecl.Type.Params.List) > 0 {
				// Get parameter types
				for _, param := range funcDecl.Type.Params.List {
					paramType := exprToString(param.Type)
					funcInfo.ParamTypes = append(funcInfo.ParamTypes, paramType)
				}
			}

			if funcDecl.Type.Results != nil && len(funcDecl.Type.Results.List) > 0 {
				// Get return types
				for _, result := range funcDecl.Type.Results.List {
					resultType := exprToString(result.Type)
					funcInfo.ReturnTypes = append(funcInfo.ReturnTypes, resultType)
				}
			}

			// Check for converter annotation
			if funcDecl.Doc != nil {
				funcInfo.IsConverter = ExtractConverterAnnotation(funcDecl.Doc)
				
				if invertsFunc, isInverter := ExtractInverterAnnotation(funcDecl.Doc); isInverter {
					funcInfo.IsInverter = true
					funcInfo.InvertsFunc = invertsFunc
					// Inverters can also be used as converters
					funcInfo.IsConverter = true
				}
			}

			functions[funcInfo.Name] = funcInfo
		}
	}

	return functions
}

// IsSafeConverterSignature checks if a function matches safe converter signature: func(T) U
func IsSafeConverterSignature(fn types.FunctionInfo) bool {
	return len(fn.ParamTypes) == 1 && len(fn.ReturnTypes) == 1
}

// IsErrorReturningConverterSignature checks if a function matches error-returning converter signature: func(T) (U, error)
func IsErrorReturningConverterSignature(fn types.FunctionInfo) bool {
	if len(fn.ParamTypes) != 1 || len(fn.ReturnTypes) != 2 {
		return false
	}
	// Check if second return type is error
	return fn.ReturnTypes[1] == "error"
}
