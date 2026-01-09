package generator

import (
	"fmt"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
	"github.com/dave/jennifer/jen"
)

// GenerateMapToMethod generates a MapTo method for bidirectional mapping
func GenerateMapToMethod(
	f *jen.File,
	dto types.DTOMapping,
	target types.SourceStruct,
	targetName, methodName string,
	cfg *config.Config,
	importMap map[string]string,
	functions map[string]types.FunctionInfo,
	converterMap map[string]types.ConverterInfo,
) {
	// Parse parameter type
	paramType := ParseTypeRefForJen(targetName, importMap)

	f.Comment(fmt.Sprintf("%s maps from %s to %s", methodName, dto.Name, targetName))

	methodBody, ignoredFields := buildMapToMethodBody(dto, target, cfg, functions, converterMap)

	// Add comment about ignored fields if any
	if len(ignoredFields) > 0 {
		f.Comment(fmt.Sprintf("Ignored fields: %s", strings.Join(ignoredFields, ", ")))
	}

	// Generate method
	f.Func().Params(
		jen.Id("d").Op("*").Id(dto.Name),
	).Id(methodName).Params(
		jen.Id("dst").Op("*").Add(paramType),
	).Error().Block(methodBody...)

	f.Line()
}

// buildMapToMethodBody constructs the MapTo method body
func buildMapToMethodBody(
	dto types.DTOMapping,
	target types.SourceStruct,
	cfg *config.Config,
	functions map[string]types.FunctionInfo,
	converterMap map[string]types.ConverterInfo,
) ([]jen.Code, []string) {
	statements := []jen.Code{
		jen.If(jen.Id("dst").Op("==").Nil()).Block(
			jen.Return(jen.Qual("errors", "New").Call(jen.Lit("destination is nil"))),
		),
		jen.Line(),
	}

	var ignoredFields []string

	// Generate field mappings
	for _, dtoField := range dto.Fields {
		if dtoField.Ignore {
			ignoredFields = append(ignoredFields, dtoField.Name)
			continue
		}

		// Determine target field name (reverse of field tag if present)
		targetFieldName := dtoField.Name
		if dtoField.FieldTag != "" {
			targetFieldName = dtoField.FieldTag
		}

		targetField, exists := target.Fields[targetFieldName]
		if !exists {
			statements = append(statements,
				jen.Comment(fmt.Sprintf("%s: target field not found, skipped", dtoField.Name)),
			)
			ignoredFields = append(ignoredFields, dtoField.Name)
			continue
		}

		// TODO: implement inversion for NestedDTO
		if dtoField.NestedDTO != "" {
			statements = append(statements,
				jen.Comment(fmt.Sprintf("%s: nested DTO mapping not implemented in MapTo, skipped", dtoField.Name)),
			)
			ignoredFields = append(ignoredFields, dtoField.Name)
			continue
		}

		// Handle converter fields
		if dtoField.ConverterTag != "" {
			// Check if there's an inverter for this converter
			if convInfo, hasConverter := converterMap[dtoField.ConverterTag]; hasConverter && convInfo.HasInverter {
				fn, fnExists := functions[convInfo.InverterFunc]
				isSafe := fnExists && IsSafeConverterSignature(fn)
				
				statements = append(statements, buildMapToConverterMapping(dtoField, targetField, targetFieldName, convInfo.InverterFunc, isSafe)...)
			} else {
				// No inverter - skip this field
				statements = append(statements,
					jen.Comment(fmt.Sprintf("%s: converter has no inverter, skipped", dtoField.Name)),
				)
				ignoredFields = append(ignoredFields, dtoField.Name)
			}
			continue
		}

		// Direct mapping only if types match
		dtoBaseType := ExtractBaseType(dtoField.Type)
		targetBaseType := targetField.BaseType

		if dtoBaseType != targetBaseType {
			statements = append(statements,
				jen.Comment(fmt.Sprintf("%s: type mismatch without converter, skipped", dtoField.Name)),
			)
			ignoredFields = append(ignoredFields, dtoField.Name)
			continue
		}

		statements = append(statements, buildMapToDirectMapping(dtoField, targetField, targetFieldName)...)
	}

	statements = append(statements, jen.Line(), jen.Return(jen.Nil()))
	return statements, ignoredFields
}

// buildMapToConverterMapping creates statements for MapTo with inverter
func buildMapToConverterMapping(
	dtoField types.FieldInfo,
	targetField types.FieldTypeInfo,
	targetFieldName string,
	inverterFunc string,
	isSafe bool,
) []jen.Code {
	if isSafe {
		return buildMapToSafeConverterMapping(dtoField, targetField, targetFieldName, inverterFunc)
	}
	return buildMapToErrorReturningConverterMapping(dtoField, targetField, targetFieldName, inverterFunc)
}

// buildMapToSafeConverterMapping creates statements for safe inverter (no error)
func buildMapToSafeConverterMapping(
	dtoField types.FieldInfo,
	targetField types.FieldTypeInfo,
	targetFieldName string,
	inverterFunc string,
) []jen.Code {
	srcIsPointer := strings.HasPrefix(dtoField.Type, "*")
	dstIsPointer := targetField.IsPointer

	// Case 1: Source is pointer
	if srcIsPointer {
		if dstIsPointer {
			// *T -> dereference -> inverter -> U -> take address -> *U
			return []jen.Code{
				jen.If(jen.Id("d").Dot(dtoField.Name).Op("!=").Nil()).Block(
					jen.Id("result").Op(":=").Id(inverterFunc).Call(
						jen.Op("*").Id("d").Dot(dtoField.Name),
					),
					jen.Id("dst").Dot(targetFieldName).Op("=").Op("&").Id("result"),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in nil", dtoField.Name)),
			}
		} else {
			// *T -> dereference -> inverter -> U
			return []jen.Code{
				jen.If(jen.Id("d").Dot(dtoField.Name).Op("!=").Nil()).Block(
					jen.Id("dst").Dot(targetFieldName).Op("=").Id(inverterFunc).Call(
						jen.Op("*").Id("d").Dot(dtoField.Name),
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
				jen.Id("result").Op(":=").Id(inverterFunc).Call(
					jen.Id("d").Dot(dtoField.Name),
				),
				jen.Id("dst").Dot(targetFieldName).Op("=").Op("&").Id("result"),
			),
		}
	}

	// Case 3: Both are values
	return []jen.Code{
		jen.Id("dst").Dot(targetFieldName).Op("=").Id(inverterFunc).Call(
			jen.Id("d").Dot(dtoField.Name),
		),
	}
}

// buildMapToErrorReturningConverterMapping creates statements for error-returning inverter
func buildMapToErrorReturningConverterMapping(
	dtoField types.FieldInfo,
	targetField types.FieldTypeInfo,
	targetFieldName string,
	inverterFunc string,
) []jen.Code {
	srcIsPointer := strings.HasPrefix(dtoField.Type, "*")
	dstIsPointer := targetField.IsPointer

	var statements []jen.Code

	// Case 1: Source is pointer
	if srcIsPointer {
		if dstIsPointer {
			// *T -> dereference -> inverter -> U -> take address -> *U
			statements = []jen.Code{
				jen.If(jen.Id("d").Dot(dtoField.Name).Op("!=").Nil()).Block(
					jen.List(jen.Id("result"), jen.Id("err")).Op(":=").Id(inverterFunc).Call(
						jen.Op("*").Id("d").Dot(dtoField.Name),
					),
					jen.If(jen.Id("err").Op("!=").Nil()).Block(
						jen.Return(jen.Qual("fmt", "Errorf").Call(
							jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
							jen.Id("err"),
						)),
					),
					jen.Id("dst").Dot(targetFieldName).Op("=").Op("&").Id("result"),
				),
				jen.Comment(fmt.Sprintf("// %s: nil pointer will result in nil", dtoField.Name)),
			}
		} else {
			// *T -> dereference -> inverter -> U
			statements = []jen.Code{
				jen.If(jen.Id("d").Dot(dtoField.Name).Op("!=").Nil()).Block(
					jen.Var().Id("err").Error(),
					jen.List(jen.Id("dst").Dot(targetFieldName), jen.Id("err")).Op("=").Id(inverterFunc).Call(
						jen.Op("*").Id("d").Dot(dtoField.Name),
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
				jen.Var().Id("result").Id(targetField.BaseType),
				jen.Var().Id("err").Error(),
				jen.List(jen.Id("result"), jen.Id("err")).Op("=").Id(inverterFunc).Call(
					jen.Id("d").Dot(dtoField.Name),
				),
				jen.If(jen.Id("err").Op("!=").Nil()).Block(
					jen.Return(jen.Qual("fmt", "Errorf").Call(
						jen.Lit(fmt.Sprintf("converting field %s: %%w", dtoField.Name)),
						jen.Id("err"),
					)),
				),
				jen.Id("dst").Dot(targetFieldName).Op("=").Op("&").Id("result"),
			),
		}
	} else {
		// Case 3: Both are values
		statements = []jen.Code{
			jen.Block(
				jen.Var().Id("err").Error(),
				jen.List(jen.Id("dst").Dot(targetFieldName), jen.Id("err")).Op("=").Id(inverterFunc).Call(
					jen.Id("d").Dot(dtoField.Name),
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

// buildMapToDirectMapping creates statements for direct field mapping in MapTo
func buildMapToDirectMapping(
	dtoField types.FieldInfo,
	targetField types.FieldTypeInfo,
	targetFieldName string,
) []jen.Code {
	srcIsPointer := strings.HasPrefix(dtoField.Type, "*")
	dstIsPointer := targetField.IsPointer

	// Case 1: Both are pointers or both are values - direct assignment
	if srcIsPointer == dstIsPointer {
		return []jen.Code{
			jen.Id("dst").Dot(targetFieldName).Op("=").Id("d").Dot(dtoField.Name),
		}
	}

	// Case 2: Source is pointer, destination is value
	if srcIsPointer && !dstIsPointer {
		return []jen.Code{
			jen.If(jen.Id("d").Dot(dtoField.Name).Op("!=").Nil()).Block(
				jen.Id("dst").Dot(targetFieldName).Op("=").Op("*").Id("d").Dot(dtoField.Name),
			),
			jen.Comment(fmt.Sprintf("// %s: nil pointer will result in zero value", dtoField.Name)),
		}
	}

	// Case 3: Source is value, destination is pointer
	if !srcIsPointer && dstIsPointer {
		return []jen.Code{
			jen.Block(
				jen.Id("v").Op(":=").Id("d").Dot(dtoField.Name),
				jen.Id("dst").Dot(targetFieldName).Op("=").Op("&").Id("v"),
			),
		}
	}

	// Fallback
	return []jen.Code{
		jen.Id("dst").Dot(targetFieldName).Op("=").Id("d").Dot(dtoField.Name),
	}
}

// IsSafeConverterSignature checks if a function matches safe converter signature: func(T) U
func IsSafeConverterSignature(fn types.FunctionInfo) bool {
	return len(fn.ParamTypes) == 1 && len(fn.ReturnTypes) == 1
}
