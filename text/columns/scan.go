package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
)

/*
 * Scan conversion
 */

// scanLine is a list of scan events with the same y value.
type scanLine struct {
	y      float64  // y == e.z ∀ e ∈ `events`.
	events []zEvent // events with e.z == `y`.
}


func (sl scanLine) String() string {
	parts := make([]string, len(sl.events))
	for i, e := range sl.events {
		parts[i] = e.String()
	}
	return fmt.Sprintf("[y=%.1f %d %s]", sl.y, len(sl.events), strings.Join(parts, " "))
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


// updateRects returns the elements of `gaps` updated by the events in `sl`.
func (sl scanLine) updateRects(gaps []idRect) []idRect {
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

// zEvent is an event for scanning top-to-bottom or left-to-right
type zEvent struct {
	idRect         // The rectangle being entered or left.
	enter  bool    // true (false) is rectangle is being entered (left).
	z      float64 // Value of x or y as idRect is entered or left
	i      int     // Index of zEvent in a slice
}

func (e zEvent) String() string {
	pos := "LEAVE"
	if e.enter {
		pos = "enter"
	}
	return fmt.Sprintf("<%5.1f %s %d %s>", e.z, pos, e.i, e.idRect)
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
