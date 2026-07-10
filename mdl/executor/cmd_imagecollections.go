// SPDX-License-Identifier: Apache-2.0

// Package executor - Image collection commands (CREATE/DROP IMAGE COLLECTION)
package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// execCreateImageCollection handles CREATE IMAGE COLLECTION statements.
func execCreateImageCollection(ctx *ExecContext, s *ast.CreateImageCollectionStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Check if image collection already exists
	existing := findImageCollection(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("image collection", s.Name.Module+"."+s.Name.Name)
	}

	containerID := module.ID
	if existing != nil {
		containerID = existing.ContainerID
	}

	// Build ImageCollection
	ic := &types.ImageCollection{
		ContainerID:   containerID,
		Name:          s.Name.Name,
		ExportLevel:   s.ExportLevel,
		Documentation: s.Comment,
	}
	if existing != nil {
		ic.ID = existing.ID
	}

	// Load image files
	for _, item := range s.Images {
		filePath := item.FilePath
		if !filepath.IsAbs(filePath) {
			cwd, err := os.Getwd()
			if err != nil {
				return mdlerrors.NewBackend("get working directory", err)
			}
			filePath = filepath.Join(cwd, filePath)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("read image file %q", item.FilePath), err)
		}
		format := extToImageFormat(filepath.Ext(filePath))
		ic.Images = append(ic.Images, types.Image{
			Name:   item.Name,
			Data:   data,
			Format: format,
		})
	}

	if existing != nil {
		if err := ctx.Backend.UpdateImageCollection(ic); err != nil {
			return mdlerrors.NewBackend("update image collection", err)
		}
		fmt.Fprintf(ctx.Output, "Modified image collection: %s\n", s.Name)
	} else {
		if err := ctx.Backend.CreateImageCollection(ic); err != nil {
			return mdlerrors.NewBackend("create image collection", err)
		}
		fmt.Fprintf(ctx.Output, "Created image collection: %s\n", s.Name)
	}

	// Invalidate hierarchy cache so the collection's container is visible
	invalidateHierarchy(ctx)
	return nil
}

// execDropImageCollection handles DROP IMAGE COLLECTION statements.
func execDropImageCollection(ctx *ExecContext, s *ast.DropImageCollectionStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	ic := findImageCollection(ctx, s.Name.Module, s.Name.Name)
	if ic == nil {
		return mdlerrors.NewNotFound("image collection", s.Name.String())
	}

	if err := ctx.Backend.DeleteImageCollection(string(ic.ID)); err != nil {
		return mdlerrors.NewBackend("delete image collection", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped image collection: %s\n", s.Name)
	return nil
}

// describeImageCollection handles DESCRIBE IMAGE COLLECTION Module.Name.
func describeImageCollection(ctx *ExecContext, name ast.QualifiedName) error {
	ic := findImageCollection(ctx, name.Module, name.Name)
	if ic == nil {
		return mdlerrors.NewNotFound("image collection", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(ic.ContainerID)
	modName := h.GetModuleName(modID)

	if ic.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", ic.Documentation)
	}

	exportLevel := ic.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}

	qualifiedName := fmt.Sprintf("%s.%s", modName, ic.Name)

	if len(ic.Images) == 0 {
		fmt.Fprintf(ctx.Output, "create or modify image collection %s", qualifiedName)
		if exportLevel != "Hidden" {
			fmt.Fprintf(ctx.Output, " export level '%s'", exportLevel)
		}
		fmt.Fprintln(ctx.Output, ";")
		fmt.Fprintln(ctx.Output, "/")
		return nil
	}

	// Write image data to temp files and output CREATE statement with IMAGE lines
	previewDir := filepath.Join("/tmp/mxcli-preview", qualifiedName)
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		return mdlerrors.NewBackend("create preview directory", err)
	}

	fmt.Fprintf(ctx.Output, "create or modify image collection %s", qualifiedName)
	if exportLevel != "Hidden" {
		fmt.Fprintf(ctx.Output, " export level '%s'", exportLevel)
	}
	fmt.Fprintln(ctx.Output, " (")

	for i, img := range ic.Images {
		ext := imageFormatToExt(img.Format)
		filePath := filepath.Join(previewDir, img.Name+ext)
		if len(img.Data) > 0 {
			if err := os.WriteFile(filePath, img.Data, 0o644); err != nil {
				return mdlerrors.NewBackend(fmt.Sprintf("write image %s", img.Name), err)
			}
		}

		comma := ","
		if i == len(ic.Images)-1 {
			comma = ""
		}
		fmt.Fprintf(ctx.Output, "    image %s from file '%s'%s\n", img.Name, filePath, comma)
	}

	fmt.Fprintln(ctx.Output, ");")
	fmt.Fprintln(ctx.Output, "/")
	return nil
}

// imageFormatToExt converts a Mendix ImageFormat value to a file extension.
func imageFormatToExt(format string) string {
	switch format {
	case "Svg":
		return ".svg"
	case "Gif":
		return ".gif"
	case "Jpg":
		return ".jpg"
	case "Bmp":
		return ".bmp"
	case "Webp":
		return ".webp"
	default:
		return ".png"
	}
}

// extToImageFormat converts a file extension to a Mendix ImageFormat value.
func extToImageFormat(ext string) string {
	switch strings.ToLower(ext) {
	case ".svg":
		return "Svg"
	case ".gif":
		return "Gif"
	case ".jpg", ".jpeg":
		return "Jpg"
	case ".bmp":
		return "Bmp"
	case ".webp":
		return "Webp"
	default:
		return "Png"
	}
}

// listImageCollections handles SHOW IMAGE COLLECTION [IN module].
func listImageCollections(ctx *ExecContext, moduleName string) error {
	collections, err := ctx.Backend.ListImageCollections()
	if err != nil {
		return mdlerrors.NewBackend("list image collections", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	result := &TableResult{
		Columns: []string{"Image Collection", "Export Level", "Images"},
	}

	for _, ic := range collections {
		modID := h.FindModuleID(ic.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}

		qualifiedName := fmt.Sprintf("%s.%s", modName, ic.Name)
		exportLevel := ic.ExportLevel
		if exportLevel == "" {
			exportLevel = "Hidden"
		}
		result.Rows = append(result.Rows, []any{qualifiedName, exportLevel, len(ic.Images)})
	}

	result.Summary = fmt.Sprintf("(%d image collection(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// findImageCollection finds an image collection by module and name.
func findImageCollection(ctx *ExecContext, moduleName, collectionName string) *types.ImageCollection {
	collections, err := ctx.Backend.ListImageCollections()
	if err != nil {
		return nil
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}

	for _, ic := range collections {
		modID := h.FindModuleID(ic.ContainerID)
		modName := h.GetModuleName(modID)
		if ic.Name == collectionName && modName == moduleName {
			return ic
		}
	}
	return nil
}
