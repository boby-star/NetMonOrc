package monitor

import "testing"

func TestApprovedZoneAggregationTable(t *testing.T) {
	for _, tc := range []struct {
		states   []string
		all, any string
	}{
		{nil, "unknown", "unknown"},
		{[]string{"healthy"}, "healthy", "healthy"},
		{[]string{"healthy", "healthy"}, "healthy", "healthy"},
		{[]string{"healthy", "down"}, "degraded", "healthy"},
		{[]string{"down", "down"}, "down", "down"},
		{[]string{"healthy", "degraded"}, "degraded", "healthy"},
		{[]string{"degraded", "down"}, "degraded", "degraded"},
		{[]string{"healthy", "unknown"}, "unknown", "healthy"},
		{[]string{"down", "unknown"}, "degraded", "degraded"},
		{[]string{"unknown", "unknown"}, "unknown", "unknown"},
		{[]string{"healthy", "degraded", "unknown"}, "degraded", "healthy"},
		{[]string{"degraded", "unknown"}, "degraded", "degraded"},
		{[]string{"healthy", "down", "unknown"}, "degraded", "healthy"},
	} {
		zones := []ZoneAssessment{}
		for _, state := range tc.states {
			zones = append(zones, ZoneAssessment{State: state})
		}
		for _, mode := range []string{"", "all-required", "any"} {
			want := tc.all
			if mode == "any" {
				want = tc.any
			}
			if got := aggregateZones(mode, zones); got != want {
				t.Fatalf("%s %v: got %s want %s", mode, tc.states, got, want)
			}
		}
	}
	if _, err := NewAssessor(Plan{ZoneAggregation: "majority"}); err == nil {
		t.Fatal("unknown zone policy accepted")
	}
}
