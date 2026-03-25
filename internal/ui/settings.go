package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/ccdbp/screenshot-tool/internal/config"
)

// ShowSettings opens the settings window with tabs for each section.
func ShowSettings(a fyne.App, cfg *config.Config, parentWin fyne.Window) {
	w := a.NewWindow("设置")
	w.Resize(fyne.NewSize(560, 520))

	tabs := container.NewAppTabs(
		container.NewTabItem("通用", buildGeneralTab(cfg, w)),
		container.NewTabItem("OCR 服务", buildOCRTab(cfg)),
		container.NewTabItem("翻译服务", buildTransTab(cfg)),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	saveBtn := widget.NewButton("保存设置", func() {
		cfg.Save()
		w.Close()
	})
	saveBtn.Importance = widget.HighImportance

	w.SetContent(container.NewBorder(nil, container.NewPadded(saveBtn), nil, nil,
		container.NewPadded(tabs)))
	w.Show()
}

// ─── General tab ──────────────────────────────────────────────────────────────

func buildGeneralTab(cfg *config.Config, w fyne.Window) fyne.CanvasObject {
	saveDirLabel := widget.NewLabel(cfg.SaveDir)
	saveDirLabel.Wrapping = fyne.TextTruncate

	chooseDirBtn := widget.NewButton("浏览…", func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			cfg.SaveDir = uriToPath(uri)
			saveDirLabel.SetText(cfg.SaveDir)
		}, w)
		if lister, err := storage.ListerForURI(storage.NewFileURI(cfg.SaveDir)); err == nil {
			fd.SetLocation(lister)
		}
		fd.Show()
	})

	langEntry := widget.NewEntry()
	langEntry.SetPlaceHolder("留空使用系统语言，如 zh-Hans / en-US")
	langEntry.SetText(cfg.OCRLang)
	langEntry.OnChanged = func(s string) { cfg.OCRLang = s }

	return container.NewVBox(
		sectionLabel("截图保存位置"),
		container.NewBorder(nil, nil, nil, chooseDirBtn, saveDirLabel),
		widget.NewSeparator(),
		sectionLabel("OCR 识别语言（可选）"),
		langEntry,
	)
}

// ─── OCR tab ─────────────────────────────────────────────────────────────────

func buildOCRTab(cfg *config.Config) fyne.CanvasObject {
	providerOptions := []string{
		"auto（自动：依次尝试已配置服务，最终回退到 Windows 内置）",
		"baidu（仅百度 OCR）",
		"tencent（仅腾讯 OCR）",
		"windows（仅 Windows 内置）",
	}
	sel := widget.NewSelect(providerOptions, nil)
	switch cfg.OCRProvider {
	case "baidu":
		sel.SetSelected(providerOptions[1])
	case "tencent":
		sel.SetSelected(providerOptions[2])
	case "windows":
		sel.SetSelected(providerOptions[3])
	default:
		sel.SetSelected(providerOptions[0])
	}
	sel.OnChanged = func(s string) {
		switch s {
		case providerOptions[1]:
			cfg.OCRProvider = "baidu"
		case providerOptions[2]:
			cfg.OCRProvider = "tencent"
		case providerOptions[3]:
			cfg.OCRProvider = "windows"
		default:
			cfg.OCRProvider = "auto"
		}
	}

	// Baidu OCR fields
	baiduAPIKey := pwdEntry("API Key (AppID)", cfg.BaiduOCRAPIKey, func(s string) { cfg.BaiduOCRAPIKey = s })
	baiduSecKey := pwdEntry("Secret Key", cfg.BaiduOCRSecretKey, func(s string) { cfg.BaiduOCRSecretKey = s })
	baiduNote := noteLabel("免费额度：通用文字识别 50,000 次/天\n申请：console.bce.baidu.com → 文字识别")

	// Tencent OCR fields
	tencentSecID := pwdEntry("SecretId", cfg.TencentOCRSecretID, func(s string) { cfg.TencentOCRSecretID = s })
	tencentSecKey := pwdEntry("SecretKey", cfg.TencentOCRSecretKey, func(s string) { cfg.TencentOCRSecretKey = s })
	tencentRegion := plainEntry("地域（如 ap-guangzhou）", cfg.TencentOCRRegion, func(s string) { cfg.TencentOCRRegion = s })
	tencentNote := noteLabel("免费额度：通用印刷体识别 1,000 次/月\n申请：console.cloud.tencent.com → 文字识别")

	return container.NewVScroll(container.NewVBox(
		sectionLabel("服务商"),
		sel,
		widget.NewSeparator(),
		sectionLabel("百度 OCR"),
		baiduAPIKey, baiduSecKey, baiduNote,
		widget.NewSeparator(),
		sectionLabel("腾讯 OCR"),
		tencentSecID, tencentSecKey, tencentRegion, tencentNote,
	))
}

// ─── Translation tab ─────────────────────────────────────────────────────────

func buildTransTab(cfg *config.Config) fyne.CanvasObject {
	providerOptions := []string{
		"local（本地拼音，无需 API）",
		"youdao（有道翻译，免费非官方接口）",
		"baidu（百度翻译）",
		"tencent（腾讯翻译）",
		"deepl（DeepL，质量最佳）",
	}
	sel := widget.NewSelect(providerOptions, nil)
	switch cfg.TransProvider {
	case "youdao":
		sel.SetSelected(providerOptions[1])
	case "baidu":
		sel.SetSelected(providerOptions[2])
	case "tencent":
		sel.SetSelected(providerOptions[3])
	case "deepl":
		sel.SetSelected(providerOptions[4])
	default:
		sel.SetSelected(providerOptions[0])
	}
	sel.OnChanged = func(s string) {
		switch s {
		case providerOptions[1]:
			cfg.TransProvider = "youdao"
		case providerOptions[2]:
			cfg.TransProvider = "baidu"
		case providerOptions[3]:
			cfg.TransProvider = "tencent"
		case providerOptions[4]:
			cfg.TransProvider = "deepl"
		default:
			cfg.TransProvider = "local"
		}
	}

	// Baidu Translate
	baiduAppID := plainEntry("AppID", cfg.BaiduTransAppID, func(s string) { cfg.BaiduTransAppID = s })
	baiduSecKey := pwdEntry("Secret Key", cfg.BaiduTransSecretKey, func(s string) { cfg.BaiduTransSecretKey = s })
	baiduNote := noteLabel("免费标准版：每月 5 万字符\n申请：fanyi.baidu.com → 开放平台")

	// Tencent Translate
	tencentSecID := pwdEntry("SecretId", cfg.TencentTransSecretID, func(s string) { cfg.TencentTransSecretID = s })
	tencentSecKey := pwdEntry("SecretKey", cfg.TencentTransSecretKey, func(s string) { cfg.TencentTransSecretKey = s })
	tencentRegion := plainEntry("地域（如 ap-guangzhou）", cfg.TencentTransRegion, func(s string) { cfg.TencentTransRegion = s })
	tencentNote := noteLabel("免费额度：每月 500 万字符\n申请：console.cloud.tencent.com → 机器翻译")

	// DeepL
	deeplKey := pwdEntry("Auth Key（免费版以 :fx 结尾）", cfg.DeepLAuthKey, func(s string) { cfg.DeepLAuthKey = s })
	deeplNote := noteLabel("免费版：每月 50 万字符\n申请：www.deepl.com/pro-api")

	return container.NewVScroll(container.NewVBox(
		sectionLabel("翻译服务商"),
		sel,
		widget.NewSeparator(),
		sectionLabel("百度翻译"),
		baiduAppID, baiduSecKey, baiduNote,
		widget.NewSeparator(),
		sectionLabel("腾讯翻译"),
		tencentSecID, tencentSecKey, tencentRegion, tencentNote,
		widget.NewSeparator(),
		sectionLabel("DeepL"),
		deeplKey, deeplNote,
	))
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func sectionLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}

func noteLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

func pwdEntry(placeholder, value string, onChange func(string)) *widget.Entry {
	e := widget.NewPasswordEntry()
	e.SetPlaceHolder(placeholder)
	e.SetText(value)
	e.OnChanged = onChange
	return e
}

func plainEntry(placeholder, value string, onChange func(string)) *widget.Entry {
	e := widget.NewEntry()
	e.SetPlaceHolder(placeholder)
	e.SetText(value)
	e.OnChanged = onChange
	return e
}

// uriToPath converts a Fyne URI to a native filesystem path.
func uriToPath(uri fyne.URI) string {
	p := uri.Path()
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return p
}
