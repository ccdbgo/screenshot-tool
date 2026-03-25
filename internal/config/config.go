package config

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
)

// ─── preference keys ─────────────────────────────────────────────────────────

const (
	keySaveDir = "save_dir"
	keyOCRLang = "ocr_lang"

	// OCR provider: "auto" | "baidu" | "tencent" | "windows"
	keyOCRProvider = "ocr_provider"

	// Baidu OCR (aip.baidubce.com)
	keyBaiduOCRAPIKey = "baidu_ocr_api_key"
	keyBaiduOCRSecKey = "baidu_ocr_sec_key"

	// Tencent OCR (ocr.tencentcloudapi.com)
	keyTencentOCRSecretID  = "tencent_ocr_secret_id"
	keyTencentOCRSecretKey = "tencent_ocr_secret_key"
	keyTencentOCRRegion    = "tencent_ocr_region" // e.g. "ap-guangzhou"

	// Translation provider: "local" | "baidu" | "tencent" | "deepl" | "youdao"
	keyTransProvider = "trans_provider"

	// Baidu Translate (fanyi.baidu.com)
	keyBaiduTransAppID  = "baidu_trans_app_id"
	keyBaiduTransSecKey = "baidu_trans_sec_key"

	// Tencent Translate (tmt.tencentcloudapi.com)
	keyTencentTransSecretID  = "tencent_trans_secret_id"
	keyTencentTransSecretKey = "tencent_trans_secret_key"
	keyTencentTransRegion    = "tencent_trans_region"

	// DeepL (api-free.deepl.com)
	keyDeepLAuthKey = "deepl_auth_key"

	// Youdao Translate (openapi.youdao.com) — free unofficial endpoint, no key needed
	// (no key fields required for the unofficial endpoint)
)

// ─── Config ──────────────────────────────────────────────────────────────────

// Config holds all application settings.
type Config struct {
	SaveDir string
	OCRLang string // empty = system language; "zh-Hans", "en-US", …

	// OCR
	OCRProvider         string // "auto" | "baidu" | "tencent" | "windows"
	BaiduOCRAPIKey      string
	BaiduOCRSecretKey   string
	TencentOCRSecretID  string
	TencentOCRSecretKey string
	TencentOCRRegion    string

	// Translation / Pinyin
	TransProvider         string // "local" | "baidu" | "tencent" | "deepl" | "youdao"
	BaiduTransAppID       string
	BaiduTransSecretKey   string
	TencentTransSecretID  string
	TencentTransSecretKey string
	TencentTransRegion    string
	DeepLAuthKey          string

	prefs fyne.Preferences
}

// Load reads config from Fyne preferences with sensible defaults.
func Load(prefs fyne.Preferences) *Config {
	return &Config{
		SaveDir: prefs.StringWithFallback(keySaveDir, DefaultSaveDir()),
		OCRLang: prefs.String(keyOCRLang),

		OCRProvider:         prefs.StringWithFallback(keyOCRProvider, "auto"),
		BaiduOCRAPIKey:      prefs.String(keyBaiduOCRAPIKey),
		BaiduOCRSecretKey:   prefs.String(keyBaiduOCRSecKey),
		TencentOCRSecretID:  prefs.String(keyTencentOCRSecretID),
		TencentOCRSecretKey: prefs.String(keyTencentOCRSecretKey),
		TencentOCRRegion:    prefs.StringWithFallback(keyTencentOCRRegion, "ap-guangzhou"),

		TransProvider:         prefs.StringWithFallback(keyTransProvider, "local"),
		BaiduTransAppID:       prefs.String(keyBaiduTransAppID),
		BaiduTransSecretKey:   prefs.String(keyBaiduTransSecKey),
		TencentTransSecretID:  prefs.String(keyTencentTransSecretID),
		TencentTransSecretKey: prefs.String(keyTencentTransSecretKey),
		TencentTransRegion:    prefs.StringWithFallback(keyTencentTransRegion, "ap-guangzhou"),
		DeepLAuthKey:          prefs.String(keyDeepLAuthKey),

		prefs: prefs,
	}
}

// Save persists all fields to Fyne preferences.
func (c *Config) Save() {
	c.prefs.SetString(keySaveDir, c.SaveDir)
	c.prefs.SetString(keyOCRLang, c.OCRLang)

	c.prefs.SetString(keyOCRProvider, c.OCRProvider)
	c.prefs.SetString(keyBaiduOCRAPIKey, c.BaiduOCRAPIKey)
	c.prefs.SetString(keyBaiduOCRSecKey, c.BaiduOCRSecretKey)
	c.prefs.SetString(keyTencentOCRSecretID, c.TencentOCRSecretID)
	c.prefs.SetString(keyTencentOCRSecretKey, c.TencentOCRSecretKey)
	c.prefs.SetString(keyTencentOCRRegion, c.TencentOCRRegion)

	c.prefs.SetString(keyTransProvider, c.TransProvider)
	c.prefs.SetString(keyBaiduTransAppID, c.BaiduTransAppID)
	c.prefs.SetString(keyBaiduTransSecKey, c.BaiduTransSecretKey)
	c.prefs.SetString(keyTencentTransSecretID, c.TencentTransSecretID)
	c.prefs.SetString(keyTencentTransSecretKey, c.TencentTransSecretKey)
	c.prefs.SetString(keyTencentTransRegion, c.TencentTransRegion)
	c.prefs.SetString(keyDeepLAuthKey, c.DeepLAuthKey)
}

// DefaultSaveDir returns ~/Pictures/Screenshots.
func DefaultSaveDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Pictures", "Screenshots")
}
