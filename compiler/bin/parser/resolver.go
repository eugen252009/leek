package parser

import (
	"fmt"
	"strconv"
)

type Symbol struct {
	Name    string
	Type    string
	NodeRef *GraphNode
}

type Environment struct {
	Parent  *Environment
	Symbols map[string]Symbol
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{
		Parent:  parent,
		Symbols: make(map[string]Symbol),
	}
}

func (env *Environment) Lookup(name string) (Symbol, bool) {
	curr := env
	for curr != nil {
		if sym, ok := curr.Symbols[name]; ok {
			return sym, true
		}
		curr = curr.Parent
	}
	return Symbol{}, false
}

type Resolver struct {
	Graph        *ProgramGraph
	CurrentEnv   *Environment
	CurrentScope *GraphNode
}

func NewResolver() *Resolver {
	return &Resolver{
		Graph:      NewProgramGraph(),
		CurrentEnv: NewEnvironment(nil),
	}
}

func (r *Resolver) Resolve(statements []Statement) (*ProgramGraph, error) {
	for _, stmt := range statements {
		if err := r.resolveStatement(stmt); err != nil {
			return nil, err
		}
	}

	if err := r.validateGraphConstraints(); err != nil {
		return nil, err
	}

	return r.Graph, nil
}

func (r *Resolver) resolveStatement(stmt Statement) error {
	switch s := stmt.(type) {
	case *FunctionStatement:
		return r.resolveFunction(s)
	case *LetStatement:
		return r.resolveLet(s)
	default:
		return fmt.Errorf("unknown statement type")
	}
}

func (r *Resolver) addEdge(from, to *GraphNode, t EdgeType) {
	edge := &GraphEdge{
		Type: t,
		From: from,
		To:   to,
	}
	from.Outgoing = append(from.Outgoing, edge)
	to.Incoming = append(to.Incoming, edge)
	r.Graph.Edges = append(r.Graph.Edges, edge)
}

func (r *Resolver) enterScope(node *GraphNode) {
	if r.CurrentScope != nil {
		r.addEdge(r.CurrentScope, node, ControlEdge)
		r.CurrentScope.Children = append(r.CurrentScope.Children, node)
	}
	r.CurrentScope = node
}

func (r *Resolver) exitScope(parent *GraphNode) {
	r.CurrentScope = parent
}

func (r *Resolver) bindSymbol(name string, node *GraphNode) {
	r.CurrentEnv.Symbols[name] = Symbol{
		Name:    name,
		NodeRef: node,
	}
}

func (r *Resolver) useSymbol(name string, consumer *GraphNode) error {
	sym, ok := r.CurrentEnv.Lookup(name)
	if !ok {
		return fmt.Errorf("undefined symbol: %s", name)
	}

	r.addEdge(sym.NodeRef, consumer, DataEdge)
	return nil
}

func (r *Resolver) resolveFunction(fs *FunctionStatement) error {
	if _, exists := r.Graph.Nodes[fs.Name]; exists {
		return sourceError(fs.Pos, "duplicate function %q", fs.Name)
	}
	nodeType := PureScope
	if fs.IsImpure {
		nodeType = ImpureScope
	}

	fnNode := &GraphNode{
		ID:         fs.Name,
		Type:       nodeType,
		Arena:      &ArenaLayout{ScopeID: fs.Name},
		Children:   []*GraphNode{},
		SourceNode: fs,
	}

	r.Graph.Nodes[fs.Name] = fnNode

	oldScope := r.CurrentScope
	oldEnv := r.CurrentEnv

	r.enterScope(fnNode)
	r.CurrentEnv = NewEnvironment(oldEnv)

	for _, p := range fs.Parameters {
		paramID := fs.Name + ":" + p.Name
		paramNode := &GraphNode{
			ID:       paramID,
			Type:     PureScope,
			Children: []*GraphNode{},
		}

		r.Graph.Nodes[paramID] = paramNode

		r.bindSymbol(p.Name, paramNode)
		fnNode.Children = append(fnNode.Children, paramNode)
	}

	for _, stmt := range fs.Body.Statements {
		if err := r.resolveStatement(stmt); err != nil {
			return err
		}
	}

	r.exitScope(oldScope)
	r.CurrentEnv = oldEnv

	return nil
}
func (r *Resolver) resolveLet(ls *LetStatement) error {
	if r.CurrentScope == nil {
		return sourceError(ls.Pos, "declaration %q must be inside a function", ls.Name)
	}
	if _, exists := r.CurrentEnv.Symbols[ls.Name]; exists {
		return sourceError(ls.Pos, "duplicate declaration %q", ls.Name)
	}
	if !validType(ls.Type) {
		return sourceError(ls.Pos, "unknown type %q", ls.Type)
	}
	letNodeID := r.CurrentScope.ID + ":" + ls.Name
	letNode := &GraphNode{
		ID:         letNodeID,
		Type:       PureScope,
		Children:   []*GraphNode{},
		SourceNode: ls,
	}
	r.Graph.Nodes[letNodeID] = letNode
	r.CurrentScope.Children = append(r.CurrentScope.Children, letNode)

	r.addEdge(r.CurrentScope, letNode, ControlEdge)

	if _, err := strconv.ParseInt(ls.Value, 10, 64); err != nil {
		if err := r.useSymbol(ls.Value, letNode); err != nil {
			return sourceError(ls.Pos, "%s", err.Error())
		}
	}

	valueNodeID := letNodeID + ":val:" + ls.Value
	valueNode := &GraphNode{
		ID:         valueNodeID,
		Type:       PureScope,
		Children:   []*GraphNode{},
		SourceNode: ls,
	}
	r.Graph.Nodes[valueNodeID] = valueNode
	r.CurrentScope.Children = append(r.CurrentScope.Children, valueNode)

	r.addEdge(valueNode, letNode, DataEdge)

	r.bindSymbol(ls.Name, letNode)

	return nil
}

func validType(name string) bool {
	switch name {
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "bool":
		return true
	default:
		return false
	}
}

func sourceError(pos SourcePos, format string, args ...interface{}) error {
	return fmt.Errorf("%s:%d:%d: semantic error: %s", pos.Filename, pos.Line, pos.Column, fmt.Sprintf(format, args...))
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

func (r *Resolver) validateGraphConstraints() error {
	return validateNoCycles(r.Graph)
}
