package pf

import (
	"math"
	"testing"
)

func smallCase() *Case {
	c := &Case{
		BaseMVA:       100,
		MaxIterations: 20,
		TolerancePU:   1e-10,
		Buses: []Bus{
			{ID: "B1", Type: BusSlack, Vmin: 0.9, Vmax: 1.1, V: 1.02},
			{ID: "B2", Type: BusPQ, Vmin: 0.9, Vmax: 1.1, P: -120, Q: -40},
			{ID: "B3", Type: BusPQ, Vmin: 0.9, Vmax: 1.1, P: -80, Q: -20},
			{ID: "B4", Type: BusPV, Vmin: 0.9, Vmax: 1.1, P: 100, V: 1.0},
		},
		Lines: []Line{
			{ID: "L12", From: "B1", To: "B2", R: 0.02, X: 0.08, B: 0.02, Capacity: 200},
			{ID: "L13", From: "B1", To: "B3", R: 0.03, X: 0.10, B: 0.02, Capacity: 200},
			{ID: "L23", From: "B2", To: "B3", R: 0.025, X: 0.09, B: 0.015, Capacity: 200},
			{ID: "L24", From: "B2", To: "B4", R: 0.015, X: 0.06, B: 0.02, Capacity: 200},
			{ID: "L34", From: "B3", To: "B4", R: 0.02, X: 0.07, B: 0.018, Capacity: 200},
		},
	}
	return c
}

func TestBaseCaseConvergesAndBalances(t *testing.T) {
	c := smallCase()
	if err := c.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	n, err := BuildNetwork(c, "")
	if err != nil {
		t.Fatal(err)
	}
	sol, err := n.Solve()
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if sol.MaxResPU > 1e-8 {
		t.Fatalf("residual too large: %g", sol.MaxResPU)
	}
	// Global real-power balance: slack plus PV injection equals load plus losses.
	var totalP float64
	for _, p := range sol.PInjPU {
		totalP += p
	}
	flows := n.LineFlows(sol)
	var losses float64
	for _, f := range flows {
		losses += f.PLossMW
	}
	if math.Abs(totalP*c.BaseMVA-losses) > 1e-4 {
		t.Fatalf("injection sum %.6f MW does not equal losses %.6f MW", totalP*c.BaseMVA, losses)
	}
	for i, v := range sol.VoltagePU {
		if v < 0.9 || v > 1.1 {
			t.Fatalf("bus %d voltage %.4f outside wide test limits", i, v)
		}
	}
}

func TestPVoltageFixedAndSlackPhasorFixed(t *testing.T) {
	c := smallCase()
	n, _ := BuildNetwork(c, "")
	sol, err := n.Solve()
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sol.VoltagePU[3]-1.0) > 1e-11 {
		t.Fatalf("PV magnitude moved: %.12f", sol.VoltagePU[3])
	}
	if math.Abs(sol.VoltagePU[0]-1.02) > 1e-11 || math.Abs(sol.AngleRad[0]) > 1e-11 {
		t.Fatalf("slack phasor moved: v=%g angle=%g", sol.VoltagePU[0], sol.AngleRad[0])
	}
}

func TestOutageIndependenceAndIslandDetection(t *testing.T) {
	c := smallCase()
	report := Screen(c)
	if report.BaseCase.Status != StatusSafe {
		t.Fatalf("base status = %s, want safe (%+v)", report.BaseCase.Status, report.BaseCase.Violations)
	}
	if len(report.Contingencies) != len(c.Lines) {
		t.Fatalf("contingencies = %d", len(report.Contingencies))
	}
	for _, sc := range report.Contingencies {
		if sc.OutagedLineID == "" || sc.Status == "" {
			t.Fatalf("scenario missing identifiers: %+v", sc)
		}
	}

	radial := smallCase()
	radial.Lines = radial.Lines[:1]
	radial.Buses = radial.Buses[:2]
	// Rebuild a two-bus/one-line case explicitly.
	radial = &Case{
		BaseMVA: 100,
		Buses: []Bus{
			{ID: "S", Type: BusSlack, Vmin: .9, Vmax: 1.1, V: 1},
			{ID: "L", Type: BusPQ, Vmin: .9, Vmax: 1.1, P: -50, Q: -10},
		},
		Lines: []Line{{ID: "X", From: "S", To: "L", R: .01, X: .05, B: .01, Capacity: 100}},
	}
	r := Screen(radial)
	if len(r.Contingencies) != 1 || r.Contingencies[0].Status != StatusDeenergized {
		t.Fatalf("radial outage should be deenergized: %+v", r.Contingencies)
	}
	if len(r.Contingencies[0].IslandedBuses) != 1 || r.Contingencies[0].IslandedBuses[0] != "L" {
		t.Fatalf("unexpected islands: %+v", r.Contingencies[0].IslandedBuses)
	}
}

func TestValidationRules(t *testing.T) {
	cases := map[string]func(*Case){
		"self loop":     func(c *Case) { c.Lines[0].To = c.Lines[0].From },
		"duplicate bus": func(c *Case) { c.Buses[1].ID = c.Buses[0].ID },
		"unknown bus":   func(c *Case) { c.Lines[0].To = "NOPE" },
		"zero x":        func(c *Case) { c.Lines[0].X = 0 },
		"negative b":    func(c *Case) { c.Lines[0].B = -1 },
		"two slack":     func(c *Case) { c.Buses[1].Type = BusSlack; c.Buses[1].V = 1 },
		"nonfinite":     func(c *Case) { c.Buses[1].P = math.NaN() },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := smallCase()
			mutate(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation failure for %s", name)
			}
		})
	}
}
