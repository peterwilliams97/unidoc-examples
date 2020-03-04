package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

/*
 *  Heckbert's stack-based filling algorithm.

 */

// idRect is a numbered rectangle. The number is used to find rectangles.
type idRect struct {
	model.PdfRectangle
	id                        int
	left, right, above, below []int
}

// mosaic is a list of numbered rectangles.
// rects[i].id = i
// order*** are indexes for finding rectangles efficiently.
// `orderLlx` contains indexes of `rects` sorted by Llx
type mosaic struct {
	rects    []idRect
	orderLlx []int
	orderUrx []int
	orderLly []int
	orderUry []int
}

func createMosaic(rl rectList) mosaic {
	rects := make([]idRect, len(rl))
	for i, r := range rl {
		rects[i] = idRect{id: i, PdfRectangle: r}
	}
	orderLlx := orderedBy(rects, selectLlx)
	orderUrx := orderedBy(rects, selectUrx)
	orderLly := orderedBy(rects, selectLly)
	orderUry := orderedBy(rects, selectUry)

	m := mosaic{
		rects:    rects,
		orderLlx: orderLlx,
		orderUrx: orderUrx,
		orderLly: orderLly,
		orderUry: orderUry,
	}

	m.validate()
	return m
}

func selectLlx(r idRect) float64 { return r.Llx }
func selectUrx(r idRect) float64 { return r.Urx }
func selectLly(r idRect) float64 { return r.Lly }
func selectUry(r idRect) float64 { return r.Ury }

// String returns a string that identifies `idr`.
func (idr idRect) String() string {
	return fmt.Sprintf("(%s %4d*)", showBBox(idr.PdfRectangle), idr.id)
}

// String returns a string that shows the details of `idr`.
func (m mosaic) rectString(idr idRect) string {
	var unions, neighbours string
	{
		vert := append(idr.above, idr.id)
		vert = append(vert, idr.below...)
		horz := append(idr.left, idr.id)
		horz = append(horz, idr.right...)
		a := m.unionString("vert", vert, vertical)
		l := m.unionString("horz", horz, horizontal)
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

// unionString returns a string showing the bounding box
func (m mosaic) unionString(name string, order []int, ax axis) string {
	rl := m.asRectList(order)
	r := intersectUnion(ax, rl...)
	return fmt.Sprintf("\n\t %6s=%s", name, showBBox(r))
}

func (m mosaic) neighborString(name string, order []int) string {
	if len(order) == 0 {
		return ""
	}
	parts := make([]string, len(order))
	for i, o := range order {
		parts[i] = m.rects[o].String()
	}
	return fmt.Sprintf("\n\t%6s=%s", name, strings.Join(parts, "\n\t     | "))
}

// asRectList returns m.rects[o] ∀ o ∈ `order` as a rectList.
func (m mosaic) asRectList(order []int) rectList {
	rl := make(rectList, len(order))
	for i, o := range order {
		rl[i] = m.rects[o].PdfRectangle
	}
	return rl
}

// validate checks that `idr` is valid.
func (idr idRect) validate() {
	if !bboxValid(idr.PdfRectangle) {
		w := idr.Urx - idr.Llx
		h := idr.Ury - idr.Lly
		panic(fmt.Errorf("idr.validate rect %s %g x %g", idr, w, h))
	}
	if idr.id < 0 {
		panic(fmt.Errorf("idr.validate id %s", idr))
	}
}

// validate checks that `m` is valid.
func (m mosaic) validate() {
	checkOrder(m.rects, m.orderLlx, selectLlx)
	checkOrder(m.rects, m.orderUrx, selectUrx)
	checkOrder(m.rects, m.orderLly, selectLly)
	checkOrder(m.rects, m.orderUry, selectUry)
	for i, idr := range m.rects {
		idr.validate()
		if i != idr.id {
			panic(fmt.Errorf("idRect out of order: i=%d idr=%s", i, idr))
		}
	}
}

// intersectXY returns the indexes of the idRects that intersect
//  x, y: `llx` ≤ x ≤ `urx` and `lly` ≤ y ≤ `ury`.
func (m mosaic) intersectXY(llx, urx, lly, ury float64) []int {
	m.validate()
	xvals := m.intersectX(llx, urx)
	yvals := m.intersectY(lly, ury)
	return intersectSlices(xvals, yvals)
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
	m.validate()
	i0, _ := m.findUrx(llx)

	if i0 < 0 {
		i0 = 0
	} else if i0 == len(m.orderUrx)-1 {
		return nil
	} else {
		i0++
	}

	// i1 is the last element for which r.Llx ≤ `urx`.
	// First i1 is highest r.Llx < urx
	i1, _ := m.findLlx(urx)

	olap := intersectSlices(m.orderUrx[i0:], m.orderLlx[:i1+1])

	if doValidate {
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

// intersectY returns the indexes of the idRects that intersect y: `lly` ≤ y ≤ `ury`.
func (m mosaic) intersectY(lly, ury float64) []int {
	if lly > ury {
		panic(fmt.Errorf("mosaic.intersectY params: lly=%g ury=%g", lly, ury))
	}
	if lly == ury {
		return nil
	}
	// i0 is the first element for which r.Ury >= lly
	i0, _ := m.findUry(lly)
	if i0 < 0 {
		i0 = 0
	} else if i0 == len(m.orderUry)-1 {
		return nil
	} else {
		i0++
	}
	// i1 is the last element for which r.Lly ≤ `ury`.
	i1, _ := m.findLly(ury)

	olap := intersectSlices(m.orderUry[i0:], m.orderLly[:i1+1])

	if doValidate {
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
	checkOrder(m.rects, order, selector)
	idx := -1
	for i, o := range order {
		r := m.rects[o]
		if selector(r) < x {
			idx = i
		}
		if i > 0 {
			j := i - 1
			p := order[j]
			t := m.rects[p]
			if selector(r) < selector(t) {
				panic("out of order")
			}
		}
	}
	if idx == -1 {
		return -1, -1
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
			r := intersectUnion(vertical, rl...)
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

type axis bool

const (
	vertical   axis = false
	horizontal axis = true
)

func (way direction) getAxis() axis {
	switch way {
	case above, below:
		return vertical
	case left, right:
		return horizontal
	default:
		panic(fmt.Errorf("bad direction. way=%v", way))
	}
}

// shiftWay returns `r` shifted by distance `delta` in direction `way`.
func shiftWay(way direction, delta float64, r model.PdfRectangle) model.PdfRectangle {
	switch way {
	case above:
		r.Lly -= delta
		r.Ury -= delta
	case below:
		r.Lly += delta
		r.Ury += delta
	case left:
		r.Llx -= delta
		r.Urx -= delta
	case right:
		r.Llx += delta
		r.Urx += delta
	default:
		panic(fmt.Errorf("bad direction. way=%v", way))
	}
	return r
}

// intersectUnion returns the union of rectangles `rl` in direction `way` and the intersection of the
// rectangles in the traverse direction to `way`.
func intersectUnion(ax axis, rl ...model.PdfRectangle) model.PdfRectangle {
	// common.Log.Info("intersectUnion: way=%d rl=%d", way, len(rl))
	r0 := rl[0]
	// common.Log.Info("@# %3d: %s s", 0, showBBox(r0))
	for _, r1 := range rl[1:] {
		// r00 := r0
		switch ax {
		case vertical:
			r0 = model.PdfRectangle{
				Llx: math.Max(r0.Llx, r1.Llx),
				Urx: math.Min(r0.Urx, r1.Urx),
				Lly: math.Min(r0.Lly, r1.Lly),
				Ury: math.Max(r0.Ury, r1.Ury),
			}
		case horizontal:
			r0 = model.PdfRectangle{
				Llx: math.Min(r0.Llx, r1.Llx),
				Urx: math.Max(r0.Urx, r1.Urx),
				Lly: math.Max(r0.Lly, r1.Lly),
				Ury: math.Min(r0.Ury, r1.Ury)}
		default:
			panic(fmt.Errorf("bad axis. ax=%v", ax))
		}
		// common.Log.Info("@# %3d: %s & %s -> %s", i+1, showBBox(r00), showBBox(r1), showBBox(r0))
	}
	// common.Log.Info("!! %s", showBBox(r0))
	return r0
}

// findIntersectionWay walks through the `m.rects` indexes in `order` applies intersectUnion(`way`) to
// them and stops immediately before the intersection becomes zero.
func (m mosaic) findIntersectionWay(way direction, bound model.PdfRectangle, order []int) []int {
	if len(order) == 0 {
		return nil
	}
	common.Log.Debug("findIntersectionWay way=%d bound=%sorder= %d %v ==================",
		way, showBBox(bound), len(order), order)
	var isect []int
	for i, o := range order {
		r := m.rects[o]
		bound = intersectUnion(way.getAxis(), bound, r.PdfRectangle)
		// common.Log.Info("@# %3d: %s & %s -> %s", i, showBBox(r00), showBBox(r1), showBBox(r0))
		if bound.Llx >= bound.Urx || bound.Lly >= bound.Ury {
			break
		}
		common.Log.Debug("findIntersectionWay %d: bound=%s r=%s indexes= %d %v",
			i, showBBox(bound), showBBox(r.PdfRectangle), len(isect), isect)
		isect = append(isect, o)
	}
	// common.Log.Info("!! %s", showBBox(r0))

	if len(isect) == 0 {
		return nil
	}

	if doValidate {
		indexes := isect
		rl := m.asRectList(indexes)
		r := intersectUnion(way.getAxis(), rl...)
		common.Log.Info("findIntersectionWay: way=%d indexes=%d %v\n\tbound=%s\n\t    r=%s",
			way, len(indexes), indexes, showBBox(bound), showBBox(r))
		for i, o := range indexes {
			fmt.Printf("%4d: %s\n", i, m.rects[o])
		}
		if r.Llx >= r.Urx || r.Lly >= r.Ury {
			panic(fmt.Errorf("findIntersectionWay: no intersecton: way=%d", way))
		}
	}
	return isect
}

// connectRecursive updates each m.rects[i] by connecting its above, left, right and below slices with
// the indexes of the m.rects elements in these locations. It does this by sliding the rectangle
// by `delta` in this direction.
func (m *mosaic) connectRecursive(delta float64) {
	m.validate()
	for i, r := range m.rects {
		r.above = m.intersectRecursive(r, r, delta, above, r.id, 0, r.PdfRectangle)
		r.left = m.intersectRecursive(r, r, delta, left, r.id, 0, r.PdfRectangle)
		r.right = m.intersectRecursive(r, r, delta, right, r.id, 0, r.PdfRectangle)
		r.below = m.intersectRecursive(r, r, delta, below, r.id, 0., r.PdfRectangle)

		r.above = subtract(r.above, r.id)
		r.left = subtract(r.left, r.id)
		r.right = subtract(r.right, r.id)
		r.below = subtract(r.below, r.id)
		m.rects[i] = r
		m.validate()

		if doValidate {
			for j, o := range r.above {
				c := m.rects[o]
				if !intersectsX(r.PdfRectangle, c.PdfRectangle) {
					common.Log.Error("\n\t     r=%s", m.rectString(r))
					panic(fmt.Errorf("No x overlap: j=%d\n\tr=%s %+v\n\tc=%s %+v",
						j, r, r.PdfRectangle, c, c.PdfRectangle))
				}
			}
		}
		common.Log.Debug("connectRecursive %d: %s", i, m.rectString(r))
	}
}

var maxDepth = 0

// intersectRecursive returns the indexes of the rectangles that are enclosed by `idr` shifted
// `delta` in direction `way`.
func (m *mosaic) intersectRecursive(idr0, idr idRect, delta float64, way direction,
	root, depth int, bound model.PdfRectangle) []int {
	common.Log.Debug("intersectRecursive root=%d depth=%d way=%d delta=%g idr=%s",
		root, depth, way, delta, idr)
	if depth > 100 {
		panic("depth")
	}
	if depth > maxDepth {
		maxDepth = depth
		common.Log.Info("!!!!maxDepth=%d root=%d way=%d", maxDepth, root, way)
	}

	r := shiftWay(way, delta, idr.PdfRectangle)
	bound = intersectUnion(way.getAxis(), bound, r)
	if doValidate { // validation
		if way == above || way == below {
			if bound.Llx < r.Llx || bound.Urx > r.Urx {
				common.Log.Error("way=%d\n\tbound=%s\n\t    r=%s",
					way, showBBox(bound), showBBox(r))
				panic("bound x")
			}
		} else {
			if bound.Lly < r.Lly || bound.Ury > r.Ury {
				panic("bound y")
			}
		}

		if way == above || way == below {
			dllx := bound.Llx - r.Llx
			durx := bound.Urx - r.Urx
			if dllx < 0 || durx > 0 {
				common.Log.Error("way=%d dllx=%g durx=%g\n\tbound=%s\n\t    r=%s",
					way, dllx, durx, showBBox(bound), showBBox(r))
				panic("bound x")
			}
		} else {
			dllx := bound.Lly - r.Lly
			durx := bound.Ury - r.Ury
			if dllx < 0 || durx > 0 {
				common.Log.Error("way=%d dllx=%g durx=%g\n\tbound=%s\n\t    r=%s",
					way, dllx, durx, showBBox(bound), showBBox(r))
				panic("bound y")
			}
		}
	}

	r = constrictTraverse(way, r, idr0.PdfRectangle)
	r = constrictTraverse(way, r, bound)
	if r.Llx >= r.Urx || r.Lly >= r.Ury {
		panic("!!1")
		return nil
	}
	if bound.Llx >= bound.Urx || bound.Lly >= bound.Ury {
		panic("!!2")
		return nil
	}

	filter := func(vals []int) []int {
		vals = subtract(vals, idr0.id)
		vals = subtract(vals, idr.id)
		return vals
	}

	vals0 := m.intersectXY(r.Llx, r.Urx, r.Lly, r.Ury)
	vals0 = filter(vals0)
	vals0 = m.findIntersectionWay(way, bound, vals0)
	if len(vals0) == 0 {
		return nil
	}
	// fmt.Printf("\t << root=%d depth=%d: vals0=%d %+v\n", root, depth, len(vals0), vals0)
	indexes := vals0[:]
	common.Log.Debug("  vals0=%d %v", len(vals0), vals0)
	for i, o := range vals0 {
		idr := m.rects[o]
		vals := m.intersectRecursive(idr0, idr, delta, way, root, depth+1, bound)
		vals = filter(vals)
		common.Log.Debug("vals[%d]=%d %v", i, len(vals0), vals0)
		indexes = append(indexes, vals...)
		indexes = m.findIntersectionWay(way, bound, indexes)
	}
	if doValidate { // validation
		common.Log.Info("\t >> root=%d depth=%d: way=%d indexes=%d %+v", root, depth, way, len(indexes), indexes)
		if way == above || way == below {
			for j, o := range indexes {
				c := m.rects[o]
				if !intersectsX(idr.PdfRectangle, c.PdfRectangle) {
					common.Log.Error("idr0=%s", showBBox(idr0.PdfRectangle))
					common.Log.Error(" idr=%s", showBBox(idr.PdfRectangle))
					common.Log.Error("   r=%s", showBBox(r))
					for k, u := range indexes {
						fmt.Printf("%8d: %s %t\n", k, m.rects[u], k == j)
					}
					panic(fmt.Errorf("intersectRecursive: No x overlap: j=%d way=%d\n\tr=%s %+v\n\tc=%s %+v",
						j, way, idr, idr.PdfRectangle, c, c.PdfRectangle))
				}
			}
		} else {
			for j, o := range indexes {
				c := m.rects[o]
				if !intersectsY(idr.PdfRectangle, c.PdfRectangle) {
					common.Log.Error("\n\t   idr=%s", m.rectString(idr))
					common.Log.Error("\n\t     r=%s", showBBox(r))
					panic(fmt.Errorf("intersectRecursive: No y overlap: j=%d way=%d\n\tr=%s %+v\n\tc=%s %+v",
						j, way, idr, idr.PdfRectangle, c, c.PdfRectangle))
				}
			}
		}
		if len(indexes) > 0 {
			rl := m.asRectList(indexes)
			r := intersectUnion(way.getAxis(), rl...)
			common.Log.Info("XXX: vals0=%d\n\tbound=%s\n\tidr0=%s\n\t idr=%s\n\tr=%s indexes=%d %v",
				len(vals0), showBBox(bound), idr0, idr, showBBox(r), len(indexes), indexes)
			for i, o := range indexes {
				fmt.Printf("%4d: %s\n", i, m.rects[o])
			}
			if r.Llx >= r.Urx || r.Lly >= r.Ury {
				panic(fmt.Errorf("no intersecton: way=%d", way))
			}
		}
	}
	return indexes
}

// constrictTraverse constricts `r` in the traverse direction of `way`.
func constrictTraverse(way direction, r, r0 model.PdfRectangle) model.PdfRectangle {
	// common.Log.Info("intersectUnion: way=%d rl=%d", way, len(rl))
	switch way {
	case above, below:
		r.Llx = math.Max(r0.Llx, r.Llx)
		r.Urx = math.Min(r0.Urx, r.Urx)
	case left, right:
		r.Lly = math.Max(r0.Lly, r.Lly)
		r.Ury = math.Min(r0.Ury, r.Ury)
	}
	// common.Log.Info("!! %s", showBBox(r0))
	return r
}

// subtract returns `order` with `victim` removed.
func subtract(order []int, victim int) []int {
	var reduced []int
	for _, o := range order {
		if o != victim {
			reduced = append(reduced, o)
		}
	}
	return reduced
}

// getRects returns the rectangles from m.rects with indexes `order`.
func (m mosaic) getRects(order []int) []idRect {
	rects := make([]idRect, len(order))
	for i, o := range order {
		rects[i] = m.rects[o]
	}
	return rects
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

// orderedBy returns a slice of indexes into `rects` sorted by `selector`.
// If the returned slice is `order` then selector(rects[order[i]]) will increase with increasing i.
func orderedBy(rects []idRect, selector func(idRect) float64) []int {
	order := make([]int, len(rects))
	for i := range rects {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return ordering(rects, order, selector, i, j) })
	return order
}

// checkOrder checks that `order` is in the order produced by `orderedBy`.
func checkOrder(rects []idRect, order []int, selector func(idRect) float64) {
	// if !doValidate {
	// 	return
	// }
	sorted := sort.SliceIsSorted(order, func(i, j int) bool {
		return ordering(rects, order, selector, i, j)
	})
	if !sorted {
		panic("!sorted")
	}
}

// ordering is a sorting function that gives `order` such that
// selector(rects[order[i]]) increases with increasing i.
func ordering(rects []idRect, order []int, selector func(idRect) float64, i, j int) bool {
	oi, oj := order[i], order[j]
	ri, rj := rects[oi], rects[oj]
	xi, xj := selector(ri), selector(rj)
	if xi != xj {
		return xi < xj
	}
	return ri.id < rj.id
}
