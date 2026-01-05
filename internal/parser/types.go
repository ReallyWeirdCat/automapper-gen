package parser

import (
	"go/ast"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
)

// extractTypeInfo extracts type information from an AST expression
func extractTypeInfo(expr ast.Expr) types.FieldTypeInfo {
	info := types.FieldTypeInfo{}

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

// exprToString converts an AST expression to its string representation
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
