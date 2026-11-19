// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// OverlappingEntitiesRule flags entities whose boxes cover each other on a
// domain model diagram.
//
// This is the sibling of MPR008, which does the same for microflow activities.
// The defect is the same class and so is the consequence: the diagram is the
// first thing a person opens to understand an app they are taking over, and a
// box hidden behind another is a member list they never read. Neither Studio
// Pro's own check nor mxbuild reports it, because the model is valid.
//
// mxcli has a particular stake in it. The positions are usually OURS: dmlayout
// places every entity a script does not position, so an overlap is as likely to
// be the placer's output as the author's doing. That makes this rule a check on
// mdl/dmlayout as much as on the project.
//
// # Geometry
//
// An entity stores only its Location (its box's TOP-LEFT corner). There is no
// Size in the model — Studio Pro derives the box from the name and member list
// when it draws, and mxcli never sees the result. So the box is an estimate,
// and the estimate used here is dmlayout's own (dmlayout.EntityBox): the rule
// and the placer have to agree about how big a box is, or `mxcli layout
// --only-overlapping` would leave behind overlaps this rule still reports, or
// chase ones it does not. Sharing the estimator makes that agreement structural
// rather than a thing to keep in step by hand.
//
// The predicate, though, is NOT the placer's. dmlayout.Rect.TooClose keeps
// boxes a clearance apart, which is right when placing and wrong when checking:
// two entities 10px apart are badly laid out but they do not overlap, and this
// rule may only report what is true. Hence Intersects, with no margin.
type OverlappingEntitiesRule struct{}

func NewOverlappingEntitiesRule() *OverlappingEntitiesRule {
	return &OverlappingEntitiesRule{}
}

func (r *OverlappingEntitiesRule) ID() string                       { return "MPR016" }
func (r *OverlappingEntitiesRule) Name() string                     { return "OverlappingEntities" }
func (r *OverlappingEntitiesRule) Category() string                 { return "correctness" }
func (r *OverlappingEntitiesRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }
func (r *OverlappingEntitiesRule) Description() string {
	return "Entities whose boxes cover each other on the domain model diagram, hiding their members from anyone reading it"
}

func (r *OverlappingEntitiesRule) Check(ctx *linter.LintContext) []linter.Violation {
	dms := ctx.DomainModels()
	if len(dms) == 0 {
		return nil
	}
	moduleNames := ctx.OwnedModuleNamesByID()

	var violations []linter.Violation
	for _, dm := range dms {
		if dm == nil || len(dm.Entities) < 2 {
			continue
		}
		module := moduleNames[dm.ContainerID]
		if module == "" || ctx.IsExcluded(module) {
			continue
		}
		for _, cluster := range overlappingEntityClusters(dm.Entities) {
			if ctx.IsDocumentExcluded(module + "." + cluster[0].Name) {
				continue
			}
			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message:  overlappingEntitiesMessage(module, cluster),
				Location: linter.Location{
					Module:       module,
					DocumentType: "entity",
					DocumentName: cluster[0].Name,
					DocumentID:   string(cluster[0].ID),
				},
				Suggestion: "Run 'mxcli layout --only-overlapping' (with --dry-run first) to move just the entities that collide and leave every other position alone, or give them explicit positions.",
			})
		}
	}
	return violations
}

// maxNamedInCluster bounds how many entity names a message spells out. A pile of
// 26 named in full is a message nobody reads to the end, and the count carries
// the point by itself.
const maxNamedInCluster = 6

// overlappingEntitiesMessage words one finding. A pair reads as a pair; a larger
// cluster leads with the count, because "these 26 are stacked up" is the fact
// worth acting on and the individual names are not.
func overlappingEntitiesMessage(module string, cluster []*domainmodel.Entity) string {
	names := make([]string, 0, len(cluster))
	for _, e := range cluster {
		names = append(names, e.Name)
	}
	if len(cluster) == 2 {
		return fmt.Sprintf("Entities '%s' and '%s' overlap on the %s domain model diagram, so one hides part of the other.",
			names[0], names[1], module)
	}
	shown := names
	suffix := ""
	if len(shown) > maxNamedInCluster {
		shown = shown[:maxNamedInCluster]
		suffix = fmt.Sprintf(" and %d more", len(names)-maxNamedInCluster)
	}
	return fmt.Sprintf("%d entities overlap each other on the %s domain model diagram: %s%s.",
		len(cluster), module, strings.Join(shown, ", "), suffix)
}

// overlappingEntityClusters groups entities whose estimated boxes intersect into
// connected clusters, and returns only the clusters with something in them —
// one per group of entities that collide, never one per pair.
//
// Pairwise would be wrong at scale, not just noisy. The benchmark that motivated
// this rule found a model with "all 26 entities at the same position, one on top
// of another", which is 325 pairs and one fact. Grouping transitively reports
// that as a single finding whatever the pile's size, and needs no threshold to
// decide when a pile is a pile.
//
// Unlike MPR008 this does NOT skip the origin. An unpositioned microflow
// activity defaults to (0,0), so there the origin means "no position recorded";
// entities get a position from dmlayout on every create path, so entities at one
// position IS the defect, and skipping it would miss the worst case there is.
//
// The result is deterministic: entities are sorted by name within a cluster and
// clusters by their first name, so a finding does not move between runs.
func overlappingEntityClusters(ents []*domainmodel.Entity) [][]*domainmodel.Entity {
	live := make([]*domainmodel.Entity, 0, len(ents))
	for _, e := range ents {
		if e != nil {
			live = append(live, e)
		}
	}
	if len(live) < 2 {
		return nil
	}

	boxes := make([]dmlayout.Rect, len(live))
	for i, e := range live {
		boxes[i] = dmlayout.EntityBox(e)
	}

	// Union-find over the intersection graph: a cluster is a connected
	// component, so A over B over C is one finding even when A and C do not
	// touch each other.
	parent := make([]int, len(live))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := 0; i < len(live); i++ {
		for j := i + 1; j < len(live); j++ {
			if boxes[i].Intersects(boxes[j]) {
				if a, b := find(i), find(j); a != b {
					parent[a] = b
				}
			}
		}
	}

	groups := map[int][]*domainmodel.Entity{}
	for i, e := range live {
		root := find(i)
		groups[root] = append(groups[root], e)
	}

	var out [][]*domainmodel.Entity
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(i, j int) bool { return g[i].Name < g[j].Name })
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i][0].Name < out[j][0].Name })
	return out
}
