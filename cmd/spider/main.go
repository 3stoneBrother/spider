package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"spider/internal/crawler"
	"spider/internal/sourcemap"
	"spider/internal/storage"
)

// headerFlags 用于支持多次使用 -header 参数
type headerFlags []string

func (h *headerFlags) String() string      { return strings.Join(*h, ", ") }
func (h *headerFlags) Set(value string) error { *h = append(*h, value); return nil }

// ManifestEntry 记录每个 URL 的爬取结果
type ManifestEntry struct {
	URL       string `json:"url"`
	OutputDir string `json:"output_dir"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Attempts  int    `json:"attempts"`
}

func main() {
	var (
		targetURL   string
		urlFile     string
		outputDir   string
		timeout     int
		idleTimeout int
		cookie      string
		headers     headerFlags
		proxy       string
		userAgent   string
		concurrency int
		headless    bool
		maxRetry    int
		chromePath  string
		showHelp    bool
	)

	flag.StringVar(&targetURL, "url", "", "目标网页URL（与 -file 二选一）")
	flag.StringVar(&urlFile, "file", "", "URL文件路径，每行一个URL（与 -url 二选一）")
	flag.StringVar(&outputDir, "output", "./output", "输出目录")
	flag.IntVar(&timeout, "timeout", 30, "爬取超时时间（秒）")
	flag.IntVar(&idleTimeout, "idle-timeout", 10, "网络空闲等待上限（秒）")
	flag.StringVar(&cookie, "cookie", "", "Cookie字符串，格式: \"key1=value1; key2=value2\"")
	flag.Var(&headers, "header", "自定义Header，格式: \"Key:Value\"（可多次使用）")
	flag.StringVar(&proxy, "proxy", "", "HTTP/SOCKS5代理地址，如 \"http://127.0.0.1:8080\"")
	flag.StringVar(&userAgent, "ua", "", "自定义 User-Agent")
	flag.StringVar(&chromePath, "chrome-path", "", "Chrome/Chromium 可执行文件路径（默认自动搜索）")
	flag.IntVar(&concurrency, "concurrency", 1, "并发数（批量爬取时）")
	flag.BoolVar(&headless, "headless", true, "无头模式（默认true）")
	flag.IntVar(&maxRetry, "retry", 2, "失败重试次数（默认 2，指数退避）")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")

	flag.Parse()

	if showHelp {
		showUsage()
		return
	}

	// 校验并发数，防止 concurrency=0 时 semaphore 死锁
	if concurrency <= 0 {
		fmt.Fprintf(os.Stderr, "错误: 并发数必须大于 0，当前值: %d\n", concurrency)
		os.Exit(1)
	}

	if targetURL == "" && urlFile == "" {
		fmt.Fprintln(os.Stderr, "错误: 必须指定 -url 或 -file 参数")
		showUsage()
		os.Exit(1)
	}

	if targetURL != "" && urlFile != "" {
		fmt.Fprintln(os.Stderr, "错误: -url 和 -file 参数只能选择一个")
		showUsage()
		os.Exit(1)
	}

	// 解析 headers，并过滤含换行符的注入攻击
	headerMap := make(map[string]string)
	for _, h := range headers {
		idx := strings.Index(h, ":")
		if idx == -1 {
			log.Printf("警告: 忽略无效的 Header 格式: %s (应为 Key:Value)", h)
			continue
		}
		key := strings.TrimSpace(h[:idx])
		value := strings.TrimSpace(h[idx+1:])
		if strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			log.Printf("警告: 忽略包含非法字符的 Header: %s", h)
			continue
		}
		headerMap[key] = value
	}

	config := &crawler.Config{
		Timeout:     time.Duration(timeout) * time.Second,
		IdleTimeout: time.Duration(idleTimeout) * time.Second,
		Cookies:     cookie,
		Headers:     headerMap,
		Proxy:       proxy,
		UserAgent:   userAgent,
		ChromePath:  chromePath,
		Headless:    headless,
		Concurrency: concurrency,
		MaxRetry:    maxRetry,
	}

	log.Printf("Spider - 浏览器模拟爬虫工具")
	log.Printf("================================")
	log.Printf("输出目录: %s", outputDir)
	log.Printf("超时时间: %d秒 / 空闲等待上限: %d秒", timeout, idleTimeout)
	log.Printf("无头模式: %v / 重试次数: %d", headless, maxRetry)
	if len(headerMap) > 0 {
		log.Printf("自定义Headers: %v", headerMap)
	}
	if proxy != "" {
		log.Printf("代理: %s", proxy)
	}
	if userAgent != "" {
		log.Printf("User-Agent: %s", userAgent)
	}
	log.Printf("================================\n")

	// 获取 URL 列表
	var urls []string
	if targetURL != "" {
		urls = []string{targetURL}
	} else {
		var err error
		urls, err = readURLsFromFile(urlFile)
		if err != nil {
			log.Fatalf("读取URL文件失败: %v", err)
		}
		log.Printf("从文件读取了 %d 个URL，并发数: %d", len(urls), concurrency)
	}

	if len(urls) == 1 {
		crawlSingleURL(urls[0], config, outputDir)
	} else {
		crawlMultipleURLs(urls, config, outputDir)
	}
}

// crawlSingleURL 爬取单个URL（含重试）
func crawlSingleURL(targetURL string, config *crawler.Config, outputDir string) {
	log.Printf("目标URL: %s", targetURL)
	if _, err := crawlWithRetry(targetURL, config, outputDir); err != nil {
		log.Printf("\n错误: %v\n", err)
		if strings.Contains(err.Error(), "chrome failed to start") {
			log.Fatalf(`
Chrome 浏览器启动失败！

请确保系统中已安装 Chrome 或 Chromium 浏览器：

macOS:
  brew install --cask google-chrome

Linux (Ubuntu/Debian):
  sudo apt-get install chromium-browser

Windows:
  从 https://www.google.com/chrome/ 下载安装
`)
		}
		os.Exit(1)
	}
}

// crawlMultipleURLs 批量爬取：预启动浏览器池，按 hostname 分配输出目录，并行执行，最终写 manifest
func crawlMultipleURLs(urls []string, config *crawler.Config, baseOutputDir string) {
	// 预先按 hostname 分配稳定的输出目录，重复 host 加数字后缀
	type task struct {
		url       string
		outputDir string
	}
	usedDirs := make(map[string]int)
	tasks := make([]task, len(urls))
	for i, u := range urls {
		tasks[i] = task{
			url:       u,
			outputDir: buildBatchOutputDir(baseOutputDir, u, usedDirs),
		}
	}

	// 预热浏览器池：N 个 Chrome 进程对应 N 并发，避免每 URL 冷启动
	pool, err := crawler.NewPool(config)
	if err != nil {
		log.Fatalf("浏览器池启动失败: %v", err)
	}
	defer pool.Close()

	// Pool 的 channel 本身充当并发限制器，无需额外 semaphore
	var wg sync.WaitGroup
	entries := make([]ManifestEntry, len(tasks))
	var mu sync.Mutex
	successCount, failCount := 0, 0

	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, t task) {
			defer wg.Done()

			// Acquire 阻塞直到有空闲浏览器进程
			allocCtx := pool.Acquire()
			defer pool.Release(allocCtx)

			log.Printf("[%d/%d] 开始爬取: %s → %s", idx+1, len(tasks), t.url, t.outputDir)

			entry := ManifestEntry{
				URL:       t.url,
				OutputDir: t.outputDir,
			}

			used, err := crawlWithRetryInContext(allocCtx, t.url, config, t.outputDir)
			entry.Attempts = used
			if err != nil {
				entry.Success = false
				entry.Error = err.Error()
				log.Printf("[%d/%d] 全部重试失败: %s - %v", idx+1, len(tasks), t.url, err)
				mu.Lock()
				failCount++
				mu.Unlock()
			} else {
				entry.Success = true
				log.Printf("[%d/%d] 完成 (第 %d 次成功): %s", idx+1, len(tasks), used, t.url)
				mu.Lock()
				successCount++
				mu.Unlock()
			}

			mu.Lock()
			entries[idx] = entry
			mu.Unlock()
		}(i, t)
	}

	wg.Wait()

	writeManifest(baseOutputDir, entries)

	log.Printf("\n================================")
	log.Printf("批量爬取完成!")
	log.Printf("成功: %d, 失败: %d, 总计: %d", successCount, failCount, len(tasks))
	log.Printf("结果清单: %s/manifest.json", baseOutputDir)
	log.Printf("================================")
}

// crawlWithRetry 爬取单个 URL，失败时按指数退避重试。
// 返回实际尝试次数和错误；永久性错误（URL 非法）立即返回，不消耗重试次数。
func crawlWithRetry(targetURL string, config *crawler.Config, outputDir string) (attempts int, err error) {
	// 永久性错误：URL scheme 不合法，无需重试
	u, parseErr := url.Parse(targetURL)
	if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") {
		scheme := ""
		if u != nil {
			scheme = u.Scheme
		}
		return 0, fmt.Errorf("不支持的 URL scheme %q：仅允许 http 和 https", scheme)
	}

	maxAttempts := config.MaxRetry + 1
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			backoff := time.Duration(attempt-1) * 3 * time.Second
			log.Printf("  [重试 %d/%d] 等待 %v 后重试: %s", attempt-1, config.MaxRetry, backoff, targetURL)
			time.Sleep(backoff)
		}

		spider := crawler.New(config)
		if err := spider.Crawl(targetURL); err != nil {
			lastErr = err
			log.Printf("  [尝试 %d/%d] 失败: %v", attempt, maxAttempts, err)
			continue
		}

		processResources(spider, targetURL, outputDir, false)
		return attempt, nil
	}

	return maxAttempts, fmt.Errorf("已重试 %d 次，最后错误: %w", config.MaxRetry, lastErr)
}

// crawlWithRetryInContext 在浏览器池的 allocCtx 中爬取，失败时指数退避重试
func crawlWithRetryInContext(allocCtx context.Context, targetURL string, config *crawler.Config, outputDir string) (attempts int, err error) {
	u, parseErr := url.Parse(targetURL)
	if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") {
		scheme := ""
		if u != nil {
			scheme = u.Scheme
		}
		return 0, fmt.Errorf("不支持的 URL scheme %q：仅允许 http 和 https", scheme)
	}

	maxAttempts := config.MaxRetry + 1
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			backoff := time.Duration(attempt-1) * 3 * time.Second
			log.Printf("  [重试 %d/%d] 等待 %v: %s", attempt-1, config.MaxRetry, backoff, targetURL)
			time.Sleep(backoff)
		}

		spider := crawler.New(config)
		if err := spider.CrawlInContext(allocCtx, targetURL); err != nil {
			lastErr = err
			log.Printf("  [尝试 %d/%d] 失败: %v", attempt, maxAttempts, err)
			continue
		}

		processResources(spider, targetURL, outputDir, true)
		return attempt, nil
	}

	return maxAttempts, fmt.Errorf("已重试 %d 次，最后错误: %w", config.MaxRetry, lastErr)
}

// processResources 处理爬取到的资源：提取 source map、保存文件、生成报告。
// flatStorage=true 时使用扁平路径（批量模式的 outputDir 已含 hostname）。
func processResources(spider *crawler.Spider, targetURL, outputDir string, flatStorage bool) {
	resources := spider.GetResources()
	log.Printf("成功抓取 %d 个资源", len(resources))

	log.Printf("正在提取 Source Maps...")
	extractor := sourcemap.New(targetURL)
	sourceMapResources := make(map[string]*crawler.Resource)

	for _, res := range resources {
		sourceFiles, err := extractor.ExtractFromResource(res)
		if err != nil {
			log.Printf("警告: 提取 source map 失败: %v", err)
			continue
		}
		for _, sourceFile := range sourceFiles {
			sourceMapResources[sourceFile.URL] = sourceFile
		}
	}

	log.Printf("从 Source Maps 提取了 %d 个源文件", len(sourceMapResources))
	maps.Copy(resources, sourceMapResources)
	log.Printf("总共 %d 个资源（包括源文件）", len(resources))

	log.Printf("正在保存资源到: %s", outputDir)
	var store *storage.Storage
	if flatStorage {
		store = storage.NewFlat(outputDir)
	} else {
		store = storage.New(outputDir)
	}

	if err := store.Save(resources); err != nil {
		log.Printf("保存资源失败: %v", err)
		return
	}

	if err := store.GenerateReport(resources); err != nil {
		log.Printf("警告: 生成报告失败: %v", err)
	}

	log.Printf("完成! 所有资源已保存到: %s", outputDir)
}

// buildBatchOutputDir 根据 URL hostname 生成批量模式的输出目录名
// 重复 hostname 自动加数字后缀（example.com → example.com_2）
func buildBatchOutputDir(baseDir, rawURL string, usedDirs map[string]int) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fallbackDir(baseDir, "unknown", usedDirs)
	}
	host := strings.ReplaceAll(u.Host, ":", "_") // 端口号 : 替换为 _
	return fallbackDir(baseDir, host, usedDirs)
}

func fallbackDir(baseDir, key string, usedDirs map[string]int) string {
	count := usedDirs[key]
	usedDirs[key]++
	if count == 0 {
		return filepath.Join(baseDir, key)
	}
	return filepath.Join(baseDir, fmt.Sprintf("%s_%d", key, count+1))
}

// writeManifest 将爬取结果清单写入 manifest.json
func writeManifest(baseDir string, entries []ManifestEntry) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		log.Printf("警告: 无法创建输出目录写 manifest: %v", err)
		return
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Printf("警告: 序列化 manifest 失败: %v", err)
		return
	}

	path := filepath.Join(baseDir, "manifest.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("警告: 写入 manifest.json 失败: %v", err)
	}
}

// readURLsFromFile 从文件读取URL列表，跳过空行和注释，规范化后去重
func readURLsFromFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件: %w", err)
	}
	defer file.Close()

	var urls []string
	seen := make(map[string]int) // normalized URL → 首次出现行号
	lineNum := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		normalized, err := normalizeURL(line)
		if err != nil {
			log.Printf("警告: 第 %d 行 URL 无效，已跳过: %s (%v)", lineNum, line, err)
			continue
		}

		if firstLine, dup := seen[normalized]; dup {
			log.Printf("警告: 第 %d 行与第 %d 行重复，已跳过: %s", lineNum, firstLine, line)
			continue
		}

		seen[normalized] = lineNum
		urls = append(urls, normalized)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("文件中没有有效的URL")
	}

	return urls, nil
}

// normalizeURL 规范化 URL：统一 scheme/host 大小写，补全路径，去掉默认端口
func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("scheme 不支持: %s", u.Scheme)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	// 去掉默认端口（80/443）
	if (u.Scheme == "http" && strings.HasSuffix(u.Host, ":80")) ||
		(u.Scheme == "https" && strings.HasSuffix(u.Host, ":443")) {
		u.Host = u.Host[:strings.LastIndex(u.Host, ":")]
	}
	// 空路径补 /，但不强制末尾 /（保留原始路径语义）
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func showUsage() {
	fmt.Fprintf(os.Stderr, `Spider - 浏览器模拟爬虫工具

用法:
  spider -url <目标URL> [选项]
  spider -file <URL文件> [选项]

选项:
  -url string        目标网页URL（与 -file 二选一）
  -file string       URL文件路径，每行一个URL（与 -url 二选一）
  -output string     输出目录 (默认 "./output")
  -timeout int       爬取超时时间，单位秒 (默认 30)
  -idle-timeout int  网络空闲等待上限，单位秒 (默认 10)；
                     取代固定延迟，检测到连续 2s 无新资源则提前结束
  -retry int         失败重试次数，指数退避 (默认 2)
  -cookie string     Cookie字符串，格式: "key1=value1; key2=value2"
  -header string     自定义Header，格式: "Key:Value"（可多次使用）
  -proxy string      HTTP/SOCKS5代理地址，如 "http://127.0.0.1:8080"
  -ua string         自定义 User-Agent
  -chrome-path string Chrome/Chromium 可执行文件路径（默认自动搜索）
  -concurrency int   并发数，批量爬取时生效 (默认 1)
  -headless bool     无头模式 (默认 true)
  -help              显示此帮助信息

批量模式输出结构:
  output/
  ├── manifest.json          URL → 目录映射 + 成败记录
  ├── example.com/           按 hostname 命名
  │   └── index.html
  ├── another.com/
  │   └── ...
  └── another.com_2/         重复 hostname 自动加后缀

示例:
  spider -url https://example.com
  spider -url https://example.com -cookie "session=abc123"
  spider -url https://example.com -header "Authorization:Bearer token"
  spider -url https://example.com -proxy http://127.0.0.1:8080
  spider -file urls.txt -concurrency 3 -retry 3
  spider -url https://example.com -headless=false

`)
}
