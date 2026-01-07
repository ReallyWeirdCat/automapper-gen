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

	f.Comment(fmt.Sprintf("%s maps from %s to %s", methodName, sourceName, dto.Name))

	// Build method body
	methodBody := buildMethodBody(dto, source, cfg, importMap)

	// Generate method
	f.Func().Params(
		jen.Id("d").Op("*").Id(dto.Name),
	).Id(methodName).Params(
		jen.Id("src").Op("*").Add(paramType),
	).Error().Block(methodBody...)

	f.Line()
}

// buildMethodBody constructs the method body statements
func buildMethodBody(
	dto types.DTOMapping,
	source types.SourceStruct,
	cfg *config.Config,
	importMap map[string]string,
) []jen.Code {
	statements := []jen.Code{
		jen.If(jen.Id("src").Op("==").Nil()).Block(
			jen.Return(jen.Qual("errors", "New").Call(jen.Lit("source is nil"))),
		),
		jen.Line(),
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
			statements = append(statements, buildConverterMapping(dtoField, sourceField, sourceFieldName, importMap)...)
		} else {
			// Check for pointer mismatch and handle accordingly
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

// buildNestedDTOMapping creates statements for nested DTO mapping with pointer and slice handling
func buildNestedDTOMapping(
	dtoField types.FieldInfo, sourceField types.FieldTypeInfo, sourceFieldName string,
) []jen.Code {
	dtoTypeName := dtoField.NestedDTO
	sourceTypeName := sourceField.BaseType

	// Determine the MapFrom method name based on source type
	methodName := "MapFrom" + ExtractTypeNameWithoutPackage(sourceTypeName)

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
				jen.If(
					jen.Err().Op(":=").Id("nested").Dot(methodName).Call(jen.Id("src").Dot(sourceFieldName)),
					jen.Err().Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
						jen.Err(),
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
				jen.If(
					jen.Err().Op(":=").Id("nested").Dot(methodName).Call(jen.Id("src").Dot(sourceFieldName)),
					jen.Err().Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
						jen.Err(),
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
				jen.If(
					jen.Err().Op(":=").Id("nested").Dot(methodName).Call(jen.Op("&").Id("src").Dot(sourceFieldName)),
					jen.Err().Op("!=").Nil(),
				).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
						jen.Err(),
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
			jen.If(
				jen.Err().Op(":=").Id("nested").Dot(methodName).Call(jen.Op("&").Id("src").Dot(sourceFieldName)),
				jen.Err().Op("!=").Nil(),
			).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("mapping nested field %s: %%w", dtoField.Name)),
					jen.Err(),
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

	// Clean DTO type name if needed (remove pointer prefix if present in the DTO field type)
	cleanDtoTypeName := strings.TrimPrefix(dtoTypeName, "*")

	// Case 1: []T -> []DTO
	if !srcElemIsPointer && !dtoElemIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("d").Dot(dtoField.Name).Op("=").Make(jen.Index().Id(cleanDtoTypeName), jen.Len(jen.Id("src").Dot(sourceFieldName))),
				jen.For(jen.List(jen.Id("i"), jen.Id("item")).Op(":=").Range().Id("src").Dot(sourceFieldName)).Block(
					jen.If(
						jen.Err().Op(":=").Id("d").Dot(dtoField.Name).Index(jen.Id("i")).Dot(methodName).Call(jen.Op("&").Id("item")),
						jen.Err().Op("!=").Nil(),
					).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
							jen.Id("i"),
							jen.Err(),
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
						jen.If(
							jen.Err().Op(":=").Id("nested").Dot(methodName).Call(jen.Id("item")),
							jen.Err().Op("!=").Nil(),
						).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
								jen.Id("i"),
								jen.Err(),
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
					jen.If(
						jen.Err().Op(":=").Id("nested").Dot(methodName).Call(jen.Op("&").Id("item")),
						jen.Err().Op("!=").Nil(),
					).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
							jen.Id("i"),
							jen.Err(),
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
						jen.If(
							jen.Err().Op(":=").Id("nested").Dot(methodName).Call(jen.Id("item")),
							jen.Err().Op("!=").Nil(),
						).Block(
							jen.Return(jen.Qual("fmt", "Errorf").Call(
								jen.Lit(fmt.Sprintf("mapping nested field %s[%%d]: %%w", dtoField.Name)),
								jen.Id("i"),
								jen.Err(),
							)),
						),
						jen.Id("d").Dot(dtoField.Name).Op("=").Append(jen.Id("d").Dot(dtoField.Name), jen.Id("nested")),
					),
				),
			),
		}
	}

	// Fallback (shouldn't reach here)
	return []jen.Code{
		jen.Comment(fmt.Sprintf("// %s: unsupported slice mapping", dtoField.Name)),
	}
}

// buildConverterMapping creates statements for converter-based field mapping with pointer handling
func buildConverterMapping(
	dtoField types.FieldInfo,
	sourceField types.FieldTypeInfo,
	sourceFieldName string,
	importMap map[string]string,
) []jen.Code {
	fromTypeStr := sourceField.Type
	toTypeStr := dtoField.Type

	// Check pointer semantics
	srcIsPointer := sourceField.IsPointer
	dstIsPointer := strings.HasPrefix(dtoField.Type, "*")

	fromConvType := removePointerPrefix(fromTypeStr)
	toConvType := removePointerPrefix(toTypeStr)

	fromType := ParseTypeForJen(fromConvType, importMap)
	toType := ParseTypeForJen(toConvType, importMap)

	// Case 1: Source is pointer, needs dereferencing before conversion
	if srcIsPointer {
		if dstIsPointer {
			// *T -> dereference -> converter -> T -> take address -> *T
			return []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Var().Err().Error(),
					jen.Var().Id("result").Add(toType),
					jen.List(jen.Id("result"), jen.Err()).Op("=").Id("Convert").Types(fromType, toType).Call(
						jen.Lit(dtoField.ConverterTag),
						jen.Op("*").Id("src").Dot(sourceFieldName),
					),
					jen.If(jen.Err().Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
							jen.Err(),
						)),
					),
					jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in nil", dtoField.Name)),
			}
		} else {
			// *T -> dereference -> converter -> T
			return []jen.Code{
				jen.If(jen.Id("src").Dot(sourceFieldName).Op("!=").Nil()).Block(
					jen.Var().Err().Error(),
					jen.List(jen.Id("d").Dot(dtoField.Name), jen.Err()).Op("=").Id("Convert").Types(fromType, toType).Call(
						jen.Lit(dtoField.ConverterTag),
						jen.Op("*").Id("src").Dot(sourceFieldName),
					),
					jen.If(jen.Err().Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
							jen.Err(),
						)),
					),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in zero value", dtoField.Name)),
			}
		}
	}

	// Case 2: Source is value, destination is pointer
	if dstIsPointer {
		// T -> converter -> T -> take address -> *T
		return []jen.Code{
			jen.Block(
				jen.Var().Err().Error(),
				jen.Var().Id("result").Add(toType),
				jen.List(jen.Id("result"), jen.Err()).Op("=").Id("Convert").Types(fromType, toType).Call(
					jen.Lit(dtoField.ConverterTag),
					jen.Id("src").Dot(sourceFieldName),
				),
				jen.If(jen.Err().Op("!=").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
						jen.Err(),
					)),
				),
				jen.Id("d").Dot(dtoField.Name).Op("=").Op("&").Id("result"),
			),
		}
	}

	// Case 3: Both are values - direct converter call
	return []jen.Code{
		jen.Block(
			jen.Var().Err().Error(),
			jen.List(jen.Id("d").Dot(dtoField.Name), jen.Err()).Op("=").Id("Convert").Types(fromType, toType).Call(
				jen.Lit(dtoField.ConverterTag),
				jen.Id("src").Dot(sourceFieldName),
			),
			jen.If(jen.Err().Op("!=").Nil()).Block(
				jen.Return(jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
					jen.Err(),
				)),
			),
		),
	}
}

// removePointerPrefix removes only pointer prefix, keeping slice/array prefixes
func removePointerPrefix(typeStr string) string {
	if strings.HasPrefix(typeStr, "*") {
		return typeStr[1:]
	}
	return typeStr
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

	// If base types don't match, this needs a converter (but that's handled elsewhere)
	// Here we only handle pointer conversions for matching base types
	if dtoBaseType != srcBaseType {
		// Direct assignment if types match exactly
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

	// Fallback (shouldn't reach here)
	return []jen.Code{
		jen.Id("d").Dot(dtoField.Name).Op("=").Id("src").Dot(sourceFieldName),
	}
}
