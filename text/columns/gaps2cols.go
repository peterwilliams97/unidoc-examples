package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

// gapsToColumns returns the rectangles in `pageBound` that are separated by `pageGaps`.
func gapsToColumns(pageBound model.PdfRectangle, pageGaps rectList) rectList {
	if !bboxValid(pageBound) {
		panic(fmt.Errorf("bad pageBound: %s", showBBox(pageBound)))
	}
	if len(pageGaps) == 0 {
		return rectList{pageBound}
	}
	ss := newScanState(pageBound)
	slines := ss.gapsToScanLines(pageGaps)
	common.Log.Info("@@ gapsToColumns: pageBound=%s", showBBox(pageBound))
	ss.validate()
	var gaps []idRect
	for i, sl := range slines {
		common.Log.Debug("%2d **********  sl=%s", i, sl.String())
		common.Log.Debug("ss=%s", ss.String())
		if sl.y <= ss.pageBound.Lly {
			break
		}
		gaps = sl.updateRects(gaps)
		columns := perforate(ss.pageBound, gaps, sl.y)
		common.Log.Debug("%2d #########  columns=%d", i, len(columns))
		for j, r := range columns {
			fmt.Printf("%4d: %s\n", j, showBBox(r))
		}
		ss.extendColumns(columns, sl.y)
		common.Log.Debug("ss=%s", ss.String())
		ss.validate()
	}
	// Close all the running columns.
	common.Log.Debug("FINAL CLOSER")
	for i, idr := range ss.running {
		common.Log.Debug("running[%d]=%s", i, idr)
		idr.Lly = pageBound.Lly
		if idr.Ury-idr.Lly > 0 {
			common.Log.Debug("%4d completed[%d]=%s", i, len(ss.completed), idr)
			idr.validate()
			ss.completed = append(ss.completed, idr)
			ss.validate()
		}
	}

	sort.Slice(ss.completed, func(i, j int) bool {
		return ss.completed[i].id < ss.completed[j].id
	})
	common.Log.Debug("gapsToColumns: pageGaps=%d pageBound=%5.1f", len(pageGaps), pageBound)
	for i, c := range pageGaps {
		fmt.Printf("%4d: %5.1f\n", i, c)
	}
	common.Log.Debug("gapsToColumns: completed=%d", len(ss.completed))
	for i, c := range ss.completed {
		fmt.Printf("%4d: %s\n", i, c)
	}
	columns := make(rectList, len(ss.completed))
	for i, c := range ss.completed {
		if bboxEmpty(c.PdfRectangle) {
			panic(fmt.Errorf("bad bbox: i=%d c=%s", i, c))
		}
		columns[i] = c.PdfRectangle
	}
	common.Log.Debug("gapsToColumns: columns=%d", len(columns))
	for i, c := range columns {
		fmt.Printf("%4d: %5.1f\n", i, c)
		if bboxEmpty(c) {
			panic(fmt.Errorf("bad bbox: i=%d c=%s", i, showBBox(c)))
		}
	}
	return columns
}

type scanState struct {
	pageBound model.PdfRectangle
	running   []idRect // must be sorted left to right
	completed []idRect
	store     map[int]idRect
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
		if !bboxValid(r) {
			common.Log.Error("bound=%s y=%5.1f", showBBox(bound), y)
			common.Log.Error("    r=%s", showBBox(r))
			panic("BBox")
		}
		return rectList{r}
	}
	sortX(gaps, false)
	// checkXOverlaps(gaps)

	events := make([]zEvent, 2*len(gaps))
	for i, r := range gaps {
		events[2*i] = zEvent{idRect: r, z: r.Llx, i: i, enter: true}
		events[2*i+1] = zEvent{idRect: r, z: r.Urx, i: i, enter: false}
	}

	sort.Slice(events, func(i, j int) bool {
		ei, ej := events[i], events[j]
		xi, xj := ei.z, ej.z
		if xi != xj {
			return xi < xj
		}
		return ei.i < ej.i
	})

	var columns1 rectList
	add := func(llx, urx float64, whence string, e zEvent) {
		if urx > llx {
			r := model.PdfRectangle{Llx: llx, Urx: urx, Ury: y}
			common.Log.Debug("\tcolumns1[%d]=%s %q %s", len(columns1), showBBox(r), whence, e)
			if !bboxValid(r) {
				panic("BBox")
			}
			columns1 = append(columns1, r)
		}
	}

	common.Log.Debug("<< perforate y=%.1f gaps=%d", y, len(gaps))
	llx := bound.Llx
	depth := 0
	for i, e := range events {
		if e.enter {
			if depth == 0 {
				add(llx, e.z, "A", e) //  g.Llx)
			}
			depth++
		} else {
			depth--
			if depth == 0 {
				llx = e.Urx
			}
		}
		common.Log.Debug("%3d: depth=%d llx=%5.1f %s", i, depth, llx, e)
		if depth < 0 {
			panic("depth")
		}
	}
	add(llx, bound.Urx, "C", zEvent{})

	common.Log.Debug(">> perforate  gaps=%d", len(gaps))
	for i, idr := range gaps {
		fmt.Printf("%4d: %s\n", i, idr)
	}
	common.Log.Debug("perforate columns1=%d", len(columns1))
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

	common.Log.Debug("extendColumns: ss=%s", ss)
	sortX(ss.running, false)
	// checkXOverlaps(ss.running)
	// sortX(ss.completed, true)
	// checkXOverlaps(ss.completed)
}

// gapsToScanLines creates the list of scan lines corresponding to gaps `pageGaps`.
func (ss *scanState) gapsToScanLines(pageGaps rectList) []scanLine {
	events := make([]zEvent, 2*len(pageGaps))
	for i, gap := range pageGaps {
		r := ss.newIDRect(gap)
		events[2*i] = zEvent{idRect: r, z: r.Ury, i: i, enter: true}
		events[2*i+1] = zEvent{idRect: r, z: r.Lly, i: i, enter: false}
	}
	sort.Slice(events, func(i, j int) bool {
		ei, ej := events[i], events[j]
		yi, yj := ei.z, ej.z
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
	sl := scanLine{y: e.z, events: []zEvent{e}}
	sl.checkXOverlaps()
	common.Log.Debug("! %2d of %d: %s", 1, len(events), e)
	for i, e := range events[1:] {
		common.Log.Debug("! %2d of %d: %s", i+2, len(events), e)
		if e.z > sl.y-1.0 {
			sl.events = append(sl.events, e)
			// sl.checkXOverlaps()
		} else {
			slines = append(slines, sl)
			sl = scanLine{y: e.z, events: []zEvent{e}}
		}
	}
	slines = append(slines, sl)
	return slines
}
