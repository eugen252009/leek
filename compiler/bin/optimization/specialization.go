package optimization

import (
	"fmt"
	"sort"

	"leek/bin/constraints"
	"leek/bin/parser"
)

const maxVariantsPerFunction = 4
const maxVariants = 64
const maxSpecializationDepth = 16

// Witness refers to one exact instantiated obligation in the fresh report.
// Argument terms, assumptions and required predicates are available via that
// report; these diagnostics are never accepted back as proof authorization.
type Witness struct {
	Caller      string
	Path        []string
	Operation   string
	Obligation  int
	Environment int
}

type Routing struct{ Call, Target string }
type Variant struct {
	Name, Function string
	Eliminated     []string
	Calls          []Routing
}
type SpecializationResult struct {
	Code          string
	LocalElisions []Elision
	Variants      []Variant
	Witnesses     []Witness
	// Analysis belongs to this generation only, and is provided for inspection.
	Analysis *constraints.Report
}

// Generate retains the original local-elision diagnostics API. Interprocedural
// variants and their exact proof provenance are exposed by GenerateSpecialized.
func Generate(g *parser.ProgramGraph) (string, []Elision, error) {
	r, err := GenerateSpecialized(g)
	if err != nil {
		return "", nil, err
	}
	return r.Code, r.LocalElisions, nil
}

type candidate struct {
	f        *parser.FunctionInfo
	checks   map[*parser.GraphNode]bool
	children map[*parser.GraphNode]*candidate
}

func newCandidate(f *parser.FunctionInfo) *candidate {
	return &candidate{f: f, checks: map[*parser.GraphNode]bool{}, children: map[*parser.GraphNode]*candidate{}}
}

func sortedNodes[T any](m map[*parser.GraphNode]T) []*parser.GraphNode {
	keys := make([]*parser.GraphNode, 0, len(m))
	for n := range m {
		keys = append(keys, n)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
	return keys
}

// Each candidate tree is rooted in ONE analyzed caller. Imported obligations
// identify a sequence of semantic call sites, not just a function name. Never
// install these caller-dependent facts in a generic callee's body.
func GenerateSpecialized(g *parser.ProgramGraph) (*SpecializationResult, error) {
	report, err := constraints.Analyze(g)
	if err != nil {
		return nil, err
	}
	local, elisions := localChecks(g, report)
	out := &SpecializationResult{LocalElisions: elisions, Analysis: report}
	functions := map[string]*constraints.FunctionReport{}
	for _, f := range report.Functions {
		functions[f.Name] = f
	}
	// Structural owner lookup validates every imported call path against the
	// actual graph, including the identity of the final arithmetic operation.
	calls := map[*parser.FunctionInfo]map[*parser.GraphNode]*parser.FunctionInfo{}
	for _, f := range g.Functions {
		calls[f] = map[*parser.GraphNode]*parser.FunctionInfo{}
		for _, c := range f.Calls {
			calls[f][g.Expressions[c.Expr].Node] = c.Callee
		}
	}
	roots := []*candidate{}
	if !report.Limited {
		for _, f := range report.Functions {
			root := newCandidate(g.Functions[f.Name])
			roots = append(roots, root)
			for _, o := range f.Obligations {
				if o.Proof.Status != constraints.Proven || !o.Proof.Consistent || len(o.Origin.Via) == 0 || len(o.Origin.Via) > maxSpecializationDepth {
					continue
				}
				target := root.f
				path := []*parser.GraphNode{}
				valid := true
				for _, id := range o.Origin.Via {
					n := g.Nodes[id]
					target = calls[target][n]
					if target == nil || target.Effects != parser.EffectNone || functions[target.Decl.Name] == nil || !functions[target.Decl.Name].Complete {
						valid = false
						break
					}
					path = append(path, n)
				}
				if !valid || o.Origin.Function != target.Decl.Name {
					continue
				}
				n := o.Origin.SemanticNode
				if n == nil || g.Nodes[n.ID] != n {
					continue
				}
				e, ok := n.SourceNode.(*parser.Expr)
				if !ok || e.Kind != "binary" || g.Expressions[e] == nil || g.Expressions[e].Node != n || !supported(g.Expressions[e].Type, e.Name) {
					continue
				}
				// Recheck the entire local operation contract, not just one proven
				// fragment of a future multi-obligation arithmetic implementation.
				count := 0
				for _, own := range functions[target.Decl.Name].Obligations {
					if own.Origin.SemanticNode == n && len(own.Origin.Via) == 0 {
						count++
					}
				}
				if count != 1 || local[n] {
					continue
				}
				current := root
				for _, call := range path {
					if current.children[call] == nil {
						current.children[call] = newCandidate(calls[current.f][call])
					}
					current = current.children[call]
				}
				current.checks[n] = true
				out.Witnesses = append(out.Witnesses, Witness{f.Name, append([]string(nil), o.Origin.Via...), n.ID, o.ID, o.Environment})
			}
		}
	}
	b := &specializer{local: local, counts: map[*parser.FunctionInfo]int{}}
	routes := map[*parser.GraphNode]string{}
	for _, root := range roots {
		for _, call := range sortedNodes(root.children) {
			if name := b.variant(root.children[call]); name != "" {
				routes[call] = name
			}
		}
	}
	// A wrapper may already dispatch to these same variants using its own
	// generic branch facts. Reuse that canonical body instead of cloning an
	// implementation which offers no additional guard elimination.
	aliases := map[string]string{}
	resolve := func(name string) string {
		for aliases[name] != "" {
			name = aliases[name]
		}
		return name
	}
	for changed := true; changed; {
		changed = false
		for _, v := range b.variants {
			if aliases[v.Name] != "" {
				continue
			}
			redundant := true
			for n := range v.Proven {
				if !local[n] {
					redundant = false
					break
				}
			}
			for n, name := range v.Routes {
				if resolve(name) != resolve(routes[n]) {
					redundant = false
					break
				}
			}
			if redundant {
				aliases[v.Name] = v.Function.CName
				changed = true
			}
		}
	}
	for n, name := range routes {
		name = resolve(name)
		if name == g.Expressions[n.SourceNode.(*parser.Expr)].Callee.CName {
			delete(routes, n)
		} else {
			routes[n] = name
		}
	}
	for i := range b.variants {
		for n, name := range b.variants[i].Routes {
			name = resolve(name)
			if name == calls[b.variants[i].Function][n].CName {
				delete(b.variants[i].Routes, n)
			} else {
				b.variants[i].Routes[n] = name
			}
		}
	}
	// A parent can hit its budget after a child was considered. Discard any
	// orphan variants so static C never contains unreachable private functions.
	needed := map[string]bool{}
	var retain func(string)
	retain = func(name string) {
		if needed[name] {
			return
		}
		needed[name] = true
		for _, v := range b.variants {
			if v.Name == name {
				for _, child := range v.Routes {
					retain(child)
				}
				break
			}
		}
	}
	for _, name := range routes {
		retain(name)
	}
	variants := []parser.SpecializedFunction{}
	for _, v := range b.variants {
		if !needed[v.Name] {
			continue
		}
		variants = append(variants, v)
		d := Variant{Name: v.Name, Function: v.Function.Decl.Name}
		for _, n := range sortedNodes(v.Proven) {
			if !local[n] {
				d.Eliminated = append(d.Eliminated, n.ID)
			}
		}
		for _, n := range sortedNodes(v.Routes) {
			d.Calls = append(d.Calls, Routing{n.ID, v.Routes[n]})
		}
		out.Variants = append(out.Variants, d)
	}
	out.Code, err = parser.NewCodeGenerator().GenerateSpecializedChecks(g, local, routes, variants)
	return out, err
}

type specializer struct {
	local    map[*parser.GraphNode]bool
	counts   map[*parser.FunctionInfo]int
	variants []parser.SpecializedFunction
}

// Reuse keys are structural: exact body pointer, identical eliminated-node
// identities and identical child routes. Different argument substitutions may
// share this implementation ONLY after independent exact-site proof discharge.
// Thus a variant's contract is the conjunction of its removed guarded checks,
// not the caller's stronger, arbitrary facts. No proof text enters this gate.
func (b *specializer) variant(c *candidate) string {
	routes := map[*parser.GraphNode]string{}
	for _, call := range sortedNodes(c.children) {
		if name := b.variant(c.children[call]); name != "" {
			routes[call] = name
		}
	}
	if len(c.checks) == 0 && len(routes) == 0 {
		return ""
	}
	proven := map[*parser.GraphNode]bool{}
	for n := range b.local {
		proven[n] = true
	}
	for n := range c.checks {
		proven[n] = true
	}
	for _, v := range b.variants {
		if v.Function == c.f && sameMap(v.Proven, proven) && sameMap(v.Routes, routes) {
			return v.Name
		}
	}
	if b.counts[c.f] >= maxVariantsPerFunction || len(b.variants) >= maxVariants {
		return ""
	}
	b.counts[c.f]++
	name := fmt.Sprintf("lk_spec_%d", len(b.variants))
	b.variants = append(b.variants, parser.SpecializedFunction{Function: c.f, Name: name, Proven: proven, Routes: routes})
	return name
}
func sameMap[T comparable](a, b map[*parser.GraphNode]T) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
