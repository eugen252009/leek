package optimization

import (
	"leek/bin/constraints"
	"leek/bin/parser"
)

type QuantifiedDecision struct {
	Function         string                      `json:"function"`
	Stage            string                      `json:"stage"`
	Domain           string                      `json:"domain"`
	Callback         string                      `json:"callback"`
	Bounds           *constraints.QuantifiedInfo `json:"bounds"`
	Target           string                      `json:"target"`
	GuardsEliminated int                         `json:"guards_eliminated"`
}

// These candidates join the SAME registry as scalar candidates. Only the proof
// ledger differs: each entry denotes every reached invocation in one domain.
func quantifiedCandidates(g *parser.ProgramGraph, report *constraints.Report, roots []*candidate, functions map[string]*constraints.FunctionReport,
	calls map[*parser.FunctionInfo]map[*parser.GraphNode]*parser.FunctionInfo, local map[*parser.GraphNode]bool, engine *constraints.ImplicationEngine) error {
	if report.Limited {
		return nil
	}
	for _, root := range roots {
		f := functions[root.caller]
		for _, o := range f.QuantifiedObligations {
			if o.Environment < 0 || o.Environment >= len(f.Environments) || len(o.Origin.Via) == 0 || len(o.Origin.Via) > maxSpecializationDepth {
				continue
			}
			env := f.Environments[o.Environment]
			if env.Domain == "" || env.Stage != o.Origin.Via[0] || !env.Reachable {
				continue
			}
			// Bind to the actual stage/domain record, not another range's environment.
			validDomain := false
			for _, d := range f.Dataflow {
				if d.Node == env.Stage && d.Quantified != nil && d.Quantified.Domain != nil && d.Quantified.Domain.Node == env.Domain &&
					d.Quantified.State != constraints.EmptyDomain && d.Quantified.Domain.Proof.Status == constraints.Proven {
					validDomain = true
				}
			}
			if !validDomain {
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
			n := o.Origin.SemanticNode
			if !valid || target.Decl.Name != o.Origin.Function || n == nil || g.Nodes[n.ID] != n {
				continue
			}
			e, ok := n.SourceNode.(*parser.Expr)
			if !ok || e.Kind != "binary" || g.Expressions[e] == nil || g.Expressions[e].Node != n || !supported(g.Expressions[e].Type, e.Name) || local[n] {
				continue
			}
			count := 0
			for _, own := range functions[target.Decl.Name].Obligations {
				if own.Origin.SemanticNode == n && len(own.Origin.Via) == 0 {
					count++
				}
			}
			if count != 1 {
				continue
			}
			current := root
			for _, call := range path {
				if current.children[call] == nil {
					child := newCandidate(calls[current.f][call])
					child.caller = f.Name
					child.path = append(append([]*parser.GraphNode{}, current.path...), call)
					child.quantified = true
					current.children[call] = child
				}
				current = current.children[call]
			}
			current.instances[n] = o.ID
			proof, err := engine.Implies(f.Name, o.Environment, o.Required)
			if err != nil {
				return err
			}
			if o.Proof.Status == constraints.Proven && o.Proof.Consistent && proof.Proof.Status == constraints.Proven && proof.Proof.Consistent {
				current.checks[n] = true
			}
		}
	}
	return nil
}
func quantifiedDecisions(g *parser.ProgramGraph, report *constraints.Report, routes map[*parser.GraphNode]string, records map[string]*Variant) []QuantifiedDecision {
	out := []QuantifiedDecision{}
	for _, f := range report.Functions {
		for _, d := range f.Dataflow {
			if d.Callback == "" || d.Quantified == nil || d.Quantified.Domain == nil {
				continue
			}
			n := g.Nodes[d.Node]
			target := g.Functions[d.Callback].CName
			removed := 0
			if route := routes[n]; route != "" {
				target = route
				if v := records[route]; v != nil {
					removed = v.Cost.RemovedGuards
				}
			}
			out = append(out, QuantifiedDecision{f.Name, d.Node, d.Quantified.Domain.Node, d.Callback, d.Quantified, target, removed})
		}
	}
	return out
}
