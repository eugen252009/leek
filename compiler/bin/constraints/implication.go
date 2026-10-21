package constraints

import (
	"fmt"
	"math/big"
	"sort"
)

// IntegerDomain is a canonical inclusive integer interval with finite holes.
// Value is an arena identity, never a spelling. Type retains finite-width meaning.
type IntegerDomain struct {
	Value    ID     `json:"value"`
	Type     string `json:"type"`
	Min, Max string
	Excluded []string `json:"excluded,omitempty"`
}
type NormalizedEnvironment struct {
	Consistent  bool             `json:"consistent"`
	Domains     []IntegerDomain  `json:"domains"`
	Residual    []KnownPredicate `json:"residual_predicates"`
	OtherRanges []KnownRange     `json:"other_ranges"`
}
type Implication struct {
	Proof    Proof                 `json:"proof"`
	Premises NormalizedEnvironment `json:"premises"`
}
type ConstraintRelation struct {
	Forward, Reverse Proof
	Equivalent       bool
	Contradictory    bool
}

// ImplicationEngine owns a bounded copy of one fresh report's term arena. It
// never changes the source report or treats required predicates as premises.
// These query results are advisory; only the optimizer's fresh graph-bound
// analysis may authorize lowering. No deserialized result is a capability.
type ImplicationEngine struct {
	a         *arena
	functions map[string]*FunctionReport
	types     map[ID]string
	limited   bool
}

func NewImplicationEngine(r *Report) (*ImplicationEngine, error) {
	if r == nil || len(r.Terms) < 3 || len(r.Terms) > defaultTermLimit {
		return nil, fmt.Errorf("invalid implication term arena")
	}
	e := &ImplicationEngine{a: newArena(), functions: map[string]*FunctionReport{}, types: map[ID]string{}, limited: r.Limited}
	for i, t := range r.Terms {
		if int(t.ID) != i {
			return nil, fmt.Errorf("noncanonical term identity")
		}
		for _, arg := range t.Args {
			if arg < 0 || int(arg) >= i {
				return nil, fmt.Errorf("invalid term dependency")
			}
		}
		arity := 2
		switch t.Op {
		case "true", "false", "unknown", "var", "opaque", "int":
			arity = 0
		case "not":
			arity = 1
		case "select":
			arity = 3
		case "and", "or":
			arity = len(t.Args)
		case "+", "-", "*", "/", "%", "==", "!=", "<", "<=", ">", ">=":
		default:
			return nil, fmt.Errorf("unsupported term %s", t.Op)
		}
		if len(t.Args) != arity {
			return nil, fmt.Errorf("invalid term arity")
		}
		if t.Op == "int" {
			if n, ok := new(big.Int).SetString(t.Value, 10); !ok || n.BitLen() > maxReasoningBits {
				return nil, fmt.Errorf("invalid integer term")
			}
		}
		if e.a.raw(t.Op, t.Value, t.Args...) != t.ID {
			return nil, fmt.Errorf("noncanonical term arena")
		}
	}
	for _, f := range r.Functions {
		if f == nil || e.functions[f.Name] != nil {
			return nil, fmt.Errorf("invalid function summary identity")
		}
		e.functions[f.Name] = f
		for _, fact := range f.InputFacts {
			if _, ok := integerBounds(fact.Type); ok {
				e.types[fact.Value] = fact.Type
			}
		}
	}
	return e, nil
}

func (e *ImplicationEngine) valid(id ID) bool { return id >= 0 && int(id) < len(e.a.nodes) }
func (e *ImplicationEngine) contextAt(function string, environment int) (context, error) {
	f := e.functions[function]
	if f == nil || environment < 0 || environment >= len(f.Environments) {
		return context{}, fmt.Errorf("unknown implication environment")
	}
	snapshot := f.Environments[environment]
	c := newContext()
	c.consistent = snapshot.Reachable
	for _, r := range snapshot.Ranges {
		if !e.valid(r.Value) || r.Range == nil || r.Range.Empty {
			return context{}, fmt.Errorf("invalid premise range")
		}
		lo, ok := new(big.Int).SetString(r.Range.Min, 10)
		if !ok || lo.BitLen() > maxReasoningBits {
			return context{}, fmt.Errorf("invalid lower bound")
		}
		hi, ok := new(big.Int).SetString(r.Range.Max, 10)
		if !ok || hi.BitLen() > maxReasoningBits {
			return context{}, fmt.Errorf("invalid upper bound")
		}
		c.put(r.Value, span{lo, hi})
	}
	for _, p := range snapshot.Predicates {
		if !e.valid(p.Predicate) {
			return context{}, fmt.Errorf("invalid premise predicate")
		}
		if p.Predicate != unknown {
			c.facts[p.Predicate] = p.Holds
		}
	}
	return c, nil
}

func (e *ImplicationEngine) Implies(function string, environment int, required ...ID) (Implication, error) {
	c, err := e.contextAt(function, environment)
	if err != nil {
		return Implication{}, err
	}
	normalized := e.normalize(&c)
	for _, id := range required {
		if !e.valid(id) {
			return Implication{}, fmt.Errorf("invalid required predicate")
		}
	}
	proof := e.a.prove(e.a.op("and", required...), c)
	if e.limited || e.a.limited {
		proof = Proof{Unknown, "implication resource limit", c.consistent}
	}
	return Implication{proof, normalized}, nil
}

// Compare distinguishes equivalence, one-way implication, contradiction and
// unknown relative to a known environment. Inconsistent premises never yield
// vacuous PROVEN results. Hypothetical premises are used only in this query.
func (e *ImplicationEngine) Compare(function string, environment int, left, right ID) (ConstraintRelation, error) {
	c, err := e.contextAt(function, environment)
	if err != nil {
		return ConstraintRelation{}, err
	}
	if !e.valid(left) || !e.valid(right) {
		return ConstraintRelation{}, fmt.Errorf("invalid relation predicate")
	}
	l, r := e.a.refine(c, left), e.a.refine(c, right)
	e.normalize(&l)
	e.normalize(&r)
	fwd, rev := e.a.prove(right, l), e.a.prove(left, r)
	combined := e.a.refine(l, right)
	e.normalize(&combined)
	out := ConstraintRelation{Forward: fwd, Reverse: rev, Equivalent: fwd.Status == Proven && rev.Status == Proven, Contradictory: c.consistent && !combined.consistent}
	if e.limited || e.a.limited {
		out = ConstraintRelation{Forward: Proof{Unknown, "implication resource limit", l.consistent}, Reverse: Proof{Unknown, "implication resource limit", r.consistent}}
	}
	return out, nil
}

// Normalize only var/constant integer predicates into intervals and exclusions.
// Arithmetic/definedness predicates remain explicit residuals. No approximate
// relation is strengthened, and no required obligation is used here.
func (e *ImplicationEngine) normalize(c *context) NormalizedEnvironment {
	out := NormalizedEnvironment{}
	ids := []ID{}
	for id := range c.ranges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	holes := map[ID]map[string]bool{}
	predicates := []ID{}
	for id := range c.facts {
		predicates = append(predicates, id)
	}
	sort.Slice(predicates, func(i, j int) bool { return predicates[i] < predicates[j] })
	for _, id := range predicates {
		t := e.a.nodes[id]
		if isComparison(t.Op) && e.types[t.Args[0]] != "" && e.a.nodes[t.Args[0]].Op == "var" {
			if value, ok := e.a.constant(t.Args[1]); ok {
				if _, bounded := c.ranges[t.Args[0]]; bounded {
					op := t.Op
					if !c.facts[id] {
						op = complement(op)
					}
					if op == "!=" {
						if holes[t.Args[0]] == nil {
							holes[t.Args[0]] = map[string]bool{}
						}
						holes[t.Args[0]][value.String()] = true
					}
					continue
				}
			}
		}
		out.Residual = append(out.Residual, KnownPredicate{id, c.facts[id]})
	}
	for _, id := range ids {
		r := copySpan(c.ranges[id])
		if typ := e.types[id]; typ != "" && e.a.nodes[id].Op == "var" {
			for r.lo.Cmp(r.hi) <= 0 && holes[id][r.lo.String()] {
				r.lo.Add(r.lo, big.NewInt(1))
			}
			for r.lo.Cmp(r.hi) <= 0 && holes[id][r.hi.String()] {
				r.hi.Sub(r.hi, big.NewInt(1))
			}
			c.put(id, r)
			d := IntegerDomain{Value: id, Type: typ, Min: r.lo.String(), Max: r.hi.String()}
			values := []*big.Int{}
			for h := range holes[id] {
				n, _ := new(big.Int).SetString(h, 10)
				if n.Cmp(r.lo) >= 0 && n.Cmp(r.hi) <= 0 {
					values = append(values, n)
				}
			}
			sort.Slice(values, func(i, j int) bool { return values[i].Cmp(values[j]) < 0 })
			for _, n := range values {
				d.Excluded = append(d.Excluded, n.String())
			}
			out.Domains = append(out.Domains, d)
		} else {
			out.OtherRanges = append(out.OtherRanges, KnownRange{id, interval(r)})
		}
	}
	out.Consistent = c.consistent
	return out
}
