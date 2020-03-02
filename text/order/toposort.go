package main

import (
	"fmt"
	"os"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

type topoState struct {
	n       int
	adj     [][]bool
	visited []bool
	topo    []int
}

func (t *topoState) prepend(i int) {
	topo := []int{i}
	for _, j := range t.topo {
		if i == j {
			panic(i)
		}
	}
	t.topo = append(topo, t.topo...)
}

func (t *topoState) sort(curVert, depth int) {
	fmt.Printf("sort: curVert=%d depth=%d\n", curVert, depth)
	t.visited[curVert] = true
	for i := 0; i < t.n; i++ {
		if t.adj[curVert][i] && !t.visited[i] {
			t.sort(i, depth+1)
		}
	}
	t.prepend(curVert)

	fmt.Printf("   curVert=%d depth=%d topso=%v\n", curVert, depth, t.topo)
}

func newTopo(adj [][]bool) *topoState {
	n := len(adj)
	t := topoState{
		n:       n,
		adj:     adj,
		visited: make([]bool, n),
	}
	return &t
}

var adj = [][]bool{
	[]bool{false, false, false}, // []
	[]bool{true, false, false},  // [0]
	[]bool{false, true, false},  // [1]
}

func testTopo() {
	t := newTopo(adj)
	for i := 0; i < t.n; i++ {
		t.sort(i, 0)
	}
	fmt.Println("=========================")
	for i, k := range t.topo {
		v := t.adj[k]
		fmt.Printf("%4d: %2d %v\n", i, k, v)
	}
	os.Exit(55)
}

// 1. Line segment `a` comes before line segment `b` if their ranges of x-coordinates overlap and if
//    line segment `a` is above line segment `b` on the page.
// 2. Line segment `a` comes before line segment `b` if `a` is entirely to the left of `b` and if
//    there does not exist a line segment `c` whose y-coordinates  are between `a` and `b` and whose
//    range of x coordinates overlaps both `a` and `b`.

func before(a, b model.PdfRectangle) bool {
	return before1(a, b) || before2(a, b)
}
func before1(a, b model.PdfRectangle) bool {
	return overlappedX(a, b) && a.Ury > b.Ury
}

func before2(a, b model.PdfRectangle) bool {
	return a.Urx <= b.Llx
}

func rectListAdj(rl rectList) [][]bool {
	n := len(rl)
	adj := make([][]bool, n)
	for i := 0; i < n; i++ {
		adj[i] = make([]bool, n)
	}
	for i, ri := range rl {
		for j, rj := range rl {
			adj[i][j] = i != j && before(ri, rj)
		}
	}
	fmt.Println("-----------------------------------------------------------")
	for i := range rl {
		fmt.Printf("\t")
		for j := range rl {
			fmt.Printf("%7t", adj[i][j])
			if adj[i][j] && adj[j][i] {
				panic("loop")
			}
		}
		fmt.Printf("\n")
	}
	fmt.Println("-----------------------------------------------------------")
	return adj
}

func sortRectList(rl rectList) rectList {
	common.Log.Info("sortRectList: rl=%d ===========x=============", len(rl))
	adj := rectListAdj(rl)
	t := newTopo(adj)
	for i := 0; i < t.n; i++ {
		if !t.visited[i] {
			t.sort(i, 0)
		}
	}
	common.Log.Info("sortRectList: =========================")
	sorted := make(rectList, len(rl))
	for i, k := range t.topo {
		r := rl[k]
		var b1, b2 string
		if i < len(t.topo)-1 {
			r1 := rl[t.topo[i+1]]
			if before1(r, r1) {
				b1 = "before1"
			}
			if before2(r, r1) {
				b2 = "before2"
			}
		}
		fmt.Printf("%4d: %2d %s %7s %7s\n", i, k, showBBox(r), b1, b2)
		sorted[i] = r
	}
	return sorted
}
