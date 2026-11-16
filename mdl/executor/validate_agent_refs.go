// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// validateAgentReferences resolves what an agent names — its Model and
// Entity, each tool's Document (a microflow, or a consumed MCP service for an
// MCP tool) and each knowledge base tool's Source — for create [or modify]
// agent and alter agent, and the agent an alter names (ako/mxcli#2293).
// check --references passed all of them and exec refused them part-way
// through the script.
//
// It calls exec's own resolvers (agentModelRef, agentEntityRef,
// buildAgentToolFromDef, buildAgentKBToolFromDef), so a refusal carries the
// message exec gives, and a System entity passes as it does there. exec
// resolves each statement against the project as the statements before it
// left it, so the script is replayed in order: a reference to something an
// EARLIER statement creates passes, one to something a later statement
// creates is refused, as exec refuses it. A create agent the project's version
// cannot store is MDL-VERSION01's, which exec raises before it resolves
// anything, so it is skipped here.
func validateAgentReferences(ctx *ExecContext, prog *ast.Program) []error {
	if ctx == nil || prog == nil || !ctx.Connected() {
		return nil
	}
	created := newScriptContext()
	// Agent Editor documents an earlier statement drops: the project still
	// holds them, exec no longer finds them.
	dropped := map[string]bool{}
	has := func(kind string, qn ast.QualifiedName) (inScript, gone bool) {
		key := kind + " " + qn.String()
		return created.customDocuments[key], dropped[key]
	}
	// A reference this pass cannot judge: what an execute script or a
	// generated module brings is not in the statements.
	opaque := func(qn ast.QualifiedName) bool {
		return created.opaqueScript || created.opaqueModules[qn.Module]
	}
	var errs []error
	for i, stmt := range prog.Statements {
		report := func(err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
			}
		}
		switch s := stmt.(type) {
		case *ast.CreateAgentStmt:
			if checkVersionGates(ctx, s) != nil {
				break
			}
			name := s.Name.String()
			if s.Model != nil && !opaque(*s.Model) {
				in, gone := has("agenteditor.model", *s.Model)
				if gone && !in {
					report(fmt.Errorf("create agent %s: model not found: %s", name, s.Model))
				} else if !in {
					_, err := agentModelRef(ctx, "create agent", name, *s.Model)
					report(err)
				}
			}
			if s.Entity != nil {
				report(checkAgentEntityRef(ctx, created, "create agent", name, *s.Entity))
			}
			for _, td := range s.Tools {
				report(checkAgentToolRef(ctx, created, has, opaque, "create agent", s.Name, td))
			}
			for _, kbd := range s.KBTools {
				report(checkAgentKBRef(ctx, has, opaque, "create agent", s.Name, kbd))
			}
		case *ast.AlterAgentStmt:
			in, gone := has("agenteditor.agent", s.Name)
			if !in && !opaque(s.Name) && (gone || findAgentEditorAgent(ctx, s.Name.Module, s.Name.Name) == nil) {
				report(mdlerrors.NewNotFound("agent", s.Name.String()))
				break
			}
			// exec applies the SETs in this order and names the agent by its
			// document name.
			keys := make([]string, 0, len(s.Sets))
			for key := range s.Sets {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
			for _, key := range keys {
				qn := parseQualifiedNameStr(s.Sets[key])
				switch strings.ToLower(key) {
				case "model":
					if opaque(qn) {
						continue
					}
					in, gone := has("agenteditor.model", qn)
					if gone && !in {
						report(fmt.Errorf("alter agent %s: model not found: %s", s.Name.Name, s.Sets[key]))
					} else if !in {
						_, err := agentModelRef(ctx, "alter agent", s.Name.Name, qn)
						report(err)
					}
				case "entity":
					report(checkAgentEntityRef(ctx, created, "alter agent", s.Name.Name, qn))
				}
			}
			for _, td := range s.AddTools {
				report(checkAgentToolRef(ctx, created, has, opaque, "alter agent", s.Name, td))
			}
			for _, kbd := range s.AddKBs {
				report(checkAgentKBRef(ctx, has, opaque, "alter agent", s.Name, kbd))
			}
		case *ast.DropAgentStmt:
			dropped["agenteditor.agent "+s.Name.String()] = true
			delete(created.customDocuments, "agenteditor.agent "+s.Name.String())
		case *ast.DropModelStmt:
			dropped["agenteditor.model "+s.Name.String()] = true
			delete(created.customDocuments, "agenteditor.model "+s.Name.String())
		case *ast.DropKnowledgeBaseStmt:
			dropped["agenteditor.knowledgebase "+s.Name.String()] = true
			delete(created.customDocuments, "agenteditor.knowledgebase "+s.Name.String())
		case *ast.DropConsumedMCPServiceStmt:
			dropped["agenteditor.consumedMCPService "+s.Name.String()] = true
			delete(created.customDocuments, "agenteditor.consumedMCPService "+s.Name.String())
		}
		created.collectSingle(stmt)
	}
	return errs
}

// checkAgentEntityRef is agentEntityRef for an entity the script has not
// created (or renamed, moved or imported into place) by this statement.
func checkAgentEntityRef(ctx *ExecContext, created *scriptContext, verb, agentName string, qn ast.QualifiedName) error {
	q := qn.String()
	if created.entities[q] || created.indirectEntities[q] || created.externalEntityModules[qn.Module] ||
		created.opaqueScript || created.opaqueModules[qn.Module] {
		return nil
	}
	_, err := agentEntityRef(ctx, verb, agentName, qn)
	return err
}

// checkAgentToolRef is buildAgentToolFromDef for a tool whose document the
// script has not created by this statement.
func checkAgentToolRef(ctx *ExecContext, created *scriptContext,
	has func(string, ast.QualifiedName) (bool, bool), opaque func(ast.QualifiedName) bool,
	verb string, agentName ast.QualifiedName, td ast.AgentToolDef) error {
	if td.Document == nil || opaque(*td.Document) {
		return nil
	}
	switch {
	case strings.EqualFold(td.ToolType, "MCP"):
		in, gone := has("agenteditor.consumedMCPService", *td.Document)
		if in {
			return nil
		}
		if gone {
			return fmt.Errorf("%s %s: consumed mcp service not found: %s", verb, &agentName, td.Document)
		}
	case strings.EqualFold(td.ToolType, "Microflow"):
		if created.microflows[td.Document.String()] {
			return nil
		}
	}
	_, err := buildAgentToolFromDef(ctx, verb, &agentName, td)
	return err
}

// checkAgentKBRef is buildAgentKBToolFromDef for a knowledge base the script
// has not created by this statement.
func checkAgentKBRef(ctx *ExecContext, has func(string, ast.QualifiedName) (bool, bool), opaque func(ast.QualifiedName) bool,
	verb string, agentName ast.QualifiedName, kbd ast.AgentKBToolDef) error {
	if kbd.Source == nil || opaque(*kbd.Source) {
		return nil
	}
	in, gone := has("agenteditor.knowledgebase", *kbd.Source)
	if in {
		return nil
	}
	if gone {
		return fmt.Errorf("%s %s: knowledge base not found: %s", verb, &agentName, kbd.Source)
	}
	_, err := buildAgentKBToolFromDef(ctx, verb, &agentName, kbd)
	return err
}
