package ui

import (
	"image"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	xdraw "golang.org/x/image/draw"

	"github.com/kbinani/screenshot"
)

var (
	dimColor    = color.RGBA{R: 0, G: 0, B: 0, A: 110}
	borderColor = color.RGBA{R: 30, G: 160, B: 255, A: 255}
)

// overlayWidget is the fullscreen rubber-band selection overlay.
//
// Rendering strategy (zero per-pixel CPU work per frame):
//   - bgRaster : canvas.NewRaster returns the pre-captured screenshot at
//                physical-pixel resolution (GPU texture is cached by Fyne;
//                we never call Refresh on it so it's uploaded exactly once).
//   - 4 dim rectangles : cover the area *outside* the selection → GPU quads.
//   - 4 border rectangles : highlight the selection edge → GPU quads.
//
// On each Dragged event we only update 8 float32 values and let the GPU
// composite everything; no image copies, no per-pixel loops.
type overlayWidget struct {
	widget.BaseWidget

	// Background — static throughout the overlay session.
	bgRaster *canvas.Raster
	bgCache  *image.RGBA // computed once (on first raster call), then reused
	bgW, bgH int

	// Dim / border overlays — updated cheaply on each drag event.
	dimTop, dimBottom, dimLeft, dimRight           *canvas.Rectangle
	borderTop, borderBottom, borderLeft, borderRight *canvas.Rectangle

	startPos fyne.Position
	endPos   fyne.Position
	dragging bool

	// onSelect receives the full pre-captured image and the selection in
	// image-relative coordinates (not screen coordinates).
	onSelect func(fullImg *image.RGBA, selRect image.Rectangle)
	window   fyne.Window
	fullImg  *image.RGBA
	display  image.Rectangle // physical-pixel bounds of the captured area
}

func newRect(c color.Color) *canvas.Rectangle { return canvas.NewRectangle(c) }

func newOverlayWidget(
	fullImg *image.RGBA,
	display image.Rectangle,
	onSelect func(*image.RGBA, image.Rectangle),
	win fyne.Window,
) *overlayWidget {
	o := &overlayWidget{
		dimTop:       newRect(dimColor),
		dimBottom:    newRect(dimColor),
		dimLeft:      newRect(dimColor),
		dimRight:     newRect(dimColor),
		borderTop:    newRect(borderColor),
		borderBottom: newRect(borderColor),
		borderLeft:   newRect(borderColor),
		borderRight:  newRect(borderColor),
		onSelect:     onSelect,
		window:       win,
		fullImg:      fullImg,
		display:      display,
	}
	// The raster callback is invoked by Fyne with the physical pixel
	// dimensions of the widget. We compute bgCache once and return the
	// same pointer on subsequent calls so Fyne can reuse the GPU texture.
	o.bgRaster = canvas.NewRaster(o.generateBackground)
	o.ExtendBaseWidget(o)
	return o
}

// generateBackground is the canvas.NewRaster callback.
// It is called with physical pixel dimensions (w, h).
// On the first call it scales fullImg to (w, h); thereafter it returns the
// cached result so Fyne does not re-upload the texture every frame.
func (o *overlayWidget) generateBackground(w, h int) image.Image {
	if o.bgCache != nil && o.bgW == w && o.bgH == h {
		return o.bgCache // cache hit — Fyne reuses the GPU texture
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if o.fullImg != nil {
		src := o.fullImg
		if src.Bounds().Dx() == w && src.Bounds().Dy() == h {
			copy(dst.Pix, src.Pix) // same size — plain memcopy, no scaling
		} else {
			// High-quality scaling (Catmull-Rom) used only when dimensions
			// differ (e.g. on a fractional-DPI display).
			xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
		}
	}
	o.bgCache, o.bgW, o.bgH = dst, w, h
	return dst
}

// ---------------------------------------------------------------------------
// Widget interface
// ---------------------------------------------------------------------------

func (o *overlayWidget) CreateRenderer() fyne.WidgetRenderer {
	return &overlayRenderer{w: o}
}

func (o *overlayWidget) MinSize() fyne.Size { return fyne.NewSize(10000, 10000) }

// Tapped cancels without selection (single click, no drag).
func (o *overlayWidget) Tapped(_ *fyne.PointEvent) { o.window.Close() }

func (o *overlayWidget) Dragged(e *fyne.DragEvent) {
	if !o.dragging {
		o.startPos = e.AbsolutePosition
		o.dragging = true
	}
	o.endPos = e.AbsolutePosition
	o.Refresh() // updates rectangle layout; does NOT refresh the raster
}

func (o *overlayWidget) DragEnd() {
	o.dragging = false

	scale := o.window.Canvas().Scale()
	if scale == 0 {
		scale = 1
	}

	// Convert Fyne logical coords → physical screen pixels.
	x0 := int(math.Min(float64(o.startPos.X), float64(o.endPos.X))*float64(scale)) + o.display.Min.X
	y0 := int(math.Min(float64(o.startPos.Y), float64(o.endPos.Y))*float64(scale)) + o.display.Min.Y
	x1 := int(math.Max(float64(o.startPos.X), float64(o.endPos.X))*float64(scale)) + o.display.Min.X
	y1 := int(math.Max(float64(o.startPos.Y), float64(o.endPos.Y))*float64(scale)) + o.display.Min.Y

	screenRect := image.Rect(x0, y0, x1, y1)
	// Translate to coordinates relative to the captured image's top-left.
	relRect := screenRect.Sub(o.display.Min)
	if o.fullImg != nil {
		relRect = relRect.Intersect(o.fullImg.Bounds())
	}

	// Ignore selections that are too small to be intentional.
	if relRect.Dx() < 5 || relRect.Dy() < 5 {
		o.window.Close()
		return
	}

	fullImg := o.fullImg
	cb := o.onSelect
	win := o.window
	go func() {
		win.Close()
		// Small delay so the overlay is gone before any follow-up UI appears.
		time.Sleep(80 * time.Millisecond)
		if cb != nil {
			cb(fullImg, relRect)
		}
	}()
}

func (o *overlayWidget) Cursor() desktop.Cursor { return desktop.CrosshairCursor }

// ---------------------------------------------------------------------------
// Custom renderer
// ---------------------------------------------------------------------------

type overlayRenderer struct{ w *overlayWidget }

// Objects lists all canvas objects Fyne should composite for this widget.
// Order matters: earlier = drawn first (behind later objects).
func (r *overlayRenderer) Objects() []fyne.CanvasObject {
	w := r.w
	return []fyne.CanvasObject{
		w.bgRaster,
		w.dimTop, w.dimBottom, w.dimLeft, w.dimRight,
		w.borderTop, w.borderBottom, w.borderLeft, w.borderRight,
	}
}

func (r *overlayRenderer) Layout(size fyne.Size) { r.doLayout(size) }

// Refresh is triggered by o.Refresh() (called from Dragged).
// We intentionally do NOT refresh bgRaster here — that keeps the GPU
// texture cached and avoids any re-upload.
func (r *overlayRenderer) Refresh() {
	r.doLayout(r.w.Size())
	// Refresh only the shape overlays; bgRaster GPU texture is reused.
	for _, obj := range []fyne.CanvasObject{
		r.w.dimTop, r.w.dimBottom, r.w.dimLeft, r.w.dimRight,
		r.w.borderTop, r.w.borderBottom, r.w.borderLeft, r.w.borderRight,
	} {
		canvas.Refresh(obj)
	}
}

func (r *overlayRenderer) MinSize() fyne.Size { return r.w.MinSize() }
func (r *overlayRenderer) Destroy()           {}

// doLayout positions all 9 objects for the current drag state.
func (r *overlayRenderer) doLayout(sz fyne.Size) {
	w := r.w
	sw, sh := sz.Width, sz.Height

	// Background always fills the entire window at physical-pixel resolution.
	w.bgRaster.Move(fyne.NewPos(0, 0))
	w.bgRaster.Resize(sz)

	if !w.dragging {
		// Before first drag: dim the whole screen.
		w.dimTop.Move(fyne.NewPos(0, 0))
		w.dimTop.Resize(sz)
		collapseAll(w.dimBottom, w.dimLeft, w.dimRight)
		collapseAll(w.borderTop, w.borderBottom, w.borderLeft, w.borderRight)
		return
	}

	x0 := float32(math.Min(float64(w.startPos.X), float64(w.endPos.X)))
	y0 := float32(math.Min(float64(w.startPos.Y), float64(w.endPos.Y)))
	x1 := float32(math.Max(float64(w.startPos.X), float64(w.endPos.X)))
	y1 := float32(math.Max(float64(w.startPos.Y), float64(w.endPos.Y)))

	// 4 dim bars that leave the selection area un-covered.
	w.dimTop.Move(fyne.NewPos(0, 0))
	w.dimTop.Resize(fyne.NewSize(sw, y0))

	w.dimBottom.Move(fyne.NewPos(0, y1))
	w.dimBottom.Resize(fyne.NewSize(sw, sh-y1))

	w.dimLeft.Move(fyne.NewPos(0, y0))
	w.dimLeft.Resize(fyne.NewSize(x0, y1-y0))

	w.dimRight.Move(fyne.NewPos(x1, y0))
	w.dimRight.Resize(fyne.NewSize(sw-x1, y1-y0))

	// 4 border lines (2 logical-unit thick).
	const bw float32 = 2
	w.borderTop.Move(fyne.NewPos(x0, y0))
	w.borderTop.Resize(fyne.NewSize(x1-x0, bw))

	w.borderBottom.Move(fyne.NewPos(x0, y1-bw))
	w.borderBottom.Resize(fyne.NewSize(x1-x0, bw))

	w.borderLeft.Move(fyne.NewPos(x0, y0))
	w.borderLeft.Resize(fyne.NewSize(bw, y1-y0))

	w.borderRight.Move(fyne.NewPos(x1-bw, y0))
	w.borderRight.Resize(fyne.NewSize(bw, y1-y0))
}

func collapseAll(rects ...*canvas.Rectangle) {
	z := fyne.NewSize(0, 0)
	for _, r := range rects {
		r.Resize(z)
	}
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

// ShowOverlay captures the full virtual screen (all monitors), then displays
// a fullscreen selection overlay.  onSelect receives the pre-captured image
// and the selection in image-relative coordinates.
func ShowOverlay(a fyne.App, onSelect func(fullImg *image.RGBA, selRect image.Rectangle)) {
	// Build the virtual screen bounds (union of all connected monitors).
	n := screenshot.NumActiveDisplays()
	virtualBounds := image.Rectangle{}
	for i := 0; i < n; i++ {
		virtualBounds = virtualBounds.Union(screenshot.GetDisplayBounds(i))
	}
	if virtualBounds.Empty() {
		virtualBounds = screenshot.GetDisplayBounds(0)
	}

	// Capture desktop BEFORE creating the overlay window so the screenshot
	// is clean (the overlay itself is not captured).
	fullImg, _ := screenshot.CaptureRect(virtualBounds)

	win := a.NewWindow("")
	win.SetPadded(false)
	win.SetFixedSize(false)

	ow := newOverlayWidget(fullImg, virtualBounds, onSelect, win)
	win.SetContent(ow)
	win.SetFullScreen(true)
	win.Show()
}
