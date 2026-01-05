package parser

import (
	"go/ast"
	"strings"
)

// ExtractAnnotation extracts the automapper annotation from comments
func ExtractAnnotation(doc *ast.CommentGroup) string {
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

// ParseSourceList parses a comma-separated list of source types
func ParseSourceList(annotation string) []string {
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
