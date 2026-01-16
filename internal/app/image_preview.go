package app

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/filetype"
	"github.com/connorleisz/contexTUI/internal/terminal"
	"github.com/nfnt/resize"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
	_ "golang.org/x/image/webp"
)

// loadImageAsync returns a command that loads and renders an image
func loadImageAsync(path string, caps terminal.Capabilities, maxW, maxH int) tea.Cmd {
	return func() tea.Msg {
		return loadImage(path, caps, maxW, maxH)
	}
}

// ANSI color constants for overlay
const (
	overlayBorderColor = "\x1b[38;5;205m" // Pink accent (matches app style)
	overlayDimColor    = "\x1b[2m"         // Dim for metadata
	overlayReset       = "\x1b[0m"
)

// LoadImageForOverlay loads an image and returns Kitty overlay escape sequences
// maxW and maxH are the full screen dimensions in terminal cells
func LoadImageForOverlay(path string, maxW, maxH int) (string, error) {
	var img image.Image
	var err error

	if filetype.IsSVG(path) {
		// For SVG, rasterize at high resolution
		img, err = rasterizeSVG(path, maxW*4, maxH*4)
	} else {
		img, err = loadRasterImage(path)
	}

	if err != nil {
		return "", err
	}

	// Get image metadata
	bounds := img.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()
	filename := filepath.Base(path)
	format := filetype.DetectImageFormat(path).String()
	dims := fmt.Sprintf("%d×%d", imgW, imgH)

	// Calculate frame dimensions (need room for border + padding + header + footer)
	// Leave margin from screen edges
	maxFrameW := maxW - 4
	maxFrameH := maxH - 4

	// Calculate image display size (inside frame: -2 for borders, -2 for padding)
	availW := maxFrameW - 4
	availH := maxFrameH - 4

	// Scale to fit while maintaining aspect ratio
	scaleX := float64(availW) / float64(imgW)
	scaleY := float64(availH*2) / float64(imgH) // *2 because each row = 2 pixels

	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Calculate display dimensions in terminal cells
	displayCols := int(float64(imgW) * scale)
	displayRows := int(float64(imgH) * scale / 2)
	if displayRows < 1 {
		displayRows = 1
	}
	if displayCols < 1 {
		displayCols = 1
	}

	// Frame dimensions (image + borders)
	frameW := displayCols + 2
	frameH := displayRows + 2

	// Center the frame on screen
	frameX := (maxW - frameW) / 2
	frameY := (maxH - frameH) / 2
	if frameX < 1 {
		frameX = 1
	}
	if frameY < 1 {
		frameY = 1
	}

	var result strings.Builder

	// Clear screen
	result.WriteString("\x1b[2J")

	// Draw top border with header: ╭─ filename │ dims │ format ─────╮
	result.WriteString(positionCursor(frameY, frameX))
	result.WriteString(renderOverlayTopBorder(frameW, filename, dims, format))

	// Draw left and right borders for each row
	for row := 1; row <= displayRows; row++ {
		// Left border
		result.WriteString(positionCursor(frameY+row, frameX))
		result.WriteString(overlayBorderColor + "│" + overlayReset)
		// Right border
		result.WriteString(positionCursor(frameY+row, frameX+frameW-1))
		result.WriteString(overlayBorderColor + "│" + overlayReset)
	}

	// Draw bottom border with hint: ╰──────── Esc to exit ────────╯
	result.WriteString(positionCursor(frameY+displayRows+1, frameX))
	result.WriteString(renderOverlayBottomBorder(frameW, "Esc to exit"))

	// Position and render Kitty image inside frame
	result.WriteString(positionCursor(frameY+1, frameX+1))
	result.WriteString(RenderKittyOverlay(img, displayCols, displayRows))

	return result.String(), nil
}

// positionCursor returns ANSI escape sequence to move cursor to row, col (1-indexed)
func positionCursor(row, col int) string {
	return fmt.Sprintf("\x1b[%d;%dH", row, col)
}

// renderOverlayTopBorder creates: ╭─ filename │ dims │ format ─────╮
func renderOverlayTopBorder(width int, filename, dims, format string) string {
	// Build the metadata string
	meta := fmt.Sprintf(" %s │ %s │ %s ", filename, dims, format)

	// Calculate padding
	// Corner + dash + meta + dashes + corner = width
	// ╭─ meta ────────╮
	contentWidth := width - 2 // minus corners
	metaLen := len(meta)

	var b strings.Builder
	b.WriteString(overlayBorderColor)
	b.WriteString("╭")

	if metaLen >= contentWidth {
		// Truncate metadata if too long
		b.WriteString("─")
		truncated := meta
		if len(truncated) > contentWidth-2 {
			truncated = truncated[:contentWidth-5] + "... "
		}
		b.WriteString(overlayDimColor)
		b.WriteString(truncated)
		b.WriteString(overlayBorderColor)
	} else {
		b.WriteString("─")
		b.WriteString(overlayDimColor)
		b.WriteString(meta)
		b.WriteString(overlayBorderColor)
		// Fill remaining with dashes
		remaining := contentWidth - 1 - metaLen
		for i := 0; i < remaining; i++ {
			b.WriteString("─")
		}
	}

	b.WriteString("╮")
	b.WriteString(overlayReset)
	return b.String()
}

// renderOverlayBottomBorder creates: ╰──────── hint ────────╯
func renderOverlayBottomBorder(width int, hint string) string {
	// Calculate centering for hint
	contentWidth := width - 2 // minus corners
	hintWithSpaces := " " + hint + " "
	hintLen := len(hintWithSpaces)

	var b strings.Builder
	b.WriteString(overlayBorderColor)
	b.WriteString("╰")

	if hintLen >= contentWidth {
		// Just dashes if hint too long
		for i := 0; i < contentWidth; i++ {
			b.WriteString("─")
		}
	} else {
		// Center the hint
		leftPad := (contentWidth - hintLen) / 2
		rightPad := contentWidth - hintLen - leftPad

		for i := 0; i < leftPad; i++ {
			b.WriteString("─")
		}
		b.WriteString(overlayDimColor)
		b.WriteString(hintWithSpaces)
		b.WriteString(overlayBorderColor)
		for i := 0; i < rightPad; i++ {
			b.WriteString("─")
		}
	}

	b.WriteString("╯")
	b.WriteString(overlayReset)
	return b.String()
}

// loadImage loads, scales, and renders an image file
func loadImage(path string, caps terminal.Capabilities, maxW, maxH int) ImageLoadedMsg {
	info, err := os.Stat(path)
	if err != nil {
		return ImageLoadedMsg{Path: path, Error: err}
	}

	var img image.Image

	if filetype.IsSVG(path) {
		img, err = rasterizeSVG(path, maxW, maxH)
	} else {
		img, err = loadRasterImage(path)
	}

	if err != nil {
		return ImageLoadedMsg{Path: path, Error: err}
	}

	// Get original dimensions for display
	origBounds := img.Bounds()
	origW := origBounds.Dx()
	origH := origBounds.Dy()

	// Scale to fit preview pane
	scaledImg := scaleToFit(img, maxW, maxH, caps.Graphics)

	// Calculate rendered dimensions in terminal cells
	scaledBounds := scaledImg.Bounds()
	renderedW := scaledBounds.Dx()
	renderedH := scaledBounds.Dy() / 2 // Each block char shows 2 pixels
	if scaledBounds.Dy()%2 != 0 {
		renderedH++ // Round up for odd heights
	}

	// Render based on protocol (pass cell dimensions for Kitty)
	renderData := renderForProtocol(scaledImg, caps, renderedW, renderedH)

	return ImageLoadedMsg{
		Path:       path,
		Width:      origW,            // Original image width
		Height:     origH,            // Original image height
		RenderW:    renderedW,        // Rendered width in terminal cells
		RenderH:    renderedH,        // Rendered height in terminal cells
		RenderData: renderData,
		ModTime:    info.ModTime(),
	}
}

// loadRasterImage loads a PNG, JPG, GIF, or WebP image
func loadRasterImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return img, nil
}

// rasterizeSVG converts an SVG file to a raster image
func rasterizeSVG(path string, maxW, maxH int) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open SVG: %w", err)
	}
	defer f.Close()

	c, err := canvas.ParseSVG(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SVG: %w", err)
	}

	// Get SVG dimensions using Size() method
	svgW, svgH := c.Size()
	if svgW <= 0 || svgH <= 0 {
		// Default size if SVG doesn't specify
		svgW, svgH = 100, 100
	}

	// Calculate scale to fit within max dimensions
	// Target higher resolution for quality (2x)
	targetW := float64(maxW * 2)
	targetH := float64(maxH * 4) // Account for block char aspect ratio

	scaleX := targetW / svgW
	scaleY := targetH / svgH
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Render at calculated DPI
	dpi := scale * 72 // Base DPI is 72

	img := rasterizer.Draw(c, canvas.DPI(dpi), canvas.DefaultColorSpace)
	return img, nil
}

// scaleToFit scales an image to fit within the given dimensions
// maxW and maxH are in terminal cells (characters)
func scaleToFit(img image.Image, maxW, maxH int, protocol terminal.GraphicsProtocol) image.Image {
	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	// Leave room for header (2 lines: info + blank line)
	maxH = maxH - 2
	if maxH < 4 {
		maxH = 4
	}

	// For block rendering: each character represents 2 vertical pixels
	// Terminal cells are roughly 1:2 aspect ratio (width:height in pixels)
	// So 1 cell width = ~1 pixel, 1 cell height = ~2 pixels

	// Target pixel dimensions based on terminal cells
	// Width: 1 terminal cell = 1 character width in pixels for our block chars
	// Height: 1 terminal cell can show 2 vertical pixels (using half-blocks)
	targetW := maxW
	targetH := maxH * 2 // Each row of chars shows 2 pixels

	// Calculate scale maintaining aspect ratio
	scaleX := float64(targetW) / float64(origW)
	scaleY := float64(targetH) / float64(origH)

	// Use the smaller scale to fit within bounds
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Allow upscaling up to 2x to fill preview pane for small images
	// Beyond 2x tends to look too pixelated
	if scale > 2.0 {
		scale = 2.0
	}

	newW := int(float64(origW) * scale)
	newH := int(float64(origH) * scale)

	// Ensure minimum size
	if newW < 1 {
		newW = 1
	}
	if newH < 2 {
		newH = 2 // Need at least 2 pixels for one row of blocks
	}

	// Ensure height is even for clean block rendering
	if newH%2 != 0 {
		newH++
	}

	// Use Lanczos3 for quality scaling
	return resize.Resize(uint(newW), uint(newH), img, resize.Lanczos3)
}

// renderForProtocol renders an image using the appropriate protocol
// Always uses block characters for normal preview (viewport compatible)
func renderForProtocol(img image.Image, caps terminal.Capabilities, cols, rows int) string {
	// Always use blocks for in-pane preview - Kitty is incompatible with viewport
	return renderBlocks(img, caps.TrueColor)
}

// RenderKittyOverlay renders an image using the Kitty graphics protocol
// for full-screen overlay mode. cols and rows specify the display size in terminal cells.
// This is exported for use in update.go when entering overlay mode.
func RenderKittyOverlay(img image.Image, cols, rows int) string {
	// Encode image to PNG in memory
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// Return empty on error - caller should handle
		return ""
	}

	// Base64 encode the PNG data
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Build escape sequence with chunking
	var result strings.Builder

	// Kitty protocol: f=100 (PNG), a=T (transmit+display), c/r for cell size
	header := fmt.Sprintf("\x1b_Gf=100,a=T,c=%d,r=%d", cols, rows)

	// Chunk into 4096-byte pieces (Kitty protocol requirement)
	const chunkSize = 4096
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]

		if i == 0 {
			// First chunk includes header
			if end < len(encoded) {
				result.WriteString(header + ",m=1;" + chunk + "\x1b\\")
			} else {
				result.WriteString(header + ";" + chunk + "\x1b\\")
			}
		} else if end < len(encoded) {
			// Middle chunk
			result.WriteString("\x1b_Gm=1;" + chunk + "\x1b\\")
		} else {
			// Final chunk
			result.WriteString("\x1b_Gm=0;" + chunk + "\x1b\\")
		}
	}

	return result.String()
}

// ClearKittyImages returns the escape sequence to clear all Kitty images
func ClearKittyImages() string {
	return "\x1b_Ga=d\x1b\\"
}

// renderBlocks renders an image using Unicode half-block characters
// Each character represents 2 vertical pixels using ▀ (upper half block)
// The foreground color is the top pixel, background is the bottom pixel
func renderBlocks(img image.Image, trueColor bool) string {
	bounds := img.Bounds()
	var result strings.Builder

	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			topColor := img.At(x, y)
			var bottomColor color.Color
			if y+1 < bounds.Max.Y {
				bottomColor = img.At(x, y+1)
			} else {
				bottomColor = topColor
			}

			result.WriteString(colorBlock(topColor, bottomColor, trueColor))
		}
		result.WriteString("\x1b[0m\n") // Reset and newline
	}

	return result.String()
}

// colorBlock returns an ANSI escape sequence for a half-block character
// with the given top (foreground) and bottom (background) colors
func colorBlock(top, bottom color.Color, trueColor bool) string {
	tr, tg, tb, _ := top.RGBA()
	br, bg, bb, _ := bottom.RGBA()

	// Convert from 16-bit to 8-bit color
	tr, tg, tb = tr>>8, tg>>8, tb>>8
	br, bg, bb = br>>8, bg>>8, bb>>8

	if trueColor {
		// 24-bit true color
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
			tr, tg, tb, br, bg, bb)
	}

	// 256 color fallback
	topIdx := rgbTo256(uint8(tr), uint8(tg), uint8(tb))
	bottomIdx := rgbTo256(uint8(br), uint8(bg), uint8(bb))
	return fmt.Sprintf("\x1b[38;5;%dm\x1b[48;5;%dm▀", topIdx, bottomIdx)
}

// rgbTo256 converts RGB values to the closest 256-color palette index
func rgbTo256(r, g, b uint8) int {
	// Check if it's a grayscale color
	if r == g && g == b {
		if r < 8 {
			return 16 // Black
		}
		if r > 248 {
			return 231 // White
		}
		return int((r-8)/10) + 232 // Grayscale ramp
	}

	// Convert to 6x6x6 color cube
	rIdx := int(float64(r) / 255 * 5)
	gIdx := int(float64(g) / 255 * 5)
	bIdx := int(float64(b) / 255 * 5)

	return 16 + 36*rIdx + 6*gIdx + bIdx
}

// validateImageCache checks if a cached image is still valid
func validateImageCache(cached CachedImage, path string, viewportW, viewportH int) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Cache is invalid if file changed or viewport size changed significantly
	if !info.ModTime().Equal(cached.ModTime) {
		return false
	}
	// Allow small viewport changes (within 5 cells) to avoid constant re-renders
	wDiff := abs(cached.ViewportW - viewportW)
	hDiff := abs(cached.ViewportH - viewportH)
	return wDiff <= 5 && hDiff <= 5
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// imageFromCache creates an ImageLoadedMsg from cached data
func imageFromCache(cached CachedImage, path string) ImageLoadedMsg {
	return ImageLoadedMsg{
		Path:       path,
		Width:      cached.Width,
		Height:     cached.Height,
		RenderData: cached.RenderData,
		ModTime:    cached.ModTime,
	}
}

// clearImagePreview resets image preview state
func (m *Model) clearImagePreview() {
	m.previewIsImage = false
	m.currentImage = nil
}

// formatImageInfo returns a formatted string with image metadata
func formatImageInfo(msg *ImageLoadedMsg) string {
	if msg == nil {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%dx%d", msg.Width, msg.Height))

	// Add format info based on path
	format := filetype.DetectImageFormat(msg.Path)
	if format != filetype.FormatUnknown {
		buf.WriteString(" ")
		buf.WriteString(format.String())
	}

	// Add modification time
	buf.WriteString(" ")
	buf.WriteString(msg.ModTime.Format(time.RFC822))

	return buf.String()
}
