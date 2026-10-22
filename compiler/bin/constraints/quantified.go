package constraints

import (
	"math/big"

	"leek/bin/parser"
)

type Emptiness string

const (
	EmptyDomain      Emptiness = "EMPTY"
	NonemptyDomain   Emptiness = "NONEMPTY"
	MaybeEmptyDomain Emptiness = "POSSIBLY_EMPTY"
)

type IterationDomain struct {
	Node         string    `json:"node"`
	ElementType  string    `json:"element_type"`
	Start        ID        `json:"start"`
	End          ID        `json:"end"`
	EndExclusive bool      `json:"end_exclusive"`
	Ordering     string    `json:"ordering"`
	State        Emptiness `json:"state"`
	Elements     *Interval `json:"actual_element_interval,omitempty"`
	Count        *Interval `json:"cardinality,omitempty"`
	Environment  int       `json:"path_environment"`
	Proof        Proof     `json:"proof"`
}
type PrefixInvariant struct {
	Rule          string    `json:"rule"`
	Initial       *Interval `json:"initial"`
	MaxCount      string    `json:"max_count"`
	ElementUpper  string    `json:"element_upper"`
	PreviousState *Interval `json:"previous_state"`
	AllPrefixes   *Interval `json:"all_prefixes"`
	Proof         Proof     `json:"proof"`
}
type QuantifiedInfo struct {
	Domain        *IterationDomain `json:"domain"`
	State         Emptiness        `json:"state"`
	Value         ID               `json:"value"`
	Elements      *Interval        `json:"success_conditional_elements,omitempty"`
	Count         *Interval        `json:"cardinality,omitempty"`
	Environment   int              `json:"invocation_environment"`
	Prefix        *PrefixInvariant `json:"prefix,omitempty"`
	Proof         Proof            `json:"callback_safety"`
	PipelineProof Proof            `json:"pipeline_safety"`
}

func quantUnknown(reason string) Proof { return Proof{Unknown, reason, true} }
func asSpan(i *Interval) *span {
	if i == nil || i.Empty {
		return nil
	}
	return &span{integer(i.Min), integer(i.Max)}
}
func nonnegative(x *big.Int) *big.Int {
	if x.Sign() < 0 {
		return new(big.Int)
	}
	return x
}
func (s *functionState) typedSpan(id ID, typ string, c context) *span {
	domain, ok := valueBounds(typ)
	if !ok || id == unknown || !c.consistent {
		return nil
	}
	if m := s.b.a.reasoner(c).rangeOf(id); m != nil {
		if m.lo.Cmp(domain.lo) > 0 {
			domain.lo.Set(m.lo)
		}
		if m.hi.Cmp(domain.hi) < 0 {
			domain.hi.Set(m.hi)
		}
	}
	// An unsupported or impossible successful result gives no narrower fact.
	if domain.lo.Cmp(domain.hi) > 0 {
		return nil
	}
	return &domain
}
func (s *functionState) quantifiedSnapshot(c context, guard ID, domain, stage string) int {
	id := s.snapshot(c, guard)
	s.report.Environments[id].Domain = domain
	s.report.Environments[id].Stage = stage
	return id
}
func (s *functionState) rangeDomain(r *DataflowInfo, c context) *IterationDomain {
	a := s.b.a
	domain := &IterationDomain{Node: r.Node, ElementType: r.ElementType, Start: *r.Start, End: *r.End, EndExclusive: true, Ordering: r.Order,
		Environment: s.snapshot(c, r.Guard), Proof: quantUnknown("endpoint facts unavailable"), State: MaybeEmptyDomain}
	start, end := s.typedSpan(*r.Start, r.ElementType, c), s.typedSpan(*r.End, r.ElementType, c)
	if start == nil || end == nil {
		return domain
	}
	maxCount := nonnegative(new(big.Int).Sub(end.hi, start.lo))
	minCount := nonnegative(new(big.Int).Sub(end.lo, start.hi))
	if *r.Start == *r.End || a.prove(a.op("<=", *r.End, *r.Start), c).Status == Proven {
		maxCount.SetInt64(0)
		minCount.SetInt64(0)
	}
	domain.Count = interval(span{minCount, maxCount})
	domain.Proof = Proof{Proven, "exact half-open interval enclosure and mathematical cardinality bounds at this path", true}
	if maxCount.Sign() == 0 {
		domain.State = EmptyDomain
		return domain
	}
	if minCount.Sign() > 0 {
		domain.State = NonemptyDomain
	}
	domain.Elements = interval(span{new(big.Int).Set(start.lo), new(big.Int).Sub(end.hi, big.NewInt(1))})
	return domain
}
func (s *functionState) quantify(e *parser.Expr, r *DataflowInfo) {
	a := s.b.a
	d := s.b.g.Expressions[e].Dataflow
	c := a.refine(s.context, r.Guard)
	q := &QuantifiedInfo{State: MaybeEmptyDomain, Value: r.Element, Environment: -1, Proof: quantUnknown("unsupported invocation domain"), PipelineProof: quantUnknown("upstream or callback safety unresolved")}
	r.Quantified = q
	defer func() { r.Proof = q.Proof }()
	if d.Kind == parser.Range {
		q.Domain = s.rangeDomain(r, c)
		q.State = q.Domain.State
		q.Elements = q.Domain.Elements
		q.Count = q.Domain.Count
		q.Proof = q.Domain.Proof
		q.PipelineProof = q.Domain.Proof
		return
	}
	var incoming *QuantifiedInfo
	for i := len(s.report.Dataflow) - 1; i >= 0; i-- {
		if s.report.Dataflow[i].Node == r.Input {
			incoming = s.report.Dataflow[i].Quantified
			break
		}
	}
	if incoming == nil || incoming.Domain == nil {
		return
	}
	q.Domain = incoming.Domain
	q.State = incoming.State
	q.Count = incoming.Count
	if q.Domain.Proof.Status != Proven || !c.consistent {
		return
	}
	if q.State == EmptyDomain {
		q.Proof = Proof{Proven, "no actual callback invocations; no element environment constructed", true}
		q.PipelineProof = incoming.PipelineProof
		for i := range r.Requirements {
			r.Requirements[i].Proof = q.Proof
		}
		return // no optimization witnesses or fictitious element
	}
	input := asSpan(incoming.Elements)
	if input == nil {
		return
	}
	// The original scalar summary was substituted onto this exact incoming value.
	c.put(incoming.Value, copySpan(*input))
	if r.Accumulator != nil {
		state := s.typedSpan(*r.Initial, d.StateType, c)
		bounds, ok := valueBounds(d.StateType)
		if !ok {
			return
		}
		if state == nil {
			return
		}
		c.put(*r.Accumulator, bounds) // any reached typed state, not an assumed initial
		if d.Kind == parser.Reduce && len(d.StateType) > 0 && d.StateType[0] == 'u' && input.lo.Sign() >= 0 {
			params := s.b.params[d.Callback]
			summary := s.b.summaries[d.Callback]
			if len(params) == 2 && summary.Complete && summary.Output != nil &&
				(summary.Output.Relation == a.op("+", params[0], params[1]) || summary.Output.Relation == a.op("+", params[1], params[0])) {
				count := asSpan(q.Count)
				if count != nil && count.hi.Sign() > 0 {
					beforeCount := new(big.Int).Sub(count.hi, big.NewInt(1))
					beforeMax := new(big.Int).Add(state.hi, new(big.Int).Mul(beforeCount, input.hi))
					afterMax := new(big.Int).Add(state.hi, new(big.Int).Mul(count.hi, input.hi))
					previous := span{new(big.Int).Set(state.lo), new(big.Int).Set(beforeMax)}
					if previous.hi.Cmp(bounds.hi) > 0 {
						previous.hi.Set(bounds.hi)
					}
					c.put(*r.Accumulator, previous)
					proof := a.prove(a.op("<=", a.number(afterMax), a.number(bounds.hi)), newContext())
					if proof.Status != Proven {
						proof = quantUnknown("conservative all-prefix bound exceeds the state type")
					}
					q.Prefix = &PrefixInvariant{Rule: "ordered-unsigned-addition-v1", Initial: interval(*state), MaxCount: count.hi.String(), ElementUpper: input.hi.String(),
						PreviousState: interval(previous), AllPrefixes: interval(span{new(big.Int).Set(state.lo), afterMax}), Proof: proof}
				}
			}
		}
	}
	q.Environment = s.quantifiedSnapshot(c, r.Guard, q.Domain.Node, r.Node)
	complete := s.b.summaries[d.Callback].Complete
	all := complete
	for i := range r.Requirements {
		o := &r.Requirements[i]
		premises := a.refine(c, o.Guard)
		o.Environment = s.quantifiedSnapshot(premises, r.Guard, q.Domain.Node, r.Node)
		o.Proof = a.prove(o.Required, premises)
		if o.Proof.Status != Proven && premises.consistent {
			all = false
		}
		if o.Proof.Status != Proven {
			o.Proof = Proof{Unknown, "universal safety not established at a reached scalar operation", premises.consistent}
		}
		// Separate from scalar success obligations. Original operation and ordered
		// callback/callee path bind each witness; requirements are never assumptions.
		if len(s.report.QuantifiedObligations) >= maxObligations {
			s.limit("quantified obligation budget exhausted")
			all = false
			break
		}
		o.ID = len(s.report.QuantifiedObligations)
		s.report.QuantifiedObligations = append(s.report.QuantifiedObligations, *o)
	}
	if all {
		q.Proof = Proof{Proven, "every retained scalar safety obligation follows from this actual-invocation domain", true}
	}
	if all && incoming.PipelineProof.Status == Proven {
		q.PipelineProof = Proof{Proven, "all upstream and callback obligations established without assuming their requirements", true}
	}
	switch d.Kind {
	case parser.Map:
		if out := s.typedSpan(r.Relation, d.ElementType, c); out != nil {
			q.Elements = interval(*out)
		}
	case parser.Filter:
		q.Value = r.Relation
		retained := a.refine(c, *r.Predicate)
		count := asSpan(incoming.Count)
		if count != nil {
			q.Count = interval(span{new(big.Int), new(big.Int).Set(count.hi)})
		}
		q.State = MaybeEmptyDomain
		if !retained.consistent {
			q.State = EmptyDomain
			q.Count = &Interval{Min: "0", Max: "0"}
		} else if bound, ok := retained.ranges[incoming.Value]; ok {
			q.Elements = interval(bound)
		}
		// No claim that an arbitrary filter reduces maximum cardinality.
	case parser.Reduce, parser.Fold:
		// A prefix rule covers states, never reorders the recurrence.
	}
	if a.limited {
		s.limit("symbolic term budget exhausted")
	}
}
