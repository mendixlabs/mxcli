// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func (pb *pageBuilder) getFilterWidgetIDForAttribute(attrPath string) string {
	attrType := pb.findAttributeType(attrPath)
	if attrType == nil {
		return pages.WidgetIDDataGridTextFilter // Default to text filter
	}

	switch attrType.(type) {
	case *domainmodel.StringAttributeType:
		return pages.WidgetIDDataGridTextFilter
	case *domainmodel.IntegerAttributeType, *domainmodel.LongAttributeType,
		*domainmodel.DecimalAttributeType, *domainmodel.AutoNumberAttributeType:
		return pages.WidgetIDDataGridNumberFilter
	case *domainmodel.DateTimeAttributeType, *domainmodel.DateAttributeType:
		return pages.WidgetIDDataGridDateFilter
	case *domainmodel.BooleanAttributeType, *domainmodel.EnumerationAttributeType:
		return pages.WidgetIDDataGridDropdownFilter
	default:
		return pages.WidgetIDDataGridTextFilter
	}
}

func (pb *pageBuilder) findAttributeType(attrPath string) domainmodel.AttributeType {
	if attrPath == "" {
		return nil
	}

	// Parse the attribute path
	parts := strings.Split(attrPath, ".")
	var entityName, attrName string

	if len(parts) >= 3 {
		// Format: Module.Entity.Attribute
		entityName = parts[0] + "." + parts[1]
		attrName = parts[len(parts)-1]
	} else if len(parts) == 2 {
		// Could be Entity.Attribute or Module.Entity - use context
		if pb.entityContext != "" {
			entityName = pb.entityContext
			attrName = parts[len(parts)-1]
		} else {
			// Assume Module.Entity format without attribute
			return nil
		}
	} else {
		// Just attribute name, use entity context
		if pb.entityContext != "" {
			entityName = pb.entityContext
			attrName = parts[0]
		} else {
			return nil
		}
	}

	// Find the entity and attribute
	domainModels, err := pb.getDomainModels()
	if err != nil {
		return nil
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return nil
	}

	// Parse entity qualified name
	entityParts := strings.Split(entityName, ".")
	if len(entityParts) < 2 {
		return nil
	}
	moduleName := entityParts[0]
	entityShortName := entityParts[1]

	// Find the entity
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		if modName != moduleName {
			continue
		}
		for _, entity := range dm.Entities {
			if entity.Name == entityShortName {
				attr := entity.FindAttributeByName(attrName)
				if attr != nil {
					return attr.Type
				}
				return nil
			}
		}
	}

	return nil
}
