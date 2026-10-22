package optimization

import (
	"fmt"

	"leek/bin/constraints"
	"leek/bin/parser"
)

const checkedUnsignedSemantics = "checked-integer-v1"
const maxAdditionalUnits = 4096
const maxBodyUnits = 256
const unitsPerAdditionalGuard = 32

type Requirement struct {
	Path            []string
	Operation       string
	Obligation      int
	Guard, Required constraints.ID
}
type Cost struct {
	BodyUnits       int
	RemovedGuards   int
	RemainingGuards int
}
type VariantAssessment struct {
	Variant string
	Proof   constraints.Proof
}
type Decision struct {
	Caller                string
	Path                  []string
	Function              string
	Action, Reason        string
	Selected, FinalTarget string
	Cost                  Cost
	Considered            []VariantAssessment
}
type PolicySummary struct {
	Generated, Allocated, Reused, RedundantAvoided, AdditionalUnits int
	BudgetExhausted                                                 bool
}
type proofQuery struct {
	caller      string
	environment int
	required    constraints.ID
}

// This registry belongs to one GenerateSpecialized invocation. Body and operation
// pointers, summary IDs and binding paths cannot escape into another compilation.
type specializer struct {
	local           map[*parser.GraphNode]bool
	counts          map[*parser.FunctionInfo]int
	variants        []parser.SpecializedFunction
	records         map[string]*Variant
	functions       map[string]*constraints.FunctionReport
	implication     *constraints.ImplicationEngine
	proofCache      map[proofQuery]constraints.Proof
	costs           map[*parser.FunctionInfo]int
	decisions       []Decision
	units           int
	budgetExhausted bool
}

func pathIDs(path []*parser.GraphNode) []string {
	ids := make([]string, len(path))
	for i, n := range path {
		ids[i] = n.ID
	}
	return ids
}

// Applicability is a logical gate BEFORE ranking. Every formal contract member
// is matched to the corresponding exact imported obligation, with the analyzer's
// actual-argument substitution and exact guarded environment. Cost never enters.
func (b *specializer) applicable(c *candidate, v parser.SpecializedFunction) constraints.Proof {
	unknown := constraints.Proof{Status: constraints.Unknown, Reason: "incompatible body, semantics, binding, or incomplete contract", Consistent: true}
	record := b.records[v.Name]
	if v.Function != c.f || record == nil || record.Semantics != checkedUnsignedSemantics {
		return unknown
	}
	for _, requirement := range record.Assumptions {
		bound := c
		for _, id := range requirement.Path {
			var next *candidate
			for n, child := range bound.children {
				if n.ID == id {
					next = child
					break
				}
			}
			if next == nil {
				return unknown
			}
			bound = next
		}
		var index int
		found := false
		for n, i := range bound.instances {
			if n.ID == requirement.Operation {
				index = i
				found = true
				break
			}
		}
		if !found {
			return unknown
		}
		ledger := b.functions[c.caller].Obligations
		if bound.quantified {
			ledger = b.functions[c.caller].QuantifiedObligations
		}
		if index < 0 || index >= len(ledger) {
			return unknown
		}
		o := ledger[index]
		if o.Origin.SemanticNode == nil || o.Origin.SemanticNode.ID != requirement.Operation {
			return unknown
		}
		key := proofQuery{c.caller, o.Environment, o.Required}
		proof, ok := b.proofCache[key]
		if !ok {
			query, err := b.implication.Implies(c.caller, o.Environment, o.Required)
			if err != nil {
				return unknown
			}
			proof = query.Proof
			b.proofCache[key] = proof
		}
		if proof.Status != constraints.Proven || !proof.Consistent {
			return proof
		}
	}
	if len(record.Assumptions) == 0 {
		return unknown
	}
	return constraints.Proof{Status: constraints.Proven, Reason: "caller environments imply every bound variant requirement", Consistent: true}
}

func better(a, b *Variant) bool {
	if b == nil {
		return true
	}
	if a.Cost.RemovedGuards != b.Cost.RemovedGuards {
		return a.Cost.RemovedGuards > b.Cost.RemovedGuards
	}
	if a.Cost.RemainingGuards != b.Cost.RemainingGuards {
		return a.Cost.RemainingGuards < b.Cost.RemainingGuards
	}
	if a.Cost.BodyUnits != b.Cost.BodyUnits {
		return a.Cost.BodyUnits < b.Cost.BodyUnits
	}
	return false // Stable registry order breaks ties; never human proof text.
}

func (b *specializer) variant(c *candidate) string {
	routes := map[*parser.GraphNode]string{}
	for _, call := range sortedNodes(c.children) {
		if name := b.variant(c.children[call]); name != "" {
			routes[call] = name
		}
	}
	d := Decision{Caller: c.caller, Path: pathIDs(c.path), Function: c.f.Decl.Name}
	if b.costs[c.f] == 0 {
		b.costs[c.f] = bodyUnits(c.f.Decl)
	}
	d.Cost.BodyUnits = b.costs[c.f]
	proven := map[*parser.GraphNode]bool{}
	for n := range b.local {
		proven[n] = true
	}
	for n := range c.checks {
		proven[n] = true
	}
	d.Cost.RemovedGuards = len(c.checks)
	for _, name := range routes {
		d.Cost.RemovedGuards += b.records[name].Cost.RemovedGuards
	}
	operations := map[*parser.GraphNode]bool{}
	for _, o := range b.functions[c.f.Decl.Name].Obligations {
		if len(o.Origin.Via) == 0 && o.Origin.SemanticNode != nil {
			operations[o.Origin.SemanticNode] = true
		}
	}
	for n := range operations {
		if !proven[n] {
			d.Cost.RemainingGuards++
		}
	}
	var best *Variant
	for _, v := range b.variants {
		if v.Function != c.f {
			continue
		}
		proof := b.applicable(c, v)
		d.Considered = append(d.Considered, VariantAssessment{v.Name, proof})
		if proof.Status == constraints.Proven && proof.Consistent && better(b.records[v.Name], best) {
			best = b.records[v.Name]
		}
	}
	finish := func(action, reason, target string) string {
		d.Action, d.Reason, d.Selected = action, reason, target
		b.decisions = append(b.decisions, d)
		return target
	}
	benefit := d.Cost.RemovedGuards
	if best != nil {
		benefit -= best.Cost.RemovedGuards
	}
	if best != nil && benefit <= 0 {
		return finish("REUSE", "applicable_variant_no_additional_benefit", best.Name)
	}
	if d.Cost.RemovedGuards == 0 {
		return finish("FALLBACK", "no_proven_guard_benefit", "")
	}
	if d.Cost.BodyUnits > maxBodyUnits || d.Cost.BodyUnits > benefit*unitsPerAdditionalGuard {
		if best != nil {
			return finish("REUSE", "new_body_cost_exceeds_incremental_benefit", best.Name)
		}
		return finish("FALLBACK", "new_body_cost_exceeds_benefit", "")
	}
	if b.counts[c.f] >= maxVariantsPerFunction || len(b.variants) >= maxVariants || b.units+d.Cost.BodyUnits > maxAdditionalUnits {
		b.budgetExhausted = true
		if best != nil {
			return finish("REUSE", "budget_exhausted_reuse", best.Name)
		}
		return finish("FALLBACK", "budget_exhausted", "")
	}
	contract := []Requirement{}
	// Read the formal contract from this target's summary. Descendant contracts
	// carry relative call paths, not caller input names or textual predicates.
	for _, o := range b.functions[c.f.Decl.Name].Obligations {
		selected := len(o.Origin.Via) == 0 && c.checks[o.Origin.SemanticNode]
		if len(o.Origin.Via) > 0 {
			for n, name := range routes {
				if n.ID != o.Origin.Via[0] {
					continue
				}
				for _, child := range b.records[name].Assumptions {
					if child.Operation == o.Origin.Node && samePath(child.Path, o.Origin.Via[1:]) {
						selected = true
					}
				}
			}
		}
		if selected {
			contract = append(contract, Requirement{append([]string(nil), o.Origin.Via...), o.Origin.Node, o.ID, o.Guard, o.Required})
		}
	}
	name := fmt.Sprintf("lk_spec_%d", len(b.variants))
	v := parser.SpecializedFunction{Function: c.f, Name: name, Proven: proven, Routes: routes}
	record := &Variant{Name: name, Function: c.f.Decl.Name, Assumptions: contract, Parameters: append([]constraints.Fact(nil), b.functions[c.f.Decl.Name].InputFacts...), Semantics: checkedUnsignedSemantics, Cost: d.Cost}
	b.records[name] = record
	// Fail closed if a future summary representation cannot bind the complete
	// planned guard set. Current cardinality checks make this an exact count.
	if len(contract) != d.Cost.RemovedGuards || b.applicable(c, v).Status != constraints.Proven {
		delete(b.records, name)
		if best != nil {
			return finish("REUSE", "incomplete_new_variant_contract", best.Name)
		}
		return finish("FALLBACK", "incomplete_new_variant_contract", "")
	}
	b.counts[c.f]++
	b.units += d.Cost.BodyUnits
	b.variants = append(b.variants, v)
	return finish("CREATE", "proven_benefit_within_cost_and_budget", name)
}

func samePath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Static AST units approximate C generation/storage, not execution time. Every
// statement/expression and parameter costs a unit; no profile is consulted.
func bodyUnits(f *parser.FunctionStatement) int {
	var expr func(*parser.Expr) int
	expr = func(e *parser.Expr) int {
		if e == nil {
			return 0
		}
		n := 1 + expr(e.Left) + expr(e.Right)
		for _, a := range e.Args {
			n += expr(a)
		}
		return n
	}
	var block func(*parser.BlockStatement) int
	block = func(b *parser.BlockStatement) int {
		if b == nil {
			return 0
		}
		n := 1
		for _, s := range b.Statements {
			n++
			switch x := s.(type) {
			case *parser.LetStatement:
				n += expr(x.Expr)
			case *parser.ReturnStatement:
				n += expr(x.Value)
			case *parser.ExpressionStatement:
				n += expr(x.Expr)
			case *parser.IfStatement:
				n += expr(x.Condition) + block(x.Then) + block(x.Else)
			}
		}
		return n
	}
	return 1 + len(f.Parameters) + block(f.Body)
}
