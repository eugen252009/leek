package constraints

import (
	"fmt"
	"math/big"
	"testing"
)

// CA-PROOF: this evaluator is a test oracle, not part of the simplifier/prover.
// It uses exact arithmetic and explicit partial division. A nil result means
// undefined/unsupported, never true.
func evaluate(nodes []Term, id ID, env map[string]*big.Int) *big.Int {
	memo := map[ID]*big.Int{}
	var eval func(ID) *big.Int
	eval = func(i ID) *big.Int {
		if v, ok := memo[i]; ok {
			return v
		}
		t := nodes[i]
		var out *big.Int
		switch t.Op {
		case "true":
			out = big.NewInt(1)
		case "false":
			out = big.NewInt(0)
		case "int":
			out = new(big.Int)
			out.SetString(t.Value, 10)
		case "var":
			out = env[t.Value]
		case "and", "or":
			sawUnknown := false
			out = big.NewInt(1)
			if t.Op == "or" {
				out = big.NewInt(0)
			}
			decided := false
			for _, arg := range t.Args {
				v := eval(arg)
				if v == nil {
					sawUnknown = true
					continue
				}
				if t.Op == "and" && v.Sign() == 0 {
					out = big.NewInt(0)
					decided = true
					break
				}
				if t.Op == "or" && v.Sign() != 0 {
					out = big.NewInt(1)
					decided = true
					break
				}
			}
			if sawUnknown && !decided {
				out = nil
			}
		case "select":
			if condition := eval(t.Args[0]); condition != nil {
				branch := t.Args[2]
				if condition.Sign() != 0 {
					branch = t.Args[1]
				}
				out = eval(branch)
			}
		case "not":
			if x := eval(t.Args[0]); x != nil {
				out = big.NewInt(0)
				if x.Sign() == 0 {
					out.SetInt64(1)
				}
			}
		default:
			if len(t.Args) != 2 {
				break
			}
			x, y := eval(t.Args[0]), eval(t.Args[1])
			if x == nil || y == nil {
				break
			}
			out = new(big.Int)
			switch t.Op {
			case "+":
				out.Add(x, y)
			case "-":
				out.Sub(x, y)
			case "*":
				out.Mul(x, y)
			case "/":
				if y.Sign() == 0 {
					out = nil
				} else {
					out.Quo(x, y)
				}
			case "%":
				if y.Sign() == 0 {
					out = nil
				} else {
					out.Rem(x, y)
				}
			case "==":
				if x.Cmp(y) == 0 {
					out.SetInt64(1)
				}
			case "!=":
				if x.Cmp(y) != 0 {
					out.SetInt64(1)
				}
			case "<":
				if x.Cmp(y) < 0 {
					out.SetInt64(1)
				}
			case "<=":
				if x.Cmp(y) <= 0 {
					out.SetInt64(1)
				}
			case ">":
				if x.Cmp(y) > 0 {
					out.SetInt64(1)
				}
			case ">=":
				if x.Cmp(y) >= 0 {
					out.SetInt64(1)
				}
			default:
				out = nil
			}
		}
		memo[i] = out
		return out
	}
	return eval(id)
}
func TestSimplificationAndContradictions(t *testing.T) {
	a := newArena()
	x := a.raw("var", "x")
	condition := a.op("and", a.op(">=", x, a.num(1)), a.op("!=", a.op("-", x, a.num(1)), a.num(0)))
	if text := a.text(condition); text != "(x > 1)" {
		t.Fatal(text)
	}
	redundant := a.op("and", a.op(">=", x, a.num(0)), condition)
	if a.text(redundant) != "(x > 1)" {
		t.Fatal(a.text(redundant))
	}
	impossible := a.op("and", a.op("==", x, a.num(5)), a.op(">", x, a.num(10)))
	ctx := newContext()
	ctx.put(x, span{big.NewInt(0), big.NewInt(255)})
	if p := a.prove(impossible, ctx); p.Status != Violated || !p.Consistent {
		t.Fatalf("contradictory requirement: %+v", p)
	}
	inconsistent := a.refine(ctx, impossible)
	for _, goal := range []ID{yes, no, condition} {
		if p := a.prove(goal, inconsistent); p.Status != Unknown || p.Consistent {
			t.Fatalf("vacuous proof from impossible facts: %+v", p)
		}
	}
	equalFive := a.refine(ctx, a.op("==", x, a.num(5)))
	if p := a.prove(a.op(">", x, a.num(10)), equalFive); p.Status != Violated {
		t.Fatal(p)
	}
	// An unsupported nonlinear relationship remains UNKNOWN, not a failed proof
	// reported as a universal violation.
	if p := a.prove(a.op("<=", a.op("*", x, x), a.num(100)), ctx); p.Status != Unknown {
		t.Fatal(p)
	}
}
func TestAffineRewritesExhaustiveSmallDomain(t *testing.T) {
	for coef := int64(-4); coef <= 4; coef++ {
		if coef == 0 {
			continue
		}
		for offset := int64(-3); offset <= 3; offset++ {
			for bound := int64(-5); bound <= 5; bound++ {
				for _, op := range []string{"==", "!=", "<", "<=", ">", ">="} {
					a := newArena()
					x := a.raw("var", "x")
					term := a.op(op, a.op("+", a.op("*", a.num(coef), x), a.num(offset)), a.num(bound))
					for v := int64(-10); v <= 10; v++ {
						got := evaluate(a.nodes, term, map[string]*big.Int{"x": big.NewInt(v)})
						lhs := coef*v + offset
						want := false
						switch op {
						case "==":
							want = lhs == bound
						case "!=":
							want = lhs != bound
						case "<":
							want = lhs < bound
						case "<=":
							want = lhs <= bound
						case ">":
							want = lhs > bound
						case ">=":
							want = lhs >= bound
						}
						if got == nil || (got.Sign() != 0) != want {
							t.Fatalf("%d*x+%d %s %d at %d rewritten as %s", coef, offset, op, bound, v, a.text(term))
						}
					}
				}
			}
		}
	}
}

// CA-ARITH: enumerate the entire u8 and i8 operand domains. The reference uses
// int64, whose range contains every tested mathematical intermediate.
func TestArithmeticPreconditionsExhaustive8Bit(t *testing.T) {
	for _, typ := range []string{"u8", "i8"} {
		for _, op := range []string{"+", "-", "*", "/", "%"} {
			t.Run(typ+op, func(t *testing.T) {
				r := reportFor(t, fmt.Sprintf("fn f(a %s,b %s) %s { return a %s b }", typ, typ, typ, op))
				f := function(t, r, "f")
				lo, hi := int64(0), int64(255)
				if typ == "i8" {
					lo, hi = -128, 127
				}
				for x := lo; x <= hi; x++ {
					for y := lo; y <= hi; y++ {
						want := true
						value := int64(0)
						switch op {
						case "+":
							value = x + y
						case "-":
							value = x - y
						case "*":
							value = x * y
						case "/", "%":
							if y == 0 || (typ == "i8" && x == -128 && y == -1) {
								want = false
							} else if op == "/" {
								value = x / y
							} else {
								value = x % y
							}
						}
						want = want && value >= lo && value <= hi
						got := evaluate(r.Terms, f.Precondition, map[string]*big.Int{"f:a": big.NewInt(x), "f:b": big.NewInt(y)})
						if got == nil || (got.Sign() != 0) != want {
							t.Fatalf("%s %s: %d,%d: pre=%v success=%v", typ, op, x, y, got, want)
						}
					}
				}
			})
		}
	}
}
func TestCompositionAndCheckedCancellationExhaustive8Bit(t *testing.T) {
	r := reportFor(t, `fn minus1(x u8) u8 { return x-1 }
fn div(x u8) u8 { return 12/x }
fn pipeline(x u8) u8 { return x -> minus1 -> div }
fn cancel(x u8) u8 { return (x+1)-(x+1) }
fn multiply(a u8,b u8) u8 { return (a+b)*2 }`)
	for x := int64(0); x < 256; x++ {
		for _, name := range []string{"pipeline", "cancel"} {
			f := function(t, r, name)
			got := evaluate(r.Terms, f.Precondition, map[string]*big.Int{name + ":x": big.NewInt(x)})
			want := x > 1
			if name == "cancel" {
				want = x < 255
			}
			if got == nil || (got.Sign() != 0) != want {
				t.Fatalf("%s(%d): %v", name, x, got)
			}
		}
		for y := int64(0); y < 256; y++ {
			f := function(t, r, "multiply")
			got := evaluate(r.Terms, f.Precondition, map[string]*big.Int{"multiply:a": big.NewInt(x), "multiply:b": big.NewInt(y)})
			if got == nil || (got.Sign() != 0) != ((x+y)*2 <= 255) {
				t.Fatalf("multiply(%d,%d): %v", x, y, got)
			}
		}
	}
}
func TestIntervalProofSoundnessSmallDomains(t *testing.T) {
	for _, op := range []string{"+", "-", "*", "/", "%"} {
		for lo := int64(-4); lo <= 4; lo++ {
			for hi := lo; hi <= 4; hi++ {
				a := newArena()
				x := a.raw("var", "x")
				y := a.raw("var", "y")
				ctx := newContext()
				ctx.put(x, span{big.NewInt(lo), big.NewInt(hi)})
				ctx.put(y, span{big.NewInt(lo), big.NewInt(hi)})
				expression := a.op(op, x, y)
				for _, cmp := range []string{"==", "!=", "<", "<=", ">", ">="} {
					goal := a.op(cmp, expression, a.num(1))
					proof := a.prove(goal, ctx)
					for xv := lo; xv <= hi; xv++ {
						for yv := lo; yv <= hi; yv++ {
							v := evaluate(a.nodes, goal, map[string]*big.Int{"x": big.NewInt(xv), "y": big.NewInt(yv)})
							if proof.Status != Unknown && v == nil {
								t.Fatalf("proof across undefined math: %s %+v", a.text(goal), proof)
							}
							if v != nil && ((proof.Status == Proven && v.Sign() == 0) || (proof.Status == Violated && v.Sign() != 0)) {
								t.Fatalf("false proof: %s at %d,%d: %+v", a.text(goal), xv, yv, proof)
							}
						}
					}
				}
			}
		}
	}
}
func TestResourceLimitsDoNotBecomeProofs(t *testing.T) {
	a := newArena()
	a.limit = 4
	x := a.raw("var", "x")
	y := a.raw("var", "y")
	if y != unknown || !a.limited {
		t.Fatal("budget not applied")
	}
	if id := a.op("==", y, y); a.prove(id, newContext()).Status != Unknown {
		t.Fatal("two unknowns became an equality proof")
	}
	ctx := newContext()
	ctx.put(x, span{big.NewInt(0), big.NewInt(255)})
	if p := a.prove(a.op("<=", a.op("*", x, x), a.num(255)), ctx); p.Status != Unknown {
		t.Fatal(p)
	}
}
func FuzzAffineRewriteSoundness(f *testing.F) {
	f.Add(int64(1), int64(-1), int64(0), int64(1), uint8(0))
	f.Add(int64(-3), int64(2), int64(-7), int64(3), uint8(3))
	f.Fuzz(func(t *testing.T, coef, offset, bound, value int64, which uint8) {
		// Bound resource use, not arithmetic precision. Reference intermediates here
		// remain within int64; the production analyzer uses big.Int.
		coef %= 1000
		offset %= 1000
		bound %= 1000
		value %= 1000
		ops := []string{"==", "!=", "<", "<=", ">", ">="}
		op := ops[int(which)%len(ops)]
		a := newArena()
		x := a.raw("var", "x")
		goal := a.op(op, a.op("+", a.op("*", a.num(coef), x), a.num(offset)), a.num(bound))
		got := evaluate(a.nodes, goal, map[string]*big.Int{"x": big.NewInt(value)})
		lhs := coef*value + offset
		want := false
		switch op {
		case "==":
			want = lhs == bound
		case "!=":
			want = lhs != bound
		case "<":
			want = lhs < bound
		case "<=":
			want = lhs <= bound
		case ">":
			want = lhs > bound
		case ">=":
			want = lhs >= bound
		}
		if got == nil || (got.Sign() != 0) != want {
			t.Fatalf("unsound rewrite %s", a.text(goal))
		}
	})
}
