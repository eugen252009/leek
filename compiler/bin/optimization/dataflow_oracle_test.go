package optimization

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"leek/bin/parser"
)

type referenceFailure struct {
	pos    parser.SourcePos
	reason string
}
type sequenceReference struct {
	t          testing.TB
	g          *parser.ProgramGraph
	out, trace strings.Builder
	steps      int
}

func (r *sequenceReference) fail(e *parser.Expr, reason string) {
	panic(referenceFailure{e.Pos, reason})
}
func (r *sequenceReference) call(f *parser.FunctionInfo, args []*big.Int) *big.Int {
	values := []string{}
	for _, arg := range args {
		values = append(values, arg.String())
	}
	r.trace.WriteString("trace:" + f.Decl.Name + "(" + strings.Join(values, ",") + ")\n")
	env := map[*parser.GraphNode]*big.Int{}
	for i, p := range f.Decl.Parameters {
		env[r.g.Nodes[f.Decl.Name+":"+p.Name]] = new(big.Int).Set(args[i])
	}
	var block func(*parser.BlockStatement) (*big.Int, bool)
	block = func(b *parser.BlockStatement) (*big.Int, bool) {
		for _, stmt := range b.Statements {
			switch s := stmt.(type) {
			case *parser.LetStatement:
				env[r.g.Declarations[s]] = r.expr(s.Expr, env)
			case *parser.ReturnStatement:
				if s.Value == nil {
					return nil, true
				}
				return r.expr(s.Value, env), true
			case *parser.ExpressionStatement:
				r.expr(s.Expr, env)
			case *parser.IfStatement:
				branch := s.Else
				if r.expr(s.Condition, env).Sign() != 0 {
					branch = s.Then
				}
				if branch != nil {
					if value, returned := block(branch); returned {
						return value, true
					}
				}
			}
		}
		return nil, false
	}
	value, _ := block(f.Decl.Body)
	return value
}
func (r *sequenceReference) expr(e *parser.Expr, env map[*parser.GraphNode]*big.Int) *big.Int {
	info := r.g.Expressions[e]
	if info.Constant != "" {
		n, ok := new(big.Int).SetString(info.Constant, 10)
		if !ok {
			r.t.Fatal("noninteger oracle input")
		}
		return n
	}
	// The reference derives collection structure from source syntax, not the
	// semantic Pipeline helper, optimizer plan, iterator code, or fused lowering.
	if e.Kind == "composition" && (e.Name == "reduce" || e.Name == "fold") {
		return r.sequence(e, env)
	}
	switch e.Kind {
	case "name":
		return new(big.Int).Set(env[info.Binding])
	case "call", "composition":
		args := []*big.Int{}
		if e.Kind == "composition" {
			args = append(args, r.expr(e.Left, env))
		}
		for _, a := range e.Args {
			args = append(args, r.expr(a, env))
		}
		if info.Callee.Builtin {
			r.out.WriteString(args[0].String() + "\n")
			return nil
		}
		return r.call(info.Callee, args)
	case "unary":
		x := r.expr(e.Left, env)
		if e.Name == "!" {
			return truth(x.Sign() == 0)
		}
		if e.Name == "+" {
			return x
		}
		return r.arithmetic(e, "-", new(big.Int), x)
	case "binary":
		x := r.expr(e.Left, env)
		if e.Name == "&&" && x.Sign() == 0 {
			return truth(false)
		}
		if e.Name == "||" && x.Sign() != 0 {
			return truth(true)
		}
		y := r.expr(e.Right, env)
		switch e.Name {
		case "&&":
			return truth(x.Sign() != 0 && y.Sign() != 0)
		case "||":
			return truth(x.Sign() != 0 || y.Sign() != 0)
		case "==":
			return truth(x.Cmp(y) == 0)
		case "!=":
			return truth(x.Cmp(y) != 0)
		case "<":
			return truth(x.Cmp(y) < 0)
		case "<=":
			return truth(x.Cmp(y) <= 0)
		case ">":
			return truth(x.Cmp(y) > 0)
		case ">=":
			return truth(x.Cmp(y) >= 0)
		default:
			return r.arithmetic(e, e.Name, x, y)
		}
	}
	r.t.Fatalf("unsupported oracle expression: %s", e.Kind)
	return nil
}
func truth(value bool) *big.Int {
	if value {
		return big.NewInt(1)
	}
	return new(big.Int)
}
func referenceBounds(typ string) (*big.Int, *big.Int) {
	bits, _ := strconv.Atoi(typ[1:])
	hi := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	lo := new(big.Int)
	if typ[0] == 'i' {
		hi.Rsh(hi, 1)
		lo.Neg(new(big.Int).Set(hi))
	}
	hi.Sub(hi, big.NewInt(1))
	return lo, hi
}
func (r *sequenceReference) arithmetic(e *parser.Expr, op string, x, y *big.Int) *big.Int {
	typ := r.g.Expressions[e].Type
	lo, hi := referenceBounds(typ)
	value := new(big.Int)
	switch op {
	case "+":
		value.Add(x, y)
	case "-":
		value.Sub(x, y)
	case "*":
		value.Mul(x, y)
	case "/", "%":
		if y.Sign() == 0 {
			r.fail(e, "division by zero")
		}
		if typ[0] == 'i' && x.Cmp(lo) == 0 && y.Cmp(big.NewInt(-1)) == 0 {
			r.fail(e, "integer overflow or underflow")
		}
		if op == "/" {
			value.Quo(x, y)
		} else {
			value.Rem(x, y)
		}
	}
	if value.Cmp(lo) < 0 || value.Cmp(hi) > 0 {
		r.fail(e, "integer overflow or underflow")
	}
	return value
}
func (r *sequenceReference) sequence(terminal *parser.Expr, env map[*parser.GraphNode]*big.Int) *big.Int {
	// Retain an ordered syntax recipe, not eager stage-major intermediate arrays.
	stages := []*parser.Expr{}
	source := terminal.Left
	for source.Kind == "composition" {
		stages = append([]*parser.Expr{source}, stages...)
		source = source.Left
	}
	if source.Kind != "call" || source.Name != "range" {
		r.t.Fatal("oracle supports inline ranges only")
	}
	start := r.expr(source.Args[0], env)
	end := r.expr(source.Args[1], env)
	acc := r.expr(terminal.Args[0], env)
	for x := new(big.Int).Set(start); x.Cmp(end) < 0; x.Add(x, big.NewInt(1)) {
		r.steps++
		if r.steps > 4096 {
			r.t.Fatal("oracle input outside bounded domain")
		}
		value := new(big.Int).Set(x)
		keep := true
		for _, stage := range stages {
			callback := r.g.Functions[stage.Args[0].Name]
			next := r.call(callback, []*big.Int{value})
			if stage.Name == "filter" {
				if next.Sign() == 0 {
					keep = false
					break
				}
			} else {
				value = next
			}
		}
		if keep {
			acc = r.call(r.g.Functions[terminal.Args[1].Name], []*big.Int{acc, value})
		}
	}
	return acc
}
func referenceOutcome(t testing.TB, g *parser.ProgramGraph, args []string) (result outcome) {
	t.Helper()
	r := &sequenceReference{t: t, g: g}
	defer func() {
		if caught := recover(); caught != nil {
			failure, ok := caught.(referenceFailure)
			if !ok {
				panic(caught)
			}
			result = outcome{r.out.String(), r.trace.String() + fmt.Sprintf("%s:%d:%d: runtime failure: %s\n", failure.pos.Filename, failure.pos.Line, failure.pos.Column, failure.reason), 1}
		}
	}()
	values := []*big.Int{}
	for _, arg := range args {
		v, ok := new(big.Int).SetString(arg, 10)
		if !ok {
			t.Fatal(arg)
		}
		values = append(values, v)
	}
	value := r.call(g.Functions["f"], values)
	return outcome{r.out.String() + value.String() + "\n", r.trace.String(), 0}
}

// Instrument source-function entries, including specialized variants, solely in
// the test harness. This makes logical callback order observable to the oracle
// without adding effects to the language or depending on backend iterator helpers.
func traceFunctions(g *parser.ProgramGraph, code string, variants []Variant) string {
	names := map[string]string{}
	for name, f := range g.Functions {
		names[f.CName] = name
	}
	for _, v := range variants {
		names[v.Name] = v.Function
	}
	for cname, name := range names {
		pattern := regexp.MustCompile(`(?m)^(?:static )?\w+ ` + cname + `\([^;{}]*\) \{`)
		code = pattern.ReplaceAllStringFunc(code, func(def string) string {
			f := g.Functions[name]
			format := strconv.Quote("trace:" + name + "(")
			values := []string{}
			for i, p := range f.Decl.Parameters {
				if i > 0 {
					format += " \",\""
				}
				macro, cast := "PRIu64", "uint64_t"
				if p.Type[0] == 'i' {
					macro, cast = "PRId64", "int64_t"
				}
				format += " \"%\" " + macro
				values = append(values, fmt.Sprintf("(%s)lk_p%d", cast, i))
			}
			format += " " + strconv.Quote(")\n")
			arguments := ""
			if len(values) > 0 {
				arguments = ", " + strings.Join(values, ",")
			}
			return def + " fprintf(stderr, " + format + arguments + ");"
		})
	}
	return code
}
func dataflowHarness(g *parser.ProgramGraph) string {
	f := g.Functions["f"]
	args := []string{}
	for i, p := range f.Decl.Parameters {
		ctype := "uint" + p.Type[1:] + "_t"
		parse := "strtoull"
		if p.Type[0] == 'i' {
			ctype = "int" + p.Type[1:] + "_t"
			parse = "strtoll"
		}
		args = append(args, fmt.Sprintf("(%s)%s(argv[%d],NULL,10)", ctype, parse, i+1))
	}
	macro, cast := "PRIu64", "uint64_t"
	if f.Decl.ReturnType[0] == 'i' {
		macro, cast = "PRId64", "int64_t"
	}
	return fmt.Sprintf("\nint main(int argc,char** argv){ if(argc!=%d)return 2; printf(\"%%\" %s \"\\n\",(%s)%s(%s));return 0;}\n", len(args)+1, macro, cast, f.CName, strings.Join(args, ","))
}
func compareDataflow(t testing.TB, source string, inputs [][]string, opt string) (int, int) {
	t.Helper()
	g := graphFor(t, source)
	canonical, err := parser.NewCodeGenerator().Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	optimized, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	harness := dataflowHarness(g)
	a := buildC(t, traceFunctions(g, canonical, nil)+harness, opt)
	b := buildC(t, traceFunctions(g, optimized.Code, optimized.Variants)+harness, opt)
	passed, failed := 0, 0
	for _, input := range inputs {
		want := referenceOutcome(t, g, input)
		ca, cb := execute(t, a, input...), execute(t, b, input...)
		if ca != want || cb != want {
			t.Fatalf("input %v %s\nreference %+v\ncanonical %+v\noptimized %+v\n%s", input, opt, want, ca, cb, source)
		}
		if want.status == 0 {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}
