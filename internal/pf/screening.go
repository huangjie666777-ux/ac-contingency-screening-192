package pf

import "math"

// Scenario statuses. "safe" requires convergence, slack connectivity, and zero
// overload or voltage-limit violations.
const (
	StatusSafe         = "safe"
	StatusViolation    = "violation"
	StatusDeenergized  = "deenergized"
	StatusUndetermined = "undetermined"
)

// VoltageDTO is the JSON representation of a converged bus voltage.
type VoltageDTO struct {
	BusID    string  `json:"bus_id"`
	VMagPU   float64 `json:"v_pu"`
	AngleDeg float64 `json:"angle_deg"`
}

// LineFlowDTO is the JSON representation of both-end branch power and loss.
type LineFlowDTO struct {
	LineID  string  `json:"line_id"`
	FromBus string  `json:"from_bus"`
	ToBus   string  `json:"to_bus"`
	FromP   float64 `json:"from_p_mw"`
	FromQ   float64 `json:"from_q_mvar"`
	ToP     float64 `json:"to_p_mw"`
	ToQ     float64 `json:"to_q_mvar"`
	PLossMW float64 `json:"p_loss_mw"`
	FromMVA float64 `json:"from_mva"`
	ToMVA   float64 `json:"to_mva"`
}

// ScenarioResult is the result for the base case or one line outage.
type ScenarioResult struct {
	OutagedLineID  string        `json:"outaged_line_id,omitempty"`
	Status         string        `json:"status"`
	IslandedBuses  []string      `json:"islanded_buses,omitempty"`
	Message        string        `json:"message,omitempty"`
	Iterations     int           `json:"iterations,omitempty"`
	MaxResidualPU  float64       `json:"max_power_residual_pu,omitempty"`
	MaxResidualMVA float64       `json:"max_power_residual_mva,omitempty"`
	Voltages       []VoltageDTO  `json:"voltages,omitempty"`
	LineFlows      []LineFlowDTO `json:"line_flows,omitempty"`
	Violations     []Violation   `json:"violations,omitempty"`
}

// Report is the full screening response.
type Report struct {
	BaseMVA       float64          `json:"base_mva"`
	BaseCase      ScenarioResult   `json:"base_case"`
	Contingencies []ScenarioResult `json:"contingencies"`
}

func roundFloat(x float64) float64 {
	if math.Abs(x) < 1e-12 {
		return 0
	}
	return math.Round(x*1e9) / 1e9
}

func toDTOs(bvs []BusVoltage) []VoltageDTO {
	out := make([]VoltageDTO, len(bvs))
	for i, v := range bvs {
		out[i] = VoltageDTO{
			BusID:    v.BusID,
			VMagPU:   roundFloat(v.VMagPU),
			AngleDeg: roundFloat(v.AngleDeg),
		}
	}
	return out
}

func flowDTOs(fs []LineFlow) []LineFlowDTO {
	out := make([]LineFlowDTO, len(fs))
	for i, f := range fs {
		out[i] = LineFlowDTO{
			LineID:  f.LineID,
			FromBus: f.FromBus,
			ToBus:   f.ToBus,
			FromP:   roundFloat(f.FromP),
			FromQ:   roundFloat(f.FromQ),
			ToP:     roundFloat(f.ToP),
			ToQ:     roundFloat(f.ToQ),
			PLossMW: roundFloat(f.PLossMW),
			FromMVA: roundFloat(f.FromMVA),
			ToMVA:   roundFloat(f.ToMVA),
		}
	}
	return out
}

func roundViolations(vs []Violation) []Violation {
	if len(vs) == 0 {
		return nil
	}
	out := make([]Violation, len(vs))
	for i, v := range vs {
		v.Value = roundFloat(v.Value)
		v.Limit = roundFloat(v.Limit)
		out[i] = v
	}
	return out
}

func (n *Network) evaluate(baseMVA float64) ScenarioResult {
	islanded := n.IslandedBuses()
	if len(islanded) > 0 {
		// Do not fabricate a steady-state solution for buses that have no
		// electrical path to the slack reference.
		return ScenarioResult{
			Status:        StatusDeenergized,
			IslandedBuses: islanded,
			Message:       "one or more buses are disconnected from the slack bus",
		}
	}

	sol, err := n.Solve()
	if err != nil {
		return ScenarioResult{
			Status:  StatusUndetermined,
			Message: err.Error(),
		}
	}

	flows := n.LineFlows(sol)
	violations := n.FindViolations(sol, flows)
	status := StatusSafe
	if len(violations) > 0 {
		status = StatusViolation
	}
	return ScenarioResult{
		Status:         status,
		IslandedBuses:  islanded,
		Iterations:     sol.Iterations,
		MaxResidualPU:  roundFloat(sol.MaxResPU),
		MaxResidualMVA: roundFloat(sol.MaxResPU * baseMVA),
		Voltages:       toDTOs(n.BusVoltages(sol)),
		LineFlows:      flowDTOs(flows),
		Violations:     roundViolations(violations),
	}
}

// Screen solves the intact base case, then independently removes each input
// line in input order and solves each resulting network from the original
// initial conditions. A failed scenario never aborts later scenarios.
func Screen(c *Case) *Report {
	report := &Report{BaseMVA: c.BaseMVA, Contingencies: []ScenarioResult{}}

	baseNet, err := BuildNetwork(c, "")
	if err != nil {
		report.BaseCase = ScenarioResult{Status: StatusUndetermined, Message: err.Error()}
		return report
	}
	report.BaseCase = baseNet.evaluate(c.BaseMVA)

	for _, line := range c.Lines {
		net, err := BuildNetwork(c, line.ID)
		var result ScenarioResult
		if err != nil {
			result = ScenarioResult{
				OutagedLineID: line.ID,
				Status:        StatusUndetermined,
				Message:       err.Error(),
			}
		} else {
			result = net.evaluate(c.BaseMVA)
			result.OutagedLineID = line.ID
		}
		report.Contingencies = append(report.Contingencies, result)
	}
	return report
}
