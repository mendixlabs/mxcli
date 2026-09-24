// SPDX-License-Identifier: Apache-2.0

package ast

// ImageItem represents an image to add to a collection: IMAGE "name" FROM FILE 'path'.
type ImageItem struct {
	Name     string // Image name (e.g. "logo")
	FilePath string // Path to image file on disk
}

// CreateImageCollectionStmt represents:
//
//	CREATE IMAGE COLLECTION Module.Name [EXPORT LEVEL 'Public'] [COMMENT '...'] [(IMAGE "name" FROM FILE 'path', ...)]
type CreateImageCollectionStmt struct {
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	CreateOrModify   bool
	ExportLevel      string // "Hidden" (default) or "Public"
	Comment          string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Images           []ImageItem
}

func (s *CreateImageCollectionStmt) isStatement() {}

// DropImageCollectionStmt represents: DROP IMAGE COLLECTION Module.Name
type DropImageCollectionStmt struct {
	Name QualifiedName
	DropIfExists
}

func (s *DropImageCollectionStmt) isStatement() {}
