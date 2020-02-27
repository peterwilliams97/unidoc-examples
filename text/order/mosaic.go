package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

func testMosaic() {
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
			fmt.Printf("%4d: %s\n", i, m.rects[o])
		}
	}
	show("Llx", m.orderLlx)
	show("Urx", m.orderUrx)
	// show("Lly", m.orderLly)
	// show("Ury", m.orderUry)

	start, end, delta := 1.0, 100.0, 20.0
	mul := math.Sqrt(delta)
	common.Log.Info("findLLx ----------------")
	for x := start; x < end; x *= mul {
		i, o := m.findLlx(x)
		fmt.Printf("  x=%5.1f i=%d o=%d r=%s\n", x, i, o, m.getRect(o))
	}
	common.Log.Info("findUrx ----------------")
	for x := start; x < end; x *= mul {
		i, o := m.findUrx(x)
		fmt.Printf("  x=%5.1f i=%d o=%d r=%s\n", x, i, o, m.getRect(o))
	}
	common.Log.Info("findLLy ----------------")
	for y := start; y < end; y *= mul {
		i, o := m.findLly(y)
		fmt.Printf("  y=%5.1f i=%d o=%d r=%s\n", y, i, o, m.getRect(o))
	}
	common.Log.Info("findUry ----------------")
	for y := start; y < end; y *= mul {
		i, o := m.findUry(y)
		fmt.Printf("  y=%5.1f i=%d o=%d r=%s\n", y, i, o, m.getRect(o))
	}

	{
		llx, urx := 100.0, 120.0
		name := fmt.Sprintf("Test **OVERLAP** intersectX: x=%5.1f - %5.1f", llx, urx)
		common.Log.Info("%40s ===================", name)
		olap := m.intersectX(llx, urx)
		m.show(name, olap)
		if len(olap) > 0 {
			panic("overlap X")
		}
	}

	{
		llx, urx := 100.0, 120.0
		name := fmt.Sprintf("Test **OVERLAP** intersectY: x=%5.1f - %5.1f", llx, urx)
		common.Log.Info("%40s ===================", name)
		olap := m.intersectY(llx, urx)
		m.show(name, olap)
		if len(olap) > 0 {
			panic("overlap Y")
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
		// panic("done")
	}

	fmt.Println("intersectY ------------------------------------------------")
	for z := start; z <= end; z += delta {
		lly := z
		ury := z + end/5.0
		name := fmt.Sprintf("intersectY y=%5.1f - %5.1f", lly, ury)
		// fmt.Printf("%40s ==========*========\n", name)
		olap := m.intersectY(lly, ury)
		m.show(name, olap)
		// panic("done")
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
		// panic("done")
	}

	os.Exit(-1)
}

// idRect is a numbered rectangle. The number is used to find rectangles.
type idRect struct {
	model.PdfRectangle
	id                        int
	left, right, above, below []int
}

func (idr idRect) String() string {
	return fmt.Sprintf("(%s %4d*)", showBBox(idr.PdfRectangle), idr.id)
}

func (idr idRect) validate() {
	if !validBBox(idr.PdfRectangle) {
		w := idr.Urx - idr.Llx
		h := idr.Ury - idr.Lly
		panic(fmt.Errorf("idr.validate rect %s %g x %g", idr, w, h))
	}
	if idr.id <= 0 {
		panic(fmt.Errorf("validate id %s", idr))
	}
}

type mosaic struct {
	rects    []idRect
	orderLlx []int
	orderUrx []int
	orderLly []int
	orderUry []int
}

// intersectXY returns the indexes of the idRects that intersect
//  x, y: `llx` ≤ x ≤ `urx` and `lly` ≤ y ≤ `ury`.
func (m mosaic) intersectXY(llx, urx, lly, ury float64) []int {
	xvals := m.intersectX(llx, urx)
	yvals := m.intersectY(lly, ury)
	return sliceIntersection(xvals, yvals)
}

// intersectX returns the indexes of the idRects that intersect  x: `llx` ≤ x ≤ `urx`.
func (m mosaic) intersectX(llx, urx float64) []int {
	// i0 is the first element for which r.Urx >= llx
	i0, _ := m.findUrx(llx)
	// common.Log.Info("<< i0=%d", i0)
	if i0 < 0 {
		i0 = 0
	} else if i0 == len(m.orderUrx)-1 {
		return nil
	} else {
		i0++
	}
	// common.Log.Info(">> i0=%d %s", i0, m.rects[m.orderUrx[i0]])

	// i1 is the last element for which r.Llx ≤ `urx`.
	i1, _ := m.findLlx(urx)
	// common.Log.Info("<< i1=%d", i1)
	if i1 < 0 {
		i1 = len(m.orderLlx) - 1
	}
	// common.Log.Info(">> i1=%d %s", i1, m.rects[m.orderLlx[i1]])

	olap := sliceIntersection(m.orderUrx[i0:], m.orderLlx[:i1+1])
	// m.show("  Left match", m.orderUrx[i0:])
	// m.show(" Right match", m.orderLlx[:i1+1])
	// m.show("Intersection", olap)
	return olap
}

// intersectY returns the indexes of the idRects that intersect  y: `lly` ≤ y ≤ `ury`.
func (m mosaic) intersectY(lly, ury float64) []int {
	// i0 is the first element for which r.Ury >= lly
	i0, _ := m.findUry(lly)
	// common.Log.Info("<< i0=%d", i0)
	if i0 < 0 {
		i0 = 0
	} else if i0 == len(m.orderUry)-1 {
		return nil
	} else {
		i0++
	}
	// common.Log.Info(">> i0=%d %s", i0, m.rects[m.orderUry[i0]])

	// i1 is the last element for which r.Lly ≤ `ury`.
	i1, _ := m.findLly(ury)
	// common.Log.Info("<< i1=%d", i1)
	if i1 < 0 {
		i1 = len(m.orderLly) - 1
	}
	// common.Log.Info(">> i1=%d %s", i1, m.rects[m.orderLly[i1]])

	olap := sliceIntersection(m.orderUry[i0:], m.orderLly[:i1+1])
	// m.show("  Left match", m.orderUry[i0:])
	// m.show(" Right match", m.orderLly[:i1+1])
	// m.show("Intersection", olap)
	return olap
}

// findLlx returns the index of the idRect with highest Llx ≤ `x`.
// Returns index into m.orderLlx, index into m.rects
func (m mosaic) findLlx(x float64) (int, int) {
	return m.find(x, m.orderLlx, selectLlx)
}

// findUrx returns the index of the idRect with highest Urx ≤ `x`.
func (m mosaic) findUrx(x float64) (int, int) {
	return m.find(x, m.orderUrx, selectUrx)
}

// findLly returns the index of the idRect with highest Lly ≤ `x`.
func (m mosaic) findLly(x float64) (int, int) {
	return m.find(x, m.orderLly, selectLly)
}

// findUry returns the index of the idRect with highest Ury ≤ `x`.
func (m mosaic) findUry(x float64) (int, int) {
	return m.find(x, m.orderUry, selectUry)
}

// find returns the highest index `idx` in `order` for which
// `selector`(m.rects[`order`[idx]]) ≤ `x` .
// The second return value is the index into m.rects
// -1, -1 is returned if there is no match.
func (m mosaic) find(x float64, order []int, selector func(idRect) float64) (int, int) {
	idx := -1
	for i, o := range order {
		r := m.rects[o]
		if selector(r) < x {
			idx = i
		}
	}
	if idx == -1 {
		return -1, -1
	}
	return idx, order[idx]
}

func selectLlx(r idRect) float64 { return r.Llx }
func selectUrx(r idRect) float64 { return r.Urx }
func selectLly(r idRect) float64 { return r.Lly }
func selectUry(r idRect) float64 { return r.Ury }

func createMosaic(rl rectList) mosaic {
	n := len(rl)
	rects := make([]idRect, n)
	orderLlx := make([]int, n)
	orderUrx := make([]int, n)
	orderLly := make([]int, n)
	orderUry := make([]int, n)
	for i, r := range rl {
		rects[i] = idRect{id: i, PdfRectangle: r}
		orderLlx[i] = i
		orderUrx[i] = i
		orderLly[i] = i
		orderUry[i] = i
	}
	sortIota(rects, orderLlx, func(r idRect) float64 { return r.Llx })
	sortIota(rects, orderUrx, func(r idRect) float64 { return r.Urx })
	sortIota(rects, orderLly, func(r idRect) float64 { return r.Lly })
	sortIota(rects, orderUry, func(r idRect) float64 { return r.Ury })

	checkIota(rects, orderLlx, func(r idRect) float64 { return r.Llx })
	checkIota(rects, orderUrx, func(r idRect) float64 { return r.Urx })
	checkIota(rects, orderLly, func(r idRect) float64 { return r.Lly })
	checkIota(rects, orderUry, func(r idRect) float64 { return r.Ury })

	return mosaic{
		rects:    rects,
		orderLlx: orderLlx,
		orderUrx: orderUrx,
		orderLly: orderLly,
		orderUry: orderUry,
	}

}

func sortIota(rects []idRect, order []int, selector func(idRect) float64) {
	sort.Slice(order, func(i, j int) bool {
		oi, oj := order[i], order[j]
		ri, rj := rects[oi], rects[oj]
		xi, xj := selector(ri), selector(rj)
		if xi != xj {
			return xi < xj
		}
		return ri.id < rj.id
	})
}

func checkIota(rects []idRect, order []int, selector func(idRect) float64) {
	sorted := sort.SliceIsSorted(order, func(i, j int) bool {
		oi, oj := order[i], order[j]
		ri, rj := rects[oi], rects[oj]
		xi, xj := selector(ri), selector(rj)
		if xi != xj {
			return xi < xj
		}
		return ri.id < rj.id
	})
	if !sorted {
		panic("!sorted")
	}
}

// getRects returns the rectangles with indexes `order`.
func (m mosaic) getRects(order []int) []idRect {
	rects := make([]idRect, len(order))
	for i, o := range order {
		rects[i] = m.rects[o]
	}
	return rects
}

// getRect returns the rectangle with index `o`.
func (m mosaic) getRect(o int) idRect {
	if o < 0 {
		return idRect{}
	}
	return m.rects[o]
}

func (m mosaic) show(name string, order []int) {
	olap := order[:]
	sort.Ints(olap)

	// intersectXY x= 61.0 - 101.0 & y= 39.0 -  59.0: 3 [3 6 8]
	s := fmt.Sprintf("%d %+v", len(olap), olap)
	fmt.Printf("## %45s: %-20s ----------\n", name, s)
	for i, o := range order {
		r := m.rects[o]
		fmt.Printf("%4d: %s\n", i, r)
	}
}
