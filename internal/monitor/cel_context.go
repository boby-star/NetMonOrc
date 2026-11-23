package monitor

import "google.golang.org/protobuf/types/known/anypb"

type RuntimeMonitorView struct {
	Name          string            `cel:"name"`
	State         string            `cel:"state"`
	CoverageState string            `cel:"coverageState"`
	Labels        map[string]string `cel:"labels"`
	WeightedScore *float64          `cel:"weightedScore"`
}
type EvaluatedZoneView struct {
	Name          string               `cel:"name"`
	State         string               `cel:"state"`
	Coverage      string               `cel:"coverage"`
	Labels        map[string]string    `cel:"labels"`
	Checks        map[string]CheckView `cel:"checks"`
	WeightedScore *float64             `cel:"weightedScore"`
}

func MonitorContext(p Plan, a Assessment) RuntimeMonitorView {
	v := RuntimeMonitorView{Name: a.Monitor, State: a.State, CoverageState: "sufficient", Labels: p.Labels}
	count := 0
	for _, zone := range a.Zones {
		count += len(zone.Evidence)
		if zone.Coverage != "complete" {
			v.CoverageState = "degraded"
		}
	}
	if len(a.Zones) == 0 || count == 0 {
		v.CoverageState = "unknown"
	}
	// A multi-zone scalar has no normative weighting. Never invent an average.
	if len(a.Zones) == 1 {
		v.WeightedScore = a.Zones[0].H
	}
	return v
}
func TargetCELContext(p Plan) TargetView {
	v := TargetView{Address: p.TargetAddress, Type: "endpoint", NetworkZone: "unspecified", Criticality: 3, Labels: map[string]string{}, Attributes: map[string]*anypb.Any{}}
	if t := p.TargetContext; t != nil {
		v.Type = t.Type
		v.NetworkZone = t.NetworkZone
		v.Criticality = t.Criticality
		v.Labels = t.Labels
		v.Attributes = ScalarValues(t.Attributes)
	}
	return v
}
