package main

import (
	"fmt"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

// sortReadingOrder returns `columns` sorted in reading order.
func sortReadingOrder(columns rectList) {
	common.Log.Info("sortReadingOrder: columns=%d ===========x=============", len(columns))
	if len(columns) <= 1 {
		return
	}
	adj := rectListAdj(columns)
	ts := newTopoState(adj)
	for i := 0; i < ts.n; i++ {
		if !ts.visited[i] {
			ts.sort(i, 0)
		}
	}
	sorted := make(rectList, len(columns))
	for i, k := range ts.order {
		sorted[i] = columns[k]
	}
	for i, r := range sorted {
		columns[i] = r
	}
	if common.Log.IsLogLevel(common.LogLevelDebug) {
		common.Log.Debug("sortReadingOrder: =========================")
		for i, r := range sorted {
			var b1, b2 string
			if i < len(columns)-1 {
				r1 := columns[i+1]
				if before1(r, r1) {
					b1 = "before1"
				}
				if before2(r, r1) {
					b2 = "before2"
				}
			}
			fmt.Printf("%4d:  %s %7s %7s\n", i, showBBox(r), b1, b2)
		}
	}
}

func newTopoState(adj [][]bool) *topoState {
	n := len(adj)
	t := topoState{
		n:       n,
		adj:     adj,
		visited: make([]bool, n),
	}
	return &t
}

type topoState struct {
	n       int
	adj     [][]bool
	visited []bool
	order   []int
}

func (ts *topoState) sort(curVert, depth int) {
	common.Log.Debug("sort: curVert=%d depth=%d\n", curVert, depth)
	ts.visited[curVert] = true
	for i := 0; i < ts.n; i++ {
		if ts.adj[curVert][i] && !ts.visited[i] {
			ts.sort(i, depth+1)
		}
	}
	ts.prepend(curVert)
	common.Log.Debug("   curVert=%d depth=%d topso=%v\n", curVert, depth, ts.order)
}

// rectListAdj creates an adjacency list for the DAG of connections over `columns`. The connections are
//
func rectListAdj(columns rectList) [][]bool {
	n := len(columns)
	adj := make([][]bool, n)
	for i, ri := range columns {
		adj[i] = make([]bool, n)
		for j, rj := range columns {
			adj[i][j] = i != j && before(ri, rj)
		}
		if bboxEmpty(ri) {
			panic(fmt.Errorf("bad bbox: i=%d r=%s", i, showBBox(ri)))
		}
	}
	if common.Log.IsLogLevel(common.LogLevelDebug) {
		fmt.Println("-----------------------------------------------------------")
		for i := range columns {
			fmt.Printf("\t")
			for j := range columns {
				fmt.Printf("%7t", adj[i][j])
				if adj[i][j] && adj[j][i] {
					panic("cycle")
				}
			}
			fmt.Printf("\n")
		}
		fmt.Println("-----------------------------------------------------------")
	}
	for i, r := range columns {
		if before(r, r) {
			panic(fmt.Errorf("before is ambiguous i=%d r=%s before1=%t before2=%t",
				i, showBBox(r), before1(r, r), before2(r, r)))
		}
		if bboxEmpty(r) {
			panic(fmt.Errorf("bad bbox: i=%d c=%s", i, showBBox(r)))
		}

	}
	return adj
}

func (ts *topoState) prepend(i int) {
	topo := []int{i}
	for _, j := range ts.order {
		if i == j {
			panic(i)
		}
	}
	ts.order = append(topo, ts.order...)
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
	return a.Urx < b.Llx
}
