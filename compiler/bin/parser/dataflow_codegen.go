package parser

import (
	"fmt"
	"sort"
)

func DataflowTerminals(g *ProgramGraph) []*Expr {
	out := []*Expr{}
	for e, i := range g.Expressions {
		if i.Dataflow != nil && (i.Dataflow.Kind == Reduce || i.Dataflow.Kind == Fold) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return g.Expressions[out[i]].Node.ID < g.Expressions[out[j]].Node.ID })
	return out
}
func (cg *CodeGenerator) iteratorDefinitions() {
	cg.iterators = map[*Expr]string{}
	for _, terminal := range DataflowTerminals(cg.graph) {
		if cg.fused[cg.graph.Expressions[terminal].Node] {
			continue
		}
		chain := Pipeline(cg.graph, terminal)
		for _, e := range chain[:len(chain)-1] {
			if cg.iterators[e] != "" {
				continue
			}
			name := fmt.Sprintf("lk_iter_%d", len(cg.iterators))
			cg.iterators[e] = name
			d := cg.graph.Expressions[e].Dataflow
			ct := cg.mapType(d.ElementType)
			cg.line("// canonical pull %s: %s", cg.graph.Expressions[e].Node.ID, d.Kind)
			if d.Kind == Range {
				cg.line("typedef struct { %s current, end; } %s_state;", ct, name)
				cg.line("static _Bool %s_next(%s_state* s, %s* out) {", name, name, ct)
				cg.line("    if (s->current >= s->end) return 0;")
				cg.line("    *out = s->current;")
				cg.line("    ++s->current; // current < end <= MAX, cannot overflow")
				cg.line("    return 1;")
			} else {
				prev := cg.iterators[d.Input]
				cg.line("typedef struct { %s_state* input; } %s_state;", prev, name)
				cg.line("static _Bool %s_next(%s_state* s, %s* out) {", name, name, ct)
				cg.line("    %s element;", cg.mapType(d.InputType))
				if d.Kind == Map {
					cg.line("    if (!%s_next(s->input, &element)) return 0;", prev)
					cg.line("    *out = %s(element);", d.Callback.CName)
					cg.line("    return 1;")
				} else {
					cg.line("    while (%s_next(s->input, &element)) {", prev)
					cg.line("        if (%s(element)) { *out = element; return 1; }", d.Callback.CName)
					cg.line("    }")
					cg.line("    return 0;")
				}
			}
			cg.line("}")
		}
	}
}
func (cg *CodeGenerator) fresh(prefix string) string {
	name := fmt.Sprintf("lk_%s%d", prefix, cg.serial)
	cg.serial++
	return name
}
func (cg *CodeGenerator) dataflow(e *Expr) string {
	chain := Pipeline(cg.graph, e)
	terminal := cg.graph.Expressions[e].Dataflow
	if terminal.Kind != Reduce && terminal.Kind != Fold {
		panic("unconsumed validated sequence")
	}
	domain := cg.graph.Expressions[chain[0]].Dataflow
	// Setup follows source order. No callback is invoked before initial succeeds.
	start := cg.expr(domain.Start)
	end := cg.expr(domain.End)
	if cg.fused[cg.graph.Expressions[e].Node] {
		return cg.fusedDataflow(e, chain, start, end)
	}
	cg.line("// dataflow %s: canonical pull; no intermediate arrays", cg.graph.Expressions[e].Node.ID)
	previous := ""
	for _, stage := range chain[:len(chain)-1] {
		state := cg.fresh("state")
		name := cg.iterators[stage]
		if previous == "" {
			cg.line("%s_state %s = { %s, %s };", name, state, start, end)
		} else {
			cg.line("%s_state %s = { &%s };", name, state, previous)
		}
		previous = state
	}
	initial := cg.expr(terminal.Initial)
	acc := cg.temp(terminal.StateType, initial)
	element := cg.fresh("element")
	cg.line("%s %s;", cg.mapType(terminal.InputType), element)
	last := cg.iterators[terminal.Input]
	cg.line("while (%s_next(&%s, &%s)) {", last, previous, element)
	cg.indent += "    "
	cg.line("%s = %s(%s, %s);", acc, terminal.Callback.CName, acc, element)
	cg.indent = cg.indent[:len(cg.indent)-4]
	cg.line("}")
	return acc
}

// Fused lowering is intentionally separate from the pull implementation. It
// preserves callback order and does not consult scalar failure-safety proofs.
func (cg *CodeGenerator) fusedDataflow(e *Expr, chain []*Expr, start, end string) string {
	terminal := cg.graph.Expressions[e].Dataflow
	domain := cg.graph.Expressions[chain[0]].Dataflow
	cg.line("// dataflow %s: fused element-major loop; no iterator state or arrays", cg.graph.Expressions[e].Node.ID)
	initial := cg.expr(terminal.Initial)
	acc := cg.temp(terminal.StateType, initial)
	index := cg.fresh("index")
	cg.line("for (%s %s = %s; %s < %s; ++%s) {", cg.mapType(domain.ElementType), index, start, index, end, index)
	cg.indent += "    "
	element := index
	for _, stage := range chain[1 : len(chain)-1] {
		d := cg.graph.Expressions[stage].Dataflow
		if d.Kind == Map {
			element = cg.temp(d.ElementType, d.Callback.CName+"("+element+")")
		} else {
			cg.line("if (!%s(%s)) continue;", d.Callback.CName, element)
		}
	}
	cg.line("%s = %s(%s, %s);", acc, terminal.Callback.CName, acc, element)
	cg.indent = cg.indent[:len(cg.indent)-4]
	cg.line("}")
	return acc
}
