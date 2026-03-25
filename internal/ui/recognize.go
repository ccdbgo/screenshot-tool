package ui

import (
	"strings"

	goPinyin "github.com/mozillazg/go-pinyin"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ccdbp/screenshot-tool/internal/config"
	"github.com/ccdbp/screenshot-tool/internal/translate"
)

// ShowRecognizeResult displays a window with OCR text and options to convert
// to pinyin or translate to English using the configured provider.
func ShowRecognizeResult(a fyne.App, cfg *config.Config, text string) {
	w := a.NewWindow("截图识字")
	w.Resize(fyne.NewSize(660, 560))

	ocrEntry := widget.NewMultiLineEntry()
	ocrEntry.SetText(text)
	ocrEntry.Wrapping = fyne.TextWrapWord
	ocrEntry.SetMinRowsVisible(6)

	resultEntry := widget.NewMultiLineEntry()
	resultEntry.SetPlaceHolder("点击【转拼音】或【英文翻译】按钮…")
	resultEntry.Wrapping = fyne.TextWrapWord
	resultEntry.SetMinRowsVisible(6)

	statusLabel := widget.NewLabel("")

	copyOCRBtn := widget.NewButton("复制文字", func() {
		w.Clipboard().SetContent(ocrEntry.Text)
	})

	pinyinBtn := widget.NewButton("转拼音", func() {
		src := ocrEntry.Text
		statusLabel.SetText("转换中…")
		go func() {
			py := textToPinyin(src)
			fyne.Do(func() {
				resultEntry.SetText(py)
				statusLabel.SetText("")
			})
		}()
	})

	translateBtn := widget.NewButton("英文翻译", func() {
		src := ocrEntry.Text
		statusLabel.SetText("翻译中…")
		opts := translate.Options{
			Provider:         cfg.TransProvider,
			BaiduAppID:       cfg.BaiduTransAppID,
			BaiduSecretKey:   cfg.BaiduTransSecretKey,
			TencentSecretID:  cfg.TencentTransSecretID,
			TencentSecretKey: cfg.TencentTransSecretKey,
			TencentRegion:    cfg.TencentTransRegion,
			DeepLAuthKey:     cfg.DeepLAuthKey,
		}
		go func() {
			result, err := translate.Translate(src, opts)
			fyne.Do(func() {
				if err != nil {
					statusLabel.SetText("翻译失败：" + err.Error())
				} else {
					resultEntry.SetText(result)
					statusLabel.SetText("")
				}
			})
		}()
	})

	copyResultBtn := widget.NewButton("复制结果", func() {
		w.Clipboard().SetContent(resultEntry.Text)
	})
	closeBtn := widget.NewButton("关闭", func() { w.Close() })

	w.SetContent(container.NewPadded(container.NewVBox(
		widget.NewLabel("识别文字："),
		ocrEntry,
		container.NewHBox(copyOCRBtn),
		widget.NewSeparator(),
		container.NewHBox(pinyinBtn, translateBtn, statusLabel),
		widget.NewLabel("转换结果："),
		resultEntry,
		container.NewHBox(copyResultBtn, closeBtn),
	)))
	w.Show()
}

// textToPinyin converts Chinese characters to pinyin with tone marks.
// Non-Chinese characters are kept as-is.
func textToPinyin(text string) string {
	args := goPinyin.NewArgs()
	args.Style = goPinyin.Tone
	var sb strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\r' {
			sb.WriteRune(r)
			continue
		}
		py := goPinyin.Pinyin(string(r), args)
		if len(py) > 0 && len(py[0]) > 0 {
			sb.WriteString(py[0][0])
			sb.WriteByte(' ')
		} else {
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}
