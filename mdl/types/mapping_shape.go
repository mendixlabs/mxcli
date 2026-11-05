// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// JsonStructureLookup is the one backend call deciding a JSON mapping's root
// shape needs.
type JsonStructureLookup interface {
	GetJsonStructureByQualifiedName(moduleName, name string) (*JsonStructure, error)
}

// JsonMappingRootIsList answers "does this import mapping return a list" from
// the two sources that are DEFINITE — the root element's JSON path, and the JSON
// structure the mapping is built on — and reports known=false when neither
// answers. The microflow builder then falls back to the root element's
// occurrence bound; a rule that judges an activity against the shape (MDL-MAP04,
// at check time and in lint) stays silent instead, since a guess there would
// be a report against something that may build and run.
//
// It lives here rather than in the executor so the lint rules, which cannot
// import the executor, decide the shape the same way the builder does.
func JsonMappingRootIsList(lookup JsonStructureLookup, im *model.ImportMapping) (list, known bool) {
	if im == nil {
		return false, false
	}
	// A mapping rooted BELOW an array yields one object per item, whatever the
	// structure's own root is (#267). The structure check below answers from
	// js.Elements[0], which for `root choices/message` is the document root —
	// an Object — so it reported a single object and mxbuild rejected the
	// activity with CE0243 "the mapping ... now returns a value of type
	// 'List of X'". The mapping document itself is right: Studio Pro's own
	// OpenAI_API.IM_OpenAI stores the same MaxOccurs 1 on that root, so
	// list-ness cannot be read off the root element either — only off its path.
	if len(im.Elements) > 0 && im.Elements[0] != nil &&
		MappingRootPathCrossesArray(im.Elements[0].JsonPath) {
		return true, true
	}
	// A root whose path is stored is decided by it alone: one that does not
	// cross an array yields one object, even in an array-rooted structure.
	// A mapping may root at "(Array)" — the array itself — with its items a
	// child at "(Array)|(Object)"; it returns one root object, and answering
	// "list" from the structure wrote the import as returning a list: mx check
	// 11.14 CE0243 "The mapping used to return a value of type 'List of X', but
	// now returns a value of type 'X'" (ako/mxcli#1938). The structure decides
	// only when the mapping stores no path.
	if len(im.Elements) > 0 && im.Elements[0] != nil && im.Elements[0].JsonPath != "" {
		return false, true
	}
	if im.JsonStructure != "" && lookup != nil {
		parts := strings.SplitN(im.JsonStructure, ".", 2)
		if len(parts) == 2 {
			if js, err := lookup.GetJsonStructureByQualifiedName(parts[0], parts[1]); err == nil && js != nil && len(js.Elements) > 0 {
				return js.Elements[0].ElementType == "Array", true
			}
		}
	}
	return false, false
}

// MappingRootPathCrossesArray reports whether a mapping root's stored JsonPath
// passes through an array on its way down.
//
// An array contributes a marker segment for its item — "(Object)" for objects,
// "(Wrapper)" for primitives — so any marker AFTER the first segment means the
// root sits inside an array and the mapping yields many objects:
//
//	(Object)                             one object
//	(Object)|data                        one object, a nested root
//	(Array)|(Object)                     many — an array-rooted structure (#248)
//	(Object)|choices|(Object)|message    many — rooted below an array (#267)
//
// The first segment is skipped because it is always the document's own root
// marker, which says nothing about arrays.
func MappingRootPathCrossesArray(jsonPath string) bool {
	segs := strings.Split(jsonPath, "|")
	for i, seg := range segs {
		if i == 0 {
			continue
		}
		switch seg {
		case "(Object)", "(Array)", "(Wrapper)":
			return true
		}
	}
	return false
}
