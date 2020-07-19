/*
 * Splice the images from one PDF with everthing but the images from another PDF.
 *
 * Run as: go run pdf_splice.go images.pdf text.pdf spliced.pdf
 */

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/unidoc/unipdf/v3/common"
	unicommon "github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/contentstream"
	pdfcontent "github.com/unidoc/unipdf/v3/contentstream"
	"github.com/unidoc/unipdf/v3/core"
	pdfcore "github.com/unidoc/unipdf/v3/core"
	pdf "github.com/unidoc/unipdf/v3/model"
)

func init() {
	unicommon.SetLogger(unicommon.NewConsoleLogger(unicommon.LogLevelInfo))
}

func main() {
	if len(os.Args) <= 3 {
		fmt.Printf("Syntax: go run pdf_splice.go images.pdf text.pdf spliced.pdf\n")
		os.Exit(1)
	}

	imagePath := os.Args[1]
	textPath := os.Args[2]
	outputPath := os.Args[3]

	err := splicePDFs(imagePath, textPath, outputPath)
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Completed, see output %s\n", outputPath)
}

func splicePDFs(imagePath, textPath, outPath string) error {
	common.Log.Info("Stripping %q", textPath)
	textPages, err := readPages(textPath)
	if err != nil {
		return err
	}
	imagePages, err := readPages(imagePath)
	if err != nil {
		return err
	}
	for i, page := range textPages {
		imagePage := imagePages[i]
		// if err := stripImages(page); err != nil {
		// 	return err
		// }
		if err := addImages(page, imagePage); err != nil {
			return err
		}
	}
	return writePages(outPath, textPages)
}

// stripImages adds the images from `imagePage` to `page`.
func addImages(page, imagePage *pdf.PdfPage) error {
	// For each page, we go through the resources and look for the images.
	imageAllContents, err := imagePage.GetAllContentStreams()
	if err != nil {
		return err
	}
	common.Log.Info("image contents ------------------\n%s", imageAllContents)
	imageContents, err := extractContentStreamImages(imageAllContents, imagePage.Resources)
	if err != nil {
		return err
	}

	pageXobjs := core.TraceToDirectObject(page.Resources.XObject)
	common.Log.Info("xobjs=%T", pageXobjs)
	pageDict := pageXobjs.(*pdfcore.PdfObjectDictionary)
	imageXobjs := core.TraceToDirectObject(imagePage.Resources.XObject)
	common.Log.Info("xobjs=%T", imageXobjs)
	imageDict := imageXobjs.(*pdfcore.PdfObjectDictionary)

	for _, name := range imageDict.Keys() {
		obj := imageDict.Get(name)
		pageDict.Set(name, obj)
	}

	// common.Log.Info("    contents=%s", contents)
	common.Log.Info("imageContents=%s", string(imageContents))

	// pageContents, err := page.GetAllContentStreams()
	// if err != nil {
	// 	return err
	// }

	page.SetContentStreams([]string{
		// pageContents,
		string(imageContents)},
		pdfcore.NewFlateEncoder())

	if true {
		contents, err := page.GetAllContentStreams()
		if err != nil {
			panic(err)
		}
		common.Log.Info("spliced contents ------------------\n%s", contents)
		// panic("done")
	}
	return nil
}

func extractContentStreamImages(contents string, resources *pdf.PdfPageResources) ([]byte, error) {
	cstreamParser := pdfcontent.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return nil, err
	}
	q := &pdfcontent.ContentStreamOperation{Operand: "q"}
	Q := &pdfcontent.ContentStreamOperation{Operand: "Q"}
	processedOperations := &pdfcontent.ContentStreamOperations{q}
	processedXObjects := map[string]bool{} // Keep track of processed XObjects to avoid repetition.

	processor := pdfcontent.NewContentStreamProcessor(*operations)
	processor.AddHandler(pdfcontent.HandlerConditionEnumAllOperands, "",
		func(op *pdfcontent.ContentStreamOperation, gs pdfcontent.GraphicsState, resources *pdf.PdfPageResources) error {
			found := false
			switch op.Operand {
			case "cm", "q", "Q", "g", "G", "rg", "RG":
				found = true
			case "Do":
				name := op.Params[0].(*pdfcore.PdfObjectName)
				if _, ok := processedXObjects[string(*name)]; !ok {
					processedXObjects[string(*name)] = true
					_, xtype := resources.GetXObjectByName(*name)
					found = xtype == pdf.XObjectTypeImage
				}
			}
			if found {
				*processedOperations = append(*processedOperations, op)

			} else if op.Operand != "Tj" {
				// common.Log.Info("op=%+v ops=%d found=%t", op, len(*processedOperations), found)
			}

			return nil
		})

	err = processor.Process(resources)
	if err != nil {
		fmt.Printf("Error processing: %v\n", err)
		return nil, err
	}
	*processedOperations = append(*processedOperations, Q)
	return processedOperations.Bytes(), nil
}

// stripImages strips the images out of `page`.
func stripImages(page *pdf.PdfPage) error {
	// For each page, we go through the resources and look for the images.
	contents, err := page.GetAllContentStreams()
	if err != nil {
		return err
	}
	strippedContent, err := stripContentStreamImages(contents, page.Resources)
	if err != nil {
		return err
	}
	page.SetContentStreams([]string{string(strippedContent)}, pdfcore.NewFlateEncoder())
	return nil
}

func stripContentStreamImages(contents string, resources *pdf.PdfPageResources) ([]byte, error) {
	cstreamParser := pdfcontent.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return nil, err
	}
	q := &pdfcontent.ContentStreamOperation{Operand: "q"}
	Q := &pdfcontent.ContentStreamOperation{Operand: "Q"}
	processedOperations := &pdfcontent.ContentStreamOperations{q}
	processedXObjects := map[string]bool{} // Keep track of processed XObjects to avoid repetition.

	processor := pdfcontent.NewContentStreamProcessor(*operations)
	processor.AddHandler(pdfcontent.HandlerConditionEnumAllOperands, "",
		func(op *pdfcontent.ContentStreamOperation, gs pdfcontent.GraphicsState, resources *pdf.PdfPageResources) error {
			removed := false
			if op.Operand == "Do" {
				name := op.Params[0].(*pdfcore.PdfObjectName)
				// common.Log.Info("op=%+v", op)
				if _, ok := processedXObjects[string(*name)]; !ok {
					processedXObjects[string(*name)] = true
					_, xtype := resources.GetXObjectByName(*name)
					if xtype == pdf.XObjectTypeImage {
						xobjs := core.TraceToDirectObject(resources.XObject)
						// common.Log.Info("xobjs=%T", xobjs)
						xobjDict := xobjs.(*pdfcore.PdfObjectDictionary)
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
		fmt.Printf("Error processing: %v\n", err)
		return nil, err
	}
	*processedOperations = append(*processedOperations, Q)
	return processedOperations.Bytes(), nil
}

func readPages(inPath string) ([]*pdf.PdfPage, error) {
	f, err := os.Open(inPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pdfReader, err := pdf.NewPdfReader(f)
	if err != nil {
		return nil, err
	}

	isEncrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return nil, err
	}

	// Try decrypting with an empty one.
	if isEncrypted {
		auth, err := pdfReader.Decrypt([]byte(""))
		if err != nil {
			// Encrypted and we cannot do anything about it.
			return nil, err
		}
		if !auth {
			return nil, errors.New("Need to decrypt with password")
		}
	}

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return nil, err
	}
	common.Log.Info("PDF Num Pages: %d %q\n", numPages, inPath)

	pages := make([]*pdf.PdfPage, numPages)
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		common.Log.Info("Processing page %d/%d ", pageNum, numPages)
		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			return nil, err
		}
		// page.Duplicate()
		mbox, err := page.GetMediaBox()
		if err != nil {
			return nil, err
		}
		common.Log.Info("%.0f %d", *mbox, *page.Rotate)

		if page.Rotate != nil && (*page.Rotate == 90 || *page.Rotate == 270) {
			// TODO: This is a "hack" to change the perspective of the extractor to account for the rotation.
			contents, err := page.GetContentStreams()
			if err != nil {
				return nil, fmt.Errorf("GetContentStreams failed. %q pageNum=%d err=%w", inPath, pageNum, err)
			}

			cc := contentstream.NewContentCreator()
			// // cc.Add_cm(0, 1, 1, 0, 0, 0)
			// // cc.Add_cm(1, 0, 0, -1, 0, 0)
			// //  1  0  x  0  1   =  0  1
			// //  0 -1     1  0      -1 0
			// cc.Add_cm(0, 1, -1, 0, 0, 0)
			// cc.Translate(0, -mbox.Height())
			if *page.Rotate == 270 {
				cc.Add_cm(0, 1, -1, 0, mbox.Height(), 0)
			} else {
				cc.Add_cm(0, -1, 1, 0, 0, mbox.Width())
			}
			// cc.Add_cm(-1, 0, 0, -1, 1, 1)
			// // cc.Translate(mbox.Width()/2, mbox.Height()/2)
			// cc.RotateDeg(-float64(*page.Rotate))
			// // cc.Translate(-mbox.Height()/2, -mbox.Width()/2)
			rotateOps := cc.Operations().String()
			contents = append([]string{rotateOps}, contents...)

			page.Duplicate()
			if err = page.SetContentStreams(contents, core.NewRawEncoder()); err != nil {
				return nil, fmt.Errorf("SetContentStreams failed. %q pageNum=%d err=%w", inPath, pageNum, err)
			}
			page.Rotate = nil
		}
		pages[pageNum-1] = page
	}
	return pages, nil
}

func writePages(outPath string, pages []*pdf.PdfPage) error {
	pdfWriter := pdf.NewPdfWriter()
	for _, page := range pages {
		if err := pdfWriter.AddPage(page); err != nil {
			return err
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return pdfWriter.Write(f)
}
