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

func splicePDFs(imagePath, textPath, outputPath string) error {
	pdfWriter := pdf.NewPdfWriter()

	common.Log.Info("Stripping %q", textPath)
	f, err := os.Open(textPath)
	if err != nil {
		return err
	}
	defer f.Close()

	pdfReader, err := pdf.NewPdfReader(f)
	if err != nil {
		return err
	}

	isEncrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return err
	}

	// Try decrypting with an empty one.
	if isEncrypted {
		auth, err := pdfReader.Decrypt([]byte(""))
		if err != nil {
			// Encrypted and we cannot do anything about it.
			return err
		}
		if !auth {
			return errors.New("Need to decrypt with password")
		}
	}

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return err
	}
	fmt.Printf("PDF Num Pages: %d\n", numPages)

	for i := 0; i < numPages; i++ {
		fmt.Printf("Processing page %d/%d\n", i+1, numPages)
		page, err := pdfReader.GetPage(i + 1)
		if err != nil {
			return err
		}

		err = stripImages(page)
		if err != nil {
			return err
		}

		err = pdfWriter.AddPage(page)
		if err != nil {
			return err
		}
	}

	fWrite, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer fWrite.Close()

	err = pdfWriter.Write(fWrite)
	if err != nil {
		return err
	}

	return nil
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
	processedOperations := &pdfcontent.ContentStreamOperations{}
	processedXObjects := map[string]bool{} // Keep track of processed XObjects to avoid repetition.

	processor := pdfcontent.NewContentStreamProcessor(*operations)
	processor.AddHandler(pdfcontent.HandlerConditionEnumAllOperands, "",
		func(op *pdfcontent.ContentStreamOperation, gs pdfcontent.GraphicsState, resources *pdf.PdfPageResources) error {
			removed := false
			if op.Operand == "Do" {
				name := op.Params[0].(*pdfcore.PdfObjectName)
				common.Log.Info("op=%+v", op)
				if _, ok := processedXObjects[string(*name)]; !ok {
					processedXObjects[string(*name)] = true
					_, xtype := resources.GetXObjectByName(*name)
					if xtype == pdf.XObjectTypeImage {
						xobjs := core.TraceToDirectObject(resources.XObject)
						common.Log.Info("xobjs=%T", xobjs)
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
	return processedOperations.Bytes(), nil
}
