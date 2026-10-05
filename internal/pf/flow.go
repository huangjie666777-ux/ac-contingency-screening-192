package pf

import "math"

// LineFlow is the complex power entering each line end, plus real losses.
type LineFlow struct {
	LineID      string
	FromBus     string
	ToBus       string
	FromP       float64 // MW flowing into the line at the from end
	FromQ       float64 // MVAr flowing into the line at the from end
	ToP         float64 // MW flowing into the line at the to end
	ToQ         float64 // MVAr flowing into the line at the to end
	FromMVA     float64
	ToMVA       float64
	PLossMW     float64
	QLossMVAr   float64
	CapacityMVA float64
}

// BusVoltage is a converged bus voltage result.
type BusVoltage struct {
	BusID    string
	VMagPU   float64
	AngleDeg float64
}

// Violation describes one overloaded line end or one bus outside its voltage
// limits. It is intentionally per object and per monitored end.
type Violation struct {
	Kind   string  `json:"kind"` // line_mva | bus_voltage
	Object string  `json:"object"`
	Detail string  `json:"detail"`
	Value  float64 `json:"value"`
	Limit  float64 `json:"limit"`
}

// ComplexPowerAtEnd calculates S = V * conj(I) entering a line end. The
// pi model places b/2 of charging susceptance at each end.
func complexPowerAtEnd(l *Line, vFrom, vTo complex128, atFrom bool) complex128 {
	ys := LineSeriesAdmittance(l)
	// Convention: positive power flows from the bus into the branch.
	if atFrom {
		iFrom := (ys+complex(0, l.B/2.0))*vFrom - ys*vTo
		return vFrom * conjugate(iFrom)
	}
	iTo := (ys+complex(0, l.B/2.0))*vTo - ys*vFrom
	return vTo * conjugate(iTo)
}

func conjugate(z complex128) complex128 { return complex(real(z), -imag(z)) }

// LineFlows computes power at both ends and real losses for in-service lines.
func (n *Network) LineFlows(sol *Solution) []LineFlow {
	nb := len(sol.VoltagePU)
	v := make([]complex128, nb)
	for i := 0; i < nb; i++ {
		v[i] = complex(sol.VoltagePU[i], 0) * ceExp(sol.AngleRad[i])
	}
	base := n.c.BaseMVA
	flows := make([]LineFlow, 0, len(n.activeLine))
	for _, li := range n.activeLine {
		l := &n.c.Lines[li]
		i, j := n.busIndex[l.From], n.busIndex[l.To]
		sf := complexPowerAtEnd(l, v[i], v[j], true) * complex(base, 0)
		st := complexPowerAtEnd(l, v[i], v[j], false) * complex(base, 0)
		flows = append(flows, LineFlow{
			LineID:      l.ID,
			FromBus:     l.From,
			ToBus:       l.To,
			FromP:       real(sf),
			FromQ:       imag(sf),
			ToP:         real(st),
			ToQ:         imag(st),
			FromMVA:     math.Hypot(real(sf), imag(sf)),
			ToMVA:       math.Hypot(real(st), imag(st)),
			PLossMW:     real(sf) + real(st),
			QLossMVAr:   imag(sf) + imag(st),
			CapacityMVA: l.Capacity,
		})
	}
	return flows
}

func ceExp(theta float64) complex128 {
	return complex(math.Cos(theta), math.Sin(theta))
}

// BusVoltages extracts voltage results in bus input order.
func (n *Network) BusVoltages(sol *Solution) []BusVoltage {
	out := make([]BusVoltage, 0, len(n.c.Buses))
	for i, b := range n.c.Buses {
		out = append(out, BusVoltage{
			BusID:    b.ID,
			VMagPU:   sol.VoltagePU[i],
			AngleDeg: sol.AngleRad[i] * 180.0 / math.Pi,
		})
	}
	return out
}

// FindViolations checks every bus voltage magnitude and both line-end apparent
// powers. Limits are compared item by item, so one overloaded branch can
// produce two end-specific violations.
func (n *Network) FindViolations(sol *Solution, flows []LineFlow) []Violation {
	var violations []Violation
	for i, b := range n.c.Buses {
		v := sol.VoltagePU[i]
		if v < b.Vmin {
			violations = append(violations, Violation{
				Kind: "bus_voltage", Object: b.ID, Detail: "below v_min_pu",
				Value: v, Limit: b.Vmin,
			})
		}
		if v > b.Vmax {
			violations = append(violations, Violation{
				Kind: "bus_voltage", Object: b.ID, Detail: "above v_max_pu",
				Value: v, Limit: b.Vmax,
			})
		}
	}
	for _, f := range flows {
		if f.FromMVA > f.CapacityMVA {
			violations = append(violations, Violation{
				Kind: "line_mva", Object: f.LineID, Detail: "from_end " + f.FromBus,
				Value: f.FromMVA, Limit: f.CapacityMVA,
			})
		}
		if f.ToMVA > f.CapacityMVA {
			violations = append(violations, Violation{
				Kind: "line_mva", Object: f.LineID, Detail: "to_end " + f.ToBus,
				Value: f.ToMVA, Limit: f.CapacityMVA,
			})
		}
	}
	return violations
}
