/*
 * Write a subset of the pages in one PDF to another PDF.
 *
 * Run as: Usage: go run page_subset.go params.json
 */

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/unidoc/unipdf/v3/model"
)

func main() {
	outDir := "out.dir"
	flag.StringVar(&outDir, "o", outDir, "Write output files here.")
	if len(flag.Args()) < 1 || outDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: go run page_subset.go params.json\n")
		os.Exit(1)
	}

	if err := mkDir(outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't make output directory. err=%v\n", err)
		os.Exit(1)
	}

	for i, arg := range flag.Args() {
		params, err := readParams(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "readParams failed arg=%q err=%v\n", arg, err)
			os.Exit(1)
		}
		if err := params.apply(outDir); err != nil {
			fmt.Fprintf(os.Stderr, "apply failed arg=%q err=%v\n", arg, err)
			os.Exit(1)
		}
		fmt.Printf("%4d: wrote %q\n", i, params, outDir)
	}
}

type subset struct {
	InPath string
	Pages  []int
}

func (s subset) apply(outDir string) error {
	f, err := os.Open(s.InPath)
	if err != nil {
		return err
	}
	defer f.Close()
	pdfReader, err := model.NewPdfReaderLazy(f)
	if err != nil {
		return err
	}

	isEncrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return err
	}
	if isEncrypted {
		_, err = pdfReader.Decrypt([]byte(""))
		if err != nil {
			return err
		}
	}

	pdfWriter := model.NewPdfWriter()
	// for pageNum := pageFrom; pageNum <= pageTo; pageNum++ {
	for _, pageNum := range s.Pages {
		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			return err
		}
		err = pdfWriter.AddPage(page)
		if err != nil {
			return err
		}
	}

	fWrite, err := os.Create(s.outPath(outDir))
	if err != nil {
		return err
	}
	defer fWrite.Close()
	return pdfWriter.Write(fWrite)
}

func (s subset) outPath(outDir string) string {
	name := filepath.Base(s.InPath)
	return filepath.Join(outDir, name)
}

// ReadParams return the parameters encoded in the JSON file in `filename`.
func readParams(filename string) (subset, error) {
	var params subset
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return params, err
	}

	err = json.Unmarshal(b, &params)
	return params, err
}

// mkDir creates a directory called `dirname` if it doesn't already exist.
func mkDir(dirname string) error {
	if _, err := os.Stat(dirname); !os.IsNotExist(err) {
		if err != nil {
			return err
		}
		return nil
	}
	return os.MkdirAll(dirname, 0777)
}
