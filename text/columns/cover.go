package main

import (
	"fmt"
	"sort"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

func boundedObstacles(pageBound model.PdfRectangle, words []extractor.TextMarkArray) (
	model.PdfRectangle, rectList) {
	if len(words) == 0 {
		panic("boundedObstacles: no words")
	}
	obstacles := wordBBoxes(words)
	sigObstacles = wordBBoxMap(words)
	bound := pageBound
	{
		envelope := obstacles.union()
		for _, r := range obstacles {
			if r.Llx < envelope.Llx {
				panic("A) llx")
			}
			if r.Urx > envelope.Urx {
				panic("B) urx")
			}
		}

		contraction, _ := geometricIntersection(bound, envelope)
		common.Log.Debug("contraction\n\t   bound=%s\n\tenvelope=%s\n\tcontract=%s",
			showBBox(bound), showBBox(envelope), showBBox(contraction))
		bound = contraction

		var visible rectList
		for _, r := range obstacles {
			v, ok := geometricIntersection(bound, r)
			if ok {
				visible = append(visible, v)
			}
		}
		obstacles = visible
	}

	return bound, obstacles
}

var sigObstacles map[float64]extractor.TextMarkArray

// removeUnseparated removes adjacent gaps (elements of `cover`) which have no intervening text.
// It removes shorter gaps first.
func removeUnseparated(bound model.PdfRectangle, cover, obstacles rectList) rectList {
	common.Log.Info("removeUnseparated: cover=%d obstacles=%d", len(cover), len(obstacles))
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
	common.Log.Info("removeUnseparated: %d -> %d", len(cover), len(reduced))
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
	expansionL := r
	expansionL.Llx -= width
	expansionR := r
	expansionR.Urx += width
	dyL := yRange(wordCount(expansionL, obstacles))
	dyR := yRange(wordCount(expansionR, obstacles))
	return dyL > width && dyR > width
}

func partEltSig(r model.PdfRectangle) float64 {
	return r.Llx + r.Urx*1e3 + r.Lly*1e6 + r.Ury*1e9
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
		return -1.0
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
