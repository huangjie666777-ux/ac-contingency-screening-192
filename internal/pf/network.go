package pf

import (
	"math"
)

// Network is a validated, indexed view of a Case with an optional single line
// removed for an N-1 scenario.
type Network struct {
	c          *Case
	busIndex   map[string]int
	slack      int
	pv         []int
	activeLine []int // indices into Case.Lines that remain in service
}

// BuildNetwork indexes the case and removes the line with removedID when it is
// non-empty. The removed line must reference an existing line id.
func BuildNetwork(c *Case, removedID string) (*Network, error) {
	n := &Network{c: c, busIndex: make(map[string]int, len(c.Buses))}
	for i := range c.Buses {
		n.busIndex[c.Buses[i].ID] = i
		if c.Buses[i].Type == BusSlack {
			n.slack = i
		} else if c.Buses[i].Type == BusPV {
			n.pv = append(n.pv, i)
		}
	}
	removedIdx := -1
	if removedID != "" {
		var ok bool
		removedIdx, ok = n.lineIndex(removedID)
		if !ok {
			return nil, errLineNotFound(removedID)
		}
	}
	for i := range c.Lines {
		if i != removedIdx {
			n.activeLine = append(n.activeLine, i)
		}
	}
	return n, nil
}

func errLineNotFound(id string) error {
	return &lineNotFoundError{id: id}
}

type lineNotFoundError struct{ id string }

func (e *lineNotFoundError) Error() string { return "line not found: " + e.id }

func (n *Network) lineIndex(id string) (int, bool) {
	for i := range n.c.Lines {
		if n.c.Lines[i].ID == id {
			return i, true
		}
	}
	return -1, false
}

// Slack returns the index of the unique slack bus.
func (n *Network) Slack() int { return n.slack }

// BusIndex exposes the id-to-index mapping.
func (n *Network) BusIndex() map[string]int { return n.busIndex }

// ReachableFromSlack performs an undirected BFS over in-service lines and
// returns the set of buses connected to the slack bus.
func (n *Network) ReachableFromSlack() []bool {
	nb := len(n.c.Buses)
	adj := make([][]int, nb)
	for _, li := range n.activeLine {
		l := &n.c.Lines[li]
		i, j := n.busIndex[l.From], n.busIndex[l.To]
		adj[i] = append(adj[i], j)
		adj[j] = append(adj[j], i)
	}
	reach := make([]bool, nb)
	reach[n.slack] = true
	queue := []int{n.slack}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range adj[u] {
			if !reach[v] {
				reach[v] = true
				queue = append(queue, v)
			}
		}
	}
	return reach
}

// IslandedBuses returns the bus IDs that are electrically disconnected from
// the slack bus after the outage.
func (n *Network) IslandedBuses() []string {
	reach := n.ReachableFromSlack()
	var out []string
	for i := range n.c.Buses {
		if !reach[i] {
			out = append(out, n.c.Buses[i].ID)
		}
	}
	return out
}

// Ybus assembles the complex nodal admittance matrix using the line pi model.
// Series admittance is 1/(r+jx); total susceptance b is split b/2 per end.
func (n *Network) Ybus() [][]complex128 {
	nb := len(n.c.Buses)
	y := make([][]complex128, nb)
	for i := range y {
		y[i] = make([]complex128, nb)
	}
	for _, li := range n.activeLine {
		l := &n.c.Lines[li]
		i, j := n.busIndex[l.From], n.busIndex[l.To]
		zs := complex(l.R, l.X)
		ys := 1.0 / zs
		ysh := complex(0, l.B/2.0)
		y[i][i] += ys + ysh
		y[j][j] += ys + ysh
		y[i][j] -= ys
		y[j][i] -= ys
	}
	return y
}

// LineSeriesAdmittance exposes the series admittance used by the flow code.
func LineSeriesAdmittance(l *Line) complex128 {
	return 1.0 / complex(l.R, l.X)
}

// FiniteComplex reports whether both components of a complex number are finite.
func FiniteComplex(z complex128) bool {
	return !math.IsNaN(real(z)) && !math.IsInf(real(z), 0) &&
		!math.IsNaN(imag(z)) && !math.IsInf(imag(z), 0)
}
