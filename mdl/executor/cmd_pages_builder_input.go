// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// unquoteIdentifier strips surrounding double-quotes or backticks from a quoted identifier.
func unquoteIdentifier(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// unquoteQualifiedName strips quotes from each segment of a dotted qualified name.
func unquoteQualifiedName(s string) string {
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = unquoteIdentifier(p)
	}
	return strings.Join(parts, ".")
}

// resolveAttributePath resolves a short attribute name to a fully qualified name
// using the current entity context. If the attribute already has dots or no entity
// context is available, the attribute is returned as-is.
func (pb *pageBuilder) resolveAttributePath(attr string) string {
	if attr == "" {
		return ""
	}
	// If the attribute already contains a dot, it's already qualified
	if strings.Contains(attr, ".") {
		return attr
	}
	// If we have an entity context, prefix the attribute with it
	if pb.entityContext != "" {
		return pb.entityContext + "." + attr
	}
	return attr
}

// resolveAssociationPath resolves a short association name to a fully qualified name.
// Associations are module-level objects, so the path is Module.AssociationName (2-part).
// If the name already contains a dot, it's returned as-is.
func (pb *pageBuilder) resolveAssociationPath(assocName string) string {
	if assocName == "" {
		return ""
	}
	// If already qualified (contains a dot), return as-is
	if strings.Contains(assocName, ".") {
		return assocName
	}
	// Extract module name from entity context (e.g., "PgTest.Order" → "PgTest")
	if pb.entityContext != "" {
		parts := strings.SplitN(pb.entityContext, ".", 2)
		if len(parts) >= 1 {
			return parts[0] + "." + assocName
		}
	}
	return assocName
}

// resolveSnippetRef resolves a snippet qualified name to its ID.
func (pb *pageBuilder) resolveSnippetRef(snippetRef string) (model.ID, error) {
	if snippetRef == "" {
		return "", mdlerrors.NewValidation("empty snippet reference")
	}

	snippetRef = unquoteQualifiedName(snippetRef)
	parts := strings.Split(snippetRef, ".")
	var moduleName, snippetName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		snippetName = parts[len(parts)-1]
	} else {
		snippetName = snippetRef
	}

	snippets, err := pb.backend.ListSnippets()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, s := range snippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if s.Name == snippetName && (moduleName == "" || modName == moduleName) {
			return s.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("snippet", snippetRef)
}

func (pb *pageBuilder) resolveMicroflow(qualifiedName string) (model.ID, error) {
	qualifiedName = unquoteQualifiedName(qualifiedName)
	// Parse qualified name
	parts := strings.Split(qualifiedName, ".")
	if len(parts) < 2 {
		return "", mdlerrors.NewValidationf("invalid microflow name: %s", qualifiedName)
	}
	moduleName := parts[0]
	mfName := strings.Join(parts[1:], ".")

	// First, check if the microflow was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdMicroflows != nil {
		if info, ok := pb.execCache.createdMicroflows[qualifiedName]; ok {
			return info.ID, nil
		}
	}

	// Get microflows from backend
	mfs, err := pb.getMicroflows()
	if err != nil {
		return "", mdlerrors.NewBackend("list microflows", err)
	}

	// Use hierarchy to resolve module names (handles microflows in folders)
	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find matching microflow
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && mf.Name == mfName {
			return mf.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("microflow", qualifiedName)
}

func (pb *pageBuilder) resolvePageRef(pageRef string) (model.ID, error) {
	if pageRef == "" {
		return "", mdlerrors.NewValidation("empty page reference")
	}

	pageRef = unquoteQualifiedName(pageRef)
	parts := strings.Split(pageRef, ".")
	var moduleName, pageName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		pageName = parts[len(parts)-1]
	} else {
		pageName = pageRef
	}

	// First, check if the page was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdPages != nil {
		if info, ok := pb.execCache.createdPages[pageRef]; ok {
			return info.ID, nil
		}
		// Also check with module prefix if not found
		if moduleName != "" {
			if info, ok := pb.execCache.createdPages[moduleName+"."+pageName]; ok {
				return info.ID, nil
			}
		}
	}

	pgs, err := pb.getPages()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, p := range pgs {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if p.Name == pageName && (moduleName == "" || modName == moduleName) {
			return p.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("page", pageRef)
}
