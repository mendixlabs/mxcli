// SPDX-License-Identifier: Apache-2.0

// Command codegen generates Go types from Mendix reflection data.
//
// Usage:
//
//	go run cmd/codegen/main.go -version 10.0.0 -output ./generated
//
// Flags:
//
//	-version    Mendix version to generate for (default: "10.0.0")
//	-input      Path to reflection data directory (default: "libs/mendixmodellib/reflection-data")
//	-output     Output directory for generated Go files (default: "generated")
//	-namespace  Generate only specific namespace (comma-separated, or empty for all)
//	-single     Generate all types in a single 'metamodel' package (avoids import cycles)
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/internal/codegen/emit"
	"github.com/JordtenBulte-OLC/mxcli/internal/codegen/schema"
	"github.com/JordtenBulte-OLC/mxcli/internal/codegen/transform"
)

func main() {
	version := flag.String("version", "10.0.0", "Mendix version to generate for")
	inputDir := flag.String("input", "libs/mendixmodellib/reflection-data", "Path to reflection data directory")
	outputDir := flag.String("output", "generated", "Output directory for generated Go files")
	namespace := flag.String("namespace", "", "Generate only specific namespace (comma-separated, or empty for all)")
	singlePkg := flag.Bool("single", true, "Generate all types in a single 'metamodel' package")
	flag.Parse()

	if err := run(*version, *inputDir, *outputDir, *namespace, *singlePkg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(version, inputDir, outputDir, namespaceFilter string, singlePkg bool) error {
	fmt.Printf("Loading reflection data for Mendix %s...\n", version)

	// Load reflection data
	data, err := schema.Load(inputDir, version)
	if err != nil {
		return fmt.Errorf("failed to load reflection data: %w", err)
	}

	fmt.Printf("Loaded %d type definitions\n", len(data))

	// Get namespaces to generate
	allNamespaces := data.GetNamespaces()
	sort.Strings(allNamespaces)

	var namespacesToGenerate []string
	if namespaceFilter != "" {
		// Filter to specific namespaces
		filterSet := make(map[string]bool)
		for ns := range strings.SplitSeq(namespaceFilter, ",") {
			filterSet[strings.TrimSpace(ns)] = true
		}
		for _, ns := range allNamespaces {
			if filterSet[ns] {
				namespacesToGenerate = append(namespacesToGenerate, ns)
			}
		}
	} else {
		namespacesToGenerate = allNamespaces
	}

	fmt.Printf("Generating %d namespaces: %v\n", len(namespacesToGenerate), namespacesToGenerate)

	// Create transformer
	transformer := transform.NewTransformer(data)

	if singlePkg {
		return runSinglePackage(version, outputDir, namespacesToGenerate, transformer)
	}

	return runMultiPackage(version, outputDir, namespacesToGenerate, transformer)
}

func runSinglePackage(version, outputDir string, namespaces []string, transformer *transform.Transformer) error {
	// Collect all types into a single merged package
	merged := &transform.GoPackage{
		Name:      "metamodel",
		Namespace: "Metamodel",
		Imports:   make(map[string]bool),
	}
	merged.Imports["github.com/JordtenBulte-OLC/mxcli/model"] = true
	merged.Imports["time"] = true // Always include time for DateTime fields

	var totalTypes, totalInterfaces, totalEnums int

	for _, ns := range namespaces {
		fmt.Printf("  Processing %s...\n", ns)

		pkg := transformer.TransformNamespace(ns)

		// Collect imports (excluding cross-package references which are now in same package)
		for imp := range pkg.Imports {
			if !strings.Contains(imp, "/generated/") {
				merged.Imports[imp] = true
			}
		}

		// Merge types with namespace prefix to avoid conflicts
		for _, t := range pkg.Types {
			// Prefix type name with namespace for uniqueness
			t.Name = ns + t.Name
			t.Comment = "// " + t.Name + " represents a " + t.QualifiedName + " element."
			merged.Types = append(merged.Types, t)
		}

		for _, iface := range pkg.Interfaces {
			iface.Name = ns + iface.Name
			iface.MarkerMethod = "is" + strings.ToLower(ns) + iface.MarkerMethod[2:]
			merged.Interfaces = append(merged.Interfaces, iface)
		}

		for _, enum := range pkg.Enums {
			enum.Name = ns + enum.Name
			for _, v := range enum.Values {
				v.Name = ns + v.Name
			}
			merged.Enums = append(merged.Enums, enum)
		}

		// Also update marker methods in types to match renamed interfaces
		for _, t := range pkg.Types {
			for i, m := range t.MarkerMethods {
				t.MarkerMethods[i] = "is" + strings.ToLower(ns) + m[2:]
			}
		}

		totalTypes += len(pkg.Types)
		totalInterfaces += len(pkg.Interfaces)
		totalEnums += len(pkg.Enums)
	}

	fmt.Printf("Merged: %d types, %d interfaces, %d enums\n", totalTypes, totalInterfaces, totalEnums)

	// Sort for deterministic output
	sort.Slice(merged.Types, func(i, j int) bool {
		return merged.Types[i].Name < merged.Types[j].Name
	})
	sort.Slice(merged.Interfaces, func(i, j int) bool {
		return merged.Interfaces[i].Name < merged.Interfaces[j].Name
	})
	sort.Slice(merged.Enums, func(i, j int) bool {
		return merged.Enums[i].Name < merged.Enums[j].Name
	})

	// Emit single package
	emitter := emit.NewEmitter(outputDir, version)
	if err := emitter.EmitPackage(merged); err != nil {
		return fmt.Errorf("failed to emit metamodel package: %w", err)
	}

	fmt.Printf("Done! Generated files in %s/metamodel/\n", outputDir)
	return nil
}

func runMultiPackage(version, outputDir string, namespaces []string, transformer *transform.Transformer) error {
	emitter := emit.NewEmitter(outputDir, version)

	// Generate each namespace as separate package
	for _, ns := range namespaces {
		fmt.Printf("  Generating %s...\n", ns)

		pkg := transformer.TransformNamespace(ns)

		if err := emitter.EmitPackage(pkg); err != nil {
			return fmt.Errorf("failed to emit package %s: %w", ns, err)
		}

		fmt.Printf("    - %d types, %d interfaces, %d enums\n",
			len(pkg.Types), len(pkg.Interfaces), len(pkg.Enums))
	}

	fmt.Printf("Done! Generated files in %s/\n", outputDir)
	return nil
}
