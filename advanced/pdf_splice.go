/*
 * Splice the images from one PDF with everything but the images from another PDF.
 *
 * Run as: go run pdf_splice.go images.pdf text.pdf spliced.pdf
 */

package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/contentstream"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/model"
)

const (
	usage = `Splice the images from one PDF with everthing but the images from another model.
 go run pdf_splice.go <image pdf> <text pdf> <output pdf>
 e.g. go run pdf_splice.go images.pdf text.pdf spliced.pdf
 `
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
	companyName      = "PaperCut Software International Pty Ltd"
)

func main() {
	var imagePath, textPath, outPath string
	var debug, trace bool
	var clearContent bool
	flag.StringVar(&imagePath, "i", "", "Image PDF.")
	flag.StringVar(&textPath, "t", "", "Text PDF.")
	flag.StringVar(&outPath, "o", "", "Outpu PDF.")
	flag.BoolVar(&debug, "d", false, "Print debugging information.")
	flag.BoolVar(&trace, "e", false, "Print detailed debugging information.")
	flag.BoolVar(&clearContent, "c", false, "Don't encode content streams.")
	makeUsage(usage)
	flag.Parse()

	if outPath == "" || (imagePath == "" && textPath == "") {
		flag.Usage()
		os.Exit(1)
	}
	if trace {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelTrace))
	} else if debug {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))
	} else {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))
	}

	if uniDocLicenseKey != "" {
		if err := license.SetLicenseKey(uniDocLicenseKey, companyName); err != nil {
			common.Log.Error("error loading UniDoc license: err=%v", err)
		}
	}
	model.SetPdfCreator(companyName)

	err := splicePDFs(imagePath, textPath, outPath, clearContent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed: err=%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Completed, see output %s\n", outPath)
}

// splicePDFs combines the images from PDF `imagePath` with everything but the images from PDF
// `textPath` and writes the resulting PDF to `outPath`.
func splicePDFs(imagePath, textPath, outPath string, clearContent bool) error {
	var encoder core.StreamEncoder
	if clearContent {
		encoder = core.NewRawEncoder()
	} else {
		encoder = core.NewFlateEncoder()
	}

	imagePages, err := readPages(imagePath)
	if err != nil {
		return fmt.Errorf("splicePDFs: (%w)", err)
	}
	if imagePages != nil {
		for i, page := range imagePages {
			if err := stripText(page, encoder); err != nil {
				return fmt.Errorf("splicePDFs: imagePath=%q page %d (%w)", imagePath, i+1, err)
			}
		}
	}
	textPages, err := readPages(textPath)
	if err != nil {
		return err
	}
	if textPages != nil {
		for i, page := range textPages {
			if err := stripImages(page, encoder); err != nil {
				return fmt.Errorf("splicePDFs: textPath=%q page %d (%w)", textPath, i+1, err)
			}
		}
	}
	if imagePages == nil {
		return writePages(outPath, textPages)
	} else if textPages == nil {
		return writePages(outPath, imagePages)
	}

	if len(textPages) != len(imagePages) {
		return fmt.Errorf("splicePDFs: imagePath=%q has %d pages textPath=%q has %d pages",
			imagePath, len(imagePages), textPath, len(textPages))
	}
	numPages := len(imagePages)

	outPages := make([]*model.PdfPage, numPages)
	for i := 0; i < numPages; i++ {
		imagePage := imagePages[i]
		textPage := textPages[i]
		tbox, _ := textPage.GetMediaBox()
		ibox, _ := imagePage.GetMediaBox()
		if !equalRects(*tbox, *ibox) {
			return fmt.Errorf("splicePDFs: page sizes different %q page %d MediaBox=%.1f != %q page %d MediaBox=%.1f",
				imagePath, i+1, *ibox, textPath, i+1, *tbox)
		}
		page, err := combinePages(textPage, imagePage, encoder)
		if err != nil {
			return fmt.Errorf("splicePDFs: %q page %d (%w)", textPath, i+1, err)
		}
		outPages[i] = page
	}
	return writePages(outPath, outPages)
}

// stripImages removes the images from `page`.
func stripImages(page *model.PdfPage, encoder core.StreamEncoder) error {
	// For each page, we go through the resources and look for the images.
	contents, err := page.GetAllContentStreams()
	if err != nil {
		return fmt.Errorf("stripImages (%w)", err)
	}
	strippedContent, err := removeContentStreamImages(contents, page.Resources)
	if err != nil {
		return fmt.Errorf("stripImages (%w)", err)
	}
	page.SetContentStreams([]string{strippedContent}, encoder)
	return nil
}

// combinePages adds the images from `imagePage` to `page`.
func stripText(page *model.PdfPage, encoder core.StreamEncoder) error {
	// For each page, we go through the resources and look for the images.
	contents, err := page.GetAllContentStreams()
	if err != nil {
		return fmt.Errorf("stripText (%w)", err)
	}
	strippedContent, err := extractContentStreamImages(contents, page.Resources)
	if err != nil {
		return fmt.Errorf("stripText (%w)", err)
	}
	page.SetContentStreams([]string{strippedContent}, encoder)
	return nil
}

func combinePages(textPage, imagePage *model.PdfPage, encoder core.StreamEncoder) (*model.PdfPage, error) {
	pageXobjs := core.TraceToDirectObject(textPage.Resources.XObject)
	pageDict, ok := core.GetDict(pageXobjs)
	if !ok {
		return nil, fmt.Errorf("combinePages pageXobjs is not a dict %T", pageXobjs)
	}
	imageXobjs := core.TraceToDirectObject(imagePage.Resources.XObject)
	imageDict, ok := core.GetDict(imageXobjs)
	if !ok {
		return nil, fmt.Errorf("combinePages imageXobjs is not a dict %T", imageXobjs)
	}

	for _, name := range imageDict.Keys() {
		obj := imageDict.Get(name)
		pageDict.Set(name, obj)
	}

	// common.Log.Info("    contents=%s", contents)
	// common.& ("imageContents=%s", string(imageContents))
	pageContents, err := textPage.GetAllContentStreams()
	if err != nil {
		return nil, fmt.Errorf("combinePages (%w)", err)
	}

	// common.Log.Info("    contents=%s", contents)
	// common.& ("imageContents=%s", string(imageContents))
	imageContents, err := imagePage.GetAllContentStreams()
	if err != nil {
		return nil, fmt.Errorf("combinePages (%w)", err)
	}

	outContents := []string{pageContents, imageContents}
	common.Log.Info("outContents=%d textPage=%t imagePage=%t",
		len(outContents), textPage != nil, imagePage != nil)

	page := textPage
	page.SetContentStreams(outContents, encoder)

	if false {
		contents, err := page.GetAllContentStreams()
		if err != nil {
			panic(err)
		}
		common.Log.Info("spliced contents ------------------ \n%s", contents)
		panic("done")
	}
	return page, nil
}

var (
	opq = &contentstream.ContentStreamOperation{Operand: "q"}
	opQ = &contentstream.ContentStreamOperation{Operand: "Q"}
)

// extractContentStreamImages returns a content stream containing the image operation from content
// stream `contents`.
func extractContentStreamImages(contents string, resources *model.PdfPageResources) (string, error) {
	cstreamParser := contentstream.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return "", err
	}
	processedOperations := &contentstream.ContentStreamOperations{opq}
	processedXObjects := map[string]bool{} // Keep track of processed XObjects to avoid repetition.

	fontDict, has := core.GetDict(resources.Font)
	if has {
		for _, name := range fontDict.Keys() {
			fontDict.Remove(name)
			common.Log.Info("remove font %#q", name)
		}
	}
	fontDict, has = core.GetDict(resources.Font)
	if has {
		for _, name := range fontDict.Keys() {
			common.Log.Info("remaining font %#q", name)
			panic("fonts")
		}
	}
	resources.Font = nil

	processor := contentstream.NewContentStreamProcessor(*operations)
	processor.AddHandler(contentstream.HandlerConditionEnumAllOperands, "",
		func(op *contentstream.ContentStreamOperation, gs contentstream.GraphicsState, resources *model.PdfPageResources) error {
			found := false
			switch op.Operand {
			case "cm", "q", "Q", "g", "G", "rg", "RG":
				found = true
			case "Do":
				name := op.Params[0].(*core.PdfObjectName)
				if _, ok := processedXObjects[string(*name)]; !ok {
					processedXObjects[string(*name)] = true
					_, xtype := resources.GetXObjectByName(*name)
					found = xtype == model.XObjectTypeImage
				}
			}
			if found {
				*processedOperations = append(*processedOperations, op)
				fmt.Printf("%d: %s\n", len(*processedOperations), op)
				if op.Operand == "Tj" {
					panic("text")
				}
			}
			return nil
		})

	err = processor.Process(resources)
	if err != nil {
		return "", fmt.Errorf("extractContentStreamImages Process failed (%w)", err)
	}
	*processedOperations = append(*processedOperations, opQ)
	return processedOperations.String(), nil
}

// removeContentStreamImages the content stream `contents` with removes the images remvoved.
// The images from `resources` are removed in place.
func removeContentStreamImages(contents string, resources *model.PdfPageResources) (string, error) {
	cstreamParser := contentstream.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return "", fmt.Errorf("removeContentStreamImages (%w)", err)
	}
	processedOperations := &contentstream.ContentStreamOperations{opq}
	processedXObjects := map[string]bool{} // Keep track of processed XObjects to avoid repetition.

	xobjs := core.TraceToDirectObject(resources.XObject)
	xobjDict := xobjs.(*core.PdfObjectDictionary)

	processor := contentstream.NewContentStreamProcessor(*operations)
	processor.AddHandler(contentstream.HandlerConditionEnumAllOperands, "",
		func(op *contentstream.ContentStreamOperation, gs contentstream.GraphicsState, resources *model.PdfPageResources) error {
			removed := false
			if op.Operand == "Do" {
				name := op.Params[0].(*core.PdfObjectName)
				if _, ok := processedXObjects[string(*name)]; !ok {
					processedXObjects[string(*name)] = true
					_, xtype := resources.GetXObjectByName(*name)
					if xtype == model.XObjectTypeImage {
						xobjDict.Remove(*name)
						removed = true
					}
				}
			}
			if !removed {
				*processedOperations = append(*processedOperations, op)
			}
			return nil
		})

	err = processor.Process(resources)
	if err != nil {
		return "", fmt.Errorf("removeContentStreamImages Process failed (%w)", err)
	}
	*processedOperations = append(*processedOperations, opQ)
	return processedOperations.String(), nil
}

// readPages returns the pages in PDF file `inPath`.
func readPages(inPath string) ([]*model.PdfPage, error) {
	if inPath == "" {
		return nil, nil
	}
	decorate := func(err error) error { return fmt.Errorf("readPages %q (%w)", inPath, err) }
	f, err := os.Open(inPath)
	if err != nil {
		return nil, decorate(err)
	}
	defer f.Close()

	pdfReader, err := model.NewPdfReader(f)
	if err != nil {
		return nil, decorate(err)
	}

	isEncrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return nil, decorate(err)
	}

	// Try decrypting with an empty one.
	if isEncrypted {
		auth, err := pdfReader.Decrypt([]byte(""))
		if err != nil {
			// Encrypted and we cannot do anything about it.
			return nil, decorate(err)
		}
		if !auth {
			return nil, decorate(errors.New("Need to decrypt with password"))
		}
	}

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return nil, decorate(err)
	}
	// common.Log.Info("PDF Num Pages: %d %q\n", numPages, inPath)

	pages := make([]*model.PdfPage, numPages)
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		decorate := func(err error) error { return fmt.Errorf("readPages %q page %d (%w)", inPath, pageNum, err) }
		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			return nil, decorate(err)
		}
		mbox, err := page.GetMediaBox()
		if err != nil {
			return nil, decorate(err)
		}
		// common.Log.Info("%.0f %d", *mbox, *page.Rotate)

		if page.Rotate != nil && *page.Rotate != 0 {
			// Normalize all pagees to no viewer rotation.
			cc := contentstream.NewContentCreator()
			switch *page.Rotate {
			case 90:
				cc.Add_cm(0, -1, 1, 0, 0, mbox.Width())
				mbox.Llx, mbox.Lly = mbox.Lly, mbox.Llx
				mbox.Urx, mbox.Ury = mbox.Ury, mbox.Urx
			case 180:
				cc.Add_cm(-1, 0, 0, -1, mbox.Width(), mbox.Height())
			case 270:
				cc.Add_cm(0, 1, -1, 0, mbox.Height(), 0)
				mbox.Llx, mbox.Lly = mbox.Lly, mbox.Llx
				mbox.Urx, mbox.Ury = mbox.Ury, mbox.Urx

			}
			rotateOps := cc.Operations().String()

			contents, err := page.GetContentStreams()
			if err != nil {
				return nil, decorate(err)
			}
			contents = append([]string{rotateOps}, contents...)

			page = page.Duplicate()
			if err = page.SetContentStreams(contents, nil); err != nil {
				return nil, decorate(err)
			}
			page.Rotate = nil
		}
		pages[pageNum-1] = page
	}
	return pages, nil
}

// writePages writes `pages` to PDF file `outPath`.
func writePages(outPath string, pages []*model.PdfPage) error {
	model.SetIsPDFA(true)
	model.SetPdfCreationDate(time.Now())
	model.SetPdfModifiedDate(time.Now().Add(time.Second))
	// model.SetPdfSubject("SUBJECT")
	// model.SetPdfAuthor("AUTHONR")
	pdfWriter := model.NewPdfWriter()
	for _, page := range pages {
		if err := pdfWriter.AddPage(page); err != nil {
			return fmt.Errorf("writePages (%w)", err)
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("writePages %q (%w)", outPath, err)
	}
	defer f.Close()
	if err := pdfWriter.Write(f); err != nil {
		return fmt.Errorf("writePages %q (%w)", outPath, err)
	}
	return nil
}

// equalRects returns true if `r1` and `r2` have the same coordinates. It allows for rounding errors.
func equalRects(r1, r2 model.PdfRectangle) bool {
	const tol = 1.0e-3
	eq := func(x1, x2 float64) bool { return math.Abs(x1-x2) < tol }
	return eq(r1.Llx, r2.Llx) && eq(r1.Lly, r2.Lly) && eq(r1.Urx, r2.Urx) && eq(r1.Ury, r2.Ury)
}

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}

//  160171 18 Jul 13:11 image.pdf   (Xerox mixed raster)
// 1296640 18 Jul 13:11 text.pdf    (PaperCut OCR)
//  167405 19 Jul 18:51 spliced.pdf (Xerox images + PaperCut text)
