// SPDX-License-Identifier: Apache-2.0

package types

import (
	"encoding/json"
	"regexp"
	"strings"
)

// CustomBlobDocumentUnitType is the $Type of every Agent Editor document — agent,
// model, knowledge base, consumed and published MCP service.
const CustomBlobDocumentUnitType = "CustomBlobDocuments$CustomBlobDocument"

// blobQualifiedName matches one `"qualifiedName": "<value>"` pair in an Agent
// Editor document's Contents, whatever the whitespace around the colon (Agent
// Editor 2.6 indents Contents, older versions do not).
var blobQualifiedName = regexp.MustCompile(`("qualifiedName"\s*:\s*)("(?:[^"\\]|\\.)*")`)

// RewriteBlobQualifiedNames rewrites the references to oldName in the Contents
// JSON of an Agent Editor document (ako/mxcli#2265).
//
// Agent Editor keeps every reference — an agent's tools, knowledge bases, MCP
// services, model and context entity; a model's or knowledge base's key
// constant; an MCP service's endpoint and microflows — as
// {"documentId": …, "qualifiedName": …} inside one JSON string, so a sweep over
// BSON string values never sees the name on its own. Studio Pro re-syncs them
// from the agent-editor extension; neither mx nor mxbuild loads it, so after a
// headless rename nothing else would.
//
// A value is rewritten when it equals oldName or starts with oldName + "." —
// the same rule as a BSON reference — and only the value changes: the
// documentId, the key order and the layout are kept byte for byte, so a
// rename leaves the rest of the document as Agent Editor wrote it.
func RewriteBlobQualifiedNames(contents, oldName, newName string) (string, int) {
	if oldName == "" || !strings.Contains(contents, oldName) {
		return contents, 0
	}
	count := 0
	out := blobQualifiedName.ReplaceAllStringFunc(contents, func(pair string) string {
		m := blobQualifiedName.FindStringSubmatch(pair)
		var value string
		if json.Unmarshal([]byte(m[2]), &value) != nil {
			return pair
		}
		var renamed string
		switch {
		case value == oldName:
			renamed = newName
		case strings.HasPrefix(value, oldName+"."):
			renamed = newName + value[len(oldName):]
		default:
			return pair
		}
		quoted, err := json.Marshal(renamed)
		if err != nil {
			return pair
		}
		count++
		return m[1] + string(quoted)
	})
	return out, count
}
