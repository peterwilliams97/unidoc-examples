/*
 * Write a subset of the pages in one PDF to another PDF.
 *
 * Run as: Usage: go run page_subset.go params.json
 */

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/model"
)

const (
	uniDocLicenseKey = ``
	companyName      = "PaperCut Software International Pty Ltd"
	creatorName      = "PaperCut Software International Pty Ltd"
)

func init() {
	err := license.SetLicenseKey(uniDocLicenseKey, companyName)
	if err != nil {
		common.Log.Error("err=%v", err)
		panic(err)
	}
	model.SetPdfCreator(creatorName)
	common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))
}

func main() {
	outDir := "out.dir"
	var lazy bool
	flag.StringVar(&outDir, "o", outDir, "Write output files here.")
	flag.BoolVar(&lazy, "l", false, "Use lazy reading.")
	flag.Parse()
	if len(flag.Args()) < 1 || outDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: go run page_subset.go params.json\n\targs=%q\n\toutDir=%q\n",
			flag.Args(), outDir)
		os.Exit(1)
	}

	fmt.Printf("args=%q\n", flag.Args())
	if err := mkDir(outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't make output directory. err=%v\n", err)
		os.Exit(2)
	}

	var failures []string
	for i, arg := range flag.Args() {
		// fmt.Printf("%4d: processing %q\n", i, arg)
		params, err := readParams(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "readParams failed arg=%q err=%v\n", arg, err)
			os.Exit(3)
		}
		if err := params.apply(outDir, lazy); err != nil {
			fmt.Fprintf(os.Stderr, "apply failed arg=%q err=%v\n", arg, err)
			os.Exit(4)
		}
		outPath := params.outPath(outDir)
		fmt.Fprintf(os.Stderr, "%4d: %3d pages %20q %4.1f MB -> %4.1f MB %q\n",
			i, len(params.Pages),
			params.InPath, fileSizeMB(params.InPath),
			fileSizeMB(outPath), outPath)
		// fmt.Printf("%4d: wrote %q\n", i, outPath)
		if err := checkPDF(outPath); err != nil {
			if err2 := checkPDF(params.InPath); err2 == nil {
				fmt.Fprintf(os.Stderr, "bad PDF=%q err=\n\t%v\n", outPath, err)
				failures = append(failures, arg)
			}
		}
	}
	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "%d failures of %d ======================\n",
			len(failures), len(flag.Args()))
		for i, arg := range failures {
			fmt.Fprintf(os.Stderr, "%4d: %s\n", i, arg)
		}
	}
}

// subset is processing instructions to create a PDF from the (1-offset) page numbers `Pages` from
// PDF `InPath`.
type subset struct {
	InPath string
	Pages  []int
}

// apply creates the PDF based on the instructions in `s` and writes it to `outDir`.
func (s subset) apply(outDir string, lazy bool) error {
	f, err := os.Open(s.InPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var pdfReader *model.PdfReader
	if lazy {
		pdfReader, err = model.NewPdfReaderLazy(f)
	} else {
		pdfReader, err = model.NewPdfReader(f)
	}
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

	pages := make([]*model.PdfPage, len(s.Pages))
	for i, pageNum := range s.Pages {
		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			return err
		}
		pages[i] = page
	}

	pdfWriter := model.NewPdfWriter()
	for i, pageNum := range s.Pages {
		page, err := pdfReader.GetPage(pageNum)
		if err != nil {
			return err
		}
		common.Log.Debug("***NEW PAGE %d was %d", i+1, pageNum)
		if err = pdfWriter.AddPage(page); err != nil {
			return fmt.Errorf("apply: s=%+v AddPage(pageNum=%d) err=%w", s, pageNum, err)
		}
		common.Log.Debug("***DONE PAGE %d was %d", i+1, pageNum)
	}

	fWrite, err := os.Create(s.outPath(outDir))
	if err != nil {
		return err
	}
	defer fWrite.Close()
	if err := pdfWriter.Write(fWrite); err != nil {
		return fmt.Errorf("apply: s=%+v pdfWriter.Write err=%w", s, err)
	}
	return nil
}

// outPath returns the path of `s` gets copied to in `outDir`.
func (s subset) outPath(outDir string) string {
	name := filepath.Base(s.InPath)
	return filepath.Join(outDir, name)
}

// ReadParams returns the parameters encoded in the JSON file in `filename`.
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

// checkPDF checks if `filename` is a valid PDF.
func checkPDF(filename string) error {
	prog := "pdftotext"
	args := []string{filename, "-"}
	_, stderr, err := run(prog, args)
	stderr = strings.Trim(stderr, " \t\n")
	if err != nil || len(stderr) > 0 {
		if len(stderr) > 100 {
			stderr = stderr[:100]
		}
		return fmt.Errorf("prog=%q args=%+v\n\terr=%v\n\tstderr=<%s>", prog, args, err, stderr)
	}
	return nil
}

// run executes binary `prog` with arguments `args` and returns its stdout and stderr.
func run(prog string, args []string) (string, string, error) {
	cmd := exec.Command(prog, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = bufio.NewWriter(&stdout)
	cmd.Stderr = bufio.NewWriter(&stderr)
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

// fileSizeMB returns the size of file `path` in megabytes.
func fileSizeMB(path string) float64 {
	fi, err := os.Stat(path)
	if err != nil {
		return -1.0
	}
	return float64(fi.Size()) / 1024.0 / 1024.0
}
