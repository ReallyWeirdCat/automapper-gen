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

		if after, ok := strings.CutPrefix(text, "automapper:from="); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// ExtractBidirectionalFlag checks if struct should generate MapTo methods
func ExtractBidirectionalFlag(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}

	for _, comment := range doc.List {
		text := comment.Text
		text = strings.TrimSpace(text)

		if strings.HasPrefix(text, "//") {
			text = strings.TrimSpace(text[2:])
		} else if strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/") {
			text = strings.TrimSpace(text[2 : len(text)-2])
		}

		// Check for bidirectional flag
		if strings.Contains(text, "automapper:bidirectional") {
			return true
		}
	}
	return false
}

// ExtractConverterAnnotation checks if a function is annotated as a converter
func ExtractConverterAnnotation(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}

	for _, comment := range doc.List {
		text := comment.Text
		text = strings.TrimSpace(text)

		if strings.HasPrefix(text, "//") {
			text = strings.TrimSpace(text[2:])
		} else if strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/") {
			text = strings.TrimSpace(text[2 : len(text)-2])
		}

		if strings.TrimSpace(text) == "automapper:converter" {
			return true
		}
	}
	return false
}

// ExtractInverterAnnotation extracts the converter name that this function inverts
// Returns the converter function name and true if it's an inverter
func ExtractInverterAnnotation(doc *ast.CommentGroup) (string, bool) {
	if doc == nil {
		return "", false
	}

	for _, comment := range doc.List {
		text := comment.Text
		text = strings.TrimSpace(text)

		if strings.HasPrefix(text, "//") {
			text = strings.TrimSpace(text[2:])
		} else if strings.HasPrefix(text, "/*") && strings.HasSuffix(text, "*/") {
			text = strings.TrimSpace(text[2 : len(text)-2])
		}

		if after, ok := strings.CutPrefix(text, "automapper:inverter="); ok {
			converterName := strings.TrimSpace(after)
			return converterName, true
		}
	}
	return "", false
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
