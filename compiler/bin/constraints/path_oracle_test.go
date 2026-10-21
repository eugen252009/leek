package constraints

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"leek/bin/parser"
)

// PS-SOUND: a small test-only interpreter follows the typed source tree, NOT
// analyzer guards, output relations, contexts, or derived preconditions. Exact
// arithmetic and declared 8-bit limits provide the independent failure oracle.
type pathOracle struct {
	t      testing.TB
	g      *parser.ProgramGraph
	proofs map[string][]Obligation
	source string
}

func proofKey(node string, via []string) string {
	return strings.Join(append(append([]string{}, via...), node), "/")
}
func (o *pathOracle) call(f *parser.FunctionInfo, args []*big.Int, via []string) (*big.Int, bool) {
	env := map[*parser.GraphNode]*big.Int{}
	for i, p := range f.Decl.Parameters {
		env[o.g.Nodes[f.Decl.Name+":"+p.Name]] = args[i]
	}
	var expr func(*parser.Expr) (*big.Int, bool)
	expr = func(e *parser.Expr) (*big.Int, bool) {
		info := o.g.Expressions[e]
		if info.Constant != "" {
			v, _ := new(big.Int).SetString(info.Constant, 10)
			return v, true
		}
		switch e.Kind {
		case "name":
			return new(big.Int).Set(env[info.Binding]), true
		case "call", "composition":
			args := []*big.Int{}
			if e.Kind == "composition" {
				v, ok := expr(e.Left)
				if !ok {
					return nil, false
				}
				args = append(args, v)
			}
			for _, arg := range e.Args {
				v, ok := expr(arg)
				if !ok {
					return nil, false
				}
				args = append(args, v)
			}
			return o.call(info.Callee, args, append(append([]string{}, via...), info.Node.ID))
		case "unary":
			x, ok := expr(e.Left)
			if !ok {
				return nil, false
			}
			if e.Name == "!" {
				if x.Sign() == 0 {
					return big.NewInt(1), true
				}
				return big.NewInt(0), true
			}
			if e.Name == "+" {
				return x, true
			}
			return o.arithmetic(e, big.NewInt(0), x, via)
		case "binary":
			x, ok := expr(e.Left)
			if !ok {
				return nil, false
			}
			if e.Name == "&&" && x.Sign() == 0 {
				return big.NewInt(0), true
			}
			if e.Name == "||" && x.Sign() != 0 {
				return big.NewInt(1), true
			}
			y, ok := expr(e.Right)
			if !ok {
				return nil, false
			}
			truth := false
			switch e.Name {
			case "&&":
				truth = x.Sign() != 0 && y.Sign() != 0
			case "||":
				truth = x.Sign() != 0 || y.Sign() != 0
			case "==":
				truth = x.Cmp(y) == 0
			case "!=":
				truth = x.Cmp(y) != 0
			case "<":
				truth = x.Cmp(y) < 0
			case "<=":
				truth = x.Cmp(y) <= 0
			case ">":
				truth = x.Cmp(y) > 0
			case ">=":
				truth = x.Cmp(y) >= 0
			default:
				return o.arithmetic(e, x, y, via)
			}
			if truth {
				return big.NewInt(1), true
			}
			return big.NewInt(0), true
		}
		o.t.Fatalf("unsupported oracle expression %s", e.Kind)
		return nil, false
	}
	var block func(*parser.BlockStatement) (*big.Int, bool, bool)
	block = func(b *parser.BlockStatement) (*big.Int, bool, bool) {
		for _, stmt := range b.Statements {
			switch s := stmt.(type) {
			case *parser.LetStatement:
				v, ok := expr(s.Expr)
				if !ok {
					return nil, false, false
				}
				env[o.g.Declarations[s]] = v
			case *parser.ReturnStatement:
				v, ok := expr(s.Value)
				return v, ok, true
			case *parser.IfStatement:
				v, ok := expr(s.Condition)
				if !ok {
					return nil, false, false
				}
				branch := s.Else
				if v.Sign() != 0 {
					branch = s.Then
				}
				if branch != nil {
					value, ok, returned := block(branch)
					if !ok || returned {
						return value, ok, returned
					}
				}
			}
		}
		return nil, true, false
	}
	v, ok, _ := block(f.Decl.Body)
	return v, ok
}
func (o *pathOracle) arithmetic(e *parser.Expr, x, y *big.Int, via []string) (*big.Int, bool) {
	info := o.g.Expressions[e]
	lo, hi := int64(0), int64(255)
	if info.Type == "i8" {
		lo, hi = -128, 127
	} else if info.Type != "u8" {
		o.t.Fatal("oracle domain must be 8 bit")
	}
	result := new(big.Int)
	parts := []bool{}
	switch e.Name {
	case "+":
		result.Add(x, y)
	case "-":
		result.Sub(x, y)
	case "*":
		result.Mul(x, y)
	case "/", "%":
		parts = append(parts, y.Sign() != 0)
		if info.Type == "i8" {
			parts = append(parts, !(x.Int64() == -128 && y.Int64() == -1))
		}
		if y.Sign() != 0 {
			if e.Name == "/" {
				result.Quo(x, y)
			} else {
				result.Rem(x, y)
			}
		}
	}
	if len(parts) == 0 {
		if info.Type == "i8" {
			parts = append(parts, result.Cmp(big.NewInt(lo)) >= 0, result.Cmp(big.NewInt(hi)) <= 0)
		} else if e.Name == "-" {
			parts = append(parts, x.Cmp(y) >= 0)
		} else {
			parts = append(parts, result.Cmp(big.NewInt(hi)) <= 0)
		}
	}
	records := o.proofs[proofKey(info.Node.ID, via)]
	if len(records) != len(parts) {
		o.t.Fatalf("missing obligations at %s via %v: %d, want %d\n%s", info.Node.ID, via, len(records), len(parts), o.source)
	}
	safe := true
	for i, valid := range parts {
		proof := records[i].Proof
		if proof.Status == Proven && !valid || proof.Status == Violated && valid {
			o.t.Fatalf("unsound proof at %s via %v: %s, operands %s,%s, required actually %v\n%s", info.Node.ID, via, proof.Status, x, y, valid, o.source)
		}
		safe = safe && valid
	}
	return result, safe
}
func checkPathDomain(t testing.TB, source string, inputs [][]int64) {
	t.Helper()
	g := graphFor(t, source)
	r, err := Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	f := function(t, r, "f")
	o := pathOracle{t: t, g: g, source: source, proofs: map[string][]Obligation{}}
	for _, ob := range f.Obligations {
		key := proofKey(ob.Origin.Node, ob.Origin.Via)
		o.proofs[key] = append(o.proofs[key], ob)
	}
	for _, input := range inputs {
		args := []*big.Int{}
		for _, x := range input {
			args = append(args, big.NewInt(x))
		}
		value, ok := o.call(g.Functions["f"], args, nil)
		valuation := map[string]*big.Int{}
		for i, p := range g.Functions["f"].Decl.Parameters {
			valuation["f:"+p.Name] = args[i]
		}
		if ok && f.Output != nil {
			predicted := evaluate(r.Terms, f.Output.Relation, valuation)
			if predicted == nil || predicted.Cmp(value) != 0 {
				t.Fatalf("incorrect conditional return relation: actual %v, predicted %v, inputs %v\n%s", value, predicted, input, source)
			}
		}
		if pre := evaluate(r.Terms, f.Precondition, valuation); pre != nil && (pre.Sign() != 0) != ok {
			t.Fatalf("incorrect precondition: %s, success=%v, input=%v\n%s", f.PreconditionText, ok, input, source)
		}
		if f.Proof.Status == Proven && !ok || f.Proof.Status == Violated && ok {
			t.Fatalf("whole function %s but success=%v at %v\n%s", f.Proof.Status, ok, input, source)
		}
	}
}
func pathCase(signed bool, threshold int, comparison, form, operation byte) string {
	typ := "u8"
	if signed {
		typ = "i8"
	}
	cmp := []string{"==", "!=", "<", "<=", ">", ">="}[int(comparison)%6]
	c := fmt.Sprintf("x %s %d", cmp, threshold)
	switch form % 6 {
	case 1:
		c = "!(" + c + ")"
	case 2:
		c = "(" + c + ") && x != 0"
	case 3:
		c = "(" + c + ") || x == 0"
	case 4:
		c = "!((" + c + ") || x == 0)"
	case 5:
		c = "!((" + c + ") && x != 0)"
	}
	value := []string{"x+5", "x-1", "x*x", "100/x", "100%x", "x -> sub -> inverse"}[int(operation)%6]
	body := fmt.Sprintf("if %s { return %s } return 0", c, value)
	if form%3 == 1 {
		body = fmt.Sprintf("if %s { return 0 } return %s", c, value)
	}
	if form%3 == 2 {
		body = fmt.Sprintf("if %s { let v %s = %s } return x-1", c, typ, value)
	}
	return fmt.Sprintf("fn sub(x %s) %s { return x-1 } fn inverse(x %s) %s { return 100/x } fn f(x %s) %s { %s }", typ, typ, typ, typ, typ, typ, body)
}
func TestPathProofsExhaustive8Bit(t *testing.T) {
	cases := 0
	for _, signed := range []bool{false, true} {
		low := 0
		thresholds := []int{0, 1, 2, 10, 127, 254, 255}
		if signed {
			low = -128
			thresholds = []int{-128, -127, -1, 0, 1, 10, 126, 127}
		}
		inputs := [][]int64{}
		for x := low; x < low+256; x++ {
			inputs = append(inputs, []int64{int64(x)})
		}
		for _, threshold := range thresholds {
			for cmp := byte(0); cmp < 6; cmp++ {
				for form := byte(0); form < 6; form++ {
					for op := byte(0); op < 6; op++ {
						checkPathDomain(t, pathCase(signed, threshold, cmp, form, op), inputs)
						cases += len(inputs)
					}
				}
			}
		}
	}
	t.Logf("%d exhaustive input/program valuations", cases)
}
func TestPathRelationalPairs(t *testing.T) {
	inputs := [][]int64{}
	for x := int64(0); x < 256; x++ {
		for y := int64(0); y < 256; y++ {
			inputs = append(inputs, []int64{x, y})
		}
	}
	checkPathDomain(t, `fn f(x u8,y u8) u8 { if x >= y { return x-y } return y-x }`, inputs)
}
func FuzzPathProofSoundness(f *testing.F) {
	for i := byte(0); i < 12; i++ {
		f.Add(i, i*21, i%6, (i+1)%6, (i+2)%6)
	}
	f.Fuzz(func(t *testing.T, sign, threshold, cmp, form, op byte) {
		signed := sign%2 != 0
		boundary := int(threshold)
		low := 0
		if signed {
			boundary = int(int8(threshold))
			low = -128
		}
		inputs := [][]int64{}
		for x := low; x < low+256; x++ {
			inputs = append(inputs, []int64{int64(x)})
		}
		checkPathDomain(t, pathCase(signed, boundary, cmp, form, op), inputs)
	})
}
