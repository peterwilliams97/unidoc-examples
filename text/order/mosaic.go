package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

/*
 *  Heckbert's stack-based filling algorithm.

 */

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
	sortIota(rects, orderLlx, selectLlx)
	sortIota(rects, orderUrx, selectUrx)
	sortIota(rects, orderLly, selectLly)
	sortIota(rects, orderUry, selectUry)

	checkIota(rects, orderLlx, selectLlx)
	checkIota(rects, orderUrx, selectUrx)
	checkIota(rects, orderLly, selectLly)
	checkIota(rects, orderUry, selectUry)

	return mosaic{
		rects:    rects,
		orderLlx: orderLlx,
		orderUrx: orderUrx,
		orderLly: orderLly,
		orderUry: orderUry,
	}

}

type mosaic struct {
	rects    []idRect
	orderLlx []int
	orderUrx []int
	orderLly []int
	orderUry []int
}

// idRect is a numbered rectangle. The number is used to find rectangles.
type idRect struct {
	model.PdfRectangle
	id                        int
	left, right, above, below []int
}

func selectLlx(r idRect) float64 { return r.Llx }
func selectUrx(r idRect) float64 { return r.Urx }
func selectLly(r idRect) float64 { return r.Lly }
func selectUry(r idRect) float64 { return r.Ury }

func (idr idRect) String() string {
	return fmt.Sprintf("(%s %4d*)", showBBox(idr.PdfRectangle), idr.id)
}

func (m mosaic) rectString(idr idRect) string {
	var unions, neighbours string
	{
		vert := append(idr.above, idr.id)
		vert = append(vert, idr.below...)
		horz := append(idr.left, idr.id)
		horz = append(horz, idr.right...)
		a := m.unionString("vert", vert, above)
		l := m.unionString("horz", horz, left)
		unions = a + l
	}
	{
		a := m.neighborString("above", idr.above)
		l := m.neighborString("left", idr.left)
		r := m.neighborString("right", idr.right)
		b := m.neighborString("below", idr.below)
		neighbours = a + l + r + b
	}
	return fmt.Sprintf("%s %s %s", idr.String(), unions, neighbours)
}

func (m mosaic) unionString(name string, order []int, way direction) string {
	rl := m.asRectList(order)
	// common.Log.Info("unionString %q way=%d rl=%d order=%d %v", name, way, len(rl), len(order), order)
	r := intersectWay(way, rl...)
	return fmt.Sprintf("\n\t %6s=%s", name, showBBox(r))
}

func (m mosaic) neighborString(name string, order []int) string {
	if len(order) == 0 {
		return ""
	}
	parts := make([]string, len(order))
	for i, o := range order {
		parts[i] = m.getRect(o).String()
	}
	return fmt.Sprintf("\n\t%6s=%s", name, strings.Join(parts, "\n\t     | "))
}

func (m mosaic) asRectList(order []int) rectList {
	rl := make(rectList, len(order))
	for i, o := range order {
		rl[i] = m.rects[o].PdfRectangle
	}
	return rl
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

func (m mosaic) validate() {
	checkIota(m.rects, m.orderLlx, selectLlx)
	checkIota(m.rects, m.orderUrx, selectUrx)
	checkIota(m.rects, m.orderLly, selectLly)
	checkIota(m.rects, m.orderUry, selectUry)
}

// intersectXY returns the indexes of the idRects that intersect
//  x, y: `llx` ≤ x ≤ `urx` and `lly` ≤ y ≤ `ury`.
func (m mosaic) intersectXY(llx, urx, lly, ury float64) []int {
	m.validate()
	xvals := m.intersectX(llx, urx)
	yvals := m.intersectY(lly, ury)
	return sliceIntersection(xvals, yvals)
}

// intersectX returns the m.rects indexes that intersect  x: `llx` ≤ x ≤ `urx`.
func (m mosaic) intersectX(llx, urx float64) []int {
	if llx > urx {
		panic(fmt.Errorf("mosaic.intersectX params: llx=%g urx=%g", llx, urx))
	}
	if llx == urx {
		return nil
	}
	// i0 is the first element for which r.Urx >= llx
	// common.Log.Info("intersectX: llx=%5.1f urx=%5.1f %d rects =============================================",
	// 	llx, urx, len(m.rects))
	m.validate()
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
	// First i1 is highest r.Llx < urx
	i1, _ := m.findLlx(urx)
	// common.Log.Info("<< i1=%d", i1)
	// if i1 < 0 {
	// 	common.Log.Info(">> i1=%d ", i1)
	// } else {
	// 	common.Log.Info(">> i1=%d %s", i1, m.rects[m.orderLlx[i1]])
	// }

	olap := sliceIntersection(m.orderUrx[i0:], m.orderLlx[:i1+1])
	// m.show("  Left match", m.orderUrx[i0:])
	// m.show(" Right match", m.orderLlx[:i1+1])
	// m.show("Intersection", olap)

	{
		var r idRect
		r.Llx = llx
		r.Urx = urx
		for j, o := range olap {
			c := m.rects[o]
			if !intersectsX(r.PdfRectangle, c.PdfRectangle) {
				panic(fmt.Errorf("No x overlap: j=%d of %d\n\tr=%s\n\tc=%s", j, len(olap), r, c))
			}
		}
	}
	return olap
}

// intersectY returns the indexes of the idRects that intersect  y: `lly` ≤ y ≤ `ury`.
func (m mosaic) intersectY(lly, ury float64) []int {
	if lly > ury {
		panic(fmt.Errorf("mosaic.intersectY params: lly=%g ury=%g", lly, ury))
	}
	if lly == ury {
		return nil
	}
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

	// common.Log.Info(">> i1=%d %s", i1, m.rects[m.orderLly[i1]])

	olap := sliceIntersection(m.orderUry[i0:], m.orderLly[:i1+1])
	// m.show("  Left match", m.orderUry[i0:])
	// m.show(" Right match", m.orderLly[:i1+1])
	// m.show("Intersection", olap)

	{
		var r idRect
		r.Lly = lly
		r.Ury = ury
		for j, o := range olap {
			c := m.rects[o]
			if !intersectsY(r.PdfRectangle, c.PdfRectangle) {
				panic(fmt.Errorf("No y overlap: j=%d of %d\n\tr=%s\n\tc=%s", j, len(olap), r, c))
			}
		}
	}
	return olap
}

// findLlx returns the index of the idRect with highest Llx ≤ `x`.
// Returns index into m.orderLlx, index into m.rects
func (m mosaic) findLlx(x float64) (int, int) {
	checkIota(m.rects, m.orderLlx, selectLlx)
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

var findVerbose = false

// find returns the highest index `idx` in `order` for which
// `selector`(m.rects[`order`[idx]]) ≤ `x` .
// The second return value is the index into m.rects
// -1, -1 is returned if there is no match.
func (m mosaic) find(x float64, order []int, selector func(idRect) float64) (int, int) {
	idx := -1
	// if findVerbose {
	// 	common.Log.Info("^^ find: x=%5.3f order=%d", x, len(order))
	// }
	for i, o := range order {
		r := m.rects[o]
		if selector(r) < x {
			idx = i
		}
		// if findVerbose {
		// 	common.Log.Info("i=%d r=%s < %5.2f=%t idx=%d", i, r, x, selector(r) < x, idx)

		if i > 0 {
			j := i - 1
			p := order[j]
			t := m.rects[p]
			if selector(r) < selector(t) {
				panic("out of order")
			}
		}
		// }
	}
	if idx == -1 {
		return -1, -1
	}
	if findVerbose {
		common.Log.Info("idx=%d", idx)
	}
	return idx, order[idx]
}

func (m mosaic) bestVert(order []int, minGap float64) (model.PdfRectangle, []int) {
	rrl := m.asRectList(order)
	longest := 0.0
	besti0 := -1
	besti1 := -1
	var bestr model.PdfRectangle
	for i0 := 0; i0 < len(order); i0++ {
		for i1 := i0; i1 < len(order); i1++ {
			rl := rrl[i0 : i1+1]
			r := intersectWay(above, rl...)
			if r.Urx-r.Llx < minGap {
				continue
			}
			h := r.Ury - r.Lly
			if h > longest {
				longest = h
				besti0 = i0
				besti1 = i1
				bestr = r
			}
		}
	}
	if besti0 < 0 {
		return bestr, nil
	}
	return bestr, order[besti0 : besti1+1]
}

type direction int

const (
	above direction = iota
	below
	left
	right
)

func shiftWay(r model.PdfRectangle, delta float64, way direction) model.PdfRectangle {
	switch way {
	case above:
		r.Lly += delta
		r.Ury += delta
	case left:
		r.Llx -= delta
		r.Urx -= delta
	case right:
		r.Llx += delta
		r.Urx += delta
	case below:
		r.Lly -= delta
		r.Ury -= delta
	}
	return r
}

func intersectWay(way direction, rl ...model.PdfRectangle) model.PdfRectangle {
	// common.Log.Info("intersectWay: way=%d rl=%d", way, len(rl))
	r0 := rl[0]
	for _, r1 := range rl[1:] {
		// r00 := r0
		switch way {
		case above, below:
			r0 = model.PdfRectangle{
				Llx: math.Max(r0.Llx, r1.Llx),
				Urx: math.Min(r0.Urx, r1.Urx),
				Lly: math.Min(r0.Lly, r1.Lly),
				Ury: math.Max(r0.Ury, r1.Ury),
			}
		case left, right:
			r0 = model.PdfRectangle{
				Llx: math.Min(r0.Llx, r1.Llx),
				Urx: math.Max(r0.Urx, r1.Urx),
				Lly: math.Max(r0.Lly, r1.Lly),
				Ury: math.Min(r0.Ury, r1.Ury)}
		}
		// common.Log.Info("@# %3d: %s & %s -> %s", i, showBBox(r00), showBBox(r1), showBBox(r0))
	}
	// common.Log.Info("!! %s", showBBox(r0))
	return r0
}

var maxDepth = 0

func (m *mosaic) intersectRecursive(bound model.PdfRectangle, idr idRect, delta float64, way direction,
	root, depth int) []int {
	common.Log.Info("intersectRecursive root=%d depth=%d  way=%d delta=%g idr=%s",
		root, depth, way, delta, idr)
	if depth > 100 {
		panic("depth")
	}
	if depth > maxDepth {
		maxDepth = depth
		common.Log.Info("!!!!maxDepth=%d", maxDepth)
	}

	r0 := shiftWay(idr.PdfRectangle, delta, way)
	bound = intersectWay(way, r0, bound)
	r, ok := geometricIntersection(bound, r0)
	if !ok {
		return nil
	}
	vals0 := m.intersectXY(r.Llx, r.Urx, r.Lly, r.Ury)
	vals0 = subtract(vals0, idr.id)
	fmt.Printf("\t << root=%d depth=%d: vals0=%d %+v\n", root, depth, len(vals0), vals0)
	indexes := vals0[:]
	for _, i := range vals0 {
		idr := m.getRect(i)
		vals := m.intersectRecursive(bound, idr, delta, way, root, depth+1)
		indexes = append(indexes, vals...)
	}
	fmt.Printf("\t >> root=%d depth=%d: way=%d indexes=%d %+v\n", root, depth, way, len(indexes), indexes)
	if way == above || way == below {
		for j, o := range indexes {
			c := m.rects[o]
			if !intersectsX(idr.PdfRectangle, c.PdfRectangle) {
				common.Log.Error("\n\t   idr=%s", m.rectString(idr))
				common.Log.Error("\n\t     r=%s", showBBox(r))
				panic(fmt.Errorf("intersectRecursive: No x overlap: j=%d\n\tr=%s %+v\n\tc=%s %+v",
					j, idr, idr.PdfRectangle, c, c.PdfRectangle))
			}
		}
	} else {
		for j, o := range indexes {
			c := m.rects[o]
			if !intersectsY(idr.PdfRectangle, c.PdfRectangle) {
				common.Log.Error("\n\t   idr=%s", m.rectString(idr))
				common.Log.Error("\n\t     r=%s", showBBox(r))
				panic(fmt.Errorf("intersectRecursive: No y overlap: j=%d\n\tr=%s %+v\n\tc=%s %+v",
					j, idr, idr.PdfRectangle, c, c.PdfRectangle))
			}
		}
	}
	if len(indexes) > 0 {
		rl := m.asRectList(indexes)
		r := intersectWay(way, rl...)
		common.Log.Info("XXX: idr=%s\n\tr=%s indexes=%d %v", idr, showBBox(r), len(indexes), indexes)
		for i, o := range indexes {
			fmt.Printf("%4d: %s\n", i, m.getRect(o))
		}
		if r.Llx >= r.Urx || r.Lly >= r.Ury {
			panic("no intersecton")
		}
	}
	return indexes
}

func (m *mosaic) connectRecursive(delta float64) {
	m.validate()
	for i, r := range m.rects {
		r.above = m.intersectRecursive(r.PdfRectangle, r, delta, above, r.id, 0)
		r.left = m.intersectRecursive(r.PdfRectangle, r, delta, left, r.id, 0)
		r.right = m.intersectRecursive(r.PdfRectangle, r, delta, right, r.id, 0)
		r.below = m.intersectRecursive(r.PdfRectangle, r, delta, below, r.id, 0)

		r.above = subtract(r.above, r.id)
		r.left = subtract(r.left, r.id)
		r.right = subtract(r.right, r.id)
		r.below = subtract(r.below, r.id)
		m.rects[i] = r
		m.validate()

		for j, o := range r.above {
			c := m.rects[o]
			if !intersectsX(r.PdfRectangle, c.PdfRectangle) {
				common.Log.Error("\n\t     r=%s", m.rectString(r))
				panic(fmt.Errorf("No x overlap: j=%d\n\tr=%s %+v\n\tc=%s %+v",
					j, r, r.PdfRectangle, c, c.PdfRectangle))
			}
		}
		common.Log.Info("connectRecursive %d: %s", i, m.rectString(r))
		// panic("done")
		// }
	}
}
func (m *mosaic) connect(border float64) {
	m.validate()
	i0 := 0
	for i, r := range m.rects[i0:] {
		r.below = m.intersectXY(r.Llx, r.Urx, r.Lly-border, r.Ury)
		r.above = m.intersectXY(r.Llx, r.Urx, r.Ury, r.Ury+border)
		r.left = m.intersectXY(r.Llx-border, r.Llx, r.Ury, r.Ury)
		r.right = m.intersectXY(r.Urx, r.Urx+border, r.Ury, r.Ury)

		r.above = subtract(r.above, r.id)
		r.left = subtract(r.left, r.id)
		r.right = subtract(r.right, r.id)
		r.below = subtract(r.below, r.id)
		m.rects[i0+i] = r
		m.validate()

		for j, o := range r.above {
			c := m.rects[o]
			if !intersectsX(r.PdfRectangle, c.PdfRectangle) {
				common.Log.Error("\n\t     r=%s", m.rectString(r))
				panic(fmt.Errorf("No x overlap: j=%d\n\tr=%s %+v\n\tc=%s %+v",
					j, r, r.PdfRectangle, c, c.PdfRectangle))
			}
		}
		// panic("done")
		// }
	}
}

func subtract(order []int, victim int) []int {
	var reduced []int
	for _, o := range order {
		if o != victim {
			reduced = append(reduced, o)
		}
	}
	return reduced
}

// func (m mosaic) subset(order []int) mosaic {
// 	rects := m.getRects(order)
// 	rl := idRectsToRectList(rects)
// 	return createMosaic(rl)
// }

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
