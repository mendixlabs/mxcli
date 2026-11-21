// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// publishedRestOperationDropRule warns that `create or modify published rest
// service` removes stored operations it does not list (ako/mxcli#2449,
// mendixlabs/mxcli#1903). The statement is the whole service, so a rewrite
// that names only the operation it means to add deletes the others — and exec
// said only "Modified published rest service". A warning, as for an entity's
// unstated members (MDL107): "modify to this shape" is a legitimate use.
const publishedRestOperationDropRule = "MDL-REST06"

// publishedRestOperationKey identifies an operation within its service:
// resource, method and path, as describe prints them.
func publishedRestOperationKey(resource, method, path string) string {
	p := strings.Trim(path, "/")
	return fmt.Sprintf("%s %s", strings.ToUpper(method), strings.TrimSuffix(resource+"/"+p, "/"))
}

// droppedPublishedRestOperations lists the stored operations a create or
// modify statement does not declare, in stored order.
func droppedPublishedRestOperations(stored *model.PublishedRestService, s *ast.CreatePublishedRestServiceStmt) []string {
	if stored == nil {
		return nil
	}
	declared := map[string]bool{}
	for _, r := range s.Resources {
		for _, op := range r.Operations {
			declared[publishedRestOperationKey(r.Name, op.HTTPMethod, op.Path)] = true
		}
	}
	var out []string
	for _, r := range stored.Resources {
		for _, op := range r.Operations {
			if k := publishedRestOperationKey(r.Name, op.HTTPMethod, op.Path); !declared[k] {
				out = append(out, k)
			}
		}
	}
	return out
}

func publishedRestDropMessage(qn string, dropped []string) string {
	return fmt.Sprintf("create or modify published rest service %s removes %d stored operation(s) it does not list: %s",
		qn, len(dropped), strings.Join(dropped, ", "))
}

const publishedRestDropSuggestion = "List every operation the service keeps — `describe published rest service` prints them — " +
	"or change one operation and leave the rest alone: `alter published rest service … add operation in resource '<name>' { … }` " +
	"or `drop operation <method> ['<path>'] in resource '<name>'`."

// CheckPublishedRestOperationDrops reports, before exec, each create or modify
// of a stored published REST service that would remove operations.
func (e *Executor) CheckPublishedRestOperationDrops(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	ctx := e.newExecContext(context.Background())
	if prog == nil || !ctx.Connected() {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreatePublishedRestServiceStmt)
		if !ok || !s.CreateOrModify {
			continue
		}
		stored, err := findPublishedRestService(ctx, s.Name.Module, s.Name.Name)
		if err != nil || stored == nil {
			continue
		}
		if dropped := droppedPublishedRestOperations(stored, s); len(dropped) > 0 {
			out = append(out, linter.Violation{
				RuleID:     publishedRestOperationDropRule,
				Severity:   linter.SeverityWarning,
				Location:   linter.Location{Module: s.Name.Module, DocumentType: "published rest service", DocumentName: s.Name.Name},
				Message:    publishedRestDropMessage(s.Name.String(), dropped),
				Suggestion: publishedRestDropSuggestion,
			})
		}
	}
	return out
}

// publishedRestAlterResourceProblems are the resource actions of an ALTER
// that exec refuses against the stored service: adding a resource it already
// has, or dropping one it does not. check passed both (ako/mxcli#2449). The
// actions are applied in order, as exec applies them.
func publishedRestAlterResourceProblems(stored *model.PublishedRestService, s *ast.AlterPublishedRestServiceStmt) []string {
	have := map[string]bool{}
	ops := map[string]bool{}
	for _, r := range stored.Resources {
		have[r.Name] = true
		for _, op := range r.Operations {
			ops[publishedRestOperationKey(r.Name, op.HTTPMethod, op.Path)] = true
		}
	}
	var out []string
	for _, a := range s.Actions {
		switch act := a.(type) {
		case *ast.PublishedRestAddOperationsAction:
			if !have[act.Resource] {
				out = append(out, fmt.Sprintf("resource %s not found on %s: `add operation in resource` adds to a stored resource; "+
					"a new one is `add resource %s { … }`", mdlQuoted(act.Resource), s.Name, mdlQuoted(act.Resource)))
				continue
			}
			for _, op := range act.Operations {
				k := publishedRestOperationKey(act.Resource, op.HTTPMethod, op.Path)
				if ops[k] {
					out = append(out, fmt.Sprintf("operation %s already exists on %s: `add operation` adds a new one; "+
						"to change it, `drop operation %s in resource %s` first and add it back",
						k, s.Name, publishedRestOperationSpelling(op.HTTPMethod, op.Path), mdlQuoted(act.Resource)))
				}
				ops[k] = true
			}
		case *ast.PublishedRestDropOperationAction:
			k := publishedRestOperationKey(act.Resource, act.HTTPMethod, act.Path)
			if !ops[k] {
				out = append(out, fmt.Sprintf("operation %s not found on %s", k, s.Name))
			}
			delete(ops, k)
		case *ast.PublishedRestAddResourceAction:
			if have[act.Resource.Name] {
				out = append(out, fmt.Sprintf("resource %s already exists on %s: `add resource` adds a new one; "+
					"to change it, `drop resource %s` first and add it back with every operation it keeps",
					mdlQuoted(act.Resource.Name), s.Name, mdlQuoted(act.Resource.Name)))
			}
			have[act.Resource.Name] = true
		case *ast.PublishedRestDropResourceAction:
			if !have[act.Name] {
				out = append(out, fmt.Sprintf("resource %s not found on %s", mdlQuoted(act.Name), s.Name))
			}
			delete(have, act.Name)
		}
	}
	return out
}
