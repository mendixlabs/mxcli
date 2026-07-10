// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/JordtenBulte-OLC/mxcli/model"

// ModuleSettings holds the Projects$ModuleSettings document for a module.
type ModuleSettings struct {
	ID                  model.ID
	ContainerID         model.ID // module ID
	ExportLevel         string   // "Source" | "Protected"
	ProtectedModuleType string   // "AddOn" | "Solution"
	Version             string
	BasedOnVersion      string
	ExtensionName       string
	SolutionIdentifier  string
	JarDependencies     []*JarDependency
}

// JarDependency represents a single Maven JAR dependency in a module.
type JarDependency struct {
	ID         model.ID
	GroupID    string
	ArtifactID string
	Version    string
	IsIncluded bool
	Exclusions []*JarDependencyExclusion
}

// Coordinate returns the Maven coordinate string "groupId:artifactId".
func (d *JarDependency) Coordinate() string {
	return d.GroupID + ":" + d.ArtifactID
}

// JarDependencyExclusion represents a Maven exclusion (Mendix 10.20+).
type JarDependencyExclusion struct {
	ID         model.ID
	GroupID    string
	ArtifactID string
}

// Coordinate returns the Maven coordinate string "groupId:artifactId".
func (e *JarDependencyExclusion) Coordinate() string {
	return e.GroupID + ":" + e.ArtifactID
}
