package capture

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"github.com/kbinani/screenshot"

	"github.com/ccdbp/screenshot-tool/internal/config"
	"github.com/ccdbp/screenshot-tool/internal/ocr"
	uipkg "github.com/ccdbp/screenshot-tool/internal/ui"
)

// TriggerCapture hides the main window, shows the Win32 selection overlay,
// captures the chosen region, then opens a Save-As dialog.
func TriggerCapture(a fyne.App, cfg *config.Config, parentWin fyne.Window) {
	fyne.DoAndWait(func() { parentWin.Hide() })

	rect := showWin32Selection()
	if rect.Empty() {
		fyne.Do(func() { parentWin.Show() })
		return
	}

	time.Sleep(80 * time.Millisecond)

	img, err := screenshot.CaptureRect(rect)
	if err != nil {
		fyne.Do(func() { parentWin.Show() })
		return
	}

	fyne.DoAndWait(func() {
		parentWin.Show()
		defaultName := time.Now().Format("screenshot_2006-01-02_150405.png")
		showSaveDialog(a, cfg.SaveDir, defaultName, img, parentWin)
	})
}

// showSaveDialog presents a Fyne file-save dialog. Must be called on the Fyne main thread.
func showSaveDialog(
	a fyne.App,
	defaultDir string,
	defaultName string,
	img image.Image,
	parentWin fyne.Window,
) {
	fd := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, parentWin)
			return
		}
		if writer == nil {
			return
		}
		defer writer.Close()

		if err := png.Encode(writer, img); err != nil {
			dialog.ShowError(err, parentWin)
			return
		}
		savedPath := uriToPath(writer.URI())
		a.SendNotification(fyne.NewNotification("截图已保存", savedPath))
	}, parentWin)

	fd.SetFileName(defaultName)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".png"}))

	if err := os.MkdirAll(defaultDir, 0755); err == nil {
		if lister, err := storage.ListerForURI(storage.NewFileURI(defaultDir)); err == nil {
			fd.SetLocation(lister)
		}
	}
	fd.Show()
}

// runOCRAndShow is the common helper for both OCR features.
// It hides mainWin, shows the overlay, captures, runs OCR, then calls showFn with the text.
// showFn is always called — on error it receives the error text so the user sees a window.
func runOCRAndShow(a fyne.App, cfg *config.Config, parentWin fyne.Window, showFn func(string)) {
	fyne.DoAndWait(func() { parentWin.Hide() })

	rect := showWin32Selection()
	if rect.Empty() {
		fyne.Do(func() { parentWin.Show() })
		return
	}

	time.Sleep(80 * time.Millisecond)

	img, err := screenshot.CaptureRect(rect)
	if err != nil {
		fyne.DoAndWait(func() {
			parentWin.Show()
			dialog.ShowError(fmt.Errorf("截图失败: %w", err), parentWin)
		})
		return
	}

	fyne.Do(func() { parentWin.Show() })

	// Save to temp PNG for OCR.
	tmp, err := os.CreateTemp("", "ocr_*.png")
	if err != nil {
		fyne.DoAndWait(func() {
			dialog.ShowError(fmt.Errorf("创建临时文件失败: %w", err), parentWin)
		})
		return
	}
	tmpPath := tmp.Name()
	if encErr := png.Encode(tmp, img); encErr != nil {
		tmp.Close()
		os.Remove(tmpPath)
		fyne.DoAndWait(func() {
			dialog.ShowError(fmt.Errorf("编码图片失败: %w", encErr), parentWin)
		})
		return
	}
	tmp.Close()

	opts := ocr.Options{
		Lang:             cfg.OCRLang,
		Provider:         cfg.OCRProvider,
		BaiduAPIKey:      cfg.BaiduOCRAPIKey,
		BaiduSecretKey:   cfg.BaiduOCRSecretKey,
		TencentSecretID:  cfg.TencentOCRSecretID,
		TencentSecretKey: cfg.TencentOCRSecretKey,
		TencentRegion:    cfg.TencentOCRRegion,
	}

	text, ocrErr := uipkg.RunOCR(tmpPath, opts)
	os.Remove(tmpPath)

	if ocrErr != nil {
		text = "OCR 识别失败：\n" + ocrErr.Error()
	} else if strings.TrimSpace(text) == "" {
		text = "（未识别到文字）"
	}

	fyne.DoAndWait(func() { showFn(text) })
}

// TriggerOCRCapture shows overlay → captures → runs OCR → shows result window.
func TriggerOCRCapture(a fyne.App, cfg *config.Config, parentWin fyne.Window) {
	runOCRAndShow(a, cfg, parentWin, func(text string) {
		uipkg.ShowOCRResult(a, text)
	})
}

// TriggerRecognizeCapture shows overlay → captures → runs OCR → shows 识字 window.
func TriggerRecognizeCapture(a fyne.App, cfg *config.Config, parentWin fyne.Window) {
	runOCRAndShow(a, cfg, parentWin, func(text string) {
		uipkg.ShowRecognizeResult(a, cfg, text)
	})
}

// uriToPath converts a Fyne URI to a native filesystem path.
func uriToPath(uri fyne.URI) string {
	p := uri.Path()
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}
