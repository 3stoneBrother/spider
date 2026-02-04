package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/3stoneBrother/spider/internal/crawler"
	"github.com/3stoneBrother/spider/internal/sourcemap"
	"github.com/3stoneBrother/spider/internal/storage"
)

// headerFlags 用于支持多次使用 -header 参数
type headerFlags []string

func (h *headerFlags) String() string {
	return strings.Join(*h, ", ")
}

func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func main() {
	// 命令行参数
	var (
		targetURL   string
		urlFile     string
		outputDir   string
		timeout     int
		cookie      string
		headers     headerFlags
		proxy       string
		userAgent   string
		concurrency int
		headless    bool
		showHelp    bool
	)

	flag.StringVar(&targetURL, "url", "", "目标网页URL（与 -file 二选一）")
	flag.StringVar(&urlFile, "file", "", "URL文件路径，每行一个URL（与 -url 二选一）")
	flag.StringVar(&outputDir, "output", "./output", "输出目录")
	flag.IntVar(&timeout, "timeout", 30, "爬取超时时间（秒）")
	flag.StringVar(&cookie, "cookie", "", "Cookie字符串，格式: \"key1=value1; key2=value2\"")
	flag.Var(&headers, "header", "自定义Header，格式: \"Key:Value\"（可多次使用）")
	flag.StringVar(&proxy, "proxy", "", "HTTP/SOCKS5代理地址，如 \"http://127.0.0.1:8080\"")
	flag.StringVar(&userAgent, "ua", "", "自定义 User-Agent")
	flag.IntVar(&concurrency, "concurrency", 1, "并发数（批量爬取时）")
	flag.BoolVar(&headless, "headless", true, "无头模式（默认true）")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")

	flag.Parse()

	// 显示帮助
	if showHelp {
		showUsage()
		return
	}

	// 校验参数
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

	// 解析 headers
	headerMap := make(map[string]string)
	for _, h := range headers {
		idx := strings.Index(h, ":")
		if idx == -1 {
			log.Printf("警告: 忽略无效的 Header 格式: %s (应为 Key:Value)", h)
			continue
		}
		key := strings.TrimSpace(h[:idx])
		value := strings.TrimSpace(h[idx+1:])
		headerMap[key] = value
	}

	// 构建配置
	config := &crawler.Config{
		Timeout:     time.Duration(timeout) * time.Second,
		Cookies:     cookie,
		Headers:     headerMap,
		Proxy:       proxy,
		UserAgent:   userAgent,
		Headless:    headless,
		Concurrency: concurrency,
	}

	log.Printf("Spider - 浏览器模拟爬虫工具")
	log.Printf("================================")
	log.Printf("输出目录: %s", outputDir)
	log.Printf("超时时间: %d秒", timeout)
	log.Printf("无头模式: %v", headless)
	if cookie != "" {
		log.Printf("Cookie: %s", cookie)
	}
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
		log.Printf("从文件读取了 %d 个URL", len(urls))
		log.Printf("并发数: %d", concurrency)
	}

	// 执行爬取
	if len(urls) == 1 {
		// 单URL模式
		crawlSingleURL(urls[0], config, outputDir)
	} else {
		// 批量模式
		crawlMultipleURLs(urls, config, outputDir, concurrency)
	}
}

// crawlSingleURL 爬取单个URL
func crawlSingleURL(targetURL string, config *crawler.Config, outputDir string) {
	log.Printf("目标URL: %s", targetURL)

	// 创建爬虫实例
	spider := crawler.New(config)

	// 开始爬取
	log.Printf("开始爬取网页...")
	if err := spider.Crawl(targetURL); err != nil {
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

详细信息请查看 README.md
`)
		}
		os.Exit(1)
	}

	// 处理资源
	processResources(spider, targetURL, outputDir)
}

// crawlMultipleURLs 批量爬取多个URL
func crawlMultipleURLs(urls []string, config *crawler.Config, baseOutputDir string, concurrency int) {
	// 使用 semaphore 控制并发
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// 结果统计
	var (
		successCount int
		failCount    int
		mu           sync.Mutex
	)

	for i, url := range urls {
		wg.Add(1)
		sem <- struct{}{} // 获取信号量

		go func(idx int, targetURL string) {
			defer wg.Done()
			defer func() { <-sem }() // 释放信号量

			log.Printf("[%d/%d] 开始爬取: %s", idx+1, len(urls), targetURL)

			// 创建爬虫实例
			spider := crawler.New(config)

			// 生成输出目录（基于URL索引）
			outputDir := fmt.Sprintf("%s/url_%d", baseOutputDir, idx+1)

			// 执行爬取
			if err := spider.Crawl(targetURL); err != nil {
				log.Printf("[%d/%d] 爬取失败: %s - %v", idx+1, len(urls), targetURL, err)
				mu.Lock()
				failCount++
				mu.Unlock()
				return
			}

			// 处理资源
			processResources(spider, targetURL, outputDir)

			mu.Lock()
			successCount++
			mu.Unlock()

			log.Printf("[%d/%d] 完成: %s", idx+1, len(urls), targetURL)
		}(i, url)
	}

	wg.Wait()

	log.Printf("\n================================")
	log.Printf("批量爬取完成!")
	log.Printf("成功: %d, 失败: %d, 总计: %d", successCount, failCount, len(urls))
	log.Printf("================================")
}

// processResources 处理爬取到的资源
func processResources(spider *crawler.Spider, targetURL, outputDir string) {
	// 获取资源
	resources := spider.GetResources()
	log.Printf("成功抓取 %d 个资源", len(resources))

	// 提取 Source Maps
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

	// 合并资源
	for url, res := range sourceMapResources {
		resources[url] = res
	}

	log.Printf("总共 %d 个资源（包括源文件）", len(resources))

	// 保存资源
	log.Printf("正在保存资源到: %s", outputDir)
	store := storage.New(outputDir)

	if err := store.Save(resources); err != nil {
		log.Printf("保存资源失败: %v", err)
		return
	}

	// 生成报告
	if err := store.GenerateReport(resources); err != nil {
		log.Printf("警告: 生成报告失败: %v", err)
	}

	log.Printf("完成! 所有资源已保存到: %s", outputDir)
}

// readURLsFromFile 从文件读取URL列表
func readURLsFromFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件: %w", err)
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("文件中没有有效的URL")
	}

	return urls, nil
}

func showUsage() {
	fmt.Fprintf(os.Stderr, `Spider - 浏览器模拟爬虫工具

用法:
  spider -url <目标URL> [选项]
  spider -file <URL文件> [选项]

选项:
  -url string
        目标网页URL（与 -file 二选一）
  -file string
        URL文件路径，每行一个URL（与 -url 二选一）
  -output string
        输出目录 (默认 "./output")
  -timeout int
        爬取超时时间，单位秒 (默认 30)
  -cookie string
        Cookie字符串，格式: "key1=value1; key2=value2"
  -header string
        自定义Header，格式: "Key:Value"（可多次使用）
  -proxy string
        HTTP/SOCKS5代理地址，如 "http://127.0.0.1:8080"
  -ua string
        自定义 User-Agent
  -concurrency int
        并发数，批量爬取时生效 (默认 1)
  -headless bool
        无头模式 (默认 true)
  -help
        显示此帮助信息

示例:
  # 基本使用
  spider -url https://example.com

  # 使用 Cookie
  spider -url https://example.com -cookie "session=abc123; token=xyz"

  # 自定义 Header
  spider -url https://example.com -header "Authorization:Bearer token" -header "X-Custom:value"

  # 使用代理
  spider -url https://example.com -proxy http://127.0.0.1:8080

  # 批量爬取
  spider -file urls.txt -concurrency 3

  # 可视化模式（调试）
  spider -url https://example.com -headless=false

功能特性:
  - 模拟真实浏览器加载网页
  - 执行JavaScript动态加载的内容
  - 抓取所有网络请求的资源
  - 自动提取 Source Maps 中的源代码文件
  - 按域名和路径组织文件树结构
  - 生成详细的抓取报告
  - 支持 Cookie、自定义 Headers、代理
  - 支持批量 URL 并发爬取

`)
}
