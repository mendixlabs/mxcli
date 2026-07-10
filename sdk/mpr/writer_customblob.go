// SPDX-License-Identifier: Apache-2.0

// Package mpr - Generic writer for CustomBlobDocument units (the BSON
// wrapper used by all four agent-editor document types: Agent, Model,
// Knowledge Base, Consumed MCP Service).
//
// Type-specific Contents JSON encoders live in writer_agenteditor_*.go.
package mpr

import (
	"encoding/json"
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"

	"go.mongodb.org/mongo-driver/bson"
)

// customBlobInput holds the per-type payload for the wrapper writer.
type customBlobInput struct {
	UnitID             string // canonical UUID of the document
	ContainerID        string // canonical UUID of the parent container (module/folder)
	Name               string
	Documentation      string
	Excluded           bool
	ExportLevel        string // "Hidden" by default
	CustomDocumentType string // e.g. "agenteditor.model"
	ReadableTypeName   string // e.g. "Model"
	MetadataID         string // canonical UUID for the embedded Metadata $ID
	ContentsJSON       string // type-specific JSON payload
}

// writeCustomBlobDocument serializes a CustomBlobDocument BSON wrapper
// and inserts it as a Documents-containment unit in the project.
func (w *Writer) writeCustomBlobDocument(in customBlobInput) error {
	if in.UnitID == "" {
		return fmt.Errorf("CustomBlobDocument unit ID is required")
	}
	if in.ContainerID == "" {
		return fmt.Errorf("CustomBlobDocument container ID is required")
	}
	if in.CustomDocumentType == "" {
		return fmt.Errorf("CustomDocumentType is required")
	}
	if in.ReadableTypeName == "" {
		return fmt.Errorf("ReadableTypeName is required")
	}
	if in.ExportLevel == "" {
		in.ExportLevel = "Hidden"
	}
	if in.MetadataID == "" {
		in.MetadataID = generateUUID()
	}

	metadata := bson.D{
		{Key: "$ID", Value: idToBsonBinary(in.MetadataID)},
		{Key: "$Type", Value: "CustomBlobDocuments$CustomBlobDocumentMetadata"},
		{Key: "CreatedByExtension", Value: agenteditor.CreatedByExtensionID},
		{Key: "ReadableTypeName", Value: in.ReadableTypeName},
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(in.UnitID)},
		{Key: "$Type", Value: customBlobDocType},
		{Key: "Contents", Value: in.ContentsJSON},
		{Key: "CustomDocumentType", Value: in.CustomDocumentType},
		{Key: "Documentation", Value: in.Documentation},
		{Key: "Excluded", Value: in.Excluded},
		{Key: "ExportLevel", Value: in.ExportLevel},
		{Key: "Metadata", Value: metadata},
		{Key: "Name", Value: in.Name},
	}

	contents, err := marshalUnitIDFirst(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal CustomBlobDocument BSON: %w", err)
	}

	return w.insertUnit(in.UnitID, in.ContainerID, "Documents", customBlobDocType, contents)
}

// updateCustomBlobDocument serializes a CustomBlobDocument BSON wrapper and
// replaces the existing unit in the project, preserving its UUID.
func (w *Writer) updateCustomBlobDocument(in customBlobInput) error {
	if in.UnitID == "" {
		return fmt.Errorf("CustomBlobDocument unit ID is required for update")
	}
	if in.ExportLevel == "" {
		in.ExportLevel = "Hidden"
	}
	if in.MetadataID == "" {
		in.MetadataID = generateUUID()
	}

	metadata := bson.D{
		{Key: "$ID", Value: idToBsonBinary(in.MetadataID)},
		{Key: "$Type", Value: "CustomBlobDocuments$CustomBlobDocumentMetadata"},
		{Key: "CreatedByExtension", Value: agenteditor.CreatedByExtensionID},
		{Key: "ReadableTypeName", Value: in.ReadableTypeName},
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(in.UnitID)},
		{Key: "$Type", Value: customBlobDocType},
		{Key: "Contents", Value: in.ContentsJSON},
		{Key: "CustomDocumentType", Value: in.CustomDocumentType},
		{Key: "Documentation", Value: in.Documentation},
		{Key: "Excluded", Value: in.Excluded},
		{Key: "ExportLevel", Value: in.ExportLevel},
		{Key: "Metadata", Value: metadata},
		{Key: "Name", Value: in.Name},
	}

	contents, err := marshalUnitIDFirst(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal CustomBlobDocument BSON: %w", err)
	}

	return w.updateUnit(in.UnitID, contents)
}

// marshalCanonicalJSON produces JSON without HTML escaping, matching the
// shape Studio Pro's agent-editor extension produces.
func marshalCanonicalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
