package main

import (
	"math"
	"testing"
)

func testCase() *Case {
	return &Case{
		BaseMVA: 100,
		Buses: []Bus{
			{ID: "B1", Type: BusSlack, V: 1.02, ThetaDeg: 0, VMin: 0.95, VMax: 1.05},
			{ID: "B2", Type: BusPV, P: 80, V: 1.01, VMin: 0.95, VMax: 1.05},
			{ID: "B3", Type: BusPQ, P: -90, Q: -30, VMin: 0.92, VMax: 1.08},
			{ID: "B4", Type: BusPQ, P: -40, Q: -15, VMin: 0.92, VMax: 1.08},
			{ID: "B5", Type: BusPQ, P: -10, Q: -4, VMin: 0.92, VMax: 1.08},
		},
		Lines: []Line{
			{ID: "L12", From: "B1", To: "B2", R: 0.02, X: 0.06, B: 0.06, RateMVA: 120},
			{ID: "L13", From: "B1", To: "B3", R: 0.03, X: 0.08, B: 0.05, RateMVA: 100},
			{ID: "L23", From: "B2", To: "B3", R: 0.025, X: 0.07, B: 0.05, RateMVA: 90},
			{ID: "L34", From: "B3", To: "B4", R: 0.02, X: 0.05, B: 0.02, RateMVA: 60},
			{ID: "L24", From: "B2", To: "B4", R: 0.03, X: 0.09, B: 0.02, RateMVA: 45},
			{ID: "L45", From: "B4", To: "B5", R: 0.04, X: 0.10, B: 0.01, RateMVA: 25},
		},
	}
}

func TestBaseConverges(t *testing.T) {
	c := testCase()
	if err := validateCase(c); err != nil {
		t.Fatalf("validate: %v", err)
	}
	n := newNetwork(c, "")
	sol, ok, err := solvePowerFlow(n, n.buildYbus(), 50, 1e-8)
	if err != nil || !ok {
		t.Fatalf("base case failed: ok=%v err=%v", ok, err)
	}
	if sol.maxMis >= 1e-8 {
		t.Fatalf("residual too large: %g", sol.maxMis)
	}
	if math.Abs(sol.vmag[0]-1.02) > 1e-12 || sol.theta[0] != 0 {
		t.Fatalf("slack phasor not fixed: %v %v", sol.vmag[0], sol.theta[0])
	}
	if math.Abs(sol.vmag[1]-1.01) > 1e-12 {
		t.Fatalf("PV magnitude not fixed: %v", sol.vmag[1])
	}
	for _, v := range sol.vmag {
		if v < 0.9 || v > 1.1 {
			t.Fatalf("implausible voltage: %v", sol.vmag)
		}
	}
}

func TestScreeningStatuses(t *testing.T) {
	rep := screenCase(testCase())
	if rep.Base.Status != statusSafe {
		t.Fatalf("base should be safe, got %s (%s)", rep.Base.Status, rep.Base.Reason)
	}
	if len(rep.Outages) != 6 {
		t.Fatalf("want 6 outage scenarios, got %d", len(rep.Outages))
	}
	var overload, island bool
	for _, o := range rep.Outages {
		switch o.LineID {
		case "L34":
			if o.Status != statusViolated {
				t.Fatalf("L34 outage should violate (L24 overload), got %s", o.Status)
			}
			for _, v := range o.Violations {
				if v.Type == "line_overload" && v.ObjectID == "L24" {
					overload = true
				}
			}
		case "L45":
			if len(o.IslandedBuses) != 1 || o.IslandedBuses[0] != "B5" {
				t.Fatalf("L45 outage should island B5, got %v", o.IslandedBuses)
			}
			if o.Status == statusSafe {
				t.Fatalf("islanded scenario must not be safe")
			}
			island = true
		}
	}
	if !overload || !island {
		t.Fatalf("expected overload and islanding scenarios (overload=%v island=%v)", overload, island)
	}
}

func TestLineLossPositive(t *testing.T) {
	rep := screenCase(testCase())
	total := 0.0
	for _, l := range rep.Base.Lines {
		if l.LossMW <= 0 {
			t.Fatalf("line %s loss should be positive, got %g", l.ID, l.LossMW)
		}
		total += l.LossMW
	}
	if total <= 0 || total > 20 {
		t.Fatalf("implausible total loss %g MW", total)
	}
}

func TestValidationRejects(t *testing.T) {
	base := testCase()
	cases := map[string]func(*Case){
		"duplicate bus":  func(c *Case) { c.Buses = append(c.Buses[:4], c.Buses[0]) },
		"two slacks":     func(c *Case) { c.Buses[1].Type = BusSlack },
		"self loop":      func(c *Case) { c.Lines[0].To = c.Lines[0].From },
		"unknown bus":    func(c *Case) { c.Lines[0].To = "BX" },
		"duplicate line": func(c *Case) { c.Lines[1].ID = c.Lines[0].ID },
		"negative x":     func(c *Case) { c.Lines[0].X = -1 },
		"zero rating":    func(c *Case) { c.Lines[0].RateMVA = 0 },
		"bad base":       func(c *Case) { c.BaseMVA = 0 },
		"too few buses":  func(c *Case) { c.Buses = c.Buses[:1] },
		"bad limits":     func(c *Case) { c.Buses[2].VMin = 1.2 },
	}
	for name, mutate := range cases {
		c := *base
		c.Buses = append([]Bus(nil), base.Buses...)
		c.Lines = append([]Line(nil), base.Lines...)
		mutate(&c)
		if err := validateCase(&c); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestSingularNetworkUndetermined(t *testing.T) {
	c := testCase()
	// B5 hangs only off L45; removing it leaves B5 with zero admittance row.
	res := runScenario(c, "L45")
	if res.Status == statusSafe {
		t.Fatalf("disconnected PQ bus must not be safe")
	}
	if res.Status != statusUndetermined && res.Status != statusViolated {
		t.Fatalf("unexpected status %s", res.Status)
	}
}
