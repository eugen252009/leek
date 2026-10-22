package constraints

import (
	"fmt"
	"math/big"
	"sort"
	"strings"

	"leek/bin/parser"
)

type Fact struct {
	Label  string    `json:"label"`
	Value  ID        `json:"value"`
	Type   string    `json:"type"`
	Range  *Interval `json:"range,omitempty"`
	Scope  string    `json:"scope"`
	Reason string    `json:"reason"`
}
type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

func at(p parser.SourcePos) Location { return Location{p.Filename, p.Line, p.Column} }

type Origin struct {
	Function     string            `json:"function"`
	Node         string            `json:"node"`
	Operation    string            `json:"operation"`
	Location     Location          `json:"location"`
	Via          []string          `json:"via,omitempty"`
	SemanticNode *parser.GraphNode `json:"-"` // exact operation identity in this validated graph
}
type Obligation struct {
	ID          int    `json:"id"` // function-local structural identity
	Environment int    `json:"environment"`
	Guard       ID     `json:"guard"`
	Required    ID     `json:"required"`
	Effective   ID     `json:"effective"` // guard => required
	Origin      Origin `json:"origin"`
	Proof       Proof  `json:"proof"`
}
type ValueInfo struct {
	Node     string    `json:"node"`
	Type     string    `json:"type"`
	Relation ID        `json:"relation"`
	Guard    ID        `json:"guard"`
	Range    *Interval `json:"range,omitempty"`
	Scope    string    `json:"scope"`
}
type CallInfo struct {
	Node      string `json:"node"`
	Function  string `json:"function"`
	Kind      string `json:"kind"`
	Arguments []ID   `json:"arguments"`
	Pure      bool   `json:"pure"`
	// These IDs belong to this report's term arena and premise snapshots.
	Guard        ID    `json:"guard"`
	Environment  int   `json:"environment"`
	Precondition ID    `json:"instantiated_precondition"`
	Proof        Proof `json:"proof"`
	Complete     bool  `json:"complete"`
}

type FunctionReport struct {
	QuantifiedObligations []Obligation   `json:"quantified_obligations,omitempty"`
	Dataflow              []DataflowInfo `json:"dataflow,omitempty"`
	Name                  string         `json:"name"`
	Pure                  bool           `json:"pure"`
	InputFacts            []Fact         `json:"input_facts"`
	AuxiliaryFacts        []Fact         `json:"auxiliary_facts"`
	Obligations           []Obligation   `json:"obligations"`
	Precondition          ID             `json:"derived_precondition"`
	PreconditionText      string         `json:"derived_precondition_text"`
	Proof                 Proof          `json:"proof"`
	Complete              bool           `json:"complete"`
	Limitations           []string       `json:"limitations"`
	Output                *ValueInfo     `json:"output,omitempty"`
	Values                []ValueInfo    `json:"values"`
	Calls                 []CallInfo     `json:"calls"`
	Environments          []Environment  `json:"environments"`
	Returns               []ValueInfo    `json:"returns"`
}
type Report struct {
	Version       int               `json:"version"`
	AnalysisOnly  bool              `json:"analysis_only"`
	RuntimeChecks string            `json:"runtime_checks"`
	Limited       bool              `json:"resource_limit_reached"`
	Terms         []Term            `json:"terms"`
	Functions     []*FunctionReport `json:"functions"`
}

type analyzer struct {
	g         *parser.ProgramGraph
	a         *arena
	summaries map[*parser.FunctionInfo]*FunctionReport
	params    map[*parser.FunctionInfo][]ID
	visiting  map[*parser.FunctionInfo]bool
	limited   bool
}

// Analyze consumes validated typed metadata but never writes to it. In
// particular, UNKNOWN and VIOLATED are report values, never compiler errors.
func Analyze(g *parser.ProgramGraph) (*Report, error) {
	if g == nil || !g.Validated {
		return nil, fmt.Errorf("constraint analysis requires a validated semantic graph")
	}
	b := &analyzer{g: g, a: newArena(), summaries: map[*parser.FunctionInfo]*FunctionReport{}, params: map[*parser.FunctionInfo][]ID{}, visiting: map[*parser.FunctionInfo]bool{}}
	names := []string{}
	for name := range g.Functions {
		names = append(names, name)
	}
	sort.Strings(names)
	report := &Report{Version: 2, AnalysisOnly: true, RuntimeChecks: "preserved", Functions: []*FunctionReport{}}
	for _, name := range names {
		r, err := b.function(g.Functions[name])
		if err != nil {
			return nil, err
		}
		report.Functions = append(report.Functions, r)
	}
	report.Terms = b.a.nodes
	report.Limited = b.a.limited || b.limited
	return report, nil
}

const maxObligations = 4096
const maxExpressionVisits = 20000

type functionState struct {
	b         *analyzer
	f         *parser.FunctionInfo
	report    *FunctionReport
	env       map[*parser.GraphNode]ID
	context   context
	remaining int
	truncated bool
}

func (b *analyzer) function(f *parser.FunctionInfo) (*FunctionReport, error) {
	if r := b.summaries[f]; r != nil {
		return r, nil
	}
	if b.visiting[f] {
		return nil, fmt.Errorf("unexpected recursive call graph in validated input")
	}
	b.visiting[f] = true
	defer delete(b.visiting, f)
	// Acyclic summaries, not a recursive fixed-point solver. Effectful callees
	// stay opaque, even if today's implementation happens to return a constant.
	for _, call := range f.Calls {
		if call.Callee.Effects == parser.EffectNone {
			if _, err := b.function(call.Callee); err != nil {
				return nil, err
			}
		}
	}
	r := &FunctionReport{Name: f.Decl.Name, Pure: f.Effects == parser.EffectNone, Complete: true, InputFacts: []Fact{}, AuxiliaryFacts: []Fact{}, Obligations: []Obligation{}, Values: []ValueInfo{}, Calls: []CallInfo{}, Limitations: []string{}}
	s := &functionState{b: b, f: f, report: r, env: map[*parser.GraphNode]ID{}, context: newContext(), remaining: maxExpressionVisits}
	if !r.Pure {
		s.incomplete("effectful function: external success is not established")
	}
	for _, p := range f.Decl.Parameters {
		id := b.a.raw("var", f.Decl.Name+":"+p.Name)
		if _, ok := integerBounds(p.Type); !ok && p.Type != "bool" {
			id = b.a.raw("opaque", f.Decl.Name+":"+p.Name)
			s.incomplete("floating-point value relations are not analyzed")
		}
		b.params[f] = append(b.params[f], id)
		s.env[b.g.Nodes[f.Decl.Name+":"+p.Name]] = id
		fact := Fact{Label: f.Decl.Name + ":" + p.Name, Value: id, Type: p.Type, Scope: "function input", Reason: "declared type"}
		if domain, ok := valueBounds(p.Type); ok {
			fact.Range = interval(domain)
			if id != unknown {
				s.context.put(id, domain)
			}
		}
		r.InputFacts = append(r.InputFacts, fact)
	}
	inputContext := s.context.clone()
	s.statements(f.Decl.Body, yes)
	if len(r.Returns) > 0 {
		// Validation establishes that every successful value-returning execution
		// reaches one of these returns. The last is therefore a safe default.
		id := r.Returns[len(r.Returns)-1].Relation
		for i := len(r.Returns) - 2; i >= 0; i-- {
			id = b.a.op("select", r.Returns[i].Guard, r.Returns[i].Relation, id)
		}
		r.Output = &ValueInfo{Node: f.Decl.Name + ":$return", Type: f.Decl.ReturnType, Relation: id, Guard: yes, Scope: "if the entire function completes successfully"}
	}
	// Never prove an earlier operation from the final/continuing environment.
	s.context = inputContext
	if b.a.limited {
		s.limit("symbolic term budget exhausted")
	}
	conditions := []ID{}
	for i := range r.Obligations {
		o := &r.Obligations[i]
		o.Effective = b.a.op("or", b.a.op("not", o.Guard), o.Required)
		if o.Proof.Status == Proven || !o.Proof.Consistent {
			// An unreachable obligation is vacuous for the function contract,
			// but remains UNKNOWN (not eligible for check removal) at its site.
			conditions = append(conditions, yes)
		} else {
			conditions = append(conditions, o.Effective)
		}
	}
	pre := b.a.op("and", conditions...)
	r.Precondition = b.a.simplifyKnown(pre, s.context)
	if b.a.limited {
		s.limit("symbolic term budget exhausted")
		r.Precondition = b.a.op("and", r.Precondition, unknown)
	}
	r.PreconditionText = strings.ReplaceAll(b.a.text(r.Precondition), f.Decl.Name+":", "")
	r.Proof = b.a.prove(r.Precondition, s.context)
	if !r.Complete && r.Proof.Status == Proven {
		r.Proof.Status = Unknown
		r.Proof.Reason = "partial analysis; unresolved effects, values, or resource limits remain"
	}
	if r.Output != nil {
		successContext := b.a.refine(s.context, r.Precondition)
		r.Output.Range = s.resultRange(r.Output.Relation, r.Output.Type, successContext)
	}
	if b.a.limited {
		s.limit("symbolic term budget exhausted")
		r.Precondition = b.a.op("and", r.Precondition, unknown)
		r.PreconditionText = strings.ReplaceAll(b.a.text(r.Precondition), f.Decl.Name+":", "")
		r.Proof = b.a.prove(r.Precondition, s.context)
		if r.Proof.Status == Proven {
			r.Proof = Proof{Unknown, "resource limit reached", true}
		}
	}
	b.summaries[f] = r
	return r, nil
}
func valueBounds(typ string) (span, bool) {
	if typ == "bool" {
		return span{big.NewInt(0), big.NewInt(1)}, true
	}
	return integerBounds(typ)
}
func (s *functionState) incomplete(reason string) {
	s.report.Complete = false
	for _, r := range s.report.Limitations {
		if r == reason {
			return
		}
	}
	s.report.Limitations = append(s.report.Limitations, reason)
}
func (s *functionState) limit(reason string) {
	s.b.limited = true
	s.incomplete(reason)
	if !s.truncated {
		s.truncated = true
		s.report.Obligations = append(s.report.Obligations, Obligation{ID: len(s.report.Obligations), Environment: s.snapshot(newContext(), yes), Guard: yes, Required: unknown, Effective: unknown, Proof: Proof{Unknown, reason, true}, Origin: Origin{Function: s.f.Decl.Name, Operation: "analysis limit"}})
	}
}
func (s *functionState) require(guard, required ID, origin Origin) {
	if len(s.report.Obligations) >= maxObligations {
		s.limit("obligation expansion budget exhausted")
		return
	}
	c := s.b.a.refine(s.context, guard)
	s.report.Obligations = append(s.report.Obligations, Obligation{ID: len(s.report.Obligations), Environment: s.snapshot(c, guard), Guard: guard, Required: required, Origin: origin, Proof: s.b.a.prove(required, c)})
}
func (s *functionState) origin(e *parser.Expr) Origin {
	return Origin{Function: s.f.Decl.Name, Node: s.b.g.Expressions[e].Node.ID, Operation: e.Name, Location: at(e.Pos), SemanticNode: s.b.g.Expressions[e].Node}
}
func (s *functionState) resultRange(id ID, typ string, c context) *Interval {
	if !c.consistent {
		return &Interval{Empty: true}
	}
	domain, ok := valueBounds(typ)
	if !ok {
		return nil
	}
	mathematical := s.b.a.reasoner(c).rangeOf(id)
	if mathematical != nil {
		if mathematical.lo.Cmp(domain.lo) > 0 {
			domain.lo = new(big.Int).Set(mathematical.lo)
		}
		if mathematical.hi.Cmp(domain.hi) < 0 {
			domain.hi = new(big.Int).Set(mathematical.hi)
		}
	}
	// Intersection with the type is justified ONLY by successful evaluation.
	if domain.lo.Cmp(domain.hi) > 0 {
		return &Interval{Empty: true}
	}
	return interval(domain)
}
func (s *functionState) opaque(e *parser.Expr, typ, reason string) ID {
	s.incomplete(reason)
	id := s.b.a.raw("opaque", s.b.g.Expressions[e].Node.ID+":value")
	fact := Fact{Label: s.b.g.Expressions[e].Node.ID, Value: id, Type: typ, Scope: "if this call or expression returns", Reason: reason}
	if domain, ok := valueBounds(typ); ok {
		fact.Range = interval(domain)
		if id != unknown {
			s.context.put(id, domain)
		}
	}
	s.report.AuxiliaryFacts = append(s.report.AuxiliaryFacts, fact)
	return id
}
func (s *functionState) expr(e *parser.Expr, guard ID) ID {
	s.remaining--
	if s.remaining < 0 {
		s.limit("expression visit budget exhausted")
		return unknown
	}
	a := s.b.a
	info := s.b.g.Expressions[e]
	result := unknown
	if info.Dataflow != nil {
		result = s.dataflow(e, guard)
	} else if info.Constant != "" {
		switch info.Type {
		case "bool":
			result = no
			if info.Constant == "1" {
				result = yes
			}
		case "f32", "f64":
			result = s.opaque(e, info.Type, "floating-point value relations are not analyzed")
		default:
			result = a.number(integer(info.Constant))
		}
	} else {
		switch e.Kind {
		case "name":
			var ok bool
			result, ok = s.env[info.Binding]
			if !ok {
				s.limit("unresolved symbolic binding")
				result = unknown
			}
		case "unary":
			x := s.expr(e.Left, guard)
			switch e.Name {
			case "+":
				result = x
			case "!":
				result = a.op("not", x)
			case "-":
				result = a.op("-", a.num(0), x)
				s.arithmetic(e, guard, "-", a.num(0), x, result, info.Type)
			}
		case "binary":
			x := s.expr(e.Left, guard)
			rightGuard := guard
			if e.Name == "&&" {
				rightGuard = a.op("and", guard, x)
			} else if e.Name == "||" {
				rightGuard = a.op("and", guard, a.op("not", x))
			}
			y := s.expr(e.Right, rightGuard)
			switch e.Name {
			case "&&":
				result = a.op("and", x, y)
			case "||":
				result = a.op("or", x, y)
			default:
				operandType := s.b.g.Expressions[e.Left].Type
				if operandType == "f32" || operandType == "f64" {
					result = s.opaque(e, info.Type, "floating-point comparisons are not analyzed")
				} else {
					result = a.op(e.Name, x, y)
					if !isComparison(e.Name) {
						s.arithmetic(e, guard, e.Name, x, y, result, info.Type)
					}
				}
			}
		case "call", "composition":
			args := []ID{}
			if e.Kind == "composition" {
				args = append(args, s.expr(e.Left, guard))
			}
			for _, arg := range e.Args {
				args = append(args, s.expr(arg, guard))
			}
			callee := info.Callee
			callContext := a.refine(s.context, guard)
			call := CallInfo{Node: info.Node.ID, Function: callee.Decl.Name, Kind: e.Kind, Arguments: append([]ID{}, args...), Pure: callee.Effects == parser.EffectNone,
				Guard: guard, Environment: s.snapshot(callContext, guard), Precondition: unknown,
				Proof: Proof{Unknown, "callee success is not established", callContext.consistent}}
			if callee.Effects != parser.EffectNone {
				success := a.raw("opaque", info.Node.ID+":call-completes")
				s.require(guard, success, s.origin(e))
				result = s.opaque(e, info.Type, "effectful calls are opaque; their success is not established")
			} else {
				summary := s.b.summaries[callee]
				if !summary.Complete {
					s.incomplete("callee " + callee.Decl.Name + " has incomplete analysis")
				}
				mapping := map[ID]ID{}
				for i, p := range s.b.params[callee] {
					if p != unknown {
						mapping[p] = args[i]
					}
				}
				memo := map[ID]ID{}
				call.Complete = summary.Complete
				call.Precondition = a.substitute(summary.Precondition, mapping, memo)
				call.Proof = a.prove(call.Precondition, callContext)
				if !summary.Complete && call.Proof.Status == Proven {
					call.Proof.Status = Unknown
					call.Proof.Reason = "incomplete callee summary"
				}
				for _, o := range summary.Obligations {
					if len(s.report.Obligations) >= maxObligations {
						s.limit("obligation expansion budget exhausted")
						break
					}
					origin := o.Origin
					origin.Via = append([]string{info.Node.ID}, o.Origin.Via...)
					s.require(a.op("and", guard, a.substitute(o.Guard, mapping, memo)), a.substitute(o.Required, mapping, memo), origin)
				}
				if summary.Output != nil {
					result = a.substitute(summary.Output.Relation, mapping, memo)
				} else {
					result = yes
				}
				if !summary.Complete && info.Type != "" {
					// An opaque callee output is not a function of substituted parameters.
					// Reusing its identity across different calls would equate unrelated
					// endpoints (e.g. two sequence reductions) and invent empty domains.
					result = s.opaque(e, info.Type, "incomplete callee result has a call-site-local opaque identity")
				}
				// Pure incomplete callees may contain opaque float relations. They have
				// no integer return relations inferred from rounded source spellings.
			}
			s.report.Calls = append(s.report.Calls, call)
		default:
			s.limit("unsupported expression operation")
		}
	}
	if a.limited {
		s.limit("symbolic term budget exhausted")
	}
	scope := "if this expression is evaluated and completes successfully"
	if info.Constant != "" && (info.Type != "f32" && info.Type != "f64") {
		scope = "literal value"
	}
	s.report.Values = append(s.report.Values, ValueInfo{Node: info.Node.ID, Type: info.Type, Relation: result, Guard: guard, Range: s.resultRange(result, info.Type, a.refine(s.context, guard)), Scope: scope})
	return result
}
func (s *functionState) arithmetic(e *parser.Expr, guard ID, op string, x, y, result ID, typ string) {
	a := s.b.a
	domain, ok := integerBounds(typ)
	if !ok {
		s.limit("unsupported arithmetic type")
		return
	}
	origin := s.origin(e)
	switch op {
	case "+", "-", "*":
		if typ[0] == 'u' {
			if op == "-" {
				s.require(guard, a.op(">=", x, y), origin)
			} else {
				s.require(guard, a.op("<=", result, a.number(domain.hi)), origin)
			}
		} else {
			s.require(guard, a.op(">=", result, a.number(domain.lo)), origin)
			s.require(guard, a.op("<=", result, a.number(domain.hi)), origin)
		}
	case "/", "%":
		s.require(guard, a.op("!=", y, a.num(0)), origin)
		if typ[0] == 'i' {
			s.require(guard, a.op("not", a.op("and", a.op("==", x, a.number(domain.lo)), a.op("==", y, a.num(-1)))), origin)
		}
	}
}
