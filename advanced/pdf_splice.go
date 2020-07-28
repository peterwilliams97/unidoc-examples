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
	noBgd = false
	noFgd = false

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
	var firstPage, lastPage int
	flag.IntVar(&firstPage, "f", -1, "First page")
	flag.IntVar(&lastPage, "l", 100000, "Last page")
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

	err := splicePDFs(imagePath, textPath, outPath, firstPage, lastPage, clearContent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed: err=%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Completed, see output %s\n", outPath)
}

// splicePDFs combines the images from PDF `imagePath` with everything but the images from PDF
// `textPath` and writes the resulting PDF to `outPath`.
func splicePDFs(imagePath, textPath, outPath string, firstPage, lastPage int, clearContent bool) error {
	encoder := getEncoder(clearContent)
	imagePages, err := readModifyPages(imagePath, firstPage, lastPage, encoder, extractContentStreamImages)
	if err != nil {
		return fmt.Errorf("splicePDFs: imagePath (%w)", err)
	}
	textPages, err := readModifyPages(textPath, firstPage, lastPage, encoder, removeContentStreamImages)
	if err != nil {
		return fmt.Errorf("splicePDFs: textPath (%w)", err)
	}
	if imagePages == nil {
		return writePages(outPath, textPages)
	} else if textPages == nil {
		return writePages(outPath, imagePages)
	}

	// There are text and image pages.

	if len(imagePages) != len(textPages) {
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
				imagePath, i+firstPage, *ibox, textPath, i+1, *tbox)
		}
		page, err := combinePages(imagePage, textPage, encoder)
		if err != nil {
			return fmt.Errorf("splicePDFs: %q page %d (%w)", textPath, i+1, err)
		}
		outPages[i] = page
	}
	return writePages(outPath, outPages)
}

// combinePages combines `imagePage` with `textPage`, encodes the combined page with `encoder` and
// returns the resulting page.
func combinePages(imagePage, textPage *model.PdfPage, encoder core.StreamEncoder) (*model.PdfPage, error) {
	textContents, err := textPage.GetAllContentStreams()
	if err != nil {
		return nil, fmt.Errorf("combinePages (%w)", err)
	}
	imageContents, err := imagePage.GetAllContentStreams()
	if err != nil {
		return nil, fmt.Errorf("combinePages (%w)", err)
	}
	pageContents := []string{textContents, imageContents}

	textXobjs := core.TraceToDirectObject(textPage.Resources.XObject)
	textDict, ok := core.GetDict(textXobjs)
	if !ok {
		return nil, fmt.Errorf("combinePages pageXobjs is not a dict %T", textXobjs)
	}
	imageXobjs := core.TraceToDirectObject(imagePage.Resources.XObject)
	imageDict, ok := core.GetDict(imageXobjs)
	if !ok {
		return nil, fmt.Errorf("combinePages imageXobjs is not a dict %T", imageXobjs)
	}

	pageDict := core.MakeDict()
	for _, name := range imageDict.Keys() {
		obj := imageDict.Get(name)
		pageDict.Set(name, obj)
	}
	for _, name := range textDict.Keys() {
		obj := textDict.Get(name)
		pageDict.Set(name, obj)
	}

	page := textPage.Duplicate()
	page.Resources.XObject = pageDict
	page.SetContentStreams(pageContents, encoder)

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

// extractContentStreamImages returns a content stream containing the image operations from content
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
	resources.Font = nil

	xobjs := core.TraceToDirectObject(resources.XObject)
	xobjDict := xobjs.(*core.PdfObjectDictionary)

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
					ximg, xtype := resources.GetXObjectByName(*name)
					found = xtype == model.XObjectTypeImage
					if found {
						filter := ximg.Get("Filter")
						isFgd := filter.String() == "JBIG2Decode" || filter.String() == "CCITTFaxDecode"
						if (noFgd && isFgd) || (noBgd && !isFgd) {
							found = false
						}

						if found {
							w, _ := core.GetIntVal(ximg.Get("Width"))
							h, _ := core.GetIntVal(ximg.Get("Height"))
							common.Log.Debug("fiter=%#q %d x %d %q", filter.String(), w, h, ximg.Keys())
						}
					}
					if !found {
						xobjDict.Remove(*name)
					}
				}
			}
			if found {
				*processedOperations = append(*processedOperations, op)
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

// removeContentStreamImages returns the content stream `contents` with the image references removed.
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

// readModifyPages reads the pages in PDF file `inPath`,  applies `modifier` to each, encodes each
// page contents with `eoncoder` and returns the pagea.
func readModifyPages(inPath string, firstPage, lastPage int, encoder core.StreamEncoder, modifier pageModifier) (
	[]*model.PdfPage, error) {
	pages, err := readPages(inPath, firstPage, lastPage)
	if err != nil {
		return nil, fmt.Errorf("readModifyPages: (%w)", err)
	}
	if pages == nil {
		return nil, nil
	}
	for i, page := range pages {
		if err := modifyPage(page, encoder, modifier); err != nil {
			return nil, fmt.Errorf("PdfPage: inPath=%q page %d (%w)", inPath, i+1, err)
		}
	}
	return pages, nil
}

type pageModifier func(contents string, resources *model.PdfPageResources) (string, error)

// modifyPage applies `modifier` to `page`. The page contents are encoded with `eoncoder`.
func modifyPage(page *model.PdfPage, encoder core.StreamEncoder, modifier pageModifier) error {
	contents, err := page.GetAllContentStreams()
	if err != nil {
		return fmt.Errorf("modifyPage (%w)", err)
	}
	strippedContent, err := modifier(contents, page.Resources)
	if err != nil {
		return fmt.Errorf("modifyPage (%w)", err)
	}
	page.SetContentStreams([]string{strippedContent}, encoder)
	return nil
}

// readPages returns the pages in PDF file `inPath`.
func readPages(inPath string, firstPage, lastPage int) ([]*model.PdfPage, error) {
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
	firstPage = maxInt(1, firstPage)
	lastPage = minInt(numPages, lastPage)

	pages := make([]*model.PdfPage, lastPage-firstPage+1)
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		if !(firstPage <= pageNum && pageNum <= lastPage) {
			continue
		}
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
			// Normalize all pages to no viewer rotation.
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
			page = page.Duplicate() // XXX(peterwilliams97): Not necessary for this example as we don't reuse page.
			if err = page.SetContentStreams(contents, nil); err != nil {
				return nil, decorate(err)
			}
			page.Rotate = nil
		}
		pages[pageNum-firstPage] = page
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
	const tol = 0.5
	eq := func(x1, x2 float64) bool { return math.Abs(x1-x2) < tol }
	return eq(r1.Llx, r2.Llx) && eq(r1.Lly, r2.Lly) && eq(r1.Urx, r2.Urx) && eq(r1.Ury, r2.Ury)
}

func getEncoder(clearContent bool) core.StreamEncoder {
	if clearContent {
		return core.NewRawEncoder()
	}
	return core.NewFlateEncoder()
}

// makeUsage updates flag.Usage to include usage message `msg`.
func makeUsage(msg string) {
	usage := flag.Usage
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, msg)
		usage()
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

//  160171 18 Jul 13:11 image.pdf   (Xerox mixed raster)
// 1296640 18 Jul 13:11 text.pdf    (PaperCut OCR)
//  167405 19 Jul 18:51 spliced.pdf (Xerox images + PaperCut text)
