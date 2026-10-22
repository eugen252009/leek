package constraints

import (
	"fmt"
	"math/big"
	"strconv"
	"testing"
)

// Q-ORACLE: independently enumerate small domains using explicit u8 arithmetic,
// not symbolic transfer, interval propagation, or analyzer-derived guards.
func checkQuantifiedCase(t testing.TB, limit, start, initial int, operation, predicate byte) {
	t.Helper()
	bodies := []string{"return x+1", "return x-1", "return x*x", "return 100/x", "return x%7", "if x<10 { return x } return 10"}
	filters := []string{"", " -> filter(even)", " -> filter(small)"}
	source := fmt.Sprintf(`fn transform(x u8) u8 { %s }
 fn even(x u8) bool { return x%%2==0 } fn small(x u8) bool { return x<10 }
 fn add(a u8,b u8) u8 { return a+b }
 fn f(n u8) u8 { if n<=%d { return range(%d,n)%s -> map(transform) -> reduce(%d,add) } return 0 }`, bodies[int(operation)%6], limit, start, filters[int(predicate)%3], initial)
	r := reportFor(t, source)
	f := function(t, r, "f")
	d := f.Dataflow
	domain := d[0].Quantified
	mapped := d[len(d)-2].Quantified
	reduced := d[len(d)-1].Quantified
	visits := 0
	for n := 0; n <= limit; n++ {
		count := n - start
		if count < 0 {
			count = 0
		}
		maxCount, _ := strconv.Atoi(domain.Count.Max)
		minCount, _ := strconv.Atoi(domain.Count.Min)
		if count > maxCount || count < minCount || domain.State == EmptyDomain && count != 0 || domain.State == NonemptyDomain && count == 0 {
			t.Fatalf("unsound cardinality %+v at n=%d\n%s", domain, n, source)
		}
		retained := 0
		acc := initial
		stopped := false
		for x := start; x < n; x++ {
			visits++
			if domain.Elements == nil {
				t.Fatal("nonempty domain has no element bound")
			}
			contained(t, domain.Elements, x, source)
			keep := true
			if predicate%3 == 1 {
				keep = x%2 == 0
			}
			if predicate%3 == 2 {
				keep = x < 10
			}
			if !keep {
				continue
			}
			retained++
			value := 0
			safe := true
			switch operation % 6 {
			case 0:
				value = x + 1
			case 1:
				value = x - 1
			case 2:
				value = x * x
			case 3:
				if x == 0 {
					safe = false
				} else {
					value = 100 / x
				}
			case 4:
				value = x % 7
			case 5:
				value = x
				if x >= 10 {
					value = 10
				}
			}
			safe = safe && value >= 0 && value <= 255
			if mapped.Proof.Status == Proven && !safe {
				t.Fatalf("PROVEN map fails at %d\n%s", x, source)
			}
			if safe && mapped.Elements != nil {
				contained(t, mapped.Elements, value, source)
			}
			if stopped {
				continue
			}
			if !safe {
				stopped = true
				continue
			}
			next := acc + value
			if reduced.Prefix != nil {
				contained(t, reduced.Prefix.AllPrefixes, acc, source)
				contained(t, reduced.Prefix.AllPrefixes, next, source)
				contained(t, reduced.Prefix.PreviousState, acc, source)
			}
			if reduced.Proof.Status == Proven && next > 255 {
				t.Fatalf("PROVEN reduction overflows: %d+%d\n%s", acc, value, source)
			}
			if next > 255 {
				stopped = true
			} else {
				acc = next
			}
		}
		upper, _ := strconv.Atoi(mapped.Count.Max)
		if retained > upper {
			t.Fatalf("filter/cardinality underestimated %d > %d\n%s", retained, upper, source)
		}
	}
	_ = visits
}
func contained(t testing.TB, interval *Interval, value int, source string) {
	t.Helper()
	if interval == nil || interval.Empty || big.NewInt(int64(value)).Cmp(integer(interval.Min)) < 0 || big.NewInt(int64(value)).Cmp(integer(interval.Max)) > 0 {
		t.Fatalf("bound %+v does not contain %d\n%s", interval, value, source)
	}
}
func TestQuantifiedBoundsExhaustiveSmallDomains(t *testing.T) {
	cases := 0
	for limit := 0; limit <= 20; limit++ {
		for start := 0; start <= 4; start++ {
			for _, initial := range []int{0, 7, 250} {
				for op := byte(0); op < 6; op++ {
					for filter := byte(0); filter < 3; filter++ {
						checkQuantifiedCase(t, limit, start, initial, op, filter)
						cases++
					}
				}
			}
		}
	}
	t.Logf("%d programs; every n in 0..limit and every represented u8 element/prefix independently checked", cases)
}
func FuzzQuantifiedBounds(f *testing.F) {
	for i := byte(0); i < 12; i++ {
		f.Add(i, i%5, i*21, i%6, i%3)
	}
	f.Fuzz(func(t *testing.T, limit, start, initial, operation, predicate byte) {
		checkQuantifiedCase(t, int(limit%33), int(start%9), int(initial), operation, predicate)
	})
}
