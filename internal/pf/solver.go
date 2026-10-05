package pf

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrSingular     = errors.New("power flow Jacobian is singular")
	ErrNotConverged = errors.New("power flow did not converge within iteration limit")
)

// Solution holds the converged bus state in physical and per-unit units.
type Solution struct {
	VoltagePU  []float64
	AngleRad   []float64
	PInjPU     []float64
	QInjPU     []float64
	MaxResPU   float64
	Iterations int
}

type solveFailure struct{ err error }

func (f *solveFailure) Error() string { return f.err.Error() }

// Solve runs a polar Newton-Raphson AC power flow for the in-service network.
// PV voltage magnitudes and the slack phasor remain fixed; there are no
// generator reactive-power limits.
func (n *Network) Solve() (*Solution, error) {
	nb := len(n.c.Buses)
	Y := n.Ybus()
	G := make([][]float64, nb)
	B := make([][]float64, nb)
	for i := 0; i < nb; i++ {
		G[i] = make([]float64, nb)
		B[i] = make([]float64, nb)
		for j := 0; j < nb; j++ {
			G[i][j] = real(Y[i][j])
			B[i][j] = imag(Y[i][j])
		}
	}

	V := make([]float64, nb)
	theta := make([]float64, nb)
	pSpec := make([]float64, nb)
	qSpec := make([]float64, nb)
	isPQ := make([]bool, nb)
	isPV := make([]bool, nb)
	for i, b := range n.c.Buses {
		pSpec[i] = b.P / n.c.BaseMVA
		qSpec[i] = b.Q / n.c.BaseMVA
		V[i] = 1.0
		switch b.Type {
		case BusSlack:
			V[i] = b.V
			theta[i] = b.AngleD * math.Pi / 180.0
		case BusPV:
			V[i] = b.V
			isPV[i] = true
		case BusPQ:
			isPQ[i] = true
		}
	}

	// Unknown order: every non-slack angle, followed by every PQ magnitude.
	var angleBus, magBus []int
	for i := 0; i < nb; i++ {
		if i != n.slack {
			angleBus = append(angleBus, i)
		}
		if isPQ[i] {
			magBus = append(magBus, i)
		}
	}
	nAng := len(angleBus)
	nMag := len(magBus)
	nVar := nAng + nMag
	angPos := make([]int, nb)
	magPos := make([]int, nb)
	for k, i := range angleBus {
		angPos[i] = k
	}
	for k, i := range magBus {
		magPos[i] = nAng + k
	}

	calcInjections := func() (p, q []float64) {
		p = make([]float64, nb)
		q = make([]float64, nb)
		for i := 0; i < nb; i++ {
			for k := 0; k < nb; k++ {
				d := theta[i] - theta[k]
				p[i] += V[i] * V[k] * (G[i][k]*math.Cos(d) + B[i][k]*math.Sin(d))
				q[i] += V[i] * V[k] * (G[i][k]*math.Sin(d) - B[i][k]*math.Cos(d))
			}
		}
		return p, q
	}

	maxIter, tol := n.c.Config()
	maxRes := math.Inf(1)
	for iter := 0; iter < maxIter; iter++ {
		p, q := calcInjections()
		rhs := make([]float64, nVar)
		maxRes = 0
		for _, i := range angleBus {
			r := pSpec[i] - p[i]
			rhs[angPos[i]] = r
			maxRes = math.Max(maxRes, math.Abs(r))
		}
		for _, i := range magBus {
			r := qSpec[i] - q[i]
			rhs[magPos[i]] = r
			maxRes = math.Max(maxRes, math.Abs(r))
		}
		if maxRes <= tol {
			p, q = calcInjections()
			return &Solution{
				VoltagePU:  append([]float64(nil), V...),
				AngleRad:   append([]float64(nil), theta...),
				PInjPU:     p,
				QInjPU:     q,
				MaxResPU:   maxRes,
				Iterations: iter,
			}, nil
		}

		J := make([][]float64, nVar)
		for r := range J {
			J[r] = make([]float64, nVar)
		}
		for _, i := range angleBus {
			row := angPos[i]
			for k := 0; k < nb; k++ {
				d := theta[i] - theta[k]
				cosd, sind := math.Cos(d), math.Sin(d)
				if k != n.slack {
					if k == i {
						J[row][angPos[i]] = -q[i] - B[i][i]*V[i]*V[i]
					} else {
						J[row][angPos[k]] = V[i] * V[k] * (G[i][k]*sind - B[i][k]*cosd)
					}
				}
				if isPQ[k] {
					if k == i {
						J[row][magPos[i]] = p[i] + G[i][i]*V[i]*V[i]
					} else {
						J[row][magPos[k]] = V[i] * (G[i][k]*cosd + B[i][k]*sind)
					}
				}
			}
		}
		for _, i := range magBus {
			row := magPos[i]
			for k := 0; k < nb; k++ {
				d := theta[i] - theta[k]
				cosd, sind := math.Cos(d), math.Sin(d)
				if k != n.slack {
					if k == i {
						J[row][angPos[i]] = p[i] - G[i][i]*V[i]*V[i]
					} else {
						J[row][magPos[k]] = -V[i] * V[k] * (G[i][k]*cosd + B[i][k]*sind)
					}
				}
				if isPQ[k] {
					if k == i {
						J[row][magPos[i]] = q[i]/V[i] - B[i][i]*V[i]
					} else {
						J[row][magPos[k]] = V[i] * (G[i][k]*sind - B[i][k]*cosd)
					}
				}
			}
		}

		dx, err := linearSolve(J, rhs)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSingular, err)
		}
		for _, i := range angleBus {
			next := theta[i] + dx[angPos[i]]
			if !isFinite(next) {
				return nil, fmt.Errorf("%w: non-finite angle update", ErrSingular)
			}
			theta[i] = next
		}
		for _, i := range magBus {
			next := V[i] + dx[magPos[i]]
			if !isFinite(next) || next <= 0 {
				return nil, fmt.Errorf("%w: invalid voltage magnitude update", ErrSingular)
			}
			V[i] = next
		}
	}
	return nil, fmt.Errorf("%w after %d iterations (residual %.3g pu)", ErrNotConverged, maxIter, maxRes)
}

// linearSolve performs Gaussian elimination with partial pivoting. A pivot that
// is negligible relative to the column magnitude marks the matrix singular.
func linearSolve(A [][]float64, b []float64) ([]float64, error) {
	n := len(b)
	m := make([][]float64, n)
	for i := range A {
		m[i] = append([]float64(nil), A[i]...)
		m[i] = append(m[i], b[i])
	}
	for col := 0; col < n; col++ {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][col]) < 1e-12 {
			return nil, fmt.Errorf("near-zero pivot at column %d", col)
		}
		m[col], m[pivot] = m[pivot], m[col]
		for r := col + 1; r < n; r++ {
			f := m[r][col] / m[col][col]
			for k := col; k <= n; k++ {
				m[r][k] -= f * m[col][k]
			}
		}
	}
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := m[i][n]
		for k := i + 1; k < n; k++ {
			sum -= m[i][k] * x[k]
		}
		x[i] = sum / m[i][i]
	}
	return x, nil
}
