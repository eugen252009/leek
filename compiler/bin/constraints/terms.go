// Package constraints performs read-only analysis of a validated Leek graph.
// It does not rewrite execution IR. A separate opt-in consumer may use proofs.
package constraints

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

type ID int

// Term is a node of a mathematical DAG, not a runtime expression. Integers are
// decimal strings so JSON consumers cannot accidentally round u64 bounds.
// Args reference Report.Terms by ID. Checked definedness is tracked separately.
type Term struct {
	ID    ID     `json:"id"`
	Op    string `json:"op"`
	Value string `json:"value,omitempty"`
	Args  []ID   `json:"args,omitempty"`
}

const (
	yes ID = iota
	no
	unknown
)
const defaultTermLimit = 50000

type arena struct {
	nodes   []Term
	index   map[string]ID
	limit   int
	limited bool
}

func newArena() *arena {
	a := &arena{index: map[string]ID{}, limit: defaultTermLimit}
	a.raw("true", "")
	a.raw("false", "")
	a.raw("unknown", "analysis limit")
	return a
}
func (a *arena) raw(op, value string, args ...ID) ID {
	key := op + ":" + strconv.Quote(value)
	for _, v := range args {
		key += ":" + strconv.Itoa(int(v))
	}
	if id, ok := a.index[key]; ok {
		return id
	}
	if len(a.nodes) >= a.limit {
		a.limited = true
		return unknown
	}
	id := ID(len(a.nodes))
	a.nodes = append(a.nodes, Term{ID: id, Op: op, Value: value, Args: append([]ID(nil), args...)})
	a.index[key] = id
	return id
}
func integer(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("invalid internal integer: " + s)
	}
	return n
}
func (a *arena) number(n *big.Int) ID { return a.raw("int", n.String()) }
func (a *arena) num(n int64) ID       { return a.number(big.NewInt(n)) }
func (a *arena) constant(id ID) (*big.Int, bool) {
	t := a.nodes[id]
	if t.Op == "int" {
		return integer(t.Value), true
	}
	return nil, false
}
func (a *arena) total(id ID) bool {
	// Division and opaque/resource-limited expressions are not algebraically
	// cancellable unless their definedness has independently been established.
	memo := map[ID]bool{}
	var visit func(ID) bool
	visit = func(i ID) bool {
		if b, ok := memo[i]; ok {
			return b
		}
		t := a.nodes[i]
		b := t.Op != "/" && t.Op != "%" && t.Op != "unknown" && t.Op != "opaque"
		if b {
			for _, v := range t.Args {
				if !visit(v) {
					b = false
					break
				}
			}
		}
		memo[i] = b
		return b
	}
	return visit(id)
}
func (a *arena) op(op string, args ...ID) ID {
	if op == "select" {
		if args[0] == yes {
			return args[1]
		}
		if args[0] == no {
			return args[2]
		}
		if args[1] == args[2] {
			return args[1]
		}
		return a.raw(op, "", args...)
	}
	if op == "and" || op == "or" {
		return a.logic(op, args)
	}
	if op == "not" {
		switch args[0] {
		case yes:
			return no
		case no:
			return yes
		case unknown:
			return unknown
		}
		t := a.nodes[args[0]]
		if t.Op == "not" {
			return t.Args[0]
		}
		if inverse := complement(t.Op); inverse != "" {
			return a.op(inverse, t.Args...)
		}
		if t.Op == "and" || t.Op == "or" {
			negated := make([]ID, len(t.Args))
			for i, arg := range t.Args {
				negated[i] = a.op("not", arg)
			}
			other := "and"
			if t.Op == "and" {
				other = "or"
			}
			return a.op(other, negated...)
		}
		return a.raw(op, "", args...)
	}
	x, y := args[0], args[1]
	nx, cx := a.constant(x)
	ny, cy := a.constant(y)
	if cx && cy && nx.BitLen()+ny.BitLen() <= maxReasoningBits {
		n := new(big.Int)
		switch op {
		case "+":
			return a.number(n.Add(nx, ny))
		case "-":
			return a.number(n.Sub(nx, ny))
		case "*":
			return a.number(n.Mul(nx, ny))
		case "/":
			if ny.Sign() != 0 {
				return a.number(n.Quo(nx, ny))
			}
		case "%":
			if ny.Sign() != 0 {
				return a.number(n.Rem(nx, ny))
			}
		default:
			if isComparison(op) {
				if compare(op, nx.Cmp(ny)) {
					return yes
				}
				return no
			}
		}
	}
	if x == y && a.total(x) {
		switch op {
		case "-":
			return a.num(0)
		case "==", "<=", ">=":
			return yes
		case "!=", "<", ">":
			return no
		}
	}
	if cy && ny.Sign() == 0 {
		if op == "+" || op == "-" {
			return x
		}
		if op == "*" && a.total(x) {
			return y
		}
	}
	if cx && nx.Sign() == 0 && op == "+" {
		return y
	}
	if cy && ny.Cmp(big.NewInt(1)) == 0 && (op == "*" || op == "/") {
		return x
	}
	if isComparison(op) {
		// Only affine integer relations are rewritten; prior checked obligations
		// remain independently recorded. No inverse of a general function is used.
		if cx && !cy {
			return a.op(reverse(op), y, x)
		}
		if cy {
			v, coef, offset, ok := a.affine(x)
			if ok && coef.Sign() != 0 {
				rhs := new(big.Int).Sub(ny, offset)
				if coef.Sign() < 0 {
					coef.Neg(coef)
					rhs.Neg(rhs)
					op = reverse(op)
				}
				if coef.Cmp(big.NewInt(1)) == 0 {
					return a.raw(op, "", v, a.number(rhs))
				}
				switch op {
				case "==", "!=":
					q, r := new(big.Int), new(big.Int)
					q.QuoRem(rhs, coef, r)
					if r.Sign() != 0 {
						if op == "==" {
							return no
						}
						return yes
					}
					return a.raw(op, "", v, a.number(q))
				case "<=":
					return a.raw("<=", "", v, a.number(new(big.Int).Div(rhs, coef)))
				case "<":
					rhs.Sub(rhs, big.NewInt(1))
					return a.raw("<=", "", v, a.number(new(big.Int).Div(rhs, coef)))
				case ">=":
					rhs.Neg(rhs)
					rhs.Div(rhs, coef)
					rhs.Neg(rhs)
					return a.raw(">=", "", v, a.number(rhs))
				case ">":
					rhs.Div(rhs, coef)
					rhs.Add(rhs, big.NewInt(1))
					return a.raw(">=", "", v, a.number(rhs))
				}
			}
		}
	}
	return a.raw(op, "", args...)
}
func isComparison(op string) bool {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	}
	return false
}
func reverse(op string) string {
	switch op {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	}
	return op
}
func compare(op string, c int) bool {
	switch op {
	case "==":
		return c == 0
	case "!=":
		return c != 0
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	}
	return false
}

// affine recognizes at most one variable and a*x+b. DAG memoization avoids
// repeated traversals of shared relations such as repeated square compositions.
type affineForm struct {
	v    ID
	c, b *big.Int
	ok   bool
}

func (a *arena) affine(id ID) (ID, *big.Int, *big.Int, bool) {
	memo := map[ID]affineForm{}
	var visit func(ID) affineForm
	visit = func(i ID) affineForm {
		if r, ok := memo[i]; ok {
			return r
		}
		t := a.nodes[i]
		r := affineForm{}
		switch t.Op {
		case "var":
			r = affineForm{i, big.NewInt(1), big.NewInt(0), true}
		case "int":
			r = affineForm{0, big.NewInt(0), integer(t.Value), true}
		case "+", "-", "*":
			x, y := visit(t.Args[0]), visit(t.Args[1])
			if x.ok && y.ok {
				if t.Op == "*" {
					if x.c.Sign() == 0 {
						x, y = y, x
					}
					if y.c.Sign() == 0 && x.c.BitLen()+y.b.BitLen() <= maxReasoningBits && x.b.BitLen()+y.b.BitLen() <= maxReasoningBits {
						r = affineForm{x.v, new(big.Int).Mul(x.c, y.b), new(big.Int).Mul(x.b, y.b), true}
					}
				} else if x.c.Sign() == 0 || y.c.Sign() == 0 || x.v == y.v {
					v := x.v
					if x.c.Sign() == 0 {
						v = y.v
					}
					c, b := new(big.Int), new(big.Int)
					if t.Op == "+" {
						c.Add(x.c, y.c)
						b.Add(x.b, y.b)
					} else {
						c.Sub(x.c, y.c)
						b.Sub(x.b, y.b)
					}
					r = affineForm{v, c, b, true}
				}
			}
		}
		if r.ok && (r.c.BitLen() > maxReasoningBits || r.b.BitLen() > maxReasoningBits) {
			r.ok = false
		}
		memo[i] = r
		return r
	}
	r := visit(id)
	if !r.ok {
		return 0, nil, nil, false
	}
	return r.v, new(big.Int).Set(r.c), new(big.Int).Set(r.b), true
}
func (a *arena) logic(op string, args []ID) ID {
	flat := []ID{}
	seen := map[ID]bool{}
	var add func(ID) bool
	add = func(i ID) bool {
		if (op == "and" && i == no) || (op == "or" && i == yes) {
			return false
		}
		if (op == "and" && i == yes) || (op == "or" && i == no) {
			return true
		}
		if a.nodes[i].Op == op {
			for _, v := range a.nodes[i].Args {
				if !add(v) {
					return false
				}
			}
			return true
		}
		if !seen[i] {
			seen[i] = true
			flat = append(flat, i)
		}
		return true
	}
	for _, i := range args {
		if !add(i) {
			if op == "and" {
				return no
			}
			return yes
		}
	}
	for _, i := range flat {
		t := a.nodes[i]
		if t.Op == "not" && seen[t.Args[0]] && a.total(t.Args[0]) {
			if op == "and" {
				return no
			}
			return yes
		}
	}
	if op == "and" {
		var ok bool
		flat, ok = a.tighten(flat)
		if !ok {
			return no
		}
	}
	sort.Slice(flat, func(i, j int) bool { return flat[i] < flat[j] })
	if len(flat) == 0 {
		if op == "and" {
			return yes
		}
		return no
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return a.raw(op, "", flat...)
}

// tighten only merges bounds/excluded points of the SAME symbolic variable.
// It never promotes these requirements to known incoming facts.
type bounds struct {
	lo, hi             *big.Int
	strictLo, strictHi bool
	excluded           map[string]bool
}

func (a *arena) tighten(ids []ID) ([]ID, bool) {
	groups := map[ID]*bounds{}
	rest := []ID{}
	for _, i := range ids {
		t := a.nodes[i]
		if !isComparison(t.Op) || a.nodes[t.Args[0]].Op != "var" {
			rest = append(rest, i)
			continue
		}
		n, ok := a.constant(t.Args[1])
		if !ok {
			rest = append(rest, i)
			continue
		}
		v := t.Args[0]
		b := groups[v]
		if b == nil {
			b = &bounds{excluded: map[string]bool{}}
			groups[v] = b
		}
		switch t.Op {
		case "!=":
			b.excluded[n.String()] = true
		case ">", ">=":
			strict := t.Op == ">"
			if strict {
				n.Add(n, big.NewInt(1))
			}
			if b.lo == nil || n.Cmp(b.lo) > 0 {
				b.lo = n
				b.strictLo = strict
			}
		case "<", "<=":
			strict := t.Op == "<"
			if strict {
				n.Sub(n, big.NewInt(1))
			}
			if b.hi == nil || n.Cmp(b.hi) < 0 {
				b.hi = n
				b.strictHi = strict
			}
		case "==":
			if b.lo == nil || n.Cmp(b.lo) > 0 {
				b.lo = new(big.Int).Set(n)
				b.strictLo = false
			}
			if b.hi == nil || n.Cmp(b.hi) < 0 {
				b.hi = new(big.Int).Set(n)
				b.strictHi = false
			}
		}
	}
	vars := []ID{}
	for v := range groups {
		vars = append(vars, v)
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i] < vars[j] })
	for _, v := range vars {
		b := groups[v]
		for b.lo != nil && b.excluded[b.lo.String()] {
			delete(b.excluded, b.lo.String())
			b.lo.Add(b.lo, big.NewInt(1))
			b.strictLo = true
		}
		for b.hi != nil && b.excluded[b.hi.String()] {
			delete(b.excluded, b.hi.String())
			b.hi.Sub(b.hi, big.NewInt(1))
			b.strictHi = true
		}
		if b.lo != nil && b.hi != nil && b.lo.Cmp(b.hi) > 0 {
			return nil, false
		}
		if b.lo != nil && b.hi != nil && b.lo.Cmp(b.hi) == 0 {
			rest = append(rest, a.raw("==", "", v, a.number(b.lo)))
			continue
		}
		if b.lo != nil {
			op := ">="
			n := new(big.Int).Set(b.lo)
			if b.strictLo {
				op = ">"
				n.Sub(n, big.NewInt(1))
			}
			rest = append(rest, a.raw(op, "", v, a.number(n)))
		}
		if b.hi != nil {
			op := "<="
			n := new(big.Int).Set(b.hi)
			if b.strictHi {
				op = "<"
				n.Add(n, big.NewInt(1))
			}
			rest = append(rest, a.raw(op, "", v, a.number(n)))
		}
		excluded := []string{}
		for s := range b.excluded {
			excluded = append(excluded, s)
		}
		sort.Strings(excluded)
		for _, s := range excluded {
			n := integer(s)
			if (b.lo == nil || n.Cmp(b.lo) >= 0) && (b.hi == nil || n.Cmp(b.hi) <= 0) {
				rest = append(rest, a.raw("!=", "", v, a.number(n)))
			}
		}
	}
	return rest, true
}
func (a *arena) substitute(id ID, values map[ID]ID, memo map[ID]ID) ID {
	if id == unknown {
		return unknown
	}
	if v, ok := values[id]; ok {
		return v
	}
	if v, ok := memo[id]; ok {
		return v
	}
	t := a.nodes[id]
	if len(t.Args) == 0 {
		return id
	}
	args := make([]ID, len(t.Args))
	for i, v := range t.Args {
		args[i] = a.substitute(v, values, memo)
	}
	result := a.op(t.Op, args...)
	memo[id] = result
	return result
}
func (a *arena) text(id ID) string {
	// Rendering is bounded independently of the compact DAG size.
	remaining := 150
	var render func(ID) string
	render = func(i ID) string {
		remaining--
		if remaining < 0 {
			return fmt.Sprintf("term#%d", i)
		}
		t := a.nodes[i]
		switch t.Op {
		case "var", "opaque":
			return t.Value
		case "int":
			return t.Value
		case "true", "false":
			return t.Op
		case "unknown":
			return "UNKNOWN"
		case "not":
			return "NOT(" + render(t.Args[0]) + ")"
		}
		parts := []string{}
		for _, v := range t.Args {
			parts = append(parts, render(v))
		}
		return "(" + strings.Join(parts, " "+t.Op+" ") + ")"
	}
	return render(id)
}
