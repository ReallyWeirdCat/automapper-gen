package validator

import (
	"fmt"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/logger"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/types"
)

type Severity string
const (
    SeverityError   = "error"
    SeverityWarning = "warning"
)

// ValidationError represents a validation error
type ValidationError struct {
	DTO        string
	Source     string
	Field      string
	Message    string
	Severity   Severity // "error" or "warning"
	Fixable    bool
	Suggestion string
}

func (e ValidationError) Error() string {
	severityPrefix := "[ERROR]"
	if e.Severity == SeverityWarning {
		severityPrefix = "[WARN] "
	}

	msg := fmt.Sprintf("%s %s.%s -> %s.%s: %s",
		severityPrefix, e.Source, e.Field, e.DTO, e.Field, e.Message)

	if e.Suggestion != "" {
		msg += fmt.Sprintf("\n         Suggestion: %s", e.Suggestion)
	}

	return msg
}

// ValidationResult holds the results of validation
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
	Stats    map[string]int
}

// IsValid returns true if there are no errors
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// Validator validates DTO mappings before code generation
type Validator struct {
	cfg     *config.Config
	sources map[string]types.SourceStruct
	dtos    map[string]types.DTOMapping
	visited map[string]bool // for circular dependency detection
}

// NewValidator creates a new validator
func NewValidator(
	cfg *config.Config, dtos []types.DTOMapping, sources map[string]types.SourceStruct,
) *Validator {
	dtoMap := make(map[string]types.DTOMapping)
	for _, dto := range dtos {
		dtoMap[dto.Name] = dto
	}

	return &Validator{
		cfg:     cfg,
		sources: sources,
		dtos:    dtoMap,
		visited: make(map[string]bool),
	}
}

// Validate performs validation
func (v *Validator) Validate() *ValidationResult {
	logger.Section("Validation")

	result := &ValidationResult{
		Errors:   []ValidationError{},
		Warnings: []ValidationError{},
		Stats:    make(map[string]int),
	}

	result.Stats["total_dtos"] = len(v.dtos)
	result.Stats["total_sources"] = len(v.sources)

	totalFields := 0
	for _, dto := range v.dtos {
		totalFields += len(dto.Fields)
		logger.Verbose("Validating DTO: %s (sources: %v)", dto.Name, dto.Sources)

		for _, sourceName := range dto.Sources {
			v.validateDTOMapping(dto, sourceName, result)
		}
	}

	result.Stats["total_fields"] = totalFields
	result.Stats["errors"] = len(result.Errors)
	result.Stats["warnings"] = len(result.Warnings)

	// Print summary with color-coded severity
	if len(result.Warnings) > 0 {
		logger.Warning("Found %d warnings", len(result.Warnings))
		for _, w := range result.Warnings {
			logger.Warning("%s", w.Error())
		}
	}

	if len(result.Errors) > 0 {
		logger.Error("Found %d errors that will prevent code generation", len(result.Errors))
		for _, e := range result.Errors {
			logger.Error("%s", e.Error())
		}
	} else {
		logger.Success("Validation passed")
	}

	logger.Stats("Validation Statistics", map[string]any{
		"DTOs validated":   result.Stats["total_dtos"],
		"Source structs":   result.Stats["total_sources"],
		"Fields validated": result.Stats["total_fields"],
		"Errors":           result.Stats["errors"],
		"Warnings":         result.Stats["warnings"],
	})

	return result
}

// validateDTOMapping validates a single DTO to source mapping
func (v *Validator) validateDTOMapping(
	dto types.DTOMapping, sourceName string, result *ValidationResult,
) {
	source, exists := v.sources[sourceName]
	if !exists {
		result.Errors = append(result.Errors, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Message:    "Source struct not found",
			Severity:   SeverityWarning,
			Suggestion: fmt.Sprintf("Ensure %s is defined in the package or included in external packages", sourceName),
		})
		return
	}

	logger.Debug("Validating %s <- %s (%d fields)", dto.Name, sourceName, len(dto.Fields))

	for _, field := range dto.Fields {
		if field.Ignore {
			logger.Debug("  Skipping ignored field: %s", field.Name)
			continue
		}

		v.validateField(dto, source, sourceName, field, result)
	}
}

// validateField validates a single field mapping
func (v *Validator) validateField(
	dto types.DTOMapping,
	source types.SourceStruct,
	sourceName string,
	field types.FieldInfo,
	result *ValidationResult,
) {
	sourceFieldName := v.resolveSourceFieldName(field, source)
	sourceField, exists := source.Fields[sourceFieldName]

	if !exists {
		// Check if it's intentionally unmapped
		if field.FieldTag != "" || field.ConverterTag != "" || field.NestedDTO != "" {
			result.Errors = append(result.Errors, ValidationError{
				DTO:        dto.Name,
				Source:     sourceName,
				Field:      field.Name,
				Message:    fmt.Sprintf("Source field '%s' not found", sourceFieldName),
				Severity:   SeverityError,
				Suggestion: "Check if field name is correct or remove mapping configuration",
			})
		} else {
			result.Warnings = append(result.Warnings, ValidationError{
				DTO:        dto.Name,
				Source:     sourceName,
				Field:      field.Name,
				Message:    fmt.Sprintf("Source field '%s' not found, will use zero value", sourceFieldName),
				Severity:   SeverityWarning,
				Fixable:    true,
				Suggestion: "Add 'automapper:\"-\"' tag to explicitly ignore, or add source field",
			})
		}
		return
	}

	logger.Debug("  Field %s: %s <- %s: %s", field.Name, field.Type, sourceFieldName, sourceField.Type)

	// Validate nested DTO mapping
	if field.NestedDTO != "" {
		v.validateNestedDTO(dto, sourceName, field, sourceField, result)
		return
	}

	// Validate converter mapping
	if field.ConverterTag != "" {
		v.validateConverter(dto, sourceName, field, sourceField, result)
		return
	}

	// Validate direct mapping
	v.validateDirectMapping(dto, sourceName, field, sourceField, result)
}

// validateNestedDTO validates nested DTO mappings
func (v *Validator) validateNestedDTO(
	dto types.DTOMapping,
	sourceName string,
	field types.FieldInfo,
	sourceField types.FieldTypeInfo,
	result *ValidationResult,
) {
	nestedDTOName := field.NestedDTO

	// Check if nested DTO exists
	if _, exists := v.dtos[nestedDTOName]; !exists {
		result.Errors = append(result.Errors, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Nested DTO '%s' not found", nestedDTOName),
			Severity:   SeverityError,
			Suggestion: fmt.Sprintf("Ensure %s is defined with automapper:from annotation", nestedDTOName),
		})
		return
	}

	// Check for circular dependencies
	if v.detectCircularDependency(dto.Name, nestedDTOName) {
		result.Errors = append(result.Errors, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Circular dependency detected with %s", nestedDTOName),
			Severity:   SeverityError,
			Suggestion: "Remove circular references or use a converter instead",
		})
		return
	}

	// Validate slice compatibility
	dtoIsSlice := strings.HasPrefix(field.Type, "[]")
	srcIsSlice := sourceField.IsSlice

	if dtoIsSlice != srcIsSlice {
		result.Errors = append(result.Errors, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Incompatible slice/non-slice types: %s vs %s", field.Type, sourceField.Type),
			Severity:   SeverityError,
			Suggestion: "Both source and destination must be slices or both must be single values",
		})
	}

	logger.Debug("    OK: Nested DTO mapping valid: %s", nestedDTOName)
}

// validateConverter validates converter-based mappings
func (v *Validator) validateConverter(
	dto types.DTOMapping,
	sourceName string,
	field types.FieldInfo,
	sourceField types.FieldTypeInfo,
	result *ValidationResult,
) {
	converterName := field.ConverterTag

	// Check if converter is registered in default converters
	found := false
	for _, conv := range v.cfg.DefaultConverters {
		if conv.Name == converterName {
			found = true
			logger.Debug("    OK: Using registered converter: %s", converterName)
			break
		}
	}

	if !found {
		result.Warnings = append(result.Warnings, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Converter '%s' not found in defaultConverters", converterName),
			Severity:   SeverityWarning,
			Fixable:    true,
			Suggestion: "Register converter in automapper.json or ensure it's registered at runtime",
		})
	}

	// Validate that types are compatible for conversion
	// Remove pointer/slice prefixes for base type comparison
	srcBaseType := extractBaseType(sourceField.Type)
	dstBaseType := extractBaseType(field.Type)

	// We can't validate custom converters at generation time, but we can warn about obvious issues
	if srcBaseType == dstBaseType {
		result.Warnings = append(result.Warnings, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Converter specified but types are identical: %s", srcBaseType),
			Severity:   SeverityWarning,
			Fixable:    true,
			Suggestion: "Remove converter tag for direct assignment or verify this is intentional",
		})
	}
}

// validateDirectMapping validates direct field-to-field mappings
func (v *Validator) validateDirectMapping(
	dto types.DTOMapping,
	sourceName string,
	field types.FieldInfo,
	sourceField types.FieldTypeInfo,
	result *ValidationResult,
) {
	// Extract base types
	dtoBaseType := extractBaseType(field.Type)
	srcBaseType := sourceField.BaseType

	// Check if types are compatible
	if !v.areTypesCompatible(dtoBaseType, srcBaseType) {
		result.Errors = append(result.Errors, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Type mismatch: %s <- %s (cannot convert without converter)", field.Type, sourceField.Type),
			Severity:   SeverityError,
			Fixable:    true,
			Suggestion: "Add converter tag: `automapper:\"converter=YourConverter\"`",
		})
		return
	}

	// Warn about pointer conversions (they're valid but might cause unexpected behavior)
	dtoIsPointer := strings.HasPrefix(field.Type, "*")
	srcIsPointer := sourceField.IsPointer

	if dtoIsPointer != srcIsPointer {
		result.Warnings = append(result.Warnings, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Pointer conversion: %s <- %s", field.Type, sourceField.Type),
			Severity:   SeverityWarning,
			Suggestion: "Verify this pointer conversion is intentional",
		})
	}

	logger.Debug("    OK: Direct mapping valid")
}

// resolveSourceFieldName determines the source field name
func (v *Validator) resolveSourceFieldName(
	field types.FieldInfo, source types.SourceStruct,
) string {
	if field.FieldTag != "" {
		return field.FieldTag
	}

	if v.cfg.FieldNameTransform == "snake_to_camel" {
		for srcFieldName := range source.Fields {
			if snakeToCamel(srcFieldName) == field.Name {
				return srcFieldName
			}
		}
	}

	return field.Name
}

// areTypesCompatible checks if two types can be directly assigned
func (v *Validator) areTypesCompatible(type1, type2 string) bool {
	// Remove pointer and slice prefixes for comparison
	base1 := extractBaseType(type1)
	base2 := extractBaseType(type2)

	// Exact match
	if base1 == base2 {
		return true
	}

	// Check for package-qualified types that might be the same
	// e.g., "time.Time" vs "Time" when both refer to time.Time
	if strings.Contains(base1, ".") && !strings.Contains(base2, ".") {
		parts := strings.Split(base1, ".")
		if parts[len(parts)-1] == base2 {
			return true // Might be compatible, let compiler decide
		}
	}

	if strings.Contains(base2, ".") && !strings.Contains(base1, ".") {
		parts := strings.Split(base2, ".")
		if parts[len(parts)-1] == base1 {
			return true // Might be compatible, let compiler decide
		}
	}

	return false
}

// detectCircularDependency checks for circular DTO dependencies
// A circular dependency exists if: starting from nestedDTO, we can reach back to currentDTO
func (v *Validator) detectCircularDependency(currentDTO, nestedDTO string) bool {
	visited := make(map[string]bool)
	return v.canReach(nestedDTO, currentDTO, visited)
}

// canReach checks if we can reach 'to' starting from 'from' by following nested DTO references
func (v *Validator) canReach(from, to string, visited map[string]bool) bool {
	// If we've reached the target, there's a circular dependency
	if from == to {
		return true
	}

	// Avoid infinite loops
	if visited[from] {
		return false
	}

	visited[from] = true

	// Check all nested DTOs in 'from' to see if any can reach 'to'
	if dto, exists := v.dtos[from]; exists {
		for _, field := range dto.Fields {
			if field.NestedDTO != "" {
				if v.canReach(field.NestedDTO, to, visited) {
					return true
				}
			}
		}
	}

	return false
}

// extractBaseType removes pointer and slice prefixes
func extractBaseType(typeStr string) string {
	typeStr = strings.TrimPrefix(typeStr, "*")
	typeStr = strings.TrimPrefix(typeStr, "[]")
	return typeStr
}

// snakeToCamel converts snake_case to CamelCase
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}
