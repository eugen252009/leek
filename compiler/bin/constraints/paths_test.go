package constraints

import (
	"encoding/json"
	"fmt"
	"leek/bin/parser"
	"strings"
	"testing"
)

// PS-PROOF / PS-JOIN: facts belong to execution points, not entire functions.
func TestPathSensitiveProofs(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		statuses   []Status
		whole      Status
	}{
		{"subtraction", `if x > 0 { return x-1 } return 0`, []Status{Proven}, Proven},
		{"addition", `if x <= 100 { return x+5 } return 0`, []Status{Proven}, Proven},
		{"multiplication unknown", `if x > 0 { return x*x } return 0`, []Status{Unknown}, Unknown},
		{"division", `if x != 0 { return 100/x } return 0`, []Status{Proven}, Proven},
		{"and", `if x != 0 && 100/x > 2 { return 1 } return 0`, []Status{Proven}, Proven},
		{"or short circuit", `if x == 0 || 100/x > 2 { return 1 } return 0`, []Status{Proven}, Proven},
		{"disjunction", `if x > 10 || x == 0 { return x-1 } return 0`, []Status{Unknown}, Unknown},
		{"negation", `if !(x <= 1) { return x-1 } return 0`, []Status{Proven}, Proven},
		{"no leaking", `if x > 1 { let y u32 = x-1 } return x-1`, []Status{Proven, Unknown}, Unknown},
		{"return refinement", `if x == 0 { return 0 } return x-1`, []Status{Proven}, Proven},
		{"else refinement", `if x == 0 { return 0 } else { return x-1 }`, []Status{Proven}, Proven},
		{"no retroactive proofs", `let y u32 = x-1 if x > 1 { return y } return 0`, []Status{Unknown}, Unknown},
		{"condition before branch", `if x-1 > 0 { return x-1 } return 0`, []Status{Unknown, Proven}, Unknown},
		{"join cannot invent unreachable", `if x > 1 { let y u32 = 1 } else { let z u32 = 1 } if x == 0 { return 1/0 } return 0`, []Status{Violated}, Unknown},
		{"join both live", `if x > 1 { let y u32 = x-1 } else { let z u32 = x+1 } return x-1`, []Status{Proven, Proven, Unknown}, Unknown},
		{"nested survivor", `if x > 0 { if x == 1 { return 0 } return x-2 } return 0`, []Status{Proven}, Proven},
		{"contradiction", `if x > 10 && x < 5 { return 1/0 } return 0`, []Status{Unknown}, Proven},
		{"one impossible predecessor", `if x < 0 { let y u32 = 1/0 } return x-1`, []Status{Unknown, Unknown}, Unknown},
		{"contradictory interior exclusion", `if x != 5 { if x == 5 { return 1/0 } } return 0`, []Status{Unknown}, Proven},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := reportFor(t, "fn f(x u32) u32 { "+tt.body+" }")
			f := function(t, r, "f")
			if len(f.Obligations) != len(tt.statuses) || f.Proof.Status != tt.whole {
				t.Fatalf("%s: %+v", f.PreconditionText, f)
			}
			for i, o := range f.Obligations {
				if o.Proof.Status != tt.statuses[i] {
					t.Fatalf("obligation %d: %+v", i, o)
				}
				if o.ID != i || o.Origin.SemanticNode == nil || o.Origin.Node != o.Origin.SemanticNode.ID {
					t.Fatal("missing structural operation identity")
				}
				env := f.Environments[o.Environment]
				if env.PathCondition != o.Guard || env.Reachable != o.Proof.Consistent {
					t.Fatal("invalid proof provenance")
				}
			}
		})
	}
}
func TestPathCompositionAndConditionalOutputs(t *testing.T) {
	source := `fn subtract(x u32, amount u32) u32 { return x-amount }
 fn inverse(x u32) u32 { return 100/x }
 fn pipeline(x u32) u32 { if x > 10 && x < 100 { return x -> subtract(10) -> inverse } return 0 }
 fn direct(x u32) u32 { if x > 10 && x < 100 { return inverse(subtract(x,10)) } return 0 }
 fn positive(x u32) u32 { if x > 1 { return x-1 } return 1 }
 fn use(x u32) u32 { return 100/positive(x) }`
	r := reportFor(t, source)
	for _, name := range []string{"pipeline", "direct", "positive", "use"} {
		f := function(t, r, name)
		if f.Proof.Status != Proven {
			t.Fatalf("%s: %+v", name, f)
		}
		for _, o := range f.Obligations {
			if o.Proof.Status != Proven {
				t.Fatalf("%s: %+v", name, o)
			}
		}
	}
	for _, name := range []string{"pipeline", "direct"} {
		f := function(t, r, name)
		if len(f.Obligations) != 2 {
			t.Fatal("callee obligations lost")
		}
		for _, o := range f.Obligations {
			if len(o.Origin.Via) != 1 {
				t.Fatal("call path lost")
			}
		}
	}
	if function(t, r, "subtract").Proof.Status != Unknown || function(t, r, "inverse").Proof.Status != Unknown {
		t.Fatal("caller proof leaked into generic callee")
	}
	if got := function(t, r, "positive").Output.Range; got.Min != "1" || got.Max != "4294967294" {
		t.Fatalf("piecewise range: %+v", got)
	}
}
func TestPathComparisonsAllIntegerTypes(t *testing.T) {
	for _, typ := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64"} {
		conditions := []string{"x > 0", "x >= 1", "x == 1", "x != 0", "!(x == 0)", "!(x < 1 || x == 0)"}
		if typ[0] == 'i' {
			conditions = append(conditions, "x < 0", "x <= -1")
		}
		for _, condition := range conditions {
			f := function(t, reportFor(t, fmt.Sprintf("fn f(x %s) %s { if %s { return 100/x } return 0 }", typ, typ, condition)), "f")
			for _, o := range f.Obligations {
				if o.Proof.Status != Proven {
					t.Fatalf("%s %s: %+v", typ, condition, o)
				}
			}
		}
	}
	// Signed minimum / -1 and % -1 are separate requirements, not just nonzero.
	for _, op := range []string{"/", "%"} {
		f := function(t, reportFor(t, fmt.Sprintf("fn f(x i8) i8 { if x != 0 { return x %s -1 } return 0 }", op)), "f")
		if f.Obligations[0].Proof.Status != Proven || f.Obligations[1].Proof.Status != Unknown {
			t.Fatalf("signed exception lost: %+v", f.Obligations)
		}
	}
}
func TestPathAnalysisReadOnlyAndDeterministic(t *testing.T) {
	source := `fn f(x u32) u32 { if x > 1 { let y u32 = x-1 return 100/y } return 0 }`
	g := graphFor(t, source)
	before, _ := parser.NewCodeGenerator().Generate(g)
	dot := g.ToDOT()
	a, err := Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	after, _ := parser.NewCodeGenerator().Generate(g)
	if before != after || dot != g.ToDOT() || string(aj) != string(bj) {
		t.Fatal("analysis mutated execution or produced nondeterministic proof environments")
	}
	if !strings.Contains(string(aj), `"environments"`) {
		t.Fatal("missing environment report")
	}
}
