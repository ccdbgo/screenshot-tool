package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.design/x/hotkey"

	"github.com/ccdbp/screenshot-tool/internal/capture"
	"github.com/ccdbp/screenshot-tool/internal/config"
	uipkg "github.com/ccdbp/screenshot-tool/internal/ui"
)

func main() {
	a := app.NewWithID("com.ccdbp.screenshottool")
	a.Settings().SetTheme(uipkg.NewElegantTheme())
	if resourceIconPng != nil {
		a.SetIcon(resourceIconPng)
	}

	cfg := config.Load(a.Preferences())

	mainWin := a.NewWindow("易截")
	mainWin.Resize(fyne.NewSize(460, 420))
	mainWin.SetCloseIntercept(func() { mainWin.Hide() })

	// System tray
	if desk, ok := a.(desktop.App); ok {
		menu := fyne.NewMenu("易截",
			fyne.NewMenuItem("截图  (Ctrl+PrtSc)", func() {
				go capture.TriggerCapture(a, cfg, mainWin)
			}),
			fyne.NewMenuItem("截图提取文字", func() {
				go capture.TriggerOCRCapture(a, cfg, mainWin)
			}),
			fyne.NewMenuItem("截图识字", func() {
				go capture.TriggerRecognizeCapture(a, cfg, mainWin)
			}),
			fyne.NewMenuItem("设置", func() {
				uipkg.ShowSettings(a, cfg, mainWin)
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("退出", func() { a.Quit() }),
		)
		desk.SetSystemTrayMenu(menu)
		if resourceIconPng != nil {
			desk.SetSystemTrayIcon(resourceIconPng)
		}
	}

	// Global hotkey: Ctrl+PrintScreen
	hk := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl}, hotkey.Key(0x2C))
	if err := hk.Register(); err != nil {
		log.Printf("无法注册热键: %v", err)
	} else {
		go func() {
			for range hk.Keydown() {
				go capture.TriggerCapture(a, cfg, mainWin)
			}
		}()
		defer hk.Unregister() //nolint:errcheck
	}

	mainWin.SetContent(buildUI(a, cfg, mainWin))
	mainWin.ShowAndRun()
}

// featureItem builds a full-width button with a single-line description below.
func featureItem(
	label string,
	icon fyne.Resource,
	importance widget.ButtonImportance,
	desc string,
	action func(),
) fyne.CanvasObject {
	btn := widget.NewButtonWithIcon(label, icon, action)
	btn.Importance = importance

	descLabel := widget.NewLabel(desc)
	descLabel.Alignment = fyne.TextAlignCenter
	descLabel.TextStyle = fyne.TextStyle{Italic: true}

	return container.NewVBox(btn, descLabel)
}

func buildUI(a fyne.App, cfg *config.Config, mainWin fyne.Window) fyne.CanvasObject {
	// ── Title ─────────────────────────────────────────────────────────────────
	title := widget.NewRichTextFromMarkdown("# 易截")
	for _, seg := range title.Segments {
		if ts, ok := seg.(*widget.TextSegment); ok {
			ts.Style.Alignment = fyne.TextAlignCenter
		}
	}

	subtitle := widget.NewLabel("智能截图  ·  文字识别  ·  拼音标注  ·  英文翻译")
	subtitle.Alignment = fyne.TextAlignCenter

	// ── Feature sections ──────────────────────────────────────────────────────
	screenshotItem := featureItem(
		"截图",
		theme.ViewFullScreenIcon(),
		widget.HighImportance,
		"截图并提供保存文件功能",
		func() { go capture.TriggerCapture(a, cfg, mainWin) },
	)

	ocrItem := featureItem(
		"截图提取文字",
		theme.FileTextIcon(),
		widget.HighImportance,
		"提取截图中的文字并提供复制功能",
		func() { go capture.TriggerOCRCapture(a, cfg, mainWin) },
	)

	recognizeItem := featureItem(
		"截图识字",
		theme.SearchIcon(),
		widget.HighImportance,
		"提取截图中的文字并提供汉语拼音或英文翻译",
		func() { go capture.TriggerRecognizeCapture(a, cfg, mainWin) },
	)

	// ── Settings ──────────────────────────────────────────────────────────────
	settingsBtn := widget.NewButtonWithIcon("设置", theme.SettingsIcon(), func() {
		uipkg.ShowSettings(a, cfg, mainWin)
	})
	settingsBtn.Importance = widget.LowImportance

	// ── Layout ───────────────────────────────────────────────────────────────
	featuresCard := widget.NewCard("", "", container.NewVBox(
		screenshotItem,
		widget.NewSeparator(),
		ocrItem,
		widget.NewSeparator(),
		recognizeItem,
	))

	footer := container.NewBorder(nil, nil,
		nil,
		settingsBtn,
		widget.NewLabel("Ctrl+PrtSc 快速截图  •  最小化后可从托盘访问"),
	)

	return container.NewPadded(container.NewVBox(
		container.NewCenter(title),
		subtitle,
		widget.NewSeparator(),
		featuresCard,
		footer,
	))
}
