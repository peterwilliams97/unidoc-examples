package main

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// whitespaceCover returns a best-effort maximum rectangle cover of the part of `pageBound` that
// excludes the bounding boxes of `textMarks`
func whitespaceCover(pageBound model.PdfRectangle, words []extractor.TextMarkArray) (
	model.PdfRectangle, rectList, rectList) {
	maxboxes := 20
	maxoverlap := 0.01
	maxperim := pageBound.Width() + pageBound.Height()*0.05
	frac := 0.01
	maxpops := 20000

	obstacles := wordBBoxes(words)
	sigObstacles = wordBBoxMap(words)
	bound := pageBound
	{
		envelope := obstacles.union()
		contraction, _ := geometricIntersection(bound, envelope)
		// contraction.Llx += 100
		// contraction.Urx -= 100
		common.Log.Info("contraction\n\t   bound=%s\n\tenvelope=%s\n\tcontract=%s",
			showBBox(bound), showBBox(envelope), showBBox(contraction))
		bound = contraction
	}
	cover := obstacleCover(bound, obstacles, maxboxes, maxoverlap, maxperim, frac, maxpops)
	return bound, obstacles, cover
}

var sigObstacles map[float64]extractor.TextMarkArray

// obstacleCover returns a best-effort maximum rectangle cover of the part of `bound` that
// excludes `obstacles`.
// Based on "Two Geometric Algorithms for Layout Analysis" by Thomas Breuel
// https://www.researchgate.net/publication/2504221_Two_Geometric_Algorithms_for_Layout_Analysis
func obstacleCover(bound model.PdfRectangle, obstacles rectList,
	maxboxes int, maxoverlap, maxperim, frac float64, maxpops int) rectList {
	common.Log.Info("whitespaceCover: bound=%5.1f obstacles=%d maxboxes=%d\n"+
		"\tmaxoverlap=%g maxperim=%g frac=%g maxpops=%d",
		bound, len(obstacles), maxboxes,
		maxoverlap, maxperim, frac, maxpops)
	if len(obstacles) == 0 {
		return nil
	}
	W = bound.Width()
	H = bound.Height()
	pq := newPriorityQueue()
	partel := newPartElt(bound, obstacles)
	pq.myPush(partel)
	var cover rectList

	var tos rectList
	var tosP []partElt

	// var snaps []string
	for cnt := 0; pq.Len() > 0; cnt++ {
		partel := pq.myPop()
		common.Log.Info("npush=%3d npop=%3d cover=%3d cnt=%3d\n\tpartel=%s\n\t    pq=%s",
			pq.npush, pq.npop, len(cover), cnt, partel.String(), pq.String())

		tos = append(tos, partel.bound)
		tosP = append(tosP, *partel)

		if cnt > 100000 {
			panic("cnt")
		}
		// snaps = append(snaps, pq.String())

		if pq.npop > maxpops {
			common.Log.Info("npop > maxpops npop=%d maxpops=%d", pq.npop, maxpops)
			break
		}

		// Extract the contents

		// Got an empty rectangle?
		if len(partel.obstacles) == 0 {
			common.Log.Info("EMPTY: partel=%s cover=%d", partel, len(cover))
			if !intersectionSignificant(partel.bound, cover, maxoverlap) {
				partel = partel.extend(bound, obstacles)
				cover = append(cover, partel.bound)
				common.Log.Info("ADDING cover=%d bound=%5.1f", len(cover), partel.bound)
			}
			if len(cover) >= maxboxes { // we're done
				break
			}
			continue
		}

		// Generate up to 4 subdivisions and put them on the heap
		subdivisions := subdivide(partel.bound, append(partel.obstacles, cover...), maxperim, frac)
		for _, subbound := range subdivisions {
			subobstacles := partel.obstacles.intersects(subbound)
			partel := newPartElt(subbound, subobstacles)
			if !accept(partel.bound) {
				continue
			}
			pq.myPush(partel)
		}
	}

	n := len(tos)
	if n > 30 {
		n = 30
	}
	saveParams.markups[saveParams.curPage]["marks"] = tos[:n]
	common.Log.Info("tos=%d", len(tosP))
	for i, r := range tosP {
		// fmt.Printf("%4d: %s %5.3f\n", i, showBBox(r), partEltQuality(r))
		fmt.Printf("%4d: %s\n", i, r.String())
	}

	// common.Log.Info("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	// for i, s := range snaps {
	// 	fmt.Printf("%6d: %s\n", i, s)
	// }
	// cover = removeNonSeparating(bound, cover, obstacles) !@#$
	cover = absorbCover(bound, cover, obstacles)
	return cover
}

// absorbCover removes adjacent gaps (elements of `cover`) which have no intervening text.
// It removes shorter gaps first.
func absorbCover(bound model.PdfRectangle, cover, obstacles rectList) rectList {
	byHeight := make([]int, len(cover))
	for i := 0; i < len(byHeight); i++ {
		byHeight[i] = i
	}
	sort.SliceStable(cover, func(i, j int) bool {
		oi, oj := cover[i], cover[j]
		xi, xj := oi.Llx, oj.Llx
		if xi != xj {
			return xi < xj
		}
		return oi.Lly > oj.Lly
	})
	sort.Slice(byHeight, func(i, j int) bool {
		oi, oj := cover[byHeight[i]], cover[byHeight[j]]
		hi, hj := oi.Height(), oj.Height()
		if hi != hj {
			return hi < hj
		}
		wi, wj := oi.Width(), oj.Width()
		if wi != wj {
			return wi < wj
		}
		return i < j
	})
	// common.Log.Info("byHeight=%v", byHeight)
	// common.Log.Info("cover-------------")
	// for i, r := range cover {
	// 	fmt.Printf("%3d: %s\n", i, showBBox(r))
	// }
	// common.Log.Info("byHeight-------------")
	// for i, i0 := range byHeight {
	// 	r := cover[i0]
	// 	fmt.Printf("%3d: %3d: %s\n", i, i0, showBBox(r))
	// }

	absorbed := map[int]struct{}{}
	for i := range cover {
		if absorbedBy(cover, obstacles, i, absorbed) {
			absorbed[i] = struct{}{}
		}
	}

	var reduced rectList
	for i, i0 := range byHeight {
		r := cover[i0]
		_, ok := absorbed[i0]
		if !ok {
			reduced = append(reduced, r)
		}
		fmt.Printf("%3d: %3d: %s %t\n", i, i0, showBBox(r), ok)
	}
	common.Log.Info("absorbCover: %d -> %d", len(cover), len(reduced))
	return reduced
}

// absorbedBy returns true if `cover`[`i0`] has no intervening `obstacles` with at least one other
// element of `cover`. `absorbed` are the indexes of previously removed elements of cover.
func absorbedBy(cover, obstacles rectList, i0 int, absorbed map[int]struct{}) bool {
	r0 := cover[i0]

	for i := i0 + 1; i < len(cover); i++ {
		if _, ok := absorbed[i]; ok {
			continue
		}
		r := cover[i]
		if r.Lly <= r0.Lly && r.Ury >= r0.Ury {
			v := r0
			v.Urx = r.Llx
			v.Ury -= 2 // To exclude tiny overlaps
			v.Lly += 2 // To exclude tiny overlaps
			overl := wordCount(v, obstacles)
			if len(overl) == 0 {
				common.Log.Info("-absorbed v=%s\n\t%s %d by\n\t%s %d",
					showBBox(v), showBBox(r0), i0, showBBox(r), i)
				return true
			}
		}
	}
	for i := i0 - 1; i >= 0; i-- {
		if _, ok := absorbed[i]; ok {
			continue
		}
		r := cover[i]
		if r.Lly <= r0.Lly && r.Ury >= r0.Ury {
			v := r0
			v.Llx = r.Urx
			v.Ury -= 2 // To exclude tiny overlaps
			v.Lly += 2 // To exclude tiny overlaps
			overl := wordCount(v, obstacles)
			if len(overl) == 0 {
				common.Log.Info("+absorbed v=%s\n\t%s %d by\n\t%s %d",
					showBBox(v), showBBox(r0), i0, showBBox(r), i)
				return true
			}
		}
	}
	return false
}

const searchWidth = 60

// removeNonSeparating returns `cover` stripped of elements that don't separate elements of `obstacles`.
func removeNonSeparating(bound model.PdfRectangle, cover, obstacles rectList) rectList {
	reduced := make(rectList, 0, len(cover))
	for _, r := range cover {
		if separatingRect(r, searchWidth, obstacles) {
			reduced = append(reduced, r)
		}
	}
	common.Log.Info("removeNonSeparating: %d -> %d", len(cover), len(reduced))
	return reduced
}

func removeEmpty(bound model.PdfRectangle, cover, obstacles rectList) rectList {
	reduced := make(rectList, 0, len(cover))
	for i, r := range cover {
		olap := wordCount(r, obstacles)
		common.Log.Info(":: %4d: %s %3d", i, showBBox(r), len(olap))
		if len(olap) > 0 {
			reduced = append(reduced, r)
		}
	}
	common.Log.Info("removeEmpty: %d -> %d", len(cover), len(reduced))
	return reduced
}

// separatingRect returns true if `r` separates sufficient elements of `obstacles` (bounding boxes
// of words). We search `width` to left and right of `r` for these elements.
func separatingRect(r model.PdfRectangle, width float64, obstacles rectList) bool {
	expansion := r
	expansion.Llx -= width
	expansion.Urx += width
	overl := wordCount(expansion, obstacles)
	// words := bboxWords(sigObstacles, obstacles)
	words := bboxWords(sigObstacles, overl)
	var texts []string
	for _, w := range words {
		texts = append(texts, w.Text())
	}
	dy := yRange(overl)
	common.Log.Info("r=%s dy=%.1f count=%d %v", showBBox(r), dy, len(overl), texts)
	return len(overl) > 0 && dy > width
}

func accept(bound model.PdfRectangle) bool {
	// return math.Max(bound.Height(), bound.Width()) > 40.0
	if bound.Height() > 30.0 && bound.Width() > 10.0 {
		return true
	}
	if bound.Height() > 5.0 && bound.Width() > 50.0 {
		return true
	}
	return false
}

var H, W float64

func partEltQuality(r model.PdfRectangle) float64 {
	x := (0.1*r.Height()/H + r.Width()/W) / 1.1
	y := (r.Height()/H + 0.1*r.Width()/W) / 1.1
	return math.Max(0.01*x, y)
}

func partEltSig(r model.PdfRectangle) float64 {
	return r.Llx + r.Urx*1e3 + r.Lly*1e6 + r.Ury*1e9
}

// subdivide subdivides `bound` in to up to 4 rectangles that don't intersect with `obstacles`.
func subdivide(bound model.PdfRectangle, obstacles rectList, maxperim, frac float64) rectList {
	subdivisions := make(rectList, 0, 4)
	pivot, err := selectPivot(bound, obstacles, maxperim, frac)
	if err != nil {
		panic(err)
	}
	if !validBBox(pivot) {
		panic(fmt.Errorf("bad pivot=%s bound=%s obstacles=%d",
			showBBox(pivot), showBBox(bound), len(obstacles)))
	}

	pivotInt, ok := geometricIntersection(bound, pivot)
	if !ok {
		return nil
	}
	pivot = pivotInt

	var parts []string
	if pivot.Llx > bound.Llx && !same(bound.Urx, pivot.Llx) { // left sub-bound
		quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: bound.Lly, Urx: pivot.Llx, Ury: bound.Ury}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "left")
	} else {
		u := obstacles.union()
		if bound.Llx < u.Llx {
			quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: bound.Lly, Urx: u.Llx, Ury: bound.Ury}
			subdivisions = append(subdivisions, quadrant)
			parts = append(parts, "left*")
		}
	}
	if pivot.Urx < bound.Urx && !same(bound.Llx, pivot.Urx) { // right sub-bound
		quadrant := model.PdfRectangle{Llx: pivot.Urx, Lly: bound.Lly, Urx: bound.Urx, Ury: bound.Ury}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "right")
	} else {
		u := obstacles.union()
		if bound.Urx > u.Urx {
			quadrant := model.PdfRectangle{Llx: u.Urx, Lly: bound.Lly, Urx: bound.Urx, Ury: bound.Ury}
			subdivisions = append(subdivisions, quadrant)
			parts = append(parts, "right*")
		}
	}
	if pivot.Ury < bound.Ury && !same(bound.Lly, pivot.Ury) { // top sub-bound
		quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: pivot.Ury, Urx: bound.Urx, Ury: bound.Ury}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "top")
	}
	if pivot.Lly > bound.Lly && !same(bound.Ury, pivot.Lly) { // bottom sub-bound
		quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: bound.Lly, Urx: bound.Urx, Ury: pivot.Lly}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "bottom")
	}

	common.Log.Info("subdivide parts=%s\n\tbound=%s\n\tpivot=%5.1f -->",
		parts, showBBox(bound), pivot)
	for i, quadrant := range subdivisions {
		fmt.Printf("\t%5d=%s\n", i, showBBox(quadrant))
		if !validBBox(quadrant) {
			panic("quadrant")
		}
	}
	return subdivisions
}

// selectPivot returns an element of `obstacles` close to the center of `bound`.
func selectPivot(bound model.PdfRectangle, obstacles rectList, maxperim, frac float64) (
	model.PdfRectangle, error) {
	if !validBBox(bound) {
		panic(fmt.Errorf("selectPivot: bound=%s", showBBox(bound)))
	}
	if len(obstacles) == 0 {
		return model.PdfRectangle{}, fmt.Errorf("no boxes in obstacles")
	}
	if frac < 0.0 || frac > 1.0 {
		common.Log.Error("frac=%g out of bound; using 0.0", frac)
		frac = 0.0
	}

	w := bound.Width()
	h := bound.Height()
	x, y := bboxCenter(bound)
	threshdist := frac * math.Sqrt(w*w+h*h)
	mindist := 1000000000.0
	minindex := 0
	smallfound := false
	for i, r := range obstacles {
		if bboxPerim(r) > maxperim {
			continue
		}
		smallfound = true

		cx, cy := bboxCenter(r)
		delx := cx - x
		dely := cy - y
		dist := delx*delx + dely*dely
		if dist <= threshdist {
			return r, nil
		}
		if dist < mindist {
			minindex = i
			mindist = dist
		}
	}

	// If there are small boxes but none are within 'frac' of the centroid, return the nearest one.
	if smallfound {
		return obstacles[minindex], nil
	}

	// No small boxes; return the smallest of the large boxes
	minsize := 1000000000.0
	minindex = 0
	for i, r := range obstacles {
		perim := bboxPerim(r)
		if perim < minsize {
			minsize = perim
			minindex = i
		}
	}
	return obstacles[minindex], nil
}

func newPartElt(bound model.PdfRectangle, obstacles rectList) *partElt {
	if !validBBox(bound) {
		panic(fmt.Errorf("bound=%s", showBBox(bound)))
	}
	for i, r := range obstacles {
		if !validBBox(r) {
			panic(fmt.Errorf("obstacles[%d]=%s", i, showBBox(r)))
		}
	}
	return &partElt{
		bound:     bound,
		obstacles: obstacles,
		quality:   partEltQuality(bound),
		sig:       partEltSig(bound),
	}
}

type partElt struct {
	quality   float64 // sorting key
	sig       float64
	bound     model.PdfRectangle // region of the element
	obstacles rectList           // set of intersecting boxes
}

func (partel *partElt) extend(bound model.PdfRectangle, obstacles rectList) *partElt {
	if len(partel.obstacles) != 0 {
		panic(fmt.Errorf("not empty: %s", partel))
	}
	bnd := partel.bound

	common.Log.Info("extend bound=%s", showBBox(bnd))

	w := bnd.Width() / 4
	bnd.Llx += 2 * w
	bnd.Urx -= w

	bnd.Ury = bound.Ury
	obs := obstacles.intersects(bnd)
	if len(obs) > 0 {
		bnd.Ury = obs.union().Lly
		// words := bboxWords(sigObstacles, obs)
		// common.Log.Info("Upward extension %d bnd=%s", len(words), showBBox(bnd))
		// for i, w := range words {
		// 	b, _ := w.BBox()
		// 	fmt.Printf("%4d: %5.1f %s\n", i, b, w.Text())
		// }
	}

	bnd.Lly = bound.Lly
	obs = obstacles.intersects(bnd)
	if len(obs) > 0 {
		bnd.Lly = obs.union().Ury
		// words := bboxWords(sigObstacles, obs)
		// common.Log.Info("Downward extension %d bnd=%s", len(words), showBBox(bnd))
		// for i, w := range words {
		// 	b, _ := w.BBox()
		// 	fmt.Printf("%4d: %5.1f %s\n", i, b, w.Text())
		// }
	}

	// bnd.Urx = bound.Urx
	// obs = obstacles.intersects(bnd)
	// if len(obs) > 0 {
	// 	bnd.Urx = obs.union().Llx
	// }

	// bnd.Llx = bound.Llx
	// obs = obstacles.intersects(bnd)
	// if len(obs) > 0 {
	// 	bnd.Llx = obs.union().Urx
	// }

	pe := newPartElt(bnd, obstacles.intersects(bnd))
	common.Log.Info("extend:\n\t%s->\n\t%s", partel, pe)
	return pe
}

func (partel *partElt) String() string {
	extra := ">"
	if len(partel.obstacles) == 0 {
		extra = " |LEAF!>"
	}
	return fmt.Sprintf("<%s %5.3f %3d%s",
		showBBox(partel.bound), partel.quality, len(partel.obstacles), extra)
}

// newPriorityQueue returns a PriorityQueue containing `items`.
func newPriorityQueue() *PriorityQueue {
	var pq PriorityQueue
	heap.Init(&pq)
	return &pq
}

// PriorityQueue implements heap.Interface and holds partElt.
type PriorityQueue struct {
	npop  int
	npush int
	elems []*partElt
}

func (pq *PriorityQueue) String() string {
	parts := make([]string, 0, len(pq.elems))
	var lines []string
	var save []*partElt
	for pq.Len() > 0 {
		pe := pq._myPop()
		save = append(save, pe)
		leaf := " "
		if len(pe.obstacles) == 0 && len(lines) < 5 {
			leaf = " LEAF!"
			lines = append(lines, fmt.Sprintf("\n\t%5.1f %5.1f", pe.quality, pe.bound))
		}
		if len(parts) >= 5 && leaf != " LEAF!" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%.1f %d%s", pe.quality, len(pe.obstacles), leaf))
	}
	for _, pe := range save {
		pq._myPush(pe)
	}
	return fmt.Sprintf("{PQ %d: %s}", pq.Len(), strings.Join(parts, ", "))
}

func (pq PriorityQueue) Len() int { return len(pq.elems) }

func (pq PriorityQueue) Less(i, j int) bool { return pq.elems[i].quality > pq.elems[j].quality }

func (pq PriorityQueue) Swap(i, j int) { pq.elems[i], pq.elems[j] = pq.elems[j], pq.elems[i] }

func (pq *PriorityQueue) Push(x interface{}) {
	partel := x.(*partElt)
	pq.elems = append(pq.elems, partel)
}

func (pq *PriorityQueue) myPush(partel *partElt) {
	for _, pe := range pq.elems {
		if pe.sig == partel.sig {
			err := fmt.Errorf("duplicate:\n\tpartel=%s\n\t    pe=%s", partel, pe)
			common.Log.Error("myPush %v", err)
			return
		}
	}
	pq.npush++
	pq._myPush(partel)
}

func (pq *PriorityQueue) _myPush(partel *partElt) {
	heap.Push(pq, partel)
}

func (pq *PriorityQueue) myPop() *partElt {
	pq.npop++
	return pq._myPop()
}

func (pq *PriorityQueue) _myPop() *partElt {
	return heap.Pop(pq).(*partElt)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := pq.elems
	n := len(old)
	partel := old[n-1]
	old[n-1] = nil // avoid memory leak
	pq.elems = old[:n-1]
	return partel
}

func wordCount(bound model.PdfRectangle, obstacles rectList) rectList {
	overl := make(rectList, 0, len(obstacles))
	for _, r := range obstacles {
		if intersects(bound, r) {
			overl = append(overl, r)
		}
	}
	return overl
}

func yRange(obstacles rectList) float64 {
	if len(obstacles) == 0 {
		return 0
	}

	min := obstacles[0].Lly
	max := obstacles[0].Lly
	for _, r := range obstacles[1:] {
		if r.Lly < min {
			min = r.Lly
		}
		if r.Lly > max {
			r.Lly = max
		}
	}
	return max - min
}
