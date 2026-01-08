package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/generator"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/logger"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/parser"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/validator"
)

var (
	verbose      = flag.Bool("verbose", false, "Enable verbose logging")
	debug        = flag.Bool("debug", false, "Enable debug logging")
	skipValidate = flag.Bool("skip-validation", false, "Skip validation phase (not recommended)")
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		fmt.Println("Usage: automapper-gen [options] <package-path>")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Configure logging
	if *debug {
		logger.SetLevel(logger.LogLevelDebug)
	} else if *verbose {
		logger.SetLevel(logger.LogLevelVerbose)
	}

	pkgPath := args[0]
	startTime := time.Now()

	logger.Section("automapper-gen v0.0.1 | MIT License | git.weirdcat.su/weirdcat/automapper-gen")
	logger.Info("Package: %s", pkgPath)
	logger.Info("Verbose mode: %v", *verbose || *debug)

	if err := run(pkgPath, startTime); err != nil {
		logger.Error("Generation failed: %v", err)
		os.Exit(1)
	}
}

func run(pkgPath string, startTime time.Time) error {
	totalSteps := 5
	currentStep := 1

	// Step 1: Load configuration
	logger.Step(currentStep, totalSteps, "Loading configuration")
	currentStep++
	stepStart := time.Now()

	cfgPath := filepath.Join(pkgPath, "automapper.json")
	logger.Verbose("Config file: %s", cfgPath)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger.Progress(stepStart, "Config loaded")
	logger.Verbose("Output file: %s", cfg.Output)
	logger.Verbose("External packages: %d", len(cfg.ExternalPackages))

	if len(cfg.ExternalPackages) > 0 {
		for _, pkg := range cfg.ExternalPackages {
			logger.Verbose("  - %s (alias: %s)", pkg.ImportPath, pkg.Alias)
		}
	}

	if len(cfg.Converters) > 0 {
		logger.Verbose("Converters: %d", len(cfg.Converters))
		for _, conv := range cfg.Converters {
			safeStr := ""
			if conv.Trusted {
				safeStr = " [safe]"
			}
			logger.Debug("  - %s -> %s%s", conv.Name, conv.Function, safeStr)
		}
	}

	// Step 2: Parse package
	logger.Step(currentStep, totalSteps, "Parsing Go package")
	currentStep++
	stepStart = time.Now()

	dtos, sources, functions, pkgName, err := parser.ParsePackage(pkgPath, cfg)
	if err != nil {
		return fmt.Errorf("parsing package: %w", err)
	}

	logger.Progress(stepStart, "Parsing complete")
	logger.Verbose("Package name: %s", pkgName)
	logger.Verbose("Found %d DTOs with automapper annotations", len(dtos))
	logger.Verbose("Found %d source structs", len(sources))
	logger.Verbose("Found %d functions", len(functions))

	// List DTOs found
	for _, dto := range dtos {
		logger.Debug("DTO: %s (sources: %v, fields: %d)", dto.Name, dto.Sources, len(dto.Fields))
	}

	// List source structs
	if logger.IsDebugEnabled() {
		for name, source := range sources {
			external := ""
			if source.IsExternal {
				external = fmt.Sprintf(" [external: %s]", source.ImportPath)
			}
			logger.Debug("Source: %s (fields: %d)%s", name, len(source.Fields), external)
		}
	}

	// List functions
	if logger.IsDebugEnabled() {
		for name, fn := range functions {
			logger.Debug("Function: %s (params: %d, returns: %d)", name, len(fn.ParamTypes), len(fn.ReturnTypes))
		}
	}

	if len(dtos) == 0 {
		logger.Warning("No DTOs with automapper annotations found")
		logger.Info("Add automapper:from=SourceType comment above your DTO structs")
		logger.Info("Example:")
		logger.Info("  // automapper:from=User")
		logger.Info("  type UserDTO struct {")
		logger.Info("      ID   int64")
		logger.Info("      Name string")
		logger.Info("  }")
		return nil
	}

	// Step 3: Validation
	if !*skipValidate {
		logger.Step(currentStep, totalSteps, "Validating mappings")
		currentStep++
		stepStart = time.Now()

		v := validator.NewValidator(cfg, dtos, sources, functions)
		validationResult := v.Validate()

		logger.Progress(stepStart, "Validation complete")

		if !validationResult.IsValid() {
			return fmt.Errorf("validation failed with %d errors", len(validationResult.Errors))
		}

		if len(validationResult.Warnings) > 0 {
			logger.Warning("Proceeding with %d warnings", len(validationResult.Warnings))
		}
	} else {
		logger.Warning("Skipping validation (not recommended)")
	}

	// Step 4: Generate code
	logger.Step(currentStep, totalSteps, "Generating mapper code")
	currentStep++
	stepStart = time.Now()

	file, err := generator.Generate(dtos, sources, cfg, pkgName)
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	logger.Progress(stepStart, "Code generation complete")

	// Step 5: Write output
	logger.Step(currentStep, totalSteps, "Writing output file")
	stepStart = time.Now()

	outputPath := filepath.Join(pkgPath, cfg.Output)
	logger.Verbose("Output path: %s", outputPath)

	if err := file.Save(outputPath); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	logger.Progress(stepStart, "File written")

	// Final statistics
	logger.Stats("Generation Summary", map[string]any{
		"DTOs mapped":       len(dtos),
		"Source structs":    len(sources),
		"External packages": len(cfg.ExternalPackages),
		"Output file":       cfg.Output,
	})

	// Calculate total time
	elapsed := time.Since(startTime)
	logger.Success("Generation completed successfully in %v", elapsed.Round(time.Millisecond))

	return nil
}
