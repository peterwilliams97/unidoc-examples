/*
 * PDF to text: Extract all text for each page of a pdf file.
 *
 * Run as: go run pdf_extract_text.go input.pdf
 */

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar"
	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/contentstream"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

const (
	startIndex = 0
	usage      = "Usage: go run pdf_extract_text.go [options] <file1.pdf> <file2.pdf> ...\n"
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

func main() {
	var (
		firstPage, lastPage int
		debug, trace        bool
		outDir              string

		doProfile bool
	)
	flag.StringVar(&outDir, "o", "./outtext", "Output text (default outtext)")
	flag.IntVar(&firstPage, "f", -1, "First page")
	flag.IntVar(&lastPage, "l", 100000, "Last page")
	flag.BoolVar(&debug, "d", false, "Print debugging information.")
	flag.BoolVar(&trace, "e", false, "Print detailed debugging information.")
	flag.BoolVar(&doProfile, "p", false, "Save profiling information")
	makeUsage(usage)
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
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
			panic(fmt.Errorf("error loading UniDoc license: err=%w", err))
		}
		model.SetPdfCreator(companyName)
	}

	if outDir == "." || outDir == ".." {
		panic(fmt.Errorf("outDir=%q not allowed", outDir))
	}
	if outDir != "" {
		outDir, err := filepath.Abs(outDir)
		if err != nil {
			panic(fmt.Errorf("Abs failed. outDir=%q err=%w", outDir, err))
		}
		if err := os.MkdirAll(outDir, 0777); err != nil {
			panic(fmt.Errorf("Couldn't create outDir=%q err=%w", outDir, err))
		}
	}

	if doProfile {
		f, err := os.Create("cpu.profile")
		if err != nil {
			panic(fmt.Errorf("could not create CPU profile: ", err))
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			panic(fmt.Errorf("could not start CPU profile: ", err))
		}
		defer pprof.StopCPUProfile()
	}

	pathList, err := patternsToPaths(os.Args[1:])
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "%d PDF files", len(pathList))

	for i, inPath := range pathList {
		if len(pathList) > startIndex && i < startIndex {
			continue
		}
		if len(pathList) > 1 && isBadFile(inPath) {
			continue
		}

		outPath := changePath(outDir, filepath.Base(inPath), "", ".txt")
		if strings.ToLower(filepath.Ext(outPath)) == ".pdf" {
			panic(fmt.Errorf("output can't be PDF %q", outPath))
		}
		fmt.Printf("%4d of %d: %q\n", i+1, len(pathList), inPath)
		fmt.Fprintf(os.Stderr, "\n%4d of %d: ", i+1, len(pathList))
		t0 := time.Now()
		err, important := extractDocText(inPath, outPath, firstPage, lastPage, false)
		dt := time.Since(t0)
		fmt.Fprintf(os.Stderr, ": %.1f sec", dt.Seconds())
		if err != nil {
			if important {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			continue
		}
	}
	fmt.Fprintf(os.Stderr, "\nDONE\n")
}

// extractDocText extracts text columns pages `firstPage` to `lastPage` in PDF file `inPath` and
// outputs the data as an annotated text file to `outPath`.
func extractDocText(inPath, outPath string, firstPage, lastPage int, show bool) (error, bool) {
	common.Log.Info("extractDocText: inPath=%q [%d:%d]->%q", inPath, firstPage, lastPage, outPath)
	fmt.Fprintf(os.Stderr, "%q [%d:%d]->%q %.2f MB, ",
		inPath, firstPage, lastPage, outPath, fileSize(inPath))

	f, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("Could not open %q err=%w", inPath, err), false
	}
	defer f.Close()

	pdfReader, err := model.NewPdfReaderLazy(f)
	if err != nil {
		return fmt.Errorf("NewPdfReaderLazy failed. %q err=%w", inPath, err), false
	}
	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return fmt.Errorf("GetNumPages failed. %q err=%w", inPath, err), false
	}

	fmt.Fprintf(os.Stderr, "%d pages:", numPages)

	if firstPage < 1 {
		firstPage = 1
	}
	if lastPage > numPages {
		lastPage = numPages
	}

	var pageTexts []string

	for pageNum := firstPage; pageNum <= lastPage; pageNum++ {
		fmt.Fprintf(os.Stderr, "%d ", pageNum)
		text, err := getPageText(inPath, pdfReader, pageNum)
		if err != nil {
			return fmt.Errorf("getPageText failed. inPath=%q err=%w", inPath, err), true
		}
		pageTexts = append(pageTexts, text)
		if show {
			fmt.Println("------------------------------")
			fmt.Printf("Page %d:\n", pageNum)
			fmt.Printf("\"%s\"\n", text)
			fmt.Println("------------------------------")
		}
	}

	if outPath != "" {
		docText := strings.Join(pageTexts, "\n")
		if err := ioutil.WriteFile(outPath, []byte(docText), 0666); err != nil {
			return fmt.Errorf("failed to write outPath=%q err=%w", outPath, err), true
		}
	}
	return nil, false
}

func getPageText(inPath string, pdfReader *model.PdfReader, pageNum int) (string, error) {
	page, err := pdfReader.GetPage(pageNum)
	if err != nil {
		return "", fmt.Errorf("GetPage failed. %q pageNum=%d err=%w", inPath, pageNum, err)
	}

	mbox, err := page.GetMediaBox()
	if err != nil {
		return "[COULDN'T PROCESS]", nil
		return "", fmt.Errorf("GetMediaBox failed. %q pageNum=%d err=%w", inPath, pageNum, err)
	}
	if page.Rotate != nil && *page.Rotate == 90 {
		// TODO: This is a "hack" to change the perspective of the extractor to account for the rotation.
		contents, err := page.GetContentStreams()
		if err != nil {
			return "", fmt.Errorf("GetContentStreams failed. %q pageNum=%d err=%w", inPath, pageNum, err)
		}

		cc := contentstream.NewContentCreator()
		cc.Translate(mbox.Width()/2, mbox.Height()/2)
		cc.RotateDeg(-90)
		cc.Translate(-mbox.Width()/2, -mbox.Height()/2)
		rotateOps := cc.Operations().String()
		contents = append([]string{rotateOps}, contents...)

		page.Duplicate()
		if err = page.SetContentStreams(contents, core.NewRawEncoder()); err != nil {
			return "", fmt.Errorf("SetContentStreams failed. %q pageNum=%d err=%w", inPath, pageNum, err)
		}
		page.Rotate = nil
	}

	ex, err := extractor.New(page)
	if err != nil {
		if ignoreError(err) {
			return "[COULDN'T PROCESS]", nil
		}
		return "", fmt.Errorf("extractor.New failed. %q pageNum=%d err=%w", inPath, pageNum, err)
	}
	pageText, _, _, err := ex.ExtractPageText()
	if err != nil {
		if ignoreError(err) {
			return "[COULDN'T PROCESS]", nil
		}
		return "", fmt.Errorf("ExtractPageText failed. %q pageNum=%d err=%w", inPath, pageNum, err)
	}
	return pageText.Text(), nil
}
func patternsToPaths(patternList []string) ([]string, error) {
	var pathList []string
	common.Log.Debug("patternList=%d", len(patternList))
	for i, pattern := range patternList {
		pattern = ExpandUser(pattern)
		files, err := doublestar.Glob(pattern)
		if err != nil {
			common.Log.Error("PatternsToPaths: Glob failed. pattern=%#q err=%v", pattern, err)
			return pathList, err
		}
		common.Log.Debug("patternList[%d]=%q %d matches", i, pattern, len(files))
		for _, filename := range files {
			ok, err := RegularFile(filename)
			if err != nil {
				common.Log.Error("PatternsToPaths: RegularFile failed. pattern=%#q err=%v", pattern, err)
				return pathList, err
			}
			if !ok {
				continue
			}
			pathList = append(pathList, filename)
		}
	}
	// pathList = StringUniques(pathList)
	sort.Strings(pathList)
	return pathList, nil
}

// homeDir is the current user's home directory.
var homeDir = getHomeDir()

// getHomeDir returns the current user's home directory.
func getHomeDir() string {
	usr, _ := user.Current()
	return usr.HomeDir
}

// ExpandUser returns `filename` with "~"" replaced with user's home directory.
func ExpandUser(filename string) string {
	return strings.Replace(filename, "~", homeDir, -1)
}

// RegularFile returns true if file `filename` is a regular file.
func RegularFile(filename string) (bool, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return false, err
	}
	return fi.Mode().IsRegular(), nil
}

// fileSize returns the size of file `path` in bytes
func fileSize(path string) float64 {
	fi, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	return float64(fi.Size()) / 1024.0 / 1024.0
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
	common.Log.Debug("changePath(%q,%q,%q)->%q", dirName, base, ext, path)
	return path
}

func ignoreError(err error) bool {
	if err == nil {
		return true
	}
	errMsg := err.Error()
	for _, msg := range ignorableErrors {
		if strings.Contains(errMsg, msg) {
			return true
		}
	}
	return false
}

var ignorableErrors = []string{
	"unable to read",
	"media box not defined",
	"unsupported colorspace",
	"invalid filter in multi filter array",
}

func isBadFile(inPath string) bool {
	for _, fn := range badFiles {
		if strings.Contains(inPath, fn) {
			return true
		}
	}
	return false
}

var badFiles = []string{
	"/Users/peter/testdata/other/code/pdfbox/pdfbox/src/test/resources/input/sample_fonts_solidconvertor.pdf",
	"/Users/peter/testdata/other/code/pdfbox/pdmodel",
	"circularReferencesInResources.pdf",
	"cmp_lab_spot_based_gradient.pdf",
	"fallbackForBadColorSpace.pdf",
	"itextpdf/text/pdf/parser/PdfTextExtractorUnicodeIdentityTest/user10.pdf",
	"CompareToolTest/simple_pdf.pdf",
	"PDFBOX-3964-c687766d68ac766be3f02aaec5e0d713_2.pdf",
	"math_econ/theory.pdf",
	"xarc/0607648.pdf",
	"ud-test/testing/adobe.pdf.pdf",
	"ud-test/transform/outpit/000012.pdf",
	"ud-test/transform/outpit/0000",
	"ud-test/transform/output",
	"gazette.pdf",                  // media box not defined
	"pingpdf.com_lima-peru.pdf",    // crazy dimensions
	"cid_glyphs_2-7.pdf",           // default font is wrong for this Japanese text
	"otf_glyphs_2-7.pdf",           // default font is wrong for this Japanese text
	"genko_oc_shiryo1.pdf",         // Object name not starting with /
	"isartor-6-3-2-t01-fail-a.pdf", //  pageNum=1 err=table not found: head
	"isartor-6-3-2-t01-fail",
	"js.pdf",           // err=invalid content stream object holder (*core.PdfObjectDictionary)
	"pc-test/seg1.pdf", // err=invalid content stream object holder (*core.PdfObjectNull
}
