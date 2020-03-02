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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/contentstream"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

const (
	usage = "Usage: go run main.go [options] <file.pdf> <output.txt>\n"
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
	// common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))
	// testMosaic()
	var (
		loglevel   string
		saveMarkup string
		outDir     string
		markupDir  string
		firstPage  int
		lastPage   int
	)
	flag.StringVar(&loglevel, "L", "info", "Set log level (default: info)")
	flag.StringVar(&saveMarkup, "m", "columns", "Save markup (none/marks/words/lines/columns/all)")
	flag.StringVar(&markupDir, "mf", "layouts", "Output markup directory (default layouts)")
	flag.StringVar(&outDir, "o", "outtext", "Output text (default outtext)")
	flag.IntVar(&firstPage, "f", -1, "First page")
	flag.IntVar(&lastPage, "l", 100000, "Last page")
	makeUsage(usage)
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
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

	if err := os.MkdirAll(outDir, 0777); err != nil {
		panic(fmt.Errorf("Couldn't create outDir=%q err=%w", outDir, err))
	}
	if err := os.MkdirAll(markupDir, 0777); err != nil {
		panic(fmt.Errorf("Couldn't create markupDir=%q err=%w", markupDir, err))
	}
	saveParams.markupDir = markupDir
	if markupDir == "" || markupDir == "." {
		panic(markupDir)
	}

	fileList := args
	sort.Slice(fileList, func(i, j int) bool {
		fi, fj := fileList[i], fileList[j]
		si, sj := fileSize(fi), fileSize(fj)
		if si != sj {
			return si < sj
		}
		return fi < fj
	})

	for _, inPath := range fileList {
		outPath := changePath(outDir, filepath.Base(inPath), "", ".txt")
		if strings.ToLower(filepath.Ext(outPath)) == ".pdf" {
			panic(fmt.Errorf("output can't be PDF %q", outPath))
		}

		err := extractColumnText(inPath, outPath, firstPage, lastPage)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

// extractColumnText extracts text columns from PDF file `inPath` and outputs the data as a text
// file to `outPath`.
func extractColumnText(inPath, outPath string, firstPage, lastPage int) error {
	common.Log.Info("extractColumnText: inPath=%q [%d:%d]->%q", inPath, firstPage, lastPage, outPath)
	fmt.Fprintf(os.Stderr, "\n&&& inPath=%q [%d:%d]->%q %.2f MB\n",
		inPath, firstPage, lastPage, outPath, fileSize(inPath))
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

	saveParams.markups = map[int]map[string]rectList{}

	// var pageTexts []string

	if firstPage < 1 {
		firstPage = 1
	}
	if lastPage > numPages {
		lastPage = numPages
	}

	for pageNum := firstPage; pageNum <= lastPage; pageNum++ {
		saveParams.curPage = pageNum
		saveParams.markups[saveParams.curPage] = map[string]rectList{}

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
		fmt.Fprintf(os.Stderr, " %d,", pageNum)

		common.Log.Info("%d words", len(words))

		bound, obstacles := boundedObstacles(*mbox, words)

		ss := newFragmentState(bound, obstacles)
		pageGaps := ss.scan()
		var wideGaps rectList
		for _, gap := range pageGaps {
			if gap.Width() >= 10.0 {
				wideGaps = append(wideGaps, gap)
			}
		}

		m := createMosaic(wideGaps)
		m.connectRecursive(0.5)
		// m.connect(10.0)
		common.Log.Info("m=%d", len(m.rects))

		heightOrder := make([]int, len(m.rects))
		for i := 0; i < len(m.rects); i++ {
			heightOrder[i] = i
		}

		numVert := func(r idRect) int {
			return len(r.above) + len(r.below)
		}
		sort.Slice(heightOrder, func(i, j int) bool {
			oi, oj := heightOrder[i], heightOrder[j]
			ri, rj := m.rects[oi], m.rects[oj]
			hi, hj := numVert(ri), numVert(rj)
			if hi != hj {
				return hi > hj
			}
			return ri.id < rj.id
		})
		common.Log.Info("All rects: %d", len(m.rects))
		besti := -1
		bestH := -1.0
		verts := make(rectList, len(m.rects))
		for i, o := range heightOrder {
			r := m.rects[o]
			fmt.Printf("%4d: %2d -- r=%s\n", i, numVert(r), m.rectString(r))
			vert := append(r.above, r.id)
			vert = append(vert, r.below...)
			rr, order := m.bestVert(vert, 5.0)
			verts[i] = rr
			// bbox := m.asRectList(best).union()
			fmt.Printf("   bestVert=%s %v\n", showBBox(rr), order)
			if rr.Height() > bestH {
				besti = i
				bestH = rr.Height()
			}
		}
		sort.Slice(verts, func(i, j int) bool {
			ri, rj := verts[i], verts[j]
			hi, hj := ri.Height(), rj.Height()
			if hi != hj {
				return hi > hj
			}
			return ri.Width() > rj.Width()
		})
		var talls rectList
		sigSet := map[float64]struct{}{}
		for _, r := range verts {
			if r.Height() < 40.0 {
				continue
			}
			sig := partEltSig(r)
			if _, ok := sigSet[sig]; ok {
				continue
			}
			talls = append(talls, r)
			sigSet[sig] = struct{}{}
		}
		saveParams.markups[pageNum]["marks"] = talls
		common.Log.Info("<<<<verts=%4d talls=%4d  =====================", len(verts), len(talls))
		for i, r := range verts {
			fmt.Printf("%4d: %s\n", i, showBBox(r))
		}

		var r idRect
		if besti >= 0 {
			common.Log.Info("%4d: -- r=%s =====================", besti, m.rectString(r))
			vert := append(r.above, r.id)
			vert = append(vert, r.below...)
			rr, order := m.bestVert(vert, 10.0)
			fmt.Printf("bestVert=%s %v\n", showBBox(rr), order)
		}

		columns := scanPage(bound, talls)
		// // columns = removeEmpty(pageBound, columns, obstacles)
		saveParams.markups[saveParams.curPage]["columns"] = columns

		group := make(rectList, textMarks.Len())
		for i, mark := range textMarks.Elements() {
			group[i] = mark.BBox
		}
		// saveParams.markups[pageNum]["marks"] = group

		// outPageText, err := pageMarksToColumnText(pageNum, words, *mbox)
		// if err != nil {
		// 	common.Log.Debug("Error grouping text: %v", err)
		// 	return err
		// }
		// header := fmt.Sprintf("----------------\n ### PAGE %d of %d", pageNum, numPages)
		// pageTexts = append(pageTexts, header)
		// pageTexts = append(pageTexts, outPageText)
	}
	// return nil

	// docText := strings.Join(pageTexts, "\n")
	// if err := ioutil.WriteFile(outPath, []byte(docText), 0666); err != nil {
	// 	return fmt.Errorf("failed to write outPath=%q err=%w", outPath, err)
	// }

	for _, markupType := range []string{ /*"gaps",*/ "marks", "columns"} {
		err = saveMarkedupPDF(saveParams, inPath, markupType)
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

	pageBound, _, pageGaps := whitespaceCover(pageBound, words)
	// saveParams.markups[pageNum]["page"] = rectList{pageBound}

	common.Log.Info("%d pageGaps~~~~~~~~~~~~~~~~~~~ ", len(pageGaps))
	var bigBBoxes rectList
	for _, bbox := range pageGaps {
		if bbox.Height() < 20 {
			continue
		}
		bigBBoxes = append(bigBBoxes, bbox)
	}
	common.Log.Info("%d big pageGaps~~~~~~~~~~~~~~~~~~~ ", len(bigBBoxes))
	pageGaps = bigBBoxes

	y := -100.0
	for i, bbox := range pageGaps {
		if bbox.Ury != y {
			y = bbox.Ury
			common.Log.Info("y=%5.1f", y)
		}
		common.Log.Info("%4d of %d: %s", i+1, len(pageGaps), showBBox(bbox))
	}

	saveParams.markups[saveParams.curPage]["gaps"] = pageGaps

	// columns := scanPage(pageBound, pageGaps)
	// // columns = removeEmpty(pageBound, columns, obstacles)
	// saveParams.markups[saveParams.curPage]["columns"] = columns

	// columnText := getColumnText(lines, columnBBoxes)
	// return strings.Join(columnText, "\n####\n"), nil
	return "FIX ME", nil
}

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}

// changePath inserts `insertion` into `filename` before suffix `ext`.
func changePath(dirName, filename, qualifier, ext string) string {
	base := filepath.Base(filename)
	oxt := filepath.Ext(base)
	base = base[:len(base)-len(oxt)]
	if len(qualifier) > 0 {
		base = fmt.Sprintf("%s.%s", base, qualifier)
	}
	filename = fmt.Sprintf("%s%s", base, ext)
	path := filepath.Join(dirName, filename)
	common.Log.Info("changePath(%q,%q,%q)->%q", dirName, base, ext, path)
	return path
}

// fileSize returns the size of file `path` in bytes
func fileSize(path string) float64 {
	fi, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	return float64(fi.Size()) / 1024.0 / 1024.0
}
