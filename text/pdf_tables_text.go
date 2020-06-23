/*
 * Extract all text and tables from the specified pages of one or more PDF files.
 *
 * Run as: go run pdf_tables_text.go input.pdf
 */

package main

import (
	"bytes"
	"encoding/csv"
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
	usage = "Usage: go run pdf_tables_text.go [options] <file1.pdf> <file2.pdf> ...\n"
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
		firstPage, lastPage     int
		outDir, csvDir          string
		debug, trace, doProfile bool
		repeats                 int
	)
	flag.StringVar(&outDir, "o", "./outtext", "Output text (default outtext). Set to \"\" to not save.")
	flag.StringVar(&csvDir, "c", "./outcsv", "Output CSVs (default outtext). Set to \"\" to not save.")
	flag.IntVar(&firstPage, "f", -1, "First page")
	flag.IntVar(&lastPage, "l", 100000, "Last page")
	flag.IntVar(&repeats, "r", 1, "repeat each page extraction this many time")
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

	makeDir("outDir", outDir)
	makeDir("csvDir", csvDir)

	if doProfile {
		f, err := os.Create("cpu.profile")
		if err != nil {
			panic(fmt.Errorf("could not create CPU profile: ", err))
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			panic(fmt.Errorf("could not start CPU profile: ", err))
		}
		defer pprof.StopCPUProfile()
	}

	pathList, err := patternsToPaths(os.Args[1:])
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d PDF files", len(pathList))

	var performances []performance

	t0 := time.Now()
	for i, inPath := range pathList {
		if len(pathList) > startIndex && i < startIndex {
			continue
		}
		if len(pathList) > 1 && isBadFile(inPath) {
			continue
		}

		outPath := changeDirExt(outDir, filepath.Base(inPath), "", ".txt")
		csvPath := changeDirExt(csvDir, filepath.Base(inPath), "", "")
		if strings.ToLower(filepath.Ext(outPath)) == ".pdf" {
			panic(fmt.Errorf("output can't be PDF %q", outPath))
		}
		fmt.Printf("%4d of %d: %q ", i+1, len(pathList), inPath)
		var perf performance
		err, important := extractDocText(inPath, outPath, csvPath, firstPage, lastPage, repeats, false, &perf)
		fmt.Printf(": %.1f sec\n", perf.dt)
		if err != nil {
			if important {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			continue
		}
		performances = append(performances, perf)
		// if len(performances)%20 == 0 {
		// 	logPeformances(performances)
		// }
	}
	dt := time.Since(t0)
	fmt.Printf("\nDONE %.1f seconds\n", dt.Seconds())
	logPeformances(performances)
}

func logPeformances(performances []performance) {
	sort.Slice(performances, func(i, j int) bool {
		pi, pj := performances[i], performances[j]
		if pi.perPage() != pj.perPage() {
			return pi.perPage() > pj.perPage()
		}
		if pi.dt != pj.dt {
			return pi.dt > pj.dt
		}
		return pi.name < pj.name
	})
	fmt.Printf("\n%d tests -------------------------\n", len(performances))
	for i, p := range performances {
		fmt.Printf("%4d: %s\n", i, p)
	}
}

type performance struct {
	name  string
	dt    float64
	pages int
}

func (p performance) perPage() float64 { return p.dt / float64(p.pages) }

func (p performance) String() string {
	return fmt.Sprintf("%2d pages %5.1f sec %5.2f sec/page %s", p.pages, p.dt, p.perPage(), p.name)
}

// extractDocText extracts text columns pages `firstPage` to `lastPage` in PDF file `inPath` and
//   - writes the extracted texe to `outPath`.
//   - writes any extracted tables to `csvPath`
func extractDocText(inPath, outPath, csvPath string, firstPage, lastPage, repeats int, show bool,
	perf *performance) (error, bool) {
	fmt.Printf("%q [%d:%d]->%q %.2f MB, ",
		inPath, firstPage, lastPage, outPath, fileSize(inPath))

	t0 := time.Now()
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

	fmt.Printf("%d pages:", numPages)

	if firstPage < 1 {
		firstPage = 1
	}
	if lastPage > numPages {
		lastPage = numPages
	}

	var pageTexts []string
	var pageTables [][]string

	for pageNum := firstPage; pageNum <= lastPage; pageNum++ {
		fmt.Printf("%d ", pageNum)
		text, tables, err := extractAllPageContents(inPath, pdfReader, pageNum, repeats)
		if err != nil {
			return fmt.Errorf("extractAllPageContents failed. inPath=%q err=%w", inPath, err), true
		}
		pageTexts = append(pageTexts, text)
		if show {
			fmt.Println("----------------------------------------------------------------------")
			fmt.Printf("Page %d:\n", pageNum)
			fmt.Printf("\"%s\"\n", text)
			fmt.Println("----------------------------------------------------------------------")
		}
		pageTables = append(pageTables, tables)
	}
	perf.name = inPath
	perf.pages = lastPage - firstPage + 1
	perf.dt = time.Since(t0).Seconds()

	if outPath != "" {
		docText := strings.Join(pageTexts, "\n")
		if err := ioutil.WriteFile(outPath, []byte(docText), 0666); err != nil {
			return fmt.Errorf("failed to write outPath=%q err=%w", outPath, err), true
		}
	}
	if csvPath != "" {
		for i, tables := range pageTables {
			if len(tables) == 0 {
				continue
			}
			fmt.Printf("page%d: %d tables\n", i+1, len(pageTables))
			for j, table := range tables {
				csvPath := fmt.Sprintf("%s.page%d.table%d.csv", csvPath, i+1, j+1)
				if err := ioutil.WriteFile(csvPath, []byte(table), 0666); err != nil {
					return fmt.Errorf("failed to write csvPath=%q err=%w", csvPath, err), true
				}
			}
		}
	}
	return nil, false
}

// extractAllPageContents extracts the text and tables from (1-offset) page number `pageNum` in opened
// PdfReader `pdfReader.
// - The first return is the extracted text.
// - The second return is the csv encoded contents of any tables found on the page.
func extractAllPageContents(inPath string, pdfReader *model.PdfReader, pageNum, repeats int) (string, []string, error) {
	var text string
	var tables []string
	var err error
	for i := 0; i < repeats; i++ {
		text, tables, err = _extractAllPageContents(inPath, pdfReader, pageNum)
		if err != nil {
			return "", nil, err
		}
	}
	return text, tables, nil
}

func _extractAllPageContents(inPath string, pdfReader *model.PdfReader, pageNum int) (string, []string, error) {
	page, err := pdfReader.GetPage(pageNum)
	if err != nil {
		return "", nil, fmt.Errorf("GetPage failed. %q pageNum=%d err=%w", inPath, pageNum, err)
	}

	mbox, err := page.GetMediaBox()
	if err != nil {
		return "[COULDN'T PROCESS]", nil, nil
	}
	fmt.Printf("%.0f ", *mbox)
	if page.Rotate != nil && *page.Rotate == 90 {
		// TODO: This is a "hack" to change the perspective of the extractor to account for the rotation.
		contents, err := page.GetContentStreams()
		if err != nil {
			return "", nil, fmt.Errorf("GetContentStreams failed. %q pageNum=%d err=%w", inPath, pageNum, err)
		}

		cc := contentstream.NewContentCreator()
		cc.Translate(mbox.Width()/2, mbox.Height()/2)
		cc.RotateDeg(-90)
		cc.Translate(-mbox.Width()/2, -mbox.Height()/2)
		rotateOps := cc.Operations().String()
		contents = append([]string{rotateOps}, contents...)

		page.Duplicate()
		if err = page.SetContentStreams(contents, core.NewRawEncoder()); err != nil {
			return "", nil, fmt.Errorf("SetContentStreams failed. %q pageNum=%d err=%w", inPath, pageNum, err)
		}
		page.Rotate = nil
	}

	ex, err := extractor.New(page)
	if err != nil {
		if ignoreError(err) {
			return "[COULDN'T PROCESS]", nil, nil
		}
		return "", nil, fmt.Errorf("extractor.New failed. %q pageNum=%d err=%w", inPath, pageNum, err)
	}
	pageText, _, _, err := ex.ExtractPageText()
	if err != nil {
		if ignoreError(err) {
			return "[COULDN'T PROCESS]", nil, nil
		}
		return "", nil, fmt.Errorf("ExtractPageText failed. %q pageNum=%d err=%w", inPath, pageNum, err)
	}
	var tables []string
	for _, table := range pageText.Tables() {
		tables = append(tables, toCsv(table))
	}
	// marks := pageText.Marks().Elements()
	// common.Log.Info("%d marks =====================")
	// for i, tm := range marks {
	// 	fmt.Printf("%4d: %s\n", i, tm.String())
	// }
	return pageText.Text(), tables, nil
}

// toCsv return the contents of `table` encoded as CSV.
func toCsv(table extractor.TextTable) string {
	b := new(bytes.Buffer)
	csvwriter := csv.NewWriter(b)
	// csvwriter.Comma = '\t'
	for y, row := range table.Cells {
		if len(row) != table.W {
			err := fmt.Errorf("table = %d x %d row[%d]=%d %d", table.W, table.H, y, len(row), row)
			panic(err)
		}
		csvwriter.Write(row)
	}
	csvwriter.Flush()
	return b.String()
}

// patternsToPaths returns the file paths matched by the patterns in `patternList`.
func patternsToPaths(patternList []string) ([]string, error) {
	var pathList []string
	common.Log.Debug("patternList=%d", len(patternList))
	for i, pattern := range patternList {
		pattern = expandUser(pattern)
		files, err := doublestar.Glob(pattern)
		if err != nil {
			common.Log.Error("PatternsToPaths: Glob failed. pattern=%#q err=%v", pattern, err)
			return pathList, err
		}
		common.Log.Debug("patternList[%d]=%q %d matches", i, pattern, len(files))
		for _, filename := range files {
			ok, err := regularFile(filename)
			if err != nil {
				common.Log.Error("PatternsToPaths: regularFile failed. pattern=%#q err=%v", pattern, err)
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

// expandUser returns `filename` with "~"" replaced with user's home directory.
func expandUser(filename string) string {
	return strings.Replace(filename, "~", homeDir, -1)
}

// regularFile returns true if file `filename` is a regular file.
func regularFile(filename string) (bool, error) {
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

// makeDir creates `outDir`. Name is the name of `outDir` in the calling code.
func makeDir(name, outDir string) {
	if outDir == "." || outDir == ".." {
		panic(fmt.Errorf("%s=%q not allowed", name, outDir))
	}
	if outDir == "" {
		return
	}

	outDir, err := filepath.Abs(outDir)
	if err != nil {
		panic(fmt.Errorf("Abs failed. %s=%q err=%w", name, outDir, err))
	}
	if err := os.MkdirAll(outDir, 0777); err != nil {
		panic(fmt.Errorf("Couldn't create %s=%q err=%w", name, outDir, err))
	}
}

// changeDirExt inserts `qualifier` into `filename` before its extension then changes its
// directory to `dirName` and extrension to `extName`,
func changeDirExt(dirName, filename, qualifier, extName string) string {
	if dirName == "" {
		return ""
	}
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	base = base[:len(base)-len(ext)]
	if len(qualifier) > 0 {
		base = fmt.Sprintf("%s.%s", base, qualifier)
	}
	filename = fmt.Sprintf("%s%s", base, extName)
	path := filepath.Join(dirName, filename)
	common.Log.Debug("changeDirExt(%q,%q,%q)->%q", dirName, base, extName, path)
	return path
}

// ignoreError returns true if `err` should be ignored.
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

const startIndex = 0
