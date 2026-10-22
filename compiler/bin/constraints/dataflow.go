package constraints

import "leek/bin/parser"

// These are per-element, success-conditional facts, NEVER scalar premises or
// proof capabilities. Universal sequence safety is not solved by this MVP.
type DataflowInfo struct {
	Materialization  string              `json:"materialization"`
	Node             string              `json:"node"`
	Kind             parser.DataflowKind `json:"kind"`
	Input            string              `json:"input,omitempty"`
	ElementType      string              `json:"element_type"`
	StateType        string              `json:"state_type,omitempty"`
	Order            string              `json:"order"`
	Guard            ID                  `json:"guard"`
	Start            *ID                 `json:"start,omitempty"`
	End              *ID                 `json:"end,omitempty"`
	Initial          *ID                 `json:"initial,omitempty"`
	Accumulator      *ID                 `json:"accumulator,omitempty"`
	AccumulatorRange *Interval           `json:"accumulator_range,omitempty"`
	Predicate        *ID                 `json:"predicate,omitempty"`
	Element          ID                  `json:"element"`
	Relation         ID                  `json:"success_conditional_relation"`
	ElementRange     *Interval           `json:"element_range,omitempty"`
	DomainFacts      []ID                `json:"domain_facts,omitempty"`
	Callback         string              `json:"callback,omitempty"`
	Quantifier       string              `json:"quantifier,omitempty"`
	Requirements     []Obligation        `json:"element_requirements,omitempty"`
	Proof            Proof               `json:"proof"`
}

func (s *functionState) dataflow(e *parser.Expr, guard ID) ID {
	a := s.b.a
	info := s.b.g.Expressions[e]
	d := info.Dataflow
	record := DataflowInfo{Materialization: d.Materialization, Node: info.Node.ID, Kind: d.Kind, ElementType: d.ElementType, StateType: d.StateType, Order: d.Order, Guard: guard,
		Proof: Proof{Unknown, "quantified sequence safety is not established", true}}
	record.Element = a.raw("opaque", info.Node.ID+":logical-element")
	if bounds, ok := valueBounds(d.ElementType); ok {
		record.ElementRange = interval(bounds)
	}
	result := record.Element
	if d.Kind == parser.Range {
		start, end := s.expr(d.Start, guard), s.expr(d.End, guard)
		record.Start, record.End = &start, &end
		record.DomainFacts = []ID{a.op(">=", record.Element, start), a.op("<", record.Element, end)}
		record.Relation = record.Element
	} else {
		input := s.expr(d.Input, guard)
		record.Input = s.b.g.Expressions[d.Input].Node.ID
		var state ID
		if d.Initial != nil {
			initial := s.expr(d.Initial, guard)
			record.Initial = &initial
		}
		summary := s.b.summaries[d.Callback]
		mapping := map[ID]ID{}
		actuals := []ID{input}
		if d.Initial != nil {
			// The initial value is not the value of every later accumulator.
			state = a.raw("opaque", info.Node.ID+":logical-accumulator")
			record.Accumulator = &state
			if bounds, ok := valueBounds(d.StateType); ok {
				record.AccumulatorRange = interval(bounds)
			}
			actuals = []ID{state, input}
		}
		for i, p := range s.b.params[d.Callback] {
			if p != unknown {
				mapping[p] = actuals[i]
			}
		}
		memo := map[ID]ID{}
		record.Callback = d.Callback.Decl.Name
		record.Quantifier = "for every reached callback invocation in logical element order"
		for _, o := range summary.Obligations {
			if len(record.Requirements) >= maxObligations {
				s.limit("dataflow obligation budget exhausted")
				break
			}
			origin := o.Origin
			origin.Via = append([]string{info.Node.ID}, origin.Via...)
			g, r := a.substitute(o.Guard, mapping, memo), a.substitute(o.Required, mapping, memo)
			record.Requirements = append(record.Requirements, Obligation{ID: len(record.Requirements), Environment: -1, Guard: g, Required: r, Effective: a.op("or", a.op("not", g), r), Origin: origin,
				Proof: Proof{Unknown, "per-invocation requirement retained; no quantified discharge", true}})
		}
		if summary.Output != nil {
			record.Relation = a.substitute(summary.Output.Relation, mapping, memo)
		} else {
			record.Relation = unknown
		}
		if d.Kind == parser.Filter {
			predicate := record.Relation
			record.Predicate = &predicate
			record.Relation = input
			result = input
		}
		if d.Kind == parser.Reduce || d.Kind == parser.Fold {
			s.incomplete("quantified sequence and accumulator constraints are not discharged")
			s.require(guard, unknown, s.origin(e))
			result = s.opaque(e, info.Type, "sequence result is conditional on all reached callbacks succeeding")
		}
	}
	s.report.Dataflow = append(s.report.Dataflow, record)
	return result
}
