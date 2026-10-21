package constraints

import (
	"math/big"
	"strconv"
)

type Status string

const (
	Proven   Status = "PROVEN"
	Unknown  Status = "UNKNOWN"
	Violated Status = "VIOLATED"
)

type Proof struct {
	Status     Status `json:"status"`
	Reason     string `json:"reason"`
	Consistent bool   `json:"assumptions_consistent"`
}

// Interval is an inclusive interval. Empty means no successful value was
// established; it must NEVER be used to prove an arbitrary obligation.
type Interval struct {
	Min   string `json:"min,omitempty"`
	Max   string `json:"max,omitempty"`
	Empty bool   `json:"empty,omitempty"`
}
type span struct{ lo, hi *big.Int }

func copySpan(s span) span      { return span{new(big.Int).Set(s.lo), new(big.Int).Set(s.hi)} }
func interval(s span) *Interval { return &Interval{Min: s.lo.String(), Max: s.hi.String()} }
func integerBounds(typ string) (span, bool) {
	switch typ {
	case "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64":
	default:
		return span{}, false
	}
	bits, _ := strconv.Atoi(typ[1:])
	hi := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	lo := new(big.Int)
	if typ[0] == 'i' {
		hi.Rsh(hi, 1)
		lo.Neg(new(big.Int).Set(hi))
	}
	hi.Sub(hi, big.NewInt(1))
	return span{lo, hi}, true
}

const maxReasoningBits = 4096

type context struct {
	ranges     map[ID]span
	facts      map[ID]bool // established predicates, never required obligations
	consistent bool
}

func newContext() context {
	return context{ranges: map[ID]span{}, facts: map[ID]bool{}, consistent: true}
}
func (c context) clone() context {
	n := newContext()
	n.consistent = c.consistent
	for id, value := range c.facts {
		n.facts[id] = value
	}
	for k, v := range c.ranges {
		n.ranges[k] = copySpan(v)
	}
	return n
}
func (c *context) put(id ID, s span) {
	if old, ok := c.ranges[id]; ok {
		if old.lo.Cmp(s.lo) > 0 {
			s.lo = new(big.Int).Set(old.lo)
		}
		if old.hi.Cmp(s.hi) < 0 {
			s.hi = new(big.Int).Set(old.hi)
		}
	}
	c.ranges[id] = s
	if s.lo.Cmp(s.hi) > 0 {
		c.consistent = false
	}
}

// Refine is used only with established assumptions, or in a separately labelled
// success-conditional output context. Required conditions never enter the
// context used to classify their own proof status.
func (a *arena) refine(c context, assumption ID) context {
	n := c.clone()
	// A branch fact exists only after its condition has evaluated successfully.
	// OR is retained as an alternative, never split into simultaneous facts.
	if a.prove(assumption, n).Status == Violated {
		n.consistent = false
		return n
	}
	terms := []ID{assumption}
	if a.nodes[assumption].Op == "and" {
		terms = a.nodes[assumption].Args
	}
	if assumption == no {
		n.consistent = false
		return n
	}
	for _, id := range terms {
		if id != unknown && len(n.facts) < 256 {
			n.facts[id] = true
			if opposite := a.op("not", id); opposite != unknown {
				n.facts[opposite] = false
			}
		}
		t := a.nodes[id]
		if !isComparison(t.Op) {
			continue
		}
		s, ok := n.ranges[t.Args[0]]
		if !ok {
			continue
		}
		v, ok := a.constant(t.Args[1])
		if !ok {
			continue
		}
		s = copySpan(s)
		switch t.Op {
		case ">":
			v.Add(v, big.NewInt(1))
			fallthrough
		case ">=":
			if v.Cmp(s.lo) > 0 {
				s.lo = v
			}
		case "<":
			v.Sub(v, big.NewInt(1))
			fallthrough
		case "<=":
			if v.Cmp(s.hi) < 0 {
				s.hi = v
			}
		case "==":
			if v.Cmp(s.lo) > 0 {
				s.lo = new(big.Int).Set(v)
			}
			if v.Cmp(s.hi) < 0 {
				s.hi = v
			}
		case "!=":
			if v.Cmp(s.lo) == 0 {
				s.lo.Add(s.lo, big.NewInt(1))
			}
			if v.Cmp(s.hi) == 0 {
				s.hi.Sub(s.hi, big.NewInt(1))
			}
		}
		n.put(t.Args[0], s)
	}
	return n
}

type reasoner struct {
	a         *arena
	c         context
	intervals map[ID]*span
	proofs    map[ID]Status
}

func (a *arena) reasoner(c context) *reasoner {
	return &reasoner{a: a, c: c, intervals: map[ID]*span{}, proofs: map[ID]Status{}}
}
func (a *arena) prove(id ID, c context) Proof {
	if !c.consistent {
		return Proof{Unknown, "contradictory incoming assumptions; no vacuous proof", false}
	}
	status := a.reasoner(c).truth(id)
	reason := "unsupported relation, reasoning bound, or overlapping intervals; obligation is not assumed"
	if status == Proven {
		reason = "constant, logical identity, or exact interval bound from known facts"
	}
	if status == Violated {
		reason = "condition is false for all values in the consistent known domain"
	}
	return Proof{status, reason, true}
}
func (r *reasoner) truth(id ID) Status {
	if holds, ok := r.c.facts[id]; ok && id != unknown {
		if holds {
			return Proven
		}
		return Violated
	}
	if s, ok := r.proofs[id]; ok {
		return s
	}
	t := r.a.nodes[id]
	s := Unknown
	switch t.Op {
	case "true":
		s = Proven
	case "false":
		s = Violated
	case "not":
		s = r.truth(t.Args[0])
		if s == Proven {
			s = Violated
		} else if s == Violated {
			s = Proven
		}
	case "and", "or":
		s = Proven
		if t.Op == "or" {
			s = Violated
		}
		for _, arg := range t.Args {
			v := r.truth(arg)
			if t.Op == "and" && v == Violated {
				s = Violated
				break
			}
			if t.Op == "or" && v == Proven {
				s = Proven
				break
			}
			if v == Unknown {
				s = Unknown
			}
		}
	case "==", "!=", "<", "<=", ">", ">=":
		x, y := r.rangeOf(t.Args[0]), r.rangeOf(t.Args[1])
		if x != nil && y != nil {
			switch t.Op {
			case "<":
				if x.hi.Cmp(y.lo) < 0 {
					s = Proven
				} else if x.lo.Cmp(y.hi) >= 0 {
					s = Violated
				}
			case "<=":
				if x.hi.Cmp(y.lo) <= 0 {
					s = Proven
				} else if x.lo.Cmp(y.hi) > 0 {
					s = Violated
				}
			case ">":
				if x.lo.Cmp(y.hi) > 0 {
					s = Proven
				} else if x.hi.Cmp(y.lo) <= 0 {
					s = Violated
				}
			case ">=":
				if x.lo.Cmp(y.hi) >= 0 {
					s = Proven
				} else if x.hi.Cmp(y.lo) < 0 {
					s = Violated
				}
			case "==", "!=":
				if x.lo.Cmp(x.hi) == 0 && y.lo.Cmp(y.hi) == 0 && x.lo.Cmp(y.lo) == 0 {
					s = Proven
				} else if x.hi.Cmp(y.lo) < 0 || y.hi.Cmp(x.lo) < 0 {
					s = Violated
				}
				if t.Op == "!=" {
					if s == Proven {
						s = Violated
					} else if s == Violated {
						s = Proven
					}
				}
			}
		}
	}
	r.proofs[id] = s
	return s
}
func (r *reasoner) rangeOf(id ID) *span {
	if v, ok := r.intervals[id]; ok {
		return v
	}
	t := r.a.nodes[id]
	var out *span
	if s, ok := r.c.ranges[id]; ok && s.lo.Cmp(s.hi) <= 0 {
		v := copySpan(s)
		out = &v
	} else {
		switch t.Op {
		case "int":
			n := integer(t.Value)
			out = &span{n, new(big.Int).Set(n)}
		case "true":
			out = &span{big.NewInt(1), big.NewInt(1)}
		case "false":
			out = &span{big.NewInt(0), big.NewInt(0)}
		case "select":
			// No speculative evaluation: this is only a mathematical relation.
			left := r.a.refine(r.c, t.Args[0])
			right := r.a.refine(r.c, r.a.op("not", t.Args[0]))
			var x, y *span
			if left.consistent {
				x = r.a.reasoner(left).rangeOf(t.Args[1])
			}
			if right.consistent {
				y = r.a.reasoner(right).rangeOf(t.Args[2])
			}
			if !left.consistent {
				out = y
			} else if !right.consistent {
				out = x
			} else if x != nil && y != nil {
				z := hull(*x, *y)
				out = &z
			}
		case "+", "-", "*", "/", "%":
			x, y := r.rangeOf(t.Args[0]), r.rangeOf(t.Args[1])
			if x != nil && y != nil {
				out = arithmeticRange(t.Op, *x, *y)
			}
		}
	}
	r.intervals[id] = out
	return out
}
func arithmeticRange(op string, x, y span) *span {
	// Exact bounded-resource reasoning: decline rather than overflow or allocate
	// enormous integers for a computation that may already fail at runtime.
	for _, v := range []*big.Int{x.lo, x.hi, y.lo, y.hi} {
		if v.BitLen() > maxReasoningBits {
			return nil
		}
	}
	out := &span{new(big.Int), new(big.Int)}
	switch op {
	case "+":
		out.lo.Add(x.lo, y.lo)
		out.hi.Add(x.hi, y.hi)
	case "-":
		out.lo.Sub(x.lo, y.hi)
		out.hi.Sub(x.hi, y.lo)
	case "*", "/":
		if op == "/" && y.lo.Sign() <= 0 && y.hi.Sign() >= 0 {
			return nil
		}
		vals := []*big.Int{}
		for _, a := range []*big.Int{x.lo, x.hi} {
			for _, b := range []*big.Int{y.lo, y.hi} {
				if op == "*" && a.BitLen()+b.BitLen() > maxReasoningBits {
					return nil
				}
				n := new(big.Int)
				if op == "*" {
					n.Mul(a, b)
				} else {
					n.Quo(a, b)
				}
				vals = append(vals, n)
			}
		}
		out.lo.Set(vals[0])
		out.hi.Set(vals[0])
		for _, n := range vals[1:] {
			if n.Cmp(out.lo) < 0 {
				out.lo.Set(n)
			}
			if n.Cmp(out.hi) > 0 {
				out.hi.Set(n)
			}
		}
	case "%":
		if y.lo.Sign() <= 0 && y.hi.Sign() >= 0 {
			return nil
		}
		if x.lo.Cmp(x.hi) == 0 && y.lo.Cmp(y.hi) == 0 {
			out.lo.Rem(x.lo, y.lo)
			out.hi.Set(out.lo)
			return out
		}
		d := new(big.Int).Abs(y.lo)
		h := new(big.Int).Abs(y.hi)
		if h.Cmp(d) > 0 {
			d = h
		}
		d.Sub(d, big.NewInt(1))
		if x.lo.Sign() < 0 {
			out.lo.Neg(d)
		}
		if x.hi.Sign() > 0 {
			out.hi.Set(d)
		}
	default:
		return nil
	}
	return out
}

// simplifyKnown removes only predicates proven from incoming facts. It never
// uses the requested precondition itself as a premise.
func (a *arena) simplifyKnown(id ID, c context) ID {
	t := a.nodes[id]
	if t.Op == "and" {
		args := []ID{}
		for _, i := range t.Args {
			args = append(args, a.simplifyKnown(i, c))
		}
		return a.op("and", args...)
	}
	switch a.prove(id, c).Status {
	case Proven:
		return yes
	case Violated:
		return no
	}
	return id
}
