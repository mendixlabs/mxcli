---
title: Associations to Include on a Call External Action Argument
status: draft
---

# Proposal: Associations to Include on a Call External Action Argument

**Status:** Draft
**Date:** 2026-11-03
**Issue:** ako/mxcli#1914 (mendixlabs/mxcli#1721)

## Problem Statement

Studio Pro's *Call external action* activity has, per object argument, an
**Associations to include** tree: the associations whose objects are serialised
into the OData request together with the argument. A command DTO with a sub-DTO
linked by `CHANGE $Dto (Module.Quantity = $Qty)` reaches the service without the
sub-object unless `Quantity` is ticked. Nothing fails at build time; the field is
simply missing at runtime.

#1922 made `create or modify` **carry** a stored tree onto the rewritten call
(`mdl/executor/external_action_carry.go`). What is still missing:

- `describe microflow` does not print the tree, so a reader cannot see it and a
  describe → edit → exec loop only keeps it by the carry, never by what it says.
- A **new** call, a call to a parameter that now binds another entity, or a
  `drop` + `create` of the flow cannot author a tree at all. The carry has
  nothing to carry.

## BSON Structure

Measured on Studio Pro 11.16.0, `ako/TestApp` `Odata.IncludeAssociationsMF`
(committed as `mdl/backend/modelsdk/testdata/TestApp.IncludeAssociationsMF.11_16.bson`),
the `CreateBuffer` call on `Odata.Bug1914`, with `Quantity` ticked and `Unit`
ticked under it:

```
Microflows$CallExternalAction
  ParameterMappings: [3,
    Microflows$ExternalActionParameterMapping
      ParameterName: "command"
      Argument: "$NewCreateBufferCommand"
      ParameterType: DataTypes$ObjectType { Entity: "Odata.CreateBufferCommand" }
      AdditionalAttributes: [2]                       -- 11.5.0, out of scope
      IncludedAssociations: [2,
        Microflows$IncludedAssociation
          Association: "Odata.Quantity"               -- BY_NAME, required
          IncludedAssociations: [2,
            Microflows$IncludedAssociation
              Association: "Odata.Unit"
              IncludedAssociations: [2]               -- written empty, not omitted
              IsParent: true ]
          IsParent: true ]
    Microflows$ExternalActionParameterMapping
      ParameterName: "additional"  …  IncludedAssociations: [2] ]
```

- It is a **tree**, not a flag (the upstream issue calls it a boolean; it is not).
  Each node names an association; the associations ticked under it are its
  children, reached from the entity at that association's far end.
- `IsParent` is true when the object on *this* side of the tree is the
  association's owner — its `ParentPointer`, i.e. the MDL `FROM` entity (see
  CLAUDE.md, "Association Parent/Child Pointer Semantics").
- Lists are marker-2 at both levels; a leaf writes its empty nested list.
  The reader and writer already exist and match this shape
  (`includedAssociationsFromGen` / `includedAssociationsToGen`); only the MDL
  surface is missing.
- The control fixture `…IncludeAssociationsMF_Off.11_16.bson` (nothing ticked)
  stores `IncludedAssociations: [2]` on every mapping; the writer omits the empty
  list, which builds clean (#1073).

## Proposed MDL Syntax

A clause on the **argument**, because the tree belongs to one argument and
differs between arguments of the same call:

```sql
$Response = call external action Odata.Bug1914.CreateBuffer(
  command = $NewCreateBufferCommand
    include associations (Odata.Quantity (Odata.Unit)),
  additional = empty
);
```

- `include associations ( A [ ( B, C (D) ) ], … )` — a parenthesised list of
  qualified association names; a name followed by its own `( … )` lists the
  associations included under it. Nesting mirrors the stored tree one to one.
- `IsParent` is **derived**, not written: for each node, the entity on this side
  is the argument's `ParameterType` entity at the root, and the far-end entity of
  the parent node below it. `IsParent` is whether that entity is the
  association's `FROM` end.
- A **self-association** (both ends the same entity) is **refused**: the
  direction cannot be derived, and guessing it would serialise the wrong side.
  The refusal names the association and says to tick it in Studio Pro; a stored
  one still round-trips through the #1922 carry when the clause is omitted, and
  `describe` prints a comment for it instead of a clause it could not re-execute.
- `describe microflow` prints the clause whenever the stored tree is non-empty,
  so the round trip is exact and the #1922 carry becomes the fallback for a
  script that omits the clause, rather than the only route.
- An **omitted** clause keeps today's behaviour (carry the stored tree on
  `create or modify`); `include associations ()` states "none" explicitly and
  clears it. That matches how other omitted activity settings preserve what is
  stored.

## Implementation Plan

| File | Change |
|------|--------|
| `mdl/grammar/domains/MDLMicroflow.g4` | `includeAssociationsClause` after the argument expression in the external-action argument rule; recursive `includedAssociation : qualifiedName (LPAREN includedAssociation (COMMA includedAssociation)* RPAREN)?` |
| `mdl/ast/ast_microflow.go` | `IncludeAssociations []*IncludedAssociationDef` (+ an "explicitly empty" flag) on the external-action argument |
| `mdl/visitor/…` | build the tree |
| `mdl/executor/cmd_microflows_builder_calls.go` | resolve each node: association exists, touches the entity on this side (else refuse, naming the entity), derive `IsParent`; skip the #1922 carry for an argument that states the clause |
| `mdl/executor/cmd_microflows_format_action.go` | print the clause from `ParameterMapping.IncludedAssociations` |
| `mdl/executor/validate_*.go` | the same resolution for `check -p --references` |
| `cmd/mxcli/syntax/features_integration.go` | document the clause |

No reader/writer change: both already handle the tree.

## Version Compatibility

`Microflows$IncludedAssociation` and `ExternalActionParameterMapping.IncludedAssociations`
were introduced in **10.11.0** (`modelsdk/canon/mendixmodelsdk_type_versions.json`,
`modelsdk/version/metamodel_versions_gen.go`). A non-empty clause needs a version
gate at 10.11.0; the explicit empty clause writes nothing and needs none.

## Test Plan

- Parser: the nested clause, the empty clause, and a refusal for a name that is
  not qualified.
- Builder: the fixture's tree rebuilt from MDL equals the stored one (association
  names, nesting, `IsParent`), including a case where `IsParent` is false
  (the argument entity is the association's `TO` end).
- Describe: the fixture describes with the clause; describe → exec → describe is
  "Unchanged" against the fixture project.
- Refusal: an association that does not touch the entity on its side, and a
  self-association.
- `mx check` on a fresh 10.11+ and 11.x app with the clause: 0 errors; the same
  call without it at runtime omits the sub-object (the reporter's symptom).
- `mdl-examples/bug-tests/1914-external-action-include-associations.mdl`.

## Decisions

- **Spelling:** the per-argument `include associations ( … )` clause, not a
  property list. ADR-0013's property list holds an *activity's* settings; this
  setting belongs to one argument, and the clause keeps it next to that argument.
- **Self-associations:** refused (see Proposed MDL Syntax), not defaulted.

## Open Questions

1. **Should an omitted clause keep carrying** the stored tree (proposed), or warn
   once the clause exists so that scripts converge on stating it?
2. `AdditionalAttributes` (11.5.0, also per argument, also unauthored) is out of
   scope here; it likely needs the same treatment and the same clause position.
