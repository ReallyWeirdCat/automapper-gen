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
	functions map[string]types.FunctionInfo,
) {
	// Parse parameter type
	paramType := ParseTypeRefForJen(sourceName, importMap)

	f.Comment(fmt.Sprintf("%s maps from %s to %s", methodName, sourceName, dto.Name))

	methodBody, ignoredFields := buildMethodBody(dto, source, cfg, functions)

	// Add comment about ignored fields if any
	if len(ignoredFields) > 0 {
		f.Comment(fmt.Sprintf("Ignored fields: %s", strings.Join(ignoredFields, ", ")))
	}

	// Generate method
	f.Func().Params(
		jen.Id("d").Op("*").Id(dto.Name),
	).Id(methodName).Params(
		jen.Id("src").Op("*").Add(paramType),
	).Error().Block(methodBody...)

	f.Line()
}

// buildMethodBody constructs the regular method body with error handling
// Returns statements and list of ignored field names
func buildMethodBody(
	dto types.DTOMapping,
	source types.SourceStruct,
	cfg *config.Config,
	functions map[string]types.FunctionInfo,
) ([]jen.Code, []string) {
	statements := []jen.Code{
		jen.If(jen.Id("src").Op("==").Nil()).Block(
			jen.Return(jen.Qual("errors", "New").Call(jen.Lit("source is nil"))),
		),
		jen.Line(),
	}

	var ignoredFields []string

	// Build converter map
	converterMap := make(map[string]config.ConverterDef)
	for _, conv := range cfg.Converters {
		converterMap[conv.Name] = conv
	}

	// Generate field mappings
	for _, dtoField := range dto.Fields {
		if dtoField.Ignore {
			ignoredFields = append(ignoredFields, dtoField.Name)
			continue
		}

		sourceFieldName := resolveSourceFieldName(dtoField)
		sourceField, exists := source.Fields[sourceFieldName]

		if !exists {
			statements = append(statements,
				jen.Comment(fmt.Sprintf("%s: not found in source, will be zero value", dtoField.Name)),
			)
			ignoredFields = append(ignoredFields, dtoField.Name)
			continue
		}

		// Nested DTO mapping takes precedence
		if dtoField.NestedDTO != "" {
			statements = append(statements, buildNestedDTOMapping(dtoField, sourceField, sourceFieldName)...)
		} else if dtoField.ConverterTag != "" {
			conv, exists := converterMap[dtoField.ConverterTag]
			if !exists {
				// Try to find converter function directly by name
				fn, fnExists := functions[dtoField.ConverterTag]
				if !fnExists {
					statements = append(statements,
						jen.Comment(fmt.Sprintf("%s: converter '%s' not found", dtoField.Name, dtoField.ConverterTag)),
					)
					ignoredFields = append(ignoredFields, dtoField.Name)
					continue
				}
				
				// Use function directly as converter
				isSafe := parser.IsSafeConverterSignature(fn)
				statements = append(statements, buildConverterMappingDirect(dtoField, sourceField, sourceFieldName, dtoField.ConverterTag, isSafe)...)
			} else {
				// Check if converter is safe (1 return) or error-returning (2 returns)
				fn, fnExists := functions[conv.Function]
				isSafe := fnExists && parser.IsSafeConverterSignature(fn)

				statements = append(statements, buildConverterMapping(dtoField, sourceField, sourceFieldName, conv, isSafe)...)
			}
		} else {
			statements = append(statements, buildFieldMapping(dtoField, sourceField, sourceFieldName)...)
		}
	}

	statements = append(statements, jen.Line(), jen.Return(jen.Nil()))
	return statements, ignoredFields
}

// resolveSourceFieldName determines the source field name for a DTO field
func resolveSourceFieldName(
	dtoField types.FieldInfo,
) string {
	if dtoField.FieldTag != "" {
		return dtoField.FieldTag
	}

	return dtoField.Name
}

// buildConverterMappingDirect creates statements for converter used directly by function name
func buildConverterMappingDirect(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	functionName string,
	isSafe bool,
) []jen.Code {
	if isSafe {
		return buildSafeConverterMappingDirect(dtoField, sourceField, sourceFieldName, functionName)
	}
	return buildErrorReturningConverterMappingDirect(dtoField, sourceField, sourceFieldName, functionName)
}

// buildSafeConverterMappingDirect creates statements for safe converter (no error) used directly
func buildSafeConverterMappingDirect(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	functionName string,
) []jen.Code {
	srcIsPointer := sourceField.IsPointer
	dstIsPointer := strings.HasPrefix(dtoField.Type, "*")

	// Case 1: Source is pointer
	if srcIsPointer {
		if dstIsPointer {
			return []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Id("result").Op(":=").Id(functionName).Call(
						jen.Op("*").Id("src").Dot(sourceFieldName),
					),
					jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in nil", dtoField.Name)),
			}
		} else {
			return []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Id("d").Dot(dtoField.Name).Op("=").Id(functionName).Call(
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
				jen.Id("result").Op(":=").Id(functionName).Call(
					jen.Id("src").Dot(sourceFieldName),
				),
				jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
			),
		}
	}

	// Case 3: Both are values
	return []jen.Code{
		jen.Id("d").Dot(dtoField.Name).Op("=").Id(functionName).Call(
			jen.Id("src").Dot(sourceFieldName),
		),
	}
}

// buildErrorReturningConverterMappingDirect creates statements for error-returning converter used directly
func buildErrorReturningConverterMappingDirect(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	functionName string,
) []jen.Code {
	srcIsPointer := sourceField.IsPointer
	dstIsPointer := strings.HasPrefix(dtoField.Type, "*")

	var statements []jen.Code

	// Case 1: Source is pointer
	if srcIsPointer {
		if dstIsPointer {
			statements = []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Var().Id("result").Id(ExtractBaseType(dtoField.Type)),
					jen.Var().Id("err").Error(),
					jen.List(jen.Id("result"), jen.Id("err")).Op("=").Id(functionName).Call(
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
			statements = []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Var().Id("err").Error(),
					jen.List(jen.Id("d").Dot(dtoField.Name), jen.Id("err")).Op("=").Id(functionName).Call(
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
		statements = []jen.Code{
			jen.Block(
				jen.Var().Id("result").Id(ExtractBaseType(dtoField.Type)),
				jen.Var().Id("err").Error(),
				jen.List(jen.Id("result"), jen.Id("err")).Op("=").Id(functionName).Call(
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
		statements = []jen.Code{
			jen.Block(
				jen.Var().Id("err").Error(),
				jen.List(jen.Id("d").Dot(dtoField.Name), jen.Id("err")).Op("=").Id(functionName).Call(
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

// buildConverterMapping creates statements for converter - automatically detects safe vs error-returning
func buildConverterMapping(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	conv config.ConverterDef,
	isSafe bool,
) []jen.Code {
	// For safe converters, use the safe version
	if isSafe {
		return buildSafeConverterMapping(dtoField, sourceField, sourceFieldName, conv)
	}

	// Otherwise use error-returning version
	return buildErrorReturningConverterMapping(dtoField, sourceField, sourceFieldName, conv)
}

// buildErrorReturningConverterMapping creates statements for error-returning converter
func buildErrorReturningConverterMapping(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	conv config.ConverterDef,
) []jen.Code {
	srcIsPointer := sourceField.IsPointer
	dstIsPointer := strings.HasPrefix(dtoField.Type, "*")

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
