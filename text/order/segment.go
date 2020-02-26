package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// getColumnText converts `lines` (lines of words) into table string cells by accounting for
// distribution of lines into columns as specified by `columnBBoxes`.
func getColumnText(lines [][]segmentationWord, columnBBoxes rectList) []string {
	if len(columnBBoxes) == 0 {
		return nil
	}
	columnLines := make([][]string, len(columnBBoxes))
	for _, line := range lines {
		linedata := make([][]string, len(columnBBoxes))
		for _, word := range line {
			wordBBox, ok := word.BBox()
			if !ok {
				continue
			}

			bestColumn := 0
			bestOverlap := 1.0
			for icol, colBBox := range columnBBoxes {
				overlap := columnOverlap(wordBBox, colBBox)
				if overlap < bestOverlap {
					bestOverlap = overlap
					bestColumn = icol
				}
			}
			linedata[bestColumn] = append(linedata[bestColumn], word.String())
		}
		for i, w := range linedata {
			if len(w) > 0 {
				text := strings.Join(w, " ")
				columnLines[i] = append(columnLines[i], text)
			}
		}
	}
	columnText := make([]string, len(columnBBoxes))
	for i, line := range columnLines {
		columnText[i] = strings.Join(line, "\n")
	}
	return columnText
}

// segmentationWord represents a word that has been segmented in PDF text.
type segmentationWord struct {
	ma *extractor.TextMarkArray
}

func (w segmentationWord) Elements() []extractor.TextMark {
	return w.ma.Elements()
}

func (w segmentationWord) BBox() (model.PdfRectangle, bool) {
	r, ok := w.ma.BBox()
	if r.Llx > r.Urx {
		r.Llx, r.Urx = r.Urx, r.Llx
	}
	if r.Lly > r.Ury {
		r.Lly, r.Ury = r.Ury, r.Lly
	}
	if !validBBox(r) {
		panic(fmt.Errorf("bad bbox: w=%s\n -- r=%s", w, showBBox(r)))
	}
	return r, ok
}

func (w segmentationWord) String() string {
	if w.ma == nil {
		return ""
	}

	var buf bytes.Buffer
	for _, m := range w.Elements() {
		buf.WriteString(m.Text)
	}
	return buf.String()
}

// stringFromLine returns a string describing the group of lines `block`.
func stringFromBlock(block [][]segmentationWord) string {
	lines := make([]string, len(block))
	for i, l := range block {
		lines[i] = fmt.Sprintf("%3d: %s", i, stringFromLine(l))
	}
	return fmt.Sprintf("%d lines ----------\n%s", len(lines), strings.Join(lines, "\n"))
}

// stringFromLine returns a string describing line `line`.
func stringFromLine(line []segmentationWord) string {
	words := make([]string, len(line))
	for i, w := range line {
		bbox, _ := w.BBox()
		gap := ""
		if i > 0 {
			b1, _ := line[i-1].BBox()
			gap = fmt.Sprintf("%.1f->%.1f=%.1f", b1.Urx, bbox.Llx, bbox.Llx-b1.Urx)
		}
		words[i] = fmt.Sprintf("[%.1f: %s %q]", bbox, gap, w.String())
	}
	return fmt.Sprintf("%2d: %s", len(words), strings.Join(words, "\n\t/-/"))
}

func textFromLine(line []segmentationWord) string {
	words := make([]string, len(line))
	for i, w := range line {
		words[i] = w.String()
	}
	return strings.Join(words, "|-|")
}
