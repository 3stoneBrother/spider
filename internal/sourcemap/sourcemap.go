package sourcemap

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/3stoneBrother/spider/internal/crawler"
)

// SourceMap 表示source map文件的结构
type SourceMap struct {
	Version        int      `json:"version"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
	File           string   `json:"file"`
	SourceRoot     string   `json:"sourceRoot"`
}

// Extractor source map提取器
type Extractor struct {
	baseURL string
	client  *http.Client
}

// New 创建source map提取器
func New(baseURL string) *Extractor {
	return &Extractor{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * 1000000000, // 30 seconds
		},
	}
}

// ExtractFromResource 从资源中提取source map
func (sme *Extractor) ExtractFromResource(res *crawler.Resource) ([]*crawler.Resource, error) {
	// 只处理 JavaScript 和 CSS 文件
	if !strings.Contains(res.MimeType, "javascript") && !strings.Contains(res.MimeType, "css") {
		return nil, nil
	}

	// 查找sourceMappingURL
	sourceMapURL := sme.findSourceMapURL(string(res.Content))
	if sourceMapURL == "" {
		return nil, nil
	}

	// 构建完整的source map URL
	fullURL, err := sme.buildSourceMapURL(res.URL, sourceMapURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build source map URL: %v", err)
	}

	log.Printf("发现 Source Map: %s", fullURL)

	// 下载source map
	sourceMapContent, err := sme.downloadSourceMap(fullURL)
	if err != nil {
		log.Printf("警告: 下载 source map 失败: %v", err)
		return nil, nil
	}

	// 解析source map
	sourceMap, err := sme.parseSourceMap(sourceMapContent)
	if err != nil {
		log.Printf("警告: 解析 source map 失败: %v", err)
		return nil, nil
	}

	// 提取源代码文件
	resources := sme.extractSourceFiles(sourceMap, fullURL)

	log.Printf("从 Source Map 提取了 %d 个源文件", len(resources))

	return resources, nil
}

// findSourceMapURL 查找sourceMappingURL注释
func (sme *Extractor) findSourceMapURL(content string) string {
	// 匹配 //# sourceMappingURL=xxx 或 /*# sourceMappingURL=xxx */
	patterns := []string{
		`//# sourceMappingURL=(.+)`,
		`/\*# sourceMappingURL=(.+)\*/`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}

// buildSourceMapURL 构建完整的source map URL
func (sme *Extractor) buildSourceMapURL(baseURL, sourceMapURL string) (string, error) {
	// 如果是完整URL，直接返回
	if strings.HasPrefix(sourceMapURL, "http://") || strings.HasPrefix(sourceMapURL, "https://") {
		return sourceMapURL, nil
	}

	// 解析基础URL
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// 解析相对URL
	relative, err := url.Parse(sourceMapURL)
	if err != nil {
		return "", err
	}

	// 合并URL
	fullURL := base.ResolveReference(relative)
	return fullURL.String(), nil
}

// downloadSourceMap 下载source map文件
func (sme *Extractor) downloadSourceMap(url string) ([]byte, error) {
	resp, err := sme.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// parseSourceMap 解析source map JSON
func (sme *Extractor) parseSourceMap(content []byte) (*SourceMap, error) {
	var sourceMap SourceMap
	err := json.Unmarshal(content, &sourceMap)
	if err != nil {
		return nil, err
	}

	return &sourceMap, nil
}

// extractSourceFiles 从source map中提取源文件
func (sme *Extractor) extractSourceFiles(sm *SourceMap, sourceMapURL string) []*crawler.Resource {
	var resources []*crawler.Resource

	// 解析source map URL以获取基础路径
	parsedURL, err := url.Parse(sourceMapURL)
	if err != nil {
		return resources
	}

	for i, sourcePath := range sm.Sources {
		// 跳过空源文件
		if i >= len(sm.SourcesContent) || sm.SourcesContent[i] == "" {
			continue
		}

		// 清理源文件路径
		cleanPath := sme.cleanSourcePath(sourcePath, sm.SourceRoot)

		// 构建完整的源文件URL
		sourceURL := sme.buildSourceURL(parsedURL, cleanPath)

		// 创建资源
		resource := &crawler.Resource{
			URL:        sourceURL,
			StatusCode: 200,
			MimeType:   sme.guessMimeType(cleanPath),
			Content:    []byte(sm.SourcesContent[i]),
			Headers:    map[string]string{"X-Source": "SourceMap"},
		}

		resources = append(resources, resource)
	}

	return resources
}

// cleanSourcePath 清理源文件路径
func (sme *Extractor) cleanSourcePath(sourcePath, sourceRoot string) string {
	// 移除 webpack:// 等前缀
	cleanPath := strings.TrimPrefix(sourcePath, "webpack://")
	cleanPath = strings.TrimPrefix(cleanPath, "webpack:///")

	// 移除 ./
	cleanPath = strings.TrimPrefix(cleanPath, "./")

	// 如果有 sourceRoot，也要处理
	if sourceRoot != "" {
		cleanRoot := strings.TrimPrefix(sourceRoot, "webpack://")
		cleanRoot = strings.TrimPrefix(cleanRoot, "/")
		cleanPath = filepath.Join(cleanRoot, cleanPath)
	}

	return cleanPath
}

// buildSourceURL 构建源文件的完整URL
func (sme *Extractor) buildSourceURL(baseURL *url.URL, sourcePath string) string {
	// 使用基础URL的scheme和host
	return fmt.Sprintf("%s://%s/%s", baseURL.Scheme, baseURL.Host, sourcePath)
}

// guessMimeType 根据文件扩展名猜测MIME类型
func (sme *Extractor) guessMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	mimeTypes := map[string]string{
		".js":   "application/javascript",
		".jsx":  "application/javascript",
		".ts":   "application/typescript",
		".tsx":  "application/typescript",
		".css":  "text/css",
		".scss": "text/x-scss",
		".sass": "text/x-sass",
		".less": "text/x-less",
		".json": "application/json",
		".html": "text/html",
		".vue":  "text/x-vue",
	}

	if mimeType, ok := mimeTypes[ext]; ok {
		return mimeType
	}

	return "text/plain"
}
