# Image Preview

**Category:** Feature
**Status:** Active

## Description

contexTUI supports previewing image files directly in the terminal. When you select an image file (PNG, JPG, GIF, WebP, or SVG), it renders in the preview pane using Unicode block characters. For supported terminals, press `Enter` to open a full-screen overlay using the Kitty Graphics Protocol for pixel-perfect rendering.

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

### Rendering Modes

#### Preview Pane (Default)

Images in the preview pane are rendered using **Unicode half-block characters** (`▀`) with ANSI true color:
- Each character represents 2 vertical pixels
- Foreground color = top pixel, background color = bottom pixel
- Works in any terminal with true color support
- Automatically scales to fit the preview pane

#### Full-Screen Overlay (Kitty Protocol)

Press `Enter` while viewing an image to open the **full-screen overlay**:
- Uses the **Kitty Graphics Protocol** for pixel-perfect rendering
- Image is displayed at native resolution (scaled to fit screen)
- Bordered frame shows filename, dimensions, and format
- Press `Esc` or `q` to exit and return to normal view
- Only available in terminals that support Kitty Graphics (Ghostty, Kitty, WezTerm, Konsole)

The Kitty Graphics Protocol transmits the actual image data (PNG format) to the terminal, which renders it natively. This provides much higher quality than Unicode block approximation.

### SVG Handling

SVG files are rasterized before display:
1. Parsed using `tdewolff/canvas`
2. Scaled to fit the preview pane (or screen in overlay mode)
3. Rendered at high resolution for quality
4. Displayed via block characters or Kitty protocol

## Usage

1. Navigate to an image file in the tree pane
2. The image automatically renders in the preview pane (block characters)
3. Press `Enter` to open full-screen overlay (Kitty protocol, if supported)
4. Press `o` to open the image in your OS default application
5. Press `Esc` or `q` to exit overlay mode

## Caching

Images are cached after first render:
- Cache key: file path + viewport dimensions
- Invalidation: file modification time or significant viewport size change
- Re-selecting a cached image is instant

## Limitations

- **GIF animations**: Only the first frame is displayed
- **Very large images**: Scaled down to fit the preview pane/screen
- **Color accuracy**: Block rendering depends on terminal's true color support
- **Overlay mode**: Only available in Kitty-compatible terminals
- **Overlay bottom border**: May be partially covered by the Kitty image overlay

## Drag and Drop Import

You can drag image files from your file manager into the terminal to import them:
1. Highlight the target folder in the tree
2. Drag an image file onto the terminal
3. An import overlay appears with the filename
4. Press `Enter` to confirm or edit the name first

See the File Management documentation for more details on file operations.
