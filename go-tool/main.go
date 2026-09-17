// Command prepare-for-print is a novice-friendly, double-click-able tool that
// prepares PNG images for print, tuned to CatPrint's file requirements
// (300 DPI minimum, CMYK color mode). It runs unmodified on both macOS and
// Windows (see README.md in this folder for exact run instructions on each).
//
// How a novice uses it:
//  1. Drag one or more PNG files (or a folder of PNGs) onto the program's
//     icon -- or just double-click the program and it will create an
//     "input" folder next to itself and wait for you to put files there.
//  2. Answer a few plain-English questions (or just press Enter to accept
//     the sensible defaults shown in brackets).
//  3. Find the finished files in the "output" folder next to the program.
//
// Under the hood it mirrors the logic of the project's prepare_for_print.py
// script:
//   - Computes exact pixel dimensions from physical size + DPI (never trusts
//     embedded image metadata for that math).
//   - Resamples with a high-quality Lanczos filter.
//   - Warns loudly if the source image would need significant upscaling.
//   - Supports fit / crop / stretch modes.
//   - Converts to CMYK by default (CatPrint prints CMYK, not RGB), saved as
//     TIFF since PNG has no CMYK support; RGB output stays PNG.
//   - Embeds the correct DPI into the output file's metadata.
//
// Only third-party dependency: github.com/disintegration/imaging, a pure-Go
// (no cgo) image resampling library, so this cross-compiles trivially for
// both macOS and Windows from a single machine with no system dependencies.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// DefaultDPI is CatPrint's stated minimum resolution for print-ready files.
	DefaultDPI = 300

	// MMPerInch converts millimeters to inches.
	MMPerInch = 25.4
)

var fitModes = []string{"fit", "crop", "stretch"}

// ---------------------------------------------------------------------------
// Options + result types
// ---------------------------------------------------------------------------

// Options controls how a single image is prepared for print.
type Options struct {
	Width, Height float64 // physical target size, in Units
	Units         string  // "in" or "mm"
	DPI           int     // target dots per inch
	FitMode       string  // "fit", "crop", or "stretch"
	CMYK          bool    // convert to CMYK (true) or keep RGB (false)
}

// PrintResult summarizes what happened to one file, for the console report.
type PrintResult struct {
	SourcePath              string
	OutputPath              string
	OrigW, OrigH            int
	FinalW, FinalH          int
	DPI                     int
	WidthUnits, HeightUnits float64
	Units                   string
}

// ---------------------------------------------------------------------------
// Geometry helpers
// ---------------------------------------------------------------------------

// toInches converts a physical dimension to inches given its unit label.
func toInches(value float64, units string) (float64, error) {
	switch strings.ToLower(units) {
	case "in", "inch", "inches":
		return value, nil
	case "mm", "millimeter", "millimeters":
		return value / MMPerInch, nil
	default:
		return 0, fmt.Errorf("unsupported units %q (use 'in' or 'mm')", units)
	}
}

// targetPixelSize computes the exact pixel dimensions required to print at
// width x height (in the given units) at dpi dots per inch. This is done
// with arithmetic, not by trusting any DPI value embedded in the source file.
func targetPixelSize(width, height float64, units string, dpi int) (int, int, error) {
	wIn, err := toInches(width, units)
	if err != nil {
		return 0, 0, err
	}
	hIn, err := toInches(height, units)
	if err != nil {
		return 0, 0, err
	}
	return int(math.Round(wIn * float64(dpi))), int(math.Round(hIn * float64(dpi))), nil
}

// maxSafePrintSizeInches returns the largest physical size (in inches) a
// source image can be printed at without dropping below dpi.
func maxSafePrintSizeInches(srcW, srcH, dpi int) (float64, float64) {
	return float64(srcW) / float64(dpi), float64(srcH) / float64(dpi)
}

// ---------------------------------------------------------------------------
// Color conversion (RGB/RGBA -> CMYK)
// ---------------------------------------------------------------------------

// flattenToWhite composites any transparency in img onto a white background,
// since CMYK (and print in general) has no concept of an alpha channel.
func flattenToWhite(img *image.NRGBA) *image.NRGBA {
	bounds := img.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			px := img.NRGBAAt(x, y)
			if px.A == 255 {
				out.SetNRGBA(x, y, color.NRGBA{R: px.R, G: px.G, B: px.B, A: 255})
				continue
			}
			af := float64(px.A) / 255.0
			blend := func(c uint8) uint8 {
				return uint8(math.Round(float64(c)*af + 255*(1-af)))
			}
			out.SetNRGBA(x, y, color.NRGBA{R: blend(px.R), G: blend(px.G), B: blend(px.B), A: 255})
		}
	}
	return out
}

// toCMYK converts a flattened (fully opaque) NRGBA image to CMYK using
// Go's standard (uncalibrated) RGB->CMYK conversion.
func toCMYK(img *image.NRGBA) *image.CMYK {
	bounds := img.Bounds()
	out := image.NewCMYK(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			px := img.NRGBAAt(x, y)
			c, m, ye, k := color.RGBToCMYK(px.R, px.G, px.B)
			out.SetCMYK(x, y, color.CMYK{C: c, M: m, Y: ye, K: k})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// CMYK TIFF encoder
//
// Go's standard library has no CMYK image writer at all (image/png cannot
// represent CMYK, and golang.org/x/image/tiff's encoder does not support the
// *image.CMYK type). This is a minimal, uncompressed, baseline-TIFF encoder
// good enough for print workflows: one strip, 8 bits/sample, 4 samples/pixel,
// PhotometricInterpretation=CMYK, plus an embedded DPI resolution tag.
// ---------------------------------------------------------------------------

func encodeCMYKTIFF(w io.Writer, img *image.CMYK, dpi int) error {
	bounds := img.Bounds()
	width := uint32(bounds.Dx())
	height := uint32(bounds.Dy())

	var buf bytes.Buffer

	// --- TIFF header: byte order, magic number, offset to first IFD. ---
	buf.WriteString("II")
	_ = binary.Write(&buf, binary.LittleEndian, uint16(42))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(8)) // IFD starts right after header

	type entry struct {
		tag, typ     uint16
		count, value uint32
	}
	// Entries MUST be in ascending tag order per the TIFF spec.
	const numEntries = 12
	ifdSize := 2 + numEntries*12 + 4 // count + entries + next-IFD offset
	dataOffset := uint32(8 + ifdSize)

	bitsPerSampleOffset := dataOffset // 4 x uint16 = 8 bytes
	xResOffset := bitsPerSampleOffset + 8
	yResOffset := xResOffset + 8
	pixelDataOffset := yResOffset + 8
	stripByteCount := width * height * 4

	entries := []entry{
		{256, 4, 1, width},               // ImageWidth (LONG)
		{257, 4, 1, height},              // ImageLength (LONG)
		{258, 3, 4, bitsPerSampleOffset}, // BitsPerSample: 8,8,8,8 (SHORT x4, needs offset)
		{259, 3, 1, 1},                   // Compression = none
		{262, 3, 1, 5},                   // PhotometricInterpretation = CMYK
		{273, 4, 1, pixelDataOffset},     // StripOffsets
		{277, 3, 1, 4},                   // SamplesPerPixel = 4 (C, M, Y, K)
		{278, 4, 1, height},              // RowsPerStrip (single strip)
		{279, 4, 1, stripByteCount},      // StripByteCounts
		{282, 5, 1, xResOffset},          // XResolution (RATIONAL)
		{283, 5, 1, yResOffset},          // YResolution (RATIONAL)
		{296, 3, 1, 2},                   // ResolutionUnit = inches
	}

	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(entries)))
	for _, e := range entries {
		_ = binary.Write(&buf, binary.LittleEndian, e.tag)
		_ = binary.Write(&buf, binary.LittleEndian, e.typ)
		_ = binary.Write(&buf, binary.LittleEndian, e.count)
		_ = binary.Write(&buf, binary.LittleEndian, e.value)
	}
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0)) // no next IFD

	// BitsPerSample array data: 8 bits for each of C, M, Y, K.
	for i := 0; i < 4; i++ {
		_ = binary.Write(&buf, binary.LittleEndian, uint16(8))
	}
	// XResolution / YResolution as RATIONAL (numerator, denominator) = dpi/1.
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dpi))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dpi))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(1))

	// Raw CMYK pixel data, row by row (image.CMYK rows may be padded by
	// Stride, so we must slice each row out by PixOffset rather than
	// assuming Pix is tightly packed).
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		rowStart := img.PixOffset(bounds.Min.X, y)
		buf.Write(img.Pix[rowStart : rowStart+int(width)*4])
	}

	_, err := w.Write(buf.Bytes())
	return err
}

// ---------------------------------------------------------------------------
// PNG encoder with embedded DPI (pHYs chunk)
//
// image/png has no option to set physical resolution, so we encode normally
// and then splice a pHYs chunk in right after IHDR (the position mandated by
// the PNG spec for chunk ordering).
// ---------------------------------------------------------------------------

func buildPNGChunk(chunkType string, data []byte) []byte {
	out := make([]byte, 0, 12+len(data))
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
	out = append(out, lengthBytes...)

	typeAndData := append([]byte(chunkType), data...)
	out = append(out, typeAndData...)

	crcBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(crcBytes, crc32.ChecksumIEEE(typeAndData))
	out = append(out, crcBytes...)
	return out
}

func encodePNGWithDPI(w io.Writer, img image.Image, dpi int) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	data := buf.Bytes()

	// pHYs stores pixels-per-unit; we use meters, so convert DPI (dots per
	// inch) to dots per meter (1 inch = 0.0254 meters).
	ppm := uint32(math.Round(float64(dpi) / 0.0254))
	physData := make([]byte, 9)
	binary.BigEndian.PutUint32(physData[0:4], ppm)
	binary.BigEndian.PutUint32(physData[4:8], ppm)
	physData[8] = 1 // unit specifier: 1 = meter
	physChunk := buildPNGChunk("pHYs", physData)

	// The 8-byte PNG signature is followed immediately by the IHDR chunk;
	// insert our new chunk right after it.
	const sigLen = 8
	ihdrDataLen := binary.BigEndian.Uint32(data[sigLen : sigLen+4])
	ihdrChunkTotalLen := 4 + 4 + int(ihdrDataLen) + 4 // length + type + data + crc
	insertPos := sigLen + ihdrChunkTotalLen

	out := make([]byte, 0, len(data)+len(physChunk))
	out = append(out, data[:insertPos]...)
	out = append(out, physChunk...)
	out = append(out, data[insertPos:]...)

	_, err := w.Write(out)
	return err
}

// ---------------------------------------------------------------------------
// Core per-file processing
// ---------------------------------------------------------------------------

func stemName(p string) string {
	base := filepath.Base(p)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// prepareForPrint prepares a single PNG for print and saves it into outputDir.
func prepareForPrint(inputPath, outputDir string, opt Options) (PrintResult, error) {
	valid := false
	for _, m := range fitModes {
		if opt.FitMode == m {
			valid = true
			break
		}
	}
	if !valid {
		return PrintResult{}, fmt.Errorf("fit mode must be one of %v, got %q", fitModes, opt.FitMode)
	}

	f, err := os.Open(inputPath)
	if err != nil {
		return PrintResult{}, err
	}
	defer f.Close()

	srcImg, err := png.Decode(f)
	if err != nil {
		return PrintResult{}, fmt.Errorf("could not decode PNG: %w", err)
	}

	origBounds := srcImg.Bounds()
	origW, origH := origBounds.Dx(), origBounds.Dy()

	// 1. Compute exact required pixel dimensions from physical size + DPI.
	//    We never trust the source file's embedded DPI for this calculation.
	targetW, targetH, err := targetPixelSize(opt.Width, opt.Height, opt.Units, opt.DPI)
	if err != nil {
		return PrintResult{}, err
	}

	// 2. Warn if we would be upscaling significantly beyond what the source
	//    resolution safely supports at the target DPI.
	maxSafeWIn, maxSafeHIn := maxSafePrintSizeInches(origW, origH, opt.DPI)
	reqWIn, _ := toInches(opt.Width, opt.Units)
	reqHIn, _ := toInches(opt.Height, opt.Units)

	upW := math.Inf(1)
	if maxSafeWIn > 0 {
		upW = reqWIn / maxSafeWIn
	}
	upH := math.Inf(1)
	if maxSafeHIn > 0 {
		upH = reqHIn / maxSafeHIn
	}

	// "Significantly" upscaling = requesting more than ~10% beyond the size
	// the source can natively support at the target DPI.
	if upW > 1.1 || upH > 1.1 {
		maxSafeW, maxSafeH := maxSafeWIn, maxSafeHIn
		if strings.EqualFold(opt.Units, "mm") {
			maxSafeW *= MMPerInch
			maxSafeH *= MMPerInch
		}
		fmt.Printf(
			"WARNING: '%s' is being upscaled beyond its safe print size.\n"+
				"  Source resolution: %dx%d px\n"+
				"  Requested size: %g%s x %g%s @ %d DPI\n"+
				"  Maximum safe print size at %d DPI: %.2f%s x %.2f%s\n"+
				"  CatPrint recommends at least %d DPI at final size to avoid pixelation.\n",
			filepath.Base(inputPath), origW, origH,
			opt.Width, opt.Units, opt.Height, opt.Units, opt.DPI,
			opt.DPI, maxSafeW, opt.Units, maxSafeH, opt.Units, opt.DPI,
		)
	}

	// 3. Normalize to NRGBA, then resample using the selected fit mode with
	//    high-quality Lanczos resampling.
	nrgba := imaging.Clone(srcImg)

	var resized *image.NRGBA
	switch opt.FitMode {
	case "stretch":
		// Both dimensions given explicitly -> imaging.Resize distorts to fit exactly.
		resized = imaging.Resize(nrgba, targetW, targetH, imaging.Lanczos)
	case "crop":
		// Scales to cover the target box, then center-crops the overflow.
		resized = imaging.Fill(nrgba, targetW, targetH, imaging.Center, imaging.Lanczos)
	default: // "fit"
		// Scales to fit entirely within the target box, then pads (centered)
		// with white to reach the exact target size.
		fitted := imaging.Fit(nrgba, targetW, targetH, imaging.Lanczos)
		background := imaging.New(targetW, targetH, color.White)
		resized = imaging.PasteCenter(background, fitted)
	}

	// 4 & 5. Convert color mode and pick output format/extension.
	//    CMYK is the default since CatPrint prints in CMYK. CMYK has no PNG
	//    support, so CMYK output is saved as TIFF; RGB output stays PNG.
	var outputPath string
	if opt.CMYK {
		flattened := flattenToWhite(resized)
		cmykImg := toCMYK(flattened)
		outputPath = filepath.Join(outputDir, stemName(inputPath)+"_print.tiff")
		outFile, err := os.Create(outputPath)
		if err != nil {
			return PrintResult{}, err
		}
		defer outFile.Close()
		// 6. Embed the correct DPI metadata so it matches the resampled pixels.
		if err := encodeCMYKTIFF(outFile, cmykImg, opt.DPI); err != nil {
			return PrintResult{}, err
		}
	} else {
		outputPath = filepath.Join(outputDir, stemName(inputPath)+"_print.png")
		outFile, err := os.Create(outputPath)
		if err != nil {
			return PrintResult{}, err
		}
		defer outFile.Close()
		if err := encodePNGWithDPI(outFile, resized, opt.DPI); err != nil {
			return PrintResult{}, err
		}
	}

	// 7. Compute final physical size for the summary.
	finalWUnits := float64(targetW) / float64(opt.DPI)
	finalHUnits := float64(targetH) / float64(opt.DPI)
	if strings.EqualFold(opt.Units, "mm") {
		finalWUnits *= MMPerInch
		finalHUnits *= MMPerInch
	}

	return PrintResult{
		SourcePath:  inputPath,
		OutputPath:  outputPath,
		OrigW:       origW,
		OrigH:       origH,
		FinalW:      targetW,
		FinalH:      targetH,
		DPI:         opt.DPI,
		WidthUnits:  round3(finalWUnits),
		HeightUnits: round3(finalHUnits),
		Units:       opt.Units,
	}, nil
}

func printSummary(r PrintResult) {
	fmt.Printf("%s -> %s\n", filepath.Base(r.SourcePath), filepath.Base(r.OutputPath))
	fmt.Printf("  Original resolution: %dx%d px\n", r.OrigW, r.OrigH)
	fmt.Printf("  Final resolution:    %dx%d px\n", r.FinalW, r.FinalH)
	fmt.Printf("  Final DPI:           %d\n", r.DPI)
	fmt.Printf("  Final physical size: %g%s x %g%s\n", r.WidthUnits, r.Units, r.HeightUnits, r.Units)
}

// ---------------------------------------------------------------------------
// Gathering input files (single files, folders, mixed)
// ---------------------------------------------------------------------------

func isPNG(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".png")
}

// gatherPNGs expands a list of file/folder paths into a flat, sorted list of
// PNG file paths. Folders are scanned non-recursively (top-level files only).
func gatherPNGs(paths []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Printf("  (skipping %s: %v)\n", p, err)
			continue
		}
		if info.IsDir() {
			entries, err := os.ReadDir(p)
			if err != nil {
				fmt.Printf("  (skipping folder %s: %v)\n", p, err)
				continue
			}
			for _, e := range entries {
				if !e.IsDir() && isPNG(e.Name()) {
					full := filepath.Join(p, e.Name())
					if !seen[full] {
						seen[full] = true
						out = append(out, full)
					}
				}
			}
		} else if isPNG(p) {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Interactive "novice mode" wizard
// ---------------------------------------------------------------------------

func askString(reader *bufio.Reader, prompt, def string) string {
	fmt.Printf("%s [%s]: ", prompt, def)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func askFloat(reader *bufio.Reader, prompt string, def float64) float64 {
	for {
		raw := askString(reader, prompt, strconv.FormatFloat(def, 'g', -1, 64))
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			fmt.Println("  Please enter a number.")
			continue
		}
		return v
	}
}

func askChoice(reader *bufio.Reader, prompt, def string, choices []string) string {
	for {
		raw := strings.ToLower(askString(reader, fmt.Sprintf("%s (%s)", prompt, strings.Join(choices, "/")), def))
		for _, c := range choices {
			if raw == c {
				return c
			}
		}
		fmt.Printf("  Please enter one of: %s\n", strings.Join(choices, ", "))
	}
}

func askYesNo(reader *bufio.Reader, prompt string, def bool) bool {
	defStr := "Y/n"
	if !def {
		defStr = "y/N"
	}
	raw := strings.ToLower(strings.TrimSpace(askString(reader, prompt, defStr)))
	switch raw {
	case strings.ToLower(defStr):
		// User accepted the shown default (e.g. pressed Enter).
		return def
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

func pause(reader *bufio.Reader) {
	fmt.Print("\nPress Enter to close this window...")
	_, _ = reader.ReadString('\n')
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("==================================================")
	fmt.Println(" Print-Ready Image Preparation (CatPrint settings)")
	fmt.Println("==================================================")
	fmt.Println()

	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = filepath.Abs(os.Args[0])
	}
	exeDir := filepath.Dir(exePath)

	// Gather candidate input paths: files/folders dropped onto the program
	// as arguments, or (if none given) an "input" folder next to it.
	args := os.Args[1:]
	var candidates []string
	if len(args) > 0 {
		candidates = args
	} else {
		inputDir := filepath.Join(exeDir, "input")
		for {
			if _, err := os.Stat(inputDir); os.IsNotExist(err) {
				_ = os.MkdirAll(inputDir, 0o755)
			}
			if pngs := gatherPNGs([]string{inputDir}); len(pngs) > 0 {
				candidates = []string{inputDir}
				break
			}
			fmt.Printf("No PNG files found yet.\n\nPut your PNG image(s) in this folder:\n  %s\n\n", inputDir)
			fmt.Print("Press Enter once they're there (or type Q then Enter to quit): ")
			line, _ := reader.ReadString('\n')
			if strings.EqualFold(strings.TrimSpace(line), "q") {
				fmt.Println("Cancelled.")
				pause(reader)
				return
			}
		}
	}

	pngPaths := gatherPNGs(candidates)
	if len(pngPaths) == 0 {
		fmt.Println("No PNG files found to process.")
		pause(reader)
		return
	}

	fmt.Printf("Found %d PNG file(s):\n", len(pngPaths))
	for _, p := range pngPaths {
		fmt.Printf("  - %s\n", filepath.Base(p))
	}
	fmt.Println()

	width := askFloat(reader, "Target width", 4)
	height := askFloat(reader, "Target height", 6)
	units := askChoice(reader, "Units", "in", []string{"in", "mm"})
	dpi := int(askFloat(reader, "Target DPI (CatPrint's minimum is 300)", DefaultDPI))
	fitMode := askChoice(reader, "Fit mode", "fit", fitModes)
	cmyk := askYesNo(reader, "Convert to CMYK for printing? (CatPrint requires CMYK)", true)

	outputDir := filepath.Join(exeDir, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Printf("ERROR: could not create output folder: %v\n", err)
		pause(reader)
		return
	}

	opt := Options{Width: width, Height: height, Units: units, DPI: dpi, FitMode: fitMode, CMYK: cmyk}

	fmt.Println()
	fmt.Println("Processing...")
	fmt.Println()

	processed := 0
	for _, p := range pngPaths {
		result, err := prepareForPrint(p, outputDir, opt)
		if err != nil {
			fmt.Printf("ERROR processing %s: %v\n\n", filepath.Base(p), err)
			continue
		}
		printSummary(result)
		fmt.Println()
		processed++
	}

	fmt.Printf("Done. Processed %d of %d file(s).\n", processed, len(pngPaths))
	fmt.Printf("Output folder:\n  %s\n", outputDir)

	pause(reader)
}
