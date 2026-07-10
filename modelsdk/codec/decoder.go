package codec

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Decoder decodes a raw BSON document into an Element by dispatching on $Type.
type Decoder struct {
	registry *TypeRegistry
}

// NewDecoder returns a Decoder backed by the given registry.
func NewDecoder(r *TypeRegistry) *Decoder {
	return &Decoder{registry: r}
}

// RawInitializer is an optional interface that generated types can implement
// to populate their typed fields from the raw BSON bytes after construction.
type RawInitializer interface {
	InitFromRaw(raw bson.Raw)
}

// Decode parses raw and returns the appropriate Element.
// For types not found in the registry it returns a bare *element.Base that
// still carries the original raw bytes so the document round-trips safely.
func (d *Decoder) Decode(raw bson.Raw) (element.Element, error) {
	typeName := decodeTypeName(raw)
	if typeName == "" {
		return nil, fmt.Errorf("missing $Type in BSON document")
	}

	id := decodeID(raw)

	factory, ok := d.registry.Lookup(typeName)
	if !ok {
		// Unknown type — preserve raw bytes in a generic Base so the document
		// can be round-tripped without data loss.
		b := &element.Base{}
		b.SetTypeName(typeName)
		b.SetID(id)
		b.SetRaw(raw)
		return b, nil
	}

	elem := factory()

	// All generated types embed element.Base, which exposes these setters.
	if base, ok := elem.(interface{ SetTypeName(string) }); ok {
		base.SetTypeName(typeName)
	}
	if base, ok := elem.(interface{ SetID(element.ID) }); ok {
		base.SetID(id)
	}
	if base, ok := elem.(interface{ SetRaw(bson.Raw) }); ok {
		base.SetRaw(raw)
	}

	// Let the element parse its own typed fields if it knows how.
	if ri, ok := elem.(RawInitializer); ok {
		ri.InitFromRaw(raw)
	}

	return elem, nil
}

// decodeTypeName extracts the $Type string from a raw BSON document.
func decodeTypeName(raw bson.Raw) string {
	val, err := raw.LookupErr("$Type")
	if err != nil {
		return ""
	}
	s, _ := val.StringValueOK()
	return s
}

// fieldAliases maps SDK property names to their BSON storage names where they
// differ. Mendix historically used "Form" for "Layout" and "Page", and the
// Attribute.Type child element is stored as "NewType" in current MPR BSON
// (see sdk/mpr/parser_domainmodel.go::parseAttribute — the legacy "Type"
// remains a fallback for ancient projects).
//
// Both directions are needed: Studio Pro writes "NewType" (so looking up "Type"
// falls back to "NewType"), and the mxcli legacy write path wrote "Type" (so
// looking up "NewType" must fall back to "Type").
//
// A SequenceFlow's branch case is likewise stored under "NewCaseValue" (the
// storage name) while the gen property is "CaseValue" (see
// sdk/mpr/parser_microflow.go:165 — legacy reads CaseValues, then NewCaseValue).
// Without this alias the case is never decoded, so an enumeration/boolean split
// reads as case-less and DESCRIBE MICROFLOW renders "if cond then end if;",
// dropping the entire then/when body.
var fieldAliases = map[string]string{
	"LayoutCall":   "FormCall",
	"PageSettings": "FormSettings",
	"Layout":       "Form",
	"Type":         "NewType",
	"NewType":      "Type",
	"CaseValue":    "NewCaseValue",
}

// DecodeChild decodes a single embedded document child from raw BSON by key.
// It uses DefaultRegistry to dispatch on $Type. Falls back to fieldAliases
// when the primary key is not found.
func DecodeChild(raw bson.Raw, key string) (element.Element, error) {
	val, err := raw.LookupErr(key)
	if err != nil {
		if alias, ok := fieldAliases[key]; ok {
			val, err = raw.LookupErr(alias)
		}
		if err != nil {
			return nil, fmt.Errorf("key %q not found", key)
		}
	}
	doc, ok := val.DocumentOK()
	if !ok {
		return nil, fmt.Errorf("key %q is not a document", key)
	}
	dec := NewDecoder(DefaultRegistry)
	return dec.Decode(bson.Raw(doc))
}

// DecodeChildren decodes an array of embedded document children from raw BSON.
// It uses DefaultRegistry to dispatch on $Type for each element.
func DecodeChildren(raw bson.Raw, key string) ([]element.Element, error) {
	val, err := raw.LookupErr(key)
	if err != nil {
		return nil, fmt.Errorf("key %q not found", key)
	}
	arr, ok := val.ArrayOK()
	if !ok {
		return nil, fmt.Errorf("key %q is not an array", key)
	}
	elems, err := arr.Values()
	if err != nil {
		return nil, fmt.Errorf("key %q array elements: %w", key, err)
	}
	dec := NewDecoder(DefaultRegistry)
	result := make([]element.Element, 0, len(elems))
	for _, el := range elems {
		doc, ok := el.DocumentOK()
		if !ok {
			continue // skip non-document elements
		}
		child, err := dec.Decode(bson.Raw(doc))
		if err != nil {
			continue // skip elements that fail to decode
		}
		result = append(result, child)
	}
	return result, nil
}

// decodeID extracts the $ID field, accepting either a UUID binary or a plain string.
func decodeID(raw bson.Raw) element.ID {
	val, err := raw.LookupErr("$ID")
	if err != nil {
		return ""
	}
	return decodeIDValue(val)
}

// decodeIDValue converts a BSON value (binary UUID or string) to an element.ID.
func decodeIDValue(val bson.RawValue) element.ID {
	switch val.Type {
	case bson.TypeBinary:
		_, data, ok := val.BinaryOK()
		if ok && len(data) == 16 {
			return element.ID(BinaryToUUID(data))
		}
	case bson.TypeString:
		s, _ := val.StringValueOK()
		return element.ID(s)
	}
	return ""
}

const hexchars = "0123456789abcdef"

// BinaryToUUID converts a 16-byte binary to a UUID string using Microsoft GUID
// format (little-endian first 3 groups) to match Mendix standard representation.
//
// Uses a stack-allocated [36]byte buffer instead of fmt.Sprintf to avoid
// the extra allocation and formatting overhead (fmt.Sprintf was the dominant
// alloc in DecodeRegisteredType — 32% of all allocation objects).
func BinaryToUUID(data []byte) string {
	if len(data) != 16 {
		return ""
	}
	var buf [36]byte
	// Group 1 (4 bytes, little-endian): data[3..0]
	buf[0] = hexchars[data[3]>>4]
	buf[1] = hexchars[data[3]&0xf]
	buf[2] = hexchars[data[2]>>4]
	buf[3] = hexchars[data[2]&0xf]
	buf[4] = hexchars[data[1]>>4]
	buf[5] = hexchars[data[1]&0xf]
	buf[6] = hexchars[data[0]>>4]
	buf[7] = hexchars[data[0]&0xf]
	buf[8] = '-'
	// Group 2 (2 bytes, little-endian): data[5..4]
	buf[9] = hexchars[data[5]>>4]
	buf[10] = hexchars[data[5]&0xf]
	buf[11] = hexchars[data[4]>>4]
	buf[12] = hexchars[data[4]&0xf]
	buf[13] = '-'
	// Group 3 (2 bytes, little-endian): data[7..6]
	buf[14] = hexchars[data[7]>>4]
	buf[15] = hexchars[data[7]&0xf]
	buf[16] = hexchars[data[6]>>4]
	buf[17] = hexchars[data[6]&0xf]
	buf[18] = '-'
	// Group 4 (2 bytes, big-endian): data[8..9]
	buf[19] = hexchars[data[8]>>4]
	buf[20] = hexchars[data[8]&0xf]
	buf[21] = hexchars[data[9]>>4]
	buf[22] = hexchars[data[9]&0xf]
	buf[23] = '-'
	// Group 5 (6 bytes, big-endian): data[10..15]
	buf[24] = hexchars[data[10]>>4]
	buf[25] = hexchars[data[10]&0xf]
	buf[26] = hexchars[data[11]>>4]
	buf[27] = hexchars[data[11]&0xf]
	buf[28] = hexchars[data[12]>>4]
	buf[29] = hexchars[data[12]&0xf]
	buf[30] = hexchars[data[13]>>4]
	buf[31] = hexchars[data[13]&0xf]
	buf[32] = hexchars[data[14]>>4]
	buf[33] = hexchars[data[14]&0xf]
	buf[34] = hexchars[data[15]>>4]
	buf[35] = hexchars[data[15]&0xf]
	return string(buf[:])
}
