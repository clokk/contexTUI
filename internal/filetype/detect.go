package filetype

import (
	"os"
	"path/filepath"
	"strings"
)

// FileKind represents the general type of a file
type FileKind int

const (
	KindText FileKind = iota
	KindImage
	KindBinary
)

// ImageFormat represents specific image formats
type ImageFormat int

const (
	FormatUnknown ImageFormat = iota
	FormatPNG
	FormatJPG
	FormatGIF
	FormatWebP
	FormatSVG
)

// imageExtensions maps file extensions to image formats
var imageExtensions = map[string]ImageFormat{
	".png":  FormatPNG,
	".jpg":  FormatJPG,
	".jpeg": FormatJPG,
	".gif":  FormatGIF,
	".webp": FormatWebP,
	".svg":  FormatSVG,
}

// DetectKind determines the general file type from path
func DetectKind(path string) FileKind {
	ext := strings.ToLower(filepath.Ext(path))

	// Check if it's a known image format
	if _, ok := imageExtensions[ext]; ok {
		return KindImage
	}

	// Check for binary by reading first bytes
	if isBinaryFile(path) {
		return KindBinary
	}

	return KindText
}

// DetectImageFormat returns the specific image format
func DetectImageFormat(path string) ImageFormat {
	ext := strings.ToLower(filepath.Ext(path))
	if format, ok := imageExtensions[ext]; ok {
		return format
	}
	return FormatUnknown
}

// IsSVG returns true if the file is an SVG
func IsSVG(path string) bool {
	return DetectImageFormat(path) == FormatSVG
}

// IsImage returns true if the file is a supported image format
func IsImage(path string) bool {
	return DetectKind(path) == KindImage
}

// isBinaryFile checks if a file appears to be binary by looking for null bytes
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Read first 512 bytes
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	// Check for null bytes (common in binary files)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}

	return false
}

// String returns a human-readable name for the file kind
func (k FileKind) String() string {
	switch k {
	case KindImage:
		return "Image"
	case KindBinary:
		return "Binary"
	default:
		return "Text"
	}
}

// String returns a human-readable name for the image format
func (f ImageFormat) String() string {
	switch f {
	case FormatPNG:
		return "PNG"
	case FormatJPG:
		return "JPEG"
	case FormatGIF:
		return "GIF"
	case FormatWebP:
		return "WebP"
	case FormatSVG:
		return "SVG"
	default:
		return "Unknown"
	}
}
