// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"encoding/json"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"

	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// contentsSpec describes one JSON object level of an agent-editor document's
// Contents: the keys the encoder writes, and how to pair what is below them.
//
// The encoders build Contents from mxcli's typed fields, and Agent Editor
// stores more than that: per tool userAccessApproval, displayTitle and
// displayDescription, on the agent enableBackendStreaming, enableUIStreaming
// and selectedModel, on a knowledge base tool minSimilarity and isVisible. A
// rewrite from the typed fields alone dropped all of them. Writing onto the
// stored Contents keeps every key the encoder does not write, at its stored
// position.
type contentsSpec struct {
	modelled map[string]bool
	// objects are object-valued keys merged key by key.
	objects map[string]*contentsSpec
	// lists are array keys whose entries pair with a stored entry by idKey.
	lists map[string]*contentsSpec
	// idKey is the key a list entry pairs by: "id", or "uuid" for a published
	// MCP service's tools.
	idKey string
}

func contentKeys(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

var docRefSpec = &contentsSpec{modelled: contentKeys("documentId", "qualifiedName")}

var agentContentsSpec = &contentsSpec{
	modelled: contentKeys("description", "systemPrompt", "userPrompt", "usageType", "variables", "tools",
		"knowledgebaseTools", "model", "entity", "maxTokens", "toolChoice", "temperature", "topP",
		"enableBackendStreaming", "enableUIStreaming", "selectedModel"),
	objects: map[string]*contentsSpec{"model": docRefSpec, "entity": docRefSpec,
		"selectedModel": {modelled: contentKeys("id", "name")}},
	lists: map[string]*contentsSpec{
		"tools": {
			modelled: contentKeys("id", "name", "description", "enabled", "toolType", "document",
				"userAccessApproval", "displayTitle", "displayDescription"),
			objects: map[string]*contentsSpec{"document": docRefSpec},
		},
		"knowledgebaseTools": {
			modelled: contentKeys("id", "name", "description", "enabled", "toolType", "document",
				"collectionIdentifier", "maxResults", "minSimilarity", "isVisible"),
			objects: map[string]*contentsSpec{"document": docRefSpec},
		},
	},
}

var modelContentsSpec = &contentsSpec{
	modelled: contentKeys("type", "name", "displayName", "provider", "providerFields"),
	objects: map[string]*contentsSpec{"providerFields": {
		modelled: contentKeys("environment", "deepLinkURL", "keyId", "keyName", "resourceName", "key"),
	}},
}

var knowledgeBaseContentsSpec = &contentsSpec{
	modelled: contentKeys("name", "provider", "providerFields"),
	objects: map[string]*contentsSpec{"providerFields": {
		modelled: contentKeys("environment", "deepLinkURL", "keyId", "keyName", "modelDisplayName", "modelName", "key"),
	}},
}

var consumedMCPServiceContentsSpec = &contentsSpec{
	modelled: contentKeys("protocolVersion", "documentation", "version", "connectionTimeoutSeconds"),
}

var publishedMCPServiceContentsSpec = &contentsSpec{
	modelled: contentKeys("serverName", "endpointPath", "version", "protocolVersion", "authenticationEnabled",
		"authenticationMicroflow", "authorizationServerUrl", "scopes", "tools", "documentation"),
	objects: map[string]*contentsSpec{"authenticationMicroflow": docRefSpec},
	lists: map[string]*contentsSpec{
		"tools": {
			modelled: contentKeys("uuid", "name", "description", "sourceMicroflow", "title", "schema"),
			objects:  map[string]*contentsSpec{"sourceMicroflow": docRefSpec},
			idKey:    "uuid",
		},
	},
}

func contentsSpecFor(customType string) *contentsSpec {
	switch customType {
	case agenteditor.CustomTypeAgent:
		return agentContentsSpec
	case agenteditor.CustomTypeModel:
		return modelContentsSpec
	case agenteditor.CustomTypeKnowledgeBase:
		return knowledgeBaseContentsSpec
	case agenteditor.CustomTypeConsumedMCPService:
		return consumedMCPServiceContentsSpec
	case agenteditor.CustomTypePublishedMCPService:
		return publishedMCPServiceContentsSpec
	}
	return nil
}

// storedCustomBlob is what a rewrite keeps from the stored unit: its Contents
// and the extension that created it.
type storedCustomBlob struct {
	contents  string
	createdBy string
}

func (b *Backend) readStoredCustomBlob(unitID string) storedCustomBlob {
	var out storedCustomBlob
	if b.reader == nil || unitID == "" {
		return out
	}
	raw, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil || len(raw) == 0 {
		return out
	}
	doc := bsoncore.Document(raw)
	out.contents, _ = doc.Lookup("Contents").StringValueOK()
	out.createdBy, _ = doc.Lookup("Metadata", "CreatedByExtension").StringValueOK()
	return out
}

// carryStoredContents returns written with the keys spec does not model taken
// from stored. Keys keep stored's order; a modelled key the encoder left out
// (a cleared optional value) stays out; keys new in written are appended.
func carryStoredContents(stored, written string, spec *contentsSpec) (string, error) {
	if stored == "" || spec == nil {
		return written, nil
	}
	if !json.Valid([]byte(stored)) {
		return written, nil
	}
	out, err := mergeJSON(json.RawMessage(stored), json.RawMessage(written), spec)
	if err != nil {
		return "", fmt.Errorf("carry stored agent-editor contents: %w", err)
	}
	return formatLikeStored(out, stored)
}

func mergeJSON(stored, written json.RawMessage, spec *contentsSpec) (json.RawMessage, error) {
	s, okS := decodeObject(stored)
	w, okW := decodeObject(written)
	if !okS || !okW {
		return written, nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	n := 0
	emit := func(k string, v json.RawMessage) {
		if n > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(v)
		n++
	}
	for _, k := range s.keys {
		wv, inW := w.vals[k]
		switch {
		case inW:
			v, err := mergeValue(k, s.vals[k], wv, spec)
			if err != nil {
				return nil, err
			}
			emit(k, v)
		case spec.modelled[k]:
			// The encoder writes this key when it has a value: absent is cleared.
		default:
			emit(k, s.vals[k])
		}
	}
	for _, k := range w.keys {
		// An empty or zero value the stored document did not hold says nothing
		// a missing key does not; adding it would rewrite an unchanged document.
		if _, inS := s.vals[k]; !inS && !isZeroJSON(w.vals[k]) {
			emit(k, w.vals[k])
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func mergeValue(key string, stored, written json.RawMessage, spec *contentsSpec) (json.RawMessage, error) {
	if child := spec.objects[key]; child != nil {
		return mergeJSON(stored, written, child)
	}
	item := spec.lists[key]
	if item == nil {
		return written, nil
	}
	var sItems, wItems []json.RawMessage
	if json.Unmarshal(stored, &sItems) != nil || json.Unmarshal(written, &wItems) != nil {
		return written, nil
	}
	byID := map[string]json.RawMessage{}
	for _, si := range sItems {
		if id := entryID(si, item.idKey); id != "" {
			byID[id] = si
		}
	}
	out := make([]json.RawMessage, 0, len(wItems))
	for _, wi := range wItems {
		si, ok := byID[entryID(wi, item.idKey)]
		if !ok || entryID(wi, item.idKey) == "" {
			out = append(out, wi)
			continue
		}
		v, err := mergeJSON(si, wi, item)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	// Not json.Marshal: it would escape <, > and & inside the entries.
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, v := range out {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(v)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

func isZeroJSON(raw json.RawMessage) bool {
	switch string(raw) {
	case `""`, "0", "null":
		return true
	}
	return false
}

func entryID(raw json.RawMessage, idKey string) string {
	if idKey == "" {
		idKey = "id"
	}
	var e map[string]json.RawMessage
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	var id string
	_ = json.Unmarshal(e[idKey], &id)
	return id
}

// orderedObject is a JSON object with its key order.
type orderedObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func decodeObject(raw json.RawMessage) (orderedObject, bool) {
	obj := orderedObject{vals: map[string]json.RawMessage{}}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return obj, false
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return obj, false
		}
		k, ok := t.(string)
		if !ok {
			return obj, false
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return obj, false
		}
		if _, dup := obj.vals[k]; !dup {
			obj.keys = append(obj.keys, k)
		}
		obj.vals[k] = v
	}
	return obj, true
}
