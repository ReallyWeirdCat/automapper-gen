package parser

import (
	"go/ast"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
)

// extractTypeInfo extracts type information from an AST expression
func extractTypeInfo(expr ast.Expr) types.FieldTypeInfo {
	info := types.FieldTypeInfo{}

	switch t := expr.(type) {
	case *ast.StarExpr:
		// Pointer types: *T
		info.IsPointer = true
		info.BaseType = exprToString(t.X)
		info.Type = "*" + info.BaseType

	case *ast.ArrayType:
		// Slice or array types: []T or [N]T
		if t.Len == nil {
			// Slice: []T
			info.IsSlice = true
			info.BaseType = exprToString(t.Elt)
			info.Type = "[]" + info.BaseType
		} else {
			// Array: [N]T
			info.Type = exprToString(expr)
			info.BaseType = info.Type
		}

	case *ast.MapType:
		// Map types: map[K]V
		info.Type = exprToString(expr)
		info.BaseType = info.Type

	case *ast.InterfaceType:
		// Interface types: interface{} or interface{...}
		info.Type = exprToString(expr)
		info.BaseType = info.Type

	case *ast.ChanType:
		// Channel types: chan T, <-chan T, chan<- T
		info.Type = exprToString(expr)
		info.BaseType = info.Type

	case *ast.FuncType:
		// Function types: func(...)...
		info.Type = exprToString(expr)
		info.BaseType = info.Type

	case *ast.StructType:
		// Anonymous struct types
		info.Type = "struct{...}"
		info.BaseType = info.Type

	default:
		// Basic types and named types
		info.BaseType = exprToString(expr)
		info.Type = info.BaseType
	}

	return info
}

// exprToString converts an AST expression to its string representation
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		// Simple identifier: int, string, CustomType
		return t.Name

	case *ast.SelectorExpr:
		// Qualified identifier: pkg.Type
		return exprToString(t.X) + "." + t.Sel.Name

	case *ast.StarExpr:
		// Pointer: *T
		return "*" + exprToString(t.X)

	case *ast.ArrayType:
		// Slice or array: []T or [N]T
		if t.Len == nil {
			return "[]" + exprToString(t.Elt)
		}
		// For arrays with length, include the length
		return "[" + exprToString(t.Len) + "]" + exprToString(t.Elt)

	case *ast.MapType:
		// Map: map[K]V
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)

	case *ast.InterfaceType:
		// Interface: interface{} or interface{...}
		if len(t.Methods.List) == 0 {
			return "interface{}"
		}
		// For interfaces with methods, just use generic representation
		return "interface{...}"

	case *ast.ChanType:
		// Channel: chan T, <-chan T, chan<- T
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + exprToString(t.Value)
		case ast.RECV:
			return "<-chan " + exprToString(t.Value)
		default:
			return "chan " + exprToString(t.Value)
		}

	case *ast.FuncType:
		// Function: func(params) results
		return buildFuncTypeString(t)

	case *ast.StructType:
		// Anonymous struct
		return "struct{...}"

	case *ast.Ellipsis:
		// Variadic: ...T
		return "..." + exprToString(t.Elt)

	case *ast.BasicLit:
		// Literal (for array lengths, etc.)
		return t.Value

	default:
		return ""
	}
}

// buildFuncTypeString constructs a string representation of a function type
func buildFuncTypeString(ft *ast.FuncType) string {
	var parts []string
	parts = append(parts, "func")

	// Parameters
	if ft.Params != nil {
		params := buildFieldListString(ft.Params)
		parts = append(parts, "("+params+")")
	} else {
		parts = append(parts, "()")
	}

	// Results
	if ft.Results != nil {
		results := buildFieldListString(ft.Results)
		if len(ft.Results.List) == 1 && len(ft.Results.List[0].Names) == 0 {
			// Single unnamed return: func() T
			parts = append(parts, results)
		} else {
			// Multiple or named returns: func() (T, U) or func() (x T)
			parts = append(parts, "("+results+")")
		}
	}

	return strings.Join(parts, " ")
}

// buildFieldListString converts a field list to a string
func buildFieldListString(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}

	var fields []string
	for _, field := range fl.List {
		typeStr := exprToString(field.Type)

		if len(field.Names) == 0 {
			// Unnamed field
			fields = append(fields, typeStr)
		} else {
			// Named fields
			for _, name := range field.Names {
				fields = append(fields, name.Name+" "+typeStr)
			}
		}
	}

	return strings.Join(fields, ", ")
}
