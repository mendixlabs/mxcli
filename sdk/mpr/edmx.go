// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// Type aliases — all EDMX types now live in mdl/types.
type EdmxDocument = types.EdmxDocument
type EdmSchema = types.EdmSchema
type EdmEntityType = types.EdmEntityType
type EdmProperty = types.EdmProperty
type EdmNavigationProperty = types.EdmNavigationProperty
type EdmEntitySet = types.EdmEntitySet
type EdmAction = types.EdmAction
type EdmActionParameter = types.EdmActionParameter
type EdmEnumType = types.EdmEnumType
type EdmEnumMember = types.EdmEnumMember

// ParseEdmx delegates to types.ParseEdmx.
func ParseEdmx(metadataXML string) (*EdmxDocument, error) {
	return types.ParseEdmx(metadataXML)
}

// resolveNavType delegates to types.ResolveNavType (kept for test compatibility).
func resolveNavType(t string) (string, bool) {
	return types.ResolveNavType(t)
}
