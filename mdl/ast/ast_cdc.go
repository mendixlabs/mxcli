// SPDX-License-Identifier: Apache-2.0

package ast

// CreateChangeDataCaptureServiceStmt represents CREATE [OR MODIFY] CHANGE DATA
// CAPTURE SERVICE (part C of PROPOSAL_kafka_event_integration.md).
//
// Only the live side is authored. The baseline — the frozen copy Studio Pro's
// "accept changes" writes, which identifies what it tracks by the GUIDs the
// runtime keys the database on — is computed from the domain model by the write
// path, never written here. That asymmetry is the point: a GUID an author could
// type is a GUID an author could get wrong, and a wrong one silently detaches a
// table from its data.
type CreateChangeDataCaptureServiceStmt struct {
	CreateGuard // `create … if not exists`
	Name        QualifiedName
	// ServiceName defaults to the document name. AppName is deliberately not
	// authorable: it is the topic's second segment and Studio Pro derives it
	// from the project, so MDL that set it could rename every topic the service
	// publishes to (§10).
	ServiceName    string
	ServiceNameSet bool
	Sources        []*CdcSourceDef

	CreateOrModify   bool
	Folder           string
	Documentation    string
	DocumentationSet bool // absent preserves, empty clears (mendixlabs/mxcli#1018)
}

func (s *CreateChangeDataCaptureServiceStmt) isStatement() {}

// CdcSourceDef is one tracked entity: `entity Module.Order (Members…)`, with an
// optional exposed name that defaults to the entity's own.
type CdcSourceDef struct {
	Entity      QualifiedName
	ExposedName string // from `as 'Name'`; empty means default to the entity name
	Members     []*CdcMemberDef
}

// CdcMemberDef is one tracked attribute or association. Which it is comes from
// the domain model, not from the statement: a name resolves to an attribute or
// an association of that entity, attributes first, and an ambiguity is a check
// error rather than a guess.
type CdcMemberDef struct {
	Name        string
	ExposedName string // from `as 'Name'`
}

// DropChangeDataCaptureServiceStmt represents DROP CHANGE DATA CAPTURE SERVICE.
type DropChangeDataCaptureServiceStmt struct {
	DropGuard // `drop … if exists`
	Name      QualifiedName
}

func (s *DropChangeDataCaptureServiceStmt) isStatement() {}
