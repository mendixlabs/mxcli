// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
)

// ReadBSONString reads a string field from elem's raw BSON without
// surfacing decode errors. Returns "" when elem is nil, has no raw
// payload, or the key is absent / unreadable. Mirrors the
// readBSONString helper that used to live in
// mdl/executor/cmd_javaactions_gen.go so executor describe code can
// stay off the modelsdk/codec package.
//
// Used for free-form fields that the generated JavaAction types do
// not expose as typed accessors — typically the "Entity",
// "Enumeration", "Caption", "Category", "Icon", "Name",
// "TypeParameter" and "Grammar" string fields stored on legacy
// CodeActions$* / JavaActions$* shapes.
func ReadBSONString(elem element.Element, key string) string {
	if elem == nil {
		return ""
	}
	raw := elem.Raw()
	if raw == nil {
		return ""
	}
	if s, err := codec.ReadBSONFieldString(raw, key); err == nil {
		return s
	}
	return ""
}

// DecodeChildElement decodes a single embedded document child of elem
// by BSON key via the gen-type registry. Returns nil when elem is
// nil, has no raw payload, or the child is missing / undecodable.
//
// Used to peel the "Type" sub-document out of BasicParameterType (the
// wrapper around the actual parameter type) and the "Parameter"
// sub-document out of ListType (the wrapper around the list element
// type).
func DecodeChildElement(elem element.Element, key string) element.Element {
	if elem == nil {
		return nil
	}
	raw := elem.Raw()
	if raw == nil {
		return nil
	}
	child, err := codec.DecodeChild(raw, key)
	if err != nil {
		return nil
	}
	return child
}
