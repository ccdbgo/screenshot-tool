//go:build windows

package capture

// Win32 transparent fullscreen selection overlay — WeChat-style resizable.
//
// States:
//   ovPhaseIdle   → dim screen, show hint, waiting for first drag
//   ovPhaseDrag   → rubber-band drag to create initial selection
//   ovPhaseReady  → selection shown with 8 handles + confirm/cancel toolbar
//   ovPhaseMove   → dragging interior to move the entire selection
//   ovPhaseResize → dragging a handle to resize

import (
	"image"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

// ─── lazy-loaded Win32 APIs ──────────────────────────────────────────────────

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modGDI32    = syscall.NewLazyDLL("gdi32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetLayeredWindowAttributes = modUser32.NewProc("SetLayeredWindowAttributes")
	procFillRect                   = modUser32.NewProc("FillRect")
	procFrameRect                  = modUser32.NewProc("FrameRect")
	procDrawTextW                  = modUser32.NewProc("DrawTextW")
	procSetTextAlign               = modGDI32.NewProc("SetTextAlign")
	procCreateSolidBrush           = modGDI32.NewProc("CreateSolidBrush")
	procCreatePen                  = modGDI32.NewProc("CreatePen")
	procRoundRect                  = modGDI32.NewProc("RoundRect")
	procSetCursor                  = modUser32.NewProc("SetCursor")
	procAttachThreadInput          = modUser32.NewProc("AttachThreadInput")
	procGetWindowThreadProcessId   = modUser32.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadId         = modKernel32.NewProc("GetCurrentThreadId")
	procBringWindowToTop           = modUser32.NewProc("BringWindowToTop")
	procSetActiveWindow            = modUser32.NewProc("SetActiveWindow")
)

// ─── wrapper helpers ─────────────────────────────────────────────────────────

func setLayeredWindowAttributes(hwnd win.HWND, crKey win.COLORREF, bAlpha byte, dwFlags uint32) {
	procSetLayeredWindowAttributes.Call(uintptr(hwnd), uintptr(crKey), uintptr(bAlpha), uintptr(dwFlags)) //nolint:errcheck
}
func fillRect(hdc win.HDC, r *win.RECT, hbr win.HBRUSH) {
	procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(r)), uintptr(hbr)) //nolint:errcheck
}
func frameRect(hdc win.HDC, r *win.RECT, hbr win.HBRUSH) {
	procFrameRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(r)), uintptr(hbr)) //nolint:errcheck
}
func drawTextW(hdc win.HDC, text *uint16, n int32, r *win.RECT, flags uint32) {
	procDrawTextW.Call(uintptr(hdc), uintptr(unsafe.Pointer(text)), uintptr(n),
		uintptr(unsafe.Pointer(r)), uintptr(flags)) //nolint:errcheck
}
func createSolidBrush(c win.COLORREF) win.HBRUSH {
	r, _, _ := procCreateSolidBrush.Call(uintptr(c))
	return win.HBRUSH(r)
}
func createPen(style, width int32, c win.COLORREF) win.HPEN {
	r, _, _ := procCreatePen.Call(uintptr(style), uintptr(width), uintptr(c))
	return win.HPEN(r)
}
func roundRect(hdc win.HDC, x0, y0, x1, y1, rx, ry int32) {
	procRoundRect.Call(uintptr(hdc), uintptr(x0), uintptr(y0), uintptr(x1), uintptr(y1),
		uintptr(rx), uintptr(ry)) //nolint:errcheck
}
func setCursor(hcur win.HCURSOR) { procSetCursor.Call(uintptr(hcur)) } //nolint:errcheck
func getCurrentThreadId() uint32 {
	r, _, _ := procGetCurrentThreadId.Call()
	return uint32(r)
}
func getWindowThreadId(hwnd win.HWND) uint32 {
	r, _, _ := procGetWindowThreadProcessId.Call(uintptr(hwnd), 0)
	return uint32(r)
}

// selectFont selects a LOGFONT into hdc, returns the old font (call SelectObject to restore).
func selectUIFont(hdc win.HDC, height int32, bold bool) win.HGDIOBJ {
	weight := int32(400)
	if bold {
		weight = 700
	}
	face, _ := syscall.UTF16FromString("Microsoft YaHei UI")
	lf := win.LOGFONT{
		LfHeight:         height,
		LfWeight:         weight,
		LfCharSet:        win.DEFAULT_CHARSET,
		LfOutPrecision:   win.OUT_DEFAULT_PRECIS,
		LfClipPrecision:  win.CLIP_DEFAULT_PRECIS,
		LfQuality:        win.CLEARTYPE_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	copy(lf.LfFaceName[:], face)
	hf := win.CreateFontIndirect(&lf)
	return win.SelectObject(hdc, win.HGDIOBJ(hf))
}

func forceToForeground(hwnd win.HWND) {
	fg := win.GetForegroundWindow()
	fgTid := getWindowThreadId(fg)
	myTid := getCurrentThreadId()
	if fg != 0 && fgTid != 0 && fgTid != myTid {
		procAttachThreadInput.Call(uintptr(myTid), uintptr(fgTid), 1)
		defer procAttachThreadInput.Call(uintptr(myTid), uintptr(fgTid), 0)
	}
	procBringWindowToTop.Call(uintptr(hwnd))
	procSetActiveWindow.Call(uintptr(hwnd))
	win.SetForegroundWindow(hwnd)
}

// ─── color palette (COLORREF = 0x00BBGGRR) ──────────────────────────────────

const (
	// Selection border: bright cyan-blue #00C8FF
	clrBorder = win.COLORREF(0x00FFC800)
	// Outer glow line: darker blue #006080
	clrBorderOuter = win.COLORREF(0x00806000)
	// Handle fill: white
	clrHandleFill = win.COLORREF(0x00FFFFFF)
	// Handle border: same as selection border
	clrHandleBdr = clrBorder
	// Dim overlay
	clrDim = win.COLORREF(0x00181818)
	// Size-label background: near-black #1A1A2A
	clrLabelBg = win.COLORREF(0x002A1A1A)
	// Toolbar background
	clrTbBg = win.COLORREF(0x00252525)
	// Toolbar border
	clrTbBdr = win.COLORREF(0x00454545)
	// Confirm button: deep indigo-blue #1A56DB → BGR = 0x00DB561A
	clrConfirm = win.COLORREF(0x00DB561A)
	// Confirm lighter: #4B7BF5 → BGR = 0x00F57B4B
	clrConfirmLight = win.COLORREF(0x00F57B4B)
	// Cancel button: slate #64748B → BGR = 0x008B7464
	clrCancel = win.COLORREF(0x008B7464)
	// Cancel lighter: #94A3B8 → BGR = 0x00B8A394
	clrCancelLight = win.COLORREF(0x00B8A394)
	// White text
	clrTextWhite = win.COLORREF(0x00FFFFFF)
	// Light gray text (hints)
	clrTextHint = win.COLORREF(0x00CCCCCC)

	// Transparent key (magenta)
	keyColorRef    = win.COLORREF(0x00FF00FF)
	overlayOpacity = byte(165)
)

// ─── layered window constants ────────────────────────────────────────────────

const (
	lwaColorKey = uint32(0x00000001)
	lwaAlpha    = uint32(0x00000002)

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	dtCenter     = uint32(0x00000001)
	dtVCenter    = uint32(0x00000004)
	dtSingleLine = uint32(0x00000020)
	dtNoClip     = uint32(0x00000100)
	dtEndEllipsis = uint32(0x00008000)

	// Handle half-size (drawn square) and hit-test radius
	handleHS = int32(5)
	handleHR = int32(9)

	// Toolbar layout — generous sizing for easy clicking
	tbPad  = int32(10)
	tbBtnW = int32(116)
	tbBtnH = int32(44)
	tbBtnG = int32(10)
	tbGap  = int32(12)
)

var ovClassName = syscall.StringToUTF16Ptr("_CCDBPOverlayV3_")

// ─── overlay state ────────────────────────────────────────────────────────────

type ovPhaseT int8

const (
	ovPhaseIdle   ovPhaseT = iota
	ovPhaseDrag
	ovPhaseReady
	ovPhaseMove
	ovPhaseResize
)

type ovHitT int8

const (
	ovHitNone    ovHitT = iota
	ovHitTL; ovHitTC; ovHitTR
	ovHitML;           ovHitMR
	ovHitBL; ovHitBC; ovHitBR
	ovHitInside
	ovHitConfirm
	ovHitCancel
)

var (
	ovPhase                     ovPhaseT
	ovX0, ovY0, ovX1, ovY1      int32
	ovDragHit                   ovHitT
	ovDragMX, ovDragMY          int32
	ovDragX0, ovDragY0          int32
	ovDragX1, ovDragY1          int32
	overlayResultCh             chan image.Rectangle
)

// ─── public entry point ──────────────────────────────────────────────────────

func showWin32Selection() image.Rectangle {
	ch := make(chan image.Rectangle, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		overlayResultCh = ch
		ovPhase = ovPhaseIdle
		ovX0, ovY0, ovX1, ovY1 = 0, 0, 0, 0
		runOverlayMessageLoop()
	}()
	return <-ch
}

// ─── message loop ────────────────────────────────────────────────────────────

func runOverlayMessageLoop() {
	hInst := win.GetModuleHandle(nil)
	wc := win.WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
		Style:         win.CS_HREDRAW | win.CS_VREDRAW,
		LpfnWndProc:   syscall.NewCallback(overlayWndProc),
		HInstance:     hInst,
		HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_CROSS)),
		HbrBackground: win.HBRUSH(win.GetStockObject(win.BLACK_BRUSH)),
		LpszClassName: ovClassName,
	}
	win.RegisterClassEx(&wc)
	defer win.UnregisterClass(ovClassName)

	vx := win.GetSystemMetrics(smXVirtualScreen)
	vy := win.GetSystemMetrics(smYVirtualScreen)
	vw := win.GetSystemMetrics(smCXVirtualScreen)
	vh := win.GetSystemMetrics(smCYVirtualScreen)

	hwnd := win.CreateWindowEx(
		win.WS_EX_LAYERED|win.WS_EX_TOPMOST,
		ovClassName, nil,
		win.WS_POPUP|win.WS_VISIBLE,
		vx, vy, vw, vh,
		0, 0, hInst, nil,
	)
	if hwnd == 0 {
		overlayResultCh <- image.Rectangle{}
		return
	}
	setLayeredWindowAttributes(hwnd, keyColorRef, overlayOpacity, lwaColorKey|lwaAlpha)
	win.ShowWindow(hwnd, win.SW_SHOW)
	forceToForeground(hwnd)

	var msg win.MSG
	for win.GetMessage(&msg, 0, 0, 0) > 0 {
		win.TranslateMessage(&msg)
		win.DispatchMessage(&msg)
	}
}

// ─── window procedure ────────────────────────────────────────────────────────

func overlayWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_SETCURSOR:
		updateCursor()
		return 1

	case win.WM_LBUTTONDOWN:
		mx, my := unpackXY(lParam)
		onLButtonDown(hwnd, mx, my)
		return 0

	case win.WM_MOUSEMOVE:
		mx, my := unpackXY(lParam)
		onMouseMove(hwnd, mx, my)
		return 0

	case win.WM_LBUTTONUP:
		mx, my := unpackXY(lParam)
		onLButtonUp(hwnd, mx, my)
		return 0

	case win.WM_RBUTTONDOWN:
		overlayResultCh <- image.Rectangle{}
		win.DestroyWindow(hwnd)
		return 0

	case win.WM_KEYDOWN:
		switch wParam {
		case uintptr(win.VK_ESCAPE):
			overlayResultCh <- image.Rectangle{}
			win.DestroyWindow(hwnd)
		case uintptr(win.VK_RETURN):
			if ovPhase == ovPhaseReady {
				confirmSelection(hwnd)
			}
		}
		return 0

	case win.WM_ERASEBKGND:
		return 1

	case win.WM_PAINT:
		var ps win.PAINTSTRUCT
		hdc := win.BeginPaint(hwnd, &ps)
		paintOverlay(hwnd, hdc)
		win.EndPaint(hwnd, &ps)
		return 0

	case win.WM_DESTROY:
		win.PostQuitMessage(0)
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func unpackXY(lParam uintptr) (int32, int32) {
	return int32(int16(win.LOWORD(uint32(lParam)))),
		int32(int16(win.HIWORD(uint32(lParam))))
}

// ─── input handlers ──────────────────────────────────────────────────────────

func onLButtonDown(hwnd win.HWND, mx, my int32) {
	switch ovPhase {
	case ovPhaseIdle:
		ovX0, ovY0, ovX1, ovY1 = mx, my, mx, my
		ovPhase = ovPhaseDrag
		win.SetCapture(hwnd)

	case ovPhaseReady:
		hit := ovHitTest(mx, my)
		switch hit {
		case ovHitConfirm:
			confirmSelection(hwnd)
		case ovHitCancel:
			overlayResultCh <- image.Rectangle{}
			win.DestroyWindow(hwnd)
		case ovHitNone:
			// Start fresh selection
			ovX0, ovY0, ovX1, ovY1 = mx, my, mx, my
			ovPhase = ovPhaseDrag
			win.SetCapture(hwnd)
			win.InvalidateRect(hwnd, nil, true)
		case ovHitInside:
			ovPhase = ovPhaseMove
			ovDragHit = hit
			ovDragMX, ovDragMY = mx, my
			ovDragX0, ovDragY0, ovDragX1, ovDragY1 = ovX0, ovY0, ovX1, ovY1
			win.SetCapture(hwnd)
		default:
			ovPhase = ovPhaseResize
			ovDragHit = hit
			ovDragMX, ovDragMY = mx, my
			ovDragX0, ovDragY0, ovDragX1, ovDragY1 = ovX0, ovY0, ovX1, ovY1
			win.SetCapture(hwnd)
		}
	}
}

func onMouseMove(hwnd win.HWND, mx, my int32) {
	switch ovPhase {
	case ovPhaseDrag:
		ovX1, ovY1 = mx, my
		win.InvalidateRect(hwnd, nil, true)

	case ovPhaseMove:
		dx, dy := mx-ovDragMX, my-ovDragMY
		vw := int32(win.GetSystemMetrics(smCXVirtualScreen))
		vh := int32(win.GetSystemMetrics(smCYVirtualScreen))
		w := ovDragX1 - ovDragX0
		h := ovDragY1 - ovDragY0
		ovX0 = clampI(ovDragX0+dx, 0, vw-w)
		ovY0 = clampI(ovDragY0+dy, 0, vh-h)
		ovX1, ovY1 = ovX0+w, ovY0+h
		win.InvalidateRect(hwnd, nil, true)

	case ovPhaseResize:
		applyResize(mx-ovDragMX, my-ovDragMY)
		win.InvalidateRect(hwnd, nil, true)
	}
}

func onLButtonUp(hwnd win.HWND, mx, my int32) {
	switch ovPhase {
	case ovPhaseDrag:
		win.ReleaseCapture()
		ovX1, ovY1 = mx, my
		normalizeSelection()
		if ovX1-ovX0 >= 5 && ovY1-ovY0 >= 5 {
			ovPhase = ovPhaseReady
		} else {
			ovPhase = ovPhaseIdle
		}
		win.InvalidateRect(hwnd, nil, true)

	case ovPhaseMove, ovPhaseResize:
		win.ReleaseCapture()
		normalizeSelection()
		ovPhase = ovPhaseReady
		win.InvalidateRect(hwnd, nil, true)
	}
}

func normalizeSelection() {
	if ovX0 > ovX1 {
		ovX0, ovX1 = ovX1, ovX0
	}
	if ovY0 > ovY1 {
		ovY0, ovY1 = ovY1, ovY0
	}
}

func applyResize(dx, dy int32) {
	switch ovDragHit {
	case ovHitTL:
		ovX0, ovY0 = ovDragX0+dx, ovDragY0+dy
	case ovHitTC:
		ovY0 = ovDragY0 + dy
	case ovHitTR:
		ovX1, ovY0 = ovDragX1+dx, ovDragY0+dy
	case ovHitML:
		ovX0 = ovDragX0 + dx
	case ovHitMR:
		ovX1 = ovDragX1 + dx
	case ovHitBL:
		ovX0, ovY1 = ovDragX0+dx, ovDragY1+dy
	case ovHitBC:
		ovY1 = ovDragY1 + dy
	case ovHitBR:
		ovX1, ovY1 = ovDragX1+dx, ovDragY1+dy
	}
}

func confirmSelection(hwnd win.HWND) {
	x0, y0 := minI(ovX0, ovX1), minI(ovY0, ovY1)
	x1, y1 := maxI(ovX0, ovX1), maxI(ovY0, ovY1)
	if x1-x0 < 5 || y1-y0 < 5 {
		overlayResultCh <- image.Rectangle{}
	} else {
		ox := win.GetSystemMetrics(smXVirtualScreen)
		oy := win.GetSystemMetrics(smYVirtualScreen)
		overlayResultCh <- image.Rect(int(x0+ox), int(y0+oy), int(x1+ox), int(y1+oy))
	}
	win.DestroyWindow(hwnd)
}

// ─── hit testing ─────────────────────────────────────────────────────────────

func ovHitTest(mx, my int32) ovHitT {
	if ovPhase < ovPhaseReady {
		return ovHitNone
	}
	if inRect(mx, my, ovConfirmRect()) {
		return ovHitConfirm
	}
	if inRect(mx, my, ovCancelRect()) {
		return ovHitCancel
	}
	x0, y0, x1, y1 := ovX0, ovY0, ovX1, ovY1
	cx, cy := (x0+x1)/2, (y0+y1)/2
	type hp struct{ x, y int32; h ovHitT }
	for _, p := range []hp{
		{x0, y0, ovHitTL}, {cx, y0, ovHitTC}, {x1, y0, ovHitTR},
		{x0, cy, ovHitML}, {x1, cy, ovHitMR},
		{x0, y1, ovHitBL}, {cx, y1, ovHitBC}, {x1, y1, ovHitBR},
	} {
		if abs32(mx-p.x) <= handleHR && abs32(my-p.y) <= handleHR {
			return p.h
		}
	}
	if mx > x0 && mx < x1 && my > y0 && my < y1 {
		return ovHitInside
	}
	return ovHitNone
}

func inRect(mx, my int32, r win.RECT) bool {
	return mx >= r.Left && mx < r.Right && my >= r.Top && my < r.Bottom
}

func updateCursor() {
	var pt win.POINT
	win.GetCursorPos(&pt)
	pt.X -= win.GetSystemMetrics(smXVirtualScreen)
	pt.Y -= win.GetSystemMetrics(smYVirtualScreen)
	var id uintptr
	switch ovHitTest(pt.X, pt.Y) {
	case ovHitTL, ovHitBR:
		id = win.IDC_SIZENWSE
	case ovHitTR, ovHitBL:
		id = win.IDC_SIZENESW
	case ovHitTC, ovHitBC:
		id = win.IDC_SIZENS
	case ovHitML, ovHitMR:
		id = win.IDC_SIZEWE
	case ovHitInside:
		id = win.IDC_SIZEALL
	case ovHitConfirm, ovHitCancel:
		id = win.IDC_HAND
	default:
		id = win.IDC_CROSS
	}
	setCursor(win.LoadCursor(0, win.MAKEINTRESOURCE(uintptr(id))))
}

// ─── toolbar layout ──────────────────────────────────────────────────────────

func tbTotalW() int32 { return tbPad + tbBtnW + tbBtnG + tbBtnW + tbPad }
func tbTotalH() int32 { return tbPad + tbBtnH + tbPad }

func ovToolbarOrigin() (int32, int32) {
	vw := int32(win.GetSystemMetrics(smCXVirtualScreen))
	vh := int32(win.GetSystemMetrics(smCYVirtualScreen))
	tbX := ovX1 - tbTotalW()
	tbY := ovY1 + tbGap
	if tbY+tbTotalH() > vh {
		tbY = ovY0 - tbGap - tbTotalH()
	}
	tbX = clampI(tbX, 0, vw-tbTotalW())
	tbY = clampI(tbY, 0, vh-tbTotalH())
	return tbX, tbY
}

func ovConfirmRect() win.RECT {
	tx, ty := ovToolbarOrigin()
	return win.RECT{Left: tx + tbPad, Top: ty + tbPad,
		Right: tx + tbPad + tbBtnW, Bottom: ty + tbPad + tbBtnH}
}
func ovCancelRect() win.RECT {
	tx, ty := ovToolbarOrigin()
	off := tbPad + tbBtnW + tbBtnG
	return win.RECT{Left: tx + off, Top: ty + tbPad,
		Right: tx + off + tbBtnW, Bottom: ty + tbPad + tbBtnH}
}

// ─── painting ────────────────────────────────────────────────────────────────

func paintOverlay(_ win.HWND, hdc win.HDC) {
	// ── 1. Full-screen dim ──────────────────────────────────────────────────
	dimBr := createSolidBrush(clrDim)
	vw := int32(win.GetSystemMetrics(smCXVirtualScreen))
	vh := int32(win.GetSystemMetrics(smCYVirtualScreen))
	all := win.RECT{Left: 0, Top: 0, Right: vw, Bottom: vh}
	fillRect(hdc, &all, dimBr)
	win.DeleteObject(win.HGDIOBJ(dimBr))

	if ovPhase == ovPhaseIdle {
		paintHint(hdc, all)
		return
	}

	x0, y0 := minI(ovX0, ovX1), minI(ovY0, ovY1)
	x1, y1 := maxI(ovX0, ovX1), maxI(ovY0, ovY1)

	// ── 2. Transparent hole (magenta = key colour → fully transparent) ──────
	sel := win.RECT{Left: x0, Top: y0, Right: x1, Bottom: y1}
	keyBr := createSolidBrush(keyColorRef)
	fillRect(hdc, &sel, keyBr)
	win.DeleteObject(win.HGDIOBJ(keyBr))

	// ── 3. Outer soft border (1 px, darker blue) for depth ──────────────────
	outerPen := createPen(win.PS_SOLID, 1, clrBorderOuter)
	oldPen := win.SelectObject(hdc, win.HGDIOBJ(outerPen))
	nullBr := win.GetStockObject(win.NULL_BRUSH)
	oldBr := win.SelectObject(hdc, nullBr)
	win.Rectangle_(hdc, x0-1, y0-1, x1+1, y1+1)
	win.SelectObject(hdc, oldBr)
	win.SelectObject(hdc, oldPen)
	win.DeleteObject(win.HGDIOBJ(outerPen))

	// ── 4. Main border (2 px, bright cyan-blue) ──────────────────────────────
	mainPen := createPen(win.PS_SOLID, 2, clrBorder)
	oldPen = win.SelectObject(hdc, win.HGDIOBJ(mainPen))
	oldBr = win.SelectObject(hdc, nullBr)
	win.Rectangle_(hdc, x0, y0, x1, y1)
	win.SelectObject(hdc, oldBr)
	win.SelectObject(hdc, oldPen)
	win.DeleteObject(win.HGDIOBJ(mainPen))

	// ── 5. Size label (top-left, with background panel) ──────────────────────
	paintSizeLabel(hdc, x0, y0, x1, y1)

	// ── 6. Handles + toolbar (only in Ready/Move/Resize) ─────────────────────
	if ovPhase >= ovPhaseReady {
		paintHandles(hdc, x0, y0, x1, y1)
		paintToolbar(hdc)
	}
}

func paintHint(hdc win.HDC, cr win.RECT) {
	oldFont := selectUIFont(hdc, -15, false)
	defer func() {
		hf := win.SelectObject(hdc, oldFont)
		win.DeleteObject(hf)
	}()
	text := "拖动鼠标选择截图区域  •  Enter/双击 确认  •  Esc / 右键 取消"
	ptr, _ := syscall.UTF16PtrFromString(text)
	win.SetBkMode(hdc, win.TRANSPARENT)
	win.SetTextColor(hdc, clrTextHint)
	drawTextW(hdc, ptr, -1, &cr, dtCenter|dtVCenter|dtSingleLine|dtNoClip)
}

func paintSizeLabel(hdc win.HDC, x0, y0, x1, y1 int32) {
	w, h := x1-x0, y1-y0
	var buf [32]byte
	b := appendInt(buf[:0], int(w))
	b = append(b, " \xc3\x97 "...) // " × "
	b = appendInt(b, int(h))
	label := string(b)
	ptr, _ := syscall.UTF16PtrFromString(label)

	oldFont := selectUIFont(hdc, -13, true)

	// Measure text width to size the background panel
	const panelH = int32(24)
	const padX = int32(8)
	var sz win.SIZE
	win.GetTextExtentPoint32(hdc, ptr, int32(len([]rune(label))), &sz)
	panelW := sz.CX + padX*2

	lx := x0
	ly := y0 - panelH - 4
	if ly < 2 {
		ly = y0 + 4
	}

	// Background panel
	panelRect := win.RECT{Left: lx, Top: ly, Right: lx + panelW, Bottom: ly + panelH}
	panelBr := createSolidBrush(clrLabelBg)
	fillRect(hdc, &panelRect, panelBr)
	win.DeleteObject(win.HGDIOBJ(panelBr))

	// Panel border
	bdrBr := createSolidBrush(clrBorder)
	frameRect(hdc, &panelRect, bdrBr)
	win.DeleteObject(win.HGDIOBJ(bdrBr))

	// Text
	win.SetBkMode(hdc, win.TRANSPARENT)
	win.SetTextColor(hdc, clrTextWhite)
	textRect := win.RECT{Left: lx + padX, Top: ly, Right: lx + panelW - padX, Bottom: ly + panelH}
	drawTextW(hdc, ptr, -1, &textRect, dtCenter|dtVCenter|dtSingleLine|dtNoClip)

	hf := win.SelectObject(hdc, oldFont)
	win.DeleteObject(hf)
}

func paintHandles(hdc win.HDC, x0, y0, x1, y1 int32) {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	pts := [][2]int32{
		{x0, y0}, {cx, y0}, {x1, y0},
		{x0, cy}, {x1, cy},
		{x0, y1}, {cx, y1}, {x1, y1},
	}
	fillBr := createSolidBrush(clrHandleFill)
	bdrBr := createSolidBrush(clrHandleBdr)
	for _, p := range pts {
		hx, hy := p[0], p[1]
		outer := win.RECT{Left: hx - handleHS - 1, Top: hy - handleHS - 1,
			Right: hx + handleHS + 2, Bottom: hy + handleHS + 2}
		inner := win.RECT{Left: hx - handleHS, Top: hy - handleHS,
			Right: hx + handleHS + 1, Bottom: hy + handleHS + 1}
		fillRect(hdc, &inner, fillBr)
		frameRect(hdc, &outer, bdrBr)
	}
	win.DeleteObject(win.HGDIOBJ(fillBr))
	win.DeleteObject(win.HGDIOBJ(bdrBr))
}

func paintToolbar(hdc win.HDC) {
	tx, ty := ovToolbarOrigin()
	tw, th := tbTotalW(), tbTotalH()

	// ── Toolbar background with border ───────────────────────────────────────
	tbRect := win.RECT{Left: tx, Top: ty, Right: tx + tw, Bottom: ty + th}
	bgBr := createSolidBrush(clrTbBg)
	fillRect(hdc, &tbRect, bgBr)
	win.DeleteObject(win.HGDIOBJ(bgBr))
	bdrBr := createSolidBrush(clrTbBdr)
	frameRect(hdc, &tbRect, bdrBr)
	win.DeleteObject(win.HGDIOBJ(bdrBr))

	cr := ovConfirmRect()
	ca := ovCancelRect()

	// ── Confirm button (green) ───────────────────────────────────────────────
	confBr := createSolidBrush(clrConfirm)
	fillRect(hdc, &cr, confBr)
	win.DeleteObject(win.HGDIOBJ(confBr))
	// Inner highlight line at top edge for 3D feel
	hlBr := createSolidBrush(clrConfirmLight)
	hlLine := win.RECT{Left: cr.Left + 1, Top: cr.Top + 1, Right: cr.Right - 1, Bottom: cr.Top + 2}
	fillRect(hdc, &hlLine, hlBr)
	win.DeleteObject(win.HGDIOBJ(hlBr))

	// ── Cancel button (red) ──────────────────────────────────────────────────
	canBr := createSolidBrush(clrCancel)
	fillRect(hdc, &ca, canBr)
	win.DeleteObject(win.HGDIOBJ(canBr))
	hlBr2 := createSolidBrush(clrCancelLight)
	hlLine2 := win.RECT{Left: ca.Left + 1, Top: ca.Top + 1, Right: ca.Right - 1, Bottom: ca.Top + 2}
	fillRect(hdc, &hlLine2, hlBr2)
	win.DeleteObject(win.HGDIOBJ(hlBr2))

	// ── Button labels ─────────────────────────────────────────────────────────
	oldFont := selectUIFont(hdc, -15, true)
	win.SetBkMode(hdc, win.TRANSPARENT)
	win.SetTextColor(hdc, clrTextWhite)

	confPtr, _ := syscall.UTF16PtrFromString("✓  确认")
	drawTextW(hdc, confPtr, -1, &cr, dtCenter|dtVCenter|dtSingleLine|dtNoClip)

	cancelPtr, _ := syscall.UTF16PtrFromString("✕  取消")
	drawTextW(hdc, cancelPtr, -1, &ca, dtCenter|dtVCenter|dtSingleLine|dtNoClip)

	hf := win.SelectObject(hdc, oldFont)
	win.DeleteObject(hf)

	// ── Keyboard shortcut hints (smaller text, inside bottom of buttons) ─────
	oldFont2 := selectUIFont(hdc, -11, false)
	win.SetTextColor(hdc, win.COLORREF(0x00CCCCCC))

	// Place hints in the lower quarter of the button
	crHint := win.RECT{Left: cr.Left, Top: cr.Top + (tbBtnH * 6 / 8), Right: cr.Right, Bottom: cr.Bottom}
	entPtr, _ := syscall.UTF16PtrFromString("Enter")
	drawTextW(hdc, entPtr, -1, &crHint, dtCenter|dtVCenter|dtSingleLine|dtNoClip)

	caHint := win.RECT{Left: ca.Left, Top: ca.Top + (tbBtnH * 6 / 8), Right: ca.Right, Bottom: ca.Bottom}
	escPtr, _ := syscall.UTF16PtrFromString("Esc")
	drawTextW(hdc, escPtr, -1, &caHint, dtCenter|dtVCenter|dtSingleLine|dtNoClip)

	hf2 := win.SelectObject(hdc, oldFont2)
	win.DeleteObject(hf2)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func minI(a, b int32) int32 {
	if a < b { return a }; return b
}
func maxI(a, b int32) int32 {
	if a > b { return a }; return b
}
func clampI(v, lo, hi int32) int32 {
	if v < lo { return lo }
	if v > hi { return hi }
	return v
}
func abs32(v int32) int32 {
	if v < 0 { return -v }; return v
}
func appendInt(b []byte, n int) []byte {
	if n == 0 { return append(b, '0') }
	var tmp [10]byte
	i := len(tmp)
	for n > 0 {
		i--; tmp[i] = byte('0' + n%10); n /= 10
	}
	return append(b, tmp[i:]...)
}
