package generator

import (
	"fmt"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/parser"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
	"github.com/dave/jennifer/jen"
)

// GenerateMapFromMethod generates a MapFrom method for a DTO
func GenerateMapFromMethod(f *jen.File, dto types.DTOMapping, source types.SourceStruct, sourceName, methodName string, cfg *config.Config, importMap map[string]string) {
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
func buildMethodBody(dto types.DTOMapping, source types.SourceStruct, cfg *config.Config, importMap map[string]string) []jen.Code {
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

		if dtoField.ConverterTag != "" {
			statements = append(statements, buildConverterMapping(dtoField, sourceField, sourceFieldName, importMap)...)
		} else {
			statements = append(statements, buildDirectMapping(dtoField, sourceFieldName))
		}
	}

	statements = append(statements, jen.Line(), jen.Return(jen.Nil()))
	return statements
}

// resolveSourceFieldName determines the source field name for a DTO field
func resolveSourceFieldName(dtoField types.FieldInfo, source types.SourceStruct, cfg *config.Config) string {
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

// buildConverterMapping creates statements for converter-based field mapping
func buildConverterMapping(dtoField types.FieldInfo, sourceField types.FieldTypeInfo, sourceFieldName string, importMap map[string]string) []jen.Code {
	fromType := ParseTypeForJen(sourceField.BaseType, importMap)
	toType := ParseTypeForJen(ExtractBaseType(dtoField.Type), importMap)

	return []jen.Code{
		jen.Block(
			jen.Var().Err().Error(),
			jen.List(jen.Id("d").Dot(dtoField.Name), jen.Err()).Op("=").Id("Convert").Types(
				fromType, toType,
			).Call(
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

// buildDirectMapping creates a statement for direct field assignment
func buildDirectMapping(dtoField types.FieldInfo, sourceFieldName string) jen.Code {
	return jen.Id("d").Dot(dtoField.Name).Op("=").Id("src").Dot(sourceFieldName)
}
