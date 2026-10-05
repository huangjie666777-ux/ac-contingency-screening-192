package main

import (
	"errors"
	"math"

	"github.com/huangjie666777-ux/ac-contingency-screening-192/internal/cx"
)

var errSingular = errors.New("jacobian is singular")

// solution holds a converged (or best-effort) power-flow state in per-unit
// radians/pu magnitudes.
type solution struct {
	vmag   []float64
	theta  []float64 // radians
	iter   int
	maxMis float64 // largest per-unit power mismatch at return
}

// solvePowerFlow runs polar-coordinate Newton-Raphson on the network.
// PV magnitudes and the slack phasor stay fixed. It returns errSingular if
// the Jacobian is singular, and converged=false if the iteration limit is
// hit; in both cases no valid solution is fabricated.
func solvePowerFlow(n *network, y [][]cx.C, maxIter int, tol float64) (sol *solution, converged bool, err error) {
	c := n.c
	nb := len(c.Buses)
	base := c.BaseMVA

	// Scheduled injections in per-unit.
	pSch := make([]float64, nb)
	qSch := make([]float64, nb)
	for i, b := range c.Buses {
		pSch[i] = b.P / base
		qSch[i] = b.Q / base
	}

	// Flat start: slack phasor and PV magnitudes fixed, everything else 1.0 pu.
	sol = &solution{vmag: make([]float64, nb), theta: make([]float64, nb)}
	for i, b := range c.Buses {
		sol.vmag[i] = 1.0
		switch b.Type {
		case BusSlack:
			sol.vmag[i] = b.V
			sol.theta[i] = b.ThetaDeg * math.Pi / 180
		case BusPV:
			sol.vmag[i] = b.V
		}
	}

	// Unknown ordering: angles of non-slack buses, then magnitudes of PQ buses.
	var angIdx, magIdx []int
	for i, b := range c.Buses {
		if b.Type != BusSlack {
			angIdx = append(angIdx, i)
		}
		if b.Type == BusPQ {
			magIdx = append(magIdx, i)
		}
	}
	na, nm := len(angIdx), len(magIdx)
	dim := na + nm
	if dim == 0 {
		return sol, true, nil
	}

	calcPQ := func() ([]float64, []float64) {
		p := make([]float64, nb)
		q := make([]float64, nb)
		for i := 0; i < nb; i++ {
			vi := sol.vmag[i]
			for j := 0; j < nb; j++ {
				yij := y[i][j]
				if yij.Re == 0 && yij.Im == 0 {
					continue
				}
				dij := sol.theta[i] - sol.theta[j]
				vj := sol.vmag[j]
				p[i] += vi * vj * (yij.Re*math.Cos(dij) + yij.Im*math.Sin(dij))
				q[i] += vi * vj * (yij.Re*math.Sin(dij) - yij.Im*math.Cos(dij))
			}
		}
		return p, q
	}

	for iter := 1; iter <= maxIter; iter++ {
		p, q := calcPQ()
		mis := make([]float64, dim)
		maxMis := 0.0
		for k, i := range angIdx {
			mis[k] = pSch[i] - p[i]
		}
		for k, i := range magIdx {
			mis[na+k] = qSch[i] - q[i]
		}
		for _, m := range mis {
			if a := math.Abs(m); a > maxMis {
				maxMis = a
			}
		}
		sol.iter = iter
		sol.maxMis = maxMis
		if maxMis < tol {
			return sol, true, nil
		}

		// Build the Jacobian: [H N; M L].
		j := make([][]float64, dim)
		for r := range j {
			j[r] = make([]float64, dim)
		}
		for r, i := range angIdx {
			vi := sol.vmag[i]
			for cc, jj := range angIdx {
				if i == jj {
					j[r][cc] = -q[i] - y[i][i].Im*vi*vi
				} else {
					dij := sol.theta[i] - sol.theta[jj]
					j[r][cc] = vi * sol.vmag[jj] * (y[i][jj].Re*math.Sin(dij) - y[i][jj].Im*math.Cos(dij))
				}
			}
			for cc, jj := range magIdx {
				if i == jj {
					j[r][na+cc] = p[i]/vi + y[i][i].Re*vi
				} else {
					dij := sol.theta[i] - sol.theta[jj]
					j[r][na+cc] = vi * (y[i][jj].Re*math.Cos(dij) + y[i][jj].Im*math.Sin(dij))
				}
			}
		}
		for r, i := range magIdx {
			vi := sol.vmag[i]
			for cc, jj := range angIdx {
				if i == jj {
					j[na+r][cc] = p[i] - y[i][i].Re*vi*vi
				} else {
					dij := sol.theta[i] - sol.theta[jj]
					j[na+r][cc] = -vi * sol.vmag[jj] * (y[i][jj].Re*math.Cos(dij) + y[i][jj].Im*math.Sin(dij))
				}
			}
			for cc, jj := range magIdx {
				if i == jj {
					j[na+r][na+cc] = q[i]/vi - y[i][i].Im*vi
				} else {
					dij := sol.theta[i] - sol.theta[jj]
					j[na+r][na+cc] = vi * (y[i][jj].Re*math.Sin(dij) - y[i][jj].Im*math.Cos(dij))
				}
			}
		}

		dx, serr := solveLinear(j, mis)
		if serr != nil {
			return sol, false, serr
		}
		for k, i := range angIdx {
			sol.theta[i] += dx[k]
		}
		for k, i := range magIdx {
			sol.vmag[i] += dx[na+k]
		}
	}
	return sol, false, nil
}

// solveLinear solves A x = b by Gaussian elimination with partial pivoting.
func solveLinear(a [][]float64, b []float64) ([]float64, error) {
	n := len(b)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n+1)
		copy(m[i], a[i])
		m[i][n] = b[i]
	}
	for col := 0; col < n; col++ {
		piv := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[piv][col]) {
				piv = r
			}
		}
		if math.Abs(m[piv][col]) < 1e-12 {
			return nil, errSingular
		}
		m[col], m[piv] = m[piv], m[col]
		for r := col + 1; r < n; r++ {
			f := m[r][col] / m[col][col]
			for cc := col; cc <= n; cc++ {
				m[r][cc] -= f * m[col][cc]
			}
		}
	}
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		s := m[i][n]
		for cc := i + 1; cc < n; cc++ {
			s -= m[i][cc] * x[cc]
		}
		x[i] = s / m[i][i]
	}
	return x, nil
}
