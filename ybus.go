package main

import "github.com/huangjie666777-ux/ac-contingency-screening-192/internal/cx"

// network is the per-scenario view of the case: buses plus the indices of
// in-service lines.
type network struct {
	c        *Case
	busIndex map[string]int
	active   []int // indices into c.Lines that are in service
}

func newNetwork(c *Case, outagedLineID string) *network {
	n := &network{c: c, busIndex: make(map[string]int, len(c.Buses))}
	for i, b := range c.Buses {
		n.busIndex[b.ID] = i
	}
	for i, l := range c.Lines {
		if l.ID != outagedLineID {
			n.active = append(n.active, i)
		}
	}
	return n
}

// buildYbus assembles the complex bus admittance matrix from the pi model:
// series admittance 1/(r+jx) plus half the total charging susceptance jb/2
// shunted at each end.
func (n *network) buildYbus() [][]cx.C {
	nb := len(n.c.Buses)
	y := make([][]cx.C, nb)
	for i := range y {
		y[i] = make([]cx.C, nb)
	}
	for _, li := range n.active {
		l := n.c.Lines[li]
		f, t := n.busIndex[l.From], n.busIndex[l.To]
		ys := cx.C{Re: l.R, Im: l.X}.Inv()
		sh := cx.C{Im: l.B / 2}
		y[f][f] = y[f][f].Add(ys).Add(sh)
		y[t][t] = y[t][t].Add(ys).Add(sh)
		y[f][t] = y[f][t].Sub(ys)
		y[t][f] = y[t][f].Sub(ys)
	}
	return y
}

// islandedBuses returns IDs of buses that cannot reach the slack bus through
// in-service lines, i.e. buses left without supply.
func (n *network) islandedBuses() []string {
	nb := len(n.c.Buses)
	adj := make([][]int, nb)
	for _, li := range n.active {
		l := n.c.Lines[li]
		f, t := n.busIndex[l.From], n.busIndex[l.To]
		adj[f] = append(adj[f], t)
		adj[t] = append(adj[t], f)
	}
	slack := -1
	for i, b := range n.c.Buses {
		if b.Type == BusSlack {
			slack = i
		}
	}
	seen := make([]bool, nb)
	queue := []int{slack}
	seen[slack] = true
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range adj[u] {
			if !seen[v] {
				seen[v] = true
				queue = append(queue, v)
			}
		}
	}
	var out []string
	for i, b := range n.c.Buses {
		if !seen[i] {
			out = append(out, b.ID)
		}
	}
	return out
}
