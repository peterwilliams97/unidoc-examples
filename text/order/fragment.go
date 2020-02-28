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

// fragmentPage scans the page vertically in ranges of `window` and looks for gaps in each scan line
// returns the scan line in chunks separated by >= gapSize.
func fragmentPage(pageBound model.PdfRectangle, pageWords rectList, gapSize float64) rectList {
	if len(pageWords) == 0 {
		return rectList{pageBound}
	}
	for _, r := range pageWords {
		if r.Llx < pageBound.Llx {
			panic("A) llx")
		}
		if r.Urx > pageBound.Urx {
			panic("B) urx")
		}
	}
	ss := newFragmentState(pageBound, pageWords)
	pageGaps := ss.scan()
	var wideGaps rectList
	for _, gap := range pageGaps {
		if gap.Width() >= 10.0 {
			wideGaps = append(wideGaps, gap)
		}
	}
	return wideGaps
	// slines := ss.wordsToFragmentLines(pageWords)
	// common.Log.Info("@@ fragmentPage: pageBound=%s", showBBox(pageBound))
	// ss.validate()
	// var words []idRect
	// for i, sl := range slines {
	// 	common.Log.Info("%2d **********  sl=%s", i, sl.String())
	// 	common.Log.Info("ss=%s", ss.String())
	// 	words = sl.updateWords(words)
	// 	columns := pokeHoles(ss.pageBound, words, sl.y)
	// 	common.Log.Info("%2d #########  columns=%d", i, len(columns))
	// 	for j, r := range columns {
	// 		fmt.Printf("%4d: %s\n", j, showBBox(r))
	// 	}
	// 	ss.extendColumns(columns, sl.y)
	// 	common.Log.Info("ss=%s", ss.String())
	// 	ss.validate()
	// }
	// // Close all the running columns.
	// common.Log.Info("FINAL CLOSER")
	// for i, idr := range ss.running {
	// 	common.Log.Info("running[%d]=%s", i, idr)
	// 	idr.Lly = pageBound.Lly
	// 	if idr.Ury-idr.Lly > 0 {
	// 		common.Log.Info("%4d completed[%d]=%s", i, len(ss.completed), idr)
	// 		idr.validate()
	// 		ss.completed = append(ss.completed, idr)
	// 		ss.validate()
	// 	}
	// }

	// sort.Slice(ss.completed, func(i, j int) bool {
	// 	return ss.completed[i].id < ss.completed[j].id
	// })
	// common.Log.Info("fragmentPage: pageWords=%d pageBound=%5.1f", len(pageWords), pageBound)
	// for i, c := range pageWords {
	// 	fmt.Printf("%4d: %5.1f\n", i, c)
	// }
	// common.Log.Info("fragmentPage: completed=%d", len(ss.completed))
	// for i, c := range ss.completed {
	// 	fmt.Printf("%4d: %s\n", i, c)
	// }
	// columns := make(rectList, len(ss.completed))
	// for i, c := range ss.completed {
	// 	columns[i] = c.PdfRectangle
	// }
	// common.Log.Info("fragmentPage: columns=%d", len(columns))
	// for i, c := range columns {
	// 	fmt.Printf("%4d: %5.1f\n", i, c)
	// }
	// return columns
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
	// r := model.PdfRectangle{Llx: pageBound.Llx, Urx: pageBound.Urx, Ury: pageBound.Ury}
	// idr := ss.newIDRect(r)
	// ss.running = append(ss.running, idr)

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

// fragmentEvent represents leaving or entering a rectangle while fragmentning down a page.
type fragmentEvent struct {
	idRect
	enter bool // true if entering, false if leaving `idRect`.
}

// func (ss *fragmentState) newIDRect(r model.PdfRectangle) idRect {
// 	id := len(ss.store) + 1
// 	idr := idRect{id: id, PdfRectangle: r}
// 	idr.validate()
// 	ss.store[id] = idr
// 	return idr
// }

// func (ss *fragmentState) getIDRect(id int) idRect {
// 	idr, ok := ss.store[id]
// 	if !ok {
// 		panic(fmt.Errorf("bad id=%d", id))
// 	}
// 	return idr
// }

// pokeHoles finds the gaps in a slice of words.
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
		common.Log.Info("\tholes[%d]=%s %q e%s", len(holes), showBBox(r), whence, e)
		if !validBBox(r) {
			panic("BBox")
		}
		holes = append(holes, r)
	}

	common.Log.Info("   words=%d bound=%s", len(words), showBBox(bound))
	llx := bound.Llx
	depth := 0
	for i, e := range events {
		common.Log.Info("%3d: llx=%5.1f %s depth=%d", i, llx, e, depth)
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

	common.Log.Info("pokeHoles words=%d", len(words))
	for i, idr := range words {
		fmt.Printf("%4d: %s\n", i, idr)
	}
	common.Log.Info("pokeHoles holes=%d", len(holes))
	for i, idr := range holes {
		fmt.Printf("%4d: %s\n", i, showBBox(idr))
	}

	return holes
}

// func (ss *fragmentState) extendColumns(columns rectList, y float64) {
// 	// columns.sortHor()
// 	sortHor(ss.running, true)
// 	columns.checkXOverlaps()
// 	checkXOverlaps(ss.running)

// 	delta := 1.0
// 	contRun := map[int]struct{}{}
// 	contCol := map[int]struct{}{}
// 	for i, c := range columns {
// 		for j, r := range ss.running {
// 			if math.Abs(c.Llx-r.Llx) < delta && math.Abs(c.Urx-r.Urx) < delta {
// 				contCol[i] = struct{}{}
// 				contRun[j] = struct{}{}
// 			}
// 		}
// 	}

// 	var closed []idRect
// 	var opened []idRect
// 	var running []idRect
// 	for i, r := range columns {
// 		if _, ok := contCol[i]; !ok {
// 			opened = append(opened, ss.newIDRect(r))
// 		}
// 	}
// 	for i, idr := range ss.running {
// 		if _, ok := contRun[i]; !ok {
// 			if y < idr.Ury {
// 				idr.Lly = y
// 				closed = append(closed, idr)
// 			}
// 		} else {
// 			running = append(running, idr)
// 		}
// 	}

// 	ss.running = append(running, opened...)
// 	ss.completed = append(ss.completed, closed...)

// 	common.Log.Info("extendColumns: ss=%s", ss)
// 	sortHor(ss.running, true)
// 	checkXOverlaps(ss.running)
// 	// sortHor(ss.completed, true)
// 	// checkXOverlaps(ss.completed)
// }

// // wordsToFragmentLines creates the list of fragment lines corresponding to words `pageWords`.
// func (ss *fragmentState) wordsToFragmentLines(pageWords rectList) []fragmentLine {
// 	events := make([]fragmentEvent, 2*len(pageWords))
// 	for i, word := range pageWords {
// 		idr := ss.newIDRect(word)
// 		events[2*i] = fragmentEvent{enter: true, idRect: idr}
// 		events[2*i+1] = fragmentEvent{enter: false, idRect: idr}
// 	}
// 	sort.Slice(events, func(i, j int) bool {
// 		ei, ej := events[i], events[j]
// 		yi, yj := ei.y(), ej.y()
// 		if yi != yj {
// 			return yi > yj
// 		}
// 		if ei.enter != ej.enter {
// 			return ei.enter
// 		}
// 		return ei.Llx < ej.Llx
// 	})

// 	var slines []fragmentLine
// 	e := events[0]
// 	sl := fragmentLine{y: e.y(), events: []fragmentEvent{e}}
// 	sl.checkXOverlaps()
// 	common.Log.Info("! %2d of %d: %s", 1, len(events), e)
// 	for i, e := range events[1:] {
// 		common.Log.Info("! %2d of %d: %s", i+2, len(events), e)
// 		if e.y() > sl.y-1.0 {
// 			sl.events = append(sl.events, e)
// 			// sl.checkXOverlaps()
// 		} else {
// 			slines = append(slines, sl)
// 			sl = fragmentLine{y: e.y(), events: []fragmentEvent{e}}
// 		}
// 	}
// 	slines = append(slines, sl)
// 	return slines
// }

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
