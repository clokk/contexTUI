# Image Preview

**Category:** Feature
**Status:** Active

## Description

contexTUI supports previewing image files directly in the terminal. When you select an image file (PNG, JPG, GIF, WebP, or SVG), it renders in the preview pane using Unicode block characters with true color support.

## Key Files

- internal/app/image_preview.go - Image loading, scaling, and rendering
- internal/terminal/capabilities.go - Terminal graphics capability detection
- internal/filetype/detect.go - File type detection for images
- internal/app/preview.go - Preview branching logic for images vs text
- internal/app/types.go - ImageLoadedMsg and CachedImage types

---

## Supported Formats

| Format | Extension | Notes |
|--------|-----------|-------|
| PNG | `.png` | Full support |
| JPEG | `.jpg`, `.jpeg` | Full support |
| GIF | `.gif` | Static frame only |
| WebP | `.webp` | Full support |
| SVG | `.svg` | Rasterized at preview time |

## How It Works

### Terminal Detection

On startup, contexTUI detects your terminal's graphics capabilities:

| Terminal | Detection Method | Protocol |
|----------|------------------|----------|
| Kitty | `KITTY_WINDOW_ID` env var | Kitty Graphics |
| Ghostty | `TERM_PROGRAM=ghostty` | Kitty Graphics |
| WezTerm | `WEZTERM_EXECUTABLE` env var | Kitty Graphics |
| Konsole | `KONSOLE_VERSION` env var | Kitty Graphics |
| Others | Fallback | Unicode Blocks |

### Rendering

Images are rendered using **Unicode half-block characters** (`▀`) with ANSI true color:
- Each character represents 2 vertical pixels
- Foreground color = top pixel
- Background color = bottom pixel
- Results in a 2:1 aspect ratio correction

### SVG Handling

SVG files are rasterized before display:
1. Parsed using `tdewolff/canvas`
2. Scaled to fit the preview pane
3. Rendered at 2x resolution for quality
4. Converted to block characters

## Usage

1. Navigate to an image file in the tree pane
2. The image automatically renders in the preview pane
3. A header shows: filename, dimensions (width × height)

## Caching

Images are cached after first render:
- Cache key: file path
- Invalidation: file modification time changes
- Re-selecting a cached image is instant

## Limitations

- **GIF animations**: Only the first frame is displayed
- **Very large images**: Scaled down to fit the preview pane
- **Color accuracy**: Depends on terminal's true color support
- **Resolution**: Limited by terminal cell size (~2 pixels per character vertically)

## Drag and Drop Import

You can drag image files from your file manager into the terminal to import them:
1. Highlight the target folder in the tree
2. Drag an image file onto the terminal
3. An import overlay appears with the filename
4. Press `Enter` to confirm or edit the name first

See the File Management documentation for more details on file operations.
