// Package cx provides the minimal complex arithmetic used by the power-flow
// solver.
package cx

import "math"

// C is a complex number.
type C struct {
	Re, Im float64
}

func (a C) Add(b C) C { return C{a.Re + b.Re, a.Im + b.Im} }
func (a C) Sub(b C) C { return C{a.Re - b.Re, a.Im - b.Im} }
func (a C) Mul(b C) C { return C{a.Re*b.Re - a.Im*b.Im, a.Re*b.Im + a.Im*b.Re} }

func (a C) Div(b C) C {
	d := b.Re*b.Re + b.Im*b.Im
	return C{(a.Re*b.Re + a.Im*b.Im) / d, (a.Im*b.Re - a.Re*b.Im) / d}
}

// Inv returns 1/a. Callers guarantee a is non-zero (x > 0 for all branches).
func (a C) Inv() C {
	d := a.Re*a.Re + a.Im*a.Im
	return C{a.Re / d, -a.Im / d}
}

func (a C) Conj() C      { return C{a.Re, -a.Im} }
func (a C) Abs() float64 { return math.Hypot(a.Re, a.Im) }
