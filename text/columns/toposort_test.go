package main

import (
	"fmt"
	"testing"
)

func TestToposort(t *testing.T) {
	adj := [][]bool{
		[]bool{false, false, false}, // []
		[]bool{true, false, false},  // [0]
		[]bool{false, true, false},  // [1]
	}
	order := []int{2, 1, 0}

	ts := newTopo(adj)
	for i := 0; i < ts.n; i++ {
		ts.sort(i, 0)
	}
	fmt.Println("=========================")
	for i, k := range ts.topo {
		v := ts.adj[k]
		fmt.Printf("%4d: %2d %v\n", i, k, v)
	}
	for i, o := range order {
		if ts.topo[i] != o {
			t.Errorf("Wrong order: i=%d expected=%d actual=%d", i, o, ts.topo[i])
		}
	}
}