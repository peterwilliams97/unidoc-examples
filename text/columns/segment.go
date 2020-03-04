package main

import (
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// getColumnText converts `lines` (lines of words) into table string cells by accounting for
// distribution of lines into columns as specified by `columns`.
func getColumnText(lines [][]extractor.TextMarkArray, columns rectList) []string {
	if len(columns) == 0 {
		return nil
	}
	columnLines := make([][]string, len(columns))
	for _, line := range lines {
		linedata := make([][]string, len(columns))
		for _, word := range line {
			wordBBox, ok := word.BBox()
			if !ok {
				continue
			}

			bestColumn := 0
			bestOverlap := 1.0
			for icol, colBBox := range columns {
				overlap := areaOverlap(wordBBox, colBBox)
				if overlap < bestOverlap {
					bestOverlap = overlap
					bestColumn = icol
				}
			}
			linedata[bestColumn] = append(linedata[bestColumn], word.Text())
		}
		for i, w := range linedata {
			if len(w) > 0 {
				text := strings.Join(w, " ")
				columnLines[i] = append(columnLines[i], text)
			}
		}
	}
	columnText := make([]string, len(columns))
	for i, line := range columnLines {
		columnText[i] = strings.Join(line, "\n")
	}
	return columnText
}

// identifyLines returns `words` segmented into horizontal lines (words with roughly same y position).
func identifyLines(words []extractor.TextMarkArray) [][]extractor.TextMarkArray {
	var lines [][]extractor.TextMarkArray

	for k, word := range words {
		wbbox, ok := word.BBox()
		if !ok {
			panic("bbox")
			continue
		}

		match := false
		for i, line := range lines {
			firstWord := line[0]
			firstBBox, ok := firstWord.BBox()
			if !ok {
				continue
			}

			overlap := lineOverlap(wbbox, firstBBox)
			common.Log.Debug("overlap: %+.2f word=%d line=%d \n\t%5.1f '%s'\n\t%5.1f '%s'",
				overlap, k, i, firstBBox, firstWord.String(), wbbox, word.String())
			if overlap < 0 {
				lines[i] = append(lines[i], word)
				match = true
				break
			}
		}
		if !match {
			lines = append(lines, []extractor.TextMarkArray{word})
		}
	}

	// Sort lines by base height of first word in line, top to bottom.
	sort.SliceStable(lines, func(i, j int) bool {
		bboxi, _ := lines[i][0].BBox()
		bboxj, _ := lines[j][0].BBox()
		return bboxi.Lly >= bboxj.Lly
	})
	// Sort contents of each line by x position, left to right.
	for li := range lines {
		sort.SliceStable(lines[li], func(i, j int) bool {
			bboxi, _ := lines[li][i].BBox()
			bboxj, _ := lines[li][j].BBox()
			return bboxi.Llx < bboxj.Llx
		})
	}

	// Save the line bounding boxes for markup output.
	var lineGroups []model.PdfRectangle
	for li, line := range lines {
		var lineRect model.PdfRectangle
		gotBBox := false
		common.Log.Trace("Line %d: ", li+1)
		for _, word := range line {
			wbbox, ok := word.BBox()
			if !ok {
				continue
			}
			common.Log.Trace("'%s' / ", word.String())

			if !gotBBox {
				lineRect = wbbox
				gotBBox = true
			} else {
				lineRect = rectUnion(lineRect, wbbox)
			}
		}
		lineGroups = append(lineGroups, lineRect)
	}
	saveParams.markups[saveParams.curPage]["lines"] = lineGroups
	return lines
}
