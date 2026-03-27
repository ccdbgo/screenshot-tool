//go:build windows

package ocr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Options controls which OCR backend to use and carries provider credentials.
type Options struct {
	Lang     string // "zh-Hans", "en-US", etc. — empty = system default
	Provider string // "auto" | "baidu" | "tencent" | "windows"

	BaiduAPIKey    string
	BaiduSecretKey string

	TencentSecretID  string
	TencentSecretKey string
	TencentRegion    string // default: "ap-guangzhou"

	// API endpoint URL overrides (empty = use defaults)
	BaiduBaseURL    string // default: https://aip.baidubce.com
	TencentEndpoint string // default: https://ocr.tencentcloudapi.com
}

// RecognizeFile extracts text from imagePath using the configured provider.
// "auto": tries third-party providers in order, falls back to Windows OCR.
func RecognizeFile(imagePath string, opts Options) (string, error) {
	switch opts.Provider {
	case "baidu":
		if opts.BaiduAPIKey == "" || opts.BaiduSecretKey == "" {
			return "", fmt.Errorf("百度OCR：API Key 和 Secret Key 不能为空")
		}
		return baiduRecognize(imagePath, opts)

	case "tencent":
		if opts.TencentSecretID == "" || opts.TencentSecretKey == "" {
			return "", fmt.Errorf("腾讯OCR：SecretId 和 SecretKey 不能为空")
		}
		return tencentRecognize(imagePath, opts)

	case "windows":
		return windowsRecognize(imagePath, opts.Lang)

	default: // "auto"
		if opts.BaiduAPIKey != "" && opts.BaiduSecretKey != "" { //nolint:gocritic
			if text, err := baiduRecognize(imagePath, opts); err == nil {
				return text, nil
			}
		}
		if opts.TencentSecretID != "" && opts.TencentSecretKey != "" {
			if text, err := tencentRecognize(imagePath, opts); err == nil {
				return text, nil
			}
		}
		return windowsRecognize(imagePath, opts.Lang)
	}
}

// ─── Baidu OCR (aip.baidubce.com) ────────────────────────────────────────────

var (
	baiduTokenMu     sync.Mutex
	baiduTokenCache  = map[string]string{}   // key: apiKey → token
	baiduTokenExpiry = map[string]time.Time{}
	apiHTTP          = &http.Client{Timeout: 20 * time.Second}
)

type baiduTokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func getBaiduToken(apiKey, secretKey, baseURL string) (string, error) {
	baiduTokenMu.Lock()
	defer baiduTokenMu.Unlock()

	if tok, ok := baiduTokenCache[apiKey]; ok && time.Now().Before(baiduTokenExpiry[apiKey]) {
		return tok, nil
	}
	if baseURL == "" {
		baseURL = "https://aip.baidubce.com"
	}
	tokenURL := fmt.Sprintf(
		"%s/oauth/2.0/token?grant_type=client_credentials&client_id=%s&client_secret=%s",
		baseURL, url.QueryEscape(apiKey), url.QueryEscape(secretKey),
	)
	resp, err := apiHTTP.Post(tokenURL, "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("获取百度Token失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var tr baiduTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("解析Token响应失败: %w", err)
	}
	if tr.Error != "" {
		return "", fmt.Errorf("百度Token错误: %s — %s", tr.Error, tr.ErrorDesc)
	}
	baiduTokenCache[apiKey] = tr.AccessToken
	baiduTokenExpiry[apiKey] = time.Now().Add(time.Duration(tr.ExpiresIn-86400) * time.Second)
	return tr.AccessToken, nil
}

func baiduLangType(lang string) string {
	switch strings.ToLower(lang) {
	case "en", "en-us", "en-gb":
		return "ENG"
	case "ja", "ja-jp":
		return "JAP"
	case "ko", "ko-kr":
		return "KOR"
	case "fr", "fr-fr":
		return "FRE"
	case "de", "de-de":
		return "GER"
	default:
		return "CHN_ENG"
	}
}

type baiduOCRResp struct {
	ErrorCode   int    `json:"error_code"`
	ErrorMsg    string `json:"error_msg"`
	WordsResult []struct {
		Words string `json:"words"`
	} `json:"words_result"`
}

func baiduRecognize(imagePath string, opts Options) (string, error) {
	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", err
	}
	baseURL := opts.BaiduBaseURL
	if baseURL == "" {
		baseURL = "https://aip.baidubce.com"
	}
	token, err := getBaiduToken(opts.BaiduAPIKey, opts.BaiduSecretKey, baseURL)
	if err != nil {
		return "", err
	}
	apiURL := baseURL + "/rest/2.0/ocr/v1/general_basic?access_token=" + token
	params := url.Values{}
	params.Set("image", base64.StdEncoding.EncodeToString(imgData))
	params.Set("language_type", baiduLangType(opts.Lang))
	resp, err := apiHTTP.PostForm(apiURL, params)
	if err != nil {
		return "", fmt.Errorf("百度OCR请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result baiduOCRResp
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析OCR响应失败: %w", err)
	}
	if result.ErrorCode != 0 {
		return "", fmt.Errorf("百度OCR错误 %d: %s", result.ErrorCode, result.ErrorMsg)
	}
	lines := make([]string, 0, len(result.WordsResult))
	for _, w := range result.WordsResult {
		lines = append(lines, w.Words)
	}
	return strings.Join(lines, "\n"), nil
}

// ─── Tencent OCR (ocr.tencentcloudapi.com) ───────────────────────────────────

type tencentOCRResp struct {
	Response struct {
		TextDetections []struct {
			DetectedText string `json:"DetectedText"`
		} `json:"TextDetections"`
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"Response"`
}

func tencentRecognize(imagePath string, opts Options) (string, error) {
	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", err
	}

	region := opts.TencentRegion
	if region == "" {
		region = "ap-guangzhou"
	}

	payload := map[string]string{
		"ImageBase64": base64.StdEncoding.EncodeToString(imgData),
	}
	body, _ := json.Marshal(payload)

	endpoint := opts.TencentEndpoint
	if endpoint == "" {
		endpoint = "https://ocr.tencentcloudapi.com"
	}
	// Tencent Cloud API v3 signature
	req, err := tencentSign(
		opts.TencentSecretID, opts.TencentSecretKey,
		endpoint, region, "GeneralBasicOCR", "2018-11-19",
		string(body),
	)
	if err != nil {
		return "", err
	}

	resp, err := apiHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("腾讯OCR请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result tencentOCRResp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析腾讯OCR响应失败: %w", err)
	}
	if result.Response.Error != nil {
		return "", fmt.Errorf("腾讯OCR错误 %s: %s",
			result.Response.Error.Code, result.Response.Error.Message)
	}
	lines := make([]string, 0, len(result.Response.TextDetections))
	for _, t := range result.Response.TextDetections {
		lines = append(lines, t.DetectedText)
	}
	return strings.Join(lines, "\n"), nil
}

// tencentSign builds a signed *http.Request for Tencent Cloud API v3.
// endpoint is the full URL, e.g. "https://ocr.tencentcloudapi.com".
// The service name is derived from the first subdomain of the host.
func tencentSign(secretID, secretKey, endpoint, region, action, version, payload string) (*http.Request, error) {
	// Parse host and service name from endpoint URL.
	parsedURL, err := url.Parse(endpoint)
	if err != nil || parsedURL.Host == "" {
		return nil, fmt.Errorf("腾讯云：无效的 API 端点 URL: %s", endpoint)
	}
	host := parsedURL.Host
	// Derive service name from first subdomain (e.g. "ocr" from "ocr.tencentcloudapi.com")
	service := strings.SplitN(host, ".", 2)[0]

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	date := time.Now().UTC().Format("2006-01-02")

	// Step 1: canonical request
	canonHeaders := fmt.Sprintf("content-type:application/json\nhost:%s\nx-tc-action:%s\n",
		host, strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256hex(payload)
	canonReq := strings.Join([]string{"POST", "/", "", canonHeaders, signedHeaders, hashedPayload}, "\n")

	// Step 2: string to sign
	credScope := date + "/" + service + "/tc3_request"
	strToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + credScope + "\n" + sha256hex(canonReq)

	// Step 3: signing key
	signingKey := hmacSHA256(
		hmacSHA256(hmacSHA256(hmacSHA256([]byte("TC3"+secretKey), date), service), "tc3_request"),
		strToSign,
	)
	signature := hex.EncodeToString(signingKey)

	auth := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID, credScope, signedHeaders, signature,
	)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Region", region)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("Authorization", auth)
	return req, nil
}

func sha256hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// sortedKeys is used for deterministic header signing (unused here but kept for completeness).
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ─── Windows built-in OCR (PowerShell / Windows.Media.Ocr) ───────────────────

const psScript = `
param([string]$imagePath, [string]$lang)
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Media.Ocr.OcrEngine,Windows.Foundation,ContentType=WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapDecoder,Windows.Foundation,ContentType=WindowsRuntime]
$null = [Windows.Storage.StorageFile,Windows.Foundation,ContentType=WindowsRuntime]
$null = [Windows.Storage.Streams.RandomAccessStream,Windows.Foundation,ContentType=WindowsRuntime]

function Await($WinRtTask, $ResultType) {
    $methods = [System.WindowsRuntimeSystemExtensions].GetMethods()
    $asTask = ($methods | Where-Object {
        $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and !$_.IsGenericMethod
    })[0].MakeGenericMethod($ResultType)
    $netTask = $asTask.Invoke($null, @($WinRtTask))
    $netTask.Wait(-1) | Out-Null
    $netTask.Result
}

try {
    $storageFile = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($imagePath)) ([Windows.Storage.StorageFile])
    $stream = Await ($storageFile.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
    $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    $bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
    if ($lang -and $lang -ne "") {
        $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage([Windows.Globalization.Language]::new($lang))
    } else {
        $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
    }
    if ($null -eq $engine) { Write-Error "OCR engine not available"; exit 1 }
    $result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
    Write-Output $result.Text
} catch {
    Write-Error $_.Exception.Message; exit 1
}
`

func windowsRecognize(imagePath, lang string) (string, error) {
	winPath := filepath.FromSlash(imagePath)
	cmd := fmt.Sprintf(`& { %s } -imagePath '%s' -lang '%s'`, psScript, escapePS(winPath), lang)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx,
		"powershell.exe", "-WindowStyle", "Hidden", "-NonInteractive", "-NoProfile",
		"-Command", cmd,
	)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("Windows OCR 超时（30秒）")
		}
		return "", fmt.Errorf("Windows OCR 失败: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func escapePS(s string) string { return strings.ReplaceAll(s, "'", "''") }
