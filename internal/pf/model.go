package pf

import (
	"fmt"
	"math"
)

// BusType identifies the steady-state modeling class of a bus.
type BusType string

const (
	BusPQ    BusType = "pq"
	BusPV    BusType = "pv"
	BusSlack BusType = "slack"
)

// Bus is a balanced three-phase bus specification. Net injection is positive
// for generation and negative for load. P/Q are expressed in MW/MVAr.
type Bus struct {
	ID     string  `json:"id"`
	Type   BusType `json:"type"`
	Vmin   float64 `json:"v_min_pu"`
	Vmax   float64 `json:"v_max_pu"`
	P      float64 `json:"p_mw"`
	Q      float64 `json:"q_mvar"`
	V      float64 `json:"voltage_pu"`
	AngleD float64 `json:"angle_deg"`
}

// Line is a positive-sequence pi-model branch. B is the total charging
// susceptance split equally between the two ends. r/x/b are per unit on the
// network base MVA; capacity is in MVA.
type Line struct {
	ID       string  `json:"id"`
	From     string  `json:"from_bus"`
	To       string  `json:"to_bus"`
	R        float64 `json:"r_pu"`
	X        float64 `json:"x_pu"`
	B        float64 `json:"b_pu"`
	Capacity float64 `json:"capacity_mva"`
}

// Case is a complete screening request.
type Case struct {
	BaseMVA       float64 `json:"base_mva"`
	MaxIterations int     `json:"max_iterations,omitempty"`
	TolerancePU   float64 `json:"tolerance_pu,omitempty"`
	Buses         []Bus   `json:"buses"`
	Lines         []Line  `json:"lines"`
}

const (
	DefaultMaxIterations = 30
	DefaultTolerancePU   = 1e-10
	maxBuses             = 12
	maxLines             = 24
)

// Config returns the numerical solver settings, applying defaults and bounds.
func (c *Case) Config() (maxIter int, tol float64) {
	maxIter = c.MaxIterations
	if maxIter <= 0 {
		maxIter = DefaultMaxIterations
	}
	if maxIter > 500 {
		maxIter = 500
	}
	tol = c.TolerancePU
	if tol <= 0 || math.IsNaN(tol) {
		tol = DefaultTolerancePU
	}
	return maxIter, tol
}

func isFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Validate checks every field constraint from the interface specification.
func (c *Case) Validate() error {
	if !isFinite(c.BaseMVA) || c.BaseMVA <= 0 {
		return fmt.Errorf("base_mva must be a finite positive number")
	}
	if c.MaxIterations < 0 || (c.MaxIterations != 0 && !isFinite(float64(c.MaxIterations))) {
		return fmt.Errorf("max_iterations must be a non-negative integer")
	}
	if !isFinite(c.TolerancePU) || c.TolerancePU < 0 {
		return fmt.Errorf("tolerance_pu must be finite and non-negative")
	}
	if n := len(c.Buses); n < 2 || n > maxBuses {
		return fmt.Errorf("buses count must be between 2 and %d, got %d", maxBuses, n)
	}
	if len(c.Lines) > maxLines {
		return fmt.Errorf("lines count must not exceed %d, got %d", maxLines, len(c.Lines))
	}

	seenBuses := make(map[string]bool, len(c.Buses))
	slackCount := 0
	for i := range c.Buses {
		b := &c.Buses[i]
		if b.ID == "" {
			return fmt.Errorf("bus at index %d has an empty id", i)
		}
		if seenBuses[b.ID] {
			return fmt.Errorf("duplicated bus id %q", b.ID)
		}
		seenBuses[b.ID] = true
		switch b.Type {
		case BusPQ, BusPV, BusSlack:
		default:
			return fmt.Errorf("bus %q has invalid type %q (want pq|pv|slack)", b.ID, b.Type)
		}
		for name, v := range map[string]float64{
			"v_min_pu": b.Vmin, "v_max_pu": b.Vmax, "p_mw": b.P,
			"q_mvar": b.Q, "voltage_pu": b.V, "angle_deg": b.AngleD,
		} {
			if !isFinite(v) {
				return fmt.Errorf("bus %q field %s must be finite", b.ID, name)
			}
		}
		if b.Vmin <= 0 || b.Vmax <= b.Vmin {
			return fmt.Errorf("bus %q requires 0 < v_min_pu < v_max_pu", b.ID)
		}
		switch b.Type {
		case BusPV:
			if b.V <= 0 {
				return fmt.Errorf("pv bus %q requires positive voltage_pu", b.ID)
			}
			if b.V < b.Vmin || b.V > b.Vmax {
				return fmt.Errorf("pv bus %q setpoint voltage_pu outside its limits", b.ID)
			}
		case BusSlack:
			slackCount++
			if b.V <= 0 {
				return fmt.Errorf("slack bus %q requires positive voltage_pu", b.ID)
			}
			if b.V < b.Vmin || b.V > b.Vmax {
				return fmt.Errorf("slack bus %q voltage_pu outside its limits", b.ID)
			}
		}
	}
	if slackCount != 1 {
		return fmt.Errorf("network must contain exactly one slack bus, found %d", slackCount)
	}
	seenLines := make(map[string]bool, len(c.Lines))
	for i := range c.Lines {
		l := &c.Lines[i]
		if l.ID == "" {
			return fmt.Errorf("line at index %d has an empty id", i)
		}
		if seenLines[l.ID] {
			return fmt.Errorf("duplicated line id %q", l.ID)
		}
		seenLines[l.ID] = true
		if l.From == l.To {
			return fmt.Errorf("line %q is a self-loop on bus %q", l.ID, l.From)
		}
		if !seenBuses[l.From] {
			return fmt.Errorf("line %q references unknown from_bus %q", l.ID, l.From)
		}
		if !seenBuses[l.To] {
			return fmt.Errorf("line %q references unknown to_bus %q", l.ID, l.To)
		}
		for name, v := range map[string]float64{
			"r_pu": l.R, "x_pu": l.X, "b_pu": l.B, "capacity_mva": l.Capacity,
		} {
			if !isFinite(v) {
				return fmt.Errorf("line %q field %s must be finite", l.ID, name)
			}
		}
		if l.R < 0 {
			return fmt.Errorf("line %q requires r_pu >= 0", l.ID)
		}
		if l.X <= 0 {
			return fmt.Errorf("line %q requires x_pu > 0", l.ID)
		}
		if l.B < 0 {
			return fmt.Errorf("line %q requires b_pu >= 0", l.ID)
		}
		if l.Capacity <= 0 {
			return fmt.Errorf("line %q requires capacity_mva > 0", l.ID)
		}
	}
	return nil
}
