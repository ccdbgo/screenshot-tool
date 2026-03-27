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

// ShowRecognizeResult displays a window with OCR text at the top and
// two tabs below: one for pinyin output, one for English translation.
func ShowRecognizeResult(a fyne.App, cfg *config.Config, text string) {
	w := a.NewWindow("截图识字")
	w.Resize(fyne.NewSize(700, 580))

	// ── OCR text area ──────────────────────────────────────────────────────
	ocrEntry := widget.NewMultiLineEntry()
	ocrEntry.SetText(text)
	ocrEntry.Wrapping = fyne.TextWrapWord
	ocrEntry.SetMinRowsVisible(5)

	copyOCRBtn := widget.NewButton("复制文字", func() {
		w.Clipboard().SetContent(ocrEntry.Text)
	})

	// ── Pinyin tab ─────────────────────────────────────────────────────────
	pinyinEntry := widget.NewMultiLineEntry()
	pinyinEntry.SetPlaceHolder("点击【转拼音】按钮生成拼音…")
	pinyinEntry.Wrapping = fyne.TextWrapWord

	pinyinStatus := widget.NewLabel("")

	copyPinyinBtn := widget.NewButton("复制拼音", func() {
		w.Clipboard().SetContent(pinyinEntry.Text)
	})
	copyPinyinBtn.Importance = widget.HighImportance

	pinyinBtn := widget.NewButton("转拼音", func() {
		src := ocrEntry.Text
		pinyinStatus.SetText("转换中…")
		go func() {
			py := textToPinyin(src)
			fyne.Do(func() {
				pinyinEntry.SetText(py)
				pinyinStatus.SetText("")
			})
		}()
	})
	pinyinBtn.Importance = widget.MediumImportance

	pinyinTab := container.NewBorder(
		container.NewPadded(container.NewHBox(pinyinBtn, pinyinStatus)),
		container.NewPadded(container.NewHBox(copyPinyinBtn)),
		nil, nil,
		container.NewScroll(pinyinEntry),
	)

	// ── Translation tab ────────────────────────────────────────────────────
	transEntry := widget.NewMultiLineEntry()
	transEntry.SetPlaceHolder("点击【英文翻译】按钮生成翻译…")
	transEntry.Wrapping = fyne.TextWrapWord

	transStatus := widget.NewLabel("")

	copyTransBtn := widget.NewButton("复制翻译", func() {
		w.Clipboard().SetContent(transEntry.Text)
	})
	copyTransBtn.Importance = widget.HighImportance

	translateBtn := widget.NewButton("英文翻译", func() {
		src := ocrEntry.Text
		transStatus.SetText("翻译中…")
		opts := translate.Options{
			Provider:         cfg.TransProvider,
			BaiduAppID:       cfg.BaiduTransAppID,
			BaiduSecretKey:   cfg.BaiduTransSecretKey,
			TencentSecretID:  cfg.TencentTransSecretID,
			TencentSecretKey: cfg.TencentTransSecretKey,
			TencentRegion:    cfg.TencentTransRegion,
			DeepLAuthKey:     cfg.DeepLAuthKey,
			BaiduBaseURL:     cfg.BaiduTransBaseURL,
			TencentEndpoint:  cfg.TencentTransEndpoint,
			DeepLFreeURL:     cfg.DeepLFreeBaseURL,
			DeepLPaidURL:     cfg.DeepLPaidBaseURL,
			YoudaoURL:        cfg.YoudaoBaseURL,
		}
		go func() {
			result, err := translate.Translate(src, opts)
			fyne.Do(func() {
				if err != nil {
					transEntry.SetText("翻译失败：\n" + err.Error())
				} else {
					transEntry.SetText(result)
				}
				transStatus.SetText("")
			})
		}()
	})
	translateBtn.Importance = widget.MediumImportance

	transTab := container.NewBorder(
		container.NewPadded(container.NewHBox(translateBtn, transStatus)),
		container.NewPadded(container.NewHBox(copyTransBtn)),
		nil, nil,
		container.NewScroll(transEntry),
	)

	// ── Tabs ───────────────────────────────────────────────────────────────
	tabs := container.NewAppTabs(
		container.NewTabItem("拼音", pinyinTab),
		container.NewTabItem("英文翻译", transTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	closeBtn := widget.NewButton("关闭", func() { w.Close() })

	top := container.NewVBox(
		widget.NewLabel("识别文字："),
		ocrEntry,
		container.NewHBox(copyOCRBtn),
		widget.NewSeparator(),
	)

	w.SetContent(container.NewPadded(container.NewBorder(
		top,
		container.NewPadded(closeBtn),
		nil, nil,
		tabs,
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
