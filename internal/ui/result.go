package ui

import (
	"os/exec"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ccdbp/screenshot-tool/internal/ocr"
)

// RunOCR calls the OCR engine with the given options.
func RunOCR(imagePath string, opts ocr.Options) (string, error) {
	return ocr.RecognizeFile(imagePath, opts)
}

// ShowOCRResult displays the recognised text with copy action.
func ShowOCRResult(a fyne.App, text string) {
	w := a.NewWindow("提取文字结果")
	w.Resize(fyne.NewSize(620, 440))

	entry := widget.NewMultiLineEntry()
	entry.SetText(text)
	entry.Wrapping = fyne.TextWrapWord

	copyBtn := widget.NewButton("复制文字", func() {
		w.Clipboard().SetContent(entry.Text)
	})
	copyBtn.Importance = widget.HighImportance

	closeBtn := widget.NewButton("关闭", func() { w.Close() })

	content := container.NewBorder(
		nil,
		container.NewPadded(container.NewHBox(copyBtn, closeBtn)),
		nil, nil,
		container.NewScroll(entry),
	)
	w.SetContent(content)
	w.Show()
}

// openFile opens path with the OS default application.
func openFile(path string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("explorer", path).Start() //nolint:errcheck
	case "darwin":
		exec.Command("open", path).Start() //nolint:errcheck
	default:
		exec.Command("xdg-open", path).Start() //nolint:errcheck
	}
}
