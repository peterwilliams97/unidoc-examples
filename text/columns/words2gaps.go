package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

// wordsToGaps returns the gaps between `obstacles` within `bound`.
func wordsToGaps(bound model.PdfRectangle, obstacles rectList) rectList {
	ss := newFragmentState(bound, obstacles)
	return ss.scan()
}

type fragmentState struct {
	pageBound model.PdfRectangle
	running   []idRect // must be sorted left to right
	completed []idRect
	words     mosaic
}

func newFragmentState(pageBound model.PdfRectangle, pageWords rectList) *fragmentState {
	ss := fragmentState{
		pageBound: pageBound,
		words:     createMosaic(pageWords),
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

// pokeHoles returns the gaps between `words` with bounding box `bound`.
func pokeHoles(bound model.PdfRectangle, words []idRect) rectList {
	if len(words) == 0 {
		return rectList{bound}
	}
	sortX(words, false)
	// checkXOverlaps(words)

	events := make([]zEvent, 2*len(words))
	for i, r := range words {
		events[2*i] = zEvent{idRect: r, z: r.Llx, i: i, enter: true}
		events[2*i+1] = zEvent{idRect: r, z: r.Urx, i: i, enter: false}
		if r.Llx < bound.Llx {
			panic("1) llx")
		}
		if r.Urx > bound.Urx {
			panic("2) urx")
		}
	}

	sort.Slice(events, func(i, j int) bool {
		ei, ej := events[i], events[j]
		xi, xj := ei.z, ej.z
		if xi != xj {
			return xi < xj
		}
		return ei.i < ej.i
	})

	var holes rectList
	add := func(llx, urx float64, whence string, e zEvent) {
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
				add(llx, e.z, "A", e) //  g.Llx)
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
	add(llx, bound.Urx, "C", zEvent{})
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
