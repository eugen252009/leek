package optimization

import (
	"leek/bin/constraints"
	"leek/bin/parser"
)

const maxFusedStages = 8

type FusionDecision struct {
	Function                    string                `json:"function"`
	Node                        string                `json:"node"`
	Dataflow                    []parser.DataflowKind `json:"dataflow"`
	Optimization                string                `json:"optimization"`
	Decision                    string                `json:"decision"`
	Reason                      string                `json:"reason"`
	Legality                    constraints.Proof     `json:"legality"`
	IntermediateMaterialization bool                  `json:"intermediate_materialization"`
	IteratorStates              bool                  `json:"iterator_states"`
}

// FusionLegality checks trace equivalence, NOT callback totality. Source/setup
// expressions stay once and ordered; every reached callback stays in the same
// element/stage order, so even UNKNOWN arithmetic obligations are preserved.
// No predicate text, cost estimate or required obligation is used as a premise.
func FusionLegality(g *parser.ProgramGraph, e *parser.Expr) constraints.Proof {
	unknown := constraints.Proof{Status: constraints.Unknown, Reason: "unsupported pipeline or execution model", Consistent: true}
	if g == nil || !g.Validated {
		return unknown
	}
	chain := parser.Pipeline(g, e)
	if len(chain) < 3 || len(chain) > maxFusedStages {
		return unknown
	}
	terminal := g.Expressions[e].Dataflow
	if terminal.Kind != parser.Reduce {
		return unknown
	} // Fold remains canonical in this milestone.
	if g.Expressions[chain[0]].Dataflow.Kind != parser.Range {
		return unknown
	}
	for i, stage := range chain {
		info := g.Expressions[stage]
		d := info.Dataflow
		if info.Node == nil || g.Nodes[info.Node.ID] != info.Node || d.Order != parser.ElementMajor || d.Materialization != parser.NoMaterialization {
			return unknown
		}
		if i == 0 {
			continue
		}
		prev := g.Expressions[chain[i-1]].Dataflow
		if d.Input != chain[i-1] || d.InputType != prev.ElementType || d.Callback == nil || d.Callback.Effects != parser.EffectNone {
			return unknown
		}
		if i < len(chain)-1 && d.Kind != parser.Map && d.Kind != parser.Filter {
			return unknown
		}
	}
	return constraints.Proof{Status: constraints.Proven, Reason: "same ascending element-major callback trace, ordered accumulator, and once-only setup; no arithmetic guard removed by fusion", Consistent: true}
}
func fusionPlan(g *parser.ProgramGraph, report *constraints.Report) (map[*parser.GraphNode]bool, []FusionDecision) {
	plan := map[*parser.GraphNode]bool{}
	decisions := []FusionDecision{}
	for _, e := range parser.DataflowTerminals(g) {
		proof := FusionLegality(g, e)
		if report.Limited {
			proof = constraints.Proof{Status: constraints.Unknown, Reason: "analysis resource limit; canonical fallback", Consistent: true}
		}
		decision := FusionDecision{Node: g.Expressions[e].Node.ID, Optimization: "loop_fusion", Decision: "CANONICAL", Reason: proof.Reason, Legality: proof, IteratorStates: true}
		// Function identity is semantic containment, not a split of a diagnostic ID.
		for name, f := range g.Functions {
			for _, c := range f.Calls {
				if c.Expr == e {
					decision.Function = name
				}
			}
		}
		for _, stage := range parser.Pipeline(g, e) {
			decision.Dataflow = append(decision.Dataflow, g.Expressions[stage].Dataflow.Kind)
		}
		if proof.Status == constraints.Proven && proof.Consistent {
			plan[g.Expressions[e].Node] = true
			decision.Decision = "FUSED"
			decision.IteratorStates = false
		}
		decisions = append(decisions, decision)
	}
	return plan, decisions
}
