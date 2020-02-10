/*
 * Split columns: Example illustrating capability to extract TextMarks from PDF, and group together
 * into words, rows and columnn.
 *
 * Includes debugging capabilities such as outputing a marked up PDF showing bounding boxes of marks,
 * words, lines and columns.
 *
 * Run as: go run split_columns.go -m all -mf markup.pdf table.pdf
 * - Outputs debug markup including: marks, words, lines, columns to markup.pdf
 * - The table data is outputed to table.csv with UTF-8 encoding.
 */

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/contentstream"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/creator"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

const (
	usage = "Usage: go run split_columns.go [options] <file.pdf> <output.txt>\n"
	// Make sure to enter a valid license key.
	// Otherwise text is truncated and a watermark added to the text.
	// License keys are available via: https://unidoc.io
	/*
			license.SetLicenseKey(`
		-----BEGIN UNIDOC LICENSE KEY-----
		...key contents...
		-----END UNIDOC LICENSE KEY-----
		`, "Customer Name")
	*/
	// Alternatively license can be loaded via UNIPDF_LICENSE_PATH and UNIPDF_CUSTOMER_NAME
	// environment variables,  where UNIPDF_LICENSE_PATH points to the file containing the license
	// key and the UNIPDF_CUSTOMER_NAME the explicitly specified customer name to which the key is
	// licensed.
	uniDocLicenseKey = ``
	companyName      = ""
)

var saveParams saveMarkedupParams

func main() {
	var (
		loglevel   string
		saveMarkup string
		markupPath string
	)
	flag.StringVar(&loglevel, "l", "info", "Set log level (default: info)")
	flag.StringVar(&saveMarkup, "m", "columns", "Save markup (none/marks/words/lines/columns/all)")
	flag.StringVar(&markupPath, "mf", "./layout.pdf", "Output markup path (default /tmp/markup.pdf)")
	makeUsage(usage)
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	switch strings.ToLower(loglevel) {
	case "trace":
		common.SetLogger(common.NewConsoleLogger(common.LogLevelTrace))
	case "debug":
		common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))
	default:
		common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))
	}
	if uniDocLicenseKey != "" {
		if err := license.SetLicenseKey(uniDocLicenseKey, companyName); err != nil {
			panic(fmt.Errorf("error loading UniDoc license: err=%w", err))
		}
		model.SetPdfCreator(companyName)
	}
	// testOverlappingGaps()

	saveParams = saveMarkedupParams{shownMarkups: map[string]struct{}{}}
	saveMarkupLwr := strings.ToLower(saveMarkup)
	switch saveMarkupLwr {
	case "marks", "words", "lines", "divs", "gaps", "columns":
		saveParams.shownMarkups[saveMarkupLwr] = struct{}{}
	case "all":
		saveParams.shownMarkups["columns"] = struct{}{}
		saveParams.shownMarkups["gaps"] = struct{}{}
	default:
		panic(fmt.Errorf("unknown markup type %q", saveMarkup))
	}
	saveParams.markupOutputPath = changePath(markupPath, saveMarkupLwr, ".pdf")

	inPath := args[0]
	outPath := args[1]
	err := extractColumnText(inPath, outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "markupOutputPath=%q\n", saveParams.markupOutputPath)
	fmt.Fprintf(os.Stderr, "shownMarkups=%q\n", saveParams.shownMarkups)
}

// extractColumnText extracts text columns from PDF file `inPath` and outputs the data as a text
// file to `outPath`.
func extractColumnText(inPath, outPath string) error {
	f, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("Could not open %q err=%w", inPath, err)
	}
	defer f.Close()

	pdfReader, err := model.NewPdfReaderLazy(f)
	if err != nil {
		return fmt.Errorf("NewPdfReaderLazy failed. %q err=%w", inPath, err)
	}
	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return fmt.Errorf("GetNumPages failed. %q err=%w", inPath, err)
	}

	saveParams.pdfReader = pdfReader
	saveParams.markups = map[int]map[string][]model.PdfRectangle{}

	var pageTexts []string

	for pageNum := 1; pageNum <= numPages; pageNum++ {
		saveParams.curPage = pageNum
		saveParams.markups[pageNum] = map[string][]model.PdfRectangle{}

		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			return fmt.Errorf("GetPage failed. %q pageNum=%d err=%w", inPath, pageNum, err)
		}

		mbox, err := page.GetMediaBox()
		if err != nil {
			return err
		}
		if page.Rotate != nil && *page.Rotate == 90 {
			// TODO: This is a "hack" to change the perspective of the extractor to account for the rotation.
			contents, err := page.GetContentStreams()
			if err != nil {
				return err
			}

			cc := contentstream.NewContentCreator()
			cc.Translate(mbox.Width()/2, mbox.Height()/2)
			cc.RotateDeg(-90)
			cc.Translate(-mbox.Width()/2, -mbox.Height()/2)
			rotateOps := cc.Operations().String()
			contents = append([]string{rotateOps}, contents...)

			page.Duplicate()
			err = page.SetContentStreams(contents, core.NewRawEncoder())
			if err != nil {
				return err
			}
			page.Rotate = nil
		}

		ex, err := extractor.New(page)
		if err != nil {
			return fmt.Errorf("NewPdfReaderLazy failed. %q pageNum=%d err=%w", inPath, pageNum, err)
		}
		pageText, _, _, err := ex.ExtractPageText()
		if err != nil {
			return fmt.Errorf("ExtractPageText failed. %q pageNum=%d err=%w", inPath, pageNum, err)

		}
		text := pageText.Text()
		textMarks := pageText.Marks()
		common.Log.Info("-------------------------------------------------------")
		common.Log.Info("pageNum=%d text=%d textMarks=%d", pageNum, len(text), textMarks.Len())

		group := make([]model.PdfRectangle, textMarks.Len())
		for i, mark := range textMarks.Elements() {
			group[i] = mark.BBox
		}
		saveParams.markups[pageNum]["marks"] = group

		outPageText, err := pageMarksToColumnText(textMarks, *mbox)
		if err != nil {
			common.Log.Debug("Error grouping text: %v", err)
			return err
		}
		header := fmt.Sprintf("----------------\n ### PAGE %d of %d", pageNum, numPages)
		pageTexts = append(pageTexts, header)
		pageTexts = append(pageTexts, outPageText)
	}

	docText := strings.Join(pageTexts, "\n")
	if err := ioutil.WriteFile(outPath, []byte(docText), 0666); err != nil {
		return fmt.Errorf("failed to write outPath=%q err=%w", outPath, err)
	}

	if len(saveParams.shownMarkups) != 0 {
		err = saveMarkedupPDF(saveParams)
		if err != nil {
			return fmt.Errorf("failed to save marked up pdf: %w", err)
		}
	}

	return nil
}

// pageMarksToColumnText converts `textMarks`, the text marks from a single page, into a string by
// grouping the marks into words, lines and columns and then merging the column texts.
func pageMarksToColumnText(textMarks *extractor.TextMarkArray, pageSize model.PdfRectangle) (
	string, error) {
	// STEP - Form words.
	// Group the closest text marks that are overlapping.
	var words []segmentationWord
	word := segmentationWord{ma: &extractor.TextMarkArray{}}
	var lastMark extractor.TextMark
	isFirst := true
	for i, mark := range textMarks.Elements() {
		if mark.Text == "" {
			continue
		}
		common.Log.Trace("Mark %d - '%s' (% X)", i, mark.Text, mark.Text)
		if isFirst {
			word = segmentationWord{ma: &extractor.TextMarkArray{}}
			word.ma.Append(mark)
			lastMark = mark
			isFirst = false
			continue
		}
		common.Log.Trace(" - areaOverlap: %f", areaOverlap(mark.BBox, lastMark.BBox))
		overlap := areaOverlap(mark.BBox, lastMark.BBox)
		if overlap > 0.1 {
			if len(strings.TrimSpace(word.String())) > 0 {
				common.Log.Trace("Appending word: '%s' (%d chars) (%d elements)", word.String(),
					len(word.String()), len(word.Elements()))
				words = append(words, word)
			}
			word = segmentationWord{ma: &extractor.TextMarkArray{}}
		}
		word.ma.Append(mark)
		lastMark = mark
	}
	if len(strings.TrimSpace(word.String())) > 0 {
		common.Log.Info("Appending word: '%s' (%d chars) (%d elements)", word.String(),
			len(word.String()), len(word.Elements()))
		words = append(words, word)
	}

	// Include the words in the markup.
	{
		var wbboxes []model.PdfRectangle
		for _, word := range words {
			wbbox, ok := word.BBox()
			if !ok {
				continue
			}
			wbboxes = append(wbboxes, wbbox)
		}
		saveParams.markups[saveParams.curPage]["words"] = wbboxes
	}

	lines := identifyLines(words)
	common.Log.Info("lines=\n%s", stringFromBlock(lines))
	common.Log.Info("lines=%d", len(lines))
	common.Log.Info("=============================================")

	tableLines := lines

	var tableWords []segmentationWord
	for _, line := range tableLines {
		tableWords = append(tableWords, line...)
	}

	gapSize := charMultiplier * averageWidth(textMarks)
	common.Log.Info("gapSize=%.1f = %1.f mm charMultiplier=%.1f averageWidth(textMarks)=%.1f",
		gapSize, gapSize/72.0*25.4, charMultiplier, averageWidth(textMarks))

	columnBBoxes := identifyColumns(tableLines, pageSize, gapSize)
	common.Log.Info("%d columns~~~~~~~~~~~~~~~~~~~ ", len(columnBBoxes))
	for i, bbox := range columnBBoxes {
		common.Log.Info("%4d of %d: %5.1f", i+1, len(columnBBoxes), bbox)
	}

	columnText := getColumnText(lines, columnBBoxes)
	for i, bbox := range columnBBoxes {
		common.Log.Info("%4d of %d: %5.1f %d chars^^^^^^^^^^^^^^^^^^", i+1, len(columnBBoxes), bbox,
			len(columnText[i]))
		common.Log.Info("%s", columnText)
	}

	return strings.Join(columnText, "\n####\n"), nil
}

// identifyLines returns `words` segmented into horizontal lines (words with roughly same y position).
func identifyLines(words []segmentationWord) [][]segmentationWord {
	var lines [][]segmentationWord

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
			lines = append(lines, []segmentationWord{word})
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

// identifyColumns returns the rectangles of the bounds of columns that `lines` are arranged within.
func identifyColumns(lines [][]segmentationWord, pageSize model.PdfRectangle,
	gapWidth float64) []model.PdfRectangle {
	common.Log.Info("lines=%d", len(lines))
	var pageDivs []division
	for _, line := range lines {
		div := calcLineGaps(line, pageSize.Width(), gapWidth)
		if len(div.gaps) == 0 {
			continue
		}
		pageDivs = append(pageDivs, div)
	}
	common.Log.Info("pageDivs=%d", len(pageDivs))
	for i, div := range pageDivs {
		marker := fmt.Sprintf("@@%d", len(div.gaps))
		if len(div.gaps) == 2 {
			// continue
			marker = "  "
		}
		fmt.Printf("\t\t%s %4d: %s\n", marker, i, div)
	}
	saveParams.markups[saveParams.curPage]["divs"] = pageDivsToRects(pageDivs)

	pageGaps := coallesceGaps(pageDivs, gapWidth, gapHeight)
	common.Log.Info("pageGaps=%d", len(pageGaps))

	// Include the gaps in the markup.
	saveParams.markups[saveParams.curPage]["gaps"] = pageGaps

	// Sort columns by left of first word in line, left to right.
	sort.SliceStable(pageGaps, func(i, j int) bool {
		ri, rj := pageGaps[i], pageGaps[j]
		if ri.Ury != rj.Ury {
			return ri.Ury >= rj.Ury
		}
		return ri.Llx < rj.Llx
	})

	columns := scanPage(pageGaps, pageSize)
	saveParams.markups[saveParams.curPage]["columns"] = columns
	return columns
}

// calcLineGaps returns the gaps in `line`.
func calcLineGaps(line []segmentationWord, pageWidth, gapWidth float64) division {
	bboxes := lineBboxes(line)
	if len(bboxes) == 0 {
		return division{}
	}
	common.Log.Info("bboxes<0>= %d %.1f", len(bboxes), bboxes)
	bboxes = mergeXBboxes(bboxes)
	common.Log.Info("bboxes<1>= %d %.1f", len(bboxes), bboxes)

	y0 := bboxes[0].Lly
	y1 := bboxes[0].Ury
	gap := model.PdfRectangle{Llx: 0.0, Urx: bboxes[0].Llx, Lly: y0, Ury: y1}
	gaps := []model.PdfRectangle{gap}

	widest := -100.0

	for i := 1; i < len(bboxes); i++ {
		if bboxes[i].Lly < y0 {
			y0 = bboxes[i].Lly
		}
		if bboxes[i].Ury > y0 {
			y1 = bboxes[i].Ury
		}
		x0 := bboxes[i-1].Urx
		x1 := bboxes[i].Llx
		if x1-x0 >= gapWidth {
			gap := model.PdfRectangle{Llx: x0, Urx: x1, Lly: y0, Ury: y1}
			gaps = append(gaps, gap)
		}
		common.Log.Info("%3d: x0=%.1f x1=%.f", i, x0, x1)
		if x1-x0 >= widest {
			widest = x1 - x0
		}
	}
	gap = model.PdfRectangle{
		Llx: bboxes[len(bboxes)-1].Urx,
		Urx: pageWidth,
		Lly: y0,
		Ury: y1,
	}
	gaps = append(gaps, gap)

	if len(bboxes) >= 2 {
		common.Log.Info("widest=%.1f", widest)
		if widest < 0.0 {
			panic("widest")
		}
	}

	div := division{
		gaps:   gaps,
		text:   textFromLine(line),
		widest: widest,
	}

	common.Log.Info("bboxes=%.1f -> div=%s", bboxes, div)

	return div
}

// coallesceGaps merges matching gaps in successive lines of pageGaps. A match is a gap width and
// height of at least `gapWidth` and `gapHeight` respectively.

//  +---------+   +---------+   +-------------------+
//  |         | 1 |         |   |                   |
//  |         |   |         |   |                   |  div0          |
//  +---------+   +---------+ 2 +-------------------+                |
//                                                     L             |
//  +-----------------------+   +-------+   +-------+                |
//  |                       |   |       |   |       |                v
//  |                       |   |       | 3 |       | div1
//  |                       |   |       |   |       |
//  +-----------------------+   +-------+   +-------+
//
// At line L, going downwards,
//  - gap 1 is closed
//  - gap 2 is continued
//  - gap 3 is opened
func coallesceGaps(pageGaps []division, gapWidth float64, gapHeight int) []model.PdfRectangle {
	var gaps []model.PdfRectangle
	div0 := pageGaps[0]
	div := div0
	for _, div1 := range pageGaps[1:] {
		closed, continued, opened := overlappingGaps(div, div1, gapWidth)
		if len(closed.gaps) > 0 {
			gaps = append(gaps, closed.gaps...)
			common.Log.Info("!!^CLOSED=%d of %d %5.1f %q", len(closed.gaps), len(gaps), closed.gaps, closed)
		}
		if len(opened.gaps) > 0 {
			common.Log.Info("!!^OPENED=%d %q", len(opened.gaps), opened)
		}
		div = addDivisions(continued, opened)
		common.Log.Info("div=%d %5.1f\n\tdiv1=%s", len(div.gaps), div.gaps, div1)
		if len(closed.gaps) > 0 {
			common.Log.Info("#1 closed=%d %5.1f", len(closed.gaps), closed.gaps)
		}
		common.Log.Info("#2 continued=%d %5.1f", len(continued.gaps), continued.gaps)
		if len(opened.gaps) > 0 {
			common.Log.Info("#3 opened=%d %5.1f", len(opened.gaps), opened.gaps)
		}

		// div.validate()
	}
	if len(div.gaps) > 0 {
		gaps = append(gaps, div.gaps...)
		common.Log.Info("!!^REMAIN=%d of %d %5.1f %q", len(div.gaps), len(gaps), div.gaps, div)
	}
	sort.Slice(gaps, func(i, j int) bool {
		gi, gj := gaps[i], gaps[j]
		if gi.Ury != gj.Ury {
			return gi.Ury > gj.Ury
		}
		if gi.Llx != gj.Llx {
			return gi.Llx < gj.Llx
		}
		if gi.Lly != gj.Lly {
			return gi.Lly > gj.Lly
		}
		return gi.Urx < gj.Urx
	})
	common.Log.Info("gaps=%d", len(gaps))
	for i, g := range gaps {
		fmt.Printf("%4d: %5.1f\n", i, g)
	}

	var bigGaps []model.PdfRectangle
	for _, g := range gaps {
		if g.Height() < 72.0 {
			continue
		}
		bigGaps = append(bigGaps, g)
	}
	common.Log.Info("bigGaps=%d", len(bigGaps))
	for i, g := range bigGaps {
		fmt.Printf("%4d: %5.1f\n", i, g)
	}

	simpleGaps := []model.PdfRectangle{bigGaps[0]}
	for i := 1; i < len(bigGaps); i++ {
		g := bigGaps[i]
		small := false
		for _, left := range bigGaps[:i] {
			if g.Llx < left.Urx+20.0 &&
				g.Lly > left.Lly-50.0 &&
				g.Ury < left.Ury+50.0 {
				small = true
				common.Log.Info("filter simple\n\tleft=%5.1f\n\t   g=%5.1f", left, g)
				break
			}
		}
		if small {
			continue
		}
		simpleGaps = append(simpleGaps, g)
	}
	common.Log.Info("simpleGaps=%d", len(simpleGaps))
	for i, g := range simpleGaps {
		fmt.Printf("%4d: %5.1f\n", i, g)
	}

	return simpleGaps
}

func overlappingGaps(div0, div1 division, gapWidth float64) (closed, continued, opened division) {
	// div0.validate()
	div1.validate()
	elements0 := map[int]struct{}{}
	elements1 := map[int]struct{}{}
	for i0, v0 := range div0.gaps {
		for i1, v1 := range div1.gaps {
			v := horizontalIntersection(v0, v1)
			sign := "≤"
			if v.Width() > gapWidth {
				sign = ">"
			}
			if v.Urx != 0 && v.Ury != 0 && v.Llx != 0 && v.Lly != 0 {
				common.Log.Info("%5.1f + %5.1f -> %5.1f  (%d %d) %.2f %s %.2f ",
					v0, v1, v, i0, i1, v.Width(), sign, gapWidth)
			}
			if v.Width() > gapWidth {
				elements0[i0] = struct{}{}
				elements1[i1] = struct{}{}
				continued.gaps = append(continued.gaps, v)
				// continued.validate()
			}
		}
	}
	for i, v := range div0.gaps {
		if _, ok := elements0[i]; !ok {
			closed.gaps = append(closed.gaps, v)
			closed.validate()
		}
	}
	for i, v := range div1.gaps {
		if _, ok := elements1[i]; !ok {
			opened.gaps = append(opened.gaps, v)
			opened.validate()
		}
	}
	return
}

func testOverlappingGaps() {
	div0 := division{
		gaps: []model.PdfRectangle{
			model.PdfRectangle{Lly: 10, Ury: 20, Llx: 50, Urx: 60},
			model.PdfRectangle{Lly: 10, Ury: 20, Llx: 70, Urx: 96},
			model.PdfRectangle{Lly: 10, Ury: 20, Llx: 200, Urx: 215},
		},
	}
	div1 := division{
		gaps: []model.PdfRectangle{
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

// horizontalIntersection returns a rectangle that is the horizontal intersection and vertical union
// of `v0` and `v1`.
func horizontalIntersection(v0, v1 model.PdfRectangle) model.PdfRectangle {
	v := model.PdfRectangle{
		Llx: math.Max(v0.Llx, v1.Llx),
		Urx: math.Min(v0.Urx, v1.Urx),
		Lly: math.Min(v0.Lly, v1.Lly),
		Ury: math.Max(v0.Ury, v1.Ury),
	}
	if v.Llx >= v.Urx || v.Lly >= v.Ury {
		return model.PdfRectangle{}
	}
	return v
}

func averageWidth(textMarks *extractor.TextMarkArray) float64 {
	total := 0.0
	for _, m := range textMarks.Elements() {
		w := m.BBox.Width()
		total += w
	}
	return total / float64(textMarks.Len())
}

const gapHeight = 5
const charMultiplier = 1.0

func lineBboxes(line []segmentationWord) []model.PdfRectangle {
	bboxes := make([]model.PdfRectangle, 0, len(line))
	for _, w := range line {
		b, ok := w.BBox()
		if !ok {
			continue
		}
		bboxes = append(bboxes, b)
	}
	return bboxes
}

func mergeXBboxes(bboxes []model.PdfRectangle) []model.PdfRectangle {
	merged := make([]model.PdfRectangle, 0, len(bboxes))
	merged = append(merged, bboxes[0])
	for _, b := range bboxes[1:] {
		numOverlaps := 0
		for j, m := range merged {
			if overlappedX(b, m) {
				merged[j] = rectUnion(b, m)
				numOverlaps++
			}
		}
		if numOverlaps == 0 {
			merged = append(merged, b)
		}
	}
	if len(merged) < len(bboxes) {
		common.Log.Info("EEEEE")
	}
	return merged
}

func overlappedX(r0, r1 model.PdfRectangle) bool {
	overl := overlappedX01(r0, r1) || overlappedX01(r1, r0)
	// if overl {
	// 	panic(fmt.Errorf("overlap:\n\tr0=%.1f\n\tr1=%.1f", r0, r1))
	// }
	return overl
}

func overlappedX01(r0, r1 model.PdfRectangle) bool {
	return (r0.Llx <= r1.Llx && r1.Llx <= r0.Urx) || (r0.Llx <= r1.Urx && r1.Urx <= r0.Urx)
}

type scanState struct {
	pageSize  model.PdfRectangle
	running   []idRect      // must be sorted left to right
	gapStack  map[int][]int // {gap id: columns that gap intersects}
	completed []idRect
	store     map[int]idRect
}

func (ss scanState) validate() {
	for _, idr := range ss.running {
		idr.validate()
	}
	for i, idr := range ss.completed {
		idr.validate()
		if idr.PdfRectangle.Width() == 0.0 {
			common.Log.Error("ss=%s", ss)
			panic(fmt.Errorf("width: %d: %s", i, idr))
		}
		if idr.PdfRectangle.Height() == 0.0 {
			common.Log.Error("ss=%s", ss)
			panic(fmt.Errorf("height: %d: %s", i, idr))
		}
	}
}

func (ss scanState) String() string {
	var lines []string
	lines = append(lines, fmt.Sprintf("=== completed=%d stack=%d store=%d =========",
		len(ss.completed), len(ss.gapStack), len(ss.store)))
	for i, c := range ss.completed {
		lines = append(lines, fmt.Sprintf("%4d: %s", i, c))
	}
	lines = append(lines, fmt.Sprintf("--- running=%d", len(ss.running)))
	for i, c := range ss.running {
		lines = append(lines, fmt.Sprintf("%4d: %s", i, c))
	}
	var ids []int
	for id := range ss.gapStack {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	lines = append(lines, fmt.Sprintf("--- gapStack=%d", len(ss.gapStack)))
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf("%4d: %v", id, ss.gapStack[id]))
	}
	return strings.Join(lines, "\n")
}

type scanLine struct {
	y      float64
	events []scanEvent
}

func (sl scanLine) String() string {
	parts := make([]string, len(sl.events))
	for i, e := range sl.events {
		parts[i] = e.String()
	}
	return fmt.Sprintf("[y=%.1f %d %s]", sl.y, len(sl.events), strings.Join(parts, " "))
}

type scanEvent struct {
	idRect
	enter bool
}

type idRect struct {
	model.PdfRectangle
	id int
}

func (idr idRect) validate() {
	if idr.Llx > idr.Urx {
		panic(fmt.Errorf("validate x %s", idr))
	}
	if idr.Lly > idr.Ury {
		panic(fmt.Errorf("validate y %s", idr))
	}
	if idr.id <= 0 {
		panic(fmt.Errorf("validate id %s", idr))
	}
}

func scanPage(pageGaps []model.PdfRectangle, pageSize model.PdfRectangle) []model.PdfRectangle {
	ss := newScanState(pageSize)
	slines := ss.gapsToScanLines(pageGaps)
	common.Log.Info("scanPage %s", ss)
	ss.validate()
	for i, sl := range slines {
		common.Log.Info("%2d **********  sl=%s", i, sl)
		if len(sl.opened()) > 0 {
			ss.open(sl)
			common.Log.Info("%2d OPENED %s", i, ss)
			ss.validate()
		}
		if len(sl.closed()) > 0 {
			ss.close(sl)
			common.Log.Info("%2d CLOSED %s", i, ss)
			ss.validate()
		}
	}
	common.Log.Info("scanPage: pageGaps=%d pageSize=%5.1f", len(pageGaps), pageSize)
	for i, c := range pageGaps {
		fmt.Printf("%4d: %5.1f\n", i, c)
	}
	common.Log.Info("scanPage: completed=%d", len(ss.completed))
	for i, c := range ss.completed {
		fmt.Printf("%4d: %s\n", i, c)
	}
	columns := make([]model.PdfRectangle, len(ss.completed))
	for i, c := range ss.completed {
		columns[i] = c.PdfRectangle
	}
	common.Log.Info("scanPage: columns=%d", len(columns))
	for i, c := range columns {
		fmt.Printf("%4d: %5.1f\n", i, c)
	}
	return columns
}

func newScanState(pageSize model.PdfRectangle) *scanState {
	ss := scanState{
		pageSize: pageSize,
		gapStack: map[int][]int{},
		store:    map[int]idRect{},
	}
	r := model.PdfRectangle{Llx: pageSize.Llx, Urx: pageSize.Urx, Ury: pageSize.Ury}
	idr := ss.newIDRect(r)
	ss.running = append(ss.running, idr)

	return &ss
}

func (ss *scanState) newIDRect(r model.PdfRectangle) idRect {
	id := len(ss.store) + 1
	idr := idRect{id: id, PdfRectangle: r}
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

func (ss *scanState) open(sl scanLine) {
	// save current columns that gaps intersect
	// intersect columns with inverse of gaps
	// create new columns
	common.Log.Info("sl.opened()=%s", sl.opened())
	if len(sl.opened()) == 0 {
		return
	}
	running := ss.intersect(ss.running, sl.opened(), sl.y)
	closed := difference(ss.running, running)
	common.Log.Info("\n\tss.running=%s\n\t   running=%s\n\t    closed=%s", ss.running, running, closed)
	for _, idr := range closed {
		idr.Lly = sl.y
		ss.completed = append(ss.completed, idr)
	}
	ss.running = running
}

func (ss *scanState) close(sl scanLine) {
	// complete running. added to compleleted list
	// pop old columns
	common.Log.Info("sl.closed()=%s", sl.closed())
	if len(sl.closed()) == 0 {
		return
	}
	oldRunning := ss.popIntersect(sl.closed())
	closed := difference(ss.running, oldRunning)
	common.Log.Info("\n\tss.running=%s\n\toldRunning=%s\n\t    closed=%s", ss.running, oldRunning, closed)
	for i, idr := range closed {
		if sl.y == idr.Ury {
			panic(fmt.Errorf("height: i=%d, idr=%s", i, idr))
		}
		idr.Lly = sl.y
		ss.completed = append(ss.completed, idr)
		ss.validate()
	}
	running := make([]idRect, len(oldRunning))
	for i, idr := range oldRunning {
		r := idr.PdfRectangle
		r.Ury = sl.y
		running[i] = ss.newIDRect(r)
	}

	ss.running = running
}

// difference returns the elements in `a` that aren't in `b`.
func difference(a, b []idRect) []idRect {
	bs := map[int]struct{}{}
	for _, idr := range b {
		bs[idr.id] = struct{}{}
	}
	var diff []idRect
	for _, idr := range a {
		if _, ok := bs[idr.id]; !ok {
			diff = append(diff, idr)
		}
	}
	return diff
}

func (ss *scanState) intersect(columns, gaps []idRect, y float64) []idRect {
	for _, g := range gaps {
		for _, c := range columns {
			if overlappedX(c.PdfRectangle, g.PdfRectangle) {
				ss.gapStack[g.id] = append(ss.gapStack[g.id], c.id)
			}
		}
	}
	var columns1 []idRect
	for _, c := range columns {
		olap := touchingGaps(c, gaps)
		if len(olap) == 0 {
			idr := ss.newIDRect(c.PdfRectangle)
			columns1 = append(columns1, idr)
			common.Log.Info("columns1=%d idr=%s", len(columns1), idr)
			continue
		}
		if olap[0].Llx <= c.Llx {
			c.Llx = olap[0].Urx
			olap = olap[1:]
		}
		if len(olap) == 0 {
			c.Ury = y
			idr := ss.newIDRect(c.PdfRectangle)
			columns1 = append(columns1, idr)
			common.Log.Info("columns1=%d idr=%s", len(columns1), idr)
			continue
		}
		if olap[len(olap)-1].Urx >= c.Urx {
			c.Urx = olap[len(olap)-1].Llx
			olap = olap[:len(olap)-1]
		}
		if len(olap) == 0 {
			c.Ury = y
			idr := ss.newIDRect(c.PdfRectangle)
			columns1 = append(columns1, idr)
			common.Log.Info("columns1=%d idr=%s", len(columns1), idr)
			continue
		}
		x0 := c.Llx
		for _, g := range olap {
			x1 := g.Llx
			r := model.PdfRectangle{Llx: x0, Urx: x1, Ury: y}
			idr := ss.newIDRect(r)
			columns1 = append(columns1, idr)
			common.Log.Info("columns1=%d idr=%s", len(columns1), idr)
			x0 = g.Urx
		}
		x1 := c.Urx
		r := model.PdfRectangle{Llx: x0, Urx: x1, Ury: y}
		idr := ss.newIDRect(r)
		columns1 = append(columns1, idr)
		common.Log.Info("columns1=%d idr=%s", len(columns1), idr)
	}
	return columns1
}

func touchingGaps(col idRect, gaps []idRect) []idRect {
	var olap []idRect
	for _, g := range gaps {
		if !overlappedX(col.PdfRectangle, g.PdfRectangle) {
			continue
		}
		olap = append(olap, g)
	}
	return olap
}

// popIntersect returns the columns that were split by `gaps`. This function is used to close gaps
// that were opened by intersect.
func (ss *scanState) popIntersect(gaps []idRect) []idRect {
	var columns1 []idRect
	for _, g := range gaps {
		cols := ss.gapStack[g.id]
		for _, cid := range cols {
			columns1 = append(columns1, ss.getIDRect(cid))
		}
	}
	return columns1
}

func (ss *scanState) gapsToScanLines(pageGaps []model.PdfRectangle) []scanLine {
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
	for _, e := range events[1:] {
		if e.y() > sl.y-1.0 {
			sl.events = append(sl.events, e)
		} else {
			slines = append(slines, sl)
			sl = scanLine{y: e.y(), events: []scanEvent{e}}
		}
	}
	slines = append(slines, sl)
	return slines
}

func (sl scanLine) columnsScan(pageSize model.PdfRectangle, enter bool) (
	opened, closed []model.PdfRectangle) {

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
	x0 := pageSize.Llx
	for _, e := range sl.events {
		if e.enter != enter {
			continue
		}
		x1 := e.Llx
		addCol(x0, x1)
		x0 = e.Urx
	}
	x1 := pageSize.Urx
	addCol(x0, x1)
	return opened, closed
}

func (sl scanLine) opened() []idRect {
	var idrs []idRect
	for _, e := range sl.events {
		if e.enter {
			idrs = append(idrs, e.idRect)
		}
	}
	return idrs
}

func (sl scanLine) closed() []idRect {
	var idrs []idRect
	for _, e := range sl.events {
		if !e.enter {
			idrs = append(idrs, e.idRect)
		}
	}
	return idrs
}

func (idr idRect) String() string {
	return fmt.Sprintf("(%4d %5.1f)", idr.id, idr.PdfRectangle)
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

// division is a representation of the gaps in a group of lines.
// !@#$ Rename to layout
type division struct {
	widest float64
	gaps   []model.PdfRectangle
	text   string
}

func pageDivsToRects(divs []division) []model.PdfRectangle {
	var rects []model.PdfRectangle
	for _, div := range divs {
		rects = append(rects, div.gaps...)
	}
	return rects
}

func addDivisions(div0, div1 division) division {
	return division{
		gaps: append(div0.gaps, div1.gaps...),
		text: fmt.Sprintf("%s\n~~ %s", div0.text, div1.text),
	}
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

// bboxArea returns the area of `bbox`.
func bboxArea(bbox model.PdfRectangle) float64 {
	return math.Abs(bbox.Urx-bbox.Llx) * math.Abs(bbox.Ury-bbox.Lly)
}

// bboxWidth returns the width of `bbox`.
func bboxWidth(bbox model.PdfRectangle) float64 {
	return math.Abs(bbox.Urx - bbox.Llx)
}

// bboxHeight returns the height of `bbox`.
func bboxHeight(bbox model.PdfRectangle) float64 {
	return math.Abs(bbox.Ury - bbox.Lly)
}

// getColumnText converts `lines` (lines of words) into table string cells by accounting for
// distribution of lines into columns as specified by `columnBBoxes`.
func getColumnText(lines [][]segmentationWord, columnBBoxes []model.PdfRectangle) []string {
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
	return w.ma.BBox()
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

type saveMarkedupParams struct {
	pdfReader        *model.PdfReader
	markups          map[int]map[string][]model.PdfRectangle
	curPage          int
	shownMarkups     map[string]struct{}
	markupOutputPath string
}

// Saves a marked up PDF with the original with certain groups highlighted: marks, words, lines, columns.
func saveMarkedupPDF(params saveMarkedupParams) error {
	var pageNums []int
	for pageNum := range params.markups {
		pageNums = append(pageNums, pageNum)
	}
	sort.Ints(pageNums)
	if len(pageNums) == 0 {
		return nil
	}

	// Make a new PDF creator.
	c := creator.New()
	for _, pageNum := range pageNums {
		common.Log.Debug("Page %d - %d marks", pageNum, len(params.markups[pageNum]))
		page, err := params.pdfReader.GetPage(pageNum)
		if err != nil {
			return fmt.Errorf("saveOutputPdf: Could not get page pageNum=%d. err=%w", pageNum, err)
		}
		mediaBox, err := page.GetMediaBox()
		if err != nil {
			return fmt.Errorf("saveOutputPdf: Could not get MediaBox  pageNum=%d. err=%w", pageNum, err)
		}
		if page.MediaBox == nil {
			// Deal with MediaBox inherited from Parent.
			common.Log.Info("MediaBox: %v -> %v", page.MediaBox, mediaBox)
			page.MediaBox = mediaBox
		}
		h := mediaBox.Ury

		if err := c.AddPage(page); err != nil {
			return fmt.Errorf("AddPage failed err=%w", err)
		}

		for _, markupType := range markupKeys(params.markups[pageNum]) {
			group := params.markups[pageNum][markupType]
			if _, ok := params.shownMarkups[markupType]; !ok {
				continue
			}

			common.Log.Info("markupType=%q", markupType)

			width := widths[markupType]
			borderColor := creator.ColorRGBFromHex(colors[markupType])
			bgdColor := creator.ColorRGBFromHex(bkgnds[markupType])
			common.Log.Info("borderColor=%+q %.2f", colors[markupType], borderColor)
			common.Log.Info("   bgdColor=%+q %.2f", bkgnds[markupType], bgdColor)
			for i, r := range group {
				common.Log.Info("Mark %d: %5.1f x,y,w,h=%5.1f %5.1f %5.1f %5.1f", i+1, r,
					r.Llx, h-r.Lly, r.Urx-r.Llx, -(r.Ury - r.Lly))
				dx := 5.0
				dy := 5.0
				if r.Urx-r.Llx < 4.0*dx {
					dx = (r.Urx - r.Llx) / 4.0
				}
				if r.Ury-r.Lly < 4.0*dy {
					dy = (r.Ury - r.Lly) / 4.0
				}
				llx := r.Llx + dx
				urx := r.Urx - dx
				lly := r.Lly + dy
				ury := r.Ury - dy

				rect := c.NewRectangle(llx, h-lly, urx-llx, -(ury - lly))
				rect.SetBorderColor(bgdColor)
				rect.SetBorderWidth(1.5 * width)
				err = c.Draw(rect)
				if err != nil {
					panic("1")
					return fmt.Errorf("Draw failed (background). pageNum=%d err=%w", pageNum, err)
				}
				rect = c.NewRectangle(llx, h-lly, urx-llx, -(ury - lly))
				rect.SetBorderColor(borderColor)
				rect.SetBorderWidth(1.0 * width)
				err = c.Draw(rect)
				if err != nil {
					panic("2")
					return fmt.Errorf("Draw failed (foreground).pageNum=%d err=%w", pageNum, err)
				}
			}
		}
	}

	c.SetOutlineTree(params.pdfReader.GetOutlineTree())
	if err := c.WriteToFile(saveParams.markupOutputPath); err != nil {
		return fmt.Errorf("WriteToFile failed. err=%w", err)
	}

	common.Log.Info("Saved marked-up PDF file: %q", saveParams.markupOutputPath)
	return nil
}

var (
	widths = map[string]float64{
		"marks":   0.4,
		"words":   0.3,
		"lines":   0.2,
		"divs":    0.6,
		"gaps":    0.9,
		"columns": 1.0,
	}
	colors = map[string]string{
		"marks":   "#0000ff",
		"words":   "#00ff00",
		"lines":   "#ff0000",
		"divs":    "#ffff00",
		"gaps":    "#0000ff",
		"columns": "#f0f000",
	}
	bkgnds = map[string]string{
		"marks":   "#ffff00",
		"words":   "#ff00ff",
		"lines":   "#00afaf",
		"divs":    "#0000ff",
		"gaps":    "#ffff00",
		"columns": "#000077",
	}
)

func markupKeys(markups map[string][]model.PdfRectangle) []string {
	var keys []string
	for markupType := range markups {
		keys = append(keys, markupType)
	}
	sort.Slice(keys, func(i, j int) bool {
		ki, kj := keys[i], keys[j]
		wi, wj := widths[ki], widths[kj]
		if wi != wj {
			return wi >= wj
		}
		return ki < kj
	})
	return keys
}

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
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

// changePath inserts `insertion` into `filename` before suffix `ext`.
func changePath(filename, insertion, ext string) string {
	filename = strings.TrimSuffix(filename, ext)
	return fmt.Sprintf("%s.%s%s", filename, insertion, ext)
}
