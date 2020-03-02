package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

type scanState struct {
	pageBound model.PdfRectangle
	running   []idRect // must be sorted left to right
	completed []idRect
	store     map[int]idRect
}

func (ss scanState) validate() {
	for _, idr := range ss.running {
		idr.validate()
	}
	for _, idr := range ss.completed {
		idr.validate()
	}
}

func (ss scanState) String() string {
	var lines []string
	lines = append(lines, fmt.Sprintf("=== completed=%d store=%d =========",
		len(ss.completed), len(ss.store)))
	for i, c := range ss.completed {
		lines = append(lines, fmt.Sprintf("%4d: %s", i, c))
	}
	lines = append(lines, fmt.Sprintf("--- running=%d", len(ss.running)))
	for i, c := range ss.running {
		lines = append(lines, fmt.Sprintf("%4d: %s", i, c))
	}
	return strings.Join(lines, "\n")
}

// scanLine is a list of scan events with the same y() value.
type scanLine struct {
	y      float64     // e.y() ∀ e ∈ `events`.
	events []scanEvent // events with e.y() == `y`.
}

func (sl scanLine) toRectList() rectList {
	rl := make(rectList, len(sl.events))
	for i, e := range sl.events {
		rl[i] = e.PdfRectangle
	}
	return rl
}

func (sl scanLine) checkXOverlaps() {
	rl := sl.toRectList()
	rl.checkXOverlaps()
}

func (sl scanLine) String() string {
	parts := make([]string, len(sl.events))
	for i, e := range sl.events {
		parts[i] = e.String()
	}
	return fmt.Sprintf("[y=%.1f %d %s]", sl.y, len(sl.events), strings.Join(parts, " "))
}

// scanEvent represents leaving or entering a rectangle while scanning down a page.
type scanEvent struct {
	idRect
	enter bool // true if entering, false if leaving `idRect`.
}

// scanPage returns the rectangles in `pageBound` that are separated by `pageGaps`.
func scanPage(pageBound model.PdfRectangle, pageGaps rectList) rectList {
	if len(pageGaps) == 0 {
		return rectList{pageBound}
	}
	ss := newScanState(pageBound)
	slines := ss.gapsToScanLines(pageGaps)
	common.Log.Info("@@ scanPage: pageBound=%s", showBBox(pageBound))
	ss.validate()
	var gaps []idRect
	for i, sl := range slines {
		common.Log.Info("%2d **********  sl=%s", i, sl.String())
		common.Log.Info("ss=%s", ss.String())
		if sl.y <= ss.pageBound.Lly {
			break
		}
		gaps = sl.updateGaps(gaps)
		columns := perforate(ss.pageBound, gaps, sl.y)
		common.Log.Info("%2d #########  columns=%d", i, len(columns))
		for j, r := range columns {
			fmt.Printf("%4d: %s\n", j, showBBox(r))
		}
		ss.extendColumns(columns, sl.y)
		common.Log.Info("ss=%s", ss.String())
		ss.validate()
	}
	// Close all the running columns.
	common.Log.Info("FINAL CLOSER")
	for i, idr := range ss.running {
		common.Log.Info("running[%d]=%s", i, idr)
		idr.Lly = pageBound.Lly
		if idr.Ury-idr.Lly > 0 {
			common.Log.Info("%4d completed[%d]=%s", i, len(ss.completed), idr)
			idr.validate()
			ss.completed = append(ss.completed, idr)
			ss.validate()
		}
	}

	sort.Slice(ss.completed, func(i, j int) bool {
		return ss.completed[i].id < ss.completed[j].id
	})
	common.Log.Info("scanPage: pageGaps=%d pageBound=%5.1f", len(pageGaps), pageBound)
	for i, c := range pageGaps {
		fmt.Printf("%4d: %5.1f\n", i, c)
	}
	common.Log.Info("scanPage: completed=%d", len(ss.completed))
	for i, c := range ss.completed {
		fmt.Printf("%4d: %s\n", i, c)
	}
	columns := make(rectList, len(ss.completed))
	for i, c := range ss.completed {
		columns[i] = c.PdfRectangle
	}
	common.Log.Info("scanPage: columns=%d", len(columns))
	for i, c := range columns {
		fmt.Printf("%4d: %5.1f\n", i, c)
	}
	return columns
}

func newScanState(pageBound model.PdfRectangle) *scanState {
	ss := scanState{
		pageBound: pageBound,
		store:     map[int]idRect{},
	}
	r := model.PdfRectangle{Llx: pageBound.Llx, Urx: pageBound.Urx, Ury: pageBound.Ury}
	idr := ss.newIDRect(r)
	ss.running = append(ss.running, idr)

	return &ss
}

func (ss *scanState) newIDRect(r model.PdfRectangle) idRect {
	id := len(ss.store) + 1
	idr := idRect{id: id, PdfRectangle: r}
	idr.validate()
	ss.store[id] = idr
	return idr
}

func (ss *scanState) getIDRect(id int) idRect {
	idr, ok := ss.store[id]
	if !ok {
		panic(fmt.Errorf("bad id=%d", id))
	}
	return idr
}

// perforate returns the a slice of half-open rectangles created by perforating rectangle `bound` at
// height `y` in with rectangles `gaps`. The created rectangles have top `y`, no bottom and left and
// right separated by `gaps`.
func perforate(bound model.PdfRectangle, gaps []idRect, y float64) rectList {
	if len(gaps) == 0 {
		if bound.Lly == y {
			return nil
		}
		r := bound
		r.Ury = y
		if !validBBox(r) {
			common.Log.Error("bound=%s y=%5.1f", showBBox(bound), y)
			common.Log.Error("    r=%s", showBBox(r))
			panic("BBox")
		}
		return rectList{r}
	}
	sortX(gaps, false)
	// checkXOverlaps(gaps)

	events := make([]xEvent, 2*len(gaps))
	for i, r := range gaps {
		events[2*i] = xEvent{idRect: r, x: r.Llx, i: i, enter: true}
		events[2*i+1] = xEvent{idRect: r, x: r.Urx, i: i, enter: false}
	}

	sort.Slice(events, func(i, j int) bool {
		ei, ej := events[i], events[j]
		xi, xj := ei.x, ej.x
		if xi != xj {
			return xi < xj
		}
		return ei.i < ej.i
	})

	var columns1 rectList
	add := func(llx, urx float64, whence string, e xEvent) {
		if urx > llx {
			r := model.PdfRectangle{Llx: llx, Urx: urx, Ury: y}
			common.Log.Info("\tcolumns1[%d]=%s %q %s", len(columns1), showBBox(r), whence, e)
			if !validBBox(r) {
				panic("BBox")
			}
			columns1 = append(columns1, r)
		}
	}

	common.Log.Info("<< perforate y=%.1f gaps=%d", y, len(gaps))
	// common.Log.Info("   \n\t%s", gaps)
	llx := bound.Llx
	depth := 0
	for i, e := range events {
		if e.enter {
			if depth == 0 {
				add(llx, e.x, "A", e) //  g.Llx)
			}
			depth++
		} else {
			depth--
			if depth == 0 {
				llx = e.Urx
			}
		}
		common.Log.Info("%3d: depth=%d llx=%5.1f %s", i, depth, llx, e)
		if depth < 0 {
			panic("depth")
		}
	}
	add(llx, bound.Urx, "C", xEvent{})

	common.Log.Info(">> perforate  gaps=%d", len(gaps))
	for i, idr := range gaps {
		fmt.Printf("%4d: %s\n", i, idr)
	}
	common.Log.Info("intersectingElements columns1=%d", len(columns1))
	for i, idr := range columns1 {
		fmt.Printf("%4d: %s\n", i, showBBox(idr))
	}

	return columns1
}

// extendColumns attempts to extend `ss.running` (the current open columns) downwards with `columns`
// the columns found at height `y`.
func (ss *scanState) extendColumns(columns rectList, y float64) {
	// columns.sortX()
	sortX(ss.running, true)
	columns.checkXOverlaps()
	// checkXOverlaps(ss.running)

	delta := 1.0
	contRun := map[int]struct{}{} // Indexes of ss.running that are continued.
	contCol := map[int]struct{}{} // Indexes of columns that are continuations.
	for i, c := range columns {
		for j, r := range ss.running {
			if math.Abs(c.Llx-r.Llx) < delta && math.Abs(c.Urx-r.Urx) < delta {
				contCol[i] = struct{}{}
				contRun[j] = struct{}{}
			}
		}
	}

	var closed []idRect  // Columsn that end here.
	var opened []idRect  // Columns that start here.
	var running []idRect // Columns that continue.
	for i, r := range columns {
		if _, ok := contCol[i]; !ok {
			opened = append(opened, ss.newIDRect(r))
		}
	}
	for i, idr := range ss.running {
		if _, ok := contRun[i]; !ok {
			if y < idr.Ury {
				idr.Lly = y
				closed = append(closed, idr)
			}
		} else {
			running = append(running, idr)
		}
	}

	ss.running = append(running, opened...)
	ss.completed = append(ss.completed, closed...)

	common.Log.Info("extendColumns: ss=%s", ss)
	sortX(ss.running, false)
	// checkXOverlaps(ss.running)
	// sortX(ss.completed, true)
	// checkXOverlaps(ss.completed)
}

// gapsToScanLines creates the list of scan lines corresponding to gaps `pageGaps`.
func (ss *scanState) gapsToScanLines(pageGaps rectList) []scanLine {
	events := make([]scanEvent, 2*len(pageGaps))
	for i, gap := range pageGaps {
		idr := ss.newIDRect(gap)
		events[2*i] = scanEvent{enter: true, idRect: idr}
		events[2*i+1] = scanEvent{enter: false, idRect: idr}
	}
	sort.Slice(events, func(i, j int) bool {
		ei, ej := events[i], events[j]
		yi, yj := ei.y(), ej.y()
		if yi != yj {
			return yi > yj
		}
		if ei.enter != ej.enter {
			return ei.enter
		}
		return ei.Llx < ej.Llx
	})

	var slines []scanLine
	e := events[0]
	sl := scanLine{y: e.y(), events: []scanEvent{e}}
	sl.checkXOverlaps()
	common.Log.Info("! %2d of %d: %s", 1, len(events), e)
	for i, e := range events[1:] {
		common.Log.Info("! %2d of %d: %s", i+2, len(events), e)
		if e.y() > sl.y-1.0 {
			sl.events = append(sl.events, e)
			// sl.checkXOverlaps()
		} else {
			slines = append(slines, sl)
			sl = scanLine{y: e.y(), events: []scanEvent{e}}
		}
	}
	slines = append(slines, sl)
	return slines
}

func (sl scanLine) columnsScan(pageBound model.PdfRectangle, enter bool) (
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

// updateGaps returns the elements of `gaps` updated by the events in `sl`.
func (sl scanLine) updateGaps(gaps []idRect) []idRect {
	plus := gaps
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
func (sl scanLine) opening() []idRect {
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
func (sl scanLine) closing() []idRect {
	var idrs []idRect
	for _, e := range sl.events {
		if !e.enter {
			idrs = append(idrs, e.idRect)
		}
	}
	// checkXOverlaps(idrs)
	return idrs
}

func (e scanEvent) String() string {
	dir := "leave"
	if e.enter {
		dir = "ENTER"
	}
	return fmt.Sprintf("<%5.1f %s %s>", e.y(), dir, e.idRect)
}

func (e scanEvent) y() float64 {
	if !e.enter {
		return e.idRect.Lly
	}
	return e.idRect.Ury
}

type xEvent struct {
	x     float64
	enter bool
	i     int
	idRect
}

func (e xEvent) String() string {
	pos := "LEAVE"
	if e.enter {
		pos = "enter"
	}

	return fmt.Sprintf("<%5.1f %s %d %s>", e.x, pos, e.i, e.idRect)
}

// sortX sorts `rl` by Llx then Urx. If `alreadySorted` is true then `rl` is checked to see if it is
// alreadt sorted.
func sortX(rl []idRect, alreadySorted bool) {
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
			panic("sortX")
		}
	} else {
		sort.Slice(rl, less)
	}
}
