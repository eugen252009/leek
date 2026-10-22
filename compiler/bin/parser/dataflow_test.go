package parser

import (
	"fmt"
	"strings"
	"testing"
)

const sequencePrelude = `fn square(x u32) u32 { return x*x }
fn add(a u32,b u32) u32 { return a+b }
fn even(x u32) bool { return x%2==0 }
`

// DF-IR / DF-TYPE: existing syntax resolves to explicit semantic operations.
func TestDataflowStructure(t *testing.T) {
	g := resolveSource(t, sequencePrelude+`fn f(n u32) u32 {
 return range(0,n)
  -> filter(even)
  -> map(square)
  -> reduce(0,add)
}`)
	terms := DataflowTerminals(g)
	if len(terms) != 1 {
		t.Fatal(terms)
	}
	chain := Pipeline(g, terms[0])
	want := []DataflowKind{Range, Filter, Map, Reduce}
	if len(chain) != len(want) {
		t.Fatal(chain)
	}
	for i, e := range chain {
		info := g.Expressions[e]
		d := info.Dataflow
		if d.Kind != want[i] || d.Order != ElementMajor || info.Callee != nil || d.Materialization == "" {
			t.Fatalf("stage: %+v", d)
		}
		if i > 0 {
			ref := g.Expressions[d.Reference]
			if ref == nil || ref.Reference != d.Callback || ref.Type != "$function" {
				t.Fatal("reference lost")
			}
			found := false
			for _, edge := range info.Node.Incoming {
				if edge.Type == DataEdge && edge.From == g.Expressions[d.Input].Node {
					found = true
				}
			}
			if !found {
				t.Fatal("missing input edge")
			}
		}
	}
	if g.Expressions[terms[0]].Type != "u32" {
		t.Fatal("wrong terminal type")
	}
	effectGraph := resolveSource(t, sequencePrelude+`impure fn mark(x u32) u32 { print(x) return x } impure fn main(){ print(range(mark(0),mark(3)) -> map(square) -> reduce(mark(0),add)) }`)
	terminal := DataflowTerminals(effectGraph)[0]
	info := effectGraph.Expressions[terminal]
	if info.Effects == EffectNone || info.StageEffects != EffectNone || !info.MayFail || info.Dataflow.Callback.Effects != EffectNone {
		t.Fatal("source effects, callback purity, and checked failures were conflated")
	}
}
func TestDataflowSemanticRejections(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`fn f() { range(0,2) }`, "must be consumed"},
		{`fn f() { range(0,2) -> map(square) }`, "must be consumed"},
		{`fn f() u32 { return range(0,2) }`, "must be consumed"},
		{`impure fn main() { print(range(0,2)) }`, "must be consumed"},
		{`fn f() u32 { return map(square) }`, "only supported"},
		{`fn f() u32 { return 2 -> map(square) -> reduce(0,add) }`, "requires a sequence"},
		{`fn f() u32 { return range(0) -> reduce(0,add) }`, "range(start, end)"},
		{`fn f(x bool) u32 { return range(x,x) -> reduce(0,add) }`, "integer types"},
		{`fn f(x i32,y u32) i32 { return range(x,y) -> fold(x,add) }`, "type mismatch"},
		{`fn wrong(x u64) u32 { return 1 } fn f() u32 { return range(0,2) -> map(wrong) -> reduce(0,add) }`, "map callback must accept"},
		{`impure fn bad(x u32) u32 { print(x) return x } impure fn f() u32 { return range(0,2) -> map(bad) -> reduce(0,add) }`, "pure scalar callback"},
		{`fn f() u32 { return range(0,2) -> map(add) -> reduce(0,add) }`, "map callback must accept"},
		{`fn f() u32 { return range(0,2) -> filter(square) -> reduce(0,add) }`, "return bool"},
		{`fn f() u32 { return range(0,2) -> map(square(1)) -> reduce(0,add) }`, "statically known"},
		{`fn f() u32 { return range(0,2) -> reduce(0,square) }`, "must accept (state"},
		{`fn f() u32 { return range(0,2) -> reduce(true,add) }`, "type mismatch"},
		{`fn step(a u64,b u32) u64 { return a } fn f() u64 { return range(0,2) -> reduce(0,step) }`, "use fold"},
		{`impure fn bad(x u32) u32 { return x } fn f() u32 { return range(0,2) -> map(bad) -> reduce(0,add) }`, "pure scalar callback"},
		{`impure fn bad(x u32) u32 { print(x) return x } fn wrap(x u32) u32 { return bad(x) } fn f() u32 { return range(0,2) -> map(wrap) -> reduce(0,add) }`, "transitively pure"},
		{`fn f() u32 { let square u32 = 1 return range(0,2) -> map(square) -> reduce(0,add) }`, "indirect calls"},
		{`fn map() {}`, "reserved function"},
		{`fn f() u32 { return range(0,2) -> map(f) -> reduce(0,add) }`, "map callback must accept"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			ast, err := NewParser(strings.NewReader(sequencePrelude+tt.source), "dataflow.lk").ParseProgram()
			if err == nil {
				_, err = NewResolver().Resolve(ast)
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %q, got %v", tt.want, err)
			}
		})
	}
}

// DF-ORDER / DF-RANGE / DF-FOLD: canonical lowering is independently executable.
func TestDataflowCanonical(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{sequencePrelude + `fn f(n u32) u32 { return range(0,n) -> map(square) -> reduce(0,add) } impure fn main(){ print(f(0)) print(f(1)) print(f(2)) print(f(5)) print(f(100)) }`, "0\n0\n1\n30\n328350\n"},
		{sequencePrelude + `impure fn main(){ print(range(0,6) -> filter(even) -> map(square) -> reduce(7,add)) }`, "27\n"},
		{sequencePrelude + `fn no(x u32) bool { return false } fn fail(x u32) u32 { return 1/0 } impure fn main(){ print(range(0,6) -> filter(no) -> map(fail) -> reduce(7,add)) print(range(5,2) -> map(fail) -> reduce(8,add)) }`, "7\n8\n"},
		{`fn step(state u32,x u32) u32 { return state*2+x } impure fn main(){ print(range(0,4) -> fold(0,step)) }`, "11\n"},
		{`fn step(state u64,x u32) u64 { if x==0 { return state } return state+1 } impure fn main(){ print(range(0,4) -> fold(100,step)) }`, "103\n"},
		{sequencePrelude + `impure fn mark(x u32) u32 { print(x) return x } impure fn main(){ print(range(mark(2),mark(4)) -> map(square) -> reduce(mark(10),add)) }`, "2\n4\n10\n23\n"},
		{sequencePrelude + `fn flag(x u32) bool { return x>1 } fn any(a bool,b bool) bool { return a||b } impure fn main(){ print(range(0,4) -> map(flag) -> reduce(false,any)) }`, "1\n"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			code, _ := generatedSource(t, tt.source)
			if !strings.Contains(code, "canonical pull") || strings.Contains(code, "malloc(") {
				t.Fatal("missing canonical pull or unwanted materialization")
			}
			for _, opt := range []string{"-O0", "-O3"} {
				got := executeC(t, code, opt)
				if got.status != 0 || got.stderr != "" || got.stdout != tt.want {
					t.Fatalf("%s: %+v", opt, got)
				}
			}
		})
	}
}
func TestRangeTypeBoundaries(t *testing.T) {
	for _, tt := range []struct{ typ, start, end, want string }{
		{"u8", "254", "255", "254"}, {"i8", "-128", "-127", "-128"},
		{"u16", "65534", "65535", "65534"}, {"i16", "32766", "32767", "32766"},
		{"u32", "4294967294", "4294967295", "4294967294"}, {"i32", "-2147483648", "-2147483647", "-2147483648"},
		{"u64", "18446744073709551614", "18446744073709551615", "18446744073709551614"},
		{"i64", "-9223372036854775808", "-9223372036854775807", "-9223372036854775808"},
	} {
		source := fmt.Sprintf(`fn last(a %s,b %s) %s { return b } impure fn main(){ let start %s = %s let end %s = %s print(range(start,end) -> reduce(0,last)) print(range(end,end) -> reduce(0,last)) }`, tt.typ, tt.typ, tt.typ, tt.typ, tt.start, tt.typ, tt.end)
		code, _ := generatedSource(t, source)
		got := executeC(t, code, "-O0")
		if got.status != 0 || got.stderr != "" || got.stdout != tt.want+"\n0\n" {
			t.Fatalf("%s: %+v", tt.typ, got)
		}
	}
}
func TestDataflowCanonicalFailureOrder(t *testing.T) {
	for _, tt := range []struct{ body, reason string }{
		{`print(range(0,3) -> map(first) -> map(second) -> reduce(0,add))`, "division by zero"},
		{`print(range(3,3) -> map(first) -> reduce(1/0,add))`, "division by zero"},
		{`print(range(0,3) -> map(first) -> reduce(0,add))`, "overflow or underflow"},
	} {
		source := sequencePrelude + `fn first(x u32) u32 { if x==2 { return 0-1 } return x } fn second(x u32) u32 { return 100/x } impure fn main(){ print(77) ` + tt.body + ` print(99) }`
		code, _ := generatedSource(t, source)
		got := executeC(t, code, "-O0")
		if got.stdout != "77\n" || got.status != 1 || !strings.Contains(got.stderr, tt.reason) {
			t.Fatalf("%s: %+v", tt.body, got)
		}
	}
}
