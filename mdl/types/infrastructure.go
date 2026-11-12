// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/mendixlabs/mxcli/model"

// MPRVersion identifies the MPR file format version.
type MPRVersion int

const (
	MPRVersionV1 MPRVersion = 1 // single-file format
	MPRVersionV2 MPRVersion = 2 // mprcontents folder (Mendix 10.18+)
)

// ProjectVersion holds the parsed Mendix project version.
//
// FormatVersion is the MPR storage format (MPRVersionV1/V2) the reader detected
// on disk when it opened the project — not read from _MetaData, which cannot say
// (ako/mxcli#1553). It is zero when no reader filled it in.
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

// DocumentHeader is what a name listing needs of a document, read without
// decoding the rest of it.
type DocumentHeader struct {
	ID          model.ID
	ContainerID model.ID
	Name        string
	Excluded    bool
}

// FlowAccess is what deciding a stored flow's access takes: where it is, what
// it is called, whether it is excluded, and the module roles allowed to run it
// (qualified names). Kind is "microflow" or "nanoflow".
type FlowAccess struct {
	Kind               string
	ContainerID        model.ID
	Name               string
	Excluded           bool
	AllowedModuleRoles []model.ID
}

// UnitInfo is a lightweight unit descriptor.
type UnitInfo struct {
	ID              model.ID
	ContainerID     model.ID
	ContainmentName string
	Type            string
}

// DocumentUnit is a top-level document located by name rather than by type:
// its unit identity, where it sits, and what it turned out to be.
//
// Type is the unit's stored $Type (e.g. "JsonStructures$JsonStructure"), which
// is what makes a type-agnostic lookup reportable — the caller can name the
// kind of thing it moved without having known it in advance. Kind is that
// $Type rendered for humans ("json structure"), or the raw $Type when mxcli has
// no friendlier name for it.
type DocumentUnit struct {
	ID          model.ID
	ContainerID model.ID
	Name        string
	Type        string
	Kind        string
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

// AccessRightsLevel ranks Mendix member access rights so a GRANT can take the
// higher of two values. GRANT is documented as additive — it widens access and
// never removes it — so re-granting one member must not downgrade another
// (mendixlabs/mxcli#936). Narrowing is REVOKE's job.
//
// Anything unrecognised (including the empty string, which is how an unset
// DefaultMemberAccessRights reads) ranks lowest, so it never wins a merge.
func AccessRightsLevel(s string) int {
	switch s {
	case "ReadWrite":
		return 2
	case "ReadOnly":
		return 1
	default:
		return 0
	}
}

// HigherAccessRights returns whichever of a and b grants more.
func HigherAccessRights(a, b string) string {
	if AccessRightsLevel(a) >= AccessRightsLevel(b) {
		return a
	}
	return b
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

// CustomWidgetInstance is one stored pluggable-widget instance, with the context
// needed to decide whether it may be modified. RawType/RawObject are bson.D in the
// backends; here they are any to avoid a BSON driver dependency.
type CustomWidgetInstance struct {
	WidgetID   string // e.g. com.mendix.widget.web.datagrid.Datagrid
	WidgetName string // the instance's Name
	RawType    any
	RawObject  any

	UnitID   string
	UnitName string
	UnitType string // Forms$Page | Forms$Snippet | Forms$BuildingBlock | Forms$Layout

	ModuleName string
	// ModuleIsTheme marks a module Mendix declines to modify — update-widgets skips
	// theme modules and module-import refuses them.
	ModuleIsTheme bool
}
