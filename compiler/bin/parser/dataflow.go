package parser

import "fmt"

// DataflowKind describes dependency structure, not a name-matched runtime call.
type DataflowKind string

const (
	Range  DataflowKind = "Range"
	Map    DataflowKind = "Map"
	Filter DataflowKind = "Filter"
	Reduce DataflowKind = "Reduce"
	Fold   DataflowKind = "Fold"
)
const ElementMajor = "ascending-element-major-v1"
const NoMaterialization = "none"

type DataflowOp struct {
	Kind                              DataflowKind
	Input                             *Expr
	Start, End                        *Expr
	Initial                           *Expr
	Callback                          *FunctionInfo
	Reference                         *Expr
	InputType, ElementType, StateType string
	Order                             string
	Materialization                   string
}

func collectionName(name string) bool {
	switch name {
	case "range", "map", "filter", "reduce", "fold":
		return true
	}
	return false
}
func isSequence(typ string) bool { return len(typ) > 4 && typ[:4] == "seq:" }
func (r *Resolver) resolveDataflow(e *Expr, info *ExprInfo) error {
	fail := func(format string, args ...interface{}) error { return sourceError(e.Pos, format, args...) }
	if _, ok := r.CurrentEnv.Lookup(e.Name); ok {
		return fail("intrinsic %q is shadowed by a value", e.Name)
	}
	d := &DataflowOp{Order: ElementMajor, Materialization: NoMaterialization}
	info.Dataflow = d
	if e.Name == "range" {
		if e.Kind != "call" || len(e.Args) != 2 {
			return fail("range requires range(start, end)")
		}
		typ := r.hint(e.Args[0])
		if typ == "" {
			typ = r.hint(e.Args[1])
		}
		if typ == "" {
			typ = "u32"
		}
		if !integerType(typ) {
			return fail("range bounds must have matching integer types")
		}
		if _, err := r.resolveExpr(e.Args[0], typ); err != nil {
			return err
		}
		if _, err := r.resolveExpr(e.Args[1], typ); err != nil {
			return err
		}
		d.Kind, d.Start, d.End, d.ElementType = Range, e.Args[0], e.Args[1], typ
		info.Type = "seq:" + typ
		return nil
	}
	if e.Kind != "composition" {
		return fail("%s is only supported as a sequence composition stage", e.Name)
	}
	count := 1
	if e.Name == "reduce" || e.Name == "fold" {
		count = 2
	}
	if len(e.Args) != count {
		return fail("%s expects %d stage arguments", e.Name, count)
	}
	input, err := r.resolveExpr(e.Left, "$sequence")
	if err != nil {
		return err
	}
	d.Input = e.Left
	d.InputType = input.Dataflow.ElementType
	d.ElementType = d.InputType
	ref := e.Args[count-1]
	if ref.Kind != "name" {
		return fail("%s requires a statically known function name", e.Name)
	}
	callback, err := r.function(ref.Name, ref.Pos)
	if err != nil {
		return err
	}
	if callback.Builtin || callback.Decl.IsImpure {
		return fail("%s requires a pure scalar callback", e.Name)
	}
	d.Callback, d.Reference = callback, ref
	params := callback.Decl.Parameters
	switch e.Name {
	case "map":
		d.Kind = Map
		if len(params) != 1 || params[0].Type != d.InputType || !validType(callback.Decl.ReturnType) {
			return fail("map callback must accept (%s) and return a scalar", d.InputType)
		}
		d.ElementType = callback.Decl.ReturnType
		info.Type = "seq:" + d.ElementType
	case "filter":
		d.Kind = Filter
		if len(params) != 1 || params[0].Type != d.InputType || callback.Decl.ReturnType != "bool" {
			return fail("filter callback must accept (%s) and return bool", d.InputType)
		}
		info.Type = "seq:" + d.ElementType
	case "reduce", "fold":
		d.Kind = Reduce
		if e.Name == "fold" {
			d.Kind = Fold
		}
		if len(params) != 2 || params[1].Type != d.InputType || callback.Decl.ReturnType != params[0].Type {
			return fail("%s callback must accept (state, %s) and return state", e.Name, d.InputType)
		}
		d.StateType = params[0].Type
		if d.Kind == Reduce && d.StateType != d.InputType {
			return fail("reduce requires state and element types to match; use fold for different state types")
		}
		if _, err := r.resolveExpr(e.Args[0], d.StateType); err != nil {
			return err
		}
		d.Initial = e.Args[0]
		info.Type = d.StateType
	}
	// A callback dependency participates in transitive effect/cycle analysis, but
	// is NOT a direct scalar call with the descriptor as its argument.
	r.current.Calls = append(r.current.Calls, CallSite{Expr: e, Callee: callback})
	r.serial++
	node := &GraphNode{ID: fmt.Sprintf("%s:$ref%d", r.current.Decl.Name, r.serial), Type: PureScope, SourceNode: ref}
	r.Graph.Nodes[node.ID] = node
	r.CurrentScope.Children = append(r.CurrentScope.Children, node)
	r.Graph.Expressions[ref] = &ExprInfo{Type: "$function", Reference: callback, Node: node}
	return nil
}

// Pipeline returns the semantic chain in execution order, including its terminal.
func Pipeline(g *ProgramGraph, e *Expr) []*Expr {
	if e == nil || g.Expressions[e] == nil || g.Expressions[e].Dataflow == nil {
		return nil
	}
	d := g.Expressions[e].Dataflow
	out := Pipeline(g, d.Input)
	return append(out, e)
}
