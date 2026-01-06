package parser

import (
	"go/ast"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
)

// ParseStruct extracts field information from a struct type
func ParseStruct(structType *ast.StructType) types.SourceStruct {
	s := types.SourceStruct{
		Fields: make(map[string]types.FieldTypeInfo),
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

// ParseFields extracts field information including tags
func ParseFields(structType *ast.StructType) []types.FieldInfo {
	fields := []types.FieldInfo{}

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue
		}

		fieldInfo := types.FieldInfo{
			Name: field.Names[0].Name,
			Type: exprToString(field.Type),
		}

		if field.Tag != nil {
			tag := field.Tag.Value
			tag = strings.Trim(tag, "`")
			fieldInfo.Tag = tag

			if strings.Contains(tag, "automapper:") {
				fieldInfo.ConverterTag, fieldInfo.FieldTag, fieldInfo.NestedDTO, fieldInfo.Ignore = parseAutomapperTag(tag)
			}
		}

		fields = append(fields, fieldInfo)
	}

	return fields
}

// parseAutomapperTag parses the automapper struct tag
func parseAutomapperTag(tag string) (converter, field, nestedDTO string, ignore bool) {
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

	parts := strings.SplitSeq(automapperTag, ",")
	for part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			switch key {
			case "converter":
				converter = value
			case "field":
				field = value
			case "dto":
				nestedDTO = value
			}
		}
	}

	return
}

// SnakeToCamel converts snake_case to CamelCase
func SnakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}
