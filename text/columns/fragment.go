package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

const (
	// Sliding window size in points.
	scanWindow = 20.0

	// gapSize := charMultiplier * averageWidth(textMarks)

	charMultiplier = 1.0
)

// fragmentPage returns the gaps between `obstacles` within `bound`.
func fragmentPage(bound model.PdfRectangle, obstacles rectList) rectList {
	ss := newFragmentState(bound, obstacles)
	return ss.scan()
}

type fragmentState struct {
	words     mosaic
	pageBound model.PdfRectangle
	running   []idRect // must be sorted left to right
	completed []idRect
}

func newFragmentState(pageBound model.PdfRectangle, pageWords rectList) *fragmentState {
	ss := fragmentState{
		words:     createMosaic(pageWords),
		pageBound: pageBound,
	}
	return &ss
}

func (ss fragmentState) scan() rectList {
	numLines := int(math.Ceil(ss.pageBound.Height() / scanWindow))
	var lineGaps rectList
	for i := 0; i < numLines; i++ {
		ury := ss.pageBound.Ury - float64(i)*scanWindow
		lly := ury - scanWindow
		bound := ss.pageBound
		bound.Lly = lly
		bound.Ury = ury
		wordOrder := ss.words.intersectY(lly, ury)
		words := ss.words.getRects(wordOrder)
		gaps := pokeHoles(bound, words)
		lineGaps = append(lineGaps, gaps...)
	}
	return lineGaps
}

func (ss fragmentState) validate() {
	ss.words.validate()
	for _, idr := range ss.running {
		idr.validate()
	}
	for _, idr := range ss.completed {
		idr.validate()
	}
}

// func (ss fragmentState) String() string {
// 	var lines []string
// 	lines = append(lines, fmt.Sprintf("=== completed=%d store=%d =========",
// 		len(ss.completed), len(ss.store)))
// 	for i, c := range ss.completed {
// 		lines = append(lines, fmt.Sprintf("%4d: %s", i, c))
// 	}
// 	lines = append(lines, fmt.Sprintf("--- running=%d", len(ss.running)))
// 	for i, c := range ss.running {
// 		lines = append(lines, fmt.Sprintf("%4d: %s", i, c))
// 	}
// 	return strings.Join(lines, "\n")
// }

// fragmentLine is a list of fragment events with the same y() value.
type fragmentLine struct {
	y      float64         // e.y() ∀ e ∈ `events`.
	events []fragmentEvent // events with e.y() == `y`.
}

func (sl fragmentLine) toRectList() rectList {
	rl := make(rectList, len(sl.events))
	for i, e := range sl.events {
		rl[i] = e.PdfRectangle
	}
	return rl
}

func (sl fragmentLine) checkXOverlaps() {
	rl := sl.toRectList()
	rl.checkXOverlaps()
}

func (sl fragmentLine) String() string {
	parts := make([]string, len(sl.events))
	for i, e := range sl.events {
		parts[i] = e.String()
	}
	return fmt.Sprintf("[y=%.1f %d %s]", sl.y, len(sl.events), strings.Join(parts, " "))
}

// fragmentEvent represents leaving or entering a rectangle while scanning down a page.
type fragmentEvent struct {
	idRect
	enter bool // true if entering, false if leaving `idRect`.
}

// pokeHoles returns the gaps between `words` with bounding box `bound`.
func pokeHoles(bound model.PdfRectangle, words []idRect) rectList {
	if len(words) == 0 {
		return rectList{bound}
	}
	sortHor(words, false)
	// checkXOverlaps(words)

	events := make([]horEvent, 2*len(words))
	for i, r := range words {
		events[2*i] = horEvent{idRect: r, x: r.Llx, i: i, enter: true}
		events[2*i+1] = horEvent{idRect: r, x: r.Urx, i: i, enter: false}
		if r.Llx < bound.Llx {
			panic("1) llx")
		}
		if r.Urx > bound.Urx {
			panic("2) urx")
		}
	}

	sort.Slice(events, func(i, j int) bool {
		ei, ej := events[i], events[j]
		xi, xj := ei.x, ej.x
		if xi != xj {
			return xi < xj
		}
		return ei.i < ej.i
	})

	var holes rectList
	add := func(llx, urx float64, whence string, e horEvent) {
		if llx > urx {
			panic(fmt.Errorf("add parameters:\n\tllx=%g\n\turx=%g", llx, urx))
		}
		if llx == urx {
			return
		}
		r := model.PdfRectangle{Llx: llx, Urx: urx, Lly: bound.Lly, Ury: bound.Ury}
		common.Log.Debug("\tholes[%d]=%s %q e%s", len(holes), showBBox(r), whence, e)
		if !bboxValid(r) {
			panic("BBox")
		}
		holes = append(holes, r)
	}

	common.Log.Debug("   words=%d bound=%s", len(words), showBBox(bound))
	llx := bound.Llx
	depth := 0
	for i, e := range events {
		common.Log.Debug("%3d: llx=%5.1f %s depth=%d", i, llx, e, depth)
		if llx > bound.Urx {
			panic(fmt.Errorf("i=%d llx=%5.1f  bound=%s", i, llx, showBBox(bound)))
		}
		if e.enter {
			if depth == 0 {
				add(llx, e.x, "A", e) //  g.Llx)
			}
			depth++
		} else {
			depth--
			if depth < 0 {
				panic("depth")
			}
			if depth == 0 {
				llx = e.Urx
			}
		}
		// common.Log.Info("%3d: llx=%5.1f", i, llx)
	}
	add(llx, bound.Urx, "C", horEvent{})
	if depth != 0 {
		panic("depth end")
	}

	if common.Log.IsLogLevel(common.LogLevelDebug) {
		common.Log.Debug("pokeHoles words=%d", len(words))
		for i, idr := range words {
			fmt.Printf("%4d: %s\n", i, idr)
		}
		common.Log.Debug("pokeHoles holes=%d", len(holes))
		for i, idr := range holes {
			fmt.Printf("%4d: %s\n", i, showBBox(idr))
		}
	}

	return holes
}

func (sl fragmentLine) columnsFragment(pageBound model.PdfRectangle, enter bool) (
	opened, closed rectList) {
	addCol := func(x0, x1 float64) {
		if x1 > x0 {
			r := model.PdfRectangle{Llx: x0, Urx: x1, Ury: sl.y}
			if enter {
				opened = append(opened, r)
			} else {
				closed = append(closed, r)
			}
		}
	}
	x0 := pageBound.Llx
	for _, e := range sl.events {
		if e.enter != enter {
			continue
		}
		x1 := e.Llx
		addCol(x0, x1)
		x0 = e.Urx
	}
	x1 := pageBound.Urx
	addCol(x0, x1)
	opened.checkXOverlaps()
	closed.checkXOverlaps()
	return opened, closed
}

// updateWords returns the elements of `words` updated by the events in `sl`.
func (sl fragmentLine) updateWords(words []idRect) []idRect {
	var plus []idRect
	done := map[int]struct{}{}
	for _, e := range sl.events {
		if e.enter {
			plus = append(plus, e.idRect)
		} else {
			done[e.id] = struct{}{}
		}
	}
	var minus []idRect
	for _, idr := range plus {
		if _, ok := done[idr.id]; !ok {
			minus = append(minus, idr)
		}
	}

	// checkXOverlaps(idrs)
	return minus
}

// opening returns the elements of `sl` that are opening.
func (sl fragmentLine) opening() []idRect {
	var idrs []idRect
	for _, e := range sl.events {
		if e.enter {
			idrs = append(idrs, e.idRect)
		}
	}
	// checkXOverlaps(idrs)
	return idrs
}

// closing returns the elements of `sl` that are closing.
func (sl fragmentLine) closing() []idRect {
	var idrs []idRect
	for _, e := range sl.events {
		if !e.enter {
			idrs = append(idrs, e.idRect)
		}
	}
	// checkXOverlaps(idrs)
	return idrs
}

func (e fragmentEvent) String() string {
	dir := "leave"
	if e.enter {
		dir = "ENTER"
	}
	return fmt.Sprintf("<%5.1f %s %s>", e.y(), dir, e.idRect)
}

func (e fragmentEvent) y() float64 {
	if !e.enter {
		return e.idRect.Lly
	}
	return e.idRect.Ury
}

type horEvent struct {
	x     float64
	enter bool
	i     int
	idRect
}

func (e horEvent) String() string {
	pos := "leave"
	if e.enter {
		pos = "ENTER"
	}
	return fmt.Sprintf("<%5.1f %s %d %s>", e.x, pos, e.i, e.idRect)
}

func sortHor(rl []idRect, alreadySorted bool) {
	less := func(i, j int) bool {
		xi, xj := rl[i].Llx, rl[j].Llx
		if xi != xj {
			return xi < xj
		}
		return rl[i].Urx < rl[j].Urx
	}
	if alreadySorted {
		if !sort.SliceIsSorted(rl, less) {
			common.Log.Error("NOT SORTED")
			for i, r := range rl {
				fmt.Printf("%4d: %s\n", i, r)
			}
			panic("sortHor")
		}
	} else {
		sort.Slice(rl, less)
	}
}
