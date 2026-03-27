// Package translate provides multi-provider Chinese→English translation.
package translate

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Options selects a translation backend and carries credentials.
type Options struct {
	// Provider: "local" | "baidu" | "tencent" | "deepl" | "youdao"
	Provider string

	// Baidu Translate  https://fanyi-api.baidu.com/
	BaiduAppID    string
	BaiduSecretKey string

	// Tencent Translate  https://cloud.tencent.com/product/tmt
	TencentSecretID  string
	TencentSecretKey string
	TencentRegion    string // default: "ap-guangzhou"

	// DeepL  https://www.deepl.com/pro-api  (free tier available)
	DeepLAuthKey string

	// API endpoint URL overrides (empty = use defaults)
	BaiduBaseURL    string // default: https://fanyi-api.baidu.com
	TencentEndpoint string // default: https://tmt.tencentcloudapi.com
	DeepLFreeURL    string // default: https://api-free.deepl.com
	DeepLPaidURL    string // default: https://api.deepl.com
	YoudaoURL       string // default: https://fanyi.youdao.com
}

var apiHTTP = &http.Client{Timeout: 15 * time.Second}

// Translate translates src (auto-detected, typically Chinese) into English.
func Translate(src string, opts Options) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", nil
	}
	switch opts.Provider {
	case "baidu":
		if opts.BaiduAppID == "" || opts.BaiduSecretKey == "" {
			return "", fmt.Errorf("百度翻译：AppID 和 SecretKey 不能为空")
		}
		return baiduTranslate(src, opts)
	case "tencent":
		if opts.TencentSecretID == "" || opts.TencentSecretKey == "" {
			return "", fmt.Errorf("腾讯翻译：SecretId 和 SecretKey 不能为空")
		}
		return tencentTranslate(src, opts)
	case "deepl":
		if opts.DeepLAuthKey == "" {
			return "", fmt.Errorf("DeepL：Auth Key 不能为空")
		}
		return deeplTranslate(src, opts)
	case "youdao":
		return youdaoTranslate(src, opts)
	default: // "local" — no API, just return placeholder
		return "", fmt.Errorf("请在设置中配置翻译服务商及 API Key")
	}
}

// ─── Baidu Translate ──────────────────────────────────────────────────────────

type baiduTransResp struct {
	ErrorCode string `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
	TransResult []struct {
		Dst string `json:"dst"`
	} `json:"trans_result"`
}

func baiduTranslate(src string, opts Options) (string, error) {
	salt := fmt.Sprintf("%d", time.Now().UnixNano())
	sign := md5sum(opts.BaiduAppID + src + salt + opts.BaiduSecretKey)

	params := url.Values{}
	params.Set("q", src)
	params.Set("from", "auto")
	params.Set("to", "en")
	params.Set("appid", opts.BaiduAppID)
	params.Set("salt", salt)
	params.Set("sign", sign)

	baseURL := opts.BaiduBaseURL
	if baseURL == "" {
		baseURL = "https://fanyi-api.baidu.com"
	}
	resp, err := apiHTTP.PostForm(baseURL+"/api/trans/vip/translate", params)
	if err != nil {
		return "", fmt.Errorf("百度翻译请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data baiduTransResp
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("解析百度翻译响应失败: %w", err)
	}
	if data.ErrorCode != "" && data.ErrorCode != "0" {
		return "", fmt.Errorf("百度翻译错误 %s: %s", data.ErrorCode, data.ErrorMsg)
	}
	parts := make([]string, 0, len(data.TransResult))
	for _, r := range data.TransResult {
		parts = append(parts, r.Dst)
	}
	return strings.Join(parts, "\n"), nil
}

func md5sum(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// ─── Tencent Translate ────────────────────────────────────────────────────────

type tencentTransResp struct {
	Response struct {
		TargetText string `json:"TargetText"`
		Error      *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"Response"`
}

func tencentTranslate(src string, opts Options) (string, error) {
	region := opts.TencentRegion
	if region == "" {
		region = "ap-guangzhou"
	}
	endpoint := opts.TencentEndpoint
	if endpoint == "" {
		endpoint = "https://tmt.tencentcloudapi.com"
	}
	payload := fmt.Sprintf(`{"SourceText":%s,"Source":"auto","Target":"en","ProjectId":0}`,
		jsonString(src))

	req, err := tencentSign(
		opts.TencentSecretID, opts.TencentSecretKey,
		endpoint, region, "TextTranslate", "2018-03-21", payload,
	)
	if err != nil {
		return "", err
	}
	resp, err := apiHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("腾讯翻译请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data tencentTransResp
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("解析腾讯翻译响应失败: %w", err)
	}
	if data.Response.Error != nil {
		return "", fmt.Errorf("腾讯翻译错误 %s: %s",
			data.Response.Error.Code, data.Response.Error.Message)
	}
	return data.Response.TargetText, nil
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ─── DeepL ───────────────────────────────────────────────────────────────────

type deeplResp struct {
	Translations []struct {
		Text string `json:"text"`
	} `json:"translations"`
	Message string `json:"message"`
}

func deeplTranslate(src string, opts Options) (string, error) {
	// Free tier uses api-free.deepl.com; paid uses api.deepl.com
	var baseURL string
	if strings.HasSuffix(opts.DeepLAuthKey, ":fx") {
		baseURL = opts.DeepLFreeURL
		if baseURL == "" {
			baseURL = "https://api-free.deepl.com"
		}
	} else {
		baseURL = opts.DeepLPaidURL
		if baseURL == "" {
			baseURL = "https://api.deepl.com"
		}
	}
	params := url.Values{}
	params.Set("auth_key", opts.DeepLAuthKey)
	params.Set("text", src)
	params.Set("target_lang", "EN")

	resp, err := apiHTTP.PostForm(baseURL+"/v2/translate", params)
	if err != nil {
		return "", fmt.Errorf("DeepL请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data deeplResp
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("解析DeepL响应失败: %w", err)
	}
	if data.Message != "" {
		return "", fmt.Errorf("DeepL错误: %s", data.Message)
	}
	parts := make([]string, 0, len(data.Translations))
	for _, t := range data.Translations {
		parts = append(parts, t.Text)
	}
	return strings.Join(parts, "\n"), nil
}

// ─── Youdao (unofficial free endpoint, no key needed) ────────────────────────

type youdaoResp struct {
	ErrorCode       int              `json:"errorCode"`
	TranslateResult [][]youdaoResult `json:"translateResult"`
}
type youdaoResult struct {
	Tgt string `json:"tgt"`
}

func youdaoTranslate(src string, opts Options) (string, error) {
	baseURL := opts.YoudaoURL
	if baseURL == "" {
		baseURL = "https://fanyi.youdao.com"
	}
	params := url.Values{}
	params.Set("type", "AUTO")
	params.Set("i", src)
	params.Set("doctype", "json")
	params.Set("version", "2.1")
	params.Set("keyfrom", "fanyi.web")
	// add random salt to avoid caching issues
	params.Set("salt", fmt.Sprintf("%d", rand.Int63()))

	resp, err := apiHTTP.PostForm(baseURL+"/translate", params)
	if err != nil {
		return "", fmt.Errorf("有道翻译请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var data youdaoResp
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("解析有道翻译响应失败: %w", err)
	}
	if data.ErrorCode != 0 {
		return "", fmt.Errorf("有道翻译错误码: %d", data.ErrorCode)
	}
	var parts []string
	for _, row := range data.TranslateResult {
		for _, item := range row {
			parts = append(parts, item.Tgt)
		}
	}
	return strings.Join(parts, " "), nil
}

// ─── Tencent Cloud API v3 signature (shared with OCR) ────────────────────────

// tencentSign builds a signed *http.Request for Tencent Cloud API v3.
// endpoint is the full URL, e.g. "https://tmt.tencentcloudapi.com".
// The service name is derived from the first subdomain of the host.
func tencentSign(secretID, secretKey, endpoint, region, action, version, payload string) (*http.Request, error) {
	parsedURL, parseErr := url.Parse(endpoint)
	if parseErr != nil || parsedURL.Host == "" {
		return nil, fmt.Errorf("腾讯云：无效的 API 端点 URL: %s", endpoint)
	}
	host := parsedURL.Host
	service := strings.SplitN(host, ".", 2)[0]

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	date := time.Now().UTC().Format("2006-01-02")

	canonHeaders := fmt.Sprintf("content-type:application/json\nhost:%s\nx-tc-action:%s\n",
		host, strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256hex(payload)
	canonReq := strings.Join([]string{"POST", "/", "", canonHeaders, signedHeaders, hashedPayload}, "\n")

	credScope := date + "/" + service + "/tc3_request"
	strToSign := "TC3-HMAC-SHA256\n" + timestamp + "\n" + credScope + "\n" + sha256hex(canonReq)

	sigKey := hmacSHA256(
		hmacSHA256(hmacSHA256(hmacSHA256([]byte("TC3"+secretKey), date), service), "tc3_request"),
		strToSign,
	)
	auth := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		secretID, credScope, signedHeaders, hex.EncodeToString(sigKey),
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
	h := sha256.New(); h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key); h.Write([]byte(data))
	return h.Sum(nil)
}
