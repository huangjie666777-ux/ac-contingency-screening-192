package main

import (
	"math"

	"github.com/huangjie666777-ux/ac-contingency-screening-192/internal/cx"
)

// Scenario statuses.
const (
	statusSafe         = "safe"
	statusViolated     = "violated"
	statusUndetermined = "undetermined"
)

// BusResult is the solved state of one bus.
type BusResult struct {
	ID       string  `json:"id"`
	V        float64 `json:"v_pu"`
	ThetaDeg float64 `json:"theta_deg"`
}

// LineResult reports power flowing into the line at each end and the loss.
type LineResult struct {
	ID     string  `json:"id"`
	FromP  float64 `json:"from_p_mw"`
	FromQ  float64 `json:"from_q_mvar"`
	ToP    float64 `json:"to_p_mw"`
	ToQ    float64 `json:"to_q_mvar"`
	LossMW float64 `json:"loss_mw"`
}

// Violation is one limit breach in a converged scenario.
type Violation struct {
	Type     string  `json:"type"` // "line_overload" or "voltage"
	ObjectID string  `json:"object_id"`
	Value    float64 `json:"value"`
	Limit    float64 `json:"limit"`
}

// ScenarioResult is the outcome of one network state (base or one outage).
type ScenarioResult struct {
	LineID        string       `json:"line_id,omitempty"`
	Status        string       `json:"status"`
	Converged     bool         `json:"converged"`
	Iterations    int          `json:"iterations"`
	MaxMismatch   float64      `json:"max_mismatch_pu"`
	IslandedBuses []string     `json:"islanded_buses"`
	Reason        string       `json:"reason,omitempty"`
	Buses         []BusResult  `json:"buses,omitempty"`
	Lines         []LineResult `json:"lines,omitempty"`
	Violations    []Violation  `json:"violations"`
}

// runScenario solves one network state and evaluates all limits.
func runScenario(c *Case, outagedLineID string) ScenarioResult {
	maxIter, tol := c.solverConfig()
	n := newNetwork(c, outagedLineID)
	res := ScenarioResult{
		LineID:        outagedLineID,
		IslandedBuses: n.islandedBuses(),
		Violations:    []Violation{},
	}
	if res.IslandedBuses == nil {
		res.IslandedBuses = []string{}
	}

	y := n.buildYbus()
	sol, converged, err := solvePowerFlow(n, y, maxIter, tol)
	if err != nil {
		res.Status = statusUndetermined
		res.Reason = err.Error()
		return res
	}
	res.Iterations = sol.iter
	res.MaxMismatch = sol.maxMis
	res.Converged = converged
	if !converged {
		res.Status = statusUndetermined
		res.Reason = "newton-raphson did not converge within the iteration limit"
		return res
	}

	base := c.BaseMVA
	for i, b := range c.Buses {
		res.Buses = append(res.Buses, BusResult{
			ID:       b.ID,
			V:        sol.vmag[i],
			ThetaDeg: sol.theta[i] * 180 / math.Pi,
		})
		if sol.vmag[i] > b.VMax {
			res.Violations = append(res.Violations, Violation{"voltage", b.ID, sol.vmag[i], b.VMax})
		}
		if sol.vmag[i] < b.VMin {
			res.Violations = append(res.Violations, Violation{"voltage", b.ID, sol.vmag[i], b.VMin})
		}
	}

	for _, li := range n.active {
		l := c.Lines[li]
		f, t := n.busIndex[l.From], n.busIndex[l.To]
		vf := cx.C{Re: sol.vmag[f] * math.Cos(sol.theta[f]), Im: sol.vmag[f] * math.Sin(sol.theta[f])}
		vt := cx.C{Re: sol.vmag[t] * math.Cos(sol.theta[t]), Im: sol.vmag[t] * math.Sin(sol.theta[t])}
		ys := cx.C{Re: l.R, Im: l.X}.Inv()
		sh := cx.C{Im: l.B / 2}
		sFrom := vf.Mul(vf.Sub(vt).Mul(ys).Add(vf.Mul(sh)).Conj())
		sTo := vt.Mul(vt.Sub(vf).Mul(ys).Add(vt.Mul(sh)).Conj())
		res.Lines = append(res.Lines, LineResult{
			ID:     l.ID,
			FromP:  sFrom.Re * base,
			FromQ:  sFrom.Im * base,
			ToP:    sTo.Re * base,
			ToQ:    sTo.Im * base,
			LossMW: (sFrom.Re + sTo.Re) * base,
		})
		loading := math.Max(sFrom.Abs(), sTo.Abs()) * base
		if loading > l.RateMVA {
			res.Violations = append(res.Violations, Violation{"line_overload", l.ID, loading, l.RateMVA})
		}
	}

	// A scenario is safe only when converged, free of violations, and with no
	// bus cut off from the slack (no loss of supply).
	if len(res.Violations) == 0 && len(res.IslandedBuses) == 0 {
		res.Status = statusSafe
	} else {
		res.Status = statusViolated
	}
	return res
}

// ScreeningReport is the full N-1 screening response.
type ScreeningReport struct {
	BaseMVA float64          `json:"base_mva"`
	Base    ScenarioResult   `json:"base"`
	Outages []ScenarioResult `json:"outages"`
}

// screenCase solves the base case and then every single-line outage in input
// order, each from an independent fresh start. A failed scenario never
// affects the following ones.
func screenCase(c *Case) *ScreeningReport {
	rep := &ScreeningReport{BaseMVA: c.BaseMVA, Outages: []ScenarioResult{}}
	rep.Base = runScenario(c, "")
	for _, l := range c.Lines {
		rep.Outages = append(rep.Outages, runScenario(c, l.ID))
	}
	return rep
}
