// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// MappingBackend provides import/export mapping and JSON structure operations.
type MappingBackend interface {
	ListImportMappings() ([]*model.ImportMapping, error)
	GetImportMappingByQualifiedName(moduleName, name string) (*model.ImportMapping, error)
	CreateImportMapping(im *model.ImportMapping) error
	UpdateImportMapping(im *model.ImportMapping) error
	DeleteImportMapping(id model.ID) error
	MoveImportMapping(im *model.ImportMapping) error

	ListExportMappings() ([]*model.ExportMapping, error)
	GetExportMappingByQualifiedName(moduleName, name string) (*model.ExportMapping, error)
	CreateExportMapping(em *model.ExportMapping) error
	UpdateExportMapping(em *model.ExportMapping) error
	DeleteExportMapping(id model.ID) error
	MoveExportMapping(em *model.ExportMapping) error

	ListJsonStructures() ([]*types.JsonStructure, error)
	GetJsonStructureByQualifiedName(moduleName, name string) (*types.JsonStructure, error)
	CreateJsonStructure(js *types.JsonStructure) error
	// DeleteJsonStructure removes a JSON structure by ID.
	// Takes string (not model.ID) to match the SDK writer layer convention.
	DeleteJsonStructure(id string) error
}
