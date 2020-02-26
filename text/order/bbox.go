package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// idRect is a numbered rectangle. The number is used to find rectangles.
type idRect struct {
	model.PdfRectangle
	id int
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

	start, end, delta := 0.0, 100.0, 20.0
	// fmt.Println("findLLx ----------------")
	// for x := start; x < end; x *= mul {
	// 	i, r := m.findLlx(x)
	// 	fmt.Printf("  x=%5.1f i=%d r=%s\n", x, i, r)
	// }
	// fmt.Println("findUrx ----------------")
	// for x := start; x < end; x *= mul {
	// 	i, r := m.findUrx(x)
	// 	fmt.Printf("  x=%5.1f i=%d r=%s\n", x, i, r)
	// }
	// fmt.Println("findLLy ----------------")
	// for x := start; x < end; x *= mul {
	// 	i, r := m.findLly(x)
	// 	fmt.Printf("  x=%5.1f i=%d r=%s\n", x, i, r)
	// }
	// fmt.Println("findUry ----------------")
	// for x := start; x < end; x *= mul {
	// 	i, r := m.findUry(x)
	// 	fmt.Printf("  x=%5.1f i=%d r=%s\n", x, i, r)
	// }

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
		fmt.Printf("%40s ==========*========\n", name)
		olap := m.intersectX(llx, urx)
		m.show(name, olap)
		// panic("done")
	}

	fmt.Println("intersectY ------------------------------------------------")
	for z := start; z <= end; z += delta {
		lly := z
		ury := z + end/5.0
		name := fmt.Sprintf("intersectY y=%5.1f - %5.1f", lly, ury)
		fmt.Printf("%40s ==========*========\n", name)
		olap := m.intersectY(lly, ury)
		m.show(name, olap)
		// panic("done")
		// panic("done")
	}

	fmt.Println("intersectXY -----------------------------------------------")
	for z := start; z <= end; z += delta {
		llx := z
		urx := z + end/5.0
		lly := z
		ury := z + end/5.0
		name := fmt.Sprintf("intersectXY x=%5.1f - %5.1f & y=%5.1f - %5.1f", llx, urx, lly, ury)
		fmt.Printf(" %40s ==========*========\n", name)
		olap := m.intersectXY(llx, urx, lly, ury)
		m.show(name, olap)
		// panic("done")
	}

	os.Exit(-1)
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
func (m mosaic) findLlx(x float64) (int, idRect) {
	return m.find(x, m.orderLlx, func(r idRect) float64 { return r.Llx })
}

// findUrx returns the index of the idRect with highest Urx ≤ `x`.
func (m mosaic) findUrx(x float64) (int, idRect) {
	return m.find(x, m.orderUrx, func(r idRect) float64 { return r.Urx })
}

// findLly returns the index of the idRect with highest Lly ≤ `x`.
func (m mosaic) findLly(x float64) (int, idRect) {
	return m.find(x, m.orderLly, func(r idRect) float64 { return r.Lly })
}

// findUry returns the index of the idRect with highest Ury ≤ `x`.
func (m mosaic) findUry(x float64) (int, idRect) {
	return m.find(x, m.orderUry, func(r idRect) float64 { return r.Ury })
}

func (m mosaic) find(x float64, order []int, selector func(idRect) float64) (int, idRect) {
	idx := -1
	for i, o := range order {
		r := m.rects[o]
		if selector(r) < x {
			idx = i
		}
	}
	var r idRect
	if idx >= 0 {
		o := order[idx]
		r = m.rects[o]
	}
	return idx, r
}

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
	// common.Log.Info("getRects: order= %d %+v\n", len(order), order)
	rects := make([]idRect, len(order))
	for i, o := range order {
		rects[i] = m.rects[o]
		// fmt.Printf("-- rects[%d]=%d=%v\n", i, o, rects[i])
	}
	return rects
}

func (m mosaic) show(name string, order []int) {
	olap := order[:]
	sort.Ints(olap)

	fmt.Printf("## %s: %d %+v ----------\n", name, len(olap), olap)
	for i, o := range order {
		r := m.rects[o]
		fmt.Printf("%4d: %s\n", i, r)
	}
}

func lineBBox(line []extractor.TextMarkArray) model.PdfRectangle {
	bboxes := wordBBoxes(line)
	return rectList(bboxes).union()
}

func wordBBoxes(words []extractor.TextMarkArray) rectList {
	bboxes := make(rectList, 0, len(words))
	for i, w := range words {
		r, ok := w.BBox()
		if !ok {
			panic("bbox")
		}
		if !validBBox(r) {
			panic(fmt.Errorf("bad words[%d]=%s\n -- %s", i, w, showBBox(r)))
		}
		bboxes = append(bboxes, r)
	}
	return bboxes
}

func wordBBoxMap(words []extractor.TextMarkArray) map[float64]extractor.TextMarkArray {
	sigWord := make(map[float64]extractor.TextMarkArray, len(words))
	for _, w := range words {
		b, ok := w.BBox()
		if !ok {
			panic("bbox")
		}
		sig := partEltSig(b)
		sigWord[sig] = w
	}
	return sigWord
}

func bboxWords(sigWord map[float64]extractor.TextMarkArray, bboxes rectList) []extractor.TextMarkArray {
	words := make([]extractor.TextMarkArray, len(bboxes))
	for i, b := range bboxes {
		sig := partEltSig(b)
		w, ok := sigWord[sig]
		if !ok {
			panic(fmt.Errorf("signature: b=%s", showBBox(b)))
		}
		words[i] = w
	}
	return words
}

func mergeXBboxes(bboxes rectList) rectList {
	merged := make(rectList, 0, len(bboxes))
	merged = append(merged, bboxes[0])
	for _, b := range bboxes[1:] {
		numOverlaps := 0
		for j, m := range merged {
			if overlappedX(b, m) {
				merged[j] = rectUnion(b, m)
				numOverlaps++
			}
		}
		if numOverlaps == 0 {
			merged = append(merged, b)
		}
	}
	if len(merged) < len(bboxes) {
		common.Log.Info("EEEEE")
	}
	return merged
}

// overlappedX returns true if `r0` and `r1` overlap on the x-axis. !@#$ There is another version
// of this!
func overlappedX(r0, r1 model.PdfRectangle) bool {
	return overlappedX01(r0, r1) || overlappedX01(r1, r0)
}

func overlappedX01(r0, r1 model.PdfRectangle) bool {
	return (r0.Llx <= r1.Llx && r1.Llx <= r0.Urx) || (r0.Llx <= r1.Urx && r1.Urx <= r0.Urx)
}

func xIntersection(idr idRect, players []idRect) (idRect, bool) {
	p := players[0]
	l := p.Llx
	r := p.Urx
	for _, p := range players[:1] {
		if p.Llx < l {
			l = p.Llx
		}
		if p.Urx > r {
			r = p.Urx
		}
	}
	olap := idr
	if l > olap.Llx {
		olap.Llx = l
	}
	if r < olap.Urx {
		olap.Urx = r
	}
	intsect := olap.Llx < olap.Urx
	// common.Log.Info("xIntersection: l=%5.1f r=%5.1f\n\tplayers=%s\n\tidr=%s\n\t  o=%s", l, r, players, idr, olap)
	return olap, intsect
}

// differentElements returns the elements in `a` that aren't in `b`.
func differentElements(a, b []idRect) []idRect {
	bs := map[int]struct{}{}
	for _, idr := range b {
		bs[idr.id] = struct{}{}
	}
	var diff []idRect
	for _, idr := range a {
		if _, ok := bs[idr.id]; !ok {
			diff = append(diff, idr)
		}
	}
	return diff
}

func idRectsToRectList(gaps []idRect) rectList {
	rl := make(rectList, len(gaps))
	for i, g := range gaps {
		rl[i] = g.PdfRectangle
	}
	return rl
}

// bboxArea returns the area of `bbox`.
func bboxArea(bbox model.PdfRectangle) float64 {
	return math.Abs(bbox.Urx-bbox.Llx) * math.Abs(bbox.Ury-bbox.Lly)
}

// bboxCenter returns coordinates the center of `bbox`.
func bboxCenter(bbox model.PdfRectangle) (float64, float64) {
	cx := (bbox.Llx + bbox.Urx) / 2.0
	cy := (bbox.Lly + bbox.Ury) / 2.0
	// common.Log.Info("&&&& %5.1f -> %5.1f %5.1f", bbox, cx, cy)
	return cx, cy
}

// bboxPerim returns the half perimeter of `bbox`.
func bboxPerim(bbox model.PdfRectangle) float64 {
	return bbox.Width() + bbox.Height()
}

// bboxWidth returns the width of `bbox`.
func bboxWidth(bbox model.PdfRectangle) float64 {
	return bbox.Width()
	return math.Abs(bbox.Urx - bbox.Llx)
}

// bboxHeight returns the height of `bbox`.
func bboxHeight(bbox model.PdfRectangle) float64 {
	return bbox.Height()
	return math.Abs(bbox.Ury - bbox.Lly)
}

type rectList []model.PdfRectangle

func (rl rectList) String() string {
	parts := make([]string, len(rl))
	for i, r := range rl {
		parts[i] = fmt.Sprintf("\t%3d: %s", i, showBBox(r))
	}
	return fmt.Sprintf("{RECTLIST: %d elements=[\n%s]", len(rl), strings.Join(parts, "\n"))
}

func (rl rectList) checkOverlaps() {
	if len(rl) == 0 {
		return
	}
	r0 := rl[0]
	for _, r := range rl[1:] {
		if r.Llx < r0.Urx {
			panic(fmt.Errorf("checkOverlaps:\n\tr0=%s\n\t r=%s", showBBox(r0), showBBox(r)))
		}
		r0 = r
	}
}

func checkOverlaps(rl []idRect) {
	if len(rl) == 0 {
		return
	}
	r0 := rl[0]
	for _, r := range rl[1:] {
		if r.Llx < r0.Urx {
			panic(fmt.Errorf("checkOverlaps:\n\tr0=%s\n\t r=%s", r0, r))
		}
		r0 = r
	}
}

func (rl rectList) union() model.PdfRectangle {
	var u model.PdfRectangle
	if len(rl) == 0 {
		return u
	}
	u = rl[0]
	for _, r := range rl[1:] {
		u = rectUnion(u, r)
	}
	return u
}

// intersects returns the elements of `rl` that intersect `bound`.
func (rl rectList) intersects(bound model.PdfRectangle) rectList {
	if len(rl) == 0 || !validBBox(bound) {
		panic("intersects n==0")
		return nil
	}

	var intersecting rectList
	for _, r := range rl {
		if !validBBox(r) {
			continue
		}
		if intersects(bound, r) {
			intersecting = append(intersecting, r)
		}
	}
	return intersecting
}

// intersectionSignificant returns true if `bound` has a significant (> maxoverlap) fractional
// intersection with any rectangle in `cover`.
func intersectionSignificant(bound model.PdfRectangle, cover rectList, maxoverlap float64) bool {
	if len(cover) == 0 || maxoverlap == 1.0 {
		return false
	}
	overlap := -1.0
	besti := -1
	for i, r := range cover {
		olap := intersectionFraction(r, bound)
		if olap > overlap {
			overlap = olap
			besti = i
		}
	}
	common.Log.Info("bestOverlap: overlap=%.3f bound=%.1f cover[%d]=%.1f",
		overlap, bound, besti, cover[besti])

	for _, r := range cover {
		frac := intersectionFraction(r, bound)
		// common.Log.Info("%d of %d intersectionFraction(%s, %s)=%g maxoverlap=%g", i, len(cover),
		// 	showBBox(r), showBBox(bound), frac, maxoverlap)
		if frac > maxoverlap {
			return true
		}
	}
	return false
}

// intersectionFraction the ratio of area of intersecton of `r0` and `r1` and the area of `r1`.
func intersectionFraction(r0, r1 model.PdfRectangle) float64 {
	if !(validBBox(r0) && validBBox(r1)) {
		common.Log.Error("boxes not both valid r0=%+r r1=%+r", r0, r1)
		return 0
	}
	r, overl := geometricIntersection(r0, r1)
	if !overl {
		return 0
	}
	return bboxArea(r) / bboxArea(r1)
}

// geometricIntersection returns a rectangle that is the geomteric intersection of `r0` and `r1`.
func geometricIntersection(r0, r1 model.PdfRectangle) (model.PdfRectangle, bool) {
	if !intersects(r0, r1) {
		return model.PdfRectangle{}, false
	}
	return model.PdfRectangle{
		Llx: math.Max(r0.Llx, r1.Llx),
		Urx: math.Min(r0.Urx, r1.Urx),
		Lly: math.Max(r0.Lly, r1.Lly),
		Ury: math.Min(r0.Ury, r1.Ury),
	}, true
}

// horizontalIntersection returns a rectangle that is the horizontal intersection and vertical union
// of `r0` and `r1`.
func horizontalIntersection(r0, r1 model.PdfRectangle) model.PdfRectangle {
	r := model.PdfRectangle{
		Llx: math.Max(r0.Llx, r1.Llx),
		Urx: math.Min(r0.Urx, r1.Urx),
		Lly: math.Min(r0.Lly, r1.Lly),
		Ury: math.Max(r0.Ury, r1.Ury),
	}
	if r.Llx >= r.Urx || r.Lly >= r.Ury {
		return model.PdfRectangle{}
	}
	return r
}

func intersects(r0, r1 model.PdfRectangle) bool {
	return r0.Urx > r1.Llx && r1.Urx > r0.Llx && r0.Ury > r1.Lly && r1.Ury > r0.Lly
}

func validBBox(r model.PdfRectangle) bool {
	return r.Llx <= r.Urx && r.Lly <= r.Ury
}

func showBBox(r model.PdfRectangle) string {
	w := r.Urx - r.Llx
	h := r.Ury - r.Lly
	return fmt.Sprintf("%5.1f %5.1fx%5.1f", r, w, h)
	// return fmt.Sprintf("%5.1f %5.1fx%5.1f=%5.1f", r, w, h, partEltQuality(r))
}

func same(x0, x1 float64) bool {
	const TOL = 0.1
	return math.Abs(x0-x1) < TOL
}

func sliceIntersection(slc0, slc1 []int) []int {
	m := map[int]struct{}{}
	for _, i := range slc1 {
		m[i] = struct{}{}
	}
	var isect []int
	for _, i := range slc0 {
		if _, ok := m[i]; ok {
			isect = append(isect, i)
		}
	}
	return isect
}
