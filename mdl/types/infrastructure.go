// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/JordtenBulte-OLC/mxcli/model"

// MPRVersion identifies the MPR file format version.
type MPRVersion int

const (
	MPRVersionV1 MPRVersion = 1 // single-file format
	MPRVersionV2 MPRVersion = 2 // mprcontents folder (Mendix 10.18+)
)

// ProjectVersion holds the parsed Mendix project version.
type ProjectVersion struct {
	ProductVersion string
	BuildVersion   string
	FormatVersion  int
	SchemaHash     string
	MajorVersion   int
	MinorVersion   int
	PatchVersion   int
}

// IsAtLeast returns true if this version is at least the specified major.minor version.
func (v *ProjectVersion) IsAtLeast(major, minor int) bool {
	if v.MajorVersion > major {
		return true
	}
	return v.MajorVersion == major && v.MinorVersion >= minor
}

// IsAtLeastFull returns true if this version is at least the specified major.minor.patch version.
func (v *ProjectVersion) IsAtLeastFull(major, minor, patch int) bool {
	if v.MajorVersion > major {
		return true
	}
	if v.MajorVersion == major && v.MinorVersion > minor {
		return true
	}
	if v.MajorVersion == major && v.MinorVersion == minor && v.PatchVersion >= patch {
		return true
	}
	return false
}

// String returns the product version string.
func (v *ProjectVersion) String() string {
	return v.ProductVersion
}

// IsMPRv2 returns true if the project uses MPR v2 format (mprcontents folder).
func (v *ProjectVersion) IsMPRv2() bool {
	return v.FormatVersion >= 2
}

// FolderInfo is a lightweight folder descriptor.
type FolderInfo struct {
	ID          model.ID
	ContainerID model.ID
	Name        string
}

// UnitInfo is a lightweight unit descriptor.
type UnitInfo struct {
	ID              model.ID
	ContainerID     model.ID
	ContainmentName string
	Type            string
}

// RenameHit records a single rename reference replacement.
type RenameHit struct {
	UnitID   string
	UnitType string
	Name     string
	Count    int
}

// UnitPatch holds a unit ID plus its patched BSON bytes. Returned by scan
// helpers that compute updates without writing them to disk.
type UnitPatch struct {
	ID       string
	Contents []byte
}

// BSONScanner exposes the unit-scanning methods of sdk/mpr.Reader that
// reference and rename services need, without requiring a concrete Reader.
type BSONScanner interface {
	ScanRenameReferences(oldName, newName string) ([]UnitPatch, []RenameHit, error)
	ScanQualifiedNameUpdates(oldName, newName string) ([]UnitPatch, error)
}

// RawUnit holds a unit's raw BSON contents.
type RawUnit struct {
	ID          model.ID
	ContainerID model.ID
	Type        string
	Contents    []byte
}

// RawUnitInfo holds a unit's raw contents with metadata.
type RawUnitInfo struct {
	ID            string
	QualifiedName string
	Type          string
	ModuleName    string
	Contents      []byte
}

// RawCustomWidgetType holds a custom widget's raw type/object data.
// RawType and RawObject are bson.D in sdk/mpr; here they are any to
// avoid a BSON driver dependency.
type RawCustomWidgetType struct {
	WidgetID   string
	RawType    any
	RawObject  any
	UnitID     string
	UnitName   string
	WidgetName string
}

// EntityMemberAccess describes access rights for a single entity member.
type EntityMemberAccess struct {
	AttributeRef   string
	AssociationRef string
	AccessRights   string
}

// EntityAccessRevocation describes which entity access to revoke.
type EntityAccessRevocation struct {
	RevokeCreate       bool
	RevokeDelete       bool
	RevokeReadMembers  []string
	RevokeWriteMembers []string
	RevokeReadAll      bool
	RevokeWriteAll     bool
}
