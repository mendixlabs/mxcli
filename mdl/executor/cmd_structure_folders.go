// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Folders in `show structure` (mendixlabs/mxcli#437, ako/mxcli#1240).
//
// At depth 2 and 3 a module's documents were listed flat under it, so a module
// organised into folders — Orders/Actions, Orders/Pages — read as one long
// list. The per-kind emitters still write one line per document (an entity's
// association line follows it, indented further); the lines are then regrouped
// by the folder the catalog records for each document: the documents at the
// module root first, then each folder as `Name/`, nested, sorted by name.
// Entities live in the domain model, which has no folders, so they stay at the
// root.

// structureModuleFolders maps each document of the module that sits in a
// folder to its folder path ("Orders/Actions"), from the catalog.
func structureModuleFolders(ctx *ExecContext, moduleName string) map[string]string {
	out := map[string]string{}
	result, err := structureQuery(ctx, fmt.Sprintf(
		"select QualifiedName, Folder from objects where ModuleName = '%s'", escapeSQLString(moduleName)))
	if err != nil || result == nil {
		return out
	}
	for _, row := range result.Rows {
		if folder := asString(row[1]); folder != "" {
			out[asString(row[0])] = folder
		}
	}
	return out
}

// writeStructureModuleNested runs emit with the output captured and writes its
// lines regrouped by folder.
func writeStructureModuleNested(ctx *ExecContext, moduleName string, emit func() error) error {
	saved := ctx.Output
	var buf bytes.Buffer
	ctx.Output = &buf
	err := emit()
	ctx.Output = saved
	if err != nil {
		return err
	}
	text := strings.TrimRight(buf.String(), "\n")
	if text == "" {
		return nil
	}
	for _, line := range nestStructureByFolder(strings.Split(text, "\n"), structureModuleFolders(ctx, moduleName)) {
		fmt.Fprintln(ctx.Output, line)
	}
	return nil
}

// structureFolderNode is a folder in the nested listing.
type structureFolderNode struct {
	lines    []string
	children map[string]*structureFolderNode
}

// nestStructureByFolder regroups a module's structure lines — each `  Kind
// Module.Name …`, with deeper-indented continuation lines belonging to the
// line before — under the folders documents sit in.
func nestStructureByFolder(lines []string, folders map[string]string) []string {
	if len(folders) == 0 {
		return lines
	}
	root := &structureFolderNode{children: map[string]*structureFolderNode{}}
	var current *structureFolderNode
	for _, line := range lines {
		if strings.HasPrefix(line, "    ") && current != nil {
			current.lines = append(current.lines, line)
			continue
		}
		current = root
		if path := folders[structureLineName(line)]; path != "" {
			for _, part := range strings.Split(path, "/") {
				next := current.children[part]
				if next == nil {
					next = &structureFolderNode{children: map[string]*structureFolderNode{}}
					current.children[part] = next
				}
				current = next
			}
		}
		current.lines = append(current.lines, line)
	}
	var out []string
	var write func(n *structureFolderNode, indent string)
	write = func(n *structureFolderNode, indent string) {
		for _, l := range n.lines {
			out = append(out, indent+l)
		}
		names := make([]string, 0, len(n.children))
		for name := range n.children {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, indent+"  "+name+"/")
			write(n.children[name], indent+"  ")
		}
	}
	write(root, "")
	return out
}

// structureLineName is the qualified name a structure line is about: the
// second word, up to the first character a name cannot hold.
func structureLineName(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	name := fields[1]
	if i := strings.IndexAny(name, "([:<"); i >= 0 {
		name = name[:i]
	}
	return name
}
