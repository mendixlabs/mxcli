// SPDX-License-Identifier: Apache-2.0

package projectreport

import "sort"

// digraph is a small weighted directed graph over module names. The catalog
// has its own graph package for asset-level analysis; this one exists because
// the architecture report needs module-level SCCs and layers over a CHOSEN set
// of reference kinds (domain only), which graph_module_cycles — computed over
// every kind — cannot answer (ako/mxcli#2222).
type digraph struct {
	nodes []string
	w     map[string]map[string]int
}

func newDigraph(nodes []string) *digraph {
	g := &digraph{nodes: append([]string(nil), nodes...), w: map[string]map[string]int{}}
	sort.Strings(g.nodes)
	return g
}

func (g *digraph) add(from, to string, n int) {
	if from == to {
		return
	}
	if g.w[from] == nil {
		g.w[from] = map[string]int{}
	}
	g.w[from][to] += n
}

func (g *digraph) succ(n string) []string {
	var out []string
	for t := range g.w[n] {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// sccs returns the strongly connected components (Tarjan), each sorted, in a
// deterministic order. Singletons are included.
func (g *digraph) sccs() [][]string {
	index := map[string]int{}
	low := map[string]int{}
	on := map[string]bool{}
	var stack []string
	var out [][]string
	i := 0
	var visit func(v string)
	visit = func(v string) {
		index[v], low[v] = i, i
		i++
		stack = append(stack, v)
		on[v] = true
		for _, w := range g.succ(v) {
			if _, seen := index[w]; !seen {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if on[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			sort.Strings(comp)
			out = append(out, comp)
		}
	}
	for _, n := range g.nodes {
		if _, seen := index[n]; !seen {
			visit(n)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a][0] < out[b][0] })
	return out
}

// layers places each node on the length of the longest path from its
// component down the condensed graph to a component with no dependencies.
// Layer 0 depends on nothing in the graph; a module in a cycle shares its
// layer with the rest of the cycle.
func (g *digraph) layers() map[string]int {
	comps := g.sccs()
	compOf := map[string]int{}
	for ci, c := range comps {
		for _, n := range c {
			compOf[n] = ci
		}
	}
	memo := map[int]int{}
	var depth func(ci int, seen map[int]bool) int
	depth = func(ci int, seen map[int]bool) int {
		if d, ok := memo[ci]; ok {
			return d
		}
		seen[ci] = true
		best := 0
		for _, n := range comps[ci] {
			for _, t := range g.succ(n) {
				tc := compOf[t]
				if tc == ci || seen[tc] {
					continue
				}
				best = max(best, depth(tc, seen)+1)
			}
		}
		delete(seen, ci)
		memo[ci] = best
		return best
	}
	out := map[string]int{}
	for ci, c := range comps {
		d := depth(ci, map[int]bool{})
		for _, n := range c {
			out[n] = d
		}
	}
	return out
}
