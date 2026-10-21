// Package optimization consumes fresh analysis results without modifying semantic
// IR. Canonical lowering remains parser.CodeGenerator.Generate.
package optimization

import (
	"leek/bin/constraints"
	"leek/bin/parser"
)

// Elision identifies an exact local operation and the premise snapshot used.
// These records are diagnostics, not reusable authorization tokens.
type Elision struct {
	Function    string
	Node        string
	Obligation  int
	Environment int
}

// Generate analyzes and immediately lowers the SAME validated graph. It accepts
// neither external proofs nor cached plans: proof validity cannot outlive this
// call. As for canonical generation, callers must not mutate the graph concurrently.
func Generate(g *parser.ProgramGraph) (string, []Elision, error) {
	report, err := constraints.Analyze(g)
	if err != nil {
		return "", nil, err
	}
	proven := map[*parser.GraphNode]bool{}
	elisions := []Elision{}
	// Limits or partial summaries fail closed, not open. Each supported unsigned
	// binary operation has exactly one safety obligation in the current semantics.
	if !report.Limited {
		for _, f := range report.Functions {
			if !f.Pure || !f.Complete {
				continue
			}
			for _, o := range f.Obligations {
				if o.Proof.Status != constraints.Proven || !o.Proof.Consistent || len(o.Origin.Via) != 0 || o.Origin.Function != f.Name {
					continue
				}
				node := o.Origin.SemanticNode
				if node == nil || g.Nodes[node.ID] != node {
					continue
				}
				expr, ok := node.SourceNode.(*parser.Expr)
				if !ok || expr.Kind != "binary" {
					continue
				}
				info := g.Expressions[expr]
				if info == nil || info.Node != node || !supported(info.Type, expr.Name) {
					continue
				}
				// Require the operation's entire local contract, not an arbitrary proven
				// fragment. Recheck cardinality defensively against future analyzer changes.
				count := 0
				for _, other := range f.Obligations {
					if other.Origin.SemanticNode == node && len(other.Origin.Via) == 0 {
						count++
					}
				}
				if count != 1 {
					continue
				}
				proven[node] = true
				elisions = append(elisions, Elision{f.Name, node.ID, o.ID, o.Environment})
			}
		}
	}
	code, err := parser.NewCodeGenerator().GenerateWithProvenChecks(g, proven)
	return code, elisions, err
}
func supported(typ, op string) bool {
	switch typ {
	case "u8", "u16", "u32", "u64":
	default:
		return false
	}
	switch op {
	case "+", "-", "*", "/", "%":
		return true
	}
	return false
}
