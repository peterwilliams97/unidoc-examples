/*
 * Split columns: Example illustrating capability to extract TextMarks from PDF, and group together
 * into words, rows and columnn.
 *
 * Includes debugging capabilities such as outputing a marked up PDF showing bounding boxes of marks,
 * words, lines and columns.
 *
 * Run as: go run split_columns.go -m all -mf markup.pdf table.pdf
 * - Outputs debug markup including: marks, words, lines, columns to markup.pdf
 *
 * References
 * https://www.dfki.de/fileadmin/user_upload/import/2000_HighPerfDocLayoutAna.pdf
 * https://www.researchgate.net/publication/265186943_Layout_Analysis_based_on_Text_Line_Segment_Hypotheses
 */

package main

import (
	"bytes"
	"container/heap"
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
	companyName = ""
)

var saveParams saveMarkedupParams

func main() {
	var (
		loglevel   string
		saveMarkup string
		markupPath string
		firstPage  int
		lastPage   int
	)
	flag.StringVar(&loglevel, "L", "info", "Set log level (default: info)")
	flag.StringVar(&saveMarkup, "m", "columns", "Save markup (none/marks/words/lines/columns/all)")
	flag.StringVar(&markupPath, "mf", "./layout.pdf", "Output markup path (default /tmp/markup.pdf)")
	flag.IntVar(&firstPage, "f", -1, "First page")
	flag.IntVar(&lastPage, "l", 100000, "Last page")
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
	err := extractColumnText(inPath, outPath, firstPage, lastPage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "markupOutputPath=%q\n", saveParams.markupOutputPath)
	fmt.Fprintf(os.Stderr, "shownMarkups=%q\n", saveParams.shownMarkups)
}

// extractColumnText extracts text columns from PDF file `inPath` and outputs the data as a text
// file to `outPath`.
func extractColumnText(inPath, outPath string, firstPage, lastPage int) error {
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
	saveParams.markups = map[int]map[string]rectList{}

	var pageTexts []string

	if firstPage < 1 {
		firstPage = 1
	}
	if lastPage > numPages {
		lastPage = numPages
	}

	for pageNum := firstPage; pageNum <= lastPage; pageNum++ {
		saveParams.curPage = pageNum
		saveParams.markups[pageNum] = map[string]rectList{}

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
		words := pageText.Words()
		textMarks := pageText.Marks()
		common.Log.Info("-------------------------------------------------------")
		common.Log.Info("pageNum=%d text=%d textMarks=%d", pageNum, len(text), textMarks.Len())

		common.Log.Info("%d words", len(words))
		// for i, w := range words {
		// 	b, _ := w.BBox()
		// 	fmt.Printf("%4d: %s %q\n", i, showBBox(b), w.Text())
		// 	for j, m := range w.Elements() {
		// 		b := m.BBox
		// 		fmt.Printf("%8d: %s %q\n", j, showBBox(b), m.Text)
		// 	}
		// }

		group := make(rectList, textMarks.Len())
		for i, mark := range textMarks.Elements() {
			group[i] = mark.BBox
		}
		saveParams.markups[pageNum]["marks"] = group

		outPageText, err := pageMarksToColumnText(pageNum, words, *mbox)
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
func pageMarksToColumnText(pageNum int, words []extractor.TextMarkArray, pageBound model.PdfRectangle) (
	string, error) {

	// Include the words in the markup.
	{
		var wbboxes rectList
		for _, word := range words {
			wbbox, ok := word.BBox()
			if !ok {
				continue
			}
			wbboxes = append(wbboxes, wbbox)
		}
		saveParams.markups[saveParams.curPage]["words"] = wbboxes
	}

	// gapSize := charMultiplier * averageWidth(textMarks)
	// common.Log.Info("gapSize=%.1f = %1.f mm charMultiplier=%.1f averageWidth(textMarks)=%.1f",
	// 	gapSize, gapSize/72.0*25.4, charMultiplier, averageWidth(textMarks))

	pageBound, pageGaps := whitespaceCover(pageBound, words)
	saveParams.markups[pageNum]["page"] = rectList{pageBound}

	common.Log.Info("%d pageGaps~~~~~~~~~~~~~~~~~~~ ", len(pageGaps))
	var bigBBoxes rectList
	for _, bbox := range pageGaps {
		if bbox.Height() < 20 {
			continue
		}
		bigBBoxes = append(bigBBoxes, bbox)
	}
	common.Log.Info("%d big columns~~~~~~~~~~~~~~~~~~~ ", len(bigBBoxes))
	pageGaps = bigBBoxes

	for i, bbox := range pageGaps {
		common.Log.Info("%4d of %d: %s", i+1, len(pageGaps), showBBox(bbox))
	}

	saveParams.markups[saveParams.curPage]["gaps"] = pageGaps

	columns := scanPage(pageBound, pageGaps)

	saveParams.markups[saveParams.curPage]["columns"] = columns

	// columnText := getColumnText(lines, columnBBoxes)
	// return strings.Join(columnText, "\n####\n"), nil
	return "FIX ME", nil
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
func coallesceGaps(pageGaps []division, gapWidth float64, gapHeight int) rectList {
	var gaps rectList
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

	var bigGaps rectList
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

	simpleGaps := rectList{bigGaps[0]}
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

func lineBBox(line []extractor.TextMarkArray) model.PdfRectangle {
	bboxes := wordBBoxes(line)
	return rectList(bboxes).union()
}

func wordBBoxes(words []extractor.TextMarkArray) rectList {
	bboxes := make(rectList, 0, len(words))
	for _, w := range words {
		b, ok := w.BBox()
		if !ok {
			panic("bbox")
		}
		bboxes = append(bboxes, b)
	}
	return bboxes
}

func wordBBoxMap(words []extractor.TextMarkArray) map[float64]extractor.TextMarkArray {
	sigWord := make(map[float64]extractor.TextMarkArray, len(words))
	for _, w := range words {
		b, ok := w.BBox()
		if !ok {
			panic("bbox")
		}
		sig := partEltSig(b)
		sigWord[sig] = w
	}
	return sigWord
}

func bboxWords(sigWord map[float64]extractor.TextMarkArray, bboxes rectList) []extractor.TextMarkArray {
	words := make([]extractor.TextMarkArray, len(bboxes))
	for i, b := range bboxes {
		sig := partEltSig(b)
		w, ok := sigWord[sig]
		if !ok {
			panic(fmt.Errorf("signature: b=%s", showBBox(b)))
		}
		words[i] = w
	}
	return words
}

func mergeXBboxes(bboxes rectList) rectList {
	merged := make(rectList, 0, len(bboxes))
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
	return overlappedX01(r0, r1) || overlappedX01(r1, r0)
}

func overlappedX01(r0, r1 model.PdfRectangle) bool {
	return (r0.Llx <= r1.Llx && r1.Llx <= r0.Urx) || (r0.Llx <= r1.Urx && r1.Urx <= r0.Urx)
}

type scanState struct {
	pageBound model.PdfRectangle
	running   []idRect      // must be sorted left to right
	gapStack  map[int][]int // {gap id: columns that gap intersects}
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

func (sl scanLine) toRectList() rectList {
	rl := make(rectList, len(sl.events))
	for i, e := range sl.events {
		rl[i] = e.PdfRectangle
	}
	return rl
}

func (sl scanLine) checkOverlaps() {
	rl := sl.toRectList()
	rl.checkOverlaps()
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
	if !validBBox(idr.PdfRectangle) {
		panic(fmt.Errorf("idr.validate rect %s", idr))
	}
	if idr.id <= 0 {
		panic(fmt.Errorf("validate id %s", idr))
	}
}

// scanPage returns the rectangles in `pageBound` that are separated by `pageGaps`.
func scanPage(pageBound model.PdfRectangle, pageGaps rectList) rectList {
	if len(pageGaps) == 0 {
		return rectList{pageBound}
	}
	ss := newScanState(pageBound)
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
		gapStack:  map[int][]int{},
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

func (ss *scanState) open(sl scanLine) {
	// save current columns that gaps intersect
	// intersect columns with inverse of gaps
	// create new columns
	common.Log.Info("sl.opened()=%s", sl.opened())
	if len(sl.opened()) == 0 {
		return
	}
	running := ss.intersectingElements(ss.running, sl.opened(), sl.y)
	closed := differentElements(ss.running, running)
	common.Log.Info("\n\tss.running=%s\n\t   running=%s\n\t    closed=%s", ss.running, running, closed)
	for _, idr := range closed {
		idr.validate()
		if sl.y <= idr.Ury {
			continue
		}
		idr.Lly = sl.y
		idr.validate()
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
	closed := differentElements(ss.running, oldRunning)
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

// differentElements returns the elements in `a` that aren't in `b`.
func differentElements(a, b []idRect) []idRect {
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

// intersectingElements returns the intesection of `columns` and `gaps` along the x-axis at y=`y`.
func (ss *scanState) intersectingElements(columns, gaps []idRect, y float64) []idRect {
	// {
	// 	g0 := gaps[0]
	// 	for _, g := range gaps[1:] {
	// 		if g.Llx <= g0.Urx {
	// 			panic(fmt.Errorf("overlapping\n\tg0=%s\n\t g=%s", g0, g))
	// 		}
	// 		g0 = g
	// 	}
	// }
	for _, g := range gaps {
		for _, c := range columns {
			if overlappedX(c.PdfRectangle, g.PdfRectangle) {
				ss.gapStack[g.id] = append(ss.gapStack[g.id], c.id)
			}
		}
	}
	var columns1 []idRect
	for i, c := range columns {
		olap := overlappedXElements(c, gaps)
		if len(olap) == 0 {
			idr := ss.newIDRect(c.PdfRectangle)
			columns1 = append(columns1, idr)
			common.Log.Info("columns1=%d idr=%s", len(columns1), idr)
			continue
		}
		common.Log.Info("## %3d of %d: %s olap=%d %s", i, len(columns), c, len(olap), olap[0])
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
		for j, g := range olap {
			common.Log.Info("#@ %3d of %d: %s", j, len(olap), g)
			x1 := g.Llx
			if x1 <= x0 {
				continue // overlap
				panic(fmt.Errorf("x0=%.1f x1=%.1f", x0, x1))
			}
			r := model.PdfRectangle{Llx: x0, Urx: x1, Ury: y}
			idr := ss.newIDRect(r)
			columns1 = append(columns1, idr)
			common.Log.Debug("columns1=%d idr=%s", len(columns1), idr)
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
	sl.checkOverlaps()
	for i, e := range events[1:] {
		common.Log.Info("! %2d of %d: %s", i+1, len(events), e)
		if e.y() > sl.y-1.0 {
			sl.events = append(sl.events, e)
			// sl.checkOverlaps()
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
	opened.checkOverlaps()
	closed.checkOverlaps()
	return opened, closed
}

func (sl scanLine) opened() []idRect {
	var idrs []idRect
	for _, e := range sl.events {
		if e.enter {
			idrs = append(idrs, e.idRect)
		}
	}
	// checkOverlaps(idrs)
	return idrs
}

func (sl scanLine) closed() []idRect {
	var idrs []idRect
	for _, e := range sl.events {
		if !e.enter {
			idrs = append(idrs, e.idRect)
		}
	}
	// checkOverlaps(idrs)
	return idrs
}

func (idr idRect) String() string {
	return fmt.Sprintf("(%4d %s)", idr.id, showBBox(idr.PdfRectangle))
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
	gaps   rectList
	text   string
}

func pageDivsToRects(divs []division) rectList {
	var rects rectList
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

// bboxCenter returns coordinates the center of `bbox`.
func bboxCenter(bbox model.PdfRectangle) (float64, float64) {
	cx := (bbox.Llx + bbox.Urx) / 2.0
	cy := (bbox.Lly + bbox.Ury) / 2.0
	// common.Log.Info("&&&& %5.1f -> %5.1f %5.1f", bbox, cx, cy)
	return cx, cy
}

// bboxPerim returns the half perimeter of `bbox`.
func bboxPerim(bbox model.PdfRectangle) float64 {
	return bbox.Width() + bbox.Height()
}

// bboxWidth returns the width of `bbox`.
func bboxWidth(bbox model.PdfRectangle) float64 {
	return bbox.Width()
	return math.Abs(bbox.Urx - bbox.Llx)
}

// bboxHeight returns the height of `bbox`.
func bboxHeight(bbox model.PdfRectangle) float64 {
	return bbox.Height()
	return math.Abs(bbox.Ury - bbox.Lly)
}

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
	markups          map[int]map[string]rectList
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

		params.shownMarkups["page"] = struct{}{}

		if err := c.AddPage(page); err != nil {
			return fmt.Errorf("AddPage failed err=%w", err)
		}

		for _, markupType := range markupKeys(params.markups[pageNum]) {
			group := params.markups[pageNum][markupType]
			if _, ok := params.shownMarkups[markupType]; !ok {
				continue
			}

			dx := 10.0
			dy := 10.0
			if markupType == "marks" || len(params.shownMarkups) <= 2 {
				dx = 0.0
				dy = 0.0
			}

			common.Log.Info("markupType=%q dx=%.1f dy=%.1f markups[%d]=%d",
				markupType, dx, dy, pageNum, len(params.shownMarkups))

			width := widths[markupType]
			borderColor := creator.ColorRGBFromHex(colors[markupType])
			bgdColor := creator.ColorRGBFromHex(bkgnds[markupType])
			common.Log.Debug("borderColor=%+q %.2f", colors[markupType], borderColor)
			common.Log.Debug("   bgdColor=%+q %.2f", bkgnds[markupType], bgdColor)
			for i, r := range group {
				common.Log.Debug("Mark %d: %5.1f x,y,w,h=%5.1f %5.1f %5.1f %5.1f", i+1, r,
					r.Llx, h-r.Lly, r.Urx-r.Llx, -(r.Ury - r.Lly))

				if r.Urx-r.Llx < 20.0*dx {
					dx = (r.Urx - r.Llx) / 20.0
				}
				if r.Ury-r.Lly < 20.0*dy {
					dy = (r.Ury - r.Lly) / 20.0
				}
				if dx < 0 || dy < 0 {
					panic("dx dy ")
				}

				llx := r.Llx + dx
				urx := r.Urx - dx
				lly := r.Lly + dy
				ury := r.Ury - dy

				rect := c.NewRectangle(llx, h-lly, urx-llx, -(ury - lly))
				rect.SetBorderColor(bgdColor)
				rect.SetBorderWidth(2.0 * width)
				err = c.Draw(rect)
				if err != nil {
					return fmt.Errorf("Draw failed (background). pageNum=%d err=%w", pageNum, err)
				}
				rect = c.NewRectangle(llx, h-lly, urx-llx, -(ury - lly))
				rect.SetBorderColor(borderColor)
				rect.SetBorderWidth(1.0 * width)
				err = c.Draw(rect)
				if err != nil {
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
		"marks":   0.05,
		"words":   0.1,
		"lines":   0.2,
		"divs":    0.6,
		"gaps":    0.9,
		"columns": 0.8,
		"page":    1.1,
	}
	colors = map[string]string{
		"marks":   "#0000ff",
		"words":   "#ff0000",
		"lines":   "#f0f000",
		"divs":    "#ffff00",
		"gaps":    "#0000ff",
		"columns": "#00ff00",
		"page":    "#00aabb",
	}
	bkgnds = map[string]string{
		"marks":   "#ffff00",
		"words":   "#ff00ff",
		"lines":   "#00afaf",
		"divs":    "#0000ff",
		"gaps":    "#ffff00",
		"columns": "#ff00ff",
		"page":    "#ff0000",
	}
)

func markupKeys(markups map[string]rectList) []string {
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
	common.Log.Info("keys=%q", keys)
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

// whitespaceCover returns a best-effort maximum rectangle cover of the part of `pageBound` that
// excludes the bounding boxes of `textMarks`
func whitespaceCover(pageBound model.PdfRectangle, words []extractor.TextMarkArray) (
	model.PdfRectangle, rectList) {
	maxboxes := 20
	maxoverlap := 0.01
	maxperim := pageBound.Width() + pageBound.Height()*0.05
	frac := 0.01
	maxpops := 20000

	obstacles := wordBBoxes(words)
	sigObstacles = wordBBoxMap(words)
	bound := pageBound
	{
		envelope := obstacles.union()
		contraction, _ := geometricIntersection(bound, envelope)
		// contraction.Llx += 100
		// contraction.Urx -= 100
		common.Log.Info("contraction\n\t   bound=%s\n\tenvelope=%s\n\tcontract=%s",
			showBBox(bound), showBBox(envelope), showBBox(contraction))
		bound = contraction
	}
	return bound, obstacleCover(bound, obstacles, maxboxes, maxoverlap, maxperim, frac, maxpops)
}

var sigObstacles map[float64]extractor.TextMarkArray

// obstacleCover returns a best-effort maximum rectangle cover of the part of `bound` that
// excludes  `obstacles`.
// Based on "wo Geometric Algorithms for Layout Analysis" by Thomas Breuel
// https://www.researchgate.net/publication/2504221_Two_Geometric_Algorithms_for_Layout_Analysis
func obstacleCover(bound model.PdfRectangle, obstacles rectList,
	maxboxes int, maxoverlap, maxperim, frac float64, maxpops int) rectList {
	common.Log.Info("whitespaceCover: bound=%5.1f obstacles=%d maxboxes=%d\n"+
		"\tmaxoverlap=%g maxperim=%g frac=%g maxpops=%d",
		bound, len(obstacles), maxboxes,
		maxoverlap, maxperim, frac, maxpops)
	if len(obstacles) == 0 {
		return nil
	}
	pq := newPriorityQueue()
	partel := newPartElt(bound, obstacles)
	pq.myPush(partel)
	var cover rectList

	// var snaps []string
	for cnt := 0; pq.Len() > 0; cnt++ {
		partel := pq.myPop()
		common.Log.Info("npush=%3d npop=%3d cover=%3d cnt=%3d\n\tpartel=%s\n\t    pq=%s",
			pq.npush, pq.npop, len(cover), cnt, partel.String(), pq.String())

		if cnt > 100000 {
			panic("cnt")
		}
		// snaps = append(snaps, pq.String())

		if pq.npop > maxpops {
			common.Log.Info("npop > maxpops npop=%d maxpops=%d", pq.npop, maxpops)
			break
		}

		// Extract the contents

		// Got an empty rectangle?
		if len(partel.obstacles) == 0 {
			if !intersectionSignificant(partel.bound, cover, maxoverlap) {
				partel = partel.extend(bound, obstacles)
				cover = append(cover, partel.bound)
				common.Log.Info("ADDING cover=%d bound=%5.1f", len(cover), partel.bound)
			}
			if len(cover) >= maxboxes { // we're done
				break
			}
			continue
		}

		// Generate up to 4 subdivisions and put them on the heap
		subdivisions := subdivide(partel.bound, append(partel.obstacles, cover...), maxperim, frac)
		for _, subbound := range subdivisions {
			subobstacles := partel.obstacles.intersects(subbound)
			partel := newPartElt(subbound, subobstacles)
			if !accept(partel.bound) {
				continue
			}
			pq.myPush(partel)
		}
	}

	// common.Log.Info("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	// for i, s := range snaps {
	// 	fmt.Printf("%6d: %s\n", i, s)
	// }
	cover = removeNonSeparating(bound, cover, obstacles)
	cover = absorbCover(bound, cover, obstacles)
	return cover
}

// absorbCover removes adjacent gaps (elements of `cover`) which have no intervening text.
// It removes shorter gaps first.
func absorbCover(bound model.PdfRectangle, cover, obstacles rectList) rectList {
	byHeight := make([]int, len(cover))
	for i := 0; i < len(byHeight); i++ {
		byHeight[i] = i
	}
	sort.SliceStable(cover, func(i, j int) bool {
		oi, oj := cover[i], cover[j]
		xi, xj := oi.Llx, oj.Llx
		if xi != xj {
			return xi < xj
		}
		return oi.Lly > oj.Lly
	})
	sort.Slice(byHeight, func(i, j int) bool {
		oi, oj := cover[byHeight[i]], cover[byHeight[j]]
		hi, hj := oi.Height(), oj.Height()
		if hi != hj {
			return hi < hj
		}
		wi, wj := oi.Width(), oj.Width()
		if wi != wj {
			return wi < wj
		}
		return i < j
	})
	// common.Log.Info("byHeight=%v", byHeight)
	// common.Log.Info("cover-------------")
	// for i, r := range cover {
	// 	fmt.Printf("%3d: %s\n", i, showBBox(r))
	// }
	// common.Log.Info("byHeight-------------")
	// for i, i0 := range byHeight {
	// 	r := cover[i0]
	// 	fmt.Printf("%3d: %3d: %s\n", i, i0, showBBox(r))
	// }

	absorbed := map[int]struct{}{}
	for i := range cover {
		if absorbedBy(cover, obstacles, i, absorbed) {
			absorbed[i] = struct{}{}
		}
	}

	var reduced rectList
	for i, i0 := range byHeight {
		r := cover[i0]
		_, ok := absorbed[i0]
		if !ok {
			reduced = append(reduced, r)
		}
		fmt.Printf("%3d: %3d: %s %t\n", i, i0, showBBox(r), ok)
	}
	common.Log.Info("absorbCover: %d -> %d", len(cover), len(reduced))
	return reduced
}

// absorbedBy returns true if `cover`[`i0`] has no intervening `obstacles` with at least one other
// element of `cover`. `absorbed` are the indexes of previously removed elements of cover.
func absorbedBy(cover, obstacles rectList, i0 int, absorbed map[int]struct{}) bool {
	r0 := cover[i0]

	for i := i0 + 1; i < len(cover); i++ {
		if _, ok := absorbed[i]; ok {
			continue
		}
		r := cover[i]
		if r.Lly <= r0.Lly && r.Ury >= r0.Ury {
			v := r0
			v.Urx = r.Llx
			v.Ury -= 2 // To exclude tiny overlaps
			v.Lly += 2 // To exclude tiny overlaps
			overl := wordCount(v, obstacles)
			if len(overl) == 0 {
				common.Log.Info("-absorbed v=%s\n\t%s %d by\n\t%s %d",
					showBBox(v), showBBox(r0), i0, showBBox(r), i)
				return true
			}
		}
	}
	for i := i0 - 1; i >= 0; i-- {
		if _, ok := absorbed[i]; ok {
			continue
		}
		r := cover[i]
		if r.Lly <= r0.Lly && r.Ury >= r0.Ury {
			v := r0
			v.Llx = r.Urx
			v.Ury -= 2 // To exclude tiny overlaps
			v.Lly += 2 // To exclude tiny overlaps
			overl := wordCount(v, obstacles)
			if len(overl) == 0 {
				common.Log.Info("+absorbed v=%s\n\t%s %d by\n\t%s %d",
					showBBox(v), showBBox(r0), i0, showBBox(r), i)
				return true
			}
		}
	}
	return false
}

const searchWidth = 60

// removeNonSeparating returns `cover` stripped of elements that don't separate elements of `obstacles`.
func removeNonSeparating(bound model.PdfRectangle, cover, obstacles rectList) rectList {
	reduced := make(rectList, 0, len(cover))
	for _, r := range cover {
		if separatingRect(r, searchWidth, obstacles) {
			reduced = append(reduced, r)
		}
	}
	common.Log.Info("removeNonSeparating: %d -> %d", len(cover), len(reduced))
	return reduced
}

// separatingRect returns true if `r` separates sufficient elements of `obstacles` (bounding boxes
// of words). We search `width` to left and right of `r` for these elements.
func separatingRect(r model.PdfRectangle, width float64, obstacles rectList) bool {
	expansion := r
	expansion.Llx -= width
	expansion.Urx += width
	overl := wordCount(expansion, obstacles)
	// words := bboxWords(sigObstacles, obstacles)
	words := bboxWords(sigObstacles, overl)
	var texts []string
	for _, w := range words {
		texts = append(texts, w.Text())
	}
	dy := yRange(overl)
	common.Log.Info("r=%s dy=%.1f count=%d %v", showBBox(r), dy, len(overl), texts)
	return len(overl) > 0 && dy > width
}

func wordCount(bound model.PdfRectangle, obstacles rectList) rectList {
	overl := make(rectList, 0, len(obstacles))
	for _, r := range obstacles {
		if intersects(bound, r) {
			overl = append(overl, r)
		}
	}
	return overl
}

func yRange(obstacles rectList) float64 {
	if len(obstacles) == 0 {
		return 0
	}

	min := obstacles[0].Lly
	max := obstacles[0].Lly
	for _, r := range obstacles[1:] {
		if r.Lly < min {
			min = r.Lly
		}
		if r.Lly > max {
			r.Lly = max
		}
	}
	return max - min
}

func accept(bound model.PdfRectangle) bool {
	// return math.Max(bound.Height(), bound.Width()) > 40.0
	if bound.Height() > 30.0 && bound.Width() > 10.0 {
		return true
	}
	if bound.Height() > 5.0 && bound.Width() > 50.0 {
		return true
	}
	return false
}

func partEltQuality(r model.PdfRectangle) float64 {
	x := 0.1*r.Height() + r.Width()
	y := r.Height() + 0.1*r.Width()
	return math.Max(0.5*x, y)
}

func partEltSig(r model.PdfRectangle) float64 {
	return r.Llx + r.Urx*1e3 + r.Lly*1e6 + r.Ury*1e9
}

// subdivide subdivides `bound` into up to 4 rectangles that don't intersect with `obstacles`.
func subdivide(bound model.PdfRectangle, obstacles rectList, maxperim, frac float64) rectList {
	subdivisions := make(rectList, 0, 4)
	pivot, err := selectPivot(bound, obstacles, maxperim, frac)
	if err != nil {
		panic(err)
	}
	if !validBBox(pivot) {
		panic(fmt.Errorf("bad pivot=%5.1f", pivot))
	}

	pivotInt, ok := geometricIntersection(bound, pivot)
	if !ok {
		return nil
	}
	pivot = pivotInt

	var parts []string
	if pivot.Llx > bound.Llx && !same(bound.Urx, pivot.Llx) { // left sub-bound
		quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: bound.Lly, Urx: pivot.Llx, Ury: bound.Ury}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "left")
	} else {
		u := obstacles.union()
		if bound.Llx < u.Llx {
			quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: bound.Lly, Urx: u.Llx, Ury: bound.Ury}
			subdivisions = append(subdivisions, quadrant)
			parts = append(parts, "left*")
		}
	}
	if pivot.Urx < bound.Urx && !same(bound.Llx, pivot.Urx) { // right sub-bound
		quadrant := model.PdfRectangle{Llx: pivot.Urx, Lly: bound.Lly, Urx: bound.Urx, Ury: bound.Ury}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "right")
	} else {
		u := obstacles.union()
		if bound.Urx > u.Urx {
			quadrant := model.PdfRectangle{Llx: u.Urx, Lly: bound.Lly, Urx: bound.Urx, Ury: bound.Ury}
			subdivisions = append(subdivisions, quadrant)
			parts = append(parts, "right*")
		}
	}
	if pivot.Ury < bound.Ury && !same(bound.Lly, pivot.Ury) { // top sub-bound
		quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: pivot.Ury, Urx: bound.Urx, Ury: bound.Ury}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "top")
	}
	if pivot.Lly > bound.Lly && !same(bound.Ury, pivot.Lly) { // bottom sub-bound
		quadrant := model.PdfRectangle{Llx: bound.Llx, Lly: bound.Lly, Urx: bound.Urx, Ury: pivot.Lly}
		subdivisions = append(subdivisions, quadrant)
		parts = append(parts, "bottom")
	}

	common.Log.Info("subdivide parts=%s\n\tbound=%s\n\tpivot=%5.1f -->",
		parts, showBBox(bound), pivot)
	for i, quadrant := range subdivisions {
		fmt.Printf("\t%5d=%s\n", i, showBBox(quadrant))
		if !validBBox(quadrant) {
			panic("quadrant")
		}
	}
	return subdivisions
}

// selectPivot returns an element of `obstacles` close to the center of `bound`.
func selectPivot(bound model.PdfRectangle, obstacles rectList, maxperim, frac float64) (
	model.PdfRectangle, error) {
	if !validBBox(bound) {
		panic(fmt.Errorf("selectPivot: bound=%s", showBBox(bound)))
	}
	if len(obstacles) == 0 {
		return model.PdfRectangle{}, fmt.Errorf("no boxes in obstacles")
	}
	if frac < 0.0 || frac > 1.0 {
		common.Log.Error("frac=%g out of bound; using 0.0", frac)
		frac = 0.0
	}

	w := bound.Width()
	h := bound.Height()
	x, y := bboxCenter(bound)
	threshdist := frac * math.Sqrt(w*w+h*h)
	mindist := 1000000000.0
	minindex := 0
	smallfound := false
	for i, r := range obstacles {
		if bboxPerim(r) > maxperim {
			continue
		}
		smallfound = true

		cx, cy := bboxCenter(r)
		delx := cx - x
		dely := cy - y
		dist := delx*delx + dely*dely
		if dist <= threshdist {
			return r, nil
		}
		if dist < mindist {
			minindex = i
			mindist = dist
		}
	}

	// If there are small boxes but none are within 'frac' of the centroid, return the nearest one.
	if smallfound {
		return obstacles[minindex], nil
	}

	// No small boxes; return the smallest of the large boxes
	minsize := 1000000000.0
	minindex = 0
	for i, r := range obstacles {
		perim := bboxPerim(r)
		if perim < minsize {
			minsize = perim
			minindex = i
		}
	}
	return obstacles[minindex], nil
}

func newPartElt(bound model.PdfRectangle, obstacles rectList) *partElt {
	if !validBBox(bound) {
		panic(fmt.Errorf("bound=%s", showBBox(bound)))
	}
	return &partElt{
		bound:     bound,
		obstacles: obstacles,
		quality:   partEltQuality(bound),
		sig:       partEltSig(bound),
	}
}

type partElt struct {
	quality   float64 // sorting key
	sig       float64
	bound     model.PdfRectangle // region of the element
	obstacles rectList           // set of intersecting boxes
}

func (partel *partElt) extend(bound model.PdfRectangle, obstacles rectList) *partElt {
	if len(partel.obstacles) != 0 {
		panic(fmt.Errorf("not empty: %s", partel))
	}
	bnd := partel.bound

	common.Log.Info("extend bound=%s", showBBox(bnd))

	w := bnd.Width() / 4
	bnd.Llx += 2 * w
	bnd.Urx -= w

	bnd.Ury = bound.Ury
	obs := obstacles.intersects(bnd)
	if len(obs) > 0 {
		bnd.Ury = obs.union().Lly
		// words := bboxWords(sigObstacles, obs)
		// common.Log.Info("Upward extension %d bnd=%s", len(words), showBBox(bnd))
		// for i, w := range words {
		// 	b, _ := w.BBox()
		// 	fmt.Printf("%4d: %5.1f %s\n", i, b, w.Text())
		// }
	}

	bnd.Lly = bound.Lly
	obs = obstacles.intersects(bnd)
	if len(obs) > 0 {
		bnd.Lly = obs.union().Ury
		// words := bboxWords(sigObstacles, obs)
		// common.Log.Info("Downward extension %d bnd=%s", len(words), showBBox(bnd))
		// for i, w := range words {
		// 	b, _ := w.BBox()
		// 	fmt.Printf("%4d: %5.1f %s\n", i, b, w.Text())
		// }
	}

	// bnd.Urx = bound.Urx
	// obs = obstacles.intersects(bnd)
	// if len(obs) > 0 {
	// 	bnd.Urx = obs.union().Llx
	// }

	// bnd.Llx = bound.Llx
	// obs = obstacles.intersects(bnd)
	// if len(obs) > 0 {
	// 	bnd.Llx = obs.union().Urx
	// }

	pe := newPartElt(bnd, obstacles.intersects(bnd))
	common.Log.Info("extend:\n\t%s->\n\t%s", partel, pe)
	return pe
}

func (partel *partElt) String() string {
	extra := ""
	if len(partel.obstacles) == 0 {
		extra = " LEAF!"
	}
	return fmt.Sprintf("<%d %s%s>", len(partel.obstacles), showBBox(partel.bound), extra)
}

// newPriorityQueue returns a PriorityQueue containing `items`.
func newPriorityQueue() *PriorityQueue {
	var pq PriorityQueue
	heap.Init(&pq)
	return &pq
}

// PriorityQueue implements heap.Interface and holds partElt.
type PriorityQueue struct {
	npop  int
	npush int
	elems []*partElt
}

func (pq *PriorityQueue) String() string {
	parts := make([]string, 0, len(pq.elems))
	var lines []string
	var save []*partElt
	for pq.Len() > 0 {
		pe := pq._myPop()
		save = append(save, pe)
		leaf := " "
		if len(pe.obstacles) == 0 && len(lines) < 5 {
			leaf = " LEAF!"
			lines = append(lines, fmt.Sprintf("\n\t%5.1f %5.1f", pe.quality, pe.bound))
		}
		if len(parts) >= 5 && leaf != " LEAF!" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%.1f %d%s", pe.quality, len(pe.obstacles), leaf))
	}
	for _, pe := range save {
		pq._myPush(pe)
	}
	return fmt.Sprintf("{PQ %d: %s}", pq.Len(), strings.Join(parts, ", "))
}

func (pq PriorityQueue) Len() int { return len(pq.elems) }

func (pq PriorityQueue) Less(i, j int) bool { return pq.elems[i].quality > pq.elems[j].quality }

func (pq PriorityQueue) Swap(i, j int) { pq.elems[i], pq.elems[j] = pq.elems[j], pq.elems[i] }

func (pq *PriorityQueue) Push(x interface{}) {
	partel := x.(*partElt)
	pq.elems = append(pq.elems, partel)
}

func (pq *PriorityQueue) myPush(partel *partElt) {
	for _, pe := range pq.elems {
		if pe.sig == partel.sig {
			err := fmt.Errorf("duplicate:\n\tpartel=%s\n\t    pe=%s", partel, pe)
			common.Log.Error("myPush %v", err)
			return
		}
	}
	pq.npush++
	pq._myPush(partel)
}

func (pq *PriorityQueue) _myPush(partel *partElt) {
	heap.Push(pq, partel)
}

func (pq *PriorityQueue) myPop() *partElt {
	pq.npop++
	return pq._myPop()
}

func (pq *PriorityQueue) _myPop() *partElt {
	return heap.Pop(pq).(*partElt)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := pq.elems
	n := len(old)
	partel := old[n-1]
	old[n-1] = nil // avoid memory leak
	pq.elems = old[:n-1]
	return partel
}

type rectList []model.PdfRectangle

func (rl rectList) checkOverlaps() {
	if len(rl) == 0 {
		return
	}
	r0 := rl[0]
	for _, r := range rl[1:] {
		if r.Llx <= r0.Urx {
			panic(fmt.Errorf("checkOverlaps:\n\tr0=%s\n\t r=%s", showBBox(r0), showBBox(r)))
		}
	}
}

func checkOverlaps(rl []idRect) {
	if len(rl) == 0 {
		return
	}
	r0 := rl[0]
	for _, r := range rl[1:] {
		if r.Llx <= r0.Urx {
			panic(fmt.Errorf("checkOverlaps:\n\tr0=%s\n\t r=%s", r0, r))
		}
	}
}

func (rl rectList) union() model.PdfRectangle {
	var u model.PdfRectangle
	if len(rl) == 0 {
		return u
	}
	u = rl[0]
	for _, r := range rl[1:] {
		u = rectUnion(u, r)
	}
	return u
}

// intersects returns the elements of `rl` that intersect `bound`.
func (rl rectList) intersects(bound model.PdfRectangle) rectList {
	if len(rl) == 0 || !validBBox(bound) {
		panic("intersects n==0")
		return nil
	}

	var intersecting rectList
	for _, r := range rl {
		if !validBBox(r) {
			continue
		}
		if intersects(bound, r) {
			intersecting = append(intersecting, r)
		}
	}
	return intersecting
}

// intersectionSignificant returns true if `bound` has a significant (> maxoverlap) fractional
// intersection with any rectangle in `cover`.
func intersectionSignificant(bound model.PdfRectangle, cover rectList, maxoverlap float64) bool {
	if len(cover) == 0 || maxoverlap == 1.0 {
		return false
	}
	overlap := -1.0
	besti := -1
	for i, r := range cover {
		olap := intersectionFraction(r, bound)
		if olap > overlap {
			overlap = olap
			besti = i
		}
	}
	common.Log.Info("bestOverlap: overlap=%.3f bound=%.1f cover[%d]=%.1f",
		overlap, bound, besti, cover[besti])

	for _, r := range cover {
		if intersectionFraction(r, bound) > maxoverlap {
			return true
		}
	}
	return false
}

// intersectionFraction the ratio of area of intersecton of `r0` and `r1` and the area of `r1`.
func intersectionFraction(r0, r1 model.PdfRectangle) float64 {
	if !(validBBox(r0) && validBBox(r1)) {
		common.Log.Error("boxes not both valid r0=%+r r1=%+r", r0, r1)
		return 0
	}
	r, overl := geometricIntersection(r0, r1)
	if !overl {
		return 0
	}
	return bboxArea(r) / bboxArea(r1)
}

// geometricIntersection returns a rectangle that is the geomteric intersection of `r0` and `r1`.
func geometricIntersection(r0, r1 model.PdfRectangle) (model.PdfRectangle, bool) {
	if !intersects(r0, r1) {
		return model.PdfRectangle{}, false
	}
	return model.PdfRectangle{
		Llx: math.Max(r0.Llx, r1.Llx),
		Urx: math.Min(r0.Urx, r1.Urx),
		Lly: math.Max(r0.Lly, r1.Lly),
		Ury: math.Min(r0.Ury, r1.Ury),
	}, true
}

// horizontalIntersection returns a rectangle that is the horizontal intersection and vertical union
// of `r0` and `r1`.
func horizontalIntersection(r0, r1 model.PdfRectangle) model.PdfRectangle {
	r := model.PdfRectangle{
		Llx: math.Max(r0.Llx, r1.Llx),
		Urx: math.Min(r0.Urx, r1.Urx),
		Lly: math.Min(r0.Lly, r1.Lly),
		Ury: math.Max(r0.Ury, r1.Ury),
	}
	if r.Llx >= r.Urx || r.Lly >= r.Ury {
		return model.PdfRectangle{}
	}
	return r
}

func intersects(r0, r1 model.PdfRectangle) bool {
	return r0.Urx > r1.Llx && r1.Urx > r0.Llx && r0.Ury > r1.Lly && r1.Ury > r0.Lly
}

func validBBox(r model.PdfRectangle) bool {
	return r.Llx < r.Urx && r.Lly < r.Ury
}

func showBBox(r model.PdfRectangle) string {
	w := r.Urx - r.Llx
	h := r.Ury - r.Lly
	return fmt.Sprintf("%5.1f %5.1fx%5.1f", r, w, h)
	// return fmt.Sprintf("%5.1f %5.1fx%5.1f=%5.1f", r, w, h, partEltQuality(r))
}

func same(x0, x1 float64) bool {
	const TOL = 0.1
	return math.Abs(x0-x1) < TOL
}
