package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

func overlappingGaps(div0, div1 division, gapWidth float64) (closed, continued, opened division) {
	// div0.validate()
	div1.validate()
	elements0 := map[int]struct{}{}
	elements1 := map[int]struct{}{}
	for i0, r0 := range div0.gaps {
		for i1, r1 := range div1.gaps {
			r := horizontalIntersection(r0, r1)
			sign := "≤"
			if r.Width() > gapWidth {
				sign = ">"
			}
			if r.Urx != 0 && r.Ury != 0 && r.Llx != 0 && r.Lly != 0 {
				common.Log.Info("%5.1f + %5.1f -> %5.1f  (%d %d) %.2f %s %.2f ",
					r0, r1, r, i0, i1, r.Width(), sign, gapWidth)
			}
			if r.Width() > gapWidth {
				elements0[i0] = struct{}{}
				elements1[i1] = struct{}{}
				continued.gaps = append(continued.gaps, r)
				// continued.validate()
			}
		}
	}
	for i, r := range div0.gaps {
		if _, ok := elements0[i]; !ok {
			closed.gaps = append(closed.gaps, r)
			closed.validate()
		}
	}
	for i, r := range div1.gaps {
		if _, ok := elements1[i]; !ok {
			opened.gaps = append(opened.gaps, r)
			opened.validate()
		}
	}
	return
}

func testOverlappingGaps() {
	div0 := division{
		gaps: rectList{
			model.PdfRectangle{Lly: 10, Ury: 20, Llx: 50, Urx: 60},
			model.PdfRectangle{Lly: 10, Ury: 20, Llx: 70, Urx: 96},
			model.PdfRectangle{Lly: 10, Ury: 20, Llx: 200, Urx: 215},
		},
	}
	div1 := division{
		gaps: rectList{
			model.PdfRectangle{Lly: 20, Ury: 30, Llx: 0, Urx: 5},
			model.PdfRectangle{Lly: 20, Ury: 30, Llx: 40, Urx: 55},
			model.PdfRectangle{Lly: 20, Ury: 30, Llx: 58, Urx: 76},
			model.PdfRectangle{Lly: 20, Ury: 30, Llx: 90, Urx: 100},
		},
	}
	gapWidth := 1.0
	closed, continued, opened := overlappingGaps(div0, div1, gapWidth)
	common.Log.Info("width=%g", gapWidth)
	common.Log.Info("div0=%s", div0)
	common.Log.Info("div1=%s", div1)
	common.Log.Info("   closed=%s", closed)
	common.Log.Info("continued=%s", continued)
	common.Log.Info("   opened=%s", opened)
}

// overlappedXElements returns the elements of `gaps` that overlap `col` on the x-axis.
func overlappedXElements(col idRect, gaps []idRect) []idRect {
	var olap []idRect
	for _, g := range gaps {
		if overlappedX(col.PdfRectangle, g.PdfRectangle) {
			olap = append(olap, g)
		}
	}
	return olap
}

func sortUryLlx(gaps []idRect) {
	sort.Slice(gaps, func(i, j int) bool {
		gi, gj := gaps[i], gaps[j]
		yi, yj := gi.Ury, gj.Ury
		if yi != yj {
			return yi > yj
		}
		xi, xj := gi.Llx, gj.Llx
		if xi != xj {
			return xi < xj
		}
		return gi.Urx > gj.Urx
	})
}

func splitXIntersection(columns, gaps []idRect) (spectators, players []idRect) {
	common.Log.Info("splitXIntersection: gaps=%v -----------", gaps)
	sortUryLlx(gaps)
	common.Log.Info("                  : gaps=%v -----------", gaps)
	for i, c := range columns {
		if len(overlappedXElements(c, gaps)) == 0 {
			common.Log.Info("! %4d: c=%s", i, c)
			spectators = append(spectators, c)
		} else {
			common.Log.Info("~ %4d: c=%s", i, c)
			players = append(players, c)
		}
	}
	return
}

// division is a representation of the gaps in a group of lines.
// !@#$ Rename to layout
type division struct {
	widest float64
	gaps   rectList
	text   string
}

func (d division) validate() {
	for i, gap := range d.gaps {
		for j := i + 1; j < len(d.gaps); j++ {
			jap := d.gaps[j]
			if overlappedX(gap, jap) {
				panic(fmt.Errorf("validate (%d %d) %.1f %.1f", i, j, gap, jap))
			}
		}
	}
}

func (d division) String() string {
	n := len(d.text)
	// if n > 50 {
	// 	n = 50
	// }
	return fmt.Sprintf("%5.2f {%.1f `%s`}", d.widest, d.gaps, d.text[:n])
}

// areaOverlap returns a measure of the difference between areas of `bbox1` and `bbox2` individually
// and that of the union of the two.
func areaOverlap(bbox1, bbox2 model.PdfRectangle) float64 {
	return calcOverlap(bbox1, bbox2, bboxArea)
}

// lineOverlap returns the vertical overlap of `bbox1` and `bbox2`.
// a-b is the difference in width of the boxes as they are on
//	overlap=0: boxes are touching
//	overlap<0: boxes are overlapping
//	overlap>0: boxes are separated
func lineOverlap(bbox1, bbox2 model.PdfRectangle) float64 {
	return calcOverlap(bbox1, bbox2, bboxHeight)
}

// columnOverlap returns the horizontal overlap of `bbox1` and `bbox2`.
//	overlap=0: boxes are touching
//	overlap<0: boxes are overlapping
//	overlap>0: boxes are separated
func columnOverlap(bbox1, bbox2 model.PdfRectangle) float64 {
	return calcOverlap(bbox1, bbox2, bboxWidth)
}

// calcOverlap returns the horizontal overlap of `bbox1` and `bbox2` for metric `metric`.
//	overlap=0: boxes are touching
//	overlap<0: boxes are overlapping
//	overlap>0: boxes are separated
func calcOverlap(bbox1, bbox2 model.PdfRectangle, metric func(model.PdfRectangle) float64) float64 {
	a := metric(rectUnion(bbox1, bbox2))
	b := metric(bbox1) + metric(bbox2)
	return (a - b) / (a + b)
}

// rectUnion returns the union of rectilinear rectangles `b1` and `b2`.
func rectUnion(b1, b2 model.PdfRectangle) model.PdfRectangle {
	return model.PdfRectangle{
		Llx: math.Min(b1.Llx, b2.Llx),
		Lly: math.Min(b1.Lly, b2.Lly),
		Urx: math.Max(b1.Urx, b2.Urx),
		Ury: math.Max(b1.Ury, b2.Ury),
	}
}
