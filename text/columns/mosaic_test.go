package main

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

func TestMosaic(t *testing.T) {
	rand.Seed(111)
	n := 10
	rl := make(rectList, n)
	x := make([]float64, 4)
	for i := 0; i < n; i++ {
		for j := 0; j < 4; j++ {
			x[j] = rand.Float64()
		}
		rl[i] = model.PdfRectangle{
			Llx: 50.0 * x[0],
			Urx: 50.0*x[0] + 50.0*x[1],
			Lly: 40.0 * x[2],
			Ury: 40.0*x[2] + 60.0*x[3],
		}
	}

	m := createMosaic(rl)

	show := func(name string, order []int) {
		fmt.Printf("%s --------------- %v\n", name, order)
		for i, o := range order {
			fmt.Printf("%4d: %s\n", i, getRect(m, o))
		}
	}
	show("Llx", m.orderLlx)
	show("Urx", m.orderUrx)
	show("Lly", m.orderLly)
	show("Ury", m.orderUry)

	start, end, delta := 1.0, 100.0, 20.0
	mul := math.Sqrt(delta)
	common.Log.Info("findLLx ----------------")
	for x := start; x < end; x *= mul {
		i, o := m.findLlx(x)
		fmt.Printf("  x=%5.1f i=%d o=%d r=%s\n", x, i, o, getRect(m, o))
	}
	common.Log.Info("findUrx ----------------")
	for x := start; x < end; x *= mul {
		i, o := m.findUrx(x)
		fmt.Printf("  x=%5.1f i=%d o=%d r=%s\n", x, i, o, getRect(m, o))
	}
	common.Log.Info("findLLy ----------------")
	for y := start; y < end; y *= mul {
		i, o := m.findLly(y)
		fmt.Printf("  y=%5.1f i=%d o=%d r=%s\n", y, i, o, getRect(m, o))
	}
	common.Log.Info("findUry ----------------")
	for y := start; y < end; y *= mul {
		i, o := m.findUry(y)
		fmt.Printf("  y=%5.1f i=%d o=%d r=%s\n", y, i, o, getRect(m, o))
	}

	{
		llx, urx := 100.0, 120.0
		name := fmt.Sprintf("Test **OVERLAP** intersectX: x=%5.1f - %5.1f", llx, urx)
		common.Log.Info("%40s ===================", name)
		olap := m.intersectX(llx, urx)
		m.show(name, olap)
		if len(olap) > 0 {
			t.Fatalf("overlap X: %s %v", name, olap)
		}
	}

	{
		llx, urx := 100.0, 120.0
		name := fmt.Sprintf("Test **OVERLAP** intersectY: x=%5.1f - %5.1f", llx, urx)
		common.Log.Info("%40s ===================", name)
		olap := m.intersectY(llx, urx)
		m.show(name, olap)
		if len(olap) > 0 {
			t.Fatalf("overlap X: %s %v", name, olap)
		}
	}

	fmt.Println("intersectX ------------------------------------------------")
	for z := start; z <= end; z += delta {
		llx := z
		urx := z + end/5.0
		name := fmt.Sprintf("intersectX x=%5.1f - %5.1f", llx, urx)
		// fmt.Printf("%40s ==========*========\n", name)
		olap := m.intersectX(llx, urx)
		m.show(name, olap)
	}

	fmt.Println("intersectY ------------------------------------------------")
	for z := start; z <= end; z += delta {
		lly := z
		ury := z + end/5.0
		name := fmt.Sprintf("intersectY y=%5.1f - %5.1f", lly, ury)
		olap := m.intersectY(lly, ury)
		m.show(name, olap)
	}

	fmt.Println("intersectXY -----------------------------------------------")
	for z := start; z <= end; z += delta {
		llx := z
		urx := llx + 2.0*delta
		lly := end - z
		ury := lly + delta
		name := fmt.Sprintf("intersectXY x=%5.1f - %5.1f & y=%5.1f - %5.1f", llx, urx, lly, ury)
		// fmt.Printf(" %40s ==========*========\n", name)
		olap := m.intersectXY(llx, urx, lly, ury)
		m.show(name, olap)
	}
}

func getRect(m mosaic, o int) idRect {
	var idr idRect
	if o < 0 {
		idr.id = o
	} else {
		idr = m.rects[o]
	}
	return idr
}
