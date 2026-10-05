package main

// BusType is the steady-state bus classification.
type BusType string

const (
	BusPQ    BusType = "PQ"
	BusPV    BusType = "PV"
	BusSlack BusType = "slack"
)

// Bus describes one network bus. Powers are net injections in MW/MVAr
// (generation positive, load negative). Voltages are per-unit magnitudes,
// angles are degrees.
type Bus struct {
	ID       string  `json:"id"`
	Type     BusType `json:"type"`
	P        float64 `json:"p_mw"`      // PQ and PV buses
	Q        float64 `json:"q_mvar"`    // PQ buses
	V        float64 `json:"v_pu"`      // PV and slack buses
	ThetaDeg float64 `json:"theta_deg"` // slack buses
	VMin     float64 `json:"v_min_pu"`
	VMax     float64 `json:"v_max_pu"`
}

// Line describes one pi-model branch. R/X/B are per-unit on the case base,
// B is the total charging susceptance (half applied at each end).
type Line struct {
	ID      string  `json:"id"`
	From    string  `json:"from"`
	To      string  `json:"to"`
	R       float64 `json:"r_pu"`
	X       float64 `json:"x_pu"`
	B       float64 `json:"b_pu"`
	RateMVA float64 `json:"rate_mva"`
}

// Case is the full screening request payload.
type Case struct {
	BaseMVA float64 `json:"base_mva"`
	MaxIter int     `json:"max_iter"`  // optional, default 50
	Tol     float64 `json:"tolerance"` // optional, default 1e-8 (per-unit mismatch)
	Buses   []Bus   `json:"buses"`
	Lines   []Line  `json:"lines"`
}

const (
	defaultMaxIter = 50
	defaultTol     = 1e-8
	maxBuses       = 12
	minBuses       = 2
	maxLines       = 24
)

func (c *Case) solverConfig() (maxIter int, tol float64) {
	maxIter, tol = c.MaxIter, c.Tol
	if maxIter <= 0 {
		maxIter = defaultMaxIter
	}
	if tol <= 0 {
		tol = defaultTol
	}
	return maxIter, tol
}
