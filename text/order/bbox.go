package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

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

func (rl rectList) checkXOverlaps() {
	if len(rl) == 0 {
		return
	}
	r0 := rl[0]
	for _, r := range rl[1:] {
		if r.Llx < r0.Urx {
			panic(fmt.Errorf("checkXOverlaps:\n\tr0=%s\n\t r=%s", showBBox(r0), showBBox(r)))
		}
		r0 = r
	}
}

func checkXOverlaps(rl []idRect) {
	if len(rl) == 0 {
		return
	}
	r0 := rl[0]
	for _, r := range rl[1:] {
		if r.Llx < r0.Urx {
			panic(fmt.Errorf("checkXOverlaps:\n\tr0=%s\n\t r=%s", r0, r))
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
	r := model.PdfRectangle{
		Llx: math.Max(r0.Llx, r1.Llx),
		Urx: math.Min(r0.Urx, r1.Urx),
		Lly: math.Max(r0.Lly, r1.Lly),
		Ury: math.Min(r0.Ury, r1.Ury),
	}
	ok := r.Llx < r.Urx && r.Lly < r.Ury
	return r, ok
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
	return intersectsX(r0, r1) && intersectsY(r0, r1)
}

func intersectsX(r0, r1 model.PdfRectangle) bool {
	return r0.Urx >= r1.Llx && r1.Urx >= r0.Llx
}

func intersectsY(r0, r1 model.PdfRectangle) bool {
	return r0.Ury >= r1.Lly && r1.Ury >= r0.Lly
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
