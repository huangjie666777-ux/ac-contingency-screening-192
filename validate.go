package main

import (
	"errors"
	"fmt"
	"math"
)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// validateCase enforces all structural and numeric input rules.
func validateCase(c *Case) error {
	if !finite(c.BaseMVA) || c.BaseMVA <= 0 {
		return errors.New("base_mva must be a positive finite number")
	}
	if c.MaxIter < 0 {
		return errors.New("max_iter must be >= 0")
	}
	if !finite(c.Tol) || c.Tol < 0 {
		return errors.New("tolerance must be a finite number >= 0")
	}
	if len(c.Buses) < minBuses || len(c.Buses) > maxBuses {
		return fmt.Errorf("bus count must be between %d and %d, got %d", minBuses, maxBuses, len(c.Buses))
	}
	if len(c.Lines) > maxLines {
		return fmt.Errorf("line count must be at most %d, got %d", maxLines, len(c.Lines))
	}

	seen := make(map[string]bool, len(c.Buses))
	slackCount := 0
	for i, b := range c.Buses {
		if b.ID == "" {
			return fmt.Errorf("bus %d: id must not be empty", i)
		}
		if seen[b.ID] {
			return fmt.Errorf("duplicate bus id %q", b.ID)
		}
		seen[b.ID] = true
		for _, v := range []float64{b.P, b.Q, b.V, b.ThetaDeg, b.VMin, b.VMax} {
			if !finite(v) {
				return fmt.Errorf("bus %q: non-finite numeric field", b.ID)
			}
		}
		if !(b.VMin < b.VMax) {
			return fmt.Errorf("bus %q: require v_min_pu < v_max_pu", b.ID)
		}
		switch b.Type {
		case BusPQ:
		case BusPV:
			if b.V <= 0 {
				return fmt.Errorf("bus %q: PV voltage setpoint must be positive", b.ID)
			}
		case BusSlack:
			slackCount++
			if b.V <= 0 {
				return fmt.Errorf("bus %q: slack voltage magnitude must be positive", b.ID)
			}
		default:
			return fmt.Errorf("bus %q: unknown type %q (want PQ, PV or slack)", b.ID, b.Type)
		}
	}
	if slackCount != 1 {
		return fmt.Errorf("exactly one slack bus required, got %d", slackCount)
	}

	lineSeen := make(map[string]bool, len(c.Lines))
	for i, l := range c.Lines {
		if l.ID == "" {
			return fmt.Errorf("line %d: id must not be empty", i)
		}
		if lineSeen[l.ID] {
			return fmt.Errorf("duplicate line id %q", l.ID)
		}
		lineSeen[l.ID] = true
		if l.From == l.To {
			return fmt.Errorf("line %q: self-loop is not allowed", l.ID)
		}
		if !seen[l.From] || !seen[l.To] {
			return fmt.Errorf("line %q: unknown bus endpoint", l.ID)
		}
		for _, v := range []float64{l.R, l.X, l.B, l.RateMVA} {
			if !finite(v) {
				return fmt.Errorf("line %q: non-finite numeric field", l.ID)
			}
		}
		if l.R < 0 {
			return fmt.Errorf("line %q: r_pu must be >= 0", l.ID)
		}
		if l.X <= 0 {
			return fmt.Errorf("line %q: x_pu must be > 0", l.ID)
		}
		if l.B < 0 {
			return fmt.Errorf("line %q: b_pu must be >= 0", l.ID)
		}
		if l.RateMVA <= 0 {
			return fmt.Errorf("line %q: rate_mva must be > 0", l.ID)
		}
	}
	return nil
}
