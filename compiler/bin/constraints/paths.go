package constraints

import (
	"sort"

	"leek/bin/parser"
)

func complement(op string) string {
	return map[string]string{"==": "!=", "!=": "==", "<": ">=", "<=": ">", ">": "<=", ">=": "<"}[op]
}
func hull(x, y span) span {
	out := copySpan(x)
	if y.lo.Cmp(out.lo) < 0 {
		out.lo.Set(y.lo)
	}
	if y.hi.Cmp(out.hi) > 0 {
		out.hi.Set(y.hi)
	}
	return out
}

// Join retains only common facts, widens intervals, and ignores terminated or
// demonstrably unreachable predecessors. It never intersects alternatives.
func joinContexts(x, y context) context {
	if !x.consistent {
		return y.clone()
	}
	if !y.consistent {
		return x.clone()
	}
	out := newContext()
	for id, v := range x.ranges {
		if w, ok := y.ranges[id]; ok {
			out.ranges[id] = hull(v, w)
		}
	}
	for id, v := range x.facts {
		if w, ok := y.facts[id]; ok && v == w {
			out.facts[id] = v
		}
	}
	return out
}

type KnownPredicate struct {
	Predicate ID   `json:"predicate"`
	Holds     bool `json:"holds"`
}
type KnownRange struct {
	Value ID        `json:"value"`
	Range *Interval `json:"range"`
}

// Environment is an immutable report snapshot of available premises, not a
// minimal proof core. Reachable means "not demonstrated unreachable".
type Environment struct {
	Domain        string           `json:"domain,omitempty"` // nonempty actual-invocation scope, not function input
	Stage         string           `json:"stage,omitempty"`
	ID            int              `json:"id"`
	PathCondition ID               `json:"path_condition"`
	Reachable     bool             `json:"reachable"`
	Predicates    []KnownPredicate `json:"known_predicates"`
	Ranges        []KnownRange     `json:"known_ranges"`
}

func (s *functionState) snapshot(c context, guard ID) int {
	e := Environment{ID: len(s.report.Environments), PathCondition: guard, Reachable: c.consistent,
		Predicates: []KnownPredicate{}, Ranges: []KnownRange{}}
	ids := []int{}
	for id := range c.facts {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		e.Predicates = append(e.Predicates, KnownPredicate{ID(id), c.facts[ID(id)]})
	}
	ids = nil
	for id := range c.ranges {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		e.Ranges = append(e.Ranges, KnownRange{ID(id), interval(c.ranges[ID(id)])})
	}
	s.report.Environments = append(s.report.Environments, e)
	return e.ID
}
func cloneBindings(env map[*parser.GraphNode]ID) map[*parser.GraphNode]ID {
	out := map[*parser.GraphNode]ID{}
	for k, v := range env {
		out[k] = v
	}
	return out
}

// A single abstract traversal, not path enumeration. Bindings are immutable,
// branches are scoped, and continuation guards explicitly exclude returns.
func (s *functionState) statements(block *parser.BlockStatement, guard ID) ID {
	a := s.b.a
	for _, stmt := range block.Statements {
		switch n := stmt.(type) {
		case *parser.LetStatement:
			s.env[s.b.g.Declarations[n]] = s.expr(n.Expr, guard)
		case *parser.ExpressionStatement:
			s.expr(n.Expr, guard)
		case *parser.ReturnStatement:
			if n.Value != nil {
				id := s.expr(n.Value, guard)
				s.report.Returns = append(s.report.Returns, ValueInfo{Node: s.b.g.Returns[n].ID, Type: s.f.Decl.ReturnType, Relation: id, Guard: guard,
					Range: s.resultRange(id, s.f.Decl.ReturnType, a.refine(s.context, guard)), Scope: "if this return is reached and completes successfully"})
			}
			s.context.consistent = false
			return no
		case *parser.IfStatement:
			condition := s.expr(n.Condition, guard) // requirements here precede branch facts
			incoming, bindings := s.context.clone(), s.env
			s.context = a.refine(incoming, condition)
			s.env = cloneBindings(bindings)
			left := s.statements(n.Then, a.op("and", guard, condition))
			leftContext := s.context
			negated := a.op("not", condition)
			s.context = a.refine(incoming, negated)
			s.env = cloneBindings(bindings)
			right := a.op("and", guard, negated)
			if n.Else != nil {
				right = s.statements(n.Else, right)
			}
			s.context = joinContexts(leftContext, s.context)
			s.env = bindings
			guard = a.op("or", left, right)
		}
	}
	return guard
}
