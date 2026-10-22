package parser

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

type Symbol struct {
	Name, Type string
	NodeRef    *GraphNode
	Function   *FunctionInfo // Block reads are local; cross-function captures are not.
}
type Environment struct {
	Parent  *Environment
	Symbols map[string]Symbol
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{Parent: parent, Symbols: map[string]Symbol{}}
}
func (e *Environment) Lookup(name string) (Symbol, bool) {
	for ; e != nil; e = e.Parent {
		if s, ok := e.Symbols[name]; ok {
			return s, true
		}
	}
	return Symbol{}, false
}

type Resolver struct {
	Graph        *ProgramGraph
	CurrentEnv   *Environment
	CurrentScope *GraphNode
	current      *FunctionInfo
	order        []*FunctionInfo
	exprOrder    map[*FunctionInfo][]*Expr
	serial       int
	printFn      *FunctionInfo
}

func NewResolver() *Resolver {
	g := NewProgramGraph()
	g.Functions = map[string]*FunctionInfo{}
	g.Expressions = map[*Expr]*ExprInfo{}
	g.Declarations = map[*LetStatement]*GraphNode{}
	g.Returns = map[*ReturnStatement]*GraphNode{}
	g.Blocks = map[*BlockStatement]*GraphNode{}
	return &Resolver{Graph: g, CurrentEnv: NewEnvironment(nil), exprOrder: map[*FunctionInfo][]*Expr{}, printFn: &FunctionInfo{Decl: &FunctionStatement{Name: "print", IsImpure: true}, Effects: EffectIO, MayFail: true, Builtin: true}}
}
func (r *Resolver) Resolve(statements []Statement) (*ProgramGraph, error) {
	r.Graph.Validated = false
	// Collect signatures before bodies: forward calls are supported, unknown
	// functions never become implicitly pure external declarations.
	if err := r.collect(statements, nil); err != nil {
		return nil, err
	}
	for _, s := range statements {
		f, ok := s.(*FunctionStatement)
		if !ok {
			return nil, sourceError(statementPos(s), "declarations and expressions must be inside a function")
		}
		if err := r.resolveFunction(r.Graph.Functions[f.Name]); err != nil {
			return nil, err
		}
	}
	state := map[*FunctionInfo]int{}
	var visit func(*FunctionInfo) error
	visit = func(f *FunctionInfo) error {
		if f.Builtin {
			return nil
		}
		if state[f] == 1 {
			return sourceError(f.Decl.Pos, "recursive calls are not yet supported (function %q)", f.Decl.Name)
		}
		if state[f] == 2 {
			return nil
		}
		state[f] = 1
		if f.Decl.IsImpure {
			f.Effects |= EffectBoundary
		}
		for _, c := range f.Calls {
			if err := visit(c.Callee); err != nil {
				return err
			}
			f.Effects |= c.Callee.Effects
			f.MayFail = f.MayFail || c.Callee.MayFail
		}
		state[f] = 2
		return nil
	}
	for _, f := range r.order {
		if err := visit(f); err != nil {
			return nil, err
		}
	}
	// Callees have complete summaries, including transitive effects and failures.
	for _, f := range r.order {
		var lastEffect *GraphNode
		for _, e := range r.exprOrder[f] {
			info := r.Graph.Expressions[e]
			for _, child := range exprChildren(e) {
				ci := r.Graph.Expressions[child]
				if ci == nil && info.Constant != "" {
					continue
				}
				info.Effects |= ci.Effects
				info.MayFail = info.MayFail || ci.MayFail
			}
			if info.Dataflow != nil && info.Dataflow.Callback != nil {
				callback := info.Dataflow.Callback
				if callback.Effects != EffectNone {
					return nil, sourceError(e.Pos, "dataflow %s requires a transitively pure callback; %q is effectful", info.Dataflow.Kind, callback.Decl.Name)
				}
				info.StageEffects = callback.Effects
				info.MayFail = info.MayFail || callback.MayFail
			}
			if info.Callee != nil {
				info.Effects |= info.Callee.Effects
				info.MayFail = info.MayFail || info.Callee.MayFail
				if e.Kind == "composition" {
					info.StageEffects = info.Callee.Effects
					if info.StageEffects != EffectNone {
						return nil, sourceError(e.Pos, "impure function %q cannot be used in a pure composition pipeline; '->' only accepts pure stages", e.Name)
					}
				}
				if info.Callee.Effects != EffectNone {
					if lastEffect != nil {
						r.addEdge(lastEffect, info.Node, EffectEdge)
					}
					lastEffect = info.Node
				}
			}
			if info.Effects != EffectNone {
				info.Node.Type = ImpureScope
			}
		}
	}
	for _, f := range r.order {
		if !f.Decl.IsImpure && f.Effects != EffectNone {
			for _, c := range f.Calls {
				if c.Callee.Effects != EffectNone {
					return nil, sourceError(c.Expr.Pos, "pure function %q cannot call effectful function %q; use an impure boundary", f.Decl.Name, c.Callee.Decl.Name)
				}
			}
		}
	}
	if err := validateNoCycles(r.Graph); err != nil {
		return nil, err
	}
	r.Graph.Validated = true
	return r.Graph, nil
}
func statementPos(s Statement) SourcePos {
	switch s := s.(type) {
	case *LetStatement:
		return s.Pos
	case *IfStatement:
		return s.Pos
	case *FunctionStatement:
		return s.Pos
	case *ReturnStatement:
		return s.Pos
	case *ExpressionStatement:
		return s.Pos
	}
	return SourcePos{}
}
func (r *Resolver) collect(ss []Statement, parent *FunctionInfo) error {
	for _, s := range ss {
		f, ok := s.(*FunctionStatement)
		if !ok {
			continue
		}
		if _, ok := r.Graph.Functions[f.Name]; ok || f.Name == "print" || collectionName(f.Name) {
			return sourceError(f.Pos, "duplicate or reserved function %q", f.Name)
		}
		if f.ReturnType != "" && !validType(f.ReturnType) {
			return sourceError(f.Pos, "unknown return type %q", f.ReturnType)
		}
		if f.Name == "main" && (parent != nil || len(f.Parameters) != 0 || f.ReturnType != "") {
			return sourceError(f.Pos, "entry function main must be top-level, have no parameters and no result")
		}
		node := &GraphNode{ID: f.Name, Type: PureScope, SourceNode: f, Arena: &ArenaLayout{ScopeID: f.Name}}
		if f.IsImpure {
			node.Type = ImpureScope
		}
		info := &FunctionInfo{Decl: f, Node: node, Parent: parent, CName: fmt.Sprintf("lk_fn_%d", len(r.order))}
		r.Graph.Functions[f.Name] = info
		r.Graph.Nodes[f.Name] = node
		r.order = append(r.order, info)
		if parent != nil {
			parent.Node.Children = append(parent.Node.Children, node)
		}
		if f.Body == nil {
			return sourceError(f.Pos, "function %q has no body", f.Name)
		}
		if err := r.collect(f.Body.Statements, info); err != nil {
			return err
		}
	}
	return nil
}
func (r *Resolver) addEdge(from, to *GraphNode, t EdgeType) {
	e := &GraphEdge{Type: t, From: from, To: to}
	from.Outgoing = append(from.Outgoing, e)
	to.Incoming = append(to.Incoming, e)
	r.Graph.Edges = append(r.Graph.Edges, e)
}
func (r *Resolver) resolveFunction(f *FunctionInfo) error {
	oldEnv, oldScope, oldFn := r.CurrentEnv, r.CurrentScope, r.current
	r.CurrentEnv = NewEnvironment(oldEnv)
	r.CurrentScope = f.Node
	r.current = f
	defer func() { r.CurrentEnv = oldEnv; r.CurrentScope = oldScope; r.current = oldFn }()
	for _, p := range f.Decl.Parameters {
		pos := p.Pos
		if pos.Line == 0 {
			pos = f.Decl.Pos
		}
		if _, ok := r.CurrentEnv.Symbols[p.Name]; ok {
			return sourceError(pos, "duplicate parameter %q", p.Name)
		}
		if !validType(p.Type) {
			return sourceError(pos, "unknown type %q for parameter %q", p.Type, p.Name)
		}
		node := &GraphNode{ID: f.Decl.Name + ":" + p.Name, Type: PureScope}
		r.Graph.Nodes[node.ID] = node
		f.Node.Children = append(f.Node.Children, node)
		r.CurrentEnv.Symbols[p.Name] = Symbol{Name: p.Name, Type: p.Type, NodeRef: node, Function: f}
	}
	r.Graph.Blocks[f.Decl.Body] = f.Node
	returned, err := r.resolveStatements(f.Decl.Body)
	if err != nil {
		return err
	}
	if f.Decl.ReturnType != "" && !returned {
		return sourceError(f.Decl.Pos, "function %q must return %s", f.Decl.Name, f.Decl.ReturnType)
	}
	return nil
}
func (r *Resolver) resolveStatements(block *BlockStatement) (bool, error) {
	f := r.current
	returned := false
	for _, stmt := range block.Statements {
		if nested, ok := stmt.(*FunctionStatement); ok {
			if r.CurrentScope != f.Node {
				return false, sourceError(nested.Pos, "function declarations inside conditional blocks are not yet supported")
			}
			if err := r.resolveFunction(r.Graph.Functions[nested.Name]); err != nil {
				return false, err
			}
			continue
		}
		if returned {
			return false, sourceError(statementPos(stmt), "unreachable statement after return")
		}
		switch s := stmt.(type) {
		case *LetStatement:
			if _, ok := r.CurrentEnv.Symbols[s.Name]; ok {
				return false, sourceError(s.Pos, "duplicate declaration %q", s.Name)
			}
			if !validType(s.Type) {
				return false, sourceError(s.Pos, "unknown type %q", s.Type)
			}
			if s.Expr == nil { // Compatibility with programmatically constructed legacy ASTs.
				kind := "name"
				if _, ok := new(big.Int).SetString(s.Value, 10); ok || s.Value == "true" || s.Value == "false" {
					kind = "literal"
				}
				s.Expr = &Expr{Kind: kind, Name: s.Value, Pos: s.Pos}
			}
			info, err := r.resolveExpr(s.Expr, s.Type)
			if err != nil {
				// Preserve declaration-level diagnostics for existing name initializers.
				if s.Expr.Kind == "name" {
					return false, sourceError(s.Pos, "%s", semanticMessage(err))
				}
				return false, err
			}
			node := &GraphNode{ID: r.CurrentScope.ID + ":" + s.Name, Type: PureScope, SourceNode: s}
			r.Graph.Nodes[node.ID] = node
			r.CurrentScope.Children = append(r.CurrentScope.Children, node)
			r.Graph.Declarations[s] = node
			r.addEdge(info.Node, node, DataEdge)
			r.CurrentEnv.Symbols[s.Name] = Symbol{Name: s.Name, Type: s.Type, NodeRef: node, Function: f}
		case *ReturnStatement:
			if f.Decl.ReturnType == "" {
				if s.Value != nil {
					return false, sourceError(s.Pos, "function %q has no result; use bare return", f.Decl.Name)
				}
			} else {
				if s.Value == nil {
					return false, sourceError(s.Pos, "return requires a value of type %s", f.Decl.ReturnType)
				}
				info, err := r.resolveExpr(s.Value, f.Decl.ReturnType)
				if err != nil {
					return false, err
				}
				node := &GraphNode{ID: r.CurrentScope.ID + ":$return", Type: PureScope, SourceNode: s}
				r.Graph.Nodes[node.ID] = node
				r.CurrentScope.Children = append(r.CurrentScope.Children, node)
				r.Graph.Returns[s] = node
				r.addEdge(info.Node, node, DataEdge)
			}
			returned = true
		case *ExpressionStatement:
			if _, err := r.resolveExpr(s.Expr, ""); err != nil {
				return false, err
			}
		case *IfStatement:
			condition, err := r.resolveExpr(s.Condition, "bool")
			if err != nil {
				return false, err
			}
			left, err := r.resolveBranch(s.Then, condition.Node)
			if err != nil {
				return false, err
			}
			right := false
			if s.Else != nil {
				right, err = r.resolveBranch(s.Else, condition.Node)
				if err != nil {
					return false, err
				}
			}
			returned = left && right
		default:
			return false, sourceError(statementPos(stmt), "unsupported statement")
		}
	}
	return returned, nil
}
func (r *Resolver) resolveBranch(block *BlockStatement, condition *GraphNode) (bool, error) {
	if block == nil {
		return false, sourceError(r.current.Decl.Pos, "conditional branch has no body")
	}
	oldEnv, oldScope := r.CurrentEnv, r.CurrentScope
	defer func() { r.CurrentEnv, r.CurrentScope = oldEnv, oldScope }()
	r.serial++
	node := &GraphNode{ID: fmt.Sprintf("%s:$block%d", r.current.Decl.Name, r.serial), Type: oldScope.Type, SourceNode: block}
	node.Arena = &ArenaLayout{ScopeID: node.ID}
	r.Graph.Nodes[node.ID] = node
	r.Graph.Blocks[block] = node
	oldScope.Children = append(oldScope.Children, node)
	r.addEdge(condition, node, ControlEdge)
	r.CurrentEnv, r.CurrentScope = NewEnvironment(oldEnv), node
	return r.resolveStatements(block)
}
func semanticMessage(err error) string {
	_, s, ok := strings.Cut(err.Error(), "semantic error: ")
	if ok {
		return s
	}
	return err.Error()
}
func validType(t string) bool {
	switch t {
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "bool":
		return true
	}
	return false
}
func integerType(t string) bool { return len(t) > 1 && (t[0] == 'u' || t[0] == 'i') && validType(t) }
func sourceError(pos SourcePos, format string, args ...interface{}) error {
	return fmt.Errorf("%s:%d:%d: semantic error: %s", pos.Filename, pos.Line, pos.Column, fmt.Sprintf(format, args...))
}
func exprChildren(e *Expr) []*Expr {
	if e.Kind == "composition" && collectionName(e.Name) {
		out := []*Expr{e.Left}
		if (e.Name == "reduce" || e.Name == "fold") && len(e.Args) > 0 {
			out = append(out, e.Args[0])
		}
		return out
	}
	out := []*Expr{}
	if e.Left != nil {
		out = append(out, e.Left)
	}
	if e.Right != nil {
		out = append(out, e.Right)
	}
	return append(out, e.Args...)
}
func (r *Resolver) function(name string, pos SourcePos) (*FunctionInfo, error) {
	if _, ok := r.CurrentEnv.Lookup(name); ok {
		return nil, sourceError(pos, "%q is a value; indirect calls are not supported", name)
	}
	if name == "print" {
		return r.printFn, nil
	}
	f := r.Graph.Functions[name]
	if f == nil {
		return nil, sourceError(pos, "unknown function %q; external and indirect calls are not supported", name)
	}
	if f.Parent != nil {
		visible := false
		for scope := r.current; scope != nil; scope = scope.Parent {
			if scope == f.Parent {
				visible = true
			}
		}
		if !visible {
			return nil, sourceError(pos, "function %q is outside its lexical scope", name)
		}
	}
	return f, nil
}

// Hints come only from already typed bindings/signatures, never from C coercions.
func (r *Resolver) hint(e *Expr) string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case "name":
		if s, ok := r.CurrentEnv.Lookup(e.Name); ok {
			return s.Type
		}
	case "literal":
		if e.Name == "true" || e.Name == "false" {
			return "bool"
		}
	case "call", "composition":
		if (e.Name == "reduce" || e.Name == "fold") && len(e.Args) == 2 && e.Args[1].Kind == "name" {
			if f := r.Graph.Functions[e.Args[1].Name]; f != nil {
				return f.Decl.ReturnType
			}
		}
		if f := r.Graph.Functions[e.Name]; f != nil {
			return f.Decl.ReturnType
		}
	case "unary":
		if e.Name == "!" {
			return "bool"
		}
		return r.hint(e.Left)
	case "binary":
		if precedence(e.Name) <= 5 {
			return "bool"
		}
		if h := r.hint(e.Left); h != "" {
			return h
		}
		return r.hint(e.Right)
	}
	return ""
}
func (r *Resolver) resolveExpr(e *Expr, expected string) (*ExprInfo, error) {
	info := &ExprInfo{}
	fail := func(format string, args ...interface{}) (*ExprInfo, error) {
		return nil, sourceError(e.Pos, format, args...)
	}
	if expected == "$sequence" && !((e.Kind == "call" || e.Kind == "composition") && collectionName(e.Name)) {
		return fail("dataflow stage requires a sequence input")
	}
	switch e.Kind {
	case "literal":
		typ := expected
		if typ == "" {
			typ = "u32"
		}
		if e.Name == "true" || e.Name == "false" {
			typ = "bool"
			info.Constant = "0"
			if e.Name == "true" {
				info.Constant = "1"
			}
		} else {
			constant, err := numericConstant(e.Name, typ)
			if err != nil {
				return fail("%s", err)
			}
			info.Constant = constant
		}
		info.Type = typ
	case "name":
		s, ok := r.CurrentEnv.Lookup(e.Name)
		if !ok {
			return fail("undefined symbol: %s", e.Name)
		}
		if s.Function != r.current {
			return fail("capture of outer variable %q is not yet supported; pass it as a parameter", e.Name)
		}
		info.Type = s.Type
		info.Binding = s.NodeRef
	case "unary":
		typ := expected
		if e.Name == "!" {
			typ = "bool"
		} else if typ == "" {
			typ = r.hint(e.Left)
			if typ == "" {
				typ = "i32"
			}
		}
		// Negative literal magnitudes may exceed the positive signed range by one.
		if e.Name == "-" && e.Left.Kind == "literal" && e.Left.Name != "true" && e.Left.Name != "false" {
			c, err := numericConstant("-"+e.Left.Name, typ)
			if err != nil {
				return fail("%s", err)
			}
			info.Type = typ
			info.Constant = c
		} else {
			a, err := r.resolveExpr(e.Left, typ)
			if err != nil {
				return nil, err
			}
			if e.Name != "!" && !integerType(a.Type) {
				return fail("unary %s requires an integer", e.Name)
			}
			info.Type = a.Type
			info.MayFail = e.Name == "-"
		}
	case "binary":
		logical := e.Name == "&&" || e.Name == "||"
		comparison := precedence(e.Name) == 4 || precedence(e.Name) == 5
		typ := expected
		if logical {
			typ = "bool"
		} else if comparison {
			typ = ""
		}
		if typ == "" {
			typ = r.hint(e.Left)
			if typ == "" {
				typ = r.hint(e.Right)
			}
			if typ == "" {
				typ = "u32"
			}
		}
		a, err := r.resolveExpr(e.Left, typ)
		if err != nil {
			return nil, err
		}
		b, err := r.resolveExpr(e.Right, typ)
		if err != nil {
			return nil, err
		}
		if a.Type != b.Type {
			return fail("operands have different types")
		}
		if !logical && !comparison && !integerType(typ) {
			return fail("arithmetic currently requires integer operands, got %s", typ)
		}
		if comparison && typ == "bool" && e.Name != "==" && e.Name != "!=" {
			return fail("bool supports only == and != comparisons")
		}
		info.Type = typ
		if logical || comparison {
			info.Type = "bool"
		} else {
			info.MayFail = true
		}
	case "call", "composition":
		if collectionName(e.Name) {
			if err := r.resolveDataflow(e, info); err != nil {
				return nil, err
			}
			break
		}
		callee, err := r.function(e.Name, e.Pos)
		if err != nil {
			return nil, err
		}
		info.Callee = callee
		if e.Kind == "composition" && callee.Decl.IsImpure {
			return fail("impure function %q cannot be used in a pure composition pipeline; '->' only accepts pure stages", e.Name)
		}
		args := e.Args
		if e.Kind == "composition" {
			args = append([]*Expr{e.Left}, args...)
		}
		if callee.Builtin {
			if len(args) != 1 {
				return fail("print expects one scalar argument")
			}
			a, err := r.resolveExpr(args[0], "")
			if err != nil {
				return nil, err
			}
			if a.Type == "" {
				return fail("print requires a scalar value")
			}
		} else {
			if len(args) != len(callee.Decl.Parameters) {
				return fail("function %q expects %d arguments, got %d", e.Name, len(callee.Decl.Parameters), len(args))
			}
			for i, arg := range args {
				if _, err := r.resolveExpr(arg, callee.Decl.Parameters[i].Type); err != nil {
					return nil, err
				}
			}
		}
		info.Type = callee.Decl.ReturnType
		if e.Kind == "composition" && info.Type == "" && !callee.Decl.IsImpure {
			return fail("composition stage %q must return a scalar value", e.Name)
		}
		r.current.Calls = append(r.current.Calls, CallSite{Expr: e, Callee: callee})
	default:
		return fail("unsupported expression %q", e.Kind)
	}
	if isSequence(info.Type) && expected != "$sequence" {
		return fail("sequence must be consumed inline by reduce or fold")
	}
	if expected == "$sequence" {
		if !isSequence(info.Type) {
			return fail("dataflow stage requires a sequence input")
		}
	} else if expected != "" && info.Type != expected {
		return fail("type mismatch: expected %s, got %s", expected, typeLabel(info.Type))
	}
	// Constant unary expressions deliberately retain their source subtree; it is
	// not separately evaluated, since its positive magnitude may be out of range.
	children := exprChildren(e)
	if info.Constant != "" {
		children = nil
	}
	for _, child := range children {
		c := r.Graph.Expressions[child]
		info.MayFail = info.MayFail || c.MayFail
	}
	if info.MayFail {
		r.current.MayFail = true
	}
	r.serial++
	node := &GraphNode{ID: fmt.Sprintf("%s:$expr%d", r.current.Decl.Name, r.serial), Type: PureScope, SourceNode: e}
	info.Node = node
	r.Graph.Nodes[node.ID] = node
	r.CurrentScope.Children = append(r.CurrentScope.Children, node)
	for _, child := range children {
		r.addEdge(r.Graph.Expressions[child].Node, node, DataEdge)
	}
	if info.Binding != nil {
		r.addEdge(info.Binding, node, DataEdge)
	}
	if info.Dataflow != nil && info.Dataflow.Reference != nil {
		r.addEdge(r.Graph.Expressions[info.Dataflow.Reference].Node, node, DataEdge)
	}
	r.Graph.Expressions[e] = info
	r.exprOrder[r.current] = append(r.exprOrder[r.current], e)
	return info, nil
}
func typeLabel(t string) string {
	if t == "" {
		return "no result"
	}
	return t
}
func numericConstant(text, typ string) (string, error) {
	// Decimal only: leading zeroes stay decimal, never acquire C octal semantics.
	digits := strings.TrimPrefix(text, "-")
	for _, c := range digits {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("only decimal integer literals are supported")
		}
	}
	n, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return "", fmt.Errorf("invalid integer literal %q", text)
	}
	if integerType(typ) {
		bits, _ := strconv.Atoi(typ[1:])
		max := new(big.Int).Lsh(big.NewInt(1), uint(bits))
		min := new(big.Int)
		if typ[0] == 'i' {
			max.Rsh(max, 1)
			min.Neg(new(big.Int).Set(max))
		}
		max.Sub(max, big.NewInt(1))
		if n.Cmp(min) < 0 || n.Cmp(max) > 0 {
			return "", fmt.Errorf("literal %s is out of range for %s", text, typ)
		}
		return n.String(), nil
	}
	if typ == "f32" || typ == "f64" {
		bits, _ := strconv.Atoi(typ[1:])
		if _, err := strconv.ParseFloat(n.String(), bits); err != nil {
			return "", fmt.Errorf("literal out of range for %s", typ)
		}
		return n.String(), nil
	}
	return "", fmt.Errorf("integer literal cannot have type %s", typ)
}
func validateNoCycles(g *ProgramGraph) error {
	indegree := map[*GraphNode]int{}
	for _, e := range g.Edges {
		indegree[e.To]++
	}
	queue := []*GraphNode{}
	for _, n := range g.Nodes {
		if indegree[n] == 0 {
			queue = append(queue, n)
		}
	}
	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++
		for _, e := range n.Outgoing {
			indegree[e.To]--
			if indegree[e.To] == 0 {
				queue = append(queue, e.To)
			}
		}
	}
	if visited != len(g.Nodes) {
		return fmt.Errorf("cycle detected in program graph (visited %d of %d nodes)", visited, len(g.Nodes))
	}
	return nil
}
func (r *Resolver) validateGraphConstraints() error { return validateNoCycles(r.Graph) }
func sortedFunctionNodes(g *ProgramGraph) []*GraphNode {
	nodes := []*GraphNode{}
	for _, n := range g.Nodes {
		if _, ok := n.SourceNode.(*FunctionStatement); ok {
			nodes = append(nodes, n)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}
