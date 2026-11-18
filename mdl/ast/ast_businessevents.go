// SPDX-License-Identifier: Apache-2.0

package ast

// CreateBusinessEventServiceStmt represents CREATE BUSINESS EVENT SERVICE, in
// either of the document's two shapes.
//
// A PUBLISHED service names ServiceName/EventNamePrefix and declares Messages.
// A CONSUMED one names a contract — Contract (the AsyncAPI text) or
// ContractFile (a path to it) — and binds events from it with Consumes. The two
// are mutually exclusive; the visitor refuses a statement that mixes them
// (#2384 §5.1).
type CreateBusinessEventServiceStmt struct {
	CreateGuard     // `create … if not exists` (ako/mxcli#731)
	Name            QualifiedName
	ServiceName     string
	EventNamePrefix string
	Messages        []*BusinessEventMessageDef

	// Contract is the AsyncAPI document itself, from `Contract: $$…$$`.
	Contract string
	// ContractFile is a path to it, resolved relative to the script like an
	// image's `File:`. Its text is stored verbatim, so nothing afterwards
	// depends on the file still being there.
	ContractFile string
	// ContractSet records that one of them was written at all, so an empty
	// contract can be told from an absent one.
	ContractSet bool
	Consumes    []*BusinessEventConsumeDef

	CreateOrModify   bool
	Folder           string
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
}

// IsConsumer reports whether the statement describes a consumed service.
func (s *CreateBusinessEventServiceStmt) IsConsumer() bool {
	return s.ContractSet || len(s.Consumes) > 0
}

// BusinessEventConsumeDef binds one event of the contract to the entity it is
// mapped onto and the microflow that handles it: `consume X entity … microflow …`.
type BusinessEventConsumeDef struct {
	MessageName string
	Entity      string // Qualified name of the CBE_ entity
	Microflow   string // Qualified name of the handler
}

func (s *CreateBusinessEventServiceStmt) isStatement() {}

// BusinessEventMessageDef defines a message within a business event service.
type BusinessEventMessageDef struct {
	MessageName string
	Attributes  []*BusinessEventAttributeDef
	Operation   string // "PUBLISH" or "SUBSCRIBE"
	Entity      string // Qualified name of linked entity
	Microflow   string // Optional handler microflow
}

// BusinessEventAttributeDef defines an attribute within a message.
type BusinessEventAttributeDef struct {
	Name     string
	TypeName string // "Long", "String", "Integer", "Boolean", "DateTime", "Decimal"
}

// DropBusinessEventServiceStmt represents DROP BUSINESS EVENT SERVICE.
type DropBusinessEventServiceStmt struct {
	DropGuard
	Name QualifiedName
}

func (s *DropBusinessEventServiceStmt) isStatement() {}
