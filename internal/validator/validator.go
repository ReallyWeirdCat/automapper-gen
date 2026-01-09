package validator

import (
	"fmt"
	"strings"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/logger"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/parser"
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
	Severity   Severity
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
	cfg       *config.Config
	sources   map[string]types.SourceStruct
	dtos      map[string]types.DTOMapping
	functions map[string]types.FunctionInfo
	visited   map[string]bool
}

// NewValidator creates a new validator
func NewValidator(
	cfg *config.Config,
	dtos []types.DTOMapping,
	sources map[string]types.SourceStruct,
	functions map[string]types.FunctionInfo,
) *Validator {
	dtoMap := make(map[string]types.DTOMapping)
	for _, dto := range dtos {
		dtoMap[dto.Name] = dto
	}

	return &Validator{
		cfg:       cfg,
		sources:   sources,
		dtos:      dtoMap,
		functions: functions,
		visited:   make(map[string]bool),
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

	// Validate converter functions exist and have valid signatures
	v.validateConverterFunctions(result)

	// Validate inverter relationships
	v.validateInverterRelationships(result)

	totalFields := 0
	totalBidirectional := 0
	
	for _, dto := range v.dtos {
		totalFields += len(dto.Fields)
		if dto.Bidirectional {
			totalBidirectional++
		}
		
		logger.Verbose("Validating DTO: %s (sources: %v, bidirectional: %v)", dto.Name, dto.Sources, dto.Bidirectional)

		for _, sourceName := range dto.Sources {
			v.validateDTOMapping(dto, sourceName, result)
			
			// Additional validation for bidirectional mappings
			if dto.Bidirectional {
				v.validateBidirectionalMapping(dto, sourceName, result)
			}
		}
	}

	result.Stats["total_fields"] = totalFields
	result.Stats["bidirectional_dtos"] = totalBidirectional
	result.Stats["errors"] = len(result.Errors)
	result.Stats["warnings"] = len(result.Warnings)

	// Print summary
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
		"DTOs validated":      result.Stats["total_dtos"],
		"Bidirectional DTOs":  result.Stats["bidirectional_dtos"],
		"Source structs":      result.Stats["total_sources"],
		"Fields validated":    result.Stats["total_fields"],
		"Errors":              result.Stats["errors"],
		"Warnings":            result.Stats["warnings"],
	})

	return result
}

// validateConverterFunctions validates that all converter functions exist and have valid signatures
func (v *Validator) validateConverterFunctions(result *ValidationResult) {
	logger.Verbose("Validating converter functions...")

	converterCount := 0
	inverterCount := 0

	for _, fn := range v.functions {
		if fn.IsConverter && !fn.IsInverter {
			converterCount++
			isSafe := parser.IsSafeConverterSignature(fn)
			isErrorReturning := parser.IsErrorReturningConverterSignature(fn)

			if isSafe {
				logger.Debug("  Safe converter '%s' - func(T) U", fn.Name)
			} else if isErrorReturning {
				logger.Debug("  Regular converter '%s' - func(T) (U, error)", fn.Name)
			} else {
				// Invalid signature
				result.Errors = append(result.Errors, ValidationError{
					Message: fmt.Sprintf("Converter function '%s' has invalid signature, got: %d params, %d returns",
						fn.Name, len(fn.ParamTypes), len(fn.ReturnTypes)),
					Severity:   SeverityError,
					Suggestion: "Change signature to either func(T) U (for safe converters) or func(T) (U, error)",
				})
			}
		}

		if fn.IsInverter {
			inverterCount++
		}
	}

	logger.Verbose("Converter functions validated: %d converters, %d inverters", converterCount, inverterCount)
}

// validateInverterRelationships validates that inverters reference valid converters
func (v *Validator) validateInverterRelationships(result *ValidationResult) {
	logger.Verbose("Validating inverter relationships...")

	for _, fn := range v.functions {
		if fn.IsInverter && fn.InvertsFunc != "" {
			// Check if the referenced converter exists
			converterFn, exists := v.functions[fn.InvertsFunc]
			if !exists {
				result.Errors = append(result.Errors, ValidationError{
					Message:    fmt.Sprintf("Inverter '%s' references non-existent converter '%s'", fn.Name, fn.InvertsFunc),
					Severity:   SeverityError,
					Suggestion: fmt.Sprintf("Ensure converter function '%s' exists and is annotated with //automapper:converter", fn.InvertsFunc),
				})
				continue
			}

			// Check if the referenced function is actually a converter
			if !converterFn.IsConverter {
				result.Errors = append(result.Errors, ValidationError{
					Message:    fmt.Sprintf("Inverter '%s' references '%s' which is not marked as a converter", fn.Name, fn.InvertsFunc),
					Severity:   SeverityError,
					Suggestion: fmt.Sprintf("Add //automapper:converter annotation to function '%s'", fn.InvertsFunc),
				})
			}

			// Validate inverter signature
			isSafe := parser.IsSafeConverterSignature(fn)
			isErrorReturning := parser.IsErrorReturningConverterSignature(fn)

			if !isSafe && !isErrorReturning {
				result.Errors = append(result.Errors, ValidationError{
					Message: fmt.Sprintf("Inverter function '%s' has invalid signature, got: %d params, %d returns",
						fn.Name, len(fn.ParamTypes), len(fn.ReturnTypes)),
					Severity:   SeverityError,
					Suggestion: "Change signature to either func(T) U or func(T) (U, error)",
				})
			}

			logger.Debug("  Inverter '%s' -> Converter '%s'", fn.Name, fn.InvertsFunc)
		}
	}
}

// validateBidirectionalMapping performs additional validation for bidirectional DTOs
func (v *Validator) validateBidirectionalMapping(dto types.DTOMapping, sourceName string, result *ValidationResult) {
	_, exists := v.sources[sourceName]
	if !exists {
		return // Already validated in validateDTOMapping
	}

	logger.Debug("Validating bidirectional mapping: %s <-> %s", dto.Name, sourceName)

	for _, field := range dto.Fields {
		if field.Ignore {
			continue
		}

		// Check for nested DTOs - not supported in MapTo yet
		if field.NestedDTO != "" {
			result.Warnings = append(result.Warnings, ValidationError{
				DTO:      dto.Name,
				Source:   sourceName,
				Field:    field.Name,
				Message:  "Nested DTO fields are not supported in MapTo and will be skipped",
				Severity: SeverityWarning,
				Suggestion: "Consider flattening the structure or waiting for nested DTO support in MapTo",
			})
			continue
		}

		// Check for converters without inverters
		if field.ConverterTag != "" {
			converterFn, fnExists := v.functions[field.ConverterTag]
			if !fnExists {
				// Already reported in validateConverterFunctions
				continue
			}

			// Check if this converter has an inverter
			hasInverter := false
			for _, fn := range v.functions {
				if fn.IsInverter && fn.InvertsFunc == field.ConverterTag {
					hasInverter = true
					break
				}
			}

			if !hasInverter {
				result.Warnings = append(result.Warnings, ValidationError{
					DTO:      dto.Name,
					Source:   sourceName,
					Field:    field.Name,
					Message:  fmt.Sprintf("Converter '%s' has no inverter, field will be skipped in MapTo", field.ConverterTag),
					Severity: SeverityWarning,
					Fixable:  true,
					Suggestion: fmt.Sprintf("Add an inverter function with //automapper:inverter=%s annotation", field.ConverterTag),
				})
			} else {
				logger.Debug("  Field %s: converter '%s' has inverter (bidirectional OK)", field.Name, field.ConverterTag)
			}

			// Additional check: ensure converter is marked as such
			if !converterFn.IsConverter {
				result.Errors = append(result.Errors, ValidationError{
					DTO:        dto.Name,
					Source:     sourceName,
					Field:      field.Name,
					Message:    fmt.Sprintf("Function '%s' used as converter but not marked with //automapper:converter", field.ConverterTag),
					Severity:   SeverityError,
					Suggestion: fmt.Sprintf("Add //automapper:converter annotation to function '%s'", field.ConverterTag),
				})
			}
		}
	}
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
			Severity:   SeverityError,
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
	sourceFieldName := v.resolveSourceFieldName(field)
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

	// Check if converter function exists
	fn, fnExists := v.functions[converterName]
	if !fnExists {
		result.Errors = append(result.Errors, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Converter function '%s' not found", converterName),
			Severity:   SeverityError,
			Suggestion: fmt.Sprintf("Add function '%s' with //automapper:converter annotation", converterName),
		})
		return
	}

	// Ensure function is marked as converter
	if !fn.IsConverter {
		result.Errors = append(result.Errors, ValidationError{
			DTO:        dto.Name,
			Source:     sourceName,
			Field:      field.Name,
			Message:    fmt.Sprintf("Function '%s' used as converter but not marked with //automapper:converter", converterName),
			Severity:   SeverityError,
			Suggestion: fmt.Sprintf("Add //automapper:converter annotation to function '%s'", converterName),
		})
		return
	}

	logger.Debug("    OK: Using converter function: %s", converterName)

	// Validate that types are compatible for conversion
	srcBaseType := extractBaseType(sourceField.Type)
	dstBaseType := extractBaseType(field.Type)

	// Warn if types are identical (converter might be unnecessary)
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

	// Warn about pointer conversions
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
func (v *Validator) resolveSourceFieldName(field types.FieldInfo) string {
	if field.FieldTag != "" {
		return field.FieldTag
	}

	return field.Name
}

// areTypesCompatible checks if two types can be directly assigned
func (v *Validator) areTypesCompatible(type1, type2 string) bool {
	base1 := extractBaseType(type1)
	base2 := extractBaseType(type2)

	if base1 == base2 {
		return true
	}

	// Check for package-qualified types
	if strings.Contains(base1, ".") && !strings.Contains(base2, ".") {
		parts := strings.Split(base1, ".")
		if parts[len(parts)-1] == base2 {
			return true
		}
	}

	if strings.Contains(base2, ".") && !strings.Contains(base1, ".") {
		parts := strings.Split(base2, ".")
		if parts[len(parts)-1] == base1 {
			return true
		}
	}

	return false
}

// detectCircularDependency checks for circular DTO dependencies
func (v *Validator) detectCircularDependency(currentDTO, nestedDTO string) bool {
	visited := make(map[string]bool)
	return v.canReach(nestedDTO, currentDTO, visited)
}

// canReach checks if we can reach 'to' starting from 'from' by following nested DTO references
func (v *Validator) canReach(from, to string, visited map[string]bool) bool {
	if from == to {
		return true
	}

	if visited[from] {
		return false
	}

	visited[from] = true

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
