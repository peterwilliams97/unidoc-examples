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
	imagePages, err := readPages(imagePath)
	if err != nil {
		return fmt.Errorf("splicePDFs: (%w)", err)
	}
	textPages, err := readPages(textPath)
	if err != nil {
		return err
	}
	if imagePages != nil && textPages != nil && len(textPages) != len(imagePages) {
		return fmt.Errorf("splicePDFs: imagePath=%q has %d pages textPath=%q has %d pages",
			imagePath, len(imagePages), textPath, len(textPages))
	}
	var numPages int
	if imagePages != nil {
		numPages = len(imagePages)
	} else {
		numPages = len(textPages)
	}

	outPages := make([]*model.PdfPage, numPages)
	for i := 0; i < numPages; i++ {
		var imagePage, textPage *model.PdfPage
		if imagePages != nil {
			imagePage = imagePages[i]
		}
		if textPages != nil {
			textPage = textPages[i]
		}
		if imagePage != nil && textPage != nil {
			tbox, _ := textPage.GetMediaBox()
			ibox, _ := imagePage.GetMediaBox()
			if !equalRects(*tbox, *ibox) {
				return fmt.Errorf("splicePDFs: page sizes different %q page %d MediaBox=%.1f != %q page %d MediaBox=%.1f",
					imagePath, i+1, *ibox, textPath, i+1, *tbox)
			}
		}
		if textPage != nil {
			if err := stripImages(textPage); err != nil {
				return fmt.Errorf("splicePDFs: textPath=%q page %d (%w)", textPath, i+1, err)
			}
		}
		if err := addImages(textPage, imagePage, clearContent); err != nil {
			return fmt.Errorf("splicePDFs: %q page %d (%w)", textPath, i+1, err)
		}
		page := textPage
		if page == nil {
			page = imagePage
		}
		outPages[i] = page
	}
	return writePages(outPath, outPages)
}

// stripImages removes the images from `page`.
func stripImages(page *model.PdfPage) error {
	// For each page, we go through the resources and look for the images.
	contents, err := page.GetAllContentStreams()
	if err != nil {
		return fmt.Errorf("stripImages (%w)", err)
	}
	strippedContent, err := removeContentStreamImages(contents, page.Resources)
	if err != nil {
		return fmt.Errorf("stripImages (%w)", err)
	}
	page.SetContentStreams([]string{string(strippedContent)}, core.NewFlateEncoder())
	return nil
}

// addImages adds the images from `imagePage` to `page`.
func addImages(textPage, imagePage *model.PdfPage, clearContent bool) error {
	// For each page, we go through the resources and look for the images.
	var pageContents, imageContents string

	if imagePage != nil {
		imageAllContents, err := imagePage.GetAllContentStreams()
		if err != nil {
			return fmt.Errorf("addImages (%w)", err)
		}
		// common.Log.Info("image contents ------------------\n%s", imageAllContents)
		imageContentsBytes, err := extractContentStreamImages(imageAllContents, imagePage.Resources)
		if err != nil {
			return fmt.Errorf("addImages (%w)", err)
		}
		imageContents = string(imageContentsBytes)
	}

	if textPage != nil {
		pageXobjs := core.TraceToDirectObject(textPage.Resources.XObject)
		// common.Log.Info("xobjs=%T", pageXobjs)
		pageDict, ok := core.GetDict(pageXobjs)
		if !ok {
			return fmt.Errorf("addImages pageXobjs is not a dict %T", pageXobjs)
		}
		imageXobjs := core.TraceToDirectObject(imagePage.Resources.XObject)
		// common.Log.Info("xobjs=%T", imageXobjs)
		imageDict, ok := core.GetDict(imageXobjs)
		if !ok {
			return fmt.Errorf("addImages imageXobjs is not a dict %T", imageXobjs)
		}

		for _, name := range imageDict.Keys() {
			obj := imageDict.Get(name)
			pageDict.Set(name, obj)
		}

		// common.Log.Info("    contents=%s", contents)
		// common.& ("imageContents=%s", string(imageContents))
		var err error
		pageContents, err = textPage.GetAllContentStreams()
		if err != nil {
			return fmt.Errorf("addImages (%w)", err)
		}
	}

	var encoder core.StreamEncoder
	if !clearContent {
		encoder = core.NewFlateEncoder()
	}
	var outContents []string
	if pageContents != "" {
		// common.Log.Info("================ pageContents", pageContents)
		outContents = append(outContents, pageContents)
	}
	if imageContents != "" {
		// common.Log.Info("================ imageContents\n%s", imageContents)
		outContents = append(outContents, imageContents)
	}

	// common.Log.Info("outContents=%d", len(outContents))

	page := textPage
	if page == nil {
		page = imagePage
	}

	page.SetContentStreams(outContents, encoder)

	if false {
		contents, err := page.GetAllContentStreams()
		if err != nil {
			panic(err)
		}
		common.Log.Info("spliced contents ------------------\n%s", contents)
		// panic("done")
	}
	return nil
}

var (
	opq = &contentstream.ContentStreamOperation{Operand: "q"}
	opQ = &contentstream.ContentStreamOperation{Operand: "Q"}
)

// extractContentStreamImages returns a content stream containing the image operation from content
// stream `contents`.
func extractContentStreamImages(contents string, resources *model.PdfPageResources) ([]byte, error) {
	cstreamParser := contentstream.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return nil, err
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
			}
			return nil
		})

	err = processor.Process(resources)
	if err != nil {
		return nil, fmt.Errorf("extractContentStreamImages Process failed (%w)", err)
	}
	*processedOperations = append(*processedOperations, opQ)
	return processedOperations.Bytes(), nil
}

// removeContentStreamImages the content stream `contents` with removes the images remvoved.
// The images from `resources` are removed in place.
func removeContentStreamImages(contents string, resources *model.PdfPageResources) ([]byte, error) {
	cstreamParser := contentstream.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return nil, fmt.Errorf("removeContentStreamImages (%w)", err)
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
		return nil, fmt.Errorf("removeContentStreamImages Process failed (%w)", err)
	}
	*processedOperations = append(*processedOperations, opQ)
	return processedOperations.Bytes(), nil
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
