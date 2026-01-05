package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"git.weirdcat.su/weirdcat/automapper-gen/internal/config"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/generator"
	"git.weirdcat.su/weirdcat/automapper-gen/internal/parser"
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		fmt.Println("Usage: automapper-gen <package-path>")
		os.Exit(1)
	}

	pkgPath := args[0]

	if err := run(pkgPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(pkgPath string) error {
	// Load configuration
	cfg, err := config.Load(filepath.Join(pkgPath, "automapper.json"))
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Parse Go files
	dtos, sources, pkgName, err := parser.ParsePackage(pkgPath, cfg)
	if err != nil {
		return fmt.Errorf("parsing package: %w", err)
	}

	if len(dtos) == 0 {
		fmt.Println("No DTOs with automapper annotations found")
		return nil
	}

	// Generate code
	file, err := generator.Generate(dtos, sources, cfg, pkgName)
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	// Write output
	outputPath := filepath.Join(pkgPath, cfg.Output)
	if err := file.Save(outputPath); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Printf("Generated %s with %d DTO mappings\n", outputPath, len(dtos))
	return nil
}
