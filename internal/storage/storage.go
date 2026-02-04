package storage

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/3stoneBrother/spider/internal/crawler"
)

// Storage 存储管理器
type Storage struct {
	baseDir string
}

// New 创建存储管理器
func New(baseDir string) *Storage {
	return &Storage{
		baseDir: baseDir,
	}
}

// Save 保存所有资源到文件系统
func (st *Storage) Save(resources map[string]*crawler.Resource) error {
	// 创建基础目录
	if err := os.MkdirAll(st.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create base directory: %v", err)
	}

	for _, resource := range resources {
		if err := st.saveResource(resource); err != nil {
			fmt.Printf("Warning: failed to save %s: %v\n", resource.URL, err)
			continue
		}
	}

	return nil
}

// saveResource 保存单个资源
func (st *Storage) saveResource(resource *crawler.Resource) error {
	if len(resource.Content) == 0 {
		return nil // 跳过空资源
	}

	// 解析URL并生成文件路径
	filePath, err := st.getFilePath(resource.URL)
	if err != nil {
		return err
	}

	// 创建目录
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, resource.Content, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %v", filePath, err)
	}

	return nil
}

// getFilePath 根据URL生成文件路径
func (st *Storage) getFilePath(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %v", err)
	}

	// 构建路径：baseDir/host/path
	host := parsedURL.Host
	if host == "" {
		host = "unknown"
	}

	// 清理并规范化主机名
	host = strings.ReplaceAll(host, ":", "_")

	// 获取路径部分
	path := parsedURL.Path
	if path == "" || path == "/" {
		path = "/index.html"
	}

	// 清理路径中的 ../ 和 ./，防止目录遍历
	path = filepath.Clean(path)
	path = strings.ReplaceAll(path, "..", "")

	// 移除开头的 /
	path = strings.TrimPrefix(path, "/")

	// 处理查询参数（将其作为文件名的一部分）
	if parsedURL.RawQuery != "" {
		// 清理查询字符串中的特殊字符
		query := sanitizeFileName(parsedURL.RawQuery)
		// 限制长度避免文件名过长
		if len(query) > 50 {
			query = query[:50]
		}
		path = path + "_" + query
	}

	// 如果路径为空或以 / 结尾，添加 index.html
	if path == "" || strings.HasSuffix(path, "/") {
		path = path + "index.html"
	}

	// 组合完整路径
	fullPath := filepath.Join(st.baseDir, host, path)

	// 确保路径在 baseDir 内（安全检查）
	absBase, _ := filepath.Abs(st.baseDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absBase) {
		// 如果路径不在 baseDir 内，强制放到 host 目录下
		safePath := filepath.Base(path)
		fullPath = filepath.Join(st.baseDir, host, safePath)
	}

	// 如果文件没有扩展名，尝试根据MIME类型添加
	if filepath.Ext(fullPath) == "" {
		fullPath = fullPath + ".html"
	}

	return fullPath, nil
}

// sanitizeFileName 清理文件名中的非法字符
func sanitizeFileName(name string) string {
	// 替换常见的非法字符
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"&", "_",
		"=", "_",
	)
	return replacer.Replace(name)
}

// GenerateReport 生成抓取报告
func (st *Storage) GenerateReport(resources map[string]*crawler.Resource) error {
	reportPath := filepath.Join(st.baseDir, "report.txt")

	var report strings.Builder
	report.WriteString("Spider Crawl Report\n")
	report.WriteString("==================\n\n")
	report.WriteString(fmt.Sprintf("Total Resources: %d\n\n", len(resources)))

	// 按类型分组统计
	typeCount := make(map[string]int)
	for _, res := range resources {
		mimeType := res.MimeType
		if mimeType == "" {
			mimeType = "unknown"
		}
		// 简化MIME类型
		parts := strings.Split(mimeType, ";")
		mimeType = strings.TrimSpace(parts[0])
		typeCount[mimeType]++
	}

	report.WriteString("Resources by Type:\n")
	for mimeType, count := range typeCount {
		report.WriteString(fmt.Sprintf("  %s: %d\n", mimeType, count))
	}

	report.WriteString("\n\nDetailed Resource List:\n")
	report.WriteString("----------------------\n")
	for _, res := range resources {
		report.WriteString(fmt.Sprintf("\nURL: %s\n", res.URL))
		report.WriteString(fmt.Sprintf("  Status: %d\n", res.StatusCode))
		report.WriteString(fmt.Sprintf("  Type: %s\n", res.MimeType))
		report.WriteString(fmt.Sprintf("  Size: %d bytes\n", len(res.Content)))
	}

	return os.WriteFile(reportPath, []byte(report.String()), 0644)
}
