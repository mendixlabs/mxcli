// SPDX-License-Identifier: Apache-2.0

// Package emit provides code generation functionality for producing Go source files.
package emit

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"

	"github.com/JordtenBulte-OLC/mxcli/internal/codegen/transform"
)

// Emitter generates Go source files from transformed types.
type Emitter struct {
	OutputDir string
	Version   string
}

// NewEmitter creates a new emitter.
func NewEmitter(outputDir, version string) *Emitter {
	return &Emitter{
		OutputDir: outputDir,
		Version:   version,
	}
}

// EmitPackage generates Go files for a transformed package.
func (e *Emitter) EmitPackage(pkg *transform.GoPackage) error {
	// Create package directory
	pkgDir := filepath.Join(e.OutputDir, pkg.Name)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return fmt.Errorf("failed to create package directory: %w", err)
	}

	// Emit types.go
	if len(pkg.Types) > 0 || len(pkg.Interfaces) > 0 {
		if err := e.emitTypesFile(pkgDir, pkg); err != nil {
			return fmt.Errorf("failed to emit types.go: %w", err)
		}
	}

	// Emit enums.go if there are enums
	if len(pkg.Enums) > 0 {
		if err := e.emitEnumsFile(pkgDir, pkg); err != nil {
			return fmt.Errorf("failed to emit enums.go: %w", err)
		}
	}

	return nil
}

// emitTypesFile generates the types.go file for a package.
func (e *Emitter) emitTypesFile(pkgDir string, pkg *transform.GoPackage) error {
	data := &FileData{
		Version:     e.Version,
		PackageName: pkg.Name,
		Imports:     pkg.Imports,
	}

	// Convert interfaces
	for _, iface := range pkg.Interfaces {
		data.Interfaces = append(data.Interfaces, &InterfaceData{
			Name:         iface.Name,
			Comment:      iface.Comment,
			MarkerMethod: iface.MarkerMethod,
		})
	}

	// Convert types
	for _, t := range pkg.Types {
		typeData := &TypeData{
			Name:          t.Name,
			Comment:       t.Comment,
			MarkerMethods: t.MarkerMethods,
		}

		for _, f := range t.Fields {
			typeData.Fields = append(typeData.Fields, &FieldData{
				Name:      f.Name,
				GoType:    f.GoType,
				JSONTag:   f.JSONTag,
				OmitEmpty: f.OmitEmpty,
			})
		}

		data.Types = append(data.Types, typeData)
	}

	// Render template
	var buf bytes.Buffer
	if err := Templates.ExecuteTemplate(&buf, "file", data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Format Go code
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Write unformatted for debugging
		unformattedPath := filepath.Join(pkgDir, "types.go.unformatted")
		os.WriteFile(unformattedPath, buf.Bytes(), 0644)
		return fmt.Errorf("failed to format Go code (unformatted written to %s): %w", unformattedPath, err)
	}

	// Write file
	filePath := filepath.Join(pkgDir, "types.go")
	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// emitEnumsFile generates the enums.go file for a package.
func (e *Emitter) emitEnumsFile(pkgDir string, pkg *transform.GoPackage) error {
	data := &EnumsFileData{
		Version:     e.Version,
		PackageName: pkg.Name,
	}

	// Convert enums
	for _, enum := range pkg.Enums {
		enumData := &EnumData{
			Name:    enum.Name,
			Comment: enum.Comment,
		}

		for _, v := range enum.Values {
			enumData.Values = append(enumData.Values, &EnumValueData{
				Name:  v.Name,
				Value: v.Value,
			})
		}

		data.Enums = append(data.Enums, enumData)
	}

	// Render template
	var buf bytes.Buffer
	if err := Templates.ExecuteTemplate(&buf, "enums", data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Format Go code
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Write unformatted for debugging
		unformattedPath := filepath.Join(pkgDir, "enums.go.unformatted")
		os.WriteFile(unformattedPath, buf.Bytes(), 0644)
		return fmt.Errorf("failed to format Go code (unformatted written to %s): %w", unformattedPath, err)
	}

	// Write file
	filePath := filepath.Join(pkgDir, "enums.go")
	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
