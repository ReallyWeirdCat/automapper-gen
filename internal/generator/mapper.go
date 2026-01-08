package generator

import (
	"fmt"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/parser"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
	"github.com/dave/jennifer/jen"
)

// GenerateMapFromMethod generates a MapFrom method for a DTO
func GenerateMapFromMethod(
	f *jen.File,
	dto types.DTOMapping,
	source types.SourceStruct,
	sourceName, methodName string,
	cfg *config.Config,
	importMap map[string]string,
) {
	// Parse parameter type
	paramType := ParseTypeRefForJen(sourceName, importMap)

	// Determine if this mapper can be safe (no errors possible)
	isSafe := isMapperSafe(dto, cfg)

	// Generate safe mapper if enabled and mapper is safe
	if cfg.EnableSafeMappers && isSafe {
		safeMethodName := "Safe" + methodName

		f.Comment(fmt.Sprintf("%s maps from %s to %s (no errors possible)", safeMethodName, sourceName, dto.Name))

		// Build safe method body (no error returns)
		methodBody := buildSafeMethodBody(dto, source, cfg)

		// Generate safe method (no error return)
		f.Func().Params(
			jen.Id("d").Op("*").Id(dto.Name),
		).Id(safeMethodName).Params(
			jen.Id("src").Op("*").Add(paramType),
		).Block(methodBody...)

		f.Line()

		// Generate wrapper if enabled
		if cfg.EnableUnsafeWrappers {
			f.Comment(fmt.Sprintf("%s wraps %s for backward compatibility", methodName, safeMethodName))

			f.Func().Params(
				jen.Id("d").Op("*").Id(dto.Name),
			).Id(methodName).Params(
				jen.Id("src").Op("*").Add(paramType),
			).Error().Block(
				jen.If(jen.Id("src").Op("==").Nil()).Block(
					jen.Return(jen.Qual("errors", "New").Call(jen.Lit("source is nil"))),
				),
				jen.Id("d").Dot(safeMethodName).Call(jen.Id("src")),
				jen.Return(jen.Nil()),
			)

			f.Line()
		}
	} else {
		// Generate regular error-returning method
		f.Comment(fmt.Sprintf("%s maps from %s to %s", methodName, sourceName, dto.Name))

		// Build regular method body
		methodBody := buildMethodBody(dto, source, cfg)

		// Generate method
		f.Func().Params(
			jen.Id("d").Op("*").Id(dto.Name),
		).Id(methodName).Params(
			jen.Id("src").Op("*").Add(paramType),
		).Error().Block(methodBody...)

		f.Line()
	}
}

// isMapperSafe determines if a mapper can never produce errors
func isMapperSafe(dto types.DTOMapping, cfg *config.Config) bool {
	// Build converter map for quick lookup
	converterMap := make(map[string]config.ConverterDef)
	for _, conv := range cfg.DefaultConverters {
		converterMap[conv.Name] = conv
	}

	for _, field := range dto.Fields {
		if field.Ignore {
			continue
		}

		// Nested DTOs can fail
		if field.NestedDTO != "" {
			return false
		}

		// If converter is used, check if it's safe
		if field.ConverterTag != "" {
			conv, exists := converterMap[field.ConverterTag]
			if !exists {
				// Unknown converter - assume unsafe
				return false
			}
			if !conv.Trusted {
				// Converter returns error
				return false
			}
		}

		// Pointer conversions are always safe
		// Direct assignments are always safe
	}

	return true
}

// buildSafeMethodBody constructs the safe method body (no error handling)
func buildSafeMethodBody(
	dto types.DTOMapping, source types.SourceStruct, cfg *config.Config,
) []jen.Code {
	statements := []jen.Code{
		jen.If(jen.Id("src").Op("==").Nil()).Block(
			jen.Return(),
		),
		jen.Line(),
	}

	// Build converter map
	converterMap := make(map[string]config.ConverterDef)
	for _, conv := range cfg.DefaultConverters {
		converterMap[conv.Name] = conv
	}

	// Generate field mappings
	for _, dtoField := range dto.Fields {
		if dtoField.Ignore {
			continue
		}

		sourceFieldName := resolveSourceFieldName(dtoField, source, cfg)
		sourceField, exists := source.Fields[sourceFieldName]

		if !exists {
			statements = append(statements,
				jen.Comment(fmt.Sprintf("%s: not found in source, will be zero value", dtoField.Name)),
			)
			continue
		}

		// Only converters and direct mappings are allowed in safe mappers
		if dtoField.ConverterTag != "" {
			conv := converterMap[dtoField.ConverterTag]
			statements = append(statements, buildSafeConverterMapping(dtoField, sourceField, sourceFieldName, conv)...)
		} else {
			statements = append(statements, buildFieldMapping(dtoField, sourceField, sourceFieldName)...)
		}
	}

	statements = append(statements, jen.Line())
	return statements
}

// buildMethodBody constructs the regular method body with error handling
func buildMethodBody(
	dto types.DTOMapping, source types.SourceStruct, cfg *config.Config,
) []jen.Code {
	statements := []jen.Code{
		jen.If(jen.Id("src").Op("==").Nil()).Block(
			jen.Return(jen.Qual("errors", "New").Call(jen.Lit("source is nil"))),
		),
		jen.Line(),
	}

	// Build converter map
	converterMap := make(map[string]config.ConverterDef)
	for _, conv := range cfg.DefaultConverters {
		converterMap[conv.Name] = conv
	}

	// Generate field mappings
	for _, dtoField := range dto.Fields {
		if dtoField.Ignore {
			continue
		}

		sourceFieldName := resolveSourceFieldName(dtoField, source, cfg)
		sourceField, exists := source.Fields[sourceFieldName]

		if !exists {
			statements = append(statements,
				jen.Comment(fmt.Sprintf("%s: not found in source, will be zero value", dtoField.Name)),
			)
			continue
		}

		// Nested DTO mapping takes precedence
		if dtoField.NestedDTO != "" {
			statements = append(statements, buildNestedDTOMapping(dtoField, sourceField, sourceFieldName)...)
		} else if dtoField.ConverterTag != "" {
			conv, exists := converterMap[dtoField.ConverterTag]
			if !exists {
				// This should be caught by validation, but handle it gracefully
				statements = append(statements,
					jen.Comment(fmt.Sprintf("%s: converter '%s' not found", dtoField.Name, dtoField.ConverterTag)),
				)
				continue
			}
			statements = append(statements, buildConverterMapping(dtoField, sourceField, sourceFieldName, conv)...)
		} else {
			statements = append(statements, buildFieldMapping(dtoField, sourceField, sourceFieldName)...)
		}
	}

	statements = append(statements, jen.Line(), jen.Return(jen.Nil()))
	return statements
}

// resolveSourceFieldName determines the source field name for a DTO field
func resolveSourceFieldName(
	dtoField types.FieldInfo, source types.SourceStruct, cfg *config.Config,
) string {
	if dtoField.FieldTag != "" {
		return dtoField.FieldTag
	}

	if cfg.FieldNameTransform == "snake_to_camel" {
		for srcFieldName := range source.Fields {
			if parser.SnakeToCamel(srcFieldName) == dtoField.Name {
				return srcFieldName
			}
		}
	}

	return dtoField.Name
}

// buildSafeConverterMapping creates statements for safe converter (no error)
func buildSafeConverterMapping(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	conv config.ConverterDef,
) []jen.Code {
	srcIsPointer := sourceField.IsPointer
	dstIsPointer := strings.HasPrefix(dtoField.Type, "*")

	// Safe converters have signature: func(T) U
	// So we need to handle pointer conversions ourselves

	// Case 1: Source is pointer
	if srcIsPointer {
		if dstIsPointer {
			// *T -> dereference -> converter -> T -> take address -> *T
			return []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Id("result").Op(":=").Id(conv.Function).Call(
						jen.Op("*").Id("src").Dot(sourceFieldName),
					),
					jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in nil", dtoField.Name)),
			}
		} else {
			// *T -> dereference -> converter -> T
			return []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Id("d").Dot(dtoField.Name).Op("=").Id(conv.Function).Call(
						jen.Op("*").Id("src").Dot(sourceFieldName),
					),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in zero value", dtoField.Name)),
			}
		}
	}

	// Case 2: Source is value, destination is pointer
	if dstIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("result").Op(":=").Id(conv.Function).Call(
					jen.Id("src").Dot(sourceFieldName),
				),
				jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
			),
		}
	}

	// Case 3: Both are values
	return []jen.Code{
		jen.Id("d").Dot(dtoField.Name).Op("=").Id(conv.Function).Call(
			jen.Id("src").Dot(sourceFieldName),
		),
	}
}

// buildConverterMapping creates statements for error-returning converter
func buildConverterMapping(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	conv config.ConverterDef,
) []jen.Code {
	srcIsPointer := sourceField.IsPointer
	dstIsPointer := strings.HasPrefix(dtoField.Type, "*")

	// For safe converters, use the safe version
	if conv.Trusted {
		return buildSafeConverterMapping(dtoField, sourceField, sourceFieldName, conv)
	}

	// Error-returning converters have signature: func(T) (U, error)
	var statements []jen.Code

	// Case 1: Source is pointer
	if srcIsPointer {
		if dstIsPointer {
			// *T -> dereference -> converter -> T -> take address -> *T
			statements = []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Var().Id("result").Id(ExtractBaseType(dtoField.Type)),
					jen.Var().Id("err").Error(),
					jen.List(jen.Id("result"), jen.Id("err")).Op("=").Id(conv.Function).Call(
						jen.Op("*").Id("src").Dot(sourceFieldName),
					),
					jen.If(jen.Id("err").Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
							jen.Id("err"),
						)),
					),
					jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in nil", dtoField.Name)),
			}
		} else {
			// *T -> dereference -> converter -> T
			statements = []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Var().Id("err").Error(),
					jen.List(jen.Id("d").Dot(dtoField.Name), jen.Id("err")).Op("=").Id(conv.Function).Call(
						jen.Op("*").Id("src").Dot(sourceFieldName),
					),
					jen.If(jen.Id("err").Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
							jen.Id("err"),
						)),
					),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in zero value", dtoField.Name)),
			}
		}
	} else if dstIsPointer {
		// Case 2: Source is value, destination is pointer
		statements = []jen.Code{
			jen.Block(
				jen.Var().Id("result").Id(ExtractBaseType(dtoField.Type)),
				jen.Var().Id("err").Error(),
				jen.List(jen.Id("result"), jen.Id("err")).Op("=").Id(conv.Function).Call(
					jen.Id("src").Dot(sourceFieldName),
				),
				jen.If(jen.Id("err").Op("!=").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
						jen.Id("err"),
					)),
				),
				jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
			),
		}
	} else {
		// Case 3: Both are values
		statements = []jen.Code{
			jen.Block(
				jen.Var().Id("err").Error(),
				jen.List(jen.Id("d").Dot(dtoField.Name), jen.Id("err")).Op("=").Id(conv.Function).Call(
					jen.Id("src").Dot(sourceFieldName),
				),
				jen.If(jen.Id("err").Op("!=").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
						jen.Id("err"),
					)),
				),
			),
		}
	}

	return statements
}

// buildNestedDTOMapping creates statements for nested DTO mapping with pointer and slice handling
func buildNestedDTOMapping(
	dtoField types.FieldInfo, sourceField types.FieldTypeInfo, sourceFieldName string,
) []jen.Code {
	dtoTypeName := dtoField.NestedDTO
	sourceTypeName := sourceField.BaseType

	// Determine the MapFrom method name based on source type
	methodName := "MapFrom"
	if strings.Contains(sourceTypeName, ".") {
		methodName = "MapFrom" + ExtractTypeNameWithoutPackage(sourceTypeName)
	} else {
		methodName = "MapFrom" + sourceTypeName
	}

	dtoIsPointer := strings.HasPrefix(dtoField.Type, "*")
	dtoIsSlice := strings.HasPrefix(dtoField.Type, "[]")
	srcIsPointer := sourceField.IsPointer
	srcIsSlice := sourceField.IsSlice

	// Handle slice to slice mapping
	if dtoIsSlice && srcIsSlice {
		return buildNestedSliceMapping(dtoField, sourceField, sourceFieldName, dtoTypeName, methodName)
	}

	// Handle pointer to pointer
	if dtoIsPointer && srcIsPointer {
		return []jen.Code{
			jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
				jen.Id("nested").Op(":=").Op("&").Id(dtoTypeName).Values(),
				jen.Var().Id("err").Error(),
				jen.Id("err").Op("=").Id("nested").Dot(methodName).Call(jen.Id("src").Dot(sourceFieldName)),
				jen.If(
					jen.Id("err").Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
						jen.Id("err"),
					)),
				),
				jen.Id("d").Dot(dtoField.Name).Op("=").Id("nested"),
			),
			jen.Comment(fmt.Sprintf("// %s: nil pointer will result in nil", dtoField.Name)),
		}
	}

	// Handle pointer to value
	if !dtoIsPointer && srcIsPointer {
		return []jen.Code{
			jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
				jen.Var().Id("nested").Id(dtoTypeName),
				jen.Var().Id("err").Error(),
				jen.Id("err").Op("=").Id("nested").Dot(methodName).Call(jen.Id("src").Dot(sourceFieldName)),
				jen.If(
					jen.Id("err").Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
						jen.Id("err"),
					)),
				),
				jen.Id("d").Dot(dtoField.Name).Op("=").Id("nested"),
			),
			jen.Comment(fmt.Sprintf("// %s: nil pointer will result in zero value", dtoField.Name)),
		}
	}

	// Handle value to pointer
	if dtoIsPointer && !srcIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("nested").Op(":=").Op("&").Id(dtoTypeName).Values(),
				jen.Var().Id("err").Error(),
				jen.Id("err").Op("=").Id("nested").Dot(methodName).Call(jen.Op("&").Id("src").Dot(sourceFieldName)),
				jen.If(
					jen.Id("err").Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
						jen.Id("err"),
					)),
				),
				jen.Id("d").Dot(dtoField.Name).Op("=").Id("nested"),
			),
		}
	}

	// Handle value to value (default case)
	return []jen.Code{
		jen.Block(
			jen.Var().Id("nested").Id(dtoTypeName),
			jen.Var().Id("err").Error(),
			jen.Id("err").Op("=").Id("nested").Dot(methodName).Call(jen.Op("&").Id("src").Dot(sourceFieldName)),
			jen.If(
				jen.Id("err").Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
					jen.Id("err"),
				)),
			),
			jen.Id("d").Dot(dtoField.Name).Op("=").Id("nested"),
		),
	}
}

// buildNestedSliceMapping handles slice to slice nested DTO mappings
func buildNestedSliceMapping(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	dtoTypeName string,
	methodName string,
) []jen.Code {
	// Extract slice element types
	dtoElemType := strings.TrimPrefix(dtoField.Type, "[]")
	srcElemType := strings.TrimPrefix(sourceField.Type, "[]")

	dtoElemIsPointer := strings.HasPrefix(dtoElemType, "*")
	srcElemIsPointer := strings.HasPrefix(srcElemType, "*")

	// Clean DTO type name
	cleanDtoTypeName := strings.TrimPrefix(dtoTypeName, "*")

	// Case 1: []T -> []DTO
	if !srcElemIsPointer && !dtoElemIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("d").Dot(dtoField.Name).Op("=").Make(jen.Index().Id(cleanDtoTypeName), jen.Len(jen.Id("src").Dot(sourceFieldName))),
				jen.For(jen.List(jen.Id("i"), jen.Id("item")).Op(":=").Range().Id("src").Dot(sourceFieldName)).Block(
					jen.Var().Id("err").Error(),
					jen.Id("err").Op("=").Id("d").Dot(dtoField.Name).Index(jen.Id("i")).Dot(methodName).Call(jen.Op("&").Id("item")),
					jen.If(
						jen.Id("err").Op("!=").Nil(),
					).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
							jen.Id("i"),
							jen.Id("err"),
						)),
					),
				),
			),
		}
	}

	// Case 2: []*T -> []*DTO
	if srcElemIsPointer && dtoElemIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("d").Dot(dtoField.Name).Op("=").Make(jen.Index().Op("*").Id(cleanDtoTypeName), jen.Len(jen.Id("src").Dot(sourceFieldName))),
				jen.For(jen.List(jen.Id("i"), jen.Id("item")).Op(":=").Range().Id("src").Dot(sourceFieldName)).Block(
					jen.If(jen.Id("item").Op("!=").Nil()).Block(
						jen.Id("nested").Op(":=").Op("&").Id(cleanDtoTypeName).Values(),
						jen.Var().Id("err").Error(),
						jen.Id("err").Op("=").Id("nested").Dot(methodName).Call(jen.Id("item")),
						jen.If(
							jen.Id("err").Op("!=").Nil(),
						).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
								jen.Id("i"),
								jen.Id("err"),
							)),
						),
						jen.Id("d").Dot(dtoField.Name).Index(jen.Id("i")).Op("=").Id("nested"),
					),
				),
			),
		}
	}

	// Case 3: []T -> []*DTO
	if !srcElemIsPointer && dtoElemIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("d").Dot(dtoField.Name).Op("=").Make(jen.Index().Op("*").Id(cleanDtoTypeName), jen.Len(jen.Id("src").Dot(sourceFieldName))),
				jen.For(jen.List(jen.Id("i"), jen.Id("item")).Op(":=").Range().Id("src").Dot(sourceFieldName)).Block(
					jen.Id("nested").Op(":=").Op("&").Id(cleanDtoTypeName).Values(),
					jen.Var().Id("err").Error(),
					jen.Id("err").Op("=").Id("nested").Dot(methodName).Call(jen.Op("&").Id("item")),
					jen.If(
						jen.Id("err").Op("!=").Nil(),
					).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
							jen.Id("i"),
							jen.Id("err"),
						)),
					),
					jen.Id("d").Dot(dtoField.Name).Index(jen.Id("i")).Op("=").Id("nested"),
				),
			),
		}
	}

	// Case 4: []*T -> []DTO
	if srcElemIsPointer && !dtoElemIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("d").Dot(dtoField.Name).Op("=").Make(jen.Index().Id(cleanDtoTypeName), jen.Lit(0), jen.Len(jen.Id("src").Dot(sourceFieldName))),
				jen.For(jen.List(jen.Id("i"), jen.Id("item")).Op(":=").Range().Id("src").Dot(sourceFieldName)).Block(
					jen.If(jen.Id("item").Op("!=").Nil()).Block(
						jen.Var().Id("nested").Id(cleanDtoTypeName),
						jen.Var().Id("err").Error(),
						jen.Id("err").Op("=").Id("nested").Dot(methodName).Call(jen.Id("item")),
						jen.If(
							jen.Id("err").Op("!=").Nil(),
						).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
								jen.Id("i"),
								jen.Id("err"),
							)),
						),
						jen.Id("d").Dot(dtoField.Name).Op("=").Append(jen.Id("d").Dot(dtoField.Name), jen.Id("nested")),
					),
				),
			),
		}
	}

	// Fallback
	return []jen.Code{
		jen.Comment(fmt.Sprintf("// %s: unsupported slice mapping", dtoField.Name)),
	}
}

// buildFieldMapping creates statements for field mapping with pointer conversion
func buildFieldMapping(
	dtoField types.FieldInfo, sourceField types.FieldTypeInfo, sourceFieldName string,
) []jen.Code {
	dtoIsPointer := strings.HasPrefix(dtoField.Type, "*")
	srcIsPointer := sourceField.IsPointer

	// Extract base types for comparison
	dtoBaseType := ExtractBaseType(dtoField.Type)
	srcBaseType := sourceField.BaseType

	// If base types don't match, direct assignment
	if dtoBaseType != srcBaseType {
		return []jen.Code{
			jen.Id("d").Dot(dtoField.Name).Op("=").Id("src").Dot(sourceFieldName),
		}
	}

	// Case 1: Both are pointers or both are values - direct assignment
	if dtoIsPointer == srcIsPointer {
		return []jen.Code{
			jen.Id("d").Dot(dtoField.Name).Op("=").Id("src").Dot(sourceFieldName),
		}
	}

	// Case 2: Source is pointer, destination is value
	if srcIsPointer && !dtoIsPointer {
		return []jen.Code{
			jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
				jen.Id("d").Dot(dtoField.Name).Op("=").Op("*").Id("src").Dot(sourceFieldName),
			),
			jen.Comment(fmt.Sprintf("// %s: nil pointer will result in zero value", dtoField.Name)),
		}
	}

	// Case 3: Source is value, destination is pointer
	if !srcIsPointer && dtoIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("v").Op(":=").Id("src").Dot(sourceFieldName),
				jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("v"),
			),
		}
	}

	// Fallback
	return []jen.Code{
		jen.Id("d").Dot(dtoField.Name).Op("=").Id("src").Dot(sourceFieldName),
	}
}
