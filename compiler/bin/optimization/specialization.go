package optimization

import (
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
	Assumptions    []Requirement
	Parameters     []constraints.Fact
	Semantics      string
	Cost           Cost
	ReuseCount     int
}
type SpecializationResult struct {
	Dataflow      []FusionDecision `json:"dataflow,omitempty"`
	Code          string           `json:"-"`
	LocalElisions []Elision
	Variants      []Variant
	Witnesses     []Witness
	Decisions     []Decision
	Summary       PolicySummary
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
	f         *parser.FunctionInfo
	checks    map[*parser.GraphNode]bool
	children  map[*parser.GraphNode]*candidate
	caller    string
	path      []*parser.GraphNode
	instances map[*parser.GraphNode]int
}

func newCandidate(f *parser.FunctionInfo) *candidate {
	return &candidate{f: f, checks: map[*parser.GraphNode]bool{}, children: map[*parser.GraphNode]*candidate{}, instances: map[*parser.GraphNode]int{}}
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
	implication, err := constraints.NewImplicationEngine(report)
	if err != nil {
		return nil, err
	}
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
			root.caller = f.Name
			roots = append(roots, root)
			for _, call := range f.Calls {
				callee := g.Functions[call.Function]
				if callee != nil && callee.Effects == parser.EffectNone && functions[call.Function].Complete {
					n := g.Nodes[call.Node]
					child := newCandidate(callee)
					child.caller = f.Name
					child.path = []*parser.GraphNode{n}
					root.children[n] = child
				}
			}
			for _, o := range f.Obligations {
				if len(o.Origin.Via) == 0 || len(o.Origin.Via) > maxSpecializationDepth {
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
						child := newCandidate(calls[current.f][call])
						child.caller = f.Name
						child.path = append(append([]*parser.GraphNode(nil), current.path...), call)
						current.children[call] = child
					}
					current = current.children[call]
				}
				current.instances[n] = o.ID
				proof, queryErr := implication.Implies(f.Name, o.Environment, o.Required)
				if queryErr != nil {
					return nil, queryErr
				}
				if o.Proof.Status != constraints.Proven || !o.Proof.Consistent || proof.Proof.Status != constraints.Proven || !proof.Proof.Consistent {
					continue
				}
				current.checks[n] = true
				out.Witnesses = append(out.Witnesses, Witness{f.Name, append([]string(nil), o.Origin.Via...), n.ID, o.ID, o.Environment})
			}
		}
	}
	b := &specializer{local: local, counts: map[*parser.FunctionInfo]int{}, functions: functions, implication: implication,
		records: map[string]*Variant{}, costs: map[*parser.FunctionInfo]int{}, proofCache: map[proofQuery]constraints.Proof{}}
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
		d := *b.records[v.Name]
		d.Eliminated = nil
		d.Calls = nil
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
	out.Decisions = b.decisions
	out.Summary = PolicySummary{Generated: len(variants), Allocated: len(b.variants), AdditionalUnits: b.units, BudgetExhausted: b.budgetExhausted}
	for i := range out.Decisions {
		d := &out.Decisions[i]
		actualRoutes := routes
		var actual string
		for _, id := range d.Path {
			n := g.Nodes[id]
			actual = actualRoutes[n]
			if actual == "" {
				actual = g.Expressions[n.SourceNode.(*parser.Expr)].Callee.CName
				actualRoutes = routes
			} else {
				for _, v := range variants {
					if v.Name == actual {
						actualRoutes = v.Routes
						break
					}
				}
			}
		}
		d.FinalTarget = actual
		if aliases[d.Selected] != "" {
			d.Action = "CANONICAL"
			d.Reason = "canonical_dispatch_equivalent"
			out.Summary.RedundantAvoided++
		}
		if d.Action == "REUSE" && actual == d.Selected {
			out.Summary.Reused++
			for j := range out.Variants {
				if out.Variants[j].Name == actual {
					out.Variants[j].ReuseCount++
				}
			}
		}
	}
	fused, decisions := fusionPlan(g, report)
	out.Dataflow = decisions
	out.Code, err = parser.NewCodeGenerator().GenerateDataflowPlan(g, local, routes, variants, fused)
	return out, err
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
