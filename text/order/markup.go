package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/creator"
	"github.com/unidoc/unipdf/v3/model"
)

type saveMarkedupParams struct {
	markups   map[int]map[string]rectList
	curPage   int
	markupDir string
}

// Saves a marked up PDF with the original with certain groups highlighted: marks, words, lines, columns.
func saveMarkedupPDF(params saveMarkedupParams, inPath, markupType string) error {
	markupOutputPath := changePath(params.markupDir, inPath, markupType, ".pdf")
	// fmt.Fprintf(os.Stderr, "      markupType=%q\n", markupType)
	// fmt.Fprintf(os.Stderr, "params.markupDir=%q\n", params.markupDir)
	// fmt.Fprintf(os.Stderr, "          inPath=%q\n", inPath)
	// fmt.Fprintf(os.Stderr, "      markupType=%q\n", markupType)
	// fmt.Fprintf(os.Stderr, "markupOutputPath=%q\n", markupOutputPath)

	var pageNums []int
	for pageNum := range params.markups {
		pageNums = append(pageNums, pageNum)
	}
	sort.Ints(pageNums)
	if len(pageNums) == 0 {
		return nil
	}

	f, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("Could not open %q err=%w", inPath, err)
	}
	defer f.Close()

	pdfReader, err := model.NewPdfReaderLazy(f)
	if err != nil {
		return fmt.Errorf("NewPdfReaderLazy failed. %q err=%w", inPath, err)
	}

	// Make a new PDF creator.
	c := creator.New()
	for _, pageNum := range pageNums {
		common.Log.Debug("Page %d - %d marks", pageNum, len(params.markups[pageNum]))
		page, err := pdfReader.GetPage(pageNum)
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

		group := params.markups[pageNum][markupType]
		dx := 0.0
		dy := 0.0

		common.Log.Info("markupType=%q dx=%.1f dy=%.1f pageNum=%d", markupType, dx, dy, pageNum)

		width := widths[markupType]
		borderColor := creator.ColorRGBFromHex(colors[markupType])
		bgdColor := creator.ColorRGBFromHex(bkgnds[markupType])
		common.Log.Debug("borderColor=%+q %.2f", colors[markupType], borderColor)
		common.Log.Debug("   bgdColor=%+q %.2f", bkgnds[markupType], bgdColor)
		for i, r := range group {
			common.Log.Debug("Mark %d: %5.1f x,y,w,h=%5.1f %5.1f %5.1f %5.1f", i+1, r,
				r.Llx, h-r.Lly, r.Urx-r.Llx, -(r.Ury - r.Lly))

			llx := r.Llx + dx
			urx := r.Urx - dx
			lly := r.Lly + dy
			ury := r.Ury - dy

			w := width * 1.1
			rect := c.NewRectangle(llx+w, h-(lly+w), urx-llx-2*w, -(ury - lly - 2*w))
			rect.SetBorderColor(bgdColor)
			rect.SetBorderWidth(2.0 * w)
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

	// c.SetOutlineTree(params.pdfReader.GetOutlineTree())
	if err := c.WriteToFile(markupOutputPath); err != nil {
		return fmt.Errorf("WriteToFile failed. %q err=%w", markupOutputPath, err)
	}

	common.Log.Info("Saved marked-up PDF file: %q", markupOutputPath)
	return nil
}

var (
	widths = map[string]float64{
		"marks":   0.5,
		"words":   0.1,
		"lines":   0.2,
		"divs":    0.6,
		"gaps":    0.3,
		"space":   0.35,
		"columns": 0.4,
		"page":    1.1,
	}
	colors = map[string]string{
		"marks":   "#0000ff",
		"words":   "#ff0000",
		"lines":   "#f0f000",
		"divs":    "#ffff00",
		"gaps":    "#ff0000",
		"space":   "#00ffff",
		"columns": "#00ff00",
		"page":    "#00aabb",
	}
	bkgnds = map[string]string{
		"marks":   "#ffff00",
		"words":   "#ff00ff",
		"lines":   "#00afaf",
		"divs":    "#0000ff",
		"gaps":    "#00ffff",
		"space":   "#ff0000",
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
