package adblock

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func cosmeticFilters(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("##.ad")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	return b.String()
}

func TestConvertHidingGroups(t *testing.T) {
	for _, n := range []int{0, 1, 99, 100, 101, 200, 201, 1000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			var r Rules
			if skipped := r.Convert(cosmeticFilters(n)); skipped != 0 {
				t.Fatalf("Convert(%d selectors) skipped = %d, want 0", n, skipped)
			}
			wantRules := (n + selectorsPerRule - 1) / selectorsPerRule
			if len(r.Hide) != wantRules {
				t.Errorf("Convert(%d selectors) rules = %d, want %d", n, len(r.Hide), wantRules)
			}
			var selectors []string
			for _, rule := range r.Hide {
				group := strings.Split(rule.Action.Selector, ", ")
				if len(group) > selectorsPerRule {
					t.Errorf("Convert(%d selectors) group size = %d, want <= %d", n, len(group), selectorsPerRule)
				}
				selectors = append(selectors, group...)
			}
			var want []string
			for i := range n {
				want = append(want, ".ad"+strconv.Itoa(i))
			}
			if !slices.Equal(selectors, want) {
				t.Errorf("Convert(%d selectors) selectors = %v, want %v", n, selectors, want)
			}
		})
	}
}

func BenchmarkConvertCosmeticLists(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			filters := cosmeticFilters(n)
			b.SetBytes(int64(len(filters)))
			b.ReportAllocs()
			var rules int
			for b.Loop() {
				var r Rules
				r.Convert(filters)
				r.Lists()
				rules = len(r.Hide)
			}
			b.ReportMetric(float64(rules), "rules/op")
		})
	}
}
